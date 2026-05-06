package s3admin

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
)

func TestAddBucketWritesAVUAndMappingFile(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")

	service := newTestService(t, fs)

	bucket, err := service.AddBucket("/tempZone/home/test1", "test-bucket")
	if err != nil {
		t.Fatalf("add bucket: %v", err)
	}

	if bucket.Name != "test-bucket" || bucket.IRODSPath != "/tempZone/home/test1" {
		t.Fatalf("unexpected bucket %+v", bucket)
	}

	metadata := fs.metadata["/tempZone/home/test1"]
	if len(metadata) != 1 {
		t.Fatalf("expected one metadata entry, got %d", len(metadata))
	}
	if metadata[0].Name != AVUBucketAttribute || metadata[0].Value != "test-bucket" || metadata[0].Units != "" {
		t.Fatalf("unexpected bucket metadata %+v", metadata[0])
	}

	mapping := readMappingFile(t, service.MappingFilePath())
	if mapping["test-bucket"] != "/tempZone/home/test1" {
		t.Fatalf("expected mapping for test-bucket, got %+v", mapping)
	}
}

func TestAddBucketRejectsDuplicateName(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.addCollection("/tempZone/home/test2")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "shared-bucket"},
	}

	service := newTestService(t, fs)

	_, err := service.AddBucket("/tempZone/home/test2", "shared-bucket")
	if !errors.Is(err, ErrDuplicateBucket) {
		t.Fatalf("expected ErrDuplicateBucket, got %v", err)
	}

	var duplicateErr *DuplicateBucketError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("expected DuplicateBucketError, got %T", err)
	}
	if duplicateErr.ExistingPath != "/tempZone/home/test1" || duplicateErr.RequestedPath != "/tempZone/home/test2" {
		t.Fatalf("unexpected duplicate error %+v", duplicateErr)
	}
	if !fs.wasSearched(AVUBucketAttribute, "shared-bucket") {
		t.Fatal("expected duplicate check to use metadata search")
	}
}

func TestAddBucketChecksDuplicatesBeforeIdempotentReturn(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.addCollection("/tempZone/home/test2")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "shared-bucket"},
	}
	fs.metadata["/tempZone/home/test2"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "shared-bucket"},
	}

	service := newTestService(t, fs)

	_, err := service.AddBucket("/tempZone/home/test2", "shared-bucket")
	if !errors.Is(err, ErrDuplicateBucket) {
		t.Fatalf("expected ErrDuplicateBucket, got %v", err)
	}
}

func TestAddBucketRejectsDifferentBucketOnSameCollection(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "old-bucket"},
	}

	service := newTestService(t, fs)

	if _, err := service.AddBucket("/tempZone/home/test1", "new-bucket"); !errors.Is(err, ErrBucketAlreadySet) {
		t.Fatalf("expected ErrBucketAlreadySet, got %v", err)
	}
}

func TestUpdateBucketReplacesAVUAndMappingFile(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "old-bucket"},
		{Name: "unrelated", Value: "keep"},
	}

	service := newTestService(t, fs)

	bucket, err := service.UpdateBucket("/tempZone/home/test1", "new-bucket")
	if err != nil {
		t.Fatalf("update bucket: %v", err)
	}
	if bucket.Name != "new-bucket" || bucket.IRODSPath != "/tempZone/home/test1" {
		t.Fatalf("unexpected updated bucket %+v", bucket)
	}

	metadata := fs.metadata["/tempZone/home/test1"]
	if len(metadata) != 2 {
		t.Fatalf("expected bucket and unrelated metadata, got %+v", metadata)
	}

	var bucketValues []string
	for _, avu := range metadata {
		if avu.Name == AVUBucketAttribute {
			bucketValues = append(bucketValues, avu.Value)
		}
	}
	if len(bucketValues) != 1 || bucketValues[0] != "new-bucket" {
		t.Fatalf("expected single replacement bucket AVU, got %+v", bucketValues)
	}

	mapping := readMappingFile(t, service.MappingFilePath())
	if _, ok := mapping["old-bucket"]; ok {
		t.Fatalf("expected old bucket to be removed from mapping, got %+v", mapping)
	}
	if mapping["new-bucket"] != "/tempZone/home/test1" {
		t.Fatalf("expected new bucket mapping, got %+v", mapping)
	}
}

