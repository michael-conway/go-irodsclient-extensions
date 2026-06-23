package usersandgroups

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

var (
	ErrMissingCatalog = errors.New("usersandgroups: missing catalog")
	ErrInvalidRequest = errors.New("usersandgroups: invalid request")
)

const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

// Catalog is the narrow catalog query API required by Service.
type Catalog interface {
	ListGroupSummaries(ctx context.Context, options GroupSummaryOptions) ([]GroupSummary, error)
	ListUserMembershipSummaries(ctx context.Context, options UserMembershipSummaryOptions) ([]UserMembershipSummary, error)
	ListGroupsForUser(ctx context.Context, options GroupsForUserOptions) ([]GroupRef, error)
	SearchPrincipals(ctx context.Context, options PrincipalSearchOptions) ([]PrincipalSearchResult, error)
}

// Service exposes list-scale user and group read helpers.
type Service struct {
	catalog Catalog
	zone    string
}

// NewService returns a user and group query service.
func NewService(catalog Catalog, defaultZone string) *Service {
	return &Service{
		catalog: catalog,
		zone:    strings.TrimSpace(defaultZone),
	}
}

type PrincipalKind string

const (
	PrincipalKindUser  PrincipalKind = "user"
	PrincipalKindGroup PrincipalKind = "group"
)

type GroupSummaryOptions struct {
	Zone   string
	Prefix string
	Limit  int
}

type UserMembershipSummaryOptions struct {
	Zone   string
	Prefix string
	Type   irodstypes.IRODSUserType
	Limit  int
}

type GroupsForUserOptions struct {
	Zone     string
	UserName string
	Limit    int
}

type PrincipalSearchOptions struct {
	Zone  string
	Query string
	Limit int
	Kinds []PrincipalKind
}

type GroupSummary struct {
	ID          int64
	Name        string
	Zone        string
	Type        irodstypes.IRODSUserType
	MemberCount int
}

type UserMembershipSummary struct {
	ID     int64
	Name   string
	Zone   string
	Type   irodstypes.IRODSUserType
	Groups []GroupRef
}

type GroupRef struct {
	ID   int64
	Name string
	Zone string
	Type irodstypes.IRODSUserType
}

type PrincipalSearchResult struct {
	ID    int64
	Name  string
	Zone  string
	Type  irodstypes.IRODSUserType
	Kind  PrincipalKind
	Score int
}

func (service *Service) ListGroupSummaries(ctx context.Context, options GroupSummaryOptions) ([]GroupSummary, error) {
	if err := service.validate(ctx); err != nil {
		return nil, err
	}
	normalized := GroupSummaryOptions{
		Zone:   service.zoneFor(options.Zone),
		Prefix: strings.TrimSpace(options.Prefix),
		Limit:  normalizeLimit(options.Limit),
	}
	return service.catalog.ListGroupSummaries(ctx, normalized)
}

func (service *Service) ListUserMembershipSummaries(ctx context.Context, options UserMembershipSummaryOptions) ([]UserMembershipSummary, error) {
	if err := service.validate(ctx); err != nil {
		return nil, err
	}
	normalized := UserMembershipSummaryOptions{
		Zone:   service.zoneFor(options.Zone),
		Prefix: strings.TrimSpace(options.Prefix),
		Type:   options.Type,
		Limit:  normalizeLimit(options.Limit),
	}
	users, err := service.catalog.ListUserMembershipSummaries(ctx, normalized)
	if err != nil {
		return nil, err
	}
	for idx := range users {
		sortGroups(users[idx].Groups)
	}
	return users, nil
}

func (service *Service) ListGroupsForUser(ctx context.Context, options GroupsForUserOptions) ([]GroupRef, error) {
	if err := service.validate(ctx); err != nil {
		return nil, err
	}
	username := strings.TrimSpace(options.UserName)
	if username == "" {
		return nil, fmt.Errorf("%w: user name is required", ErrInvalidRequest)
	}
	normalized := GroupsForUserOptions{
		Zone:     service.zoneFor(options.Zone),
		UserName: username,
		Limit:    normalizeLimit(options.Limit),
	}
	groups, err := service.catalog.ListGroupsForUser(ctx, normalized)
	if err != nil {
		return nil, err
	}
	sortGroups(groups)
	return groups, nil
}

func (service *Service) SearchPrincipals(ctx context.Context, options PrincipalSearchOptions) ([]PrincipalSearchResult, error) {
	if err := service.validate(ctx); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInvalidRequest)
	}
	normalized := PrincipalSearchOptions{
		Zone:  service.zoneFor(options.Zone),
		Query: query,
		Limit: normalizeLimit(options.Limit),
		Kinds: normalizeKinds(options.Kinds),
	}
	return service.catalog.SearchPrincipals(ctx, normalized)
}

func (service *Service) validate(ctx context.Context) error {
	if service == nil || service.catalog == nil {
		return ErrMissingCatalog
	}
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}

func (service *Service) zoneFor(zone string) string {
	if trimmed := strings.TrimSpace(zone); trimmed != "" {
		return trimmed
	}
	return service.zone
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func normalizeKinds(kinds []PrincipalKind) []PrincipalKind {
	if len(kinds) == 0 {
		return []PrincipalKind{PrincipalKindUser, PrincipalKindGroup}
	}

	seen := map[PrincipalKind]bool{}
	normalized := make([]PrincipalKind, 0, len(kinds))
	for _, kind := range kinds {
		switch kind {
		case PrincipalKindUser, PrincipalKindGroup:
			if !seen[kind] {
				seen[kind] = true
				normalized = append(normalized, kind)
			}
		}
	}
	if len(normalized) == 0 {
		return []PrincipalKind{PrincipalKindUser, PrincipalKindGroup}
	}
	return normalized
}

func sortGroups(groups []GroupRef) {
	sort.SliceStable(groups, func(i int, j int) bool {
		if groups[i].Zone != groups[j].Zone {
			return groups[i].Zone < groups[j].Zone
		}
		return groups[i].Name < groups[j].Name
	})
}
