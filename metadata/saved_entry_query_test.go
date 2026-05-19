package metadata

import (
	"errors"
	"path"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSavedEntryQueryCreateGetListDelete(t *testing.T) {
	fs := newSavedEntryQueryTestFilesystem()
	service, err := NewSavedEntryQueryService(fs, "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("new saved query service: %v", err)
	}

	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	withSavedEntryQueryNow(t, now)

	definition := EntryQueryDefinition{
		Type:  EntryQueryDefinitionType,
		Kinds: []EntryKind{EntryKindDataObject},
		Scope: &EntryQueryScope{
			Root: "/tempZone/home/test1",
			Mode: EntryQueryScopeChildren,
		},
		Conditions: AVUConditions("project", "frog*", AnyUnit),
		Defaults: EntryQueryDefaults{
			Limit:              25,
			IncludeMatchedAVUs: true,
		},
	}

	saved, err := service.CreateSavedQueryWithDescription(" Frog files ", " Pond matches ", definition)
	if err != nil {
		t.Fatalf("create saved query: %v", err)
	}
	if strings.TrimSpace(saved.ID) == "" {
		t.Fatal("expected generated saved query id")
	}
	if saved.Name != "Frog files" || saved.Description != "Pond matches" {
		t.Fatalf("expected trimmed name and description, got %+v", saved)
	}
	if saved.Version != SavedEntryQueryVersion {
		t.Fatalf("expected saved query version %q, got %q", SavedEntryQueryVersion, saved.Version)
	}
	if saved.Query.Type != EntryQueryDefinitionType {
		t.Fatalf("expected canonical query type %q, got %q", EntryQueryDefinitionType, saved.Query.Type)
	}
	if len(saved.Query.Conditions) != 2 {
		t.Fatalf("expected AVU conditions, got %+v", saved.Query.Conditions)
	}
	if !saved.CreatedAt.Equal(now) || !saved.UpdatedAt.Equal(now) {
		t.Fatalf("expected timestamps %s, got created=%s updated=%s", now, saved.CreatedAt, saved.UpdatedAt)
	}

	expectedCollection := "/tempZone/home/test1/.irodsext/metadata_queries"
	expectedPath := path.Join(expectedCollection, saved.ID+SavedEntryQueryFileExt)
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext"]; !ok {
		t.Fatal("expected root extension collection to be created")
	}
	if _, ok := fs.collections[expectedCollection]; !ok {
		t.Fatal("expected saved query collection to be created")
	}
	if _, ok := fs.objects[expectedPath]; !ok {
		t.Fatalf("expected saved query object %q to be written", expectedPath)
	}

	got, err := service.GetSavedQuery(saved.ID + SavedEntryQueryFileExt)
	if err != nil {
		t.Fatalf("get saved query by filename: %v", err)
	}
	if got.ID != saved.ID || got.Name != saved.Name {
		t.Fatalf("unexpected saved query from get: %+v", got)
	}

	fs.objects[path.Join(expectedCollection, "ignore.txt")] = []byte("ignored")
	summaries, err := service.ListSavedQueries()
	if err != nil {
		t.Fatalf("list saved queries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one saved query summary, got %+v", summaries)
	}
	if summaries[0].ID != saved.ID || summaries[0].IRODSPath != expectedPath {
		t.Fatalf("unexpected saved query summary %+v", summaries[0])
	}

	includeMatched := false
	query, err := service.ToEntryQuery(saved.ID, EntryQueryExecutionOptions{
		Limit:              5,
		IncludeMatchedAVUs: &includeMatched,
	})
	if err != nil {
		t.Fatalf("convert saved query to executable query: %v", err)
	}
	if query.Limit != 5 || query.IncludeMatchedAVUs {
		t.Fatalf("expected execution overrides to apply, got %+v", query)
	}
	if !EntryQueryHasKind(query, EntryKindDataObject) || EntryQueryHasKind(query, EntryKindCollection) {
		t.Fatalf("expected data object-only query, got %+v", query.Kinds)
	}

	if err := service.DeleteSavedQuery(saved.ID, true); err != nil {
		t.Fatalf("delete saved query: %v", err)
	}
	if _, ok := fs.objects[expectedPath]; ok {
		t.Fatalf("expected saved query object %q to be deleted", expectedPath)
	}
}

