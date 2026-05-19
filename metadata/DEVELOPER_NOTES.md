# metadata Developer Notes

This document captures the current design checkpoint for expanding metadata
query support in `go-irodsclient-extensions`. The immediate goal is a reusable
query layer for iRODS collection and data object AVU searches, with a shape that
can be consumed by both `irods-go-rest` and `irods-go-drs`.

## Design Goals

- Keep low-level protocol mechanics in `go-irodsclient`.
- Build a higher-level query API in `go-irodsclient-extensions/metadata`.
- Return the existing `github.com/cyverse/go-irodsclient/fs.Entry` model as the
  unified entry representation.
- Support data objects and collections through one public query surface.
- Lock selected iCAT columns to the fields needed to populate `fs.Entry`.
- Keep condition construction flexible, using the same equality and like
  semantics as `irods/message/query_request.go`.
- Default data object results to one logical entry per data object, using
  replication status to prefer the master/good replica as `go-irodsclient` does.
- Optimize for long-running AVU queries and paged consumption by REST and DRS.

## Proposed Public API

The base API should expose a domain-level query builder over `fs.Entry`, not raw
iCAT column numbers. The implementation can map fields to the data object and
collection iCAT columns internally.

```go
package metadata

import cyfs "github.com/cyverse/go-irodsclient/fs"

type Entry = cyfs.Entry

type EntryKind string

const (
	EntryKindDataObject EntryKind = "data_object"
	EntryKindCollection EntryKind = "collection"
)

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

type EntryOperator string

const (
	OpEqual EntryOperator = "="
	OpLike  EntryOperator = "like"
)

type EntryCondition struct {
	Field EntryField
	Op    EntryOperator
	Value string
}

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

type EntryQueryResult struct {
	Entries     []*Entry
	MatchedAVUs map[string][]AVUStat
	Page        EntryQueryPage
}
```

`MatchedAVUs`, when requested, should be keyed by canonical entry path.

## Builder Direction

The builder should mirror the useful parts of
`irods/message/query_request.go`: callers express equal and like conditions, and
the adapter converts them into the right `AddEqualStringCondition` or
`AddLikeStringCondition` calls through `go-irodsclient`. Multiple conditions are
ANDed, matching base GenQuery behavior. `EntryCondition.Value` is a pattern
input rather than a raw GenQuery condition string; `OpLike` should accept both
`*` and `%` and normalize to SQL `%` at execution.

```go
query := metadata.NewEntryQuery().
	BothKinds().
	Equal(metadata.FieldAVUAttrib, "foo:bar").
	Like(metadata.FieldAVUValue, "frog%").
	Limit(100).
	Build()
```

Expected builder helpers:

```go
func NewEntryQuery() *EntryQueryBuilder
func (b *EntryQueryBuilder) DataObjects() *EntryQueryBuilder
func (b *EntryQueryBuilder) Collections() *EntryQueryBuilder
func (b *EntryQueryBuilder) BothKinds() *EntryQueryBuilder
func (b *EntryQueryBuilder) Equal(field EntryField, value string) *EntryQueryBuilder
func (b *EntryQueryBuilder) Like(field EntryField, value string) *EntryQueryBuilder
func (b *EntryQueryBuilder) Scope(root string, mode EntryQueryScopeMode) *EntryQueryBuilder
func (b *EntryQueryBuilder) Limit(limit int) *EntryQueryBuilder
func (b *EntryQueryBuilder) Cursor(cursor *EntryQueryCursor) *EntryQueryBuilder
func (b *EntryQueryBuilder) Build() EntryQuery
```

Boolean semantics for the first implementation:

- Support AND-only condition lists.
- Do not add OR/grouping through client-side multi-query expansion.
- Reject unsupported boolean/grouping JSON rather than silently approximating
  behavior.
- Add OR/grouping only if it can be expressed directly through base GenQuery or
  if a later design explicitly accepts the cost and semantics of client-side
  query composition.

## AVU Convenience Helpers

The API should include shorthand helpers for common AVU searches. These helpers
should accept both shell-style `*` and SQL-style `%` wildcards.

Example: all entries with AVU attribute `foo:bar`, AVU values like `frog*`, and
any unit.

