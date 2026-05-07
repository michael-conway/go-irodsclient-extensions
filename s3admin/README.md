# s3admin

`s3admin` contains tools for managing iRODS S3 API administration state.
Bucket mappings are represented by collection AVUs and mirrored into the S3 API
bucket mapping JSON file. User S3 secret keys are stored under the user's
`userpersist` area.

## Bucket Mapping Conventions

A collection is marked as an S3 bucket root with this AVU:

```text
Name:  iRODS:S3:Bucket
Value: <bucket name>
Units:
```

`S3Service` scans for these AVUs using metadata search, validates duplicate
bucket names, and updates the shared bucket mapping JSON used by the iRODS S3
API local-file bucket mapping plugin:

```json
{
  "bucket-name": "/tempZone/home/test1/data"
}
```

Bucket names are unique within the configured scan root. Updating a bucket
renames the AVU value and refreshes the mapping file.

## User Key Directory Layout

For user home:

```text
/tempZone/home/test1
```

`S3UserKeyService` stores the user's S3 API secret key at:

```text
/tempZone/home/test1/.irodsext/
  s3admin/
    irods-s3-api-secret.txt
```

The file contains the user's secret key as plain text. The service creates
`~/.irodsext/s3admin` idempotently when storing or generating a key.

## Secret Key Format

S3 user secret keys must be exactly 40 characters. Supported characters are:

```text
A-Z a-z 0-9 / + . _ - ~
```

`GenerateS3UserSecretKey` uses crypto-random selection from that character set.
`StoreS3UserKey` validates caller-provided values and returns
`ErrInvalidUserSecretKey` for bad keys.

## Usage

```go
keys, err := s3admin.NewS3UserKeyService(filesystem)
if err != nil {
    return err
}

stored, err := keys.GenerateAndStoreS3UserKey(account)
existing, err := keys.GetS3UserKey(account)
err = keys.DeleteS3UserKey(account)
```
