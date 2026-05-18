//go:build integration
// +build integration

package integration

import (
	"io"
	"strings"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
	"github.com/rs/xid"
)

func TestSavedEntryQueryIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationSecondaryTestFilesystem(t)
	defer filesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty secondary user home path")
	}

	service, err := metadata.NewSavedEntryQueryService(&irodsSavedEntryQueryFilesystem{filesystem: filesystem}, homePath)
	if err != nil {
		t.Fatalf("create saved entry query service: %v", err)
	}

	queryName := "it-saved-entry-query-" + xid.New().String()
	saved, err := service.CreateSavedQueryWithDescription(queryName, "integration query", metadata.EntryQueryDefinition{
		Type:  metadata.AVUQueryDefinitionType,
		Kinds: []metadata.EntryKind{metadata.EntryKindDataObject, metadata.EntryKindCollection},
		Scope: &metadata.EntryQueryScope{
			Root: homePath,
			Mode: metadata.EntryQueryScopeDescendants,
		},
		AVU: &metadata.AVUQuerySpec{
			Attrib: "integration:saved-query:" + xid.New().String(),
			Value:  "frog*",
			Unit:   metadata.AnyUnit,
		},
		Defaults: metadata.EntryQueryDefaults{
			Limit:              25,
			IncludeMatchedAVUs: true,
		},
	})
	if err != nil {
		t.Fatalf("create saved entry query: %v", err)
	}
	t.Cleanup(func() {
		_ = service.DeleteSavedQuery(saved.ID, true)
	})

	got, err := service.GetSavedQuery(saved.ID)
	if err != nil {
		t.Fatalf("get saved entry query: %v", err)
	}
	if got.Name != queryName || got.Description != "integration query" {
		t.Fatalf("unexpected saved query metadata: %+v", got)
	}
	if got.Query.Type != metadata.EntryQueryDefinitionType || got.Query.AVU != nil {
		t.Fatalf("expected canonical entry query definition, got %+v", got.Query)
	}
	if len(got.Query.Conditions) != 2 {
		t.Fatalf("expected AVU shorthand to expand to two canonical conditions, got %+v", got.Query.Conditions)
	}

	updatedName := queryName + "-updated"
	updated, err := service.PutSavedQuery(saved.ID, metadata.SavedEntryQueryUpdate{
		Name:        updatedName,
		Description: "updated integration query",
		Query: metadata.EntryQueryDefinition{
			Kinds: []metadata.EntryKind{metadata.EntryKindCollection},
			Scope: &metadata.EntryQueryScope{
				Root: homePath,
				Mode: metadata.EntryQueryScopeChildren,
			},
			Conditions: []metadata.EntryCondition{
				{Field: metadata.FieldAVUAttrib, Op: metadata.OpEqual, Value: "integration:saved-query"},
			},
			Defaults: metadata.EntryQueryDefaults{
				Limit: 10,
			},
		},
	})
	if err != nil {
		t.Fatalf("update saved entry query: %v", err)
	}
	if updated.Name != updatedName {
		t.Fatalf("expected updated name %q, got %q", updatedName, updated.Name)
	}
	if !updated.CreatedAt.Equal(saved.CreatedAt) {
		t.Fatalf("expected update to preserve created_at %s, got %s", saved.CreatedAt, updated.CreatedAt)
	}

	listed, err := service.ListSavedQueries()
	if err != nil {
		t.Fatalf("list saved entry queries: %v", err)
	}
	found := false
	for _, summary := range listed {
		if summary.ID != saved.ID {
			continue
		}
		found = true
		if summary.Name != updatedName || !strings.HasSuffix(summary.FileName, metadata.SavedEntryQueryFileExt) {
			t.Fatalf("unexpected saved query summary: %+v", summary)
		}
	}
	if !found {
		t.Fatalf("expected saved query %q in list, got %+v", saved.ID, listed)
	}

	executable, err := service.ToEntryQuery(saved.ID, metadata.EntryQueryExecutionOptions{Limit: 7})
	if err != nil {
		t.Fatalf("convert saved query to executable query: %v", err)
	}
	if executable.Limit != 7 {
		t.Fatalf("expected execution limit override 7, got %d", executable.Limit)
	}
	if !metadata.EntryQueryHasKind(executable, metadata.EntryKindCollection) || metadata.EntryQueryHasKind(executable, metadata.EntryKindDataObject) {
		t.Fatalf("expected updated collection-only query, got %+v", executable.Kinds)
	}

	if err := service.DeleteSavedQuery(saved.ID, true); err != nil {
		t.Fatalf("delete saved entry query: %v", err)
	}
}

type irodsSavedEntryQueryFilesystem struct {
	filesystem *irodsfs.FileSystem
}

func (filesystem *irodsSavedEntryQueryFilesystem) CollectionExists(irodsPath string) (bool, error) {
	return filesystem.filesystem.ExistsDir(irodsPath), nil
}

func (filesystem *irodsSavedEntryQueryFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	return filesystem.filesystem.MakeDir(irodsPath, recurse)
}

func (filesystem *irodsSavedEntryQueryFilesystem) ListDataObjects(collectionPath string) ([]string, error) {
	entries, err := filesystem.filesystem.List(collectionPath)
	if err != nil {
		return nil, err
	}

	dataObjectPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.IsDir() {
			continue
		}
		dataObjectPaths = append(dataObjectPaths, entry.Path)
	}
	return dataObjectPaths, nil
}

func (filesystem *irodsSavedEntryQueryFilesystem) ReadDataObject(dataObjectPath string) ([]byte, error) {
	handle, err := filesystem.filesystem.OpenFile(dataObjectPath, "", "r")
	if err != nil {
		return nil, err
	}
	defer handle.Close() //nolint
	return io.ReadAll(handle)
}

func (filesystem *irodsSavedEntryQueryFilesystem) WriteDataObject(dataObjectPath string, contents []byte) error {
	return createFileWithContents(filesystem.filesystem, dataObjectPath, string(contents))
}

func (filesystem *irodsSavedEntryQueryFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	return filesystem.filesystem.RemoveFile(dataObjectPath, force)
}
