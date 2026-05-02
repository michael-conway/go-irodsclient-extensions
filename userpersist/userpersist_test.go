package userpersist

import (
	"errors"
	"testing"
)

func TestRootPath(t *testing.T) {
	rootPath, err := RootPath("/tempZone/home/test1")
	if err != nil {
		t.Fatalf("root path: %v", err)
	}

	if rootPath != "/tempZone/home/test1/.irodsext" {
		t.Fatalf("expected root path /tempZone/home/test1/.irodsext, got %q", rootPath)
	}
}

func TestRootPathRejectsRelativeHome(t *testing.T) {
	if _, err := RootPath("tempZone/home/test1"); !errors.Is(err, ErrInvalidUserHome) {
		t.Fatalf("expected ErrInvalidUserHome, got %v", err)
	}
}

func TestCategoryPath(t *testing.T) {
	categoryPath, err := CategoryPath("/tempZone/home/test1", "filecarts")
	if err != nil {
		t.Fatalf("category path: %v", err)
	}

	if categoryPath != "/tempZone/home/test1/.irodsext/filecarts" {
		t.Fatalf("unexpected category path %q", categoryPath)
	}
}

func TestCategoryPathRejectsNestedCategory(t *testing.T) {
	if _, err := CategoryPath("/tempZone/home/test1", "x/y"); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
}

func TestEnsureRootCollectionCreatesMissingCollection(t *testing.T) {
	fs := &testCollectionFilesystem{
		collections: map[string]struct{}{},
	}

	rootPath, err := EnsureRootCollection(fs, "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("ensure root collection: %v", err)
	}

	if rootPath != "/tempZone/home/test1/.irodsext" {
		t.Fatalf("unexpected root path %q", rootPath)
	}
	if fs.createCalls != 1 {
		t.Fatalf("expected one create call, got %d", fs.createCalls)
	}
}

func TestEnsureRootCollectionNoopIfExists(t *testing.T) {
	fs := &testCollectionFilesystem{
		collections: map[string]struct{}{
			"/tempZone/home/test1/.irodsext": {},
		},
	}

	if _, err := EnsureRootCollection(fs, "/tempZone/home/test1"); err != nil {
		t.Fatalf("ensure root collection: %v", err)
	}

	if fs.createCalls != 0 {
		t.Fatalf("expected no create calls, got %d", fs.createCalls)
	}
}

func TestEnsureCategoryCollectionCreatesRootAndCategory(t *testing.T) {
	fs := &testCollectionFilesystem{
		collections: map[string]struct{}{},
	}

	categoryPath, err := EnsureCategoryCollection(fs, "/tempZone/home/test1", "filecarts")
	if err != nil {
		t.Fatalf("ensure category collection: %v", err)
	}

	if categoryPath != "/tempZone/home/test1/.irodsext/filecarts" {
		t.Fatalf("unexpected category path %q", categoryPath)
	}
	if fs.createCalls != 2 {
		t.Fatalf("expected two create calls, got %d", fs.createCalls)
	}
}

type testCollectionFilesystem struct {
	collections map[string]struct{}
	createCalls int
	existsErr   error
	createErr   error
}

func (fs *testCollectionFilesystem) CollectionExists(irodsPath string) (bool, error) {
	if fs.existsErr != nil {
		return false, fs.existsErr
	}

	_, ok := fs.collections[irodsPath]
	return ok, nil
}

func (fs *testCollectionFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	if fs.createErr != nil {
		return fs.createErr
	}
	if fs.collections == nil {
		fs.collections = map[string]struct{}{}
	}
	fs.collections[irodsPath] = struct{}{}
	fs.createCalls++
	return nil
}
