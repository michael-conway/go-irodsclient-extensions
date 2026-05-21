package usersync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	irodscommon "github.com/cyverse/go-irodsclient/irods/common"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

var (
	ErrNotFound         = errors.New("usersync: not found")
	ErrPermissionDenied = errors.New("usersync: permission denied")
	ErrConflict         = errors.New("usersync: conflict")
	ErrInvalidRequest   = errors.New("usersync: invalid request")
	ErrPolicyViolation  = errors.New("usersync: policy violation")
)

const userNotInGroupErrorCode irodscommon.ErrorCode = -1830000

// Outcome describes the action taken to converge iRODS user/group state.
type Outcome string

const (
	OutcomeCreated       Outcome = "created"
	OutcomeUpdated       Outcome = "updated"
	OutcomeDeleted       Outcome = "deleted"
	OutcomeAdded         Outcome = "added"
	OutcomeRemoved       Outcome = "removed"
	OutcomeAlreadyExists Outcome = "already_exists"
	OutcomeAlreadyAbsent Outcome = "already_absent"
	OutcomeAlreadyMember Outcome = "already_member"
)

// Filesystem is the narrow user/group API required for desired-state sync.
type Filesystem interface {
	GetUser(username string, zoneName string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error)
	ListGroupMembers(zoneName string, groupName string) ([]*irodstypes.IRODSUser, error)
	ListUserMetadata(username string, zoneName string) ([]*irodstypes.IRODSMeta, error)
	AddUserMetadata(username string, zoneName string, attribute string, value string, unit string) error
	CreateUser(username string, zoneName string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error)
	CreateUserGroup(groupName string, zoneName string) (*irodstypes.IRODSUser, error)
	ChangeUserPassword(username string, zoneName string, newPassword string) error
	ChangeUserType(username string, zoneName string, newType irodstypes.IRODSUserType) error
	RemoveUser(username string, zoneName string, userType irodstypes.IRODSUserType) error
	RemoveUserGroup(groupName string, zoneName string) error
	AddGroupMember(groupName string, username string, zoneName string) error
	RemoveGroupMember(groupName string, username string, zoneName string) error
}

// Service reconciles user and group desired state against an iRODS catalog.
type Service struct {
	filesystem Filesystem
	zone       string
	policy     Policy
}

