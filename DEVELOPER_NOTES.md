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
- `metadata/` for manifests, entry AVU queries, query definitions, and saved queries
- `userpersist/`, `favorites/`, and `filecart/` for user-scoped persisted state
- `s3admin/` for iRODS S3 API bucket and user secret mapping workflows
- `usersync/` for desired-state user and group reconciliation
- `usersandgroups/` for list-scale user/group queries and shared igroupadmin-compatible user/group workflows
- `oidcverify/` and `irodsauth/` for shared auth plumbing used by service layers

Do not turn this repository into a single generic helpers package.

## Code boundaries

Keep the split this way:

- `go-irodsclient` owns low-level transport, connection, and base filesystem APIs
- `extensions` owns shared higher-level client workflows
- client repositories such as `irods-go-rest` and `irods-go-drs` own HTTP mapping, route behavior, and service-specific policy

If a feature needs new filesystem primitives, add them to `go-irodsclient` first or record the gap clearly.

Current recorded `go-irodsclient` gap:

- `fs.CreateUser` uses the general-admin user create path, which is appropriate
  for rodsadmin administration but does not model `igroupadmin mkuser Name
  Password [Zone]`. Until go-irodsclient exposes that primitive directly,
  `usersandgroups/irodsfs` owns a narrow `USER_ADMIN_AN` implementation for
  groupadmin-compatible `rodsuser` creation with an initial password.

## Testing

Use two layers:

- unit tests with `go test ./...`
- direct iRODS integration tests with `go test -tags=integration ./...`

Keep integration support reusable across packages under `internal/testutil/`.

Preferred live-test substrate:

- `irods-grid-stack`

Use `irods-grid-stack` as the default local iRODS grid for extension
integration testing and for cross-repo REST/DRS validation. The extensions
tests should only require backend iRODS services, so the backend-only stack is
normally sufficient:

```bash
cd ../irods-grid-stack
docker compose up -d --build
```

When testing extension behavior through consumer services such as
`irods-go-rest`, `irods-go-drs`, or Starbase, use the full frontend profile from
`irods-grid-stack`:

```bash
cd ../irods-grid-stack
docker compose --profile frontend up -d --build
```

The legacy `irods-go-drs/deployments/docker-test-framework/` compose framework
is deprecated for new extension work. Do not add new integration-test or sample
configuration dependencies on that DRS-local stack. It may remain in
`irods-go-drs` for historical reproduction while active development converges
on `irods-grid-stack`.

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
IrodsNegotiationPolicy: CS_NEG_DONT_CARE
IrodsDefaultResource: providerResc
IrodsPrimaryTestUser: test1
IrodsPrimaryTestPassword: test
IrodsSecondaryTestUser: test2
IrodsSecondaryTestPassword: test
```

The checked-in sample lives at:

- `integration/extensions-integration.sample.yaml`

That sample should stay aligned with the default host-facing
`irods-grid-stack` backend ports, users, zone, and resource names. If
`irods-grid-stack` defaults change, update this sample and the corresponding
consumer samples in `irods-go-rest/e2e/` and `irods-go-drs/e2e/` together.

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
- Before tagging an alpha release, update the README stability table, run unit
  tests, and record any experimental APIs in release notes.

## Release Process

Use [RELEASE.md](RELEASE.md) as the release checklist. For `v1.0.0-alpha`, the
minimum release gate is:

- README stability table is current
- `CHANGELOG.md` has a `v1.0.0-alpha` entry
- `go test ./...` passes
- `git diff --check` passes

Run `go test -tags=integration ./integration/...` when an iRODS grid is
available through `GOEXT_TEST_CONFIG_ENV`.

## User Sync Policy

`usersync` owns reusable iRODS user/group convergence behavior for REST,
Keycloak integration, DRS certification support, and other clients that need
repeatable sync workflows.

Keep the authorization boundary clear:

- callers authenticate and authorize before constructing the filesystem
- `usersync` assumes the filesystem has the iRODS catalog authority needed to
  perform the requested operation
- `usersync` still enforces sync guardrails that are independent of caller
  authority

Default sync guardrails:

- sync may create and manage only `rodsuser` users
- `groupadmin` and `rodsadmin` user changes are outside sync and remain the
  province of iRODS admin workflows such as iCommands
- groups may be created and managed as `rodsgroup`
- group membership sync may add/remove normal `rodsuser` accounts only
- created users/groups are marked with `iRODS:USER_SYNCH:MANAGED=true`
- existing users/groups are not claimed unless the caller explicitly enables
  claim behavior
- delete and membership-removal operations require managed AVUs unless the
  caller explicitly relaxes the policy

Use the AVU attribute family `iRODS:USER_SYNCH:<field>` for durable sync state.
Current fields include:

- `iRODS:USER_SYNCH:MANAGED`
- `iRODS:USER_SYNCH:SOURCE`
- `iRODS:USER_SYNCH:REALM`
- `iRODS:USER_SYNCH:EXTERNAL_ID`
- `iRODS:USER_SYNCH:LAST_SYNC_AT`
- `iRODS:USER_SYNCH:LAST_PLAN_ID`

Do not introduce a REST-local or tool-local state store for user sync ownership
unless iRODS-native AVUs are demonstrably insufficient.

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
