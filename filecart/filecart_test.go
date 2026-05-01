package filecart

import (
	"errors"
	"path"
	"sort"
	"strings"
	"testing"
)

func TestEnsureAndCreateCart(t *testing.T) {
	fs := newTestFilesystem()
	service, err := NewService(fs, "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	oldCartID := newCartID
	newCartID = func() string { return "cart-1" }
	defer func() { newCartID = oldCartID }()

	cart, err := service.CreateCart("Primary Cart")
	if err != nil {
		t.Fatalf("create cart: %v", err)
	}

	expectedPath := "/tempZone/home/test1/.irodsext/filecarts/cart-1.cart"
	if cart.Path != expectedPath {
		t.Fatalf("expected cart path %q, got %q", expectedPath, cart.Path)
	}
	if cart.AssignedName != "Primary Cart" {
		t.Fatalf("expected assigned name Primary Cart, got %q", cart.AssignedName)
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext"]; !ok {
		t.Fatal("expected root extension collection to be created")
	}
	if _, ok := fs.collections["/tempZone/home/test1/.irodsext/filecarts"]; !ok {
		t.Fatal("expected filecarts collection to be created")
	}

	metadata := fs.metadata[cart.Path]
	if len(metadata) != 1 {
		t.Fatalf("expected one metadata AVU, got %d", len(metadata))
	}
	if metadata[0].Name != AVUCartNameAttribute || metadata[0].Value != "Primary Cart" || metadata[0].Units != AVUCartNameUnit {
		t.Fatalf("unexpected cart metadata %+v", metadata[0])
	}
}

func TestListCartsFiltersExtensionAndSorts(t *testing.T) {
	fs := newTestFilesystem()
	fs.collections["/tempZone/home/test1/.irodsext"] = struct{}{}
	fs.collections["/tempZone/home/test1/.irodsext/filecarts"] = struct{}{}
	fs.objects["/tempZone/home/test1/.irodsext/filecarts/b.cart"] = struct{}{}
	fs.objects["/tempZone/home/test1/.irodsext/filecarts/a.cart"] = struct{}{}
	fs.objects["/tempZone/home/test1/.irodsext/filecarts/not-a-cart.txt"] = struct{}{}
	fs.metadata["/tempZone/home/test1/.irodsext/filecarts/a.cart"] = []Metadata{
		{Name: AVUCartNameAttribute, Value: "Cart A", Units: AVUCartNameUnit},
	}

	service, err := NewService(fs, "/tempZone/home/test1")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	carts, err := service.ListCarts()
	if err != nil {
		t.Fatalf("list carts: %v", err)
	}
	if len(carts) != 2 {
		t.Fatalf("expected 2 carts, got %d", len(carts))
	}
	if carts[0].FileName != "a.cart" || carts[1].FileName != "b.cart" {
		t.Fatalf("expected carts to be sorted and filtered, got %+v", carts)
	}
	if carts[0].AssignedName != "Cart A" {
		t.Fatalf("expected assigned name Cart A, got %q", carts[0].AssignedName)
	}
}

func TestSetCartNameReplacesExistingValue(t *testing.T) {
	fs := newTestFilesystem()
	cartPath := "/tempZone/home/test1/.irodsext/filecarts/a.cart"
	fs.collections["/tempZone/home/test1/.irodsext"] = struct{}{}
	fs.collections["/tempZone/home/test1/.irodsext/filecarts"] = struct{}{}
	fs.objects[cartPath] = struct{}{}
	fs.metadata[cartPath] = []Metadata{
		{Name: AVUCartNameAttribute, Value: "old", Units: AVUCartNameUnit},
		{Name: AVUCartEntryAttribute, Value: "/tempZone/home/test1/x.txt", Units: string(EntryTypeFile)},
	}

	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.SetCartName(cartPath, "new"); err != nil {
		t.Fatalf("set cart name: %v", err)
	}

	names := make([]string, 0)
	for _, avu := range fs.metadata[cartPath] {
		if avu.Name == AVUCartNameAttribute && avu.Units == AVUCartNameUnit {
			names = append(names, avu.Value)
		}
	}
	if len(names) != 1 || names[0] != "new" {
		t.Fatalf("expected single cart name AVU with value new, got %+v", names)
	}
}

func TestCopyCartCopiesEntriesAndSetsNewName(t *testing.T) {
	fs := newTestFilesystem()
	source := "/tempZone/home/test1/.irodsext/filecarts/src.cart"
	fs.collections["/tempZone/home/test1/.irodsext"] = struct{}{}
	fs.collections["/tempZone/home/test1/.irodsext/filecarts"] = struct{}{}
	fs.objects[source] = struct{}{}
	fs.metadata[source] = []Metadata{
		{Name: AVUCartNameAttribute, Value: "Source Cart", Units: AVUCartNameUnit},
		{Name: AVUCartEntryAttribute, Value: "/tempZone/home/test1/a.txt", Units: string(EntryTypeFile)},
	}

	service, _ := NewService(fs, "/tempZone/home/test1")
	oldCartID := newCartID
	newCartID = func() string { return "cart-copy" }
	defer func() { newCartID = oldCartID }()

	copied, err := service.CopyCart("src.cart", "Copied Cart")
	if err != nil {
		t.Fatalf("copy cart: %v", err)
	}
	if copied.FileName != "cart-copy.cart" {
		t.Fatalf("unexpected copied cart filename %q", copied.FileName)
	}

	copiedMetadata := fs.metadata[copied.Path]
	var nameValues []string
	for _, avu := range copiedMetadata {
		if avu.Name == AVUCartNameAttribute && avu.Units == AVUCartNameUnit {
			nameValues = append(nameValues, avu.Value)
		}
	}
	if len(nameValues) != 1 || nameValues[0] != "Copied Cart" {
		t.Fatalf("expected copied cart name to be replaced, got %+v", nameValues)
	}
}

func TestAddListRemoveItems(t *testing.T) {
	fs := newTestFilesystem()
	cartPath := "/tempZone/home/test1/.irodsext/filecarts/a.cart"
	fs.collections["/tempZone/home/test1/.irodsext"] = struct{}{}
	fs.collections["/tempZone/home/test1/.irodsext/filecarts"] = struct{}{}
	fs.objects[cartPath] = struct{}{}

	service, _ := NewService(fs, "/tempZone/home/test1")

	if err := service.AddItem(cartPath, "/tempZone/home/test1/a.txt", EntryTypeFile); err != nil {
		t.Fatalf("add file item: %v", err)
	}
	if err := service.AddItem(cartPath, "/tempZone/home/test1/folder", EntryTypeCollection); err != nil {
		t.Fatalf("add collection item: %v", err)
	}
	if err := service.AddItem(cartPath, "/tempZone/home/test1/a.txt", EntryTypeFile); err != nil {
		t.Fatalf("add duplicate item: %v", err)
	}

	items, err := service.ListItems(cartPath)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 unique items, got %d", len(items))
	}

	if err := service.RemoveItem(cartPath, "/tempZone/home/test1/a.txt"); err != nil {
		t.Fatalf("remove item: %v", err)
	}

	items, err = service.ListItems(cartPath)
	if err != nil {
		t.Fatalf("list items after remove: %v", err)
	}
	if len(items) != 1 || items[0].Path != "/tempZone/home/test1/folder" {
		t.Fatalf("unexpected items after remove %+v", items)
	}
}