// NewService returns a desired-state user/group sync service.
func NewService(filesystem Filesystem, defaultZone string, options ...Option) *Service {
	service := &Service{
		filesystem: filesystem,
		zone:       strings.TrimSpace(defaultZone),
		policy:     DefaultPolicy(),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	service.policy = service.policy.normalize()
	return service
}

type User struct {
	ID   int64
	Name string
	Zone string
	Type string
}

type Group struct {
	ID      int64
	Name    string
	Zone    string
	Type    string
	Members []GroupMember
}

type GroupMember struct {
	ID   int64
	Name string
	Zone string
	Type string
}

type EnsureUserRequest struct {
	Name     string
	Zone     string
	Type     irodstypes.IRODSUserType
	Password string
}

type UpdateUserRequest struct {
	Name            string
	Zone            string
	Type            irodstypes.IRODSUserType
	Password        string
	ChangeType      bool
	ChangePassword  bool
	CreateIfMissing bool
}

type UserRef struct {
	Name string
	Zone string
}

type GroupRef struct {
	Name string
	Zone string
}

type GroupMemberRef struct {
	GroupName string
	UserName  string
	Zone      string
}

type UserResult struct {
	User    User
	Outcome Outcome
}

type GroupResult struct {
	Group   Group
	Outcome Outcome
}

// EnsureUser creates a user, or validates an existing matching user when it is
// already present.
func (service *Service) EnsureUser(ctx context.Context, request EnsureUserRequest) (UserResult, error) {
	if err := service.validate(); err != nil {
		return UserResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return UserResult{}, err
	}

	username := strings.TrimSpace(request.Name)
	zone := service.zoneFor(request.Zone)
	userType := request.Type
	if username == "" {
		return UserResult{}, fmt.Errorf("%w: user name is required", ErrInvalidRequest)
	}
	if err := service.requireCreateUserType(userType); err != nil {
		return UserResult{}, err
	}

	if _, err := service.filesystem.CreateUser(username, zone, userType); err != nil {
		normalizedErr := NormalizeUserError("create user", username, zone, err)
		if errors.Is(normalizedErr, ErrConflict) {
			return service.reconcileExistingUser(ctx, username, zone, userType, request.Password)
		}
		return UserResult{}, normalizedErr
	}

	if strings.TrimSpace(request.Password) != "" {
		if err := service.filesystem.ChangeUserPassword(username, zone, request.Password); err != nil {
			return UserResult{}, NormalizeUserError("set user password", username, zone, err)
		}
	}

	user, err := service.filesystem.GetUser(username, zone, "")
	if err != nil {
		return UserResult{}, NormalizeUserError("get user", username, zone, err)
	}
	if err := service.markCreatedPrincipal(username, zone); err != nil {
		return UserResult{}, err
	}

	return UserResult{
		User:    mapUser(user),
		Outcome: OutcomeCreated,
	}, nil
}

// UpdateUser applies type and password changes to an existing user. When
// CreateIfMissing is set, a missing user is created only if a target type is
// part of the requested change.
func (service *Service) UpdateUser(ctx context.Context, request UpdateUserRequest) (UserResult, error) {
	if err := service.validate(); err != nil {
		return UserResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return UserResult{}, err
	}

	username := strings.TrimSpace(request.Name)
	zone := service.zoneFor(request.Zone)
	if username == "" {
		return UserResult{}, fmt.Errorf("%w: user name is required", ErrInvalidRequest)
	}
	if request.ChangeType {
		if err := service.requireCreateUserType(request.Type); err != nil {
			return UserResult{}, err
		}
	}

	existing, err := service.filesystem.GetUser(username, zone, "")
	if err != nil {
		normalizedErr := NormalizeUserError("get user", username, zone, err)
		if errors.Is(normalizedErr, ErrNotFound) && request.CreateIfMissing && request.ChangeType {
			password := ""
			if request.ChangePassword {
				password = request.Password
			}
			return service.EnsureUser(ctx, EnsureUserRequest{
				Name:     username,
				Zone:     zone,
				Type:     request.Type,
				Password: password,
			})
		}
		return UserResult{}, normalizedErr
	}
	if err := service.requireManageUserType(existing.Type, "update user"); err != nil {
		return UserResult{}, err
	}
	if !isPrincipalUserType(existing.Type) {
		return UserResult{}, fmt.Errorf("%w: user %q", ErrNotFound, username)
	}
	if err := service.reconcileExistingSyncMetadata(username, zone); err != nil {
		return UserResult{}, err
	}

	outcome := OutcomeAlreadyExists
	if request.ChangeType && existing.Type != request.Type {
		if err := service.filesystem.ChangeUserType(username, zone, request.Type); err != nil {
			return UserResult{}, NormalizeUserError("update user type", username, zone, err)
		}
		existing.Type = request.Type
		outcome = OutcomeUpdated
	}

	if request.ChangePassword {
		if err := service.filesystem.ChangeUserPassword(username, zone, request.Password); err != nil {
			return UserResult{}, NormalizeUserError("update user password", username, zone, err)
		}
		outcome = OutcomeUpdated
	}

	return UserResult{
		User:    mapUser(existing),
		Outcome: outcome,
	}, nil
}

// EnsureUserAbsent removes a user, or reports an already-absent outcome for a
// missing user or a non-user principal with the same name.
func (service *Service) EnsureUserAbsent(ctx context.Context, request UserRef) (UserResult, error) {
	if err := service.validate(); err != nil {
		return UserResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return UserResult{}, err
	}

	username := strings.TrimSpace(request.Name)
	zone := service.zoneFor(request.Zone)
	if username == "" {
		return UserResult{}, fmt.Errorf("%w: user name is required", ErrInvalidRequest)
	}

	user, err := service.filesystem.GetUser(username, zone, "")
	if err != nil {
		normalizedErr := NormalizeUserError("get user", username, zone, err)
		if errors.Is(normalizedErr, ErrNotFound) {
			return UserResult{
				User:    User{Name: username, Zone: zone},
				Outcome: OutcomeAlreadyAbsent,
			}, nil
		}
		return UserResult{}, normalizedErr
	}
	if !isPrincipalUserType(user.Type) {
		return UserResult{
			User:    User{Name: username, Zone: zone, Type: string(user.Type)},
			Outcome: OutcomeAlreadyAbsent,
		}, nil
	}
	if err := service.requireManageUserType(user.Type, "delete user"); err != nil {
		return UserResult{}, err
	}
	if err := service.requireDeleteAllowed(username, zone, "delete user"); err != nil {
		return UserResult{}, err
	}

	if err := service.filesystem.RemoveUser(username, zone, user.Type); err != nil {
		normalizedErr := NormalizeUserError("remove user", username, zone, err)
		if errors.Is(normalizedErr, ErrNotFound) {
			return UserResult{
				User:    mapUser(user),
				Outcome: OutcomeAlreadyAbsent,
			}, nil
		}
		return UserResult{}, normalizedErr
	}

	return UserResult{
		User:    mapUser(user),
		Outcome: OutcomeDeleted,
	}, nil
}

// EnsureGroup creates a group, or validates an existing group when it is
// already present.
func (service *Service) EnsureGroup(ctx context.Context, request GroupRef) (GroupResult, error) {
	if err := service.validate(); err != nil {
		return GroupResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return GroupResult{}, err
	}

	groupName := strings.TrimSpace(request.Name)
	zone := service.zoneFor(request.Zone)
	if groupName == "" {
		return GroupResult{}, fmt.Errorf("%w: group name is required", ErrInvalidRequest)
	}

	if _, err := service.filesystem.CreateUserGroup(groupName, zone); err != nil {
		normalizedErr := NormalizeGroupError("create group", groupName, zone, err)
		if errors.Is(normalizedErr, ErrConflict) {
			group, groupErr := service.groupWithMembers(groupName, zone)
			if groupErr != nil {
				return GroupResult{}, groupErr
			}
			if metadataErr := service.reconcileExistingSyncMetadata(groupName, zone); metadataErr != nil {
				return GroupResult{}, metadataErr
			}
			return GroupResult{Group: group, Outcome: OutcomeAlreadyExists}, nil
		}
		return GroupResult{}, normalizedErr
	}

	group, err := service.groupWithMembers(groupName, zone)
	if err != nil {
		return GroupResult{}, err
	}
	if err := service.markCreatedPrincipal(groupName, zone); err != nil {
		return GroupResult{}, err
	}
	return GroupResult{Group: group, Outcome: OutcomeCreated}, nil
}

// EnsureGroupAbsent removes a group, or reports an already-absent outcome for a
// missing group.
func (service *Service) EnsureGroupAbsent(ctx context.Context, request GroupRef) (GroupResult, error) {
	if err := service.validate(); err != nil {
		return GroupResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return GroupResult{}, err
	}

	groupName := strings.TrimSpace(request.Name)
	zone := service.zoneFor(request.Zone)
	if groupName == "" {
		return GroupResult{}, fmt.Errorf("%w: group name is required", ErrInvalidRequest)
	}

	group, err := service.getGroup(groupName, zone)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return GroupResult{
				Group:   Group{Name: groupName, Zone: zone, Type: string(irodstypes.IRODSUserRodsGroup)},
				Outcome: OutcomeAlreadyAbsent,
			}, nil
		}
		return GroupResult{}, err
	}
	if err := service.requireDeleteAllowed(groupName, zone, "delete group"); err != nil {
		return GroupResult{}, err
	}

	if err := service.filesystem.RemoveUserGroup(groupName, zone); err != nil {
		normalizedErr := NormalizeGroupError("remove group", groupName, zone, err)
		if errors.Is(normalizedErr, ErrNotFound) {
			return GroupResult{Group: group, Outcome: OutcomeAlreadyAbsent}, nil
		}
		return GroupResult{}, normalizedErr
	}

	return GroupResult{Group: group, Outcome: OutcomeDeleted}, nil
}

