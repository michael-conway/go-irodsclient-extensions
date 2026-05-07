package s3admin

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
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

func TestRebuildMappingFromAVUsDoesNotUseKnownMappingFallback(t *testing.T) {
	fs := newTestFilesystem()
	fs.exactSearchOnly = true
	fs.addCollection("/tempZone/home")
	fs.addCollection("/tempZone/home/test1")
	fs.metadata["/tempZone/home/test1"] = []Metadata{
		{Name: AVUBucketAttribute, Value: "known-bucket"},
	}

	service := newTestService(t, fs)
	if err := os.WriteFile(service.MappingFilePath(), []byte(`{"known-bucket":"/tempZone/home/test1"}`), 0o644); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	result, err := service.RebuildMappingFromAVUs()
	if err != nil {
		t.Fatalf("rebuild mapping: %v", err)
	}

	if result.MappingFilePath != service.MappingFilePath() {
		t.Fatalf("expected mapping file path %q, got %q", service.MappingFilePath(), result.MappingFilePath)
	}
	if len(result.Buckets) != 0 {
		t.Fatalf("expected no buckets without known-name fallback, got %+v", result.Buckets)
	}

	mapping := readMappingFile(t, service.MappingFilePath())
	if len(mapping) != 0 {
		t.Fatalf("expected rebuilt mapping to be empty, got %+v", mapping)
	}
	if !fs.wasSearched(AVUBucketAttribute, metadataValueWildcard) {
		t.Fatalf("expected wildcard search call, got %+v", fs.searches)
	}
	if fs.wasSearched(AVUBucketAttribute, "known-bucket") {
		t.Fatalf("did not expect known-name fallback search, got %+v", fs.searches)
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

func TestStoreUserSecretKeyWritesMarkerAndUserMappingFile(t *testing.T) {
	fs := newTestFilesystem()
	service := newUserMappingTestService(t, fs)
	account := testS3AdminAccount("test1")

	userMapping, err := service.StoreUserSecretKey(account, validS3UserSecretKey)
	if err != nil {
		t.Fatalf("store user secret key: %v", err)
	}

	expectedSecretPath := "/tempZone/home/test1/.irodsext/s3admin/irods-s3-api-secret.txt"
	if userMapping.UserID != "test1" || userMapping.Username != "test1" || userMapping.SecretKey != validS3UserSecretKey || userMapping.IRODSPath != expectedSecretPath {
		t.Fatalf("unexpected user mapping %+v", userMapping)
	}
	if got := string(fs.files[expectedSecretPath]); got != validS3UserSecretKey {
		t.Fatalf("expected stored secret %q, got %q", validS3UserSecretKey, got)
	}
	metadata := fs.metadata[expectedSecretPath]
	if len(metadata) != 1 || metadata[0].Name != AVUSecretAttribute || metadata[0].Value != "test1" {
		t.Fatalf("expected secret marker AVU for test1, got %+v", metadata)
	}

	mapping := readUserMappingFile(t, service.UserMappingFilePath())
	entry := mapping["test1"]
	if entry.SecretKey != validS3UserSecretKey || entry.Username != "test1" {
		t.Fatalf("expected test1 mapping, got %+v in %+v", entry, mapping)
	}
}

func TestUpdateUserSecretKeyReplacesSecretMarkerAndMapping(t *testing.T) {
	fs := newTestFilesystem()
	service := newUserMappingTestService(t, fs)
	account := testS3AdminAccount("test1")
	updatedKey := strings.Repeat("Z", S3UserSecretKeyLength)

	if _, err := service.StoreUserSecretKey(account, validS3UserSecretKey); err != nil {
		t.Fatalf("store initial key: %v", err)
	}
	if _, err := service.UpdateUserSecretKey(account, updatedKey); err != nil {
		t.Fatalf("update key: %v", err)
	}

	secretPath := "/tempZone/home/test1/.irodsext/s3admin/irods-s3-api-secret.txt"
	if got := string(fs.files[secretPath]); got != updatedKey {
		t.Fatalf("expected updated secret %q, got %q", updatedKey, got)
	}
	metadata := fs.metadata[secretPath]
	if len(metadata) != 1 || metadata[0].Name != AVUSecretAttribute || metadata[0].Value != "test1" {
		t.Fatalf("expected single replacement marker AVU, got %+v", metadata)
	}

	mapping := readUserMappingFile(t, service.UserMappingFilePath())
	entry := mapping["test1"]
	if entry.SecretKey != updatedKey || entry.Username != "test1" {
		t.Fatalf("expected updated mapping, got %+v in %+v", entry, mapping)
	}
}

func TestGenerateAndDeleteUserSecretKeyUpdatesUserMappingFile(t *testing.T) {
	fs := newTestFilesystem()
	service := newUserMappingTestService(t, fs)
	account := testS3AdminAccount("test1")

	userMapping, err := service.GenerateAndStoreUserSecretKey(account)
	if err != nil {
		t.Fatalf("generate and store key: %v", err)
	}
	if err := ValidateS3UserSecretKey(userMapping.SecretKey); err != nil {
		t.Fatalf("generated key invalid: %v", err)
	}

	mapping := readUserMappingFile(t, service.UserMappingFilePath())
	if mapping["test1"].SecretKey != userMapping.SecretKey {
		t.Fatalf("expected generated key in mapping, got %+v", mapping)
	}

	if err := service.DeleteUserSecretKey(account); err != nil {
		t.Fatalf("delete key: %v", err)
	}

	mapping = readUserMappingFile(t, service.UserMappingFilePath())
	if _, ok := mapping["test1"]; ok {
		t.Fatalf("expected deleted user to be removed from mapping, got %+v", mapping)
	}
	if _, ok := fs.files[userMapping.IRODSPath]; ok {
		t.Fatalf("expected secret file to be deleted")
	}
}

func TestRebuildUserMappingFromAVUsUsesMarkedSecretsAsSourceOfTruth(t *testing.T) {
	fs := newTestFilesystem()
	service := newUserMappingTestService(t, fs)
	keyService, err := NewS3UserKeyService(fs)
	if err != nil {
		t.Fatalf("new user key service: %v", err)
	}

	if _, err := keyService.StoreS3UserKey(testS3AdminAccount("test1"), validS3UserSecretKey); err != nil {
		t.Fatalf("store test1 key: %v", err)
	}
	test2Key := strings.Repeat("Y", S3UserSecretKeyLength)
	if _, err := keyService.StoreS3UserKey(testS3AdminAccount("test2"), test2Key); err != nil {
		t.Fatalf("store test2 key: %v", err)
	}
	if err := os.WriteFile(service.UserMappingFilePath(), []byte(`{"stale":{"secret_key":"stale","username":"stale"}}`), 0o644); err != nil {
		t.Fatalf("write stale user mapping: %v", err)
	}

	result, err := service.RebuildUserMappingFromAVUs()
	if err != nil {
		t.Fatalf("rebuild user mapping: %v", err)
	}
	if result.MappingFilePath != service.UserMappingFilePath() {
		t.Fatalf("expected mapping file path %q, got %q", service.UserMappingFilePath(), result.MappingFilePath)
	}
	if len(result.Users) != 2 {
		t.Fatalf("expected 2 rebuilt users, got %+v", result.Users)
	}

	mapping := readUserMappingFile(t, service.UserMappingFilePath())
	if len(mapping) != 2 || mapping["test1"].SecretKey != validS3UserSecretKey || mapping["test2"].SecretKey != test2Key {
		t.Fatalf("expected rebuilt user mapping, got %+v", mapping)
	}
	if !fs.wasSearched(AVUSecretAttribute, metadataValueWildcard) {
		t.Fatalf("expected marker metadata search, got %+v", fs.searches)
	}
}

func TestRebuildUserMappingRejectsDuplicateMarkedUser(t *testing.T) {
	fs := newTestFilesystem()
	service := newUserMappingTestService(t, fs)
	firstPath := "/tempZone/home/test1/.irodsext/s3admin/irods-s3-api-secret.txt"
	secondPath := "/tempZone/home/test1/other/.irodsext/s3admin/irods-s3-api-secret.txt"
	fs.objects[firstPath] = struct{}{}
	fs.objects[secondPath] = struct{}{}
	fs.files[firstPath] = []byte(validS3UserSecretKey)
	fs.files[secondPath] = []byte(strings.Repeat("Y", S3UserSecretKeyLength))
	fs.metadata[firstPath] = []Metadata{{Name: AVUSecretAttribute, Value: "test1"}}
	fs.metadata[secondPath] = []Metadata{{Name: AVUSecretAttribute, Value: "test1"}}

	_, err := service.RebuildUserMappingFromAVUs()
	if !errors.Is(err, ErrDuplicateUserMapping) {
		t.Fatalf("expected duplicate user mapping error, got %v", err)
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

func newUserMappingTestService(t *testing.T, fs *testFilesystem) *S3UserMappingService {
	t.Helper()

	service, err := NewS3UserMappingService(fs, path.Join(t.TempDir(), "user-mapping.json"))
	if err != nil {
		t.Fatalf("new user mapping service: %v", err)
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

func readUserMappingFile(t *testing.T, mappingPath string) map[string]UserMappingEntry {
	t.Helper()

	data, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatalf("read user mapping file: %v", err)
	}

	mapping := map[string]UserMappingEntry{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		t.Fatalf("decode user mapping file: %v", err)
	}
	return mapping
}

func testS3AdminAccount(username string) *irodstypes.IRODSAccount {
	return &irodstypes.IRODSAccount{
		ClientUser: username,
		ClientZone: "tempZone",
	}
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
	files       map[string][]byte
	metadata    map[string][]Metadata
	searches    []metadataSearch

	exactSearchOnly bool
}

func newTestFilesystem() *testFilesystem {
	return &testFilesystem{
		collections: map[string]struct{}{},
		objects:     map[string]struct{}{},
		files:       map[string][]byte{},
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

func (fs *testFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	if fs.collections == nil {
		fs.collections = map[string]struct{}{}
	}
	fs.collections[path.Clean(irodsPath)] = struct{}{}
	return nil
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

func (fs *testFilesystem) ReadDataObject(dataObjectPath string) ([]byte, error) {
	dataObjectPath = path.Clean(dataObjectPath)
	contents, ok := fs.files[dataObjectPath]
	if !ok {
		return nil, errors.New("data object not found")
	}
	return append([]byte(nil), contents...), nil
}

func (fs *testFilesystem) WriteDataObject(dataObjectPath string, contents []byte) error {
	dataObjectPath = path.Clean(dataObjectPath)
	if fs.objects == nil {
		fs.objects = map[string]struct{}{}
	}
	if fs.files == nil {
		fs.files = map[string][]byte{}
	}
	fs.objects[dataObjectPath] = struct{}{}
	fs.files[dataObjectPath] = append([]byte(nil), contents...)
	return nil
}

func (fs *testFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	dataObjectPath = path.Clean(dataObjectPath)
	if _, ok := fs.objects[dataObjectPath]; !ok {
		return errors.New("data object not found")
	}
	delete(fs.objects, dataObjectPath)
	delete(fs.files, dataObjectPath)
	delete(fs.metadata, dataObjectPath)
	return nil
}

func (fs *testFilesystem) ListDataObjectMetadata(dataObjectPath string) ([]Metadata, error) {
	dataObjectPath = path.Clean(dataObjectPath)
	metadata := fs.metadata[dataObjectPath]
	result := make([]Metadata, len(metadata))
	copy(result, metadata)
	return result, nil
}

func (fs *testFilesystem) AddDataObjectMetadata(dataObjectPath string, metadata Metadata) error {
	dataObjectPath = path.Clean(dataObjectPath)
	fs.metadata[dataObjectPath] = append(fs.metadata[dataObjectPath], metadata)
	return nil
}

func (fs *testFilesystem) DeleteDataObjectMetadata(dataObjectPath string, metadata Metadata) error {
	dataObjectPath = path.Clean(dataObjectPath)
	current := fs.metadata[dataObjectPath]
	next := make([]Metadata, 0, len(current))
	removed := false
	for _, avu := range current {
		if !removed && avu == metadata {
			removed = true
			continue
		}
		next = append(next, avu)
	}
	fs.metadata[dataObjectPath] = next
	return nil
}

type metadataSearch struct {
	Name  string
	Value string
}
