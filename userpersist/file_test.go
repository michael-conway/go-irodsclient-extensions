package userpersist

import (
	"errors"
	"path"
	"testing"
)

func TestFileServiceAddOrUpdateStringCreatesContextAndWritesFile(t *testing.T) {
	fs := newTestFileFilesystem()
	service := newTestFileService(t, fs)

	file, err := service.AddOrUpdateString("/tempZone/home/test1", "s3admin", "secret.txt", "first")
	if err != nil {
		t.Fatalf("add or update string: %v", err)
	}

	expectedPath := "/tempZone/home/test1/.irodsext/s3admin/secret.txt"
	if file.IRODSPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, file.IRODSPath)
	}
	if string(file.Contents) != "first" {
		t.Fatalf("expected returned contents %q, got %q", "first", string(file.Contents))
	}
	if got := string(fs.files[expectedPath]); got != "first" {
		t.Fatalf("expected stored contents %q, got %q", "first", got)
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext"]; !ok {
		t.Fatalf("expected root collection to be created")
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext/s3admin"]; !ok {
		t.Fatalf("expected context collection to be created")
	}
}

func TestFileServiceAddOrUpdateStringUpdatesExistingFileIdempotently(t *testing.T) {
	fs := newTestFileFilesystem()
	service := newTestFileService(t, fs)

	if _, err := service.AddOrUpdateString("/tempZone/home/test1", "s3admin", "secret.txt", "first"); err != nil {
		t.Fatalf("add first string: %v", err)
	}
	file, err := service.AddOrUpdateString("/tempZone/home/test1", "s3admin", "secret.txt", "second")
	if err != nil {
		t.Fatalf("update string: %v", err)
	}

	if string(file.Contents) != "second" {
		t.Fatalf("expected returned contents %q, got %q", "second", string(file.Contents))
	}
	if got := string(fs.files[file.IRODSPath]); got != "second" {
		t.Fatalf("expected stored contents %q, got %q", "second", got)
	}
	if fs.createCalls != 2 {
		t.Fatalf("expected only root and context create calls, got %d", fs.createCalls)
	}
}

func TestFileServiceGetStringYieldsContents(t *testing.T) {
	fs := newTestFileFilesystem()
	service := newTestFileService(t, fs)
	filePath := "/tempZone/home/test1/.irodsext/s3admin/secret.txt"
	fs.files[filePath] = []byte("stored")

	contents, file, err := service.GetString("/tempZone/home/test1", "s3admin", "secret.txt")
	if err != nil {
		t.Fatalf("get string: %v", err)
	}

	if contents != "stored" {
		t.Fatalf("expected contents %q, got %q", "stored", contents)
	}
	if file.IRODSPath != filePath {
		t.Fatalf("expected path %q, got %q", filePath, file.IRODSPath)
	}
}

func TestFileServiceDeleteFile(t *testing.T) {
	fs := newTestFileFilesystem()
	service := newTestFileService(t, fs)
	filePath := "/tempZone/home/test1/.irodsext/s3admin/secret.txt"
	fs.files[filePath] = []byte("stored")

	if err := service.DeleteFile("/tempZone/home/test1", "s3admin", "secret.txt", true); err != nil {
		t.Fatalf("delete file: %v", err)
	}

	if _, ok := fs.files[filePath]; ok {
		t.Fatalf("expected file to be deleted")
	}
	if !fs.lastDeleteForce {
		t.Fatalf("expected forced delete")
	}
}

func TestFileServiceRejectsInvalidContextAndFileName(t *testing.T) {
	fs := newTestFileFilesystem()
	service := newTestFileService(t, fs)

	if _, err := service.AddOrUpdateString("/tempZone/home/test1", "nested/context", "secret.txt", "contents"); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if _, err := service.AddOrUpdateString("/tempZone/home/test1", "s3admin", "nested/secret.txt", "contents"); !errors.Is(err, ErrInvalidFileName) {
		t.Fatalf("expected ErrInvalidFileName, got %v", err)
	}
	if fs.createCalls != 0 {
		t.Fatalf("expected invalid requests to avoid creating collections, got %d create calls", fs.createCalls)
	}
}

func newTestFileService(t *testing.T, fs *testFileFilesystem) *FileService {
	t.Helper()

	service, err := NewFileService(fs)
	if err != nil {
		t.Fatalf("new file service: %v", err)
	}
	return service
}

type testFileFilesystem struct {
	collections     map[string]struct{}
	files           map[string][]byte
	createCalls     int
	lastDeleteForce bool
}

func newTestFileFilesystem() *testFileFilesystem {
	return &testFileFilesystem{
		collections: map[string]struct{}{},
		files:       map[string][]byte{},
	}
}

func (fs *testFileFilesystem) CollectionExists(irodsPath string) (bool, error) {
	_, ok := fs.collections[path.Clean(irodsPath)]
	return ok, nil
}

func (fs *testFileFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	if fs.collections == nil {
		fs.collections = map[string]struct{}{}
	}
	fs.collections[path.Clean(irodsPath)] = struct{}{}
	fs.createCalls++
	return nil
}

func (fs *testFileFilesystem) ReadDataObject(dataObjectPath string) ([]byte, error) {
	contents, ok := fs.files[path.Clean(dataObjectPath)]
	if !ok {
		return nil, errors.New("data object not found")
	}
	return append([]byte(nil), contents...), nil
}

func (fs *testFileFilesystem) WriteDataObject(dataObjectPath string, contents []byte) error {
	if fs.files == nil {
		fs.files = map[string][]byte{}
	}
	fs.files[path.Clean(dataObjectPath)] = append([]byte(nil), contents...)
	return nil
}

func (fs *testFileFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	dataObjectPath = path.Clean(dataObjectPath)
	if _, ok := fs.files[dataObjectPath]; !ok {
		return errors.New("data object not found")
	}
	delete(fs.files, dataObjectPath)
	fs.lastDeleteForce = force
	return nil
}
