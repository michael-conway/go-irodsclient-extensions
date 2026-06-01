# Release Process

This repository is released as a Go module. Tags must be valid semantic
versions, for example `v1.0.0-alpha`.

The module path is:

```text
github.com/michael-conway/go-irodsclient-extensions
```

Because this is a `v1` module, import paths do not include a major-version
suffix.

## v1.0.0-alpha Checklist

Before tagging:

1. Confirm the worktree is clean except for intentional release changes.
2. Confirm `README.md` has the current package stability table.
3. Confirm `CHANGELOG.md` has a `v1.0.0-alpha` entry.
4. Run formatting for any edited Go files:

   ```bash
   gofmt -w <files>
   ```

5. Run the unit suite:

   ```bash
   go test ./...
   ```

6. When an iRODS test grid is available, run integration tests:

   ```bash
   export GOEXT_TEST_CONFIG_ENV=integration/extensions-integration.sample.yaml
   go test -tags=integration ./integration/...
   ```

7. Review public API changes:

   ```bash
   git diff --stat
   git diff --check
   ```

Tag and publish:

```bash
git tag -a v1.0.0-alpha -m "v1.0.0-alpha"
git push origin v1.0.0-alpha
```

After publishing, verify module resolution from a consumer checkout:

```bash
go get github.com/michael-conway/go-irodsclient-extensions@v1.0.0-alpha
go mod tidy
go test ./...
```

## Release Notes Template

Use this shape for GitHub release notes:

```markdown
## go-irodsclient-extensions v1.0.0-alpha

First alpha release for shared higher-level iRODS client extensions.

### Highlights

- Metadata manifests, AVU entry queries, query definitions, and saved queries.
- User-scoped persistence helpers for favorites and extension state.
- iRODS S3 API administration helpers.
- OIDC verification and shared iRODS account construction.
- Desired-state user and group sync helpers.

### Experimental

- `filecart`
- `searchplugin`

### Validation

- `go test ./...`
- Optional: `GOEXT_TEST_CONFIG_ENV=integration/extensions-integration.sample.yaml go test -tags=integration ./integration/...`
```
