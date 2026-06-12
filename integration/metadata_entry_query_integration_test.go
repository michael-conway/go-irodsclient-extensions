//go:build integration
// +build integration

package integration

import (
	"path"
	"sort"
	"strings"
	"testing"

	cyfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
	metadatairodsfs "github.com/michael-conway/go-irodsclient-extensions/metadata/irodsfs"
	"github.com/rs/xid"
)

func TestMetadataEntryQueryLiveIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixture := createEntryQueryIntegrationFixture(t, filesystem, homePath)
	adapter := metadatairodsfs.NewAdapter(filesystem)

	bothDescendants := metadata.NewEntryQuery().
		BothKinds().
		Scope(fixture.root, metadata.EntryQueryScopeDescendants).
		AVU(fixture.attrName, "frog-*", "habitat:p%").
		IncludeMatchedAVUs(true).
		Limit(50).
		Build()

	descendantResult := queryEntriesLive(t, adapter, bothDescendants)
	assertEntryPathsExactly(t, descendantResult.Entries, []string{
		fixture.alpha,
		fixture.beta,
		fixture.gamma,
		fixture.alphaFile,
		fixture.deepFile,
		fixture.bravoFile,
	})
	assertEntryPathAbsent(t, descendantResult.Entries, fixture.rootFile)
	assertMatchedAVUPresent(t, descendantResult.MatchedAVUs, fixture.alpha, fixture.attrName, "frog-coll-alpha", "habitat:pond")
	assertMatchedAVUPresent(t, descendantResult.MatchedAVUs, fixture.alphaFile, fixture.attrName, "frog-file-alpha", "habitat:pond")
	assertMatchedAVUPresent(t, descendantResult.MatchedAVUs, fixture.deepFile, fixture.attrName, "frog-file-deep", "habitat:pond")
	assertSingleReplicaForDataObject(t, descendantResult.Entries, fixture.deepFile)

	roundTrippedDescendants := queryEntriesLive(t, adapter, roundTripEntryQueryLive(t, bothDescendants))
	assertEntryPathsExactly(t, roundTrippedDescendants.Entries, entryPaths(descendantResult.Entries))

	dataObjectsChildren := metadata.NewEntryQuery().
		DataObjects().
		Scope(fixture.root, metadata.EntryQueryScopeChildren).
		AVU(fixture.attrName, "frog-*", "habitat:p*").
		Limit(20).
		Build()
	dataObjectChildrenResult := queryEntriesLive(t, adapter, dataObjectsChildren)
	assertEntryPathsExactly(t, dataObjectChildrenResult.Entries, []string{fixture.rootFile})
	assertSingleReplicaForDataObject(t, dataObjectChildrenResult.Entries, fixture.rootFile)

	roundTrippedDataObjects := queryEntriesLive(t, adapter, roundTripEntryQueryLive(t, dataObjectsChildren))
	assertEntryPathsExactly(t, roundTrippedDataObjects.Entries, []string{fixture.rootFile})

	collectionsChildren := metadata.NewEntryQuery().
		Collections().
		Scope(fixture.root, metadata.EntryQueryScopeChildren).
		Equal(metadata.FieldAVUAttrib, fixture.attrName).
		Like(metadata.FieldAVUValue, "frog-coll-*").
		Like(metadata.FieldAVUUnit, "habitat:p%").
		Limit(20).
		Build()
	collectionChildrenResult := queryEntriesLive(t, adapter, collectionsChildren)
	assertEntryPathsExactly(t, collectionChildrenResult.Entries, []string{fixture.alpha})

	collectionsUnderAlpha := metadata.NewEntryQuery().
		Collections().
		Scope(fixture.alpha, metadata.EntryQueryScopeDescendants).
		AVU(fixture.attrName, "frog-coll-*", "habitat:p*").
		Limit(20).
		Build()
	collectionDescendantResult := queryEntriesLive(t, adapter, collectionsUnderAlpha)
	assertEntryPathsExactly(t, collectionDescendantResult.Entries, []string{fixture.beta, fixture.gamma})
	assertEntryPathAbsent(t, collectionDescendantResult.Entries, fixture.alpha)
	assertEntryPathAbsent(t, collectionDescendantResult.Entries, fixture.bravo)

	roundTrippedCollections := queryEntriesLive(t, adapter, roundTripEntryQueryLive(t, collectionsUnderAlpha))
	assertEntryPathsExactly(t, roundTrippedCollections.Entries, []string{fixture.beta, fixture.gamma})

	firstPage, err := adapter.QueryEntries(metadata.NewEntryQuery().
		BothKinds().
		Scope(fixture.root, metadata.EntryQueryScopeChildren).
		AVU(fixture.attrName, "frog-%", "habitat:p%").
		Limit(1).
		Build())
	if err != nil {
		t.Fatalf("query first children page: %v", err)
	}
	if len(firstPage.Entries) != 1 {
		t.Fatalf("expected first page to return 1 entry, got %d", len(firstPage.Entries))
	}
	if !firstPage.Page.HasMore || firstPage.Page.Next == nil {
		t.Fatalf("expected first page to have a next cursor: %+v", firstPage.Page)
	}

	secondPage, err := adapter.QueryEntries(metadata.NewEntryQuery().
		BothKinds().
		Scope(fixture.root, metadata.EntryQueryScopeChildren).
		AVU(fixture.attrName, "frog-%", "habitat:p%").
		Limit(10).
		Cursor(firstPage.Page.Next).
		Build())
	if err != nil {
		t.Fatalf("query second children page: %v", err)
	}

	childrenEntries := append([]*metadata.Entry{}, firstPage.Entries...)
	childrenEntries = append(childrenEntries, secondPage.Entries...)
	assertEntryPathsExactly(t, childrenEntries, []string{fixture.alpha, fixture.rootFile})
	assertEntryPathAbsent(t, childrenEntries, fixture.alphaFile)
}

