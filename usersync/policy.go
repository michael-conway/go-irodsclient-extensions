package usersync

import (
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

const (
	AVUAttributePrefix     = "iRODS:USER_SYNCH"
	AVUAttributeManaged    = "iRODS:USER_SYNCH:MANAGED"
	AVUAttributeSource     = "iRODS:USER_SYNCH:SOURCE"
	AVUAttributeRealm      = "iRODS:USER_SYNCH:REALM"
	AVUAttributeExternalID = "iRODS:USER_SYNCH:EXTERNAL_ID"
	AVUAttributeLastSyncAt = "iRODS:USER_SYNCH:LAST_SYNC_AT"
	AVUAttributeLastPlanID = "iRODS:USER_SYNCH:LAST_PLAN_ID"

	AVUValueTrue = "true"
)

// AVU describes an iRODS metadata marker used by user sync policy.
type AVU struct {
	Attribute string
	Value     string
	Unit      string
}

// SyncMetadata describes the durable ownership metadata that usersync writes
// to created or explicitly claimed iRODS principals.
type SyncMetadata struct {
	Source     string
	Realm      string
	ExternalID string
}

// Policy defines sync guardrails. It is intentionally independent of caller
// authentication; callers pass an already-authorized iRODS filesystem.
type Policy struct {
	AllowedCreateUserTypes []irodstypes.IRODSUserType
	AllowedManageUserTypes []irodstypes.IRODSUserType
	ProtectedUserTypes     []irodstypes.IRODSUserType

	ClaimExisting bool
	MarkCreated   bool

	AllowUnmanagedDeletes            bool
	AllowUnmanagedMembershipRemovals bool

	Metadata SyncMetadata
}

// Option configures a usersync service.
type Option func(*Service)

// DefaultPolicy returns the default sync policy. Sync creates and manages only
// normal rodsuser accounts. Admin-level user changes are intentionally outside
// the sync workflow.
func DefaultPolicy() Policy {
	return Policy{
		AllowedCreateUserTypes: []irodstypes.IRODSUserType{irodstypes.IRODSUserRodsUser},
		AllowedManageUserTypes: []irodstypes.IRODSUserType{irodstypes.IRODSUserRodsUser},
		ProtectedUserTypes: []irodstypes.IRODSUserType{
			irodstypes.IRODSUserGroupAdmin,
			irodstypes.IRODSUserRodsAdmin,
		},
		MarkCreated:                      true,
		AllowUnmanagedDeletes:            false,
		AllowUnmanagedMembershipRemovals: false,
	}
}

func WithPolicy(policy Policy) Option {
	return func(service *Service) {
		service.policy = policy.normalize()
	}
}

func WithClaimExisting(claim bool) Option {
	return func(service *Service) {
		service.policy.ClaimExisting = claim
	}
}

func WithSyncMetadata(metadata SyncMetadata) Option {
	return func(service *Service) {
		service.policy.Metadata = metadata.normalize()
	}
}

func WithAllowUnmanagedDeletes(allow bool) Option {
	return func(service *Service) {
		service.policy.AllowUnmanagedDeletes = allow
	}
}

func WithAllowUnmanagedMembershipRemovals(allow bool) Option {
	return func(service *Service) {
		service.policy.AllowUnmanagedMembershipRemovals = allow
	}
}

func (policy Policy) normalize() Policy {
	defaultPolicy := DefaultPolicy()

	if len(policy.AllowedCreateUserTypes) == 0 {
		policy.AllowedCreateUserTypes = append([]irodstypes.IRODSUserType(nil), defaultPolicy.AllowedCreateUserTypes...)
	}
	if len(policy.AllowedManageUserTypes) == 0 {
		policy.AllowedManageUserTypes = append([]irodstypes.IRODSUserType(nil), defaultPolicy.AllowedManageUserTypes...)
	}
	if len(policy.ProtectedUserTypes) == 0 {
		policy.ProtectedUserTypes = append([]irodstypes.IRODSUserType(nil), defaultPolicy.ProtectedUserTypes...)
	}

	policy.Metadata = policy.Metadata.normalize()
	return policy
}

func (metadata SyncMetadata) normalize() SyncMetadata {
	return SyncMetadata{
		Source:     strings.TrimSpace(metadata.Source),
		Realm:      strings.TrimSpace(metadata.Realm),
		ExternalID: strings.TrimSpace(metadata.ExternalID),
	}
}

func (policy Policy) managedAVUs() []AVU {
	metadata := policy.Metadata.normalize()
	avus := []AVU{{
		Attribute: AVUAttributeManaged,
		Value:     AVUValueTrue,
		Unit:      metadata.Source,
	}}

	if metadata.Source != "" {
		avus = append(avus, AVU{
			Attribute: AVUAttributeSource,
			Value:     metadata.Source,
		})
	}
	if metadata.Realm != "" {
		avus = append(avus, AVU{
			Attribute: AVUAttributeRealm,
			Value:     metadata.Realm,
		})
	}
	if metadata.ExternalID != "" {
		avus = append(avus, AVU{
			Attribute: AVUAttributeExternalID,
			Value:     metadata.ExternalID,
		})
	}

	return avus
}

func userTypeIn(userType irodstypes.IRODSUserType, allowed []irodstypes.IRODSUserType) bool {
	for _, candidate := range allowed {
		if userType == candidate {
			return true
		}
	}
	return false
}
