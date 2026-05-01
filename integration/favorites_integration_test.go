//go:build integration
// +build integration

package integration

import (
	"path"
	"strings"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/michael-conway/go-irodsclient-extensions/favorites"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/rs/xid"
)

func TestFavoritesAddListRenameRemoveIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	favoritesFilesystem := &irodsFavoritesFilesystem{filesystem: filesystem}
	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	service, err := favorites.NewService(favoritesFilesystem, homePath)
	if err != nil {
		t.Fatalf("create favorites service: %v", err)
	}

	fixtureRoot := path.Join(homePath, ".goext-favorites-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureRoot, true, true)
	})

	filePath := path.Join(fixtureRoot, "alpha.txt")
	collectionPath := path.Join(fixtureRoot, "nested")
	createEmptyFile(t, filesystem, filePath)
	if err := filesystem.MakeDir(collectionPath, true); err != nil {
		t.Fatalf("create fixture collection %q: %v", collectionPath, err)
	}

	fileFavoriteName := "it-favorite-file-" + xid.New().String()
	collectionFavoriteName := "it-favorite-collection-" + xid.New().String()

	if err := service.AddFavorite(fileFavoriteName, filePath); err != nil {
		t.Fatalf("add file favorite: %v", err)
	}
	if err := service.AddFavorite(collectionFavoriteName, collectionPath); err != nil {
		t.Fatalf("add collection favorite: %v", err)
	}

	listedFavorites, err := service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites after add: %v", err)
	}
	assertFavorites(t, listedFavorites, map[string]string{
		filePath:       fileFavoriteName,
		collectionPath: collectionFavoriteName,
	})

	renamedFileFavoriteName := "it-favorite-file-renamed-" + xid.New().String()
	if err := service.RenameFavorite(filePath, renamedFileFavoriteName); err != nil {
		t.Fatalf("rename file favorite: %v", err)
	}

	listedFavorites, err = service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites after rename: %v", err)
	}
	assertFavorites(t, listedFavorites, map[string]string{
		filePath:       renamedFileFavoriteName,
		collectionPath: collectionFavoriteName,
	})

	if err := service.RemoveFavorite(collectionPath); err != nil {
		t.Fatalf("remove collection favorite: %v", err)
	}

	listedFavorites, err = service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites after remove: %v", err)
	}
	assertFavorites(t, listedFavorites, map[string]string{
		filePath: renamedFileFavoriteName,
	})
}

func assertFavorites(t *testing.T, favoritesList []favorites.Favorite, expectedByPath map[string]string) {
	t.Helper()

	if len(favoritesList) != len(expectedByPath) {
		t.Fatalf("expected %d favorites, got %d (%+v)", len(expectedByPath), len(favoritesList), favoritesList)
	}

	for _, favorite := range favoritesList {
		expectedName, ok := expectedByPath[favorite.Path]
		if !ok {
			t.Fatalf("unexpected favorite %+v", favorite)
		}
		if favorite.Name != expectedName {
			t.Fatalf("expected favorite %q name %q, got %q", favorite.Path, expectedName, favorite.Name)
		}
	}
}

type irodsFavoritesFilesystem struct {
	filesystem *irodsfs.FileSystem
}

func (filesystem *irodsFavoritesFilesystem) CollectionExists(irodsPath string) (bool, error) {
	return filesystem.filesystem.ExistsDir(irodsPath), nil
}

func (filesystem *irodsFavoritesFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	return filesystem.filesystem.MakeDir(irodsPath, recurse)
}

func (filesystem *irodsFavoritesFilesystem) CreateDataObject(dataObjectPath string) error {
	handle, err := filesystem.filesystem.CreateFile(dataObjectPath, "", "w")
	if err != nil {
		return err
	}
	return handle.Close()
}

func (filesystem *irodsFavoritesFilesystem) ListDataObjectMetadata(dataObjectPath string) ([]favorites.Metadata, error) {
	metadata, err := filesystem.filesystem.ListMetadata(dataObjectPath)
	if err != nil {
		return nil, err
	}

	result := make([]favorites.Metadata, 0, len(metadata))
	for _, avu := range metadata {
		if avu == nil {
			continue
		}
		result = append(result, favorites.Metadata{
			Name:  avu.Name,
			Value: avu.Value,
			Units: avu.Units,
		})
	}
	return result, nil
}

func (filesystem *irodsFavoritesFilesystem) AddDataObjectMetadata(dataObjectPath string, metadata favorites.Metadata) error {
	return filesystem.filesystem.AddMetadata(dataObjectPath, metadata.Name, metadata.Value, metadata.Units)
}

func (filesystem *irodsFavoritesFilesystem) DeleteDataObjectMetadata(dataObjectPath string, metadata favorites.Metadata) error {
	return filesystem.filesystem.DeleteMetadataByAVU(dataObjectPath, metadata.Name, metadata.Value, metadata.Units)
}
