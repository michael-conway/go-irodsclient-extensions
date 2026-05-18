//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/s3admin"
	s3adminirodsfs "github.com/michael-conway/go-irodsclient-extensions/s3admin/irodsfs"
	"github.com/rs/xid"
)

func TestS3AdminBucketLifecycleIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixtureRoot := path.Join(homePath, ".goext-s3admin-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureRoot, true, true)
	})

	bucketAPath := path.Join(fixtureRoot, "alpha")
	bucketBPath := path.Join(fixtureRoot, "bravo")
	bucketCPath := path.Join(fixtureRoot, "nested", "charlie")
	for _, collectionPath := range []string{bucketAPath, bucketBPath, bucketCPath} {
		if err := filesystem.MakeDir(collectionPath, true); err != nil {
			t.Fatalf("create bucket fixture collection %q: %v", collectionPath, err)
		}
		createS3AdminDummyFile(t, filesystem, path.Join(collectionPath, "dummy.txt"))
	}

	mappingFile, err := s3admin.NewMappingFile(path.Join(t.TempDir(), "bucket-mapping.json"))
	if err != nil {
		t.Fatalf("create mapping file reference: %v", err)
	}

	service, err := s3admin.NewS3ServiceWithMappingFile(s3adminirodsfs.NewAdapter(filesystem), fixtureRoot, mappingFile)
	if err != nil {
		t.Fatalf("create s3admin service: %v", err)
	}

	bucketAName := "it-s3-alpha-" + xid.New().String()
	bucketBName := "it-s3-bravo-" + xid.New().String()
	bucketCName := "it-s3-charlie-" + xid.New().String()

	if _, err := service.AddBucket(bucketAPath, bucketAName); err != nil {
		t.Fatalf("add bucket A: %v", err)
	}
	if _, err := service.AddBucket(bucketBPath, bucketBName); err != nil {
		t.Fatalf("add bucket B: %v", err)
	}
	if _, err := service.AddBucket(bucketCPath, bucketCName); err != nil {
		t.Fatalf("add bucket C: %v", err)
	}

	assertS3AdminMapping(t, mappingFile.Path(), map[string]string{
		bucketAName: bucketAPath,
		bucketBName: bucketBPath,
		bucketCName: bucketCPath,
	})

	listedBuckets, err := service.ListBuckets(s3admin.ListOptions{
		IRODSPath: fixtureRoot,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("list buckets with wildcard metadata search: %v", err)
	}
	assertS3AdminBuckets(t, listedBuckets, map[string]string{
		bucketAName: bucketAPath,
		bucketBName: bucketBPath,
		bucketCName: bucketCPath,
	})

	searchedBuckets, err := service.SearchBuckets(bucketBName, s3admin.ListOptions{
		IRODSPath: fixtureRoot,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("search bucket by name: %v", err)
	}
	assertS3AdminBuckets(t, searchedBuckets, map[string]string{
		bucketBName: bucketBPath,
	})

	if _, err := service.AddBucket(bucketCPath, bucketAName); !errors.Is(err, s3admin.ErrDuplicateBucket) {
		t.Fatalf("expected duplicate bucket error, got %v", err)
	}

	if err := service.DeleteBucket(bucketBPath); err != nil {
		t.Fatalf("delete bucket B: %v", err)
	}

	assertS3AdminMapping(t, mappingFile.Path(), map[string]string{
		bucketAName: bucketAPath,
		bucketCName: bucketCPath,
	})

	listedAfterDelete, err := service.ListBuckets(s3admin.ListOptions{
		IRODSPath: fixtureRoot,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("list buckets after delete: %v", err)
	}
	assertS3AdminBuckets(t, listedAfterDelete, map[string]string{
		bucketAName: bucketAPath,
		bucketCName: bucketCPath,
	})
}

func TestS3AdminUserKeyLifecycleIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()
	adminFilesystem := testutil.NewIntegrationAdminFilesystem(t)
	defer adminFilesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixtureHome := path.Join(homePath, ".goext-s3admin-user-key-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureHome, true); err != nil {
		t.Fatalf("create fixture user home %q: %v", fixtureHome, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureHome, true, true)
	})

	adapter := s3adminirodsfs.NewAdapterWithProxyAccount(filesystem, adminFilesystem.GetAccount(), "go-irodsclient-extensions-s3admin-user-key-integration")
	scopedAdapter := &scopedS3AdminUserMappingFilesystem{
		Adapter: adapter,
		root:    fixtureHome,
	}
	keyService, err := s3admin.NewS3UserKeyService(adapter)
	if err != nil {
		t.Fatalf("create s3 user key service: %v", err)
	}
	mappingService, err := s3admin.NewS3UserMappingService(scopedAdapter, path.Join(t.TempDir(), "irods-s3-user-mapping.json"))
	if err != nil {
		t.Fatalf("create s3 user mapping service: %v", err)
	}
	userID := testutil.IntegrationPrimaryTestUser(t)

	expectedSecretPath := path.Join(fixtureHome, ".irodsext", s3admin.S3UserKeyContext, s3admin.S3UserKeyFileName)
	ensuredPath, err := keyService.EnsureS3UserKeyStructureForHome(fixtureHome)
	if err != nil {
		t.Fatalf("ensure s3 user key structure: %v", err)
	}
	if ensuredPath != expectedSecretPath {
		t.Fatalf("expected secret path %q, got %q", expectedSecretPath, ensuredPath)
	}
	if !filesystem.ExistsDir(path.Join(fixtureHome, ".irodsext")) {
		t.Fatalf("expected userpersist root collection to exist")
	}
	if !filesystem.ExistsDir(path.Join(fixtureHome, ".irodsext", s3admin.S3UserKeyContext)) {
		t.Fatalf("expected s3admin user key collection to exist")
	}

	initialKey := "Aa1Bb~2Cc3.-Dd4Ee5Ff6Gg7Hh8Ii9_Jj0Kk1Ll2"
	stored, err := mappingService.StoreUserSecretKeyForHomeAndUser(fixtureHome, userID, initialKey)
	if err != nil {
		t.Fatalf("store s3 user key: %v", err)
	}
	if stored.IRODSPath != expectedSecretPath {
		t.Fatalf("expected stored path %q, got %q", expectedSecretPath, stored.IRODSPath)
	}
	if stored.SecretKey != initialKey {
		t.Fatalf("expected stored secret key %q, got %q", initialKey, stored.SecretKey)
	}
	userMapping := readS3AdminUserMapping(t, mappingService.UserMappingFilePath())
	if userMapping[userID].SecretKey != initialKey || userMapping[userID].Username != userID {
		t.Fatalf("expected stored key in user mapping file, got %+v", userMapping)
	}

	retrieved, err := mappingService.GetUserSecretKeyForHome(fixtureHome, userID)
	if err != nil {
		t.Fatalf("retrieve s3 user key: %v", err)
	}
	if retrieved.SecretKey != initialKey {
		t.Fatalf("expected retrieved secret key %q, got %q", initialKey, retrieved.SecretKey)
	}

	updatedKey := strings.Repeat("Z", s3admin.S3UserSecretKeyLength)
	updated, err := mappingService.UpdateUserSecretKeyForHomeAndUser(fixtureHome, userID, updatedKey)
	if err != nil {
		t.Fatalf("update s3 user key: %v", err)
	}
	if updated.SecretKey != updatedKey {
		t.Fatalf("expected updated secret key %q, got %q", updatedKey, updated.SecretKey)
	}
	userMapping = readS3AdminUserMapping(t, mappingService.UserMappingFilePath())
	if userMapping[userID].SecretKey != updatedKey || userMapping[userID].Username != userID {
		t.Fatalf("expected updated key in user mapping file, got %+v", userMapping)
	}

	if _, err := mappingService.StoreUserSecretKeyForHomeAndUser(fixtureHome, userID, "short"); !errors.Is(err, s3admin.ErrInvalidUserSecretKey) {
		t.Fatalf("expected invalid user secret key error, got %v", err)
	}

	retrievedAfterInvalid, err := mappingService.GetUserSecretKeyForHome(fixtureHome, userID)
	if err != nil {
		t.Fatalf("retrieve s3 user key after invalid store: %v", err)
	}
	if retrievedAfterInvalid.SecretKey != updatedKey {
		t.Fatalf("expected invalid store to leave key %q, got %q", updatedKey, retrievedAfterInvalid.SecretKey)
	}

	generated, err := mappingService.GenerateAndStoreUserSecretKeyForHomeAndUser(fixtureHome, userID)
	if err != nil {
		t.Fatalf("generate and store s3 user key: %v", err)
	}
	if err := s3admin.ValidateS3UserSecretKey(generated.SecretKey); err != nil {
		t.Fatalf("generated secret key was invalid: %v", err)
	}

	retrievedGenerated, err := mappingService.GetUserSecretKeyForHome(fixtureHome, userID)
	if err != nil {
		t.Fatalf("retrieve generated s3 user key: %v", err)
	}
	if retrievedGenerated.SecretKey != generated.SecretKey {
		t.Fatalf("expected generated key %q, got %q", generated.SecretKey, retrievedGenerated.SecretKey)
	}
	rebuildResult, err := mappingService.RebuildUserMappingFromAVUs()
	if err != nil {
		t.Fatalf("rebuild user mapping from AVUs: %v", err)
	}
	rebuiltByUserID := map[string]s3admin.S3UserMapping{}
	for _, user := range rebuildResult.Users {
		rebuiltByUserID[user.UserID] = user
	}
	if rebuiltByUserID[userID].SecretKey != generated.SecretKey {
		t.Fatalf("expected rebuilt user mapping for %q, got %+v", userID, rebuildResult.Users)
	}

	if err := mappingService.DeleteUserSecretKeyForHomeAndUser(fixtureHome, userID); err != nil {
		t.Fatalf("delete s3 user key: %v", err)
	}
	if filesystem.ExistsFile(expectedSecretPath) {
		t.Fatalf("expected deleted secret key file %q to be absent", expectedSecretPath)
	}
	userMapping = readS3AdminUserMapping(t, mappingService.UserMappingFilePath())
	if _, ok := userMapping[userID]; ok {
		t.Fatalf("expected deleted key to be removed from user mapping, got %+v", userMapping)
	}
}