// EnsureGroupMember adds a user to a group, or reports an already-member
// outcome when the membership is already present.
func (service *Service) EnsureGroupMember(ctx context.Context, request GroupMemberRef) (GroupResult, error) {
	if err := service.validate(); err != nil {
		return GroupResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return GroupResult{}, err
	}

	groupName := strings.TrimSpace(request.GroupName)
	username := strings.TrimSpace(request.UserName)
	zone := service.zoneFor(request.Zone)
	if groupName == "" {
		return GroupResult{}, fmt.Errorf("%w: group name is required", ErrInvalidRequest)
	}
	if username == "" {
		return GroupResult{}, fmt.Errorf("%w: user name is required", ErrInvalidRequest)
	}

	group, err := service.getGroup(groupName, zone)
	if err != nil {
		return GroupResult{}, err
	}
	if err := service.reconcileExistingSyncMetadata(group.Name, group.Zone); err != nil {
		return GroupResult{}, err
	}

	user, err := service.filesystem.GetUser(username, zone, "")
	if err != nil {
		return GroupResult{}, NormalizeUserError("get user", username, zone, err)
	}
	if user == nil || !isPrincipalUserType(user.Type) {
		return GroupResult{}, fmt.Errorf("%w: user %q", ErrNotFound, username)
	}
	if err := service.requireManageUserType(user.Type, "add group member"); err != nil {
		return GroupResult{}, err
	}
	if err := service.reconcileExistingSyncMetadata(user.Name, user.Zone); err != nil {
		return GroupResult{}, err
	}

	if err := service.filesystem.AddGroupMember(group.Name, user.Name, zone); err != nil {
		normalizedErr := NormalizeGroupError("add group member", groupName, zone, err)
		if errors.Is(normalizedErr, ErrConflict) {
			groupWithMembers, groupErr := service.groupWithMembers(groupName, zone)
			if groupErr != nil {
				return GroupResult{}, groupErr
			}
			return GroupResult{Group: groupWithMembers, Outcome: OutcomeAlreadyMember}, nil
		}
		return GroupResult{}, normalizedErr
	}

	groupWithMembers, err := service.groupWithMembers(groupName, zone)
	if err != nil {
		return GroupResult{}, err
	}
	return GroupResult{Group: groupWithMembers, Outcome: OutcomeAdded}, nil
}

