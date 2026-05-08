package ignore

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
)

func TestFilterEntriesSimplePattern(t *testing.T) {
	ignores, err := NewIgnores("/tempZone/home/test1", []string{
		"*.tmp",
	})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	entries := []*irodsfs.Entry{
		fileEntry("/tempZone/home/test1/a.tmp"),
		fileEntry("/tempZone/home/test1/sub/b.txt"),
		fileEntry("/tempZone/home/test1/sub/c.tmp"),
	}

	filtered := FilterEntries(ignores, entries)
	if len(filtered) != 1 || filtered[0].Path != "/tempZone/home/test1/sub/b.txt" {
		t.Fatalf("unexpected filtered entries %+v", pathsOf(filtered))
	}
}

func TestFilterEntriesNegation(t *testing.T) {
	ignores, err := NewIgnores("/tempZone/home/test1", []string{
		"*.tmp",
		"!keep.tmp",
	})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	entries := []*irodsfs.Entry{
		fileEntry("/tempZone/home/test1/drop.tmp"),
		fileEntry("/tempZone/home/test1/keep.tmp"),
	}

	filtered := FilterEntries(ignores, entries)
	if len(filtered) != 1 || filtered[0].Path != "/tempZone/home/test1/keep.tmp" {
		t.Fatalf("unexpected filtered entries %+v", pathsOf(filtered))
	}
}

func TestFilterEntriesDirectoryOnly(t *testing.T) {
	ignores, err := NewIgnores("/tempZone/home/test1", []string{
		"build/",
	})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	entries := []*irodsfs.Entry{
		dirEntry("/tempZone/home/test1/build"),
		fileEntry("/tempZone/home/test1/build/out.txt"),
		fileEntry("/tempZone/home/test1/build.txt"),
	}

	filtered := FilterEntries(ignores, entries)
	if len(filtered) != 1 || filtered[0].Path != "/tempZone/home/test1/build.txt" {
		t.Fatalf("unexpected filtered entries %+v", pathsOf(filtered))
	}
}

func TestFilterEntriesAnchoredPattern(t *testing.T) {
	ignores, err := NewIgnores("/tempZone/home/test1", []string{
		"/hello.*",
	})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	entries := []*irodsfs.Entry{
		fileEntry("/tempZone/home/test1/hello.txt"),
		fileEntry("/tempZone/home/test1/sub/hello.txt"),
	}

	filtered := FilterEntries(ignores, entries)
	if len(filtered) != 1 || filtered[0].Path != "/tempZone/home/test1/sub/hello.txt" {
		t.Fatalf("unexpected filtered entries %+v", pathsOf(filtered))
	}
}

func TestFilterEntriesDoubleStar(t *testing.T) {
	ignores, err := NewIgnores("/tempZone/home/test1", []string{
		"a/**/b.txt",
	})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	entries := []*irodsfs.Entry{
		fileEntry("/tempZone/home/test1/a/b.txt"),
		fileEntry("/tempZone/home/test1/a/x/b.txt"),
		fileEntry("/tempZone/home/test1/a/x/y/z.txt"),
	}

	filtered := FilterEntries(ignores, entries)
	if len(filtered) != 1 || filtered[0].Path != "/tempZone/home/test1/a/x/y/z.txt" {
		t.Fatalf("unexpected filtered entries %+v", pathsOf(filtered))
	}
}

func TestNegationCannotReincludeWhenParentDirectoryIgnored(t *testing.T) {
	ignores, err := NewIgnores("/", []string{
		"/foo/",
		"!/foo/bar.txt",
	})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	if !ignores.IsIgnored("/foo/bar.txt", false) {
		t.Fatal("expected /foo/bar.txt to remain ignored when parent /foo is ignored")
	}
}

func TestReadIgnoreFile(t *testing.T) {
	tempDir := t.TempDir()
	ignorePath := filepath.Join(tempDir, ".ignore")
	if err := os.WriteFile(ignorePath, []byte("# comment\n*.tmp\n\\#literal\n"), 0o600); err != nil {
		t.Fatalf("write ignore file: %v", err)
	}

	ignores, err := ReadIgnoreFile(ignorePath, "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("read ignore file: %v", err)
	}

	entries := ignores.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 preprocessed entries, got %d", len(entries))
	}
	if entries[1].Pattern != "#literal" {
		t.Fatalf("expected escaped # rule to be preserved, got %+v", entries[1])
	}
}