func createS3AdminDummyFile(t *testing.T, filesystem *irodsfs.FileSystem, irodsPath string) {
	t.Helper()

	handle, err := filesystem.CreateFile(irodsPath, "", "w")
	if err != nil {
		t.Fatalf("create dummy file %q: %v", irodsPath, err)
	}
	if _, err := handle.Write([]byte("s3admin integration fixture\n")); err != nil {
		_ = handle.Close()
		t.Fatalf("write dummy file %q: %v", irodsPath, err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close dummy file %q: %v", irodsPath, err)
	}
}

func assertS3AdminMapping(t *testing.T, mappingPath string, expected map[string]string) {
	t.Helper()

	content, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatalf("read bucket mapping file %q: %v", mappingPath, err)
	}

	actual := map[string]string{}
	if err := json.Unmarshal(content, &actual); err != nil {
		t.Fatalf("decode bucket mapping file %q: %v", mappingPath, err)
	}

	if len(actual) != len(expected) {
		t.Fatalf("expected mapping %s, got %s", formatS3AdminMapping(expected), formatS3AdminMapping(actual))
	}
	for bucketName, expectedPath := range expected {
		if actual[bucketName] != expectedPath {
			t.Fatalf("expected mapping %q -> %q, got %q in %s", bucketName, expectedPath, actual[bucketName], formatS3AdminMapping(actual))
		}
	}
}

