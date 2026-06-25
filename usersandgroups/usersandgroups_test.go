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
	if catalog.userOptions.Type != "" {
		t.Fatalf("expected empty user type to mean all non-group users, got %q", catalog.userOptions.Type)
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

func TestServiceCreateRodsUserWithPasswordNormalizesRequest(t *testing.T) {
	catalog := &recordingCatalog{}
	service := NewService(catalog, "tempZone")

	user, err := service.CreateRodsUserWithPassword(context.Background(), CreateRodsUserWithPasswordRequest{
		Name:     " charlie ",
		Password: " initial-pass ",
	})
	if err != nil {
		t.Fatalf("CreateRodsUserWithPassword returned error: %v", err)
	}

	want := CreateRodsUserWithPasswordRequest{
		Zone:     "tempZone",
		Name:     "charlie",
		Password: " initial-pass ",
	}
	if !reflect.DeepEqual(catalog.createUserRequest, want) {
		t.Fatalf("expected request %+v, got %+v", want, catalog.createUserRequest)
	}
	if user.Name != "charlie" || user.Zone != "tempZone" || user.Type != irodstypes.IRODSUserRodsUser {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestServiceCreateRodsUserWithPasswordRequiresPassword(t *testing.T) {
	service := NewService(&recordingCatalog{}, "tempZone")

	_, err := service.CreateRodsUserWithPassword(context.Background(), CreateRodsUserWithPasswordRequest{
		Name:     "charlie",
		Password: " ",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceCreateRodsUserWithPasswordRequiresMutator(t *testing.T) {
	service := NewService(readOnlyCatalog{}, "tempZone")

	_, err := service.CreateRodsUserWithPassword(context.Background(), CreateRodsUserWithPasswordRequest{
		Name:     "charlie",
		Password: "initial-pass",
	})
	if !errors.Is(err, ErrMissingCatalog) {
		t.Fatalf("expected ErrMissingCatalog, got %v", err)
	}
}

func TestServiceCreateGroupNormalizesRequest(t *testing.T) {
	catalog := &recordingCatalog{}
	service := NewService(catalog, "tempZone")

	group, err := service.CreateGroup(context.Background(), GroupRequest{Name: " research "})
	if err != nil {
		t.Fatalf("CreateGroup returned error: %v", err)
	}

	want := GroupRequest{Zone: "tempZone", Name: "research"}
	if !reflect.DeepEqual(catalog.createGroupRequest, want) {
		t.Fatalf("expected request %+v, got %+v", want, catalog.createGroupRequest)
	}
	if group.Name != "research" || group.Zone != "tempZone" || group.Type != irodstypes.IRODSUserRodsGroup {
		t.Fatalf("unexpected group: %+v", group)
	}
}

func TestServiceGroupMemberMutationsNormalizeRequest(t *testing.T) {
	catalog := &recordingCatalog{}
	service := NewService(catalog, "tempZone")

	if err := service.AddGroupMember(context.Background(), GroupMemberRequest{
		GroupName: " research ",
		UserName:  " alice ",
	}); err != nil {
		t.Fatalf("AddGroupMember returned error: %v", err)
	}
	if err := service.RemoveGroupMember(context.Background(), GroupMemberRequest{
		GroupName: " research ",
		UserName:  " alice ",
	}); err != nil {
		t.Fatalf("RemoveGroupMember returned error: %v", err)
	}

	want := GroupMemberRequest{Zone: "tempZone", GroupName: "research", UserName: "alice"}
	if !reflect.DeepEqual(catalog.addMemberRequest, want) {
		t.Fatalf("expected add request %+v, got %+v", want, catalog.addMemberRequest)
	}
	if !reflect.DeepEqual(catalog.removeMemberRequest, want) {
		t.Fatalf("expected remove request %+v, got %+v", want, catalog.removeMemberRequest)
	}
}

func TestServiceGroupMemberMutationsRequireNames(t *testing.T) {
	service := NewService(&recordingCatalog{}, "tempZone")

	if err := service.AddGroupMember(context.Background(), GroupMemberRequest{GroupName: " ", UserName: "alice"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for missing group name, got %v", err)
	}
	if err := service.RemoveGroupMember(context.Background(), GroupMemberRequest{GroupName: "research", UserName: " "}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for missing user name, got %v", err)
	}
}

type recordingCatalog struct {
	groupOptions        GroupSummaryOptions
	userOptions         UserMembershipSummaryOptions
	groupsForUserOpts   GroupsForUserOptions
	searchOptions       PrincipalSearchOptions
	createUserRequest   CreateRodsUserWithPasswordRequest
	createGroupRequest  GroupRequest
	addMemberRequest    GroupMemberRequest
	removeMemberRequest GroupMemberRequest
	groupsForUser       []GroupRef
	userSummaries       []UserMembershipSummary
	principalSearchOut  []PrincipalSearchResult
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

func (catalog *recordingCatalog) CreateRodsUserWithPassword(_ context.Context, request CreateRodsUserWithPasswordRequest) (User, error) {
	catalog.createUserRequest = request
	return User{
		Name: request.Name,
		Zone: request.Zone,
		Type: irodstypes.IRODSUserRodsUser,
	}, nil
}

func (catalog *recordingCatalog) CreateGroup(_ context.Context, request GroupRequest) (GroupRef, error) {
	catalog.createGroupRequest = request
	return GroupRef{
		Name: request.Name,
		Zone: request.Zone,
		Type: irodstypes.IRODSUserRodsGroup,
	}, nil
}

func (catalog *recordingCatalog) AddGroupMember(_ context.Context, request GroupMemberRequest) error {
	catalog.addMemberRequest = request
	return nil
}

func (catalog *recordingCatalog) RemoveGroupMember(_ context.Context, request GroupMemberRequest) error {
	catalog.removeMemberRequest = request
	return nil
}

type readOnlyCatalog struct{}

func (readOnlyCatalog) ListGroupSummaries(_ context.Context, _ GroupSummaryOptions) ([]GroupSummary, error) {
	return nil, nil
}

func (readOnlyCatalog) ListUserMembershipSummaries(_ context.Context, _ UserMembershipSummaryOptions) ([]UserMembershipSummary, error) {
	return nil, nil
}

func (readOnlyCatalog) ListGroupsForUser(_ context.Context, _ GroupsForUserOptions) ([]GroupRef, error) {
	return nil, nil
}

func (readOnlyCatalog) SearchPrincipals(_ context.Context, _ PrincipalSearchOptions) ([]PrincipalSearchResult, error) {
	return nil, nil
}