func TestDeleteCart(t *testing.T) {
	fs := newTestFilesystem()
	cartPath := "/tempZone/home/test1/.irodsext/filecarts/a.cart"
	fs.collections["/tempZone/home/test1/.irodsext"] = struct{}{}
	fs.collections["/tempZone/home/test1/.irodsext/filecarts"] = struct{}{}
	fs.objects[cartPath] = struct{}{}

	service, _ := NewService(fs, "/tempZone/home/test1")
	if err := service.DeleteCart(cartPath); err != nil {
		t.Fatalf("delete cart: %v", err)
	}

	if _, ok := fs.objects[cartPath]; ok {
		t.Fatal("expected cart object to be deleted")
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
	if _, err := service.CreateCart(" "); !errors.Is(err, ErrInvalidCartName) {
		t.Fatalf("expected ErrInvalidCartName, got %v", err)
	}
	if err := service.AddItem("cart-1", "x.txt", EntryTypeFile); !errors.Is(err, ErrInvalidEntryPath) {
		t.Fatalf("expected ErrInvalidEntryPath, got %v", err)
	}
	if err := service.AddItem("cart-1", "/x.txt", EntryType("bad")); !errors.Is(err, ErrInvalidEntryType) {
		t.Fatalf("expected ErrInvalidEntryType, got %v", err)
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
	irodsPath = path.Clean(irodsPath)
	fs.collections[irodsPath] = struct{}{}
	return nil
}

func (fs *testFilesystem) ListDataObjects(collectionPath string) ([]string, error) {
	collectionPath = strings.TrimSuffix(path.Clean(collectionPath), "/")

	objects := make([]string, 0)
	for objectPath := range fs.objects {
		if path.Dir(objectPath) == collectionPath {
			objects = append(objects, objectPath)
		}
	}
	sort.Strings(objects)
	return objects, nil
}

func (fs *testFilesystem) CreateDataObject(dataObjectPath string) error {
	dataObjectPath = path.Clean(dataObjectPath)
	fs.objects[dataObjectPath] = struct{}{}
	return nil
}

func (fs *testFilesystem) DeleteDataObject(dataObjectPath string, force bool) error {
	dataObjectPath = path.Clean(dataObjectPath)
	delete(fs.objects, dataObjectPath)
	delete(fs.metadata, dataObjectPath)
	return nil
}

func (fs *testFilesystem) CopyDataObject(srcPath string, destPath string, force bool) error {
	srcPath = path.Clean(srcPath)
	destPath = path.Clean(destPath)

	if _, ok := fs.objects[srcPath]; !ok {
		return errors.New("source object does not exist")
	}
	fs.objects[destPath] = struct{}{}

	copied := make([]Metadata, len(fs.metadata[srcPath]))
	copy(copied, fs.metadata[srcPath])
	fs.metadata[destPath] = copied
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
	return nil
}
