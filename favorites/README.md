# favorites

`favorites` manages a user's named favorite iRODS paths. The package uses
`userpersist.FileService` to create the user-scoped backing file and stores the
favorite records as AVUs on that file.

## Directory Layout

For user home:

```text
/tempZone/home/test1
```

favorites are stored as:

```text
/tempZone/home/test1/.irodsext/
  favorites/
    favorites
```

The `favorites` data object is intentionally empty; favorite records are stored
as metadata on that object.

## AVU Conventions

Each favorite uses:

```text
Name:  iRODS:Favorite
Value: {"name":"<display name>","absolute_path":"<absolute iRODS path>"}
Units: iRODS:Favorite
```

The path is the stable key. Adding a favorite for a path that already exists
replaces stale entries for that path with one canonical JSON value. If a stored
favorite has an empty name, listing falls back to the basename of the path.

## Usage

```go
service, err := favorites.NewService(filesystem, account.GetHomeDirPath())
if err != nil {
    return err
}

err = service.AddFavorite("Analysis inputs", "/tempZone/home/test1/inputs")
items, err := service.ListFavorites()
```
