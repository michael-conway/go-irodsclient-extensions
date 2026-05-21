package usersync

import (
	"context"
	"errors"
	"testing"

	irodscommon "github.com/cyverse/go-irodsclient/irods/common"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestEnsureUserReconcilesExistingMatchingType(t *testing.T) {
	filesystem := newFakeFilesystem()
	service := NewService(filesystem, "tempZone")

	result, err := service.EnsureUser(context.Background(), EnsureUserRequest{
		Name: "alice",
		Type: irodstypes.IRODSUserRodsUser,
	})
	if err != nil {
		t.Fatalf("EnsureUser returned error: %v", err)
	}
	if result.Outcome != OutcomeAlreadyExists {
		t.Fatalf("expected already_exists, got %q", result.Outcome)
	}
	if result.User.Name != "alice" || result.User.Type != string(irodstypes.IRODSUserRodsUser) {
		t.Fatalf("unexpected user: %+v", result.User)
	}
}

func TestEnsureUserRejectsTypeMismatch(t *testing.T) {
	filesystem := newFakeFilesystem()
	service := NewService(filesystem, "tempZone")

	_, err := service.EnsureUser(context.Background(), EnsureUserRequest{
		Name: "alice",
		Type: irodstypes.IRODSUserRodsAdmin,
	})
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected policy violation, got %v", err)
	}
}

