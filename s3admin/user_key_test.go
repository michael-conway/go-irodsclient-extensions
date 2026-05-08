package s3admin

import (
	"errors"
	"path"
	"strings"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

const validS3UserSecretKey = "Aa1Bb~2Cc3.-Dd4Ee5Ff6Gg7Hh8Ii9_Jj0Kk1Ll2"

func TestS3UserKeyPath(t *testing.T) {
	secretPath, err := S3UserKeyPath(testS3UserAccount())
	if err != nil {
		t.Fatalf("s3 user key path: %v", err)
	}

	if secretPath != "/tempZone/home/test1/.irodsext/s3admin/irods-s3-api-secret.txt" {
		t.Fatalf("unexpected secret path %q", secretPath)
	}
}

func TestStoreRetrieveAndDeleteS3UserKey(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)

	stored, err := service.StoreS3UserKey(testS3UserAccount(), validS3UserSecretKey)
	if err != nil {
		t.Fatalf("store s3 user key: %v", err)
	}

	expectedPath := "/tempZone/home/test1/.irodsext/s3admin/irods-s3-api-secret.txt"
	if stored.IRODSPath != expectedPath {
		t.Fatalf("expected secret path %q, got %q", expectedPath, stored.IRODSPath)
	}
	if stored.UserName != "test1" || stored.Zone != "tempZone" {
		t.Fatalf("unexpected user identity in result: %#v", stored)
	}
	if got := string(fs.files[expectedPath]); got != validS3UserSecretKey {
		t.Fatalf("expected stored key %q, got %q", validS3UserSecretKey, got)
	}
	metadata := fs.metadata[expectedPath]
	if len(metadata) != 1 || metadata[0].Name != AVUSecretAttribute || metadata[0].Value != "test1" {
		t.Fatalf("expected secret marker AVU for test1, got %+v", metadata)
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext"]; !ok {
		t.Fatalf("expected userpersist root collection to be created")
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext/s3admin"]; !ok {
		t.Fatalf("expected s3admin collection to be created")
	}

	retrieved, err := service.GetS3UserKey(testS3UserAccount())
	if err != nil {
		t.Fatalf("get s3 user key: %v", err)
	}
	if retrieved.SecretKey != validS3UserSecretKey {
		t.Fatalf("expected retrieved key %q, got %q", validS3UserSecretKey, retrieved.SecretKey)
	}

	if err := service.DeleteS3UserKey(testS3UserAccount()); err != nil {
		t.Fatalf("delete s3 user key: %v", err)
	}
	if _, ok := fs.files[expectedPath]; ok {
		t.Fatalf("expected secret file to be deleted")
	}
	if !fs.lastDeleteForce {
		t.Fatalf("expected delete to force removal")
	}
}

func TestStoreS3UserKeyUpdatesExistingFile(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)
	updatedKey := strings.Repeat("Z", S3UserSecretKeyLength)

	if _, err := service.StoreS3UserKey(testS3UserAccount(), validS3UserSecretKey); err != nil {
		t.Fatalf("store first key: %v", err)
	}
	if _, err := service.StoreS3UserKey(testS3UserAccount(), updatedKey); err != nil {
		t.Fatalf("store updated key: %v", err)
	}

	secretPath, err := S3UserKeyPath(testS3UserAccount())
	if err != nil {
		t.Fatalf("s3 user key path: %v", err)
	}
	if got := string(fs.files[secretPath]); got != updatedKey {
		t.Fatalf("expected updated key %q, got %q", updatedKey, got)
	}
	metadata := fs.metadata[secretPath]
	if len(metadata) != 1 || metadata[0].Name != AVUSecretAttribute || metadata[0].Value != "test1" {
		t.Fatalf("expected single replacement secret marker AVU for test1, got %+v", metadata)
	}
	if fs.createCalls != 2 {
		t.Fatalf("expected only root and category create calls, got %d", fs.createCalls)
	}
}