func TestUpdateBucketRejectsDuplicateName(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.addCollection("/tempZone/home/test2")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "alpha"},
	}
	fs.metadata["/tempZone/home/test2"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "bravo"},
	}

	service := newTestService(t, fs)

	if _, err := service.UpdateBucket("/tempZone/home/test1", "bravo"); !errors.Is(err, ErrDuplicateBucket) {
		t.Fatalf("expected ErrDuplicateBucket, got %v", err)
	}
}

func TestDeleteBucketRemovesAVUAndMappingFileEntry(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.addCollection("/tempZone/home/test2")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "alpha"},
		{Name: "unrelated", Value: "keep"},
	}
	fs.metadata["/tempZone/home/test2"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "bravo"},
	}

	service := newTestService(t, fs)

	if err := service.DeleteBucket("/tempZone/home/test1"); err != nil {
		t.Fatalf("delete bucket: %v", err)
	}

	metadata := fs.metadata["/tempZone/home/test1"]
	if len(metadata) != 1 || metadata[0].Name != "unrelated" {
		t.Fatalf("expected unrelated metadata to remain, got %+v", metadata)
	}

	mapping := readMappingFile(t, service.MappingFilePath())
	if _, ok := mapping["alpha"]; ok {
		t.Fatalf("expected deleted bucket to be removed, got %+v", mapping)
	}
	if mapping["bravo"] != "/tempZone/home/test2" {
		t.Fatalf("expected remaining bucket in mapping, got %+v", mapping)
	}
}

func TestListBucketsSupportsRecursiveOption(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.addCollection("/tempZone/home/test1/deep")
	fs.addCollection("/tempZone/home/test2")
	fs.metadata["/tempZone/home"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "home-bucket"},
	}
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "test1-bucket"},
	}
	fs.metadata["/tempZone/home/test1/deep"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "deep-bucket"},
	}
	fs.metadata["/tempZone/home/test2/file.txt"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "ignored-file-bucket"},
	}
	fs.objects["/tempZone/home/test2/file.txt"] = struct{}{}

	service := newTestService(t, fs)

	nonRecursive, err := service.ListBuckets(ListOptions{IRODSPath: "/tempZone/home", Recursive: false})
	if err != nil {
		t.Fatalf("list non-recursive buckets: %v", err)
	}
	if bucketNames(nonRecursive) != "home-bucket,test1-bucket" {
		t.Fatalf("unexpected non-recursive buckets %+v", nonRecursive)
	}

	recursive, err := service.ListBuckets(ListOptions{IRODSPath: "/tempZone/home", Recursive: true})
	if err != nil {
		t.Fatalf("list recursive buckets: %v", err)
	}
	if bucketNames(recursive) != "deep-bucket,home-bucket,test1-bucket" {
		t.Fatalf("unexpected recursive buckets %+v", recursive)
	}
	if !fs.wasSearched(AVUBucketAttribute, metadataValueWildcard) {
		t.Fatal("expected list buckets to use metadata search")
	}
}

func TestListBucketsSupportsBucketNameFilter(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.addCollection("/tempZone/home/test2")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "alpha"},
	}
	fs.metadata["/tempZone/home/test2"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "bravo"},
	}

	service := newTestService(t, fs)

	buckets, err := service.ListBuckets(ListOptions{
		IRODSPath:  "/tempZone/home",
		BucketName: "bravo",
		Recursive:  true,
	})
	if err != nil {
		t.Fatalf("list buckets by name: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Name != "bravo" || buckets[0].IRODSPath != "/tempZone/home/test2" {
		t.Fatalf("unexpected bucket-name search result %+v", buckets)
	}
	if !fs.wasSearched(AVUBucketAttribute, "bravo") {
		t.Fatal("expected bucket-name list to use exact metadata search")
	}
}

func TestSearchBucketsUsesMetadataSearch(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "alpha"},
	}

	service := newTestService(t, fs)

	buckets, err := service.SearchBuckets("alpha", ListOptions{IRODSPath: "/tempZone/home", Recursive: true})
	if err != nil {
		t.Fatalf("search buckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Name != "alpha" || buckets[0].IRODSPath != "/tempZone/home/test1" {
		t.Fatalf("unexpected search result %+v", buckets)
	}
	if !fs.wasSearched(AVUBucketAttribute, "alpha") {
		t.Fatal("expected search buckets to use metadata search")
	}
}

