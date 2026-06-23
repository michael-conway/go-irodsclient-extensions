package usersandgroups

import (
	"context"
	"errors"
	"reflect"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestServiceNormalizesGroupSummaryOptions(t *testing.T) {
	catalog := &recordingCatalog{}
	service := NewService(catalog, "tempZone")

	if _, err := service.ListGroupSummaries(context.Background(), GroupSummaryOptions{Prefix: " research ", Limit: 2000}); err != nil {
		t.Fatalf("ListGroupSummaries returned error: %v", err)
	}

	want := GroupSummaryOptions{Zone: "tempZone", Prefix: "research", Limit: MaxLimit}
	if !reflect.DeepEqual(catalog.groupOptions, want) {
		t.Fatalf("expected options %+v, got %+v", want, catalog.groupOptions)
	}
}

func TestServiceRequiresUserNameForReverseMembership(t *testing.T) {
	service := NewService(&recordingCatalog{}, "tempZone")

	_, err := service.ListGroupsForUser(context.Background(), GroupsForUserOptions{UserName: " "})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceSortsGroupRefs(t *testing.T) {
	catalog := &recordingCatalog{
		groupsForUser: []GroupRef{
			{Name: "zeta", Zone: "tempZone"},
			{Name: "alpha", Zone: "tempZone"},
		},
		userSummaries: []UserMembershipSummary{
			{
				Name: "alice",
				Groups: []GroupRef{
					{Name: "beta", Zone: "tempZone"},
					{Name: "alpha", Zone: "tempZone"},
				},
			},
		},
	}
	service := NewService(catalog, "tempZone")

	groups, err := service.ListGroupsForUser(context.Background(), GroupsForUserOptions{UserName: "alice"})
	if err != nil {
		t.Fatalf("ListGroupsForUser returned error: %v", err)
	}
	if groups[0].Name != "alpha" || groups[1].Name != "zeta" {
		t.Fatalf("expected sorted reverse groups, got %+v", groups)
	}

	users, err := service.ListUserMembershipSummaries(context.Background(), UserMembershipSummaryOptions{})
	if err != nil {
		t.Fatalf("ListUserMembershipSummaries returned error: %v", err)
	}
	if users[0].Groups[0].Name != "alpha" || users[0].Groups[1].Name != "beta" {
		t.Fatalf("expected sorted user groups, got %+v", users[0].Groups)
	}
}

func TestServiceNormalizesPrincipalSearch(t *testing.T) {
	catalog := &recordingCatalog{}
	service := NewService(catalog, "tempZone")

	if _, err := service.SearchPrincipals(context.Background(), PrincipalSearchOptions{
		Query: " ali ",
		Kinds: []PrincipalKind{"bad", PrincipalKindGroup, PrincipalKindGroup},
	}); err != nil {
		t.Fatalf("SearchPrincipals returned error: %v", err)
	}

	want := PrincipalSearchOptions{
		Zone:  "tempZone",
		Query: "ali",
		Limit: DefaultLimit,
		Kinds: []PrincipalKind{PrincipalKindGroup},
	}
	if !reflect.DeepEqual(catalog.searchOptions, want) {
		t.Fatalf("expected options %+v, got %+v", want, catalog.searchOptions)
	}
}

type recordingCatalog struct {
	groupOptions       GroupSummaryOptions
	userOptions        UserMembershipSummaryOptions
	groupsForUserOpts  GroupsForUserOptions
	searchOptions      PrincipalSearchOptions
	groupsForUser      []GroupRef
	userSummaries      []UserMembershipSummary
	principalSearchOut []PrincipalSearchResult
}

func (catalog *recordingCatalog) ListGroupSummaries(_ context.Context, options GroupSummaryOptions) ([]GroupSummary, error) {
	catalog.groupOptions = options
	return []GroupSummary{{Name: "research", Type: irodstypes.IRODSUserRodsGroup}}, nil
}

func (catalog *recordingCatalog) ListUserMembershipSummaries(_ context.Context, options UserMembershipSummaryOptions) ([]UserMembershipSummary, error) {
	catalog.userOptions = options
	return catalog.userSummaries, nil
}

func (catalog *recordingCatalog) ListGroupsForUser(_ context.Context, options GroupsForUserOptions) ([]GroupRef, error) {
	catalog.groupsForUserOpts = options
	return catalog.groupsForUser, nil
}

func (catalog *recordingCatalog) SearchPrincipals(_ context.Context, options PrincipalSearchOptions) ([]PrincipalSearchResult, error) {
	catalog.searchOptions = options
	return catalog.principalSearchOut, nil
}