```go
query := metadata.NewEntryQuery().
	BothKinds().
	AVU("foo:bar", "frog*", metadata.AnyUnit).
	Limit(100).
	Build()
```

Equivalent expanded form:

```go
query := metadata.NewEntryQuery().
	BothKinds().
	Equal(metadata.FieldAVUAttrib, "foo:bar").
	Like(metadata.FieldAVUValue, "frog%").
	Build()
```

Suggested helper surface:

```go
const AnyUnit = "*"

func (b *EntryQueryBuilder) AVU(attrib string, value string, unit string) *EntryQueryBuilder
func (b *EntryQueryBuilder) AVUAttrib(pattern string) *EntryQueryBuilder
func (b *EntryQueryBuilder) AVUValue(pattern string) *EntryQueryBuilder
func (b *EntryQueryBuilder) AVUUnit(pattern string) *EntryQueryBuilder
func AVUConditions(attrib string, value string, unit string) []EntryCondition
```

Convenience helper rules:

- `*` and `%` mean "any" and omit that condition.
- Values containing `*` are converted to SQL `%` and emitted as `like`.
- Values containing `%` are emitted as `like`.
- Values without wildcard characters are emitted as `=`.
- Exact empty-string matching, if needed, should be expressed through the lower
  level `Equal` builder method rather than the AVU shorthand.

## JSON Query Format

The query package should support JSON serialization and deserialization for
durable entry query definitions. This is intended for future virtual collection
definitions, where the stored payload must represent the query itself rather
than one execution page.

The JSON definition must be capable of representing every query supported by
the new `EntryQuery` structure, except runtime paging state. The durable JSON
model may include defaults such as preferred limit, matched AVU inclusion,
totals inclusion, and replica policy, but it should not include `Cursor`, iRODS
`continueIndex`, or branch offsets from a specific execution.

Suggested API:

```go
type EntryQueryDefinition struct {
	Version       string                 `json:"version"`
	Type          string                 `json:"type"`
	Kinds         []EntryKind            `json:"kinds,omitempty"`
	Scope         *EntryQueryScope       `json:"scope,omitempty"`
	Conditions    []EntryCondition       `json:"conditions,omitempty"`
	Defaults      EntryQueryDefaults     `json:"defaults,omitempty"`
	ReplicaPolicy ReplicaPolicy          `json:"replica_policy,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type EntryQueryScope struct {
	Root       string              `json:"root,omitempty"`
	Mode       EntryQueryScopeMode `json:"mode,omitempty"`
	PathHintable bool  `json:"path_hintable,omitempty"`
}

type EntryQueryScopeMode string

const (
	EntryQueryScopeSelf        EntryQueryScopeMode = "self"
	EntryQueryScopeChildren    EntryQueryScopeMode = "children"
	EntryQueryScopeDescendants EntryQueryScopeMode = "descendants"
	EntryQueryScopeSelfAndChildren    EntryQueryScopeMode = "self_and_children"
	EntryQueryScopeSelfAndDescendants EntryQueryScopeMode = "self_and_descendants"
	EntryQueryScopeAbsolute    EntryQueryScopeMode = "absolute"
)

type EntryQueryDefaults struct {
	Limit              int  `json:"limit,omitempty"`
	IncludeTotals      bool `json:"include_totals,omitempty"`
	IncludeMatchedAVUs bool `json:"include_matched_avus,omitempty"`
}