func TestStoreS3UserKeyRejectsInvalidKeyWithoutCreatingStructure(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)

	invalidKeys := []string{
		"short",
		strings.Repeat("a", S3UserSecretKeyLength-1),
		strings.Repeat("*", S3UserSecretKeyLength),
		strings.Repeat(" ", S3UserSecretKeyLength),
	}

	for _, invalidKey := range invalidKeys {
		if _, err := service.StoreS3UserKey(testS3UserAccount(), invalidKey); !errors.Is(err, ErrInvalidUserSecretKey) {
			t.Fatalf("expected ErrInvalidUserSecretKey for %q, got %v", invalidKey, err)
		}
	}

	if fs.createCalls != 0 {
		t.Fatalf("expected no collections to be created, got %d create calls", fs.createCalls)
	}
	if len(fs.files) != 0 {
		t.Fatalf("expected no files to be written, got %d", len(fs.files))
	}
}

func TestGenerateAndStoreS3UserKey(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)

	stored, err := service.GenerateAndStoreS3UserKey(testS3UserAccount())
	if err != nil {
		t.Fatalf("generate and store s3 user key: %v", err)
	}

	if err := ValidateS3UserSecretKey(stored.SecretKey); err != nil {
		t.Fatalf("generated key was invalid: %v", err)
	}
	if got := string(fs.files[stored.IRODSPath]); got != stored.SecretKey {
		t.Fatalf("expected stored generated key %q, got %q", stored.SecretKey, got)
	}
}

func TestEnsureS3UserKeyStructureForHomeIsIdempotent(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)

	firstPath, err := service.EnsureS3UserKeyStructureForHome("/tempZone/home/test1")
	if err != nil {
		t.Fatalf("ensure first structure: %v", err)
	}
	secondPath, err := service.EnsureS3UserKeyStructureForHome("/tempZone/home/test1")
	if err != nil {
		t.Fatalf("ensure second structure: %v", err)
	}

	if firstPath != secondPath {
		t.Fatalf("expected idempotent path %q, got %q", firstPath, secondPath)
	}
	if fs.createCalls != 2 {
		t.Fatalf("expected only root and category create calls, got %d", fs.createCalls)
	}
}

func TestGetS3UserKeyTrimsStoredFileWhitespace(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)
	secretPath, err := S3UserKeyPath(testS3UserAccount())
	if err != nil {
		t.Fatalf("s3 user key path: %v", err)
	}
	fs.files[secretPath] = []byte(validS3UserSecretKey + "\n")
	fs.metadata[secretPath] = []Metadata{{Name: AVUSecretAttribute, Value: "test1"}}

	retrieved, err := service.GetS3UserKey(testS3UserAccount())
	if err != nil {
		t.Fatalf("get s3 user key: %v", err)
	}
	if retrieved.SecretKey != validS3UserSecretKey {
		t.Fatalf("expected retrieved key %q, got %q", validS3UserSecretKey, retrieved.SecretKey)
	}
}

func TestGetS3UserKeyAtPathReadsAsMarkerUser(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)
	secretPath := "/tempZone/home/test1/.irodsext/s3admin/irods-s3-api-secret.txt"
	fs.files[secretPath] = []byte(validS3UserSecretKey)
	fs.metadata[secretPath] = []Metadata{{Name: AVUSecretAttribute, Value: "test1"}}

	retrieved, err := service.GetS3UserKeyAtPath(secretPath)
	if err != nil {
		t.Fatalf("get s3 user key at path: %v", err)
	}
	if retrieved.SecretKey != validS3UserSecretKey {
		t.Fatalf("expected retrieved key %q, got %q", validS3UserSecretKey, retrieved.SecretKey)
	}
	if len(fs.readAsUserCalls) != 1 {
		t.Fatalf("expected one read-as-user call, got %d", len(fs.readAsUserCalls))
	}
	call := fs.readAsUserCalls[0]
	if call.path != secretPath || call.userID != "test1" {
		t.Fatalf("expected read-as-user for %q as test1, got %+v", secretPath, call)
	}
}

