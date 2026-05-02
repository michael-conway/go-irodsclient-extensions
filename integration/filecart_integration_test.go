//go:build integration
// +build integration

package integration

import (
	"path"
	"strings"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/michael-conway/go-irodsclient-extensions/filecart"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/rs/xid"
)

func TestFileCartCreateListRemoveIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()

	cartFilesystem := &irodsFileCartFilesystem{filesystem: filesystem}
	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	service, err := filecart.NewService(cartFilesystem, homePath)
	if err != nil {
		t.Fatalf("create filecart service: %v", err)
	}

	fixtureRoot := path.Join(homePath, ".goext-filecart-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	t.Cleanup(func() {
		_ = filesystem.RemoveDir(fixtureRoot, true, true)
	})

	fileA := path.Join(fixtureRoot, "alpha.txt")
	fileB := path.Join(fixtureRoot, "beta.txt")
	collectionPath := path.Join(fixtureRoot, "nested")
	collectionFileA := path.Join(collectionPath, "nested-a.txt")
	collectionFileB := path.Join(collectionPath, "nested-b.txt")

	createEmptyFile(t, filesystem, fileA)
	createEmptyFile(t, filesystem, fileB)
	if err := filesystem.MakeDir(collectionPath, true); err != nil {
		t.Fatalf("create fixture collection %q: %v", collectionPath, err)
	}
	createEmptyFile(t, filesystem, collectionFileA)
	createEmptyFile(t, filesystem, collectionFileB)

	firstCartName := "it-file-cart-files-" + xid.New().String()
	firstCart, err := service.CreateCart(firstCartName)
	if err != nil {
		t.Fatalf("create first cart: %v", err)
	}
	t.Cleanup(func() {
		_ = service.DeleteCart(firstCart.Path)
	})

	if err := service.AddItem(firstCart.Path, fileA, filecart.EntryTypeFile); err != nil {
		t.Fatalf("add first file to first cart: %v", err)
	}
	if err := service.AddItem(firstCart.Path, fileB, filecart.EntryTypeFile); err != nil {
		t.Fatalf("add second file to first cart: %v", err)
	}

	secondCartName := "it-file-cart-collection-" + xid.New().String()
	secondCart, err := service.CreateCart(secondCartName)
	if err != nil {
		t.Fatalf("create second cart: %v", err)
	}
	t.Cleanup(func() {
		_ = service.DeleteCart(secondCart.Path)
	})

	if err := service.AddItem(secondCart.Path, collectionPath, filecart.EntryTypeCollection); err != nil {
		t.Fatalf("add collection to second cart: %v", err)
	}

	firstCartItems, err := service.ListItems(firstCart.Path)
	if err != nil {
		t.Fatalf("list first cart items: %v", err)
	}
	assertCartItems(t, firstCartItems, map[string]filecart.EntryType{
		fileA: filecart.EntryTypeFile,
		fileB: filecart.EntryTypeFile,
	})

	secondCartItems, err := service.ListItems(secondCart.Path)
	if err != nil {
		t.Fatalf("list second cart items: %v", err)
	}
	assertCartItems(t, secondCartItems, map[string]filecart.EntryType{
		collectionPath: filecart.EntryTypeCollection,
	})

	allCarts, err := service.ListCarts()
	if err != nil {
		t.Fatalf("list all carts: %v", err)
	}
	cartByPath := map[string]filecart.Cart{}
	for _, cart := range allCarts {
		cartByPath[cart.Path] = cart
	}
	firstListed, firstFound := cartByPath[firstCart.Path]
	secondListed, secondFound := cartByPath[secondCart.Path]
	if !firstFound || !secondFound {
		t.Fatalf("expected both integration carts to be present in list, found=%v carts=%+v", []bool{firstFound, secondFound}, allCarts)
	}
	if firstListed.AssignedName != firstCartName {
		t.Fatalf("expected first cart assigned name %q, got %q", firstCartName, firstListed.AssignedName)
	}
	if secondListed.AssignedName != secondCartName {
		t.Fatalf("expected second cart assigned name %q, got %q", secondCartName, secondListed.AssignedName)
	}

	if err := service.RemoveItem(firstCart.Path, fileA); err != nil {
		t.Fatalf("remove one file from first cart: %v", err)
	}

	firstCartItemsAfterRemove, err := service.ListItems(firstCart.Path)
	if err != nil {
		t.Fatalf("list first cart items after remove: %v", err)
	}
	assertCartItems(t, firstCartItemsAfterRemove, map[string]filecart.EntryType{
		fileB: filecart.EntryTypeFile,
	})
}

