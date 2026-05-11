# searchplugin

`searchplugin` provides registry and HTTP client helpers for federating search
requests across multiple OpenAPI-based plugin endpoints.

## Usage

```go
registry := searchplugin.NewRegistry()
if err := registry.Register(searchplugin.Endpoint{
	ID:       "plugin-a",
	Name:     "plugin-a",
	BaseURL:  "https://example.org/search",
	AuthType: searchplugin.AuthTypeNone,
	Enabled:  true,
}); err != nil {
	return err
}

client := searchplugin.NewClient(registry)
response, err := client.List(context.Background(), "plugin-a", searchplugin.RequestAuthContext{})
if err != nil {
	return err
}

_ = response
```

## Error Semantics

Sentinel errors intended for `errors.Is` checks:

- Registry/config: `ErrEndpointIDRequired`, `ErrEndpointExists`,
  `ErrEndpointNotFound`, `ErrBaseURLRequired`, `ErrInvalidBaseURL`,
  `ErrInvalidAuthType`, `ErrConfigPathNotSet`, `ErrEndpointDisabled`
- Request auth: `ErrBearerTokenRequired`, `ErrBasicAuthRequired`,
  `ErrUnsupportedAuthType`
- Request validation: `ErrIndexNameRequired`, `ErrSearchQueryRequired`

HTTP response errors are returned as `HTTPStatusError` for status codes `>= 400`
while still returning the captured response payload to callers.

## Integration Notes

- `Client` depends on a populated `Registry` and defaults to an internal
  `http.Client` with a 30-second timeout.
- The default authorizer supports endpoint auth types `none`,
  `bearer_passthrough`, and `basic_passthrough`; static/service-account auth
  types require a custom `RequestAuthorizer`.
- Health/status is recorded in-memory in `Registry` on each request; reloads and
  process restarts reset status state.
- Config-file loading expects plugin definitions compatible with
  `plugins.sample.yaml`.