func TestSavedEntryQueryPutPreservesCreatedAtAndReplacesQuery(t *testing.T) {
	fs := newSavedEntryQueryTestFilesystem()
	service, _ := NewSavedEntryQueryService(fs, "/tempZone/home/test1")

	createdAt := time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)
	withSavedEntryQueryNow(t, createdAt)
	saved, err := service.CreateSavedQuery("Original", EntryQueryDefinition{
		Kinds: []EntryKind{EntryKindDataObject},
		Conditions: []EntryCondition{
			{Field: FieldName, Op: OpLike, Value: "raw*"},
		},
	})
	if err != nil {
		t.Fatalf("create saved query: %v", err)
	}

	updatedAt := time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC)
	withSavedEntryQueryNow(t, updatedAt)
	updated, err := service.PutSavedQuery(saved.ID, SavedEntryQueryUpdate{
		Name: "Updated",
		Query: EntryQueryDefinition{
			Kinds: []EntryKind{EntryKindCollection},
			Scope: &EntryQueryScope{
				Root: "/tempZone/home/test1/project",
				Mode: EntryQueryScopeDescendants,
			},
			Conditions: []EntryCondition{
				{Field: FieldAVUAttrib, Op: OpEqual, Value: "project"},
			},
		},
	})
	if err != nil {
		t.Fatalf("put saved query: %v", err)
	}
	if updated.ID != saved.ID {
		t.Fatalf("expected update to preserve id %q, got %q", saved.ID, updated.ID)
	}
	if updated.Name != "Updated" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	if !updated.CreatedAt.Equal(createdAt) || !updated.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected timestamps after update: created=%s updated=%s", updated.CreatedAt, updated.UpdatedAt)
	}
	if !EntryQueryHasKind(mustEntryQueryFromDefinition(t, updated.Query), EntryKindCollection) {
		t.Fatalf("expected collection query after update, got %+v", updated.Query.Kinds)
	}
}

func TestSavedEntryQueryDefaultsBlankDisplayFields(t *testing.T) {
	fs := newSavedEntryQueryTestFilesystem()
	service, _ := NewSavedEntryQueryService(fs, "/tempZone/home/test1")

	saved, err := service.CreateSavedQueryWithDescription(" ", " ", EntryQueryDefinition{
		Kinds: []EntryKind{EntryKindDataObject},
		Conditions: []EntryCondition{
			{Field: FieldAVUAttrib, Op: OpEqual, Value: "project"},
		},
	})
	if err != nil {
		t.Fatalf("create saved query with blank display fields: %v", err)
	}
	if saved.Name != DefaultSavedEntryQueryName {
		t.Fatalf("expected default name %q, got %q", DefaultSavedEntryQueryName, saved.Name)
	}
	if saved.Description != "" {
		t.Fatalf("expected blank description, got %q", saved.Description)
	}

	updated, err := service.PutSavedQuery(saved.ID, SavedEntryQueryUpdate{
		Name:        "",
		Description: "  ",
		Query: EntryQueryDefinition{
			Kinds: []EntryKind{EntryKindCollection},
			Conditions: []EntryCondition{
				{Field: FieldAVUAttrib, Op: OpEqual, Value: "project"},
			},
		},
	})
	if err != nil {
		t.Fatalf("put saved query with blank display fields: %v", err)
	}
	if updated.ID != saved.ID {
		t.Fatalf("expected update to preserve id %q, got %q", saved.ID, updated.ID)
	}
	if updated.Name != DefaultSavedEntryQueryName {
		t.Fatalf("expected default updated name %q, got %q", DefaultSavedEntryQueryName, updated.Name)
	}
	if updated.Description != "" {
		t.Fatalf("expected blank updated description, got %q", updated.Description)
	}
}

func TestSavedEntryQueryDisplayNamesAreNotUniqueIDs(t *testing.T) {
	fs := newSavedEntryQueryTestFilesystem()
	service, _ := NewSavedEntryQueryService(fs, "/tempZone/home/test1")

	definition := EntryQueryDefinition{
		Kinds: []EntryKind{EntryKindDataObject},
		Conditions: []EntryCondition{
			{Field: FieldAVUAttrib, Op: OpEqual, Value: "project"},
		},
	}

	first, err := service.CreateSavedQuery("Shared Name", definition)
	if err != nil {
		t.Fatalf("create first saved query: %v", err)
	}
	second, err := service.CreateSavedQuery("Shared Name", definition)
	if err != nil {
		t.Fatalf("create second saved query: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected distinct ids for saved queries with same display name %q", first.Name)
	}
	if first.Name != second.Name {
		t.Fatalf("expected matching display names, got %q and %q", first.Name, second.Name)
	}

	summaries, err := service.ListSavedQueries()
	if err != nil {
		t.Fatalf("list saved queries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected two saved queries with duplicate display names, got %+v", summaries)
	}
	ids := []string{summaries[0].ID, summaries[1].ID}
	sort.Strings(ids)
	expectedIDs := []string{first.ID, second.ID}
	sort.Strings(expectedIDs)
	if ids[0] != expectedIDs[0] || ids[1] != expectedIDs[1] {
		t.Fatalf("unexpected saved query ids from list: got %+v expected %+v", ids, expectedIDs)
	}
}

func TestSavedEntryQueryListReturnsCorruptSavedQueryError(t *testing.T) {
	fs := newSavedEntryQueryTestFilesystem()
	service, _ := NewSavedEntryQueryService(fs, "/tempZone/home/test1")
	if _, err := service.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	fs.objects[path.Join(service.CollectionPath(), "bad"+SavedEntryQueryFileExt)] = []byte(`{"version":"metadata.saved_entry_query.v1","id":"bad"}`)

	_, err := service.ListSavedQueries()
	if !errors.Is(err, ErrInvalidSavedEntryQuery) {
		t.Fatalf("expected ErrInvalidSavedEntryQuery for corrupt saved query, got %v", err)
	}
}

