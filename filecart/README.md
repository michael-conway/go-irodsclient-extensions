# filecart

`filecart` manages user-defined carts of iRODS paths. The package uses
`userpersist.FileService` for the user-home file structure and stores cart
content as AVUs on cart data objects.

## Directory Layout

For user home:

```text
/tempZone/home/test1
```

file carts are stored as:

```text
/tempZone/home/test1/.irodsext/
  filecarts/
    <cart-id>.cart
```

Cart IDs are generated with `xid`. The `.cart` data object is intentionally
empty; its metadata carries the cart name and entries.

## AVU Conventions

Each cart object uses these AVUs:

```text
Name:  iRODS:FileCart:Name
Value: <assigned cart name>
Units: iRODS:FileCart
```

Each cart entry is stored as:

```text
Name:  iRODS:FileCart:Entry
Value: <absolute iRODS path>
Units: file | collection
```

Adding an item is idempotent when the same path and entry type are already
present. Renaming a cart replaces the existing cart-name AVU with a single
canonical value.

## Usage

```go
service, err := filecart.NewService(filesystem, account.GetHomeDirPath())
if err != nil {
    return err
}

cart, err := service.CreateCart("Run inputs")
if err != nil {
    return err
}

err = service.AddItem(cart.ID, "/tempZone/home/test1/data.txt", filecart.EntryTypeFile)
```

## Error Taxonomy

Sentinel errors intended for `errors.Is` checks:

- `ErrMissingFilesystem`
- `ErrInvalidUserHome`
- `ErrInvalidCartName`
- `ErrInvalidCartRef`
- `ErrInvalidEntryPath`
- `ErrInvalidEntryType`
- `ErrInvalidMetadataPath`

Operational/storage errors are returned with context using `%w`, so callers can
match underlying filesystem errors without parsing strings.
