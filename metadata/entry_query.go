package metadata

import (
	"errors"
	"fmt"
	"strings"

	cyfs "github.com/cyverse/go-irodsclient/fs"
)

const (
	// AnyUnit omits an AVU unit condition in convenience builders.
	AnyUnit = "*"

	// DefaultEntryQueryLimit is used when callers do not provide a page size.
	DefaultEntryQueryLimit = 100
)

var (
	ErrInvalidEntryQuery = errors.New("invalid entry query")
)

// Entry is the unified filesystem entry returned by metadata entry queries.
type Entry = cyfs.Entry

// EntryKind selects the iRODS entry branch to query.
type EntryKind string

const (
	EntryKindDataObject EntryKind = "data_object"
	EntryKindCollection EntryKind = "collection"
)

// EntryField is a queryable logical field on a unified entry listing.
type EntryField string

const (
	FieldPath      EntryField = "path"
	FieldName      EntryField = "name"
	FieldOwner     EntryField = "owner"
	FieldDataType  EntryField = "data_type"
	FieldResource  EntryField = "resource"
	FieldChecksum  EntryField = "checksum"
	FieldAVUAttrib EntryField = "avu.attrib"
	FieldAVUValue  EntryField = "avu.value"
	FieldAVUUnit   EntryField = "avu.unit"
)

// EntryOperator maps to supported GenQuery condition operators.
type EntryOperator string

const (
	OpEqual EntryOperator = "="
	OpLike  EntryOperator = "like"
)

// EntryCondition is one ANDed condition in an entry query.
type EntryCondition struct {
	Field EntryField    `json:"field"`
	Op    EntryOperator `json:"op"`
	Value string        `json:"value"`
}

// ReplicaPolicy controls how data object replicas are represented.
type ReplicaPolicy string

const (
	ReplicaPolicySingle ReplicaPolicy = "single"
)

// EntryQueryScope constrains a query to a path scope.
type EntryQueryScope struct {
	Root         string              `json:"root,omitempty"`
	Mode         EntryQueryScopeMode `json:"mode,omitempty"`
	PathHintable bool                `json:"path_hintable,omitempty"`
}

// EntryQueryScopeMode controls how Scope.Root is applied.
type EntryQueryScopeMode string

const (
	EntryQueryScopeSelf        EntryQueryScopeMode = "self"
	EntryQueryScopeChildren    EntryQueryScopeMode = "children"
	EntryQueryScopeDescendants EntryQueryScopeMode = "descendants"
	// EntryQueryScopeSelfAndChildren and EntryQueryScopeSelfAndDescendants are
	// named explicitly so callers do not approximate them with unsafe LIKE
	// patterns. They require OR semantics and are rejected by this GenQuery v1
	// implementation.
	EntryQueryScopeSelfAndChildren    EntryQueryScopeMode = "self_and_children"
	EntryQueryScopeSelfAndDescendants EntryQueryScopeMode = "self_and_descendants"
	EntryQueryScopeAbsolute           EntryQueryScopeMode = "absolute"
)

// EntryQuery contains query conditions plus one execution page request.
type EntryQuery struct {
	Kinds              []EntryKind
	Scope              *EntryQueryScope
	Conditions         []EntryCondition
	Limit              int
	Cursor             *EntryQueryCursor
	IncludeTotals      bool
	IncludeMatchedAVUs bool
	ReplicaPolicy      ReplicaPolicy
}

// EntryQueryResult contains one unified page of query results.
type EntryQueryResult struct {
	Entries     []*Entry
	MatchedAVUs map[string][]AVUStat
	Page        EntryQueryPage
}

// EntryQueryPage describes the page returned by an entry query.
type EntryQueryPage struct {
	Limit    int
	HasMore  bool
	Next     *EntryQueryCursor
	Returned EntryQueryCounts
	Scanned  EntryQueryCounts
	Totals   *EntryQueryCounts
}

