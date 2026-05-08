package favorites

import (
	"errors"
	"path"
	"sort"
	"testing"
)

func TestEnsureCreatesFavoritesStructure(t *testing.T) {
	fs := newTestFilesystem()
	service, err := NewService(fs, "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	favoritesPath, err := service.Ensure()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	expectedPath := "/tempZone/home/test1/.irodsext/favorites/favorites"
	if favoritesPath != expectedPath {
		t.Fatalf("expected favorites path %q, got %q", expectedPath, favoritesPath)
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext"]; !ok {
		t.Fatal("expected root extension collection to be created")
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext/favorites"]; !ok {
		t.Fatal("expected favorites collection to be created")
	}
	if _, ok := fs.objects[expectedPath]; !ok {
		t.Fatal("expected favorites data object to be created")
	}
}

func TestAddListRemoveFavorites(t *testing.T) {
	fs := newTestFilesystem()
	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.AddFavorite("file A", "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("add favorite file: %v", err)
	}
	if err := service.AddFavorite("folder B", "/tempZone/home/test1/folder"); err != nil {
		t.Fatalf("add favorite collection: %v", err)
	}
	if err := service.AddFavorite("file A", "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("add duplicate favorite: %v", err)
	}

	favorites, err := service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("expected 2 favorites, got %d", len(favorites))
	}
	if favorites[0].Name != "file A" || favorites[0].Path != "/tempZone/home/test1/a.txt" {
		t.Fatalf("unexpected first favorite %+v", favorites[0])
	}
	if favorites[1].Name != "folder B" || favorites[1].Path != "/tempZone/home/test1/folder" {
		t.Fatalf("unexpected second favorite %+v", favorites[1])
	}

	if err := service.RemoveFavorite("/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("remove favorite: %v", err)
	}

	favorites, err = service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites after remove: %v", err)
	}
	if len(favorites) != 1 || favorites[0].Path != "/tempZone/home/test1/folder" {
		t.Fatalf("unexpected favorites after remove %+v", favorites)
	}
}

func TestAddFavoriteUsesJSONValueAndFixedUnit(t *testing.T) {
	fs := newTestFilesystem()
	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.AddFavorite("Alpha", "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("add favorite: %v", err)
	}

	metadata := fs.metadata[service.FavoritesPath()]
	if len(metadata) != 1 {
		t.Fatalf("expected one metadata entry, got %d", len(metadata))
	}
	if metadata[0].Name != AVUFavoriteAttribute {
		t.Fatalf("expected favorite attribute, got %q", metadata[0].Name)
	}
	if metadata[0].Units != AVUFavoriteUnit {
		t.Fatalf("expected favorite unit %q, got %q", AVUFavoriteUnit, metadata[0].Units)
	}

	favorite, ok := decodeFavoriteFromMetadata(metadata[0])
	if !ok {
		t.Fatalf("expected decodable favorite metadata, got %+v", metadata[0])
	}
	if favorite.Name != "Alpha" || favorite.Path != "/tempZone/home/test1/a.txt" {
		t.Fatalf("unexpected decoded favorite %+v", favorite)
	}
}

func TestAddFavoriteReplacesNameForExistingPath(t *testing.T) {
	fs := newTestFilesystem()
	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.AddFavorite("old name", "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("add favorite old name: %v", err)
	}
	if err := service.AddFavorite("new name", "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("replace favorite name: %v", err)
	}

	favorites, err := service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if len(favorites) != 1 {
		t.Fatalf("expected 1 favorite after replace, got %d", len(favorites))
	}
	if favorites[0].Name != "new name" {
		t.Fatalf("expected replaced name new name, got %q", favorites[0].Name)
	}
}

func TestRenameFavoriteChangesName(t *testing.T) {
	fs := newTestFilesystem()
	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.AddFavorite("old name", "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("add favorite old name: %v", err)
	}

	if err := service.RenameFavorite("/tempZone/home/test1/a.txt", "new name"); err != nil {
		t.Fatalf("rename favorite: %v", err)
	}

	favorites, err := service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if len(favorites) != 1 {
		t.Fatalf("expected 1 favorite after rename, got %d", len(favorites))
	}
	if favorites[0].Name != "new name" || favorites[0].Path != "/tempZone/home/test1/a.txt" {
		t.Fatalf("unexpected favorite after rename %+v", favorites[0])
	}
}

func TestRenameFavoriteRequiresExistingFavorite(t *testing.T) {
	fs := newTestFilesystem()
	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.RenameFavorite("/tempZone/home/test1/a.txt", "new name"); !errors.Is(err, ErrFavoriteNotFound) {
		t.Fatalf("expected ErrFavoriteNotFound, got %v", err)
	}
}

