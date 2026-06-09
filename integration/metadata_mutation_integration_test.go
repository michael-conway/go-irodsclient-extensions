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

func TestMetadataReplacePathAVUIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixtureRoot := path.Join(homePath, ".goext-metadata-mutation-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureRoot, true, true)
	})

	collectionPath := path.Join(fixtureRoot, "collection")
	if err := filesystem.MakeDir(collectionPath, true); err != nil {
		t.Fatalf("create fixture collection %q: %v", collectionPath, err)
	}

	filePath := path.Join(collectionPath, "object.txt")
	if err := createFileWithContents(filesystem, filePath, "metadata mutation fixture\n"); err != nil {
		t.Fatalf("create fixture file %q: %v", filePath, err)
	}

	service, err := metadata.NewMutationService(metadatairodsfs.NewAdapter(filesystem))
	if err != nil {
		t.Fatalf("create metadata mutation service: %v", err)
	}

	t.Run("data object", func(t *testing.T) {
		attribute := "it:replace:file:" + xid.New().String()
		if err := filesystem.AddMetadata(filePath, attribute, "before", "integration"); err != nil {
			t.Fatalf("add file metadata: %v", err)
		}

		updated, err := service.ReplacePathAVU(filePath, metadata.AVUReplacement{
			From: metadata.AVUStat{Name: attribute, Value: "before", Units: "integration"},
			To:   metadata.AVUStat{Name: attribute, Value: "after", Units: "integration"},
		})
		if err != nil {
			t.Fatalf("replace file AVU: %v", err)
		}
		if updated.Name != attribute || updated.Value != "after" || updated.Units != "integration" {
			t.Fatalf("unexpected replacement AVU: %+v", updated)
		}

		metadataList, err := filesystem.ListMetadata(filePath)
		if err != nil {
			t.Fatalf("list file metadata: %v", err)
		}
		if hasIRODSAVU(metadataList, attribute, "before", "integration") {
			t.Fatalf("expected old file AVU to be absent, got %+v", metadataList)
		}
		if !hasIRODSAVU(metadataList, attribute, "after", "integration") {
			t.Fatalf("expected replacement file AVU, got %+v", metadataList)
		}
	})

	t.Run("collection", func(t *testing.T) {
		attribute := "it:replace:collection:" + xid.New().String()
		if err := filesystem.AddMetadata(collectionPath, attribute, "before", "integration"); err != nil {
			t.Fatalf("add collection metadata: %v", err)
		}

		updated, err := service.ReplacePathAVU(collectionPath, metadata.AVUReplacement{
			From: metadata.AVUStat{Name: attribute, Value: "before", Units: "integration"},
			To:   metadata.AVUStat{Name: attribute, Value: "after", Units: "integration"},
		})
		if err != nil {
			t.Fatalf("replace collection AVU: %v", err)
		}
		if updated.Name != attribute || updated.Value != "after" || updated.Units != "integration" {
			t.Fatalf("unexpected replacement AVU: %+v", updated)
		}

		metadataList, err := filesystem.ListMetadata(collectionPath)
		if err != nil {
			t.Fatalf("list collection metadata: %v", err)
		}
		if hasIRODSAVU(metadataList, attribute, "before", "integration") {
			t.Fatalf("expected old collection AVU to be absent, got %+v", metadataList)
		}
		if !hasIRODSAVU(metadataList, attribute, "after", "integration") {
			t.Fatalf("expected replacement collection AVU, got %+v", metadataList)
		}
	})
}