// EntryQueryCounts reports per-branch counts.
type EntryQueryCounts struct {
	Collections int
	DataObjects int
}

// EntryQueryCursor stores logical progress through the collection and data
// object branches.
type EntryQueryCursor struct {
	Collections EntryBranchCursor `json:"collections,omitempty"`
	DataObjects EntryBranchCursor `json:"data_objects,omitempty"`
	Phase       EntryQueryPhase   `json:"phase,omitempty"`
}

// EntryBranchCursor stores logical progress through one branch.
type EntryBranchCursor struct {
	Offset    int  `json:"offset,omitempty"`
	Exhausted bool `json:"exhausted,omitempty"`
}

// EntryQueryPhase identifies the branch to resume.
type EntryQueryPhase string

const (
	EntryQueryPhaseCollections EntryQueryPhase = "collections"
	EntryQueryPhaseDataObjects EntryQueryPhase = "data_objects"
	EntryQueryPhaseDone        EntryQueryPhase = "done"
)

// EntryQueryBuilder builds EntryQuery values.
type EntryQueryBuilder struct {
	query EntryQuery
}

// NewEntryQuery starts a query builder.
func NewEntryQuery() *EntryQueryBuilder {
	return &EntryQueryBuilder{
		query: EntryQuery{
			ReplicaPolicy: ReplicaPolicySingle,
		},
	}
}

// DataObjects limits the query to data objects.
func (builder *EntryQueryBuilder) DataObjects() *EntryQueryBuilder {
	builder.query.Kinds = []EntryKind{EntryKindDataObject}
	return builder
}

// Collections limits the query to collections.
func (builder *EntryQueryBuilder) Collections() *EntryQueryBuilder {
	builder.query.Kinds = []EntryKind{EntryKindCollection}
	return builder
}

// BothKinds queries collections and data objects.
func (builder *EntryQueryBuilder) BothKinds() *EntryQueryBuilder {
	builder.query.Kinds = []EntryKind{EntryKindCollection, EntryKindDataObject}
	return builder
}

// Equal adds an equality condition.
func (builder *EntryQueryBuilder) Equal(field EntryField, value string) *EntryQueryBuilder {
	builder.query.Conditions = append(builder.query.Conditions, EntryCondition{
		Field: field,
		Op:    OpEqual,
		Value: value,
	})
	return builder
}

// Like adds a like condition. The value may use either * or % wildcards.
func (builder *EntryQueryBuilder) Like(field EntryField, value string) *EntryQueryBuilder {
	builder.query.Conditions = append(builder.query.Conditions, EntryCondition{
		Field: field,
		Op:    OpLike,
		Value: value,
	})
	return builder
}

// Scope constrains the query to a collection root.
func (builder *EntryQueryBuilder) Scope(root string, mode EntryQueryScopeMode) *EntryQueryBuilder {
	builder.query.Scope = &EntryQueryScope{
		Root: root,
		Mode: mode,
	}
	return builder
}

// Limit sets the maximum number of entries to return.
func (builder *EntryQueryBuilder) Limit(limit int) *EntryQueryBuilder {
	builder.query.Limit = limit
	return builder
}

// Cursor resumes a query from a previous page.
func (builder *EntryQueryBuilder) Cursor(cursor *EntryQueryCursor) *EntryQueryBuilder {
	builder.query.Cursor = cursor
	return builder
}

// IncludeTotals requests branch total counts.
func (builder *EntryQueryBuilder) IncludeTotals(include bool) *EntryQueryBuilder {
	builder.query.IncludeTotals = include
	return builder
}

// IncludeMatchedAVUs requests AVU rows that caused each match when available.
func (builder *EntryQueryBuilder) IncludeMatchedAVUs(include bool) *EntryQueryBuilder {
	builder.query.IncludeMatchedAVUs = include
	return builder
}

// ReplicaPolicy sets the replica handling policy.
func (builder *EntryQueryBuilder) ReplicaPolicy(policy ReplicaPolicy) *EntryQueryBuilder {
	builder.query.ReplicaPolicy = policy
	return builder
}

