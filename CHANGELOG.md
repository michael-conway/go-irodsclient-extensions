# Changelog

This project uses Go module tags for releases. The module path remains
`github.com/michael-conway/go-irodsclient-extensions` for `v1` releases; no
`/v1` import suffix is used.

## v1.0.0-alpha - 2026-06-01

First alpha release for shared higher-level iRODS client extensions.

### Alpha Contract

- The packages marked `alpha-stable` in `README.md` are intended for early
  consumer adoption.
- Breaking changes may still happen before a stable `v1.0.0`, but they should
  be deliberate and documented in this changelog.
- Packages marked `experimental` are available for evaluation, but their APIs
  can change without compatibility guarantees.

### Alpha-Stable Packages

- `cmdcues`: command cue builders for CLIs and UI help surfaces.
- `favorites`: user-scoped favorite path lifecycle.
- `ignore`: gitignore-style path matching for iRODS path trees.
- `irodsauth`: shared auth request to `IRODSAccount` construction.
- `irodsuri`: iRODS URI parsing and formatting.
- `metadata`: metadata manifests, entry AVU queries, query definitions, and
  saved query persistence.
- `metadata/irodsfs`: `go-irodsclient/fs` adapter for metadata workflows.
- `oidcverify`: OIDC token introspection and userinfo verification.
- `s3admin`: AVU-backed iRODS S3 API bucket and user secret mapping workflows.
- `s3admin/irodsfs`: `go-irodsclient/fs` adapter for S3 admin workflows.
- `tickets`: ticket bearer-token helpers and anonymous ticket creation.
- `userpersist`: shared `~/.irodsext` persistence conventions.
- `usersync`: desired-state user, group, and membership reconciliation.
- `usersync/irodsfs`: `go-irodsclient/fs` adapter for user sync workflows.

### Experimental Packages

- `filecart`: user-scoped file cart lifecycle.
- `searchplugin`: OpenAPI-based search plugin registry and client support.

### Notes For Consumers

- Unit tests should pass with `go test ./...`.
- Live iRODS tests are behind the `integration` build tag and require
  `GOEXT_TEST_CONFIG_ENV`.
- Consumer services should pin this release with:

  ```bash
  go get github.com/michael-conway/go-irodsclient-extensions@v1.0.0-alpha
  ```

- Use `go.work` for local multi-repo development, but keep committed
  `go.mod` files pinned to real module tags or commits.
