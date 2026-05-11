//go:build integration
// +build integration

package integration

import (
	"path"
	"strings"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	ignoreext "github.com/michael-conway/go-irodsclient-extensions/ignore"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/rs/xid"
)

func TestIgnoreFilteringAgainstIRODSAbsolutePathsIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixtureRoot := path.Join(homePath, ".goext-ignore-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureRoot, true, true)
	})

	buildPath := path.Join(fixtureRoot, "build")
	if err := filesystem.MakeDir(buildPath, true); err != nil {
		t.Fatalf("create fixture build collection %q: %v", buildPath, err)
	}

	dropTmpPath := path.Join(fixtureRoot, "drop.tmp")
	keepTmpPath := path.Join(fixtureRoot, "keep.tmp")
	keepTxtPath := path.Join(fixtureRoot, "keep.txt")
	buildOutPath := path.Join(buildPath, "out.txt")
	ignorePath := path.Join(fixtureRoot, ".ignore")

	for _, filePath := range []string{dropTmpPath, keepTmpPath, keepTxtPath, buildOutPath} {
		if err := createFileWithContents(filesystem, filePath, "ignore integration fixture\n"); err != nil {
			t.Fatalf("create fixture file %q: %v", filePath, err)
		}
	}

	ignoreContents := "*.tmp\n!/keep.tmp\nbuild/\n"
	if err := createFileWithContents(filesystem, ignorePath, ignoreContents); err != nil {
		t.Fatalf("create ignore file %q: %v", ignorePath, err)
	}

	ignores, err := ignoreext.ReadIgnoreFileFromIRODS(&irodsIgnoreFilesystemAdapter{filesystem: filesystem}, ignorePath, fixtureRoot)
	if err != nil {
		t.Fatalf("read ignore file from iRODS: %v", err)
	}

	rootEntries, err := filesystem.List(fixtureRoot)
	if err != nil {
		t.Fatalf("list root entries: %v", err)
	}
	buildEntries, err := filesystem.List(buildPath)
	if err != nil {
		t.Fatalf("list build entries: %v", err)
	}
	entries := append(rootEntries, buildEntries...)
	filtered := ignoreext.FilterEntries(ignores, entries)
	filteredPaths := pathSet(filtered)

	if _, ok := filteredPaths[dropTmpPath]; ok {
		t.Fatalf("expected filtered result to exclude %q", dropTmpPath)
	}
	if _, ok := filteredPaths[buildPath]; ok {
		t.Fatalf("expected filtered result to exclude %q", buildPath)
	}
	if _, ok := filteredPaths[buildOutPath]; ok {
		t.Fatalf("expected filtered result to exclude %q", buildOutPath)
	}
	if _, ok := filteredPaths[keepTmpPath]; !ok {
		t.Fatalf("expected filtered result to include %q", keepTmpPath)
	}
	if _, ok := filteredPaths[keepTxtPath]; !ok {
		t.Fatalf("expected filtered result to include %q", keepTxtPath)
	}
}

type irodsIgnoreFilesystemAdapter struct {
	filesystem *irodsfs.FileSystem
}

func (adapter *irodsIgnoreFilesystemAdapter) Stat(irodsPath string) (*irodsfs.Entry, error) {
	return adapter.filesystem.Stat(irodsPath)
}

func (adapter *irodsIgnoreFilesystemAdapter) OpenFile(irodsPath string, resource string, mode string) (ignoreext.IRODSFileHandleReader, error) {
	return adapter.filesystem.OpenFile(irodsPath, resource, mode)
}

func pathSet(entries []*irodsfs.Entry) map[string]struct{} {
	result := map[string]struct{}{}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		result[entry.Path] = struct{}{}
	}
	return result
}
