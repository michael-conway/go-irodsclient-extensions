# oidcverify

`oidcverify` validates inbound OIDC bearer tokens against a Keycloak-compatible
provider using token introspection and userinfo endpoints.

The verifier is designed for backend services. Browser applications should use
their own public OIDC client and send bearer tokens to the service; the service
can use this package to validate those tokens.

## Usage

```go
verifier := oidcverify.NewVerifier(oidcverify.Config{
	BaseURL:      "https://keycloak:8443",
	Realm:        "drs",
	ClientID:     "irods-go-rest",
	ClientSecret: "change-me",
	InsecureSkipVerify: true,
})

token, err := verifier.VerifyToken(ctx, accessToken)
if err != nil {
	return err
}

_ = token.Username
_ = token.Introspection.ClientID
_ = token.Introspection.Audience
```

## Error Semantics

Sentinel errors intended for `errors.Is` checks:

- `ErrNotConfigured`
- `ErrUnauthorized`
- `ErrMissingUser`

Network, decode, and provider response errors are returned with context using
`%w` where appropriate.