// EnsureGroupMemberAbsent removes a user from a group, or reports an
// already-absent outcome when the user is not a current member.
func (service *Service) EnsureGroupMemberAbsent(ctx context.Context, request GroupMemberRef) (GroupResult, error) {
	if err := service.validate(); err != nil {
		return GroupResult{}, err
	}
	if err := ctxErr(ctx); err != nil {
		return GroupResult{}, err
	}

	groupName := strings.TrimSpace(request.GroupName)
	username := strings.TrimSpace(request.UserName)
	zone := service.zoneFor(request.Zone)
	if groupName == "" {
		return GroupResult{}, fmt.Errorf("%w: group name is required", ErrInvalidRequest)
	}
	if username == "" {
		return GroupResult{}, fmt.Errorf("%w: user name is required", ErrInvalidRequest)
	}

	group, err := service.getGroup(groupName, zone)
	if err != nil {
		return GroupResult{}, err
	}
	if err := service.reconcileExistingSyncMetadata(group.Name, group.Zone); err != nil {
		return GroupResult{}, err
	}
	user, err := service.filesystem.GetUser(username, zone, "")
	if err != nil {
		normalizedErr := NormalizeUserError("get user", username, zone, err)
		if !errors.Is(normalizedErr, ErrNotFound) {
			return GroupResult{}, normalizedErr
		}
	} else if user != nil && isPrincipalUserType(user.Type) {
		if err := service.requireManageUserType(user.Type, "remove group member"); err != nil {
			return GroupResult{}, err
		}
	}
	if err := service.requireMembershipRemovalAllowed(group.Name, zone); err != nil {
		return GroupResult{}, err
	}

	if err := service.filesystem.RemoveGroupMember(group.Name, username, zone); err != nil {
		normalizedErr := NormalizeGroupError("remove group member", groupName, zone, err)
		if errors.Is(normalizedErr, ErrNotFound) {
			groupWithMembers, groupErr := service.groupWithMembers(groupName, zone)
			if groupErr != nil {
				return GroupResult{}, groupErr
			}
			return GroupResult{Group: groupWithMembers, Outcome: OutcomeAlreadyAbsent}, nil
		}
		return GroupResult{}, normalizedErr
	}

	groupWithMembers, err := service.groupWithMembers(groupName, zone)
	if err != nil {
		return GroupResult{}, err
	}
	return GroupResult{Group: groupWithMembers, Outcome: OutcomeRemoved}, nil
}