func TestListFavoritesFiltersAndFallbackName(t *testing.T) {
	fs := newTestFilesystem()
	service, _ := NewService(fs, "/tempZone/home/test1")
	favoritesPath, err := service.Ensure()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	validA := `{"name":"","absolute_path":"/tempZone/home/test1/a.txt"}`
	validB, _ := encodeFavoriteValue(Favorite{
		Name: "B",
		Path: "/tempZone/home/test1/b.txt",
	})

	fs.metadata[favoritesPath] = []Metadata{
		{Name: AVUFavoriteAttribute, Value: validA, Units: AVUFavoriteUnit},
		{Name: AVUFavoriteAttribute, Value: validB, Units: AVUFavoriteUnit},
		{Name: "ignore", Value: validA, Units: AVUFavoriteUnit},
		{Name: AVUFavoriteAttribute, Value: `{"name":"bad","absolute_path":"relative/path"}`, Units: AVUFavoriteUnit},
		{Name: AVUFavoriteAttribute, Value: "not json", Units: AVUFavoriteUnit},
	}

	favorites, err := service.ListFavorites()
	if err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("expected 2 valid favorites, got %d", len(favorites))
	}
	if favorites[0].Name != "B" || favorites[0].Path != "/tempZone/home/test1/b.txt" {
		t.Fatalf("unexpected first favorite %+v", favorites[0])
	}
	if favorites[1].Name != "a.txt" || favorites[1].Path != "/tempZone/home/test1/a.txt" {
		t.Fatalf("expected fallback basename for empty AVU value name, got %+v", favorites[1])
	}
}

func TestValidation(t *testing.T) {
	fs := newTestFilesystem()

	if _, err := NewService(nil, "/tempZone/home/test1"); !errors.Is(err, ErrMissingFilesystem) {
		t.Fatalf("expected ErrMissingFilesystem, got %v", err)
	}
	if _, err := NewService(fs, "tempZone/home/test1"); !errors.Is(err, ErrInvalidUserHome) {
		t.Fatalf("expected ErrInvalidUserHome, got %v", err)
	}

	service, _ := NewService(fs, "/tempZone/home/test1")
	if err := service.AddFavorite(" ", "/tempZone/home/test1/a.txt"); !errors.Is(err, ErrInvalidFavoriteName) {
		t.Fatalf("expected ErrInvalidFavoriteName, got %v", err)
	}
	if err := service.AddFavorite("name", "a.txt"); !errors.Is(err, ErrInvalidFavoritePath) {
		t.Fatalf("expected ErrInvalidFavoritePath, got %v", err)
	}
	if err := service.RemoveFavorite("a.txt"); !errors.Is(err, ErrInvalidFavoritePath) {
		t.Fatalf("expected ErrInvalidFavoritePath, got %v", err)
	}
	if err := service.RenameFavorite("a.txt", "new name"); !errors.Is(err, ErrInvalidFavoritePath) {
		t.Fatalf("expected ErrInvalidFavoritePath, got %v", err)
	}
	if err := service.RenameFavorite("/tempZone/home/test1/a.txt", " "); !errors.Is(err, ErrInvalidFavoriteName) {
		t.Fatalf("expected ErrInvalidFavoriteName, got %v", err)
	}
}

type testFilesystem struct {
	collections map[string]struct{}
	objects     map[string]struct{}
	metadata    map[string][]Metadata
}

func newTestFilesystem() *testFilesystem {
	return &testFilesystem{
		collections: map[string]struct{}{},
		objects:     map[string]struct{}{},
		metadata:    map[string][]Metadata{},
	}
}

func (fs *testFilesystem) CollectionExists(irodsPath string) (bool, error) {
	_, ok := fs.collections[path.Clean(irodsPath)]
	return ok, nil
}

func (fs *testFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	fs.collections[path.Clean(irodsPath)] = struct{}{}
	return nil
}

func (fs *testFilesystem) CreateDataObject(dataObjectPath string) error {
	fs.objects[path.Clean(dataObjectPath)] = struct{}{}
	return nil
}

func (fs *testFilesystem) ReadDataObject(dataObjectPath string) ([]byte, error) {
	dataObjectPath = path.Clean(dataObjectPath)
	if _, ok := fs.objects[dataObjectPath]; !ok {
		return nil, errors.New("object does not exist")
	}
	return nil, nil
}

func (fs *testFilesystem) WriteDataObject(dataObjectPath string, contents []byte) error {
	fs.objects[path.Clean(dataObjectPath)] = struct{}{}
	return nil
}

func (fs *testFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	dataObjectPath = path.Clean(dataObjectPath)
	delete(fs.objects, dataObjectPath)
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
	existing := fs.metadata[dataObjectPath]
	filtered := make([]Metadata, 0, len(existing))
	deleted := false
	for _, avu := range existing {
		if !deleted && avu.Name == metadata.Name && avu.Value == metadata.Value && avu.Units == metadata.Units {
			deleted = true
			continue
		}
		filtered = append(filtered, avu)
	}
	fs.metadata[dataObjectPath] = filtered
	sort.Slice(fs.metadata[dataObjectPath], func(i, j int) bool {
		left := fs.metadata[dataObjectPath][i]
		right := fs.metadata[dataObjectPath][j]
		if left.Name == right.Name {
			if left.Units == right.Units {
				return left.Value < right.Value
			}
			return left.Units < right.Units
		}
		return left.Name < right.Name
	})
	return nil
}
