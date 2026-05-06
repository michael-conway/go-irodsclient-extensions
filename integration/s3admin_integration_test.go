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
	irodscorefs "github.com/cyverse/go-irodsclient/irods/fs"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/s3admin"
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

	service, err := s3admin.NewS3ServiceWithMappingFile(&irodsS3AdminFilesystem{filesystem: filesystem}, fixtureRoot, mappingFile)
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

type irodsS3AdminFilesystem struct {
	filesystem *irodsfs.FileSystem
}

func (filesystem *irodsS3AdminFilesystem) CollectionExists(irodsPath string) (bool, error) {
	return filesystem.filesystem.ExistsDir(irodsPath), nil
}

func (filesystem *irodsS3AdminFilesystem) SearchByMeta(metaName string, metaValue string) ([]s3admin.Entry, error) {
	if strings.ContainsAny(metaValue, "%_") {
		return filesystem.searchCollectionsByMetaWildcard(metaName, metaValue)
	}

	entries, err := filesystem.filesystem.SearchByMeta(metaName, metaValue)
	if err != nil {
		return nil, err
	}

	result := make([]s3admin.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}

		entryType := s3admin.EntryTypeFile
		if entry.IsDir() {
			entryType = s3admin.EntryTypeDirectory
		}

		result = append(result, s3admin.Entry{
			Path: entry.Path,
			Type: entryType,
		})
	}
	return result, nil
}

func (filesystem *irodsS3AdminFilesystem) searchCollectionsByMetaWildcard(metaName string, metaValue string) ([]s3admin.Entry, error) {
	conn, err := filesystem.filesystem.GetMetadataConnection(true)
	if err != nil {
		return nil, err
	}
	defer filesystem.filesystem.ReturnMetadataConnection(conn) //nolint

	collections, err := irodscorefs.SearchCollectionsByMetaWildcard(conn, metaName, metaValue)
	if err != nil {
		return nil, err
	}

	entries := make([]s3admin.Entry, 0, len(collections))
	for _, collection := range collections {
		if collection == nil {
			continue
		}
		entries = append(entries, s3admin.Entry{
			Path: collection.Path,
			Type: s3admin.EntryTypeCollection,
		})
	}
	return entries, nil
}

func (filesystem *irodsS3AdminFilesystem) ListCollectionMetadata(collectionPath string) ([]s3admin.Metadata, error) {
	metadata, err := filesystem.filesystem.ListMetadata(collectionPath)
	if err != nil {
		return nil, err
	}

	result := make([]s3admin.Metadata, 0, len(metadata))
	for _, avu := range metadata {
		if avu == nil {
			continue
		}
		result = append(result, s3admin.Metadata{
			Name:  avu.Name,
			Value: avu.Value,
			Units: avu.Units,
		})
	}
	return result, nil
}

func (filesystem *irodsS3AdminFilesystem) AddCollectionMetadata(collectionPath string, metadata s3admin.Metadata) error {
	return filesystem.filesystem.AddMetadata(collectionPath, metadata.Name, metadata.Value, metadata.Units)
}

func (filesystem *irodsS3AdminFilesystem) DeleteCollectionMetadata(collectionPath string, metadata s3admin.Metadata) error {
	return filesystem.filesystem.DeleteMetadataByAVU(collectionPath, metadata.Name, metadata.Value, metadata.Units)
}
