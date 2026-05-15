# metadata

`metadata` generates versioned JSON metadata manifests for iRODS collections
and data objects.

## Usage

```go
service, err := metadata.NewService(filesystem)
if err != nil {
    return err
}

manifest, err := service.GenerateManifest("/tempZone/home/test1/object.txt")
if err != nil {
    return err
}

_ = manifest
```

## Entry AVU Queries

`metadata/irodsfs` can execute AVU-backed entry queries that return the unified
`fs.Entry` shape used by `go-irodsclient`. Selects are fixed to entry fields;
callers provide only supported conditions and execution options.

```go
adapter := metadatairodsfs.NewAdapter(filesystem)

query := metadata.NewEntryQuery().
    BothKinds().
    Scope("/tempZone/home/test1", metadata.EntryQueryScopeDescendants).
    AVU("project", "frog*", metadata.AnyUnit).
    IncludeMatchedAVUs(true).
    Limit(100).
    Build()

result, err := adapter.QueryEntries(query)
if err != nil {
    return err
}

for _, entry := range result.Entries {
    _ = entry.Path
    _ = result.MatchedAVUs[entry.Path]
}
```

The AVU shorthand accepts `*` or `%` as wildcards. A bare `*` or `%` omits that
part of the AVU predicate, so this searches by AVU attribute only:

```go
query := metadata.NewEntryQuery().
    Collections().
    Scope("/tempZone/home/test1", metadata.EntryQueryScopeChildren).
    AVU("iRODS:S3:Bucket", "*", metadata.AnyUnit).
    IncludeMatchedAVUs(true).
    Build()
```

## Scope Semantics

The executable scope modes map directly to base GenQuery conditions:

| Mode | Collection branch | Data object branch |
| --- | --- | --- |
| `self` | `COLL_NAME = root` | skipped |
| `children` | `COLL_PARENT_NAME = root` | `COLL_NAME = root` |
| `descendants` | `COLL_NAME like root/%` | `COLL_NAME like root/%` |
| `absolute` | no path scope condition | no path scope condition |

`self_and_children` and `self_and_descendants` are named in the API vocabulary
but rejected by the current GenQuery implementation. They require OR/grouping
(`COLL_NAME = root OR COLL_PARENT_NAME = root`, for example), and this package
does not synthesize OR behavior by issuing hidden multi-query unions. Callers
that need inclusive behavior should execute `self` plus `children` or
`descendants` explicitly and merge by canonical path.

Approximate inclusive scope with `LIKE root%`; it is not path-boundary
safe and can match sibling names such as `/tempZone/home/test10` when the root
is `/tempZone/home/test1`.

## JSON Definitions

Entry AVU queries can be serialized for durable definitions such as virtual
collections:

```json
{
  "version": "metadata.entry_query.v1",
  "type": "entry_query",
  "kinds": ["collection"],
  "scope": {"root": "/tempZone/home/test1", "mode": "children"},
  "conditions": [
    {"field": "avu.attrib", "op": "=", "value": "iRODS:S3:Bucket"}
  ],
  "defaults": {"limit": 100, "include_matched_avus": true}
}
```

Boolean grouping and OR expressions are rejected unless they can be mapped to
base GenQuery without client-side filtering or hidden query expansion.

## Error Semantics

Sentinel errors intended for `errors.Is` checks:

- `ErrMissingFilesystem`
- `ErrInvalidIRODSPath`
- `ErrInvalidOutputPath`
- `ErrPathStatMissing` (filesystem returned no stat entry)

Operational/storage/rendering errors are returned with context using `%w`, so
callers can match underlying filesystem/IO errors without parsing strings.

## Integration Notes

- The service requires a filesystem implementation satisfying
  `MetadataManifestFilesystem`; `metadata/irodsfs` provides an adapter for
  `go-irodsclient/fs`.
- Manifest generation includes iRODS host/port/zone context and AVU payloads
  from the target path.
- Integration coverage exists in
  `integration/metadata_manifest_integration_test.go` for live iRODS collection
  and data-object manifest generation flows.
- Entry-query integration coverage exists in
  `integration/metadata_entry_query_integration_test.go` for live scoped AVU queries,
  matched AVU expansion, child/descendant scope behavior, logical cursors, and
  default single-replica data object entries.