func MarshalEntryQueryDefinition(def EntryQueryDefinition) ([]byte, error)
func ParseEntryQueryDefinition(data []byte) (EntryQueryDefinition, error)
func (def EntryQueryDefinition) ToEntryQuery(options EntryQueryExecutionOptions) (EntryQuery, error)
func EntryQueryDefinitionFromQuery(query EntryQuery) EntryQueryDefinition
func AVUConditions(attrib string, value string, unit string) []EntryCondition
```

Canonical JSON should use the generic condition list because it can represent
all supported fields, not just AVUs:

```json
{
  "version": "metadata.entry_query.v1",
  "type": "entry_query",
  "kinds": ["data_object", "collection"],
  "scope": {
    "root": "/tempZone/home/alice",
    "mode": "descendants",
    "path_hintable": true
  },
  "conditions": [
    {"field": "avu.attrib", "op": "=", "value": "foo:bar"},
    {"field": "avu.value", "op": "like", "value": "frog*"}
  ],
  "replica_policy": "single",
  "defaults": {
    "limit": 100,
    "include_totals": false,
    "include_matched_avus": false
  }
}
```

Deserializer rules:

- `type` should accept only `entry_query` as the durable JSON definition type.
- `conditions` is the authoritative persisted representation because it can
  serialize any supported query.
- AVU convenience belongs in Go helpers and UI builders that emit canonical
  `conditions`; it should not create a second REST/saved-query JSON shape.
- `AVUConditions(attrib, value, unit)` should be used where callers want
  shorthand wildcard handling while still producing the canonical condition
  list. `*`, `%`, and blank omit that AVU field.
- Unknown fields, unknown operators, and unknown kinds should return validation
  errors.
- Boolean grouping and OR expressions are not part of the initial JSON schema.
  Query definitions containing them should be rejected.
- Empty `kinds` means both collections and data objects.
- Empty `scope` means the query itself is unscoped. Scoped AVU queries should be
  represented directly in this metadata query model rather than bolted on only
  by REST or DRS consumers.
- Wildcards should be preserved in serialized JSON as user-facing values. SQL
  wildcard normalization should happen when building the iRODS GenQuery
  condition, not when storing the query definition.

Virtual collection fit:

- A future `virtualcollections` provider can store the `EntryQueryDefinition` as
  its provider-specific payload.
- Virtual collection definitions should still own virtual collection metadata
  such as id, display name, enabled state, and provider type.
- The metadata query definition should own only query semantics: entry kinds,
  AVU/condition filters, scope hints, defaults, and replica policy.
- `path_hintable` maps naturally to scoped execution: when true, the provider
  can constrain the stored query by a normalized path hint under `scope.root`.

## Adapter Direction

`metadata/irodsfs.Adapter` should expose the execution API backed by
`go-irodsclient`.

```go
func (adapter *Adapter) QueryEntries(query metadata.EntryQuery) (metadata.EntryQueryResult, error)
```

Potential streaming API for long-running consumers:

```go
type EntryQueryIterator interface {
	Next(limit int) (metadata.EntryQueryResult, error)
	Close() error
}

func (adapter *Adapter) OpenEntryQuery(query metadata.EntryQuery) (metadata.EntryQueryIterator, error)
```

The first implementation can start with `QueryEntries`; the iterator can follow
once cursor semantics settle.

## Paging Model

iRODS GenQuery paging is per query branch. Since collection and data object
queries are separate GenQueries, the extension API should own branch progress
and present one logical page to callers.

```go
type EntryQueryPage struct {
	Limit    int
	HasMore  bool
	Next     *EntryQueryCursor
	Returned EntryQueryCounts
	Scanned  EntryQueryCounts
	Totals   *EntryQueryCounts
}

type EntryQueryCounts struct {
	Collections int
	DataObjects int
}

type EntryQueryCursor struct {
	Collections EntryBranchCursor
	DataObjects EntryBranchCursor
	Phase       EntryQueryPhase
}