func TestS3UserKeyServiceRejectsMissingAccount(t *testing.T) {
	fs := newTestUserKeyFilesystem()
	service := newTestUserKeyService(t, fs)

	if _, err := service.StoreS3UserKey(nil, validS3UserSecretKey); !errors.Is(err, ErrMissingAccount) {
		t.Fatalf("expected ErrMissingAccount, got %v", err)
	}
}

func newTestUserKeyService(t *testing.T, fs *testUserKeyFilesystem) *S3UserKeyService {
	t.Helper()

	service, err := NewS3UserKeyService(fs)
	if err != nil {
		t.Fatalf("new s3 user key service: %v", err)
	}
	return service
}

func testS3UserAccount() *irodstypes.IRODSAccount {
	return &irodstypes.IRODSAccount{
		ClientUser: "test1",
		ClientZone: "tempZone",
	}
}

type testUserKeyFilesystem struct {
	collections     map[string]struct{}
	files           map[string][]byte
	metadata        map[string][]Metadata
	createCalls     int
	lastDeleteForce bool
	readAsUserCalls []testReadAsUserCall
}

type testReadAsUserCall struct {
	path   string
	userID string
}

func newTestUserKeyFilesystem() *testUserKeyFilesystem {
	return &testUserKeyFilesystem{
		collections: map[string]struct{}{},
		files:       map[string][]byte{},
		metadata:    map[string][]Metadata{},
	}
}

func (fs *testUserKeyFilesystem) CollectionExists(irodsPath string) (bool, error) {
	_, ok := fs.collections[path.Clean(irodsPath)]
	return ok, nil
}

func (fs *testUserKeyFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	if fs.collections == nil {
		fs.collections = map[string]struct{}{}
	}
	fs.collections[path.Clean(irodsPath)] = struct{}{}
	fs.createCalls++
	return nil
}

func (fs *testUserKeyFilesystem) ReadDataObject(dataObjectPath string) ([]byte, error) {
	contents, ok := fs.files[path.Clean(dataObjectPath)]
	if !ok {
		return nil, errors.New("data object not found")
	}
	return append([]byte(nil), contents...), nil
}

func (fs *testUserKeyFilesystem) ReadDataObjectAsUser(dataObjectPath string, userID string) ([]byte, error) {
	fs.readAsUserCalls = append(fs.readAsUserCalls, testReadAsUserCall{
		path:   path.Clean(dataObjectPath),
		userID: userID,
	})
	return fs.ReadDataObject(dataObjectPath)
}

func (fs *testUserKeyFilesystem) WriteDataObject(dataObjectPath string, contents []byte) error {
	if fs.files == nil {
		fs.files = map[string][]byte{}
	}
	fs.files[path.Clean(dataObjectPath)] = append([]byte(nil), contents...)
	return nil
}

func (fs *testUserKeyFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	dataObjectPath = path.Clean(dataObjectPath)
	if _, ok := fs.files[dataObjectPath]; !ok {
		return errors.New("data object not found")
	}
	delete(fs.files, dataObjectPath)
	delete(fs.metadata, dataObjectPath)
	fs.lastDeleteForce = force
	return nil
}

func (fs *testUserKeyFilesystem) ListDataObjectMetadata(dataObjectPath string) ([]Metadata, error) {
	dataObjectPath = path.Clean(dataObjectPath)
	metadata := fs.metadata[dataObjectPath]
	result := make([]Metadata, len(metadata))
	copy(result, metadata)
	return result, nil
}

func (fs *testUserKeyFilesystem) AddDataObjectMetadata(dataObjectPath string, metadata Metadata) error {
	dataObjectPath = path.Clean(dataObjectPath)
	fs.metadata[dataObjectPath] = append(fs.metadata[dataObjectPath], metadata)
	return nil
}

func (fs *testUserKeyFilesystem) DeleteDataObjectMetadata(dataObjectPath string, metadata Metadata) error {
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
