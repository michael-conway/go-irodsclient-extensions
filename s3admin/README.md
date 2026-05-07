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

Each secret key data object is marked with:

```text
Name:  iRODS:S3:Secret
Value: <iRODS user id>
Units:
```

The marker AVU is the source used for rebuilding the iRODS S3 API user mapping
file.

## Secret Key Format

S3 user secret keys must be exactly 40 characters. Supported characters are:

```text
A-Z a-z 0-9 / + . _ - ~
```

`GenerateS3UserSecretKey` uses crypto-random selection from that character set.
`StoreS3UserKey` validates caller-provided values and returns
`ErrInvalidUserSecretKey` for bad keys.

## User Mapping File

`S3UserMappingService` manages the local-file user mapping JSON consumed by the
iRODS S3 API user mapping plugin. The file shape is:

```json
{
  "test1": {
    "secret_key": "Aa1Bb~2Cc3.-Dd4Ee5Ff6Gg7Hh8Ii9_Jj0Kk1Ll2",
    "username": "test1"
  }
}
```

The map key is the S3 access key ID and is kept equal to the iRODS username.
The service updates this file after adding, updating, generating, or deleting a
user secret key. Writes are guarded and use an atomic temporary-file replacement
like the bucket mapping file.

`RebuildUserMappingFromAVUs` searches for `iRODS:S3:Secret` AVUs, reads each
marked secret key file, validates the stored key, and rewrites the user mapping
JSON from the discovered records. This operation should be run with an iRODS
account that can see all managed users' secret key files.

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

```go
users, err := s3admin.NewS3UserMappingService(filesystem, "/shared/irods-s3-user-mapping.json")
if err != nil {
    return err
}

stored, err := users.StoreUserSecretKey(account, secretKey)
generated, err := users.GenerateAndStoreUserSecretKey(account)
result, err := users.RebuildUserMappingFromAVUs()
```