type entryQueryIntegrationFixture struct {
	root      string
	alpha     string
	beta      string
	gamma     string
	bravo     string
	rootFile  string
	otherFile string
	alphaFile string
	betaFile  string
	deepFile  string
	bravoFile string
	attrName  string
}

func createEntryQueryIntegrationFixture(t *testing.T, filesystem *cyfs.FileSystem, homePath string) entryQueryIntegrationFixture {
	t.Helper()

	fixture := entryQueryIntegrationFixture{
		root:     path.Join(homePath, ".goext-entry-query-integration-"+xid.New().String()),
		attrName: "it:entry-query:" + xid.New().String(),
	}
	fixture.alpha = path.Join(fixture.root, "alpha")
	fixture.beta = path.Join(fixture.alpha, "beta")
	fixture.gamma = path.Join(fixture.beta, "gamma")
	fixture.bravo = path.Join(fixture.root, "bravo")
	fixture.rootFile = path.Join(fixture.root, "root-frog.txt")
	fixture.otherFile = path.Join(fixture.root, "root-lizard.txt")
	fixture.alphaFile = path.Join(fixture.alpha, "alpha-frog.txt")
	fixture.betaFile = path.Join(fixture.beta, "beta-toad.txt")
	fixture.deepFile = path.Join(fixture.gamma, "deep-frog.txt")
	fixture.bravoFile = path.Join(fixture.bravo, "bravo-frog.txt")

	for _, collectionPath := range []string{fixture.gamma, fixture.bravo} {
		if err := filesystem.MakeDir(collectionPath, true); err != nil {
			t.Fatalf("create fixture collection %q: %v", collectionPath, err)
		}
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixture.root, true, true)
	})

	for filePath, contents := range map[string]string{
		fixture.rootFile:  "root frog fixture\n",
		fixture.otherFile: "root lizard fixture\n",
		fixture.alphaFile: "alpha frog fixture\n",
		fixture.betaFile:  "beta toad fixture\n",
		fixture.deepFile:  "deep frog fixture\n",
		fixture.bravoFile: "bravo frog fixture\n",
	} {
		if err := createFileWithContents(filesystem, filePath, contents); err != nil {
			t.Fatalf("create fixture file %q: %v", filePath, err)
		}
	}

	for targetPath, avu := range map[string]struct {
		value string
		unit  string
	}{
		fixture.alpha:     {value: "frog-coll-alpha", unit: "habitat:pond"},
		fixture.beta:      {value: "frog-coll-beta", unit: "habitat:pond"},
		fixture.gamma:     {value: "frog-coll-gamma", unit: "habitat:pond"},
		fixture.bravo:     {value: "bear-coll-bravo", unit: "habitat:forest"},
		fixture.rootFile:  {value: "frog-file-root", unit: "habitat:pond"},
		fixture.otherFile: {value: "lizard-file-root", unit: "habitat:pond"},
		fixture.alphaFile: {value: "frog-file-alpha", unit: "habitat:pond"},
		fixture.betaFile:  {value: "toad-file-beta", unit: "habitat:marsh"},
		fixture.deepFile:  {value: "frog-file-deep", unit: "habitat:pond"},
		fixture.bravoFile: {value: "frog-file-bravo", unit: "habitat:pond"},
	} {
		addEntryQueryIntegrationAVU(t, filesystem, targetPath, fixture.attrName, avu.value, avu.unit)
	}

	noiseAttr := fixture.attrName + ":noise"
	addEntryQueryIntegrationAVU(t, filesystem, fixture.otherFile, noiseAttr, "frog-file-noise", "habitat:pond")
	addEntryQueryIntegrationAVU(t, filesystem, fixture.bravo, noiseAttr, "frog-coll-noise", "habitat:pond")

	return fixture
}

