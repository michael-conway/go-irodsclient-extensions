# Integration Tests

This directory is reserved for live iRODS integration tests for
`go-irodsclient-extensions`.

The preferred local test substrate is `irods-grid-stack`. The legacy
`irods-go-drs/deployments/docker-test-framework/` compose stack is deprecated
for new extension integration work and should not be used for new fixtures,
sample configs, or test setup assumptions.

For normal extension integration tests, the backend-only grid stack is enough:

```bash
cd ../irods-grid-stack
docker compose up -d --build
```

Use the full frontend profile only when validating extensions through consumer
services such as `irods-go-rest` or `irods-go-drs`:

```bash
cd ../irods-grid-stack
docker compose --profile frontend up -d --build
```

## Build Tag

Integration tests in this directory should use the `integration` build tag:

```go
//go:build integration
// +build integration
```

Run explicitly:

```bash
go test -tags=integration ./integration/...
```

## Config

Set:

* `GOEXT_TEST_CONFIG_ENV`

Use the sample config:

* [integration/extensions-integration.sample.yaml](/Users/conwaymc/Documents/workspace-gabble/go-irodsclient-extensions/integration/extensions-integration.sample.yaml)

The sample is intended to match the host-facing defaults from
`irods-grid-stack`. Keep it aligned with `irods-go-rest/e2e/` and
`irods-go-drs/e2e/` samples when grid defaults change.

Sample shape:

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
