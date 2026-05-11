# go-irodsclient-extensions

`go-irodsclient-extensions` is a shared Go library for higher-level iRODS client functionality built on top of [`go-irodsclient`](https://github.com/cyverse/go-irodsclient).

The intent of this repository is to hold reusable packages that sit above the base client and can be shared across rich iRODS applications, service layers, browser-facing APIs, and administrative tooling.

## Scope

This repository is for extensions such as:

- metadata and AVU helpers
- ticket lifecycle helpers
- higher-level search and discovery helpers
- path and naming utilities
- transfer orchestration helpers
- shared client-side workflows that are too opinionated for the base client, but broadly useful across consumers

This repository is not intended to duplicate core transport, connection, or low-level filesystem behavior already provided by `go-irodsclient`.

## Current layout

- `irodsuri/`: iRODS URI parsing and related helpers
- `searchplugin/`: shared client and registry support for OpenAPI-based search plugins
- `userpersist/`: conventions and helpers for `~/.irodsext` user persistence collections
- `filecart/`: AVU-backed file cart lifecycle and entry management under `~/.irodsext/filecarts`
- `favorites/`: AVU-backed favorite shortcuts under `~/.irodsext/favorites`
- `s3admin/`: AVU-backed iRODS S3 API bucket administration and local-file mapping synchronization
- `tickets/`: ticket creation, validation, and lifecycle helpers
- `integration/`: docker-test-framework integration tests (build-tagged)
- `internal/testutil/`: shared test helpers for this repository
- `testdata/`: fixtures used by tests

## Public API Stability (Alpha)

Stability levels used in this repository:

- `alpha-stable`: package is intended for alpha consumption; minor breaking changes may occur with release-note notice
- `experimental`: package API may change at any time

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
| `irodsuri` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `metadata` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `s3admin` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `searchplugin` | experimental | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | API may change at any time |
| `tickets` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |
| `userpersist` | alpha-stable | `irods-go-rest`, `irods-go-drs`, `drscmd`, `other Go apps` | Active |

## Development

```bash
go test ./...
```

## Design rules

- Keep low-level protocol and filesystem responsibilities in `go-irodsclient`.
- Put cross-client workflows and reusable higher-level behavior here.
- Prefer small focused packages over one large omnibus helper package.
- Avoid dragging application-specific policy into shared packages unless it is clearly reusable.