// AVU adds AVU shorthand conditions. * and % omit a condition.
func (builder *EntryQueryBuilder) AVU(attrib string, value string, unit string) *EntryQueryBuilder {
	builder.query.Conditions = append(builder.query.Conditions, AVUConditions(attrib, value, unit)...)
	return builder
}

// AVUAttrib adds an AVU attribute pattern condition.
func (builder *EntryQueryBuilder) AVUAttrib(pattern string) *EntryQueryBuilder {
	builder.addPatternCondition(FieldAVUAttrib, pattern)
	return builder
}

// AVUValue adds an AVU value pattern condition.
func (builder *EntryQueryBuilder) AVUValue(pattern string) *EntryQueryBuilder {
	builder.addPatternCondition(FieldAVUValue, pattern)
	return builder
}

// AVUUnit adds an AVU unit pattern condition.
func (builder *EntryQueryBuilder) AVUUnit(pattern string) *EntryQueryBuilder {
	builder.addPatternCondition(FieldAVUUnit, pattern)
	return builder
}

// Build returns the query value.
func (builder *EntryQueryBuilder) Build() EntryQuery {
	return cloneEntryQuery(builder.query)
}

// AVUConditions returns canonical conditions for common AVU attrib/value/unit
// patterns. Blank values, "*", and "%" omit that AVU field. Values containing
// "*" or "%" use a like condition; all other values use equality.
func AVUConditions(attrib string, value string, unit string) []EntryCondition {
	conditions := []EntryCondition{}
	for _, candidate := range []struct {
		field EntryField
		value string
	}{
		{field: FieldAVUAttrib, value: attrib},
		{field: FieldAVUValue, value: value},
		{field: FieldAVUUnit, value: unit},
	} {
		condition, ok := conditionFromPattern(candidate.field, candidate.value)
		if ok {
			conditions = append(conditions, condition)
		}
	}
	return conditions
}

func (builder *EntryQueryBuilder) addPatternCondition(field EntryField, pattern string) {
	condition, ok := conditionFromPattern(field, pattern)
	if !ok {
		return
	}
	builder.query.Conditions = append(builder.query.Conditions, condition)
}

func conditionFromPattern(field EntryField, pattern string) (EntryCondition, bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" || pattern == "%" {
		return EntryCondition{}, false
	}
	operator := OpEqual
	if strings.ContainsAny(pattern, "*%") {
		operator = OpLike
	}
	return EntryCondition{
		Field: field,
		Op:    operator,
		Value: pattern,
	}, true
}

