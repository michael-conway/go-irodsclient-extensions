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
		Type:  metadata.EntryQueryDefinitionType,
		Kinds: []metadata.EntryKind{metadata.EntryKindDataObject, metadata.EntryKindCollection},
		Scope: &metadata.EntryQueryScope{
			Root: homePath,
			Mode: metadata.EntryQueryScopeDescendants,
		},
		Conditions: metadata.AVUConditions("integration:saved-query:"+xid.New().String(), "frog*", metadata.AnyUnit),
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
	if got.Query.Type != metadata.EntryQueryDefinitionType {
		t.Fatalf("expected canonical entry query definition, got %+v", got.Query)
	}
	if len(got.Query.Conditions) != 2 {
		t.Fatalf("expected two canonical AVU conditions, got %+v", got.Query.Conditions)
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
	if updated.ID != saved.ID {
		t.Fatalf("expected update to preserve id %q, got %q", saved.ID, updated.ID)
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

func TestSavedEntryQueryDisplayDefaultsIntegration(t *testing.T) {
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

	defaulted, err := service.CreateSavedQueryWithDescription(" ", " ", savedEntryQueryIntegrationDefinition(homePath, "integration:saved-query-default:"+xid.New().String()))
	if err != nil {
		t.Fatalf("create saved query with blank display fields: %v", err)
	}
	t.Cleanup(func() {
		_ = service.DeleteSavedQuery(defaulted.ID, true)
	})

	if defaulted.Name != metadata.DefaultSavedEntryQueryName {
		t.Fatalf("expected default saved query name %q, got %q", metadata.DefaultSavedEntryQueryName, defaulted.Name)
	}
	if defaulted.Description != "" {
		t.Fatalf("expected blank saved query description, got %q", defaulted.Description)
	}

	got, err := service.GetSavedQuery(defaulted.ID)
	if err != nil {
		t.Fatalf("get defaulted saved query: %v", err)
	}
	if got.ID != defaulted.ID || got.Name != metadata.DefaultSavedEntryQueryName || got.Description != "" {
		t.Fatalf("unexpected defaulted saved query from storage: %+v", got)
	}

	updated, err := service.PutSavedQuery(defaulted.ID, metadata.SavedEntryQueryUpdate{
		Name:        "",
		Description: "   ",
		Query:       savedEntryQueryIntegrationDefinition(homePath, "integration:saved-query-default-updated:"+xid.New().String()),
	})
	if err != nil {
		t.Fatalf("update saved query with blank display fields: %v", err)
	}
	if updated.ID != defaulted.ID {
		t.Fatalf("expected blank-field update to preserve id %q, got %q", defaulted.ID, updated.ID)
	}
	if updated.Name != metadata.DefaultSavedEntryQueryName || updated.Description != "" {
		t.Fatalf("unexpected updated default display fields: %+v", updated)
	}
}

func TestSavedEntryQueryDuplicateDisplayNamesIntegration(t *testing.T) {
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

	sharedName := "it-duplicate-display-" + xid.New().String()
	first, err := service.CreateSavedQuery(sharedName, savedEntryQueryIntegrationDefinition(homePath, "integration:saved-query-duplicate:first:"+xid.New().String()))
	if err != nil {
		t.Fatalf("create first duplicate display saved query: %v", err)
	}
	t.Cleanup(func() {
		_ = service.DeleteSavedQuery(first.ID, true)
	})

	second, err := service.CreateSavedQuery(sharedName, savedEntryQueryIntegrationDefinition(homePath, "integration:saved-query-duplicate:second:"+xid.New().String()))
	if err != nil {
		t.Fatalf("create second duplicate display saved query: %v", err)
	}
	t.Cleanup(func() {
		_ = service.DeleteSavedQuery(second.ID, true)
	})

	if first.ID == second.ID {
		t.Fatalf("expected distinct saved query ids for duplicate display name %q", sharedName)
	}
	if first.Name != sharedName || second.Name != sharedName {
		t.Fatalf("expected duplicate display names %q, got %q and %q", sharedName, first.Name, second.Name)
	}

	listed, err := service.ListSavedQueries()
	if err != nil {
		t.Fatalf("list saved queries with duplicate display names: %v", err)
	}
	found := map[string]bool{}
	for _, summary := range listed {
		if summary.Name != sharedName {
			continue
		}
		found[summary.ID] = true
	}
	if !found[first.ID] || !found[second.ID] {
		t.Fatalf("expected both duplicate-display saved queries in list, found %+v in %+v", found, listed)
	}
}

func savedEntryQueryIntegrationDefinition(homePath string, attr string) metadata.EntryQueryDefinition {
	return metadata.EntryQueryDefinition{
		Kinds: []metadata.EntryKind{metadata.EntryKindDataObject},
		Scope: &metadata.EntryQueryScope{
			Root: homePath,
			Mode: metadata.EntryQueryScopeDescendants,
		},
		Conditions: []metadata.EntryCondition{
			{Field: metadata.FieldAVUAttrib, Op: metadata.OpEqual, Value: attr},
		},
		Defaults: metadata.EntryQueryDefaults{
			Limit: 25,
		},
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
