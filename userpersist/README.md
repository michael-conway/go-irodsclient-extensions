# userpersist

`userpersist` provides shared helpers for extension data stored under a user's
iRODS home collection. It is the file persistence layer used by packages such
as `favorites`, `filecart`, and `s3admin`.

## Directory Layout

Given a user home path:

```text
/tempZone/home/test1
```

`userpersist` stores extension state under:

```text
/tempZone/home/test1/.irodsext/
  <context>/
    <file>
```

The context is a single path segment such as `favorites`, `filecarts`, or
`s3admin`. File names are also single path segments. Nested context or file
names are rejected so callers cannot escape the managed structure.

## File Service

`FileService` handles the generic lifecycle for files under a string context:

- `EnsureContext` creates `~/.irodsext/<context>` idempotently.
- `EnsureFileStructure` creates the context and returns the target file path.
- `AddOrUpdateFile` and `AddOrUpdateString` create the context if needed and
  write the file contents.
- `GetFile` and `GetString` read the contents back.
- `DeleteFile` removes the named file.

Packages using `FileService` provide a filesystem adapter implementing
`FileFilesystem`. Package-specific behavior, such as AVU schemas or key
validation, should stay in the calling package.

## Example

```go
files, err := userpersist.NewFileService(filesystem)
if err != nil {
    return err
}

file, err := files.AddOrUpdateString(
    "/tempZone/home/test1",
    "s3admin",
    "irods-s3-api-secret.txt",
    secretKey,
)
```