func TestRefreshMappingUsesAVUsAsSourceOfTruth(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "test1-bucket"},
	}

	service := newTestService(t, fs)
	if err := os.WriteFile(service.MappingFilePath(), []byte(`{"stale":"/tempZone/home/stale"}`), 0o644); err != nil {
		t.Fatalf("write stale mapping: %v", err)
	}

	if err := service.RefreshMapping(); err != nil {
		t.Fatalf("refresh mapping: %v", err)
	}

	mapping := readMappingFile(t, service.MappingFilePath())
	if len(mapping) != 1 || mapping["test1-bucket"] != "/tempZone/home/test1" {
		t.Fatalf("expected AVU-derived mapping, got %+v", mapping)
	}
}

func TestRefreshMappingFallsBackToKnownMappingBucketNames(t *testing.T) {
	fs := newTestFilesystem()
	fs.exactSearchOnly = true
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "known-bucket"},
	}

	service := newTestService(t, fs)
	if err := os.WriteFile(service.MappingFilePath(), []byte(`{"known-bucket":"/tempZone/home/test1","stale":"/tempZone/home/stale"}`), 0o644); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	if err := service.RefreshMapping(); err != nil {
		t.Fatalf("refresh mapping: %v", err)
	}

	mapping := readMappingFile(t, service.MappingFilePath())
	if len(mapping) != 1 || mapping["known-bucket"] != "/tempZone/home/test1" {
		t.Fatalf("expected exact-search fallback mapping, got %+v", mapping)
	}
	if !fs.wasSearched(AVUBucketAttribute, metadataValueWildcard) || !fs.wasSearched(AVUBucketAttribute, "known-bucket") {
		t.Fatalf("expected wildcard and known-name search calls, got %+v", fs.searches)
	}
}

func TestValidation(t *testing.T) {
	fs := newTestFilesystem()
	fs.addCollection("/tempZone/home")

	if _, err := NewS3Service(nil, Config{ScanRootPath: "/tempZone/home", MappingFilePath: "mapping.json"}); !errors.Is(err, ErrMissingFilesystem) {
		t.Fatalf("expected ErrMissingFilesystem, got %v", err)
	}
	if _, err := NewS3Service(fs, Config{ScanRootPath: "relative", MappingFilePath: "mapping.json"}); !errors.Is(err, ErrInvalidScanRoot) {
		t.Fatalf("expected ErrInvalidScanRoot, got %v", err)
	}
	if _, err := NewS3Service(fs, Config{ScanRootPath: "/tempZone/home"}); !errors.Is(err, ErrMissingMappingFile) {
		t.Fatalf("expected ErrMissingMappingFile, got %v", err)
	}
	if _, err := NewS3ServiceWithMappingFile(fs, "/tempZone/home", nil); !errors.Is(err, ErrMissingMappingFile) {
		t.Fatalf("expected ErrMissingMappingFile, got %v", err)
	}

	service := newTestService(t, fs)
	if _, err := service.AddBucket("relative", "valid-name"); !errors.Is(err, ErrInvalidIRODSPath) {
		t.Fatalf("expected ErrInvalidIRODSPath, got %v", err)
	}
	if _, err := service.AddBucket("/tempZone/home", "Bad_Name"); !errors.Is(err, ErrInvalidBucketName) {
		t.Fatalf("expected ErrInvalidBucketName, got %v", err)
	}
	if _, err := service.UpdateBucket("/tempZone/home", "new-bucket"); !errors.Is(err, ErrBucketNotFound) {
		t.Fatalf("expected ErrBucketNotFound, got %v", err)
	}
	if err := service.DeleteBucket("/tempZone/home"); !errors.Is(err, ErrBucketNotFound) {
		t.Fatalf("expected ErrBucketNotFound, got %v", err)
	}
}