func TestReadIgnoreFileFromIRODS(t *testing.T) {
	ignoreContents := []byte("*.tmp\n!/keep.tmp\n")
	filesystem := &testIRODSIgnoreFilesystem{
		entries: map[string]*irodsfs.Entry{
			"/tempZone/home/test1/.ignore": {
				Type: irodsfs.FileEntry,
				Path: "/tempZone/home/test1/.ignore",
				Name: ".ignore",
				Size: int64(len(ignoreContents)),
			},
		},
		content: map[string][]byte{
			"/tempZone/home/test1/.ignore": ignoreContents,
		},
	}

	ignores, err := ReadIgnoreFileFromIRODS(filesystem, "/tempZone/home/test1/.ignore", "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("read ignore file from irods: %v", err)
	}

	if ignores.IsIgnored("/tempZone/home/test1/drop.tmp", false) != true {
		t.Fatal("expected drop.tmp to be ignored")
	}
	if ignores.IsIgnored("/tempZone/home/test1/keep.tmp", false) != false {
		t.Fatal("expected keep.tmp to be re-included")
	}
}

func TestReadIgnoreFileFromIRODSRejectsDirectory(t *testing.T) {
	filesystem := &testIRODSIgnoreFilesystem{
		entries: map[string]*irodsfs.Entry{
			"/tempZone/home/test1/.ignore": dirEntry("/tempZone/home/test1/.ignore"),
		},
		content: map[string][]byte{},
	}

	if _, err := ReadIgnoreFileFromIRODS(filesystem, "/tempZone/home/test1/.ignore", "/tempZone/home/test1"); err == nil {
		t.Fatal("expected directory error")
	}
}

func TestFilterEntriesSkipsNilAndOutsideBase(t *testing.T) {
	ignores, err := NewIgnores("/tempZone/home/test1", []string{"*.tmp"})
	if err != nil {
		t.Fatalf("new ignores: %v", err)
	}

	entries := []*irodsfs.Entry{
		nil,
		fileEntry("/otherZone/home/test1/drop.tmp"),
		fileEntry("/tempZone/home/test1/keep.txt"),
	}

	filtered := FilterEntries(ignores, entries)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 remaining entries, got %+v", pathsOf(filtered))
	}
	if filtered[0].Path != "/otherZone/home/test1/drop.tmp" || filtered[1].Path != "/tempZone/home/test1/keep.txt" {
		t.Fatalf("unexpected remaining entries %+v", pathsOf(filtered))
	}
}

func TestNewIgnoresRejectsInvalidBasePath(t *testing.T) {
	if _, err := NewIgnores("tempZone/home/test1", []string{"*.tmp"}); err == nil {
		t.Fatal("expected error for non-absolute base path")
	}
}

type testIRODSIgnoreFilesystem struct {
	entries map[string]*irodsfs.Entry
	content map[string][]byte
	openErr error
	statErr error
}

func (fs *testIRODSIgnoreFilesystem) Stat(irodsPath string) (*irodsfs.Entry, error) {
	if fs.statErr != nil {
		return nil, fs.statErr
	}
	entry, ok := fs.entries[irodsPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return entry, nil
}

func (fs *testIRODSIgnoreFilesystem) OpenFile(irodsPath string, resource string, mode string) (IRODSFileHandleReader, error) {
	if fs.openErr != nil {
		return nil, fs.openErr
	}
	content, ok := fs.content[irodsPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &testIRODSIgnoreFileHandle{
		content: append([]byte(nil), content...),
	}, nil
}

type testIRODSIgnoreFileHandle struct {
	content []byte
	closed  bool
}

func (handle *testIRODSIgnoreFileHandle) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset >= int64(len(handle.content)) {
		return 0, io.EOF
	}

	n := copy(buffer, handle.content[offset:])
	if int(offset)+n >= len(handle.content) {
		return n, io.EOF
	}
	return n, nil
}

func (handle *testIRODSIgnoreFileHandle) Close() error {
	handle.closed = true
	return nil
}

func fileEntry(path string) *irodsfs.Entry {
	return &irodsfs.Entry{
		Type: irodsfs.FileEntry,
		Path: path,
		Name: filepath.Base(path),
	}
}

func dirEntry(path string) *irodsfs.Entry {
	return &irodsfs.Entry{
		Type: irodsfs.DirectoryEntry,
		Path: path,
		Name: filepath.Base(path),
	}
}

func pathsOf(entries []*irodsfs.Entry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		result = append(result, entry.Path)
	}
	return result
}