type EntryBranchCursor struct {
	Offset    int
	Exhausted bool
}
```

Notes:

- The REST and DRS layers should treat cursors as opaque page tokens.
- Exact totals should be optional because they can require extra scanning after
  replica and AVU duplicate collapse.
- The result page should expose enough diagnostic counts to understand whether
  work was spent in the collection branch, data object branch, or both.

Cursor implementation options:

| Option | Pros | Cons |
| --- | --- | --- |
| Logical branch offsets | Easy to serialize, inspect, test, and resume across processes. Stable enough for REST/DRS page tokens when deterministic branch ordering is available. Does not expose iRODS protocol details. | Requires the implementation to skip already-seen logical rows on each page, which can be expensive for deep pages. Harder to make exact when rows are collapsed across replicas or duplicate AVU matches. Sensitive to catalog changes between page requests and to non-deterministic server ordering. |
| Internal GenQuery continue indices | Most efficient for a single active scan because it resumes where iRODS stopped. Avoids repeated skipping for long-running in-process queries. Maps closely to existing `go-irodsclient` paging loops. | Continue indices are per GenQuery branch and may be server/session scoped. They are poor durable REST tokens and awkward to persist in virtual collection definitions. They leak low-level protocol state into a higher-level API. |
| Opaque encoded state object | Gives the library room to combine branch offsets, continue indices, phase, sort, query hash, and future fields without changing public REST/DRS contracts. Can validate that a token matches the original query. Best fit for user-facing page tokens. | More implementation complexity. Harder to debug without tooling. Still needs an internal choice between offsets and continue indices. Tokens may need signing/versioning if exposed over REST. |

Decision: use logical branch offsets for the initial implementation. This keeps
the query API simple, durable, and easy to serialize for REST, DRS, and virtual
collection use cases. Add GenQuery continue-index or opaque-token complexity
only if measured performance requires it.

Implementation guardrails for logical offsets:

- Prefer deterministic ordering for every paged query, but do not invent
  ordering behavior outside current `go-irodsclient`/base GenQuery capability.
  The current client uses `AddSelect(..., 1)` and does not expose order-by
  helpers. If order-by flags are not added to the base client, v1 paging should
  document that branch offsets follow the server's returned order and are
  best-effort across catalog changes.
- Store offsets per branch. Collection and data object progress must advance
  independently because they are backed by separate GenQueries.
- Offsets count logical entries after duplicate AVU rows and data object
  replicas are collapsed. They must not count raw GenQuery rows.
- Paging is not snapshot-isolated. Catalog changes between requests may cause
  offset-style paging to skip or repeat entries; this should be documented for
  REST, DRS, and virtual collection consumers.
- Totals are optional and should remain opt-in through `IncludeTotals`.
- Cursor structs should be versionable even if they start simple, so future
  implementations can add query hashes or more efficient continuation state
  without breaking stored cursors.
- Go callers can use the explicit `EntryQueryCursor` struct. REST and DRS can
  encode that cursor as an opaque page token at their API boundary later.

## Query Execution Notes

- Data object and collection queries should be generated separately.
- `FieldAVUAttrib`, `FieldAVUValue`, and `FieldAVUUnit` map to
  `META_DATA_*` columns for data objects and `META_COLL_*` columns for
  collections.
- Path scoping is part of the base metadata query API. For example, callers
  should be able to search for an AVU pattern under collection `/foo/bar`
  without REST or DRS adding their own path constraints.
- Scope mapping:
  - `self` limits collection results by `COLL_NAME = root` and skips the data
    object branch because the scope root is a collection entry.
  - `children` limits collections by `COLL_PARENT_NAME = root` and data objects
    by `COLL_NAME = root`.
  - `descendants` limits collections by `COLL_NAME like root/%` and data
    objects by `COLL_NAME like root/%`. This intentionally searches below the
    scoped collection. Direct child data objects at exactly `root` are covered by
    `children`; a true full subtree containing both direct children and deeper
    descendants would require OR/client unioning and is out of scope for v1.
  - `self_and_children` and `self_and_descendants` are explicit vocabulary but
    rejected by the single-GenQuery implementation because they require OR or
    explicit caller-side composition.
  - `absolute` leaves path unconstrained.
- Scope roots must be normalized absolute iRODS paths before query execution.
- Scope conditions should be emitted as normal GenQuery conditions, not applied
  as client-side post filters.
- Conditions should be represented as supported field/operator/value triples,
  not raw GenQuery condition strings. This keeps the API aligned with existing
  `AddEqualStringCondition` and `AddLikeStringCondition` semantics and avoids
  exposing unsupported operators or SQL fragments.
- Data-object-only fields, such as `data_type`, `resource`, and `checksum`,
  should either filter the query to data objects or return a validation error
  when collection-only search is requested.
- Default replica policy should return one `fs.Entry` per logical data object.
- The selected replica should populate `Entry.IRODSReplicas` with one replica so
  callers can derive owner, checksum, resource, and timestamps consistently.
- Replica selection should use replication status and prefer the master/good
  replica, matching existing `go-irodsclient` master replica search helpers.
  Where multiple good replicas are returned, preserve the existing
  `go-irodsclient` tie-breaker behavior rather than inventing a new ordering.
- Query results should include only the unified `fs.Entry` listing by default.
  Matched AVUs can be returned when callers opt in through
  `IncludeMatchedAVUs`.
- Initial condition evaluation is AND-only and should map directly to base
  GenQuery conditions. Do not synthesize OR/grouped behavior by issuing multiple
  queries and merging results in the client.

## Base Client Reuse

This package should not duplicate behavior already available in
`go-irodsclient`.

Existing base-client capabilities to reuse:

- `fs.FileSystem.SearchByMeta(metaName, metaValue)` already returns unified
  `*fs.Entry` values for exact AVU attribute/value searches across collections
  and data objects.
- `irods/fs.SearchCollectionsByMeta` and
  `irods/fs.SearchDataObjectsMasterReplicaByMeta` already implement the exact
  collection/data object AVU branches.
- `irods/fs.SearchCollectionsByMetaWildcard` and
  `irods/fs.SearchDataObjectsMasterReplicaByMetaWildcard` already implement
  exact attribute plus wildcard value searches.
- `fs.NewEntryFromCollection` and `fs.NewEntryFromDataObject` already map base
  iRODS types into `fs.Entry`.
- `fs.FileSystem.GetMetadataConnection` and `ReturnMetadataConnection` expose
  the connection lifecycle needed by extension adapters.

The new metadata query layer should add only the missing reusable behavior:

- condition building across supported entry fields, including AVU attribute,
  value, and unit
- wildcard matching on AVU attribute and unit, not only value
- path-scoped AVU queries expressed as GenQuery conditions
- logical branch-offset paging
- durable JSON query definitions for virtual collections
- optional matched AVU details

Where an `EntryQuery` exactly matches an existing base-client operation, the
adapter should delegate to that operation rather than reimplementing it.

Current implementation strategy: do not push changes into `go-irodsclient` as
part of this work. Build the metadata query layer in
`go-irodsclient-extensions` using only the public capabilities currently
available from the base client. If this reveals lower-level primitives that
belong in `go-irodsclient`, capture them as future migration candidates rather
than making the initial feature depend on upstream/base-client changes.

## Future go-irodsclient Migration Candidates

Use this section to record potential issues or pull requests for later
refactoring into `go-irodsclient`. These are not blockers for the first
`go-irodsclient-extensions` implementation.

Candidate areas:

- Generic GenQuery execution helpers that accept a validated field/operator
  model without exposing raw SQL fragments.
- Base-client search helpers for AVU attribute, value, and unit conditions,
  rather than only exact attribute plus exact/wildcard value.
- Path-scoped AVU search helpers for collections and data objects.
- Order-by support or documented ordering flags for GenQuery selects, if
  supported by iRODS and needed for stable deep paging.
- A reusable paged query abstraction that exposes logical result counts while
  hiding `continueIndex`.
- Optional matched-AVU return support when search rows need to carry the AVUs
  that caused the match.

When implementation work surfaces one of these needs, add notes here with:

- the extension use case
- the current workaround in `go-irodsclient-extensions`
- the proposed `go-irodsclient` API shape
- any compatibility or migration concerns

## irods-go-rest Fit

The REST API should not overload `/api/v1/path/children` with arbitrary AVU
query semantics. That route currently represents structural collection listing
and name-pattern search over already-materialized path listings.

A future REST surface can be:

```http
POST /api/v1/path/query
```

Example request:

```json
{
  "irods_path": "/tempZone/home/alice",
  "search_scope": "descendants",
  "kinds": ["data_object", "collection"],
  "conditions": [
    {"field": "avu.attrib", "op": "=", "value": "foo:bar"},
    {"field": "avu.value", "op": "like", "value": "frog*"}
  ],
  "limit": 100,
  "page_token": ""
}
```

Example response shape:

```json
{
  "irods_path": "/tempZone/home/alice",
  "paths": [],
  "page": {
    "limit": 100,
    "has_more": true,
    "next_page_token": "opaque-token"
  },
  "query": {
    "search_scope": "descendants",
    "kinds": ["data_object", "collection"]
  }
}
```

`irods-go-rest` can map returned `*fs.Entry` values into its existing
`domain.PathEntry` response model. Default query responses should omit replica
arrays while still using the selected replica for top-level data object fields.

### irods-go-rest Semantic Alignment

Compatibility expectations with the current and planned `/path` semantics:

| Area | Existing or planned REST behavior | Metadata query alignment |
| --- | --- | --- |
| Route ownership | `/api/v1/path/children` lists structural children and supports name-pattern search. | Keep AVU/path metadata search on a separate `/api/v1/path/query` route so query results are not mislabeled as children. |
| Response collection name | `PathChildrenResponse` uses `children`. | Use `paths` or `entries` for query responses because matches may come from many collections and are not necessarily direct children. |
| Entry model | REST uses `domain.PathEntry`; `id` is currently the canonical path string, and `kind` is `data_object` or `collection`. | Extension returns `fs.Entry`; REST adapters translate `Entry.Type` `file`/`directory` into `data_object`/`collection` and keep REST `id` as the path string. |
| Scope names | Current children search supports `children`, `subtree`, and `absolute`; `subtree` is implemented by recursively listing direct children and descendants. | Metadata query v1 exposes `self`, `children`, `descendants`, and `absolute`, with `self_and_children` and `self_and_descendants` reserved but rejected for single GenQuery execution. Do not call the GenQuery-only descendant scope `subtree` because it does not include direct child data objects at `root` without OR/client unioning. |
| Path validation | `/path/children` stats the requested path and returns not found when it is not a collection. | REST should validate `irods_path` as an absolute collection path before executing `children` or `descendants` scoped metadata queries. The extension layer should still normalize and reject non-absolute scope roots. |
| Pagination | `/path/children` uses flat `offset` and `limit` after in-memory filtering and sorting. | `/path/query` should use `page_token` derived from `EntryQueryCursor`; do not reuse flat offsets because collection and data-object branches advance independently. |
| Sorting | `/path/children` supports `sort`/`order` because results are collected before paging. | Do not promise REST sort/order for metadata query v1 unless the extension can express deterministic GenQuery ordering without breaking branch-offset paging. |
| Match counts | `/path/children` returns `matched_count` after filtering. | Metadata query counts should remain opt-in and should report branch-aware counts when available; avoid implying snapshot-exact totals by default. |
| Case sensitivity | `/path/children` implements case-insensitive matching in Go for name patterns. | Do not expose `case_sensitive` for AVU `like` in v1 unless iRODS/base GenQuery support can provide it server-side. |
| Name patterns | `/path/children` `name_pattern` matches basename or absolute path depending on scope. | Metadata query callers should express path/name restrictions through explicit query conditions; AVU wildcard helpers should remain focused on AVU attrib/value/unit. |
| Metadata field | `domain.PathEntry.Metadata` is a `map[string]string` used by path representations when metadata is explicitly loaded. | Query results should not populate full metadata maps by default. If `IncludeMatchedAVUs` is requested, REST should expose matched AVUs as query-match details or a deliberate expansion, not as an implicit full AVU listing. |
| Replicas | REST omits `replicas` unless a verbose replica expansion is requested. | Keep replica arrays omitted in default query responses. The selected master/good replica is an internal source for checksum, resource, size, owner, and timestamps. |
| Links and mutations | Normal `PathEntry` links may include mutation links. Virtual collection results intentionally avoid mutation semantics at the virtual root. | Plain `/path/query` results can link to canonical path operations. Virtual collection adapters can strip or alter mutation links while still carrying canonical path handoff links. |

Current mismatch decisions:

- Do not mirror `/path/children?search_scope=subtree` in the extension API yet.
  The current REST subtree behavior includes direct children and descendants by
  recursively listing collections. Base GenQuery without OR/grouping can express
  direct children or descendants below the root, but not their union in one
  branch. The extension should keep the more precise `self`, `children`, and
  `descendants` names and let REST add a higher-level `subtree` alias later only
  if it deliberately composes multiple scopes.
- Do not map REST `offset` directly to `EntryQueryCursor`. REST can expose an
  opaque `page_token`, but the encoded value should remain derived from logical
  branch offsets in v1.
- Do not add REST-only conveniences, such as `case_sensitive` name matching, to
  the lower-level metadata query model unless they can be pushed into GenQuery
  conditions. Client-side filtering would make totals, cursors, and long-running
  AVU query optimization harder to reason about.
- Keep `fs.Entry` as the extension package boundary. `domain.PathEntry` remains
  an `irods-go-rest` adapter concern, and `irods-go-drs` can adapt the same
  `fs.Entry` listing into its own response model.

## Open Questions

- None currently for the metadata entry query plan. Revisit after the first
  implementation pass or when virtual collection integration starts.
