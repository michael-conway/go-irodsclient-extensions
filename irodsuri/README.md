# irodsuri

`irodsuri` provides helpers for parsing and building `irods://` URIs and for
converting between URI values and `go-irodsclient` account structures.

## Usage

```go
uri, err := irodsuri.BuildForAccountWithoutUserInfo(
	account,
	"/tempZone/home/test1/object.txt",
)
if err != nil {
	return err
}

parsed, err := irodsuri.Parse(uri.String())
if err != nil {
	return err
}

_ = parsed
```

## Error Semantics

This package currently returns contextual `error` values and does not define
sentinel errors.

- Validation and shape errors use clear messages (for example: empty host/path,
  invalid scheme, missing account, non-absolute path).
- Parse/build failures wrap upstream parse/type creation errors using `%w` where
  available.
- Callers should treat these as parameter/format errors and handle by checking
  inputs before retry.

## Integration Notes

- This package is pure URI/account transformation logic and does not create
  network connections or perform iRODS operations.
- Ticket-based URI support uses the explicit query parameter
  `ticket=<ticket_id>`.
- DRS/REST consumers should prefer `BuildForAccountWithoutUserInfo` when user
  credentials must not be serialized into externally visible URIs.
