# Integration Tests

This directory is reserved for docker-framework-backed integration tests for
`go-irodsclient-extensions`.

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

Sample shape:

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
