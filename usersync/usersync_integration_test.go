//go:build integration
// +build integration

package usersync_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/usersync"
	"github.com/michael-conway/go-irodsclient-extensions/usersync/irodsfs"
	"github.com/rs/xid"
)

func TestUserSyncLiveIntegration(t *testing.T) {
	fs := testutil.NewIntegrationAdminFilesystem(t)
	defer fs.Release()

	cfg := testutil.RequireIntegrationConfig(t)
	adapter := irodsfs.NewAdapter(fs)
	service := usersync.NewService(adapter, cfg.IrodsZone)

	ctx := context.Background()
	testID := xid.New().String()
	testUserName := fmt.Sprintf("it-user-%s", testID)
	testGroupName := fmt.Sprintf("it-group-%s", testID)

	// 1. Ensure User
	t.Logf("ensuring user %q", testUserName)
	ensureResult, err := service.EnsureUser(ctx, usersync.EnsureUserRequest{
		Name:     testUserName,
		Type:     irodstypes.IRODSUserRodsUser,
		Password: "test-password",
	})
	if err != nil {
		t.Fatalf("EnsureUser failed: %v", err)
	}
	if ensureResult.Outcome != usersync.OutcomeCreated {
		t.Fatalf("expected created outcome, got %q", ensureResult.Outcome)
	}

	t.Cleanup(func() {
		// Clean up user with relaxed policy if needed, or direct admin call
		_ = fs.RemoveUser(testUserName, cfg.IrodsZone, irodstypes.IRODSUserRodsUser)
	})

	// Verify metadata
	metas, err := fs.ListUserMetadata(testUserName, cfg.IrodsZone)
	if err != nil {
		t.Fatalf("list metadata for %q: %v", testUserName, err)
	}
	if !hasAVU(metas, usersync.AVU{Attribute: usersync.AVUAttributeManaged, Value: usersync.AVUValueTrue, Unit: ""}) {
		t.Errorf("expected user %q to be marked as managed", testUserName)
	}

	// 2. Ensure Group
	t.Logf("ensuring group %q", testGroupName)
	groupResult, err := service.EnsureGroup(ctx, usersync.GroupRef{
		Name: testGroupName,
	})
	if err != nil {
		t.Fatalf("EnsureGroup failed: %v", err)
	}
	if groupResult.Outcome != usersync.OutcomeCreated {
		t.Fatalf("expected created outcome for group, got %q", groupResult.Outcome)
	}

	t.Cleanup(func() {
		// Clean up group
		_ = adapter.RemoveUserGroup(testGroupName, cfg.IrodsZone)
	})

	// 3. Add Member
	t.Logf("adding user %q to group %q", testUserName, testGroupName)
	memberResult, err := service.EnsureGroupMember(ctx, usersync.GroupMemberRef{
		GroupName: testGroupName,
		UserName:  testUserName,
	})
	if err != nil {
		t.Fatalf("EnsureGroupMember failed: %v", err)
	}
	if memberResult.Outcome != usersync.OutcomeAdded {
		t.Fatalf("expected added outcome, got %q", memberResult.Outcome)
	}

	// Verify membership with retry (using fresh connections to bypass cache)
	t.Logf("verifying user %q is a member of %q", testUserName, testGroupName)
	var members []*irodstypes.IRODSUser
	found := false
	for i := 0; i < 5; i++ {
		verifyFs := testutil.NewIntegrationAdminFilesystem(t)
		members, err = verifyFs.ListGroupMembers(cfg.IrodsZone, testGroupName)
		verifyFs.Release()
		if err != nil {
			t.Fatalf("list group members for %q: %v", testGroupName, err)
		}
		for _, m := range members {
			if m.Name == testUserName {
				found = true
				break
			}
		}
		if found {
			break
		}
		t.Logf("user not found in group yet, retrying... (%d/5)", i+1)
		time.Sleep(1 * time.Second)
	}

	if !found {
		memberNames := make([]string, 0, len(members))
		for _, m := range members {
			memberNames = append(memberNames, m.Name)
		}
		t.Errorf("expected user %q to be a member of %q, but found members: %+v", testUserName, testGroupName, memberNames)
	}

	// 4. Test Reconcile Already Exists
	t.Logf("ensuring user %q again (idempotency)", testUserName)
	reconcileResult, err := service.EnsureUser(ctx, usersync.EnsureUserRequest{
		Name: testUserName,
		Type: irodstypes.IRODSUserRodsUser,
	})
	if err != nil {
		t.Fatalf("EnsureUser (reconcile) failed: %v", err)
	}
	if reconcileResult.Outcome != usersync.OutcomeAlreadyExists {
		t.Errorf("expected already_exists outcome, got %q", reconcileResult.Outcome)
	}

	// 5. Remove Member
	t.Logf("removing user %q from group %q", testUserName, testGroupName)
	removeMemberResult, err := service.EnsureGroupMemberAbsent(ctx, usersync.GroupMemberRef{
		GroupName: testGroupName,
		UserName:  testUserName,
	})
	if err != nil {
		t.Fatalf("EnsureGroupMemberAbsent failed: %v", err)
	}
	if removeMemberResult.Outcome != usersync.OutcomeRemoved {
		t.Errorf("expected removed outcome, got %q", removeMemberResult.Outcome)
	}

	// 6. Delete User
	t.Logf("deleting user %q", testUserName)
	deleteResult, err := service.EnsureUserAbsent(ctx, usersync.UserRef{
		Name: testUserName,
	})
	if err != nil {
		t.Fatalf("EnsureUserAbsent failed: %v", err)
	}
	if deleteResult.Outcome != usersync.OutcomeDeleted {
		t.Errorf("expected deleted outcome, got %q", deleteResult.Outcome)
	}
}

func TestUserSyncPolicyEnforcementLive(t *testing.T) {
	fs := testutil.NewIntegrationAdminFilesystem(t)
	defer fs.Release()

	cfg := testutil.RequireIntegrationConfig(t)
	adapter := irodsfs.NewAdapter(fs)
	service := usersync.NewService(adapter, cfg.IrodsZone)

	ctx := context.Background()
	testID := xid.New().String()
	testUserName := fmt.Sprintf("it-policy-%s", testID)

	// Create user directly via admin (unmanaged)
	t.Logf("creating unmanaged user %q", testUserName)
	_, err := fs.CreateUser(testUserName, cfg.IrodsZone, irodstypes.IRODSUserRodsUser)
	if err != nil {
		t.Fatalf("failed to create unmanaged user: %v", err)
	}
	t.Cleanup(func() {
		_ = fs.RemoveUser(testUserName, cfg.IrodsZone, irodstypes.IRODSUserRodsUser)
	})

	// Attempt to delete unmanaged user (should fail by default policy)
	t.Logf("attempting to delete unmanaged user (should fail)")
	_, err = service.EnsureUserAbsent(ctx, usersync.UserRef{
		Name: testUserName,
	})
	if err == nil {
		t.Fatal("expected policy violation error for unmanaged user delete, got nil")
	}
}

func hasAVU(metadata []*irodstypes.IRODSMeta, avu usersync.AVU) bool {
	for _, meta := range metadata {
		if meta == nil {
			continue
		}
		if meta.Name == avu.Attribute && meta.Value == avu.Value && meta.Units == avu.Unit {
			return true
		}
	}
	return false
}