// NormalizeEntryQuery validates a query and applies default execution values.
func NormalizeEntryQuery(query EntryQuery) (EntryQuery, error) {
	normalized := cloneEntryQuery(query)

	kinds, err := normalizeEntryKinds(normalized.Kinds)
	if err != nil {
		return EntryQuery{}, err
	}
	normalized.Kinds = kinds

	if normalized.Limit <= 0 {
		normalized.Limit = DefaultEntryQueryLimit
	}

	if normalized.ReplicaPolicy == "" {
		normalized.ReplicaPolicy = ReplicaPolicySingle
	}
	if normalized.ReplicaPolicy != ReplicaPolicySingle {
		return EntryQuery{}, fmt.Errorf("%w: unsupported replica policy %q", ErrInvalidEntryQuery, normalized.ReplicaPolicy)
	}

	if normalized.Scope != nil {
		scope := *normalized.Scope
		scope.Root = normalizeIRODSPath(scope.Root)
		if scope.Mode == "" {
			scope.Mode = EntryQueryScopeChildren
		}
		switch scope.Mode {
		case EntryQueryScopeSelf, EntryQueryScopeChildren, EntryQueryScopeDescendants:
			if scope.Root == "" || scope.Root == "/" {
				return EntryQuery{}, fmt.Errorf("%w: scoped queries require an absolute collection root", ErrInvalidEntryQuery)
			}
		case EntryQueryScopeSelfAndChildren, EntryQueryScopeSelfAndDescendants:
			return EntryQuery{}, fmt.Errorf("%w: scope mode %q requires OR/grouping and cannot be expressed as one base GenQuery request", ErrInvalidEntryQuery, scope.Mode)
		case EntryQueryScopeAbsolute:
			scope.Root = ""
		default:
			return EntryQuery{}, fmt.Errorf("%w: unsupported scope mode %q", ErrInvalidEntryQuery, scope.Mode)
		}
		normalized.Scope = &scope
	}

	for _, condition := range normalized.Conditions {
		if err := validateEntryCondition(condition); err != nil {
			return EntryQuery{}, err
		}
	}

	if normalized.Cursor != nil {
		cursor := *normalized.Cursor
		if cursor.Collections.Offset < 0 || cursor.DataObjects.Offset < 0 {
			return EntryQuery{}, fmt.Errorf("%w: cursor offsets must be non-negative", ErrInvalidEntryQuery)
		}
		switch cursor.Phase {
		case "", EntryQueryPhaseCollections, EntryQueryPhaseDataObjects, EntryQueryPhaseDone:
		default:
			return EntryQuery{}, fmt.Errorf("%w: unsupported cursor phase %q", ErrInvalidEntryQuery, cursor.Phase)
		}
		normalized.Cursor = &cursor
	}

	return normalized, nil
}

func normalizeEntryKinds(kinds []EntryKind) ([]EntryKind, error) {
	if len(kinds) == 0 {
		return []EntryKind{EntryKindCollection, EntryKindDataObject}, nil
	}

	result := make([]EntryKind, 0, len(kinds))
	seen := map[EntryKind]bool{}
	for _, kind := range kinds {
		switch kind {
		case EntryKindCollection, EntryKindDataObject:
		default:
			return nil, fmt.Errorf("%w: unsupported entry kind %q", ErrInvalidEntryQuery, kind)
		}
		if seen[kind] {
			continue
		}
		seen[kind] = true
		result = append(result, kind)
	}
	return result, nil
}

func validateEntryCondition(condition EntryCondition) error {
	switch condition.Field {
	case FieldPath, FieldName, FieldOwner, FieldDataType, FieldResource, FieldChecksum, FieldAVUAttrib, FieldAVUValue, FieldAVUUnit:
	default:
		return fmt.Errorf("%w: unsupported condition field %q", ErrInvalidEntryQuery, condition.Field)
	}

	switch condition.Op {
	case OpEqual, OpLike:
	default:
		return fmt.Errorf("%w: unsupported condition operator %q", ErrInvalidEntryQuery, condition.Op)
	}
	return nil
}

// EntryQueryHasKind reports whether the normalized query includes a branch.
func EntryQueryHasKind(query EntryQuery, kind EntryKind) bool {
	for _, candidate := range query.Kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// EntryQueryHasAVUConditions reports whether a query has AVU predicates.
func EntryQueryHasAVUConditions(query EntryQuery) bool {
	for _, condition := range query.Conditions {
		switch condition.Field {
		case FieldAVUAttrib, FieldAVUValue, FieldAVUUnit:
			return true
		}
	}
	return false
}

// NormalizeLikePattern converts shell-style wildcards to GenQuery SQL wildcards.
func NormalizeLikePattern(pattern string) string {
	return strings.ReplaceAll(pattern, "*", "%")
}

func cloneEntryQuery(query EntryQuery) EntryQuery {
	clone := query
	clone.Kinds = append([]EntryKind(nil), query.Kinds...)
	clone.Conditions = append([]EntryCondition(nil), query.Conditions...)
	if query.Scope != nil {
		scope := *query.Scope
		clone.Scope = &scope
	}
	if query.Cursor != nil {
		cursor := *query.Cursor
		clone.Cursor = &cursor
	}
	return clone
}
