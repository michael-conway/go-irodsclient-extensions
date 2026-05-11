# tickets

`tickets` provides shared helpers for iRODS ticket-oriented access patterns,
including ticket-form bearer token parsing/formatting and anonymous read-ticket
creation helpers.

## Usage

```go
ticketID, bearer, err := tickets.CreateAnonymousDataObjectBearerToken(
    filesystem,
    "/tempZone/home/test1/object.txt",
    50,
    720,
)
if err != nil {
    return err
}

_ = ticketID
_ = bearer
```

## Error Semantics

Sentinel validation errors intended for `errors.Is` checks:

- `ErrMissingFilesystem`
- `ErrInvalidIRODSPath`
- `ErrInvalidMaximumUses`
- `ErrInvalidTicketExpiry`

Operational iRODS ticket errors are returned with context using `%w`, so callers
can match underlying ticket API failures without parsing strings.

## Security Notes

- Bearer token formatting/parsing is explicit and does not emit logs.
- Default helper error messages avoid embedding generated ticket IDs/tokens.

## Integration Notes

- `CreateAnonymousDataObjectBearerToken` expects a filesystem implementation
  satisfying `AnonymousTicketFilesystem`; callers typically provide an adapter
  backed by `go-irodsclient/fs`.
- Generated bearer token values are formatted as `irods-ticket:<ticket_id>` and
  can be parsed with `ParseBearerToken`.
- Integration coverage exists in `tickets/bearer_integration_test.go` and
  `integration/tickets_integration_test.go` for live ticket lifecycle sanity.
