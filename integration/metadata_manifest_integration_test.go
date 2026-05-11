//go:build integration
// +build integration

package integration

import (
	"path"
	"strings"
	"testing"

	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
	metadatairodsfs "github.com/michael-conway/go-irodsclient-extensions/metadata/irodsfs"
	"github.com/rs/xid"
)

func TestMetadataManifestGenerationForFileAndCollectionIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixtureRoot := path.Join(homePath, ".goext-metadata-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureRoot, true, true)
	})

	collectionPath := path.Join(fixtureRoot, "series-a")
	if err := filesystem.MakeDir(collectionPath, true); err != nil {
		t.Fatalf("create fixture collection %q: %v", collectionPath, err)
	}

	filePath := path.Join(collectionPath, "object.txt")
	if err := createFileWithContents(filesystem, filePath, "metadata integration fixture\n"); err != nil {
		t.Fatalf("create fixture file %q: %v", filePath, err)
	}

	if err := filesystem.AddMetadata(collectionPath, "it:collection:attr", "series-a", "integration"); err != nil {
		t.Fatalf("add collection metadata: %v", err)
	}
	if err := filesystem.AddMetadata(filePath, "it:file:attr", "object", "integration"); err != nil {
		t.Fatalf("add file metadata: %v", err)
	}

	service, err := metadata.NewService(metadatairodsfs.NewAdapter(filesystem))
	if err != nil {
		t.Fatalf("create metadata service: %v", err)
	}

	fileManifest, err := service.GenerateManifest(filePath)
	if err != nil {
		t.Fatalf("generate file manifest: %v", err)
	}
	if fileManifest.EntryType != metadata.EntryTypeDataObject {
		t.Fatalf("expected file entry type %q, got %q", metadata.EntryTypeDataObject, fileManifest.EntryType)
	}
	if fileManifest.IRODSPath != filePath || fileManifest.Entry.Path != filePath {
		t.Fatalf("expected file manifest path %q, got manifest=%q entry=%q", filePath, fileManifest.IRODSPath, fileManifest.Entry.Path)
	}
	if !strings.HasPrefix(fileManifest.IRODSURI, "irods://") {
		t.Fatalf("expected irods URI prefix, got %q", fileManifest.IRODSURI)
	}
	if strings.Contains(fileManifest.IRODSURI, "@") {
		t.Fatalf("expected irods URI without userinfo, got %q", fileManifest.IRODSURI)
	}
	if !containsManifestAVU(fileManifest.AVUs, "it:file:attr", "object", "integration") {
		t.Fatalf("expected file AVU to be present, got %+v", fileManifest.AVUs)
	}

	collectionManifest, err := service.GenerateManifest(collectionPath)
	if err != nil {
		t.Fatalf("generate collection manifest: %v", err)
	}
	if collectionManifest.EntryType != metadata.EntryTypeCollection {
		t.Fatalf("expected collection entry type %q, got %q", metadata.EntryTypeCollection, collectionManifest.EntryType)
	}
	if collectionManifest.IRODSPath != collectionPath || collectionManifest.Entry.Path != collectionPath {
		t.Fatalf("expected collection manifest path %q, got manifest=%q entry=%q", collectionPath, collectionManifest.IRODSPath, collectionManifest.Entry.Path)
	}
	if !containsManifestAVU(collectionManifest.AVUs, "it:collection:attr", "series-a", "integration") {
		t.Fatalf("expected collection AVU to be present, got %+v", collectionManifest.AVUs)
	}
}

func containsManifestAVU(avus []metadata.AVU, name string, value string, units string) bool {
	for _, avu := range avus {
		if avu.Name == name && avu.Value == value && avu.Units == units {
			return true
		}
	}
	return false
}