func queryEntriesLive(t *testing.T, adapter *metadatairodsfs.Adapter, query metadata.EntryQuery) metadata.EntryQueryResult {
	t.Helper()

	result, err := adapter.QueryEntries(query)
	if err != nil {
		t.Fatalf("query entries: %v", err)
	}
	return result
}

func roundTripEntryQueryLive(t *testing.T, query metadata.EntryQuery) metadata.EntryQuery {
	t.Helper()

	data, err := metadata.MarshalEntryQueryDefinition(metadata.EntryQueryDefinitionFromQuery(query))
	if err != nil {
		t.Fatalf("marshal entry query definition: %v", err)
	}

	definition, err := metadata.ParseEntryQueryDefinition(data)
	if err != nil {
		t.Fatalf("parse entry query definition: %v", err)
	}

	roundTripped, err := definition.ToEntryQuery(metadata.EntryQueryExecutionOptions{})
	if err != nil {
		t.Fatalf("definition to entry query: %v", err)
	}
	return roundTripped
}

func addEntryQueryIntegrationAVU(t *testing.T, filesystem *cyfs.FileSystem, irodsPath string, name string, value string, unit string) {
	t.Helper()
	if err := filesystem.AddMetadata(irodsPath, name, value, unit); err != nil {
		t.Fatalf("add metadata to %q: %v", irodsPath, err)
	}
}

func assertEntryPathAbsent(t *testing.T, entries []*metadata.Entry, unexpectedPath string) {
	t.Helper()
	for _, entry := range entries {
		if entry != nil && entry.Path == unexpectedPath {
			t.Fatalf("did not expect path %q in entries %+v", unexpectedPath, entryPaths(entries))
		}
	}
}

func assertEntryPathsExactly(t *testing.T, entries []*metadata.Entry, expectedPaths []string) {
	t.Helper()

	actualPaths := entryPaths(entries)
	sort.Strings(actualPaths)
	expected := append([]string(nil), expectedPaths...)
	sort.Strings(expected)

	if len(actualPaths) != len(expected) {
		t.Fatalf("expected paths %+v, got %+v", expected, actualPaths)
	}
	for idx := range expected {
		if actualPaths[idx] != expected[idx] {
			t.Fatalf("expected paths %+v, got %+v", expected, actualPaths)
		}
	}
}

func assertMatchedAVUPresent(t *testing.T, matched map[string][]metadata.AVUStat, entryPath string, name string, value string, unit string) {
	t.Helper()
	for _, avu := range matched[entryPath] {
		if avu.Name == name && avu.Value == value && avu.Units == unit {
			if avu.ID <= 0 {
				t.Fatalf("expected matched AVU ID to be populated for %s=%s[%s], got %+v", name, value, unit, avu)
			}
			return
		}
	}
	t.Fatalf("expected matched AVU %s=%s[%s] for %q, got %+v", name, value, unit, entryPath, matched[entryPath])
}

func assertSingleReplicaForDataObject(t *testing.T, entries []*metadata.Entry, dataObjectPath string) {
	t.Helper()
	for _, entry := range entries {
		if entry == nil || entry.Path != dataObjectPath {
			continue
		}
		if len(entry.IRODSReplicas) != 1 {
			t.Fatalf("expected data object %q to include exactly one selected replica, got %d", dataObjectPath, len(entry.IRODSReplicas))
		}
		return
	}
	t.Fatalf("expected data object %q in entries", dataObjectPath)
}

func entryPaths(entries []*metadata.Entry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		paths = append(paths, entry.Path)
	}
	return paths
}
