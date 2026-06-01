# go-irodsclient-extensions

`go-irodsclient-extensions` is a shared Go library for higher-level iRODS client functionality built on top of [`go-irodsclient`](https://github.com/cyverse/go-irodsclient).

The intent of this repository is to hold reusable packages that sit above the base client and can be shared across rich iRODS applications, service layers, browser-facing APIs, and administrative tooling.

## Install

```bash
go get github.com/michael-conway/go-irodsclient-extensions@v1.0.0-alpha
```

The module path is unchanged for `v1` releases:

```go
import "github.com/michael-conway/go-irodsclient-extensions/metadata"
```

See [CHANGELOG.md](CHANGELOG.md) for release notes and [RELEASE.md](RELEASE.md)
for the release checklist.

## Scope

This repository is for extensions such as:

- metadata and AVU helpers
- OIDC bearer token verification and iRODS account construction
- ticket lifecycle helpers
- user-scoped persistence, favorites, and file carts
- S3 API administration helpers
- desired-state user/group reconciliation
- higher-level search and discovery helpers
- path and naming utilities
- shared client-side workflows that are too opinionated for the base client, but broadly useful across consumers

This repository is not intended to duplicate core transport, connection, or low-level filesystem behavior already provided by `go-irodsclient`.

## Package Map

| Package | Purpose |
| --- | --- |
| [`cmdcues`](cmdcues/README.md) | Small command/status cue helpers for CLIs and service responses. |
| [`favorites`](favorites/README.md) | User-scoped favorite path lifecycle under `~/.irodsext/favorites`. |
| [`filecart`](filecart/README.md) | User-scoped file cart lifecycle and AVU-backed cart entries under `~/.irodsext/filecarts`. |
| [`ignore`](ignore/README.md) | Ignore-rule parsing and path matching for iRODS-like path trees. |
| [`irodsauth`](irodsauth/README.md) | Shared request-auth to `IRODSAccount` construction helpers. |
| [`irodsuri`](irodsuri/README.md) | iRODS URI parsing and formatting helpers. |
| [`metadata`](metadata/README.md) | Metadata manifests, unified entry AVU/file queries, JSON query definitions, and saved user queries. |
| `metadata/irodsfs` | `go-irodsclient/fs` adapter for metadata entry queries. |
| [`oidcverify`](oidcverify/README.md) | OIDC bearer token introspection and userinfo verification helpers. |
| [`s3admin`](s3admin/README.md) | AVU-backed iRODS S3 API bucket and user-key mapping workflows. |
| `s3admin/irodsfs` | `go-irodsclient/fs` adapter for S3 admin workflows, including optimized metadata queries. |
| [`searchplugin`](searchplugin/README.md) | OpenAPI-based search plugin registry and client support. |
| [`tickets`](tickets/README.md) | Ticket creation, validation, and bearer-token lifecycle helpers. |
| [`usersync`](usersync/README.md) | Desired-state iRODS user/group reconciliation helpers for sync controllers and REST services. |
| `usersync/irodsfs` | `go-irodsclient/fs` adapter for user/group sync workflows. |
| [`userpersist`](userpersist/README.md) | Shared conventions and file helpers for user-scoped `~/.irodsext` persistence. |
| [`integration`](integration/README.md) | Build-tagged live iRODS integration tests. |
| `internal/testutil` | Shared test configuration and integration helpers for this repository. |

## Usage

Most packages are intentionally small and depend on narrow filesystem
interfaces. Application packages normally provide an adapter backed by
`go-irodsclient/fs`, or use an adapter package such as `metadata/irodsfs`.

### Metadata Manifests

```go
import "github.com/michael-conway/go-irodsclient-extensions/metadata"

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

The `filesystem` value implements the small `metadata.Filesystem` interface.
See [`metadata/README.md`](metadata/README.md) for the manifest shape.

### Unified Metadata Entry Queries

The metadata entry query API returns unified `fs.Entry` values for collections
and data objects. Query selects are fixed to the unified entry shape; callers
control kinds, scope, conditions, paging, and matched-AVU expansion.

```go
import (
    metadataext "github.com/michael-conway/go-irodsclient-extensions/metadata"
    metadatairodsfs "github.com/michael-conway/go-irodsclient-extensions/metadata/irodsfs"
)

adapter := metadatairodsfs.NewAdapter(filesystem)

query := metadataext.NewEntryQuery().
    BothKinds().
    Scope("/tempZone/home/test1", metadataext.EntryQueryScopeDescendants).
    AVU("project", "frog*", metadataext.AnyUnit).
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

Use `EntryQueryDefinition` when the query must be serialized, stored, or sent
over REST:

```go
definition := metadataext.EntryQueryDefinitionFromQuery(query)
data, err := metadataext.MarshalEntryQueryDefinition(definition)
```

### Saved User Metadata Queries

Saved entry queries persist canonical query JSON under the user's home
collection:

```text
/tempZone/home/test1/.irodsext/metadata_queries/<id>.entry-query.json
```

```go
service, err := metadataext.NewSavedEntryQueryService(filesystem, "/tempZone/home/test1")
if err != nil {
    return err
}

saved, err := service.CreateSavedQuery("Frog data", metadataext.EntryQueryDefinition{
    Type:  metadataext.EntryQueryDefinitionType,
    Kinds: []metadataext.EntryKind{metadataext.EntryKindDataObject},
    Scope: &metadataext.EntryQueryScope{
        Root: "/tempZone/home/test1",
        Mode: metadataext.EntryQueryScopeDescendants,
    },
    Conditions: metadataext.AVUConditions("project", "frog*", metadataext.AnyUnit),
    Defaults: metadataext.EntryQueryDefaults{Limit: 100},
})
if err != nil {
    return err
}

query, err := service.ToEntryQuery(saved.ID, metadataext.EntryQueryExecutionOptions{Limit: 25})
```

Saved queries intentionally persist query definitions only. Runtime cursor
state and page tokens are execution state and are not stored.

Saved query identity is the generated query ID. The display name is editable,
does not need to be unique, and defaults to `New Query` when blank. Descriptions
default to blank. Updating a saved query preserves its ID and fully replaces the
display fields and query definition; creating or duplicating a saved query
creates a new ID.

### User Persistence, Favorites, And File Carts

`userpersist` owns the shared `~/.irodsext/<context>/<file>` conventions.
Packages such as `favorites`, `filecart`, `s3admin`, and metadata saved queries
build on those conventions.

```go
import "github.com/michael-conway/go-irodsclient-extensions/favorites"

service, err := favorites.NewService(filesystem, "/tempZone/home/test1")
if err != nil {
    return err
}

err = service.AddFavorite("Inputs", "/tempZone/home/test1/project/inputs")
items, err := service.ListFavorites()
_ = items
```

```go
import "github.com/michael-conway/go-irodsclient-extensions/filecart"

cartService, err := filecart.NewService(filesystem, "/tempZone/home/test1")
if err != nil {
    return err
}

cart, err := cartService.CreateCart("Download set")
err = cartService.AddItem(cart.Path, "/tempZone/home/test1/project/results", filecart.EntryTypeCollection)
```

### S3 Administration

The `s3admin` package manages S3 bucket and user-secret mappings represented in
iRODS metadata. Consumers can provide their own filesystem adapter or use
`s3admin/irodsfs`.

```go
import (
    "github.com/michael-conway/go-irodsclient-extensions/s3admin"
    s3irodsfs "github.com/michael-conway/go-irodsclient-extensions/s3admin/irodsfs"
)

adapter := s3irodsfs.NewAdapter(filesystem)
service, err := s3admin.NewS3Service(adapter, s3admin.Config{
    ScanRootPath:    "/tempZone/home/test1",
    MappingFilePath: "/etc/irods/s3-buckets.json",
})
if err != nil {
    return err
}

buckets, err := service.ListBuckets(s3admin.ListOptions{Recursive: true})
_ = buckets
```

### Tickets

```go
import "github.com/michael-conway/go-irodsclient-extensions/tickets"

token := tickets.FormatBearerToken("ticket-name")

ticketName, ok := tickets.ParseBearerToken(token)
_ = ticketName
_ = ok
```

### URI, Ignore, Search Plugin, And Command Helpers

```go
import "github.com/michael-conway/go-irodsclient-extensions/irodsuri"

uri, err := irodsuri.Parse("irods://example.org:1247/tempZone/home/test1/file.txt")
_ = uri
```

```go
import "github.com/michael-conway/go-irodsclient-extensions/ignore"

rules, err := ignore.NewIgnores("/tempZone/home/test1", []string{"*.tmp", "cache/"})
ignored := rules.IsIgnored("/tempZone/home/test1/cache/file.tmp", false)
_ = ignored
```

```go
import "github.com/michael-conway/go-irodsclient-extensions/searchplugin"

registry := searchplugin.NewRegistry()
client := searchplugin.NewClient(registry)
_ = client
```

```go
import "github.com/michael-conway/go-irodsclient-extensions/cmdcues"

cues, err := cmdcues.BuildDataObjectCues("/tempZone/home/test1/file.txt")
_ = cues
```

## Public API Stability (Alpha)

Current planned release: `v1.0.0-alpha`.

Stability levels used for alpha release planning:

- `alpha-stable`: intended for alpha consumers; breaking changes should be limited and called out in release notes
- `experimental`: useful code, but the API may change without compatibility guarantees

Intended consumer categories:

- `irods-go-rest`
- `irods-go-drs`
- `drscmd`
- `other Go apps`

| Package | Status | Intended consumers | Notes |
| --- | --- | --- | --- |
| `cmdcues` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Public supported package |
| `favorites` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `filecart` | experimental | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | API may change at any time; no direct replacement yet |
| `ignore` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `irodsauth` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `other Go apps` | Shared Basic, bearer, and ticket account construction |
| `irodsuri` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `metadata` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `metadata/irodsfs` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Adapter for `go-irodsclient/fs` metadata query execution |
| `oidcverify` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `other Go apps` | Shared OIDC introspection and userinfo verifier |
| `s3admin` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `s3admin/irodsfs` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Adapter for `go-irodsclient/fs` S3 admin workflows |
| `searchplugin` | experimental | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | API may change at any time |
| `tickets` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `userpersist` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `usersync` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `other Go apps` | Desired-state user and group reconciliation |
| `usersync/irodsfs` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `other Go apps` | Adapter for `go-irodsclient/fs` user and group sync |

## Development

```bash
go test ./...
```

Integration tests are build-tagged and require a live iRODS grid plus
`GOEXT_TEST_CONFIG_ENV`:

```bash
GOEXT_TEST_CONFIG_ENV=integration/extensions-integration.sample.yaml go test -tags=integration ./integration/...
```

## Design Rules

- Keep low-level protocol and filesystem responsibilities in `go-irodsclient`.
- Put cross-client workflows and reusable higher-level behavior here.
- Prefer small focused packages over one large omnibus helper package.
- Avoid application-specific policy unless it is clearly reusable across
  consumers.