func TestBucketNameValidation(t *testing.T) {
	validNames := []string{
		"abc",
		"bucket-1",
		"bucket.name",
		"a1-b2.c3",
	}
	for _, name := range validNames {
		if normalizeBucketName(name) == "" {
			t.Fatalf("expected %q to be valid", name)
		}
	}

	invalidNames := []string{
		"ab",
		"-bucket",
		"bucket-",
		"bucket..name",
		"bucket.-name",
		"bucket-.name",
		"192.168.1.1",
		"bad_name",
		strings.Repeat("a", 64),
	}
	for _, name := range invalidNames {
		if normalizeBucketName(name) != "" {
			t.Fatalf("expected %q to be invalid", name)
		}
	}
}

func newTestService(t *testing.T, fs *testFilesystem) *S3Service {
	t.Helper()

	service, err := NewS3Service(fs, Config{
		ScanRootPath:    "/tempZone/home",
		MappingFilePath: path.Join(t.TempDir(), "bucket-mapping.json"),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func readMappingFile(t *testing.T, mappingPath string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatalf("read mapping file: %v", err)
	}

	mapping := map[string]string{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		t.Fatalf("decode mapping file: %v", err)
	}
	return mapping
}

func bucketNames(buckets []Bucket) string {
	names := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		names = append(names, bucket.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

type testFilesystem struct {
	collections map[string]struct{}
	objects     map[string]struct{}
	metadata    map[string][]Metadata
	searches    []metadataSearch

	exactSearchOnly bool
}

func newTestFilesystem() *testFilesystem {
	return &testFilesystem{
		collections: map[string]struct{}{},
		objects:     map[string]struct{}{},
		metadata:    map[string][]Metadata{},
	}
}

func (fs *testFilesystem) addCollection(irodsPath string) {
	fs.collections[path.Clean(irodsPath)] = struct{}{}
}

func (fs *testFilesystem) CollectionExists(irodsPath string) (bool, error) {
	_, ok := fs.collections[path.Clean(irodsPath)]
	return ok, nil
}

func (fs *testFilesystem) SearchByMeta(metaName string, metaValue string) ([]Entry, error) {
	fs.searches = append(fs.searches, metadataSearch{Name: metaName, Value: metaValue})

	entries := make([]Entry, 0)
	for collectionPath := range fs.collections {
		if fs.metadataMatches(collectionPath, metaName, metaValue) {
			entries = append(entries, Entry{
				Path: collectionPath,
				Type: EntryTypeCollection,
			})
		}
	}
	for objectPath := range fs.objects {
		if fs.metadataMatches(objectPath, metaName, metaValue) {
			entries = append(entries, Entry{
				Path: objectPath,
				Type: EntryTypeFile,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func (fs *testFilesystem) metadataMatches(irodsPath string, metaName string, metaValue string) bool {
	for _, avu := range fs.metadata[path.Clean(irodsPath)] {
		if avu.Name != metaName {
			continue
		}
		if metaValue == metadataValueWildcard && !fs.exactSearchOnly {
			return true
		}
		if avu.Value == metaValue {
			return true
		}
	}
	return false
}

func (fs *testFilesystem) wasSearched(metaName string, metaValue string) bool {
	for _, search := range fs.searches {
		if search.Name == metaName && search.Value == metaValue {
			return true
		}
	}
	return false
}

func (fs *testFilesystem) ListCollectionMetadata(collectionPath string) ([]Metadata, error) {
	collectionPath = path.Clean(collectionPath)
	metadata := fs.metadata[collectionPath]
	result := make([]Metadata, len(metadata))
	copy(result, metadata)
	return result, nil
}

func (fs *testFilesystem) AddCollectionMetadata(collectionPath string, metadata Metadata) error {
	collectionPath = path.Clean(collectionPath)
	fs.metadata[collectionPath] = append(fs.metadata[collectionPath], metadata)
	return nil
}

func (fs *testFilesystem) DeleteCollectionMetadata(collectionPath string, metadata Metadata) error {
	collectionPath = path.Clean(collectionPath)
	current := fs.metadata[collectionPath]
	next := make([]Metadata, 0, len(current))
	removed := false
	for _, avu := range current {
		if !removed && avu == metadata {
			removed = true
			continue
		}
		next = append(next, avu)
	}
	fs.metadata[collectionPath] = next
	return nil
}

type metadataSearch struct {
	Name  string
	Value string
}
