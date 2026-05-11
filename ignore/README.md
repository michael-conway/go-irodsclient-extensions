# ignore package

`ignore` filters iRODS `[]*fs.Entry` lists using gitignore-style rules.

The package supports:

- preprocessing ignore rules into an `Ignores` structure
- loading rules from a local filesystem file
- loading rules from an iRODS data object
- filtering `List` output with `FilterEntries`

## Quick usage

```go
package main

import (
	"fmt"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	ignoreext "github.com/michael-conway/go-irodsclient-extensions/ignore"
)

func filterLocal(entries []*irodsfs.Entry) ([]*irodsfs.Entry, error) {
	ignores, err := ignoreext.ReadIgnoreFileFromLocal("./sample.ignore", "/tempZone/home/test1")
	if err != nil {
		return nil, err
	}

	filtered := ignoreext.FilterEntries(ignores, entries)
	return filtered, nil
}

func filterIRODS(filesystem ignoreext.IRODSIgnoreFilesystem, entries []*irodsfs.Entry) ([]*irodsfs.Entry, error) {
	ignores, err := ignoreext.ReadIgnoreFileFromIRODS(filesystem, "/tempZone/home/test1/.ignore", "/tempZone/home/test1")
	if err != nil {
		return nil, err
	}

	filtered := ignoreext.FilterEntries(ignores, entries)
	return filtered, nil
}

func main() {
	fmt.Println("see package tests for full examples")
}
```

## Core API

- `NewIgnores(basePath, lines)`
- `ParseIgnoreFileContents(basePath, contents)`
- `ReadIgnoreFileFromLocal(ignoreFilePath, basePath)`
- `ReadIgnoreFileFromIRODS(filesystem, ignoreIRODSPath, basePath)`
- `ReadIgnoreFile(ignoreFilePath, basePath)` (local convenience)
- `FilterEntries(ignores, entries)`
- `(*Ignores).IsIgnored(irodsPath, isDir)`

## Rule semantics

Rules follow gitignore-style matching against iRODS absolute paths relative to
`basePath`.

- blank lines are ignored
- lines starting with `#` are comments
- `\#` matches a literal leading `#`
- trailing spaces are ignored unless escaped
- leading `!` negates a previous ignore
- `\!` matches a literal leading `!`
- `/` is the path separator
- patterns with `/` are path-aware (anchored to `basePath` context)
- patterns without `/` match basename at any depth
- trailing `/` means directory-only match
- `*` matches any non-`/` sequence
- `?` matches one non-`/` character
- `[a-z]` style character classes are supported
- `**` supports recursive directory matching:
  - `**/foo`
  - `abc/**`
  - `a/**/b`
- last matching rule wins
- negation cannot re-include a child path if a parent directory is still ignored

## Notes

- Matching is applied to iRODS logical paths, not local filesystem paths.
- `basePath` must be an absolute iRODS path.
- Entries outside `basePath` are not ignored by this matcher.

## Error Taxonomy

Sentinel errors intended for `errors.Is` checks:

- `ErrInvalidBasePath`
- `ErrInvalidIgnorePath`
- `ErrMissingFilesystem`
- `ErrIgnorePathIsDirectory`

The iRODS loader also preserves standard library sentinels where applicable:

- `os.ErrInvalid` (wrapped with `ErrInvalidIgnorePath`)
- `os.ErrNotExist` (missing ignore file)
- `io.EOF` handling for reads