func (service *Service) reconcileExistingUser(ctx context.Context, username string, zone string, requestedType irodstypes.IRODSUserType, password string) (UserResult, error) {
	if err := ctxErr(ctx); err != nil {
		return UserResult{}, err
	}

	existing, err := service.filesystem.GetUser(username, zone, "")
	if err != nil {
		return UserResult{}, NormalizeUserError("get existing user", username, zone, err)
	}
	if existing == nil || !isPrincipalUserType(existing.Type) {
		return UserResult{}, fmt.Errorf("%w: existing principal %q has type %q, expected %q", ErrConflict, username, existingType(existing), requestedType)
	}
	if err := service.requireManageUserType(existing.Type, "claim existing user"); err != nil {
		return UserResult{}, err
	}
	if existing.Type != requestedType {
		return UserResult{}, fmt.Errorf("%w: existing user %q has type %q, expected %q", ErrConflict, username, existing.Type, requestedType)
	}
	if err := service.reconcileExistingSyncMetadata(username, zone); err != nil {
		return UserResult{}, err
	}

	if strings.TrimSpace(password) != "" {
		if err := service.filesystem.ChangeUserPassword(username, zone, password); err != nil {
			return UserResult{}, NormalizeUserError("reconcile user password", username, zone, err)
		}
	}

	return UserResult{
		User:    mapUser(existing),
		Outcome: OutcomeAlreadyExists,
	}, nil
}

func (service *Service) getGroup(groupName string, zone string) (Group, error) {
	group, err := service.filesystem.GetUser(groupName, zone, irodstypes.IRODSUserRodsGroup)
	if err != nil {
		return Group{}, NormalizeGroupError("get group", groupName, zone, err)
	}

	mapped := mapGroup(group)
	if mapped.Name == "" || mapped.Type != string(irodstypes.IRODSUserRodsGroup) {
		return Group{}, fmt.Errorf("%w: group %q", ErrNotFound, groupName)
	}
	return mapped, nil
}

func (service *Service) groupWithMembers(groupName string, zone string) (Group, error) {
	group, err := service.getGroup(groupName, zone)
	if err != nil {
		return Group{}, err
	}

	members, err := service.filesystem.ListGroupMembers(zone, groupName)
	if err != nil {
		return Group{}, NormalizeGroupError("list group members", groupName, zone, err)
	}
	group.Members = mapGroupMembers(members)
	return group, nil
}