func TestUpdateUserCreatesMissingWhenAllowed(t *testing.T) {
	filesystem := newFakeFilesystem()
	service := NewService(filesystem, "tempZone")

	result, err := service.UpdateUser(context.Background(), UpdateUserRequest{
		Name:            "charlie",
		Type:            irodstypes.IRODSUserRodsUser,
		ChangeType:      true,
		ChangePassword:  true,
		Password:        "secret",
		CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
	if result.Outcome != OutcomeCreated {
		t.Fatalf("expected created, got %q", result.Outcome)
	}
	if !filesystem.passwordChanged[catalogKey("charlie", "tempZone")] {
		t.Fatalf("expected password to be applied")
	}
	if !filesystem.hasUserMetadata("charlie", "tempZone", AVUAttributeManaged, AVUValueTrue, "") {
		t.Fatalf("expected created user to be marked managed")
	}
}

func TestUpdateUserPasswordOnlyDoesNotCreateMissing(t *testing.T) {
	filesystem := newFakeFilesystem()
	service := NewService(filesystem, "tempZone")

	_, err := service.UpdateUser(context.Background(), UpdateUserRequest{
		Name:            "charlie",
		ChangePassword:  true,
		Password:        "secret",
		CreateIfMissing: true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestEnsureGroupMemberAbsentReconcilesMissingMembership(t *testing.T) {
	filesystem := newFakeFilesystem()
	filesystem.addUserMetadata("research-team", "tempZone", AVUAttributeManaged, AVUValueTrue, "")
	service := NewService(filesystem, "tempZone")

	result, err := service.EnsureGroupMemberAbsent(context.Background(), GroupMemberRef{
		GroupName: "research-team",
		UserName:  "bob",
	})
	if err != nil {
		t.Fatalf("EnsureGroupMemberAbsent returned error: %v", err)
	}
	if result.Outcome != OutcomeAlreadyAbsent {
		t.Fatalf("expected already_absent, got %q", result.Outcome)
	}
	if len(result.Group.Members) != 1 || result.Group.Members[0].Name != "alice" {
		t.Fatalf("unexpected group members: %+v", result.Group.Members)
	}
}

func TestEnsureGroupMemberAbsentRejectsUnmanagedGroup(t *testing.T) {
	filesystem := newFakeFilesystem()
	service := NewService(filesystem, "tempZone")

	_, err := service.EnsureGroupMemberAbsent(context.Background(), GroupMemberRef{
		GroupName: "research-team",
		UserName:  "bob",
	})
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected policy violation, got %v", err)
	}
}

func TestEnsureUserRejectsProtectedAdminType(t *testing.T) {
	filesystem := newFakeFilesystem()
	filesystem.usersByKey[catalogKey("admin", "tempZone")] = &irodstypes.IRODSUser{
		ID:   4,
		Name: "admin",
		Zone: "tempZone",
		Type: irodstypes.IRODSUserRodsAdmin,
	}
	service := NewService(filesystem, "tempZone")

	_, err := service.EnsureGroupMember(context.Background(), GroupMemberRef{
		GroupName: "research-team",
		UserName:  "admin",
	})
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected policy violation, got %v", err)
	}
}

func TestNormalizeGroupErrorMapsLiveUserNotInGroupCode(t *testing.T) {
	err := NormalizeGroupError("remove group member", "research-team", "tempZone", irodstypes.NewIRODSError(userNotInGroupErrorCode))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for live user-not-in-group code, got %v", err)
	}

	err = NormalizeGroupError("remove group member", "research-team", "tempZone", errors.New("received remove group member error: Unknown ErrorCode: -1830000"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for live user-not-in-group message, got %v", err)
	}
}

func TestNormalizeUserErrorMapsLiveAlreadyExistsCode(t *testing.T) {
	err := NormalizeUserError("create user", "alice", "tempZone", irodstypes.NewIRODSError(irodscommon.CATALOG_ALREADY_HAS_ITEM_BY_THAT_NAME))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict for already-exists code, got %v", err)
	}

	err = NormalizeUserError("create user", "alice", "tempZone", errors.New("received create user error: CATALOG_ALREADY_HAS_ITEM_BY_THAT_NAME"))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict for already-exists message, got %v", err)
	}
}

type fakeFilesystem struct {
	usersByKey      map[string]*irodstypes.IRODSUser
	membersByGroup  map[string][]string
	metadataByUser  map[string][]*irodstypes.IRODSMeta
	passwordChanged map[string]bool
}

func newFakeFilesystem() *fakeFilesystem {
	return &fakeFilesystem{
		usersByKey: map[string]*irodstypes.IRODSUser{
			catalogKey("alice", "tempZone"): &irodstypes.IRODSUser{
				ID:   1,
				Name: "alice",
				Zone: "tempZone",
				Type: irodstypes.IRODSUserRodsUser,
			},
			catalogKey("bob", "tempZone"): &irodstypes.IRODSUser{
				ID:   2,
				Name: "bob",
				Zone: "tempZone",
				Type: irodstypes.IRODSUserRodsUser,
			},
			catalogKey("research-team", "tempZone"): &irodstypes.IRODSUser{
				ID:   3,
				Name: "research-team",
				Zone: "tempZone",
				Type: irodstypes.IRODSUserRodsGroup,
			},
		},
		membersByGroup: map[string][]string{
			catalogKey("research-team", "tempZone"): []string{"alice"},
		},
		metadataByUser:  map[string][]*irodstypes.IRODSMeta{},
		passwordChanged: map[string]bool{},
	}
}

func (filesystem *fakeFilesystem) GetUser(username string, zoneName string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	user, ok := filesystem.usersByKey[catalogKey(username, zoneName)]
	if !ok {
		return nil, irodstypes.NewUserNotFoundError(username)
	}
	if userType != "" && user.Type != userType {
		return nil, irodstypes.NewUserNotFoundError(username)
	}
	return user, nil
}

func (filesystem *fakeFilesystem) ListGroupMembers(zoneName string, groupName string) ([]*irodstypes.IRODSUser, error) {
	members := filesystem.membersByGroup[catalogKey(groupName, zoneName)]
	result := make([]*irodstypes.IRODSUser, 0, len(members))
	for _, memberName := range members {
		user, ok := filesystem.usersByKey[catalogKey(memberName, zoneName)]
		if ok {
			result = append(result, user)
		}
	}
	return result, nil
}

func (filesystem *fakeFilesystem) ListUserMetadata(username string, zoneName string) ([]*irodstypes.IRODSMeta, error) {
	if _, ok := filesystem.usersByKey[catalogKey(username, zoneName)]; !ok {
		return nil, irodstypes.NewUserNotFoundError(username)
	}

	metadata := filesystem.metadataByUser[catalogKey(username, zoneName)]
	result := make([]*irodstypes.IRODSMeta, 0, len(metadata))
	for _, meta := range metadata {
		if meta == nil {
			continue
		}
		copy := *meta
		result = append(result, &copy)
	}
	return result, nil
}

func (filesystem *fakeFilesystem) AddUserMetadata(username string, zoneName string, attribute string, value string, unit string) error {
	if _, ok := filesystem.usersByKey[catalogKey(username, zoneName)]; !ok {
		return irodstypes.NewUserNotFoundError(username)
	}
	if filesystem.hasUserMetadata(username, zoneName, attribute, value, unit) {
		return errors.New("already exists")
	}

	filesystem.addUserMetadata(username, zoneName, attribute, value, unit)
	return nil
}

func (filesystem *fakeFilesystem) CreateUser(username string, zoneName string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	key := catalogKey(username, zoneName)
	if user, ok := filesystem.usersByKey[key]; ok {
		return user, errors.New("already exists")
	}

	user := &irodstypes.IRODSUser{
		ID:   int64(len(filesystem.usersByKey) + 1),
		Name: username,
		Zone: zoneName,
		Type: userType,
	}
	filesystem.usersByKey[key] = user
	return user, nil
}

func (filesystem *fakeFilesystem) CreateUserGroup(groupName string, zoneName string) (*irodstypes.IRODSUser, error) {
	return filesystem.CreateUser(groupName, zoneName, irodstypes.IRODSUserRodsGroup)
}

func (filesystem *fakeFilesystem) ChangeUserPassword(username string, zoneName string, _ string) error {
	key := catalogKey(username, zoneName)
	if _, ok := filesystem.usersByKey[key]; !ok {
		return irodstypes.NewUserNotFoundError(username)
	}
	filesystem.passwordChanged[key] = true
	return nil
}

func (filesystem *fakeFilesystem) ChangeUserType(username string, zoneName string, newType irodstypes.IRODSUserType) error {
	user, ok := filesystem.usersByKey[catalogKey(username, zoneName)]
	if !ok {
		return irodstypes.NewUserNotFoundError(username)
	}
	user.Type = newType
	return nil
}

func (filesystem *fakeFilesystem) RemoveUser(username string, zoneName string, _ irodstypes.IRODSUserType) error {
	key := catalogKey(username, zoneName)
	if _, ok := filesystem.usersByKey[key]; !ok {
		return irodstypes.NewUserNotFoundError(username)
	}
	delete(filesystem.usersByKey, key)
	delete(filesystem.membersByGroup, key)
	delete(filesystem.metadataByUser, key)
	for groupKey, members := range filesystem.membersByGroup {
		filtered := members[:0]
		for _, member := range members {
			if member != username {
				filtered = append(filtered, member)
			}
		}
		filesystem.membersByGroup[groupKey] = filtered
	}
	return nil
}

func (filesystem *fakeFilesystem) RemoveUserGroup(groupName string, zoneName string) error {
	return filesystem.RemoveUser(groupName, zoneName, irodstypes.IRODSUserRodsGroup)
}

func (filesystem *fakeFilesystem) AddGroupMember(groupName string, username string, zoneName string) error {
	if _, ok := filesystem.usersByKey[catalogKey(groupName, zoneName)]; !ok {
		return irodstypes.NewUserNotFoundError(groupName)
	}
	if _, ok := filesystem.usersByKey[catalogKey(username, zoneName)]; !ok {
		return irodstypes.NewUserNotFoundError(username)
	}

	key := catalogKey(groupName, zoneName)
	for _, member := range filesystem.membersByGroup[key] {
		if member == username {
			return errors.New("already exists")
		}
	}
	filesystem.membersByGroup[key] = append(filesystem.membersByGroup[key], username)
	return nil
}

func (filesystem *fakeFilesystem) RemoveGroupMember(groupName string, username string, zoneName string) error {
	key := catalogKey(groupName, zoneName)
	members, ok := filesystem.membersByGroup[key]
	if !ok {
		return irodstypes.NewUserNotFoundError(groupName)
	}

	filtered := members[:0]
	removed := false
	for _, member := range members {
		if member == username {
			removed = true
			continue
		}
		filtered = append(filtered, member)
	}
	if !removed {
		return irodstypes.NewIRODSError(userNotInGroupErrorCode)
	}
	filesystem.membersByGroup[key] = filtered
	return nil
}

func (filesystem *fakeFilesystem) addUserMetadata(username string, zoneName string, attribute string, value string, unit string) {
	key := catalogKey(username, zoneName)
	filesystem.metadataByUser[key] = append(filesystem.metadataByUser[key], &irodstypes.IRODSMeta{
		AVUID: int64(len(filesystem.metadataByUser[key]) + 1),
		Name:  attribute,
		Value: value,
		Units: unit,
	})
}

func (filesystem *fakeFilesystem) hasUserMetadata(username string, zoneName string, attribute string, value string, unit string) bool {
	for _, meta := range filesystem.metadataByUser[catalogKey(username, zoneName)] {
		if meta == nil {
			continue
		}
		if meta.Name == attribute && meta.Value == value && meta.Units == unit {
			return true
		}
	}
	return false
}

func catalogKey(name string, zone string) string {
	return name + "#" + zone
}