func TestParseSavedEntryQueryRejectsMissingQueryAndUnknownFields(t *testing.T) {
	payload := []byte(`{"version":"metadata.saved_entry_query.v1","id":"abc","name":"missing query","created_at":"2026-05-15T12:00:00Z","updated_at":"2026-05-15T12:00:00Z"}`)
	if _, err := ParseSavedEntryQuery(payload); !errors.Is(err, ErrInvalidSavedEntryQuery) {
		t.Fatalf("expected ErrInvalidSavedEntryQuery for missing query, got %v", err)
	}

	payload = []byte(`{"version":"metadata.saved_entry_query.v1","id":"abc","name":"unknown","query":{},"created_at":"2026-05-15T12:00:00Z","updated_at":"2026-05-15T12:00:00Z","unexpected":true}`)
	if _, err := ParseSavedEntryQuery(payload); !errors.Is(err, ErrInvalidSavedEntryQuery) {
		t.Fatalf("expected ErrInvalidSavedEntryQuery for unknown field, got %v", err)
	}
}

func TestSavedEntryQueryValidation(t *testing.T) {
	fs := newSavedEntryQueryTestFilesystem()

	if _, err := NewSavedEntryQueryService(nil, "/tempZone/home/test1"); !errors.Is(err, ErrMissingFilesystem) {
		t.Fatalf("expected ErrMissingFilesystem, got %v", err)
	}
	if _, err := NewSavedEntryQueryService(fs, "tempZone/home/test1"); !errors.Is(err, ErrInvalidUserHome) {
		t.Fatalf("expected ErrInvalidUserHome, got %v", err)
	}

	service, _ := NewSavedEntryQueryService(fs, "/tempZone/home/test1")
	if _, err := service.GetSavedQuery("../bad"); !errors.Is(err, ErrInvalidSavedEntryQueryID) {
		t.Fatalf("expected ErrInvalidSavedEntryQueryID, got %v", err)
	}
	if _, err := service.CreateSavedQuery("bad query", EntryQueryDefinition{ReplicaPolicy: ReplicaPolicy("all")}); !errors.Is(err, ErrInvalidEntryQuery) {
		t.Fatalf("expected ErrInvalidEntryQuery, got %v", err)
	}
}

func mustEntryQueryFromDefinition(t *testing.T, definition EntryQueryDefinition) EntryQuery {
	t.Helper()

	query, err := definition.ToEntryQuery(EntryQueryExecutionOptions{})
	if err != nil {
		t.Fatalf("definition to query: %v", err)
	}
	return query
}

func withSavedEntryQueryNow(t *testing.T, now time.Time) {
	t.Helper()

	oldNow := savedEntryQueryNow
	savedEntryQueryNow = func() time.Time {
		return now
	}
	t.Cleanup(func() {
		savedEntryQueryNow = oldNow
	})
}

type savedEntryQueryTestFilesystem struct {
	collections map[string]struct{}
	objects     map[string][]byte
}

func newSavedEntryQueryTestFilesystem() *savedEntryQueryTestFilesystem {
	return &savedEntryQueryTestFilesystem{
		collections: map[string]struct{}{},
		objects:     map[string][]byte{},
	}
}

func (fs *savedEntryQueryTestFilesystem) CollectionExists(irodsPath string) (bool, error) {
	_, ok := fs.collections[path.Clean(irodsPath)]
	return ok, nil
}

func (fs *savedEntryQueryTestFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	fs.collections[path.Clean(irodsPath)] = struct{}{}
	return nil
}

func (fs *savedEntryQueryTestFilesystem) ListDataObjects(collectionPath string) ([]string, error) {
	collectionPath = strings.TrimSuffix(path.Clean(collectionPath), "/")

	objects := make([]string, 0)
	for objectPath := range fs.objects {
		if path.Dir(objectPath) == collectionPath {
			objects = append(objects, objectPath)
		}
	}
	sort.Strings(objects)
	return objects, nil
}

func (fs *savedEntryQueryTestFilesystem) ReadDataObject(dataObjectPath string) ([]byte, error) {
	dataObjectPath = path.Clean(dataObjectPath)
	contents, ok := fs.objects[dataObjectPath]
	if !ok {
		return nil, errors.New("object does not exist")
	}
	return append([]byte(nil), contents...), nil
}

func (fs *savedEntryQueryTestFilesystem) WriteDataObject(dataObjectPath string, contents []byte) error {
	fs.objects[path.Clean(dataObjectPath)] = append([]byte(nil), contents...)
	return nil
}

func (fs *savedEntryQueryTestFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	delete(fs.objects, path.Clean(dataObjectPath))
	return nil
}
