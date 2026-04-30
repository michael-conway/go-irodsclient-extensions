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
- `tickets/`: ticket creation, validation, and lifecycle helpers
- `transfer/`: higher-level transfer workflow helpers
- `internal/testutil/`: shared test helpers for this repository
- `testdata/`: fixtures used by tests

## Development

```bash
go test ./...
```

## Design rules

- Keep low-level protocol and filesystem responsibilities in `go-irodsclient`.
- Put cross-client workflows and reusable higher-level behavior here.
- Prefer small focused packages over one large omnibus helper package.
- Avoid dragging application-specific policy into shared packages unless it is clearly reusable.
