# metadata

`metadata` generates versioned JSON metadata manifests for iRODS collections
and data objects.

## Usage

```go
service, err := metadata.NewService(filesystem)
if err != nil {
    return err
}

manifest, err := service.GenerateManifest("/tempZone/home/test1/object.txt")
if err != nil {
    return err
}

_ = manifest
```

## Error Semantics

Sentinel errors intended for `errors.Is` checks:

- `ErrMissingFilesystem`
- `ErrInvalidIRODSPath`
- `ErrInvalidOutputPath`
- `ErrPathStatMissing` (filesystem returned no stat entry)

Operational/storage/rendering errors are returned with context using `%w`, so
callers can match underlying filesystem/IO errors without parsing strings.

## Integration Notes

- The service requires a filesystem implementation satisfying
  `MetadataManifestFilesystem`; `metadata/irodsfs` provides an adapter for
  `go-irodsclient/fs`.
- Manifest generation includes iRODS host/port/zone context and AVU payloads
  from the target path.
- Integration coverage exists in
  `integration/metadata_manifest_integration_test.go` for live iRODS collection
  and data-object manifest generation flows.