func (service *Service) requireCreateUserType(userType irodstypes.IRODSUserType) error {
	if userType == "" {
		return fmt.Errorf("%w: user type is required", ErrInvalidRequest)
	}
	if userTypeIn(userType, service.policy.ProtectedUserTypes) {
		return fmt.Errorf("%w: sync cannot create or manage protected iRODS user type %q", ErrPolicyViolation, userType)
	}
	if !userTypeIn(userType, service.policy.AllowedCreateUserTypes) {
		return fmt.Errorf("%w: sync cannot create iRODS user type %q", ErrPolicyViolation, userType)
	}
	return nil
}

func (service *Service) requireManageUserType(userType irodstypes.IRODSUserType, operation string) error {
	if userTypeIn(userType, service.policy.ProtectedUserTypes) {
		return fmt.Errorf("%w: %s cannot manage protected iRODS user type %q", ErrPolicyViolation, operation, userType)
	}
	if !userTypeIn(userType, service.policy.AllowedManageUserTypes) {
		return fmt.Errorf("%w: %s cannot manage iRODS user type %q", ErrPolicyViolation, operation, userType)
	}
	return nil
}

func (service *Service) markCreatedPrincipal(name string, zone string) error {
	if !service.policy.MarkCreated {
		return nil
	}
	return service.ensureManagedMetadata(name, zone)
}

func (service *Service) reconcileExistingSyncMetadata(name string, zone string) error {
	status, err := service.syncMetadataStatus(name, zone)
	if err != nil {
		return err
	}
	if status.conflicting {
		return fmt.Errorf("%w: existing principal %q has conflicting %s metadata", ErrConflict, name, AVUAttributePrefix)
	}
	if service.policy.ClaimExisting || status.managed {
		return service.ensureManagedMetadata(name, zone)
	}
	return nil
}

func (service *Service) requireDeleteAllowed(name string, zone string, operation string) error {
	if service.policy.AllowUnmanagedDeletes {
		return nil
	}
	status, err := service.syncMetadataStatus(name, zone)
	if err != nil {
		return err
	}
	if status.conflicting {
		return fmt.Errorf("%w: %s target %q has conflicting %s metadata", ErrConflict, operation, name, AVUAttributePrefix)
	}
	if !status.managed {
		return fmt.Errorf("%w: %s target %q is not marked with %s", ErrPolicyViolation, operation, name, AVUAttributeManaged)
	}
	return nil
}

func (service *Service) requireMembershipRemovalAllowed(groupName string, zone string) error {
	if service.policy.AllowUnmanagedMembershipRemovals {
		return nil
	}
	status, err := service.syncMetadataStatus(groupName, zone)
	if err != nil {
		return err
	}
	if status.conflicting {
		return fmt.Errorf("%w: group %q has conflicting %s metadata", ErrConflict, groupName, AVUAttributePrefix)
	}
	if !status.managed {
		return fmt.Errorf("%w: group %q is not marked with %s", ErrPolicyViolation, groupName, AVUAttributeManaged)
	}
	return nil
}

func (service *Service) ensureManagedMetadata(name string, zone string) error {
	metadata, err := service.filesystem.ListUserMetadata(name, zone)
	if err != nil {
		return NormalizeUserError("list user sync metadata", name, zone, err)
	}

	for _, avu := range service.policy.managedAVUs() {
		if hasAVU(metadata, avu) {
			continue
		}
		if err := service.filesystem.AddUserMetadata(name, zone, avu.Attribute, avu.Value, avu.Unit); err != nil {
			normalizedErr := NormalizeUserError("add user sync metadata", name, zone, err)
			if errors.Is(normalizedErr, ErrConflict) {
				continue
			}
			return normalizedErr
		}
	}
	return nil
}

type syncMetadataStatus struct {
	managed     bool
	conflicting bool
}

func (service *Service) syncMetadataStatus(name string, zone string) (syncMetadataStatus, error) {
	metadata, err := service.filesystem.ListUserMetadata(name, zone)
	if err != nil {
		return syncMetadataStatus{}, NormalizeUserError("list user sync metadata", name, zone, err)
	}

	return classifySyncMetadata(metadata, service.policy.Metadata), nil
}