func assertCartItems(t *testing.T, items []filecart.CartEntry, expected map[string]filecart.EntryType) {
	t.Helper()

	if len(items) != len(expected) {
		t.Fatalf("expected %d cart items, got %d (%+v)", len(expected), len(items), items)
	}

	for _, item := range items {
		expectedType, ok := expected[item.Path]
		if !ok {
			t.Fatalf("unexpected cart item %+v", item)
		}
		if item.Type != expectedType {
			t.Fatalf("expected item %q type %q, got %q", item.Path, expectedType, item.Type)
		}
	}
}

func createEmptyFile(t *testing.T, filesystem *irodsfs.FileSystem, irodsPath string) {
	t.Helper()

	handle, err := filesystem.CreateFile(irodsPath, "", "w")
	if err != nil {
		t.Fatalf("create file %q: %v", irodsPath, err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close file %q: %v", irodsPath, err)
	}
}

type irodsFileCartFilesystem struct {
	filesystem *irodsfs.FileSystem
}

func (filesystem *irodsFileCartFilesystem) CollectionExists(irodsPath string) (bool, error) {
	return filesystem.filesystem.ExistsDir(irodsPath), nil
}

func (filesystem *irodsFileCartFilesystem) CreateCollection(irodsPath string, recurse bool) error {
	return filesystem.filesystem.MakeDir(irodsPath, recurse)
}

func (filesystem *irodsFileCartFilesystem) ListDataObjects(collectionPath string) ([]string, error) {
	entries, err := filesystem.filesystem.List(collectionPath)
	if err != nil {
		return nil, err
	}

	dataObjectPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.IsDir() {
			continue
		}
		dataObjectPaths = append(dataObjectPaths, entry.Path)
	}
	return dataObjectPaths, nil
}

func (filesystem *irodsFileCartFilesystem) CreateDataObject(dataObjectPath string) error {
	handle, err := filesystem.filesystem.CreateFile(dataObjectPath, "", "w")
	if err != nil {
		return err
	}
	return handle.Close()
}

func (filesystem *irodsFileCartFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	return filesystem.filesystem.RemoveFile(dataObjectPath, force)
}

func (filesystem *irodsFileCartFilesystem) CopyDataObject(srcPath string, destPath string, force bool) error {
	return filesystem.filesystem.CopyFile(srcPath, destPath, force)
}

func (filesystem *irodsFileCartFilesystem) ListDataObjectMetadata(dataObjectPath string) ([]filecart.Metadata, error) {
	metadata, err := filesystem.filesystem.ListMetadata(dataObjectPath)
	if err != nil {
		return nil, err
	}

	result := make([]filecart.Metadata, 0, len(metadata))
	for _, avu := range metadata {
		if avu == nil {
			continue
		}
		result = append(result, filecart.Metadata{
			Name:  avu.Name,
			Value: avu.Value,
			Units: avu.Units,
		})
	}
	return result, nil
}

func (filesystem *irodsFileCartFilesystem) AddDataObjectMetadata(dataObjectPath string, metadata filecart.Metadata) error {
	return filesystem.filesystem.AddMetadata(dataObjectPath, metadata.Name, metadata.Value, metadata.Units)
}

func (filesystem *irodsFileCartFilesystem) DeleteDataObjectMetadata(dataObjectPath string, metadata filecart.Metadata) error {
	return filesystem.filesystem.DeleteMetadataByAVU(dataObjectPath, metadata.Name, metadata.Value, metadata.Units)
}
