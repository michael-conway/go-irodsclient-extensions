# Developer Notes

Use this file for the main working rules in `extensions`.

## Design philosophy

This repository holds reusable Go packages built on top of `go-irodsclient`.

The purpose is to centralize higher-level iRODS client functionality that is:

- too opinionated or workflow-oriented for `go-irodsclient`
- still generic enough to be shared across multiple consumers
- better maintained once in one place than duplicated across service repos

Example client repositories that should consume common functionality from here:

- `irods-go-rest`
- `irods-go-drs`

If logic is being copied between those repos, prefer moving it here unless it is tightly bound to one service's HTTP contract or application policy.

## Package model

Keep packages small and purpose-driven.

Use packages like:

- `searchplugin/` for OpenAPI-driven search plugin client and registry workflows
- `tickets/` for ticket parsing, ticket creation helpers, and ticket-related shared policy
- `irodsuri/` for iRODS URI handling helpers
- `transfer/` for higher-level transfer orchestration helpers

Do not turn this repository into a single generic helpers package.

## Code boundaries

Keep the split this way:

- `go-irodsclient` owns low-level transport, connection, and base filesystem APIs
- `extensions` owns shared higher-level client workflows
- client repositories such as `irods-go-rest` and `irods-go-drs` own HTTP mapping, route behavior, and service-specific policy

If a feature needs new filesystem primitives, add them to `go-irodsclient` first or record the gap clearly.

## Testing

Use two layers:

- unit tests with `go test ./...`
- direct iRODS integration tests with `go test -tags=integration ./...`

Keep integration support reusable across packages under `internal/testutil/`.

Current shared integration config env var:

- `GOEXT_TEST_CONFIG_ENV`

Use that env var as the main source of live-test configuration.

## Test configuration

Sample integration config file:

```yaml
IrodsHost: localhost
IrodsPort: 1247
IrodsZone: tempZone
IrodsAdminUser: rods
IrodsAdminPassword: rods
IrodsAuthScheme: native
IrodsNegotiationPolicy: request_server_negotiation
IrodsDefaultResource: demoResc
IrodsPrimaryTestUser: test1
IrodsPrimaryTestPassword: test
IrodsSecondaryTestUser: test2
IrodsSecondaryTestPassword: test
```

The checked-in sample lives at:

- `integration/extensions-integration.sample.yaml`

Typical shell setup:

```bash
export GOEXT_TEST_CONFIG_ENV=integration/extensions-integration.sample.yaml
```

Tests should default to the configured primary and secondary test users rather than hardcoding local account names.

## Working rules

- Put shared functionality here only when it is genuinely reusable.
- Avoid leaking REST or DRS route semantics into this repository.
- Keep public APIs small and explicit.
- Prefer deterministic helpers that are easy to test in isolation.
- When adding live-test support, add a unit-tested config path first.

## Local multi-repo sync (`go.work`)

Use a workspace `go.work` file for local cross-repo development instead of
`replace ../...` directives in `go.mod`.

Current workspace scaffold (at `workspace-gabble/go.work`) includes:

- `./go-irodsclient-extensions`
- `./irods-go-rest`
- `./irods-go-drs`

Workflow:

1. develop across repos with `go.work` active
2. keep each repo `go.mod` pinned to real module versions (no local replace)
3. when shared changes are ready, push/tag in `go-irodsclient-extensions`
4. bump dependent repos with `go get <module>@<tag-or-commit>` and `go mod tidy`
