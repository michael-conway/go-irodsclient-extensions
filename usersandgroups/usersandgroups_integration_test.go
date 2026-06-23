//go:build integration
// +build integration

package usersandgroups_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/usersandgroups"
	usersandgroupsirodsfs "github.com/michael-conway/go-irodsclient-extensions/usersandgroups/irodsfs"
	usersyncirodsfs "github.com/michael-conway/go-irodsclient-extensions/usersync/irodsfs"
	"github.com/rs/xid"
)

func TestUsersAndGroupsLiveIntegration(t *testing.T) {
	cfg := testutil.RequireIntegrationConfig(t)
	primaryUser := testutil.IntegrationPrimaryTestUser(t)
	secondaryUser := testutil.IntegrationSecondaryTestUser(t)

	adminFS := testutil.NewIntegrationAdminFilesystem(t)
	defer adminFS.Release()

	syncAdapter := usersyncirodsfs.NewAdapter(adminFS)
	testID := xid.New().String()
	groupPrefix := fmt.Sprintf("it-uag-%s", testID)
	groupA := groupPrefix + "-alpha"
	groupB := groupPrefix + "-bravo"

	for _, groupName := range []string{groupA, groupB} {
		if _, err := syncAdapter.CreateUserGroup(groupName, cfg.IrodsZone); err != nil {
			t.Fatalf("create group %q: %v", groupName, err)
		}
		groupName := groupName
		t.Cleanup(func() {
			_ = syncAdapter.RemoveGroupMember(groupName, primaryUser, cfg.IrodsZone)
			_ = syncAdapter.RemoveGroupMember(groupName, secondaryUser, cfg.IrodsZone)
			_ = syncAdapter.RemoveUserGroup(groupName, cfg.IrodsZone)
		})
	}

	addMemberLive(t, syncAdapter, groupA, primaryUser, cfg.IrodsZone)
	addMemberLive(t, syncAdapter, groupA, secondaryUser, cfg.IrodsZone)
	addMemberLive(t, syncAdapter, groupB, primaryUser, cfg.IrodsZone)

	ctx := context.Background()
	serviceFactory := func(t testing.TB) (*usersandgroups.Service, func()) {
		t.Helper()
		fs := testutil.NewIntegrationAdminFilesystem(t)
		return usersandgroups.NewService(usersandgroupsirodsfs.NewAdapter(fs), cfg.IrodsZone), fs.Release
	}

	summaries := eventuallyUsersAndGroups(t, func() ([]usersandgroups.GroupSummary, error) {
		service, release := serviceFactory(t)
		defer release()
		return service.ListGroupSummaries(ctx, usersandgroups.GroupSummaryOptions{
			Prefix: groupPrefix,
			Limit:  10,
		})
	}, func(summaries []usersandgroups.GroupSummary) bool {
		return groupSummaryCount(summaries, groupA) == 2 && groupSummaryCount(summaries, groupB) == 1
	})
	if groupSummaryCount(summaries, groupA) != 2 || groupSummaryCount(summaries, groupB) != 1 {
		t.Fatalf("expected group counts %s=2 and %s=1, got %+v", groupA, groupB, summaries)
	}

	userSummaries := eventuallyUsersAndGroups(t, func() ([]usersandgroups.UserMembershipSummary, error) {
		service, release := serviceFactory(t)
		defer release()
		return service.ListUserMembershipSummaries(ctx, usersandgroups.UserMembershipSummaryOptions{
			Prefix: primaryUser,
			Limit:  10,
		})
	}, func(users []usersandgroups.UserMembershipSummary) bool {
		return userHasGroups(users, primaryUser, groupA, groupB)
	})
	if !userHasGroups(userSummaries, primaryUser, groupA, groupB) {
		t.Fatalf("expected primary user %q to include groups %q and %q, got %+v", primaryUser, groupA, groupB, userSummaries)
	}

	groupsForUser := eventuallyUsersAndGroups(t, func() ([]usersandgroups.GroupRef, error) {
		service, release := serviceFactory(t)
		defer release()
		return service.ListGroupsForUser(ctx, usersandgroups.GroupsForUserOptions{
			UserName: primaryUser,
			Limit:    100,
		})
	}, func(groups []usersandgroups.GroupRef) bool {
		return groupRefsContain(groups, groupA, groupB)
	})
	if !groupRefsContain(groupsForUser, groupA, groupB) {
		t.Fatalf("expected reverse memberships for %q to include %q and %q, got %+v", primaryUser, groupA, groupB, groupsForUser)
	}

	principals := eventuallyUsersAndGroups(t, func() ([]usersandgroups.PrincipalSearchResult, error) {
		service, release := serviceFactory(t)
		defer release()
		return service.SearchPrincipals(ctx, usersandgroups.PrincipalSearchOptions{
			Query: groupPrefix,
			Kinds: []usersandgroups.PrincipalKind{
				usersandgroups.PrincipalKindGroup,
			},
			Limit: 10,
		})
	}, func(results []usersandgroups.PrincipalSearchResult) bool {
		return principalResultsContainGroups(results, groupA, groupB)
	})
	if !principalResultsContainGroups(principals, groupA, groupB) {
		t.Fatalf("expected principal search to include groups %q and %q, got %+v", groupA, groupB, principals)
	}
}

func addMemberLive(t testing.TB, adapter *usersyncirodsfs.Adapter, groupName string, username string, zone string) {
	t.Helper()
	if err := adapter.AddGroupMember(groupName, username, zone); err != nil {
		t.Fatalf("add user %q to group %q: %v", username, groupName, err)
	}
}

func eventuallyUsersAndGroups[T any](t testing.TB, fetch func() (T, error), ok func(T) bool) T {
	t.Helper()

	var last T
	for attempt := 0; attempt < 5; attempt++ {
		got, err := fetch()
		if err != nil {
			t.Fatalf("fetch usersandgroups result: %v", err)
		}
		last = got
		if ok(got) {
			return got
		}
		time.Sleep(1 * time.Second)
	}
	return last
}

func groupSummaryCount(summaries []usersandgroups.GroupSummary, groupName string) int {
	for _, summary := range summaries {
		if summary.Name == groupName {
			return summary.MemberCount
		}
	}
	return -1
}

func userHasGroups(users []usersandgroups.UserMembershipSummary, username string, groupNames ...string) bool {
	for _, user := range users {
		if user.Name != username {
			continue
		}
		actual := make([]string, 0, len(user.Groups))
		for _, group := range user.Groups {
			actual = append(actual, group.Name)
		}
		return containsAllNames(actual, groupNames...)
	}
	return false
}

func groupRefsContain(groups []usersandgroups.GroupRef, groupNames ...string) bool {
	actual := make([]string, 0, len(groups))
	for _, group := range groups {
		actual = append(actual, group.Name)
	}
	return containsAllNames(actual, groupNames...)
}

func principalResultsContainGroups(results []usersandgroups.PrincipalSearchResult, groupNames ...string) bool {
	actual := make([]string, 0, len(results))
	for _, result := range results {
		if result.Kind == usersandgroups.PrincipalKindGroup {
			actual = append(actual, result.Name)
		}
	}
	return containsAllNames(actual, groupNames...)
}

func containsAllNames(actual []string, expected ...string) bool {
	seen := map[string]bool{}
	for _, name := range actual {
		seen[strings.TrimSpace(name)] = true
	}
	for _, name := range expected {
		if !seen[strings.TrimSpace(name)] {
			return false
		}
	}
	return true
}