func classifySyncMetadata(metadata []*irodstypes.IRODSMeta, expected SyncMetadata) syncMetadataStatus {
	expected = expected.normalize()
	status := syncMetadataStatus{}

	for _, meta := range metadata {
		if meta == nil {
			continue
		}
		attribute := strings.TrimSpace(meta.Name)
		if !strings.HasPrefix(attribute, AVUAttributePrefix+":") && attribute != AVUAttributePrefix {
			continue
		}

		value := strings.TrimSpace(meta.Value)
		unit := strings.TrimSpace(meta.Units)
		switch attribute {
		case AVUAttributeManaged:
			if strings.EqualFold(value, AVUValueTrue) {
				if expected.Source == "" || unit == "" || unit == expected.Source {
					status.managed = true
				} else {
					status.conflicting = true
				}
			}
		case AVUAttributeSource:
			if expected.Source != "" && value != "" && value != expected.Source {
				status.conflicting = true
			}
		case AVUAttributeRealm:
			if expected.Realm != "" && value != "" && value != expected.Realm {
				status.conflicting = true
			}
		}
	}

	return status
}

func hasAVU(metadata []*irodstypes.IRODSMeta, avu AVU) bool {
	for _, meta := range metadata {
		if meta == nil {
			continue
		}
		if strings.TrimSpace(meta.Name) == avu.Attribute &&
			strings.TrimSpace(meta.Value) == avu.Value &&
			strings.TrimSpace(meta.Units) == avu.Unit {
			return true
		}
	}
	return false
}

func (service *Service) validate() error {
	if service == nil || service.filesystem == nil {
		return fmt.Errorf("%w: filesystem is required", ErrInvalidRequest)
	}
	return nil
}

func (service *Service) zoneFor(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone != "" {
		return zone
	}
	return service.zone
}

// NormalizeUserError maps common go-irodsclient and iRODS catalog errors into
// usersync sentinel errors with operation context.
func NormalizeUserError(operation string, username string, zone string, err error) error {
	if err == nil {
		return nil
	}
	if IsKnownError(err) {
		return err
	}

	switch irodstypes.GetIRODSErrorCode(err) {
	case irodscommon.CAT_NO_ACCESS_PERMISSION, irodscommon.SYS_NO_API_PRIV:
		return fmt.Errorf("%w: user %q", ErrPermissionDenied, username)
	case irodscommon.CAT_NO_ROWS_FOUND:
		return fmt.Errorf("%w: user %q", ErrNotFound, username)
	case irodscommon.CATALOG_ALREADY_HAS_ITEM_BY_THAT_NAME:
		return fmt.Errorf("%w: user %q", ErrConflict, username)
	}

	if irodstypes.IsUserNotFoundError(err) {
		return fmt.Errorf("%w: user %q", ErrNotFound, username)
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") || strings.Contains(message, "no rows") {
		return fmt.Errorf("%w: user %q", ErrNotFound, username)
	}
	if strings.Contains(message, "already exists") ||
		strings.Contains(message, "already has item by that name") ||
		strings.Contains(message, "catalog_already_has_item_by_that_name") ||
		strings.Contains(message, "exists as") {
		return fmt.Errorf("%w: user %q", ErrConflict, username)
	}
	if strings.Contains(message, "no access permission") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "not authorized") {
		return fmt.Errorf("%w: user %q", ErrPermissionDenied, username)
	}

	if strings.TrimSpace(zone) != "" {
		return fmt.Errorf("%s user %q in zone %q: %w", operation, username, zone, err)
	}
	return fmt.Errorf("%s user %q: %w", operation, username, err)
}