func readS3AdminUserMapping(t *testing.T, mappingPath string) map[string]s3admin.UserMappingEntry {
	t.Helper()

	content, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatalf("read user mapping file %q: %v", mappingPath, err)
	}

	actual := map[string]s3admin.UserMappingEntry{}
	if err := json.Unmarshal(content, &actual); err != nil {
		t.Fatalf("decode user mapping file %q: %v", mappingPath, err)
	}
	return actual
}

type scopedS3AdminUserMappingFilesystem struct {
	*s3adminirodsfs.Adapter
	root string
}

func (filesystem *scopedS3AdminUserMappingFilesystem) QueryDataObjectMetadata(metaName string, metaValue string) ([]s3admin.DataObjectMetadataMatch, error) {
	matches, err := filesystem.Adapter.QueryDataObjectMetadata(metaName, metaValue)
	if err != nil {
		return nil, err
	}

	root := normalizeIntegrationIRODSPath(filesystem.root)
	filtered := make([]s3admin.DataObjectMetadataMatch, 0, len(matches))
	for _, match := range matches {
		if integrationPathWithinScope(match.IRODSPath, root) {
			filtered = append(filtered, match)
		}
	}
	return filtered, nil
}

func integrationPathWithinScope(candidatePath string, root string) bool {
	candidatePath = normalizeIntegrationIRODSPath(candidatePath)
	root = normalizeIntegrationIRODSPath(root)
	if candidatePath == "" || root == "" {
		return false
	}
	return candidatePath == root || strings.HasPrefix(candidatePath, root+"/")
}

func normalizeIntegrationIRODSPath(irodsPath string) string {
	irodsPath = strings.TrimSpace(irodsPath)
	if irodsPath == "" || !strings.HasPrefix(irodsPath, "/") {
		return ""
	}
	return path.Clean(irodsPath)
}

func assertS3AdminBuckets(t *testing.T, buckets []s3admin.Bucket, expected map[string]string) {
	t.Helper()

	actual := map[string]string{}
	for _, bucket := range buckets {
		actual[bucket.Name] = bucket.IRODSPath
	}

	if len(actual) != len(expected) {
		t.Fatalf("expected buckets %s, got %s", formatS3AdminMapping(expected), formatS3AdminMapping(actual))
	}
	for bucketName, expectedPath := range expected {
		if actual[bucketName] != expectedPath {
			t.Fatalf("expected bucket %q at %q, got %q in %s", bucketName, expectedPath, actual[bucketName], formatS3AdminMapping(actual))
		}
	}
}

func formatS3AdminMapping(mapping map[string]string) string {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+mapping[key])
	}
	return strings.Join(parts, ",")
}