// NormalizeGroupError maps common go-irodsclient and iRODS catalog errors into
// usersync sentinel errors with operation context.
func NormalizeGroupError(operation string, groupName string, zone string, err error) error {
	if err == nil {
		return nil
	}
	if IsKnownError(err) {
		return err
	}

	switch irodstypes.GetIRODSErrorCode(err) {
	case irodscommon.CAT_NO_ACCESS_PERMISSION, irodscommon.SYS_NO_API_PRIV:
		return fmt.Errorf("%w: group %q", ErrPermissionDenied, groupName)
	case irodscommon.CAT_NO_ROWS_FOUND, userNotInGroupErrorCode:
		return fmt.Errorf("%w: group %q", ErrNotFound, groupName)
	case irodscommon.CATALOG_ALREADY_HAS_ITEM_BY_THAT_NAME:
		return fmt.Errorf("%w: group %q", ErrConflict, groupName)
	}

	if irodstypes.IsUserNotFoundError(err) {
		return fmt.Errorf("%w: group %q", ErrNotFound, groupName)
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") ||
		strings.Contains(message, "no rows") ||
		strings.Contains(message, "not a member") ||
		strings.Contains(message, "not in group") ||
		strings.Contains(message, "unknown errorcode: -1830000") {
		return fmt.Errorf("%w: group %q", ErrNotFound, groupName)
	}
	if strings.Contains(message, "already exists") ||
		strings.Contains(message, "already has item by that name") ||
		strings.Contains(message, "catalog_already_has_item_by_that_name") ||
		strings.Contains(message, "exists as") {
		return fmt.Errorf("%w: group %q", ErrConflict, groupName)
	}
	if strings.Contains(message, "no access permission") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "not authorized") {
		return fmt.Errorf("%w: group %q", ErrPermissionDenied, groupName)
	}

	if strings.TrimSpace(zone) != "" {
		return fmt.Errorf("%s group %q in zone %q: %w", operation, groupName, zone, err)
	}
	return fmt.Errorf("%s group %q: %w", operation, groupName, err)
}

// IsKnownError reports whether err wraps a usersync sentinel error.
func IsKnownError(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrInvalidRequest) ||
		errors.Is(err, ErrPolicyViolation)
}

func mapUser(user *irodstypes.IRODSUser) User {
	if user == nil {
		return User{}
	}

	return User{
		ID:   user.ID,
		Name: strings.TrimSpace(user.Name),
		Zone: strings.TrimSpace(user.Zone),
		Type: strings.TrimSpace(string(user.Type)),
	}
}

func mapGroup(user *irodstypes.IRODSUser) Group {
	if user == nil {
		return Group{}
	}

	return Group{
		ID:   user.ID,
		Name: strings.TrimSpace(user.Name),
		Zone: strings.TrimSpace(user.Zone),
		Type: strings.TrimSpace(string(user.Type)),
	}
}

func mapGroupMembers(users []*irodstypes.IRODSUser) []GroupMember {
	if len(users) == 0 {
		return []GroupMember{}
	}

	members := make([]GroupMember, 0, len(users))
	for _, user := range users {
		if user == nil || !isPrincipalUserType(user.Type) {
			continue
		}

		mapped := mapUser(user)
		if mapped.Name == "" {
			continue
		}

		members = append(members, GroupMember{
			ID:   mapped.ID,
			Name: mapped.Name,
			Zone: mapped.Zone,
			Type: mapped.Type,
		})
	}

	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Name != members[j].Name {
			return members[i].Name < members[j].Name
		}
		if members[i].Zone != members[j].Zone {
			return members[i].Zone < members[j].Zone
		}
		return members[i].Type < members[j].Type
	})

	return members
}

func isPrincipalUserType(userType irodstypes.IRODSUserType) bool {
	switch strings.TrimSpace(string(userType)) {
	case string(irodstypes.IRODSUserRodsUser), string(irodstypes.IRODSUserGroupAdmin), string(irodstypes.IRODSUserRodsAdmin):
		return true
	default:
		return false
	}
}

func existingType(user *irodstypes.IRODSUser) irodstypes.IRODSUserType {
	if user == nil {
		return ""
	}
	return user.Type
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
