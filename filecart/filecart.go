package filecart

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/michael-conway/go-irodsclient-extensions/userpersist"
	"github.com/rs/xid"
)

const (
	CategoryName = "filecarts"
	CartExt      = ".cart"

	AVUCartNameAttribute = "iRODS:FileCart:Name"
	AVUCartNameUnit      = "iRODS:FileCart"

	AVUCartEntryAttribute = "iRODS:FileCart:Entry"
)

type EntryType string

const (
	EntryTypeFile       EntryType = "file"
	EntryTypeCollection EntryType = "collection"
)

var (
	ErrMissingFilesystem   = errors.New("missing filesystem")
	ErrInvalidUserHome     = errors.New("invalid user home path")
	ErrInvalidCartName     = errors.New("invalid cart name")
	ErrInvalidCartRef      = errors.New("invalid cart reference")
	ErrInvalidEntryPath    = errors.New("invalid entry path")
	ErrInvalidEntryType    = errors.New("invalid entry type")
	ErrInvalidMetadataPath = errors.New("invalid metadata path")
)

type Metadata struct {
	Name  string
	Value string
	Units string
}

type Filesystem interface {
	userpersist.CollectionFilesystem
	ListDataObjects(collectionPath string) ([]string, error)
	CreateDataObject(dataObjectPath string) error
	DeleteDataObject(dataObjectPath string, force bool) error
	CopyDataObject(srcPath string, destPath string, force bool) error
	ListDataObjectMetadata(dataObjectPath string) ([]Metadata, error)
	AddDataObjectMetadata(dataObjectPath string, metadata Metadata) error
	DeleteDataObjectMetadata(dataObjectPath string, metadata Metadata) error
}

type Cart struct {
	ID           string `json:"id"`
	FileName     string `json:"file_name"`
	Path         string `json:"path"`
	AssignedName string `json:"assigned_name"`
}

type CartEntry struct {
	Path string    `json:"path"`
	Type EntryType `json:"type"`
}

type Service struct {
	filesystem   Filesystem
	userHomePath string
	categoryPath string
}

var newCartID = func() string {
	return xid.New().String()
}

func NewService(filesystem Filesystem, userHomePath string) (*Service, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	userHomePath = strings.TrimSpace(userHomePath)
	if userHomePath == "" || !strings.HasPrefix(userHomePath, "/") {
		return nil, ErrInvalidUserHome
	}

	cartCollectionPath, err := userpersist.CategoryPath(userHomePath, CategoryName)
	if err != nil {
		return nil, err
	}

	return &Service{
		filesystem:   filesystem,
		userHomePath: path.Clean(userHomePath),
		categoryPath: cartCollectionPath,
	}, nil
}

func (service *Service) UserHomePath() string {
	return service.userHomePath
}

func (service *Service) CollectionPath() string {
	return service.categoryPath
}

func (service *Service) Ensure() (string, error) {
	cartCollectionPath, err := userpersist.EnsureCategoryCollection(service.filesystem, service.userHomePath, CategoryName)
	if err != nil {
		return "", err
	}
	service.categoryPath = cartCollectionPath
	return cartCollectionPath, nil
}

func (service *Service) CreateCart(assignedName string) (Cart, error) {
	assignedName = strings.TrimSpace(assignedName)
	if assignedName == "" {
		return Cart{}, ErrInvalidCartName
	}

	if _, err := service.Ensure(); err != nil {
		return Cart{}, err
	}

	fileName := newCartID() + CartExt
	cartPath := path.Join(service.categoryPath, fileName)

	if err := service.filesystem.CreateDataObject(cartPath); err != nil {
		return Cart{}, fmt.Errorf("create cart data object %q: %w", cartPath, err)
	}

	if err := service.filesystem.AddDataObjectMetadata(cartPath, Metadata{
		Name:  AVUCartNameAttribute,
		Value: assignedName,
		Units: AVUCartNameUnit,
	}); err != nil {
		return Cart{}, fmt.Errorf("add cart name metadata to %q: %w", cartPath, err)
	}

	return Cart{
		ID:           strings.TrimSuffix(fileName, CartExt),
		FileName:     fileName,
		Path:         cartPath,
		AssignedName: assignedName,
	}, nil
}

func (service *Service) ListCarts() ([]Cart, error) {
	if _, err := service.Ensure(); err != nil {
		return nil, err
	}

	dataObjectPaths, err := service.filesystem.ListDataObjects(service.categoryPath)
	if err != nil {
		return nil, fmt.Errorf("list cart objects from %q: %w", service.categoryPath, err)
	}

	carts := make([]Cart, 0, len(dataObjectPaths))
	for _, dataObjectPath := range dataObjectPaths {
		fileName := path.Base(strings.TrimSpace(dataObjectPath))
		if !strings.HasSuffix(fileName, CartExt) {
			continue
		}

		assignedName, err := service.getAssignedName(dataObjectPath)
		if err != nil {
			return nil, err
		}

		carts = append(carts, Cart{
			ID:           strings.TrimSuffix(fileName, CartExt),
			FileName:     fileName,
			Path:         dataObjectPath,
			AssignedName: assignedName,
		})
	}

	sort.Slice(carts, func(i, j int) bool {
		return carts[i].FileName < carts[j].FileName
	})

	return carts, nil
}

func (service *Service) SetCartName(cartRef string, assignedName string) error {
	assignedName = strings.TrimSpace(assignedName)
	if assignedName == "" {
		return ErrInvalidCartName
	}

	cartPath, err := service.resolveCartPath(cartRef)
	if err != nil {
		return err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(cartPath)
	if err != nil {
		return fmt.Errorf("list cart metadata for %q: %w", cartPath, err)
	}

	for _, avu := range metadata {
		if avu.Name == AVUCartNameAttribute && avu.Units == AVUCartNameUnit {
			if err := service.filesystem.DeleteDataObjectMetadata(cartPath, avu); err != nil {
				return fmt.Errorf("delete existing cart name metadata from %q: %w", cartPath, err)
			}
		}
	}

	if err := service.filesystem.AddDataObjectMetadata(cartPath, Metadata{
		Name:  AVUCartNameAttribute,
		Value: assignedName,
		Units: AVUCartNameUnit,
	}); err != nil {
		return fmt.Errorf("set cart name metadata on %q: %w", cartPath, err)
	}

	return nil
}

func (service *Service) DeleteCart(cartRef string) error {
	cartPath, err := service.resolveCartPath(cartRef)
	if err != nil {
		return err
	}

	if err := service.filesystem.DeleteDataObject(cartPath, true); err != nil {
		return fmt.Errorf("delete cart data object %q: %w", cartPath, err)
	}
	return nil
}

func (service *Service) CopyCart(sourceCartRef string, assignedName string) (Cart, error) {
	sourceCartPath, err := service.resolveCartPath(sourceCartRef)
	if err != nil {
		return Cart{}, err
	}

	if _, err := service.Ensure(); err != nil {
		return Cart{}, err
	}

	fileName := newCartID() + CartExt
	newCartPath := path.Join(service.categoryPath, fileName)
	if err := service.filesystem.CopyDataObject(sourceCartPath, newCartPath, false); err != nil {
		return Cart{}, fmt.Errorf("copy cart from %q to %q: %w", sourceCartPath, newCartPath, err)
	}

	assignedName = strings.TrimSpace(assignedName)
	if assignedName == "" {
		assignedName, err = service.getAssignedName(sourceCartPath)
		if err != nil {
			return Cart{}, err
		}
	}

	if assignedName != "" {
		if err := service.SetCartName(newCartPath, assignedName); err != nil {
			return Cart{}, err
		}
	}

	return Cart{
		ID:           strings.TrimSuffix(fileName, CartExt),
		FileName:     fileName,
		Path:         newCartPath,
		AssignedName: assignedName,
	}, nil
}

func (service *Service) AddItem(cartRef string, itemPath string, itemType EntryType) error {
	itemPath = strings.TrimSpace(itemPath)
	if itemPath == "" || !strings.HasPrefix(itemPath, "/") {
		return ErrInvalidEntryPath
	}
	if itemType != EntryTypeFile && itemType != EntryTypeCollection {
		return ErrInvalidEntryType
	}

	cartPath, err := service.resolveCartPath(cartRef)
	if err != nil {
		return err
	}

	existing, err := service.ListItems(cartPath)
	if err != nil {
		return err
	}
	for _, entry := range existing {
		if entry.Path == itemPath && entry.Type == itemType {
			return nil
		}
	}

	if err := service.filesystem.AddDataObjectMetadata(cartPath, Metadata{
		Name:  AVUCartEntryAttribute,
		Value: itemPath,
		Units: string(itemType),
	}); err != nil {
		return fmt.Errorf("add cart entry metadata on %q: %w", cartPath, err)
	}
	return nil
}

func (service *Service) RemoveItem(cartRef string, itemPath string) error {
	itemPath = strings.TrimSpace(itemPath)
	if itemPath == "" || !strings.HasPrefix(itemPath, "/") {
		return ErrInvalidEntryPath
	}

	cartPath, err := service.resolveCartPath(cartRef)
	if err != nil {
		return err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(cartPath)
	if err != nil {
		return fmt.Errorf("list cart metadata for %q: %w", cartPath, err)
	}

	for _, avu := range metadata {
		if avu.Name == AVUCartEntryAttribute && avu.Value == itemPath &&
			(avu.Units == string(EntryTypeFile) || avu.Units == string(EntryTypeCollection)) {
			if err := service.filesystem.DeleteDataObjectMetadata(cartPath, avu); err != nil {
				return fmt.Errorf("delete cart entry metadata from %q: %w", cartPath, err)
			}
		}
	}
	return nil
}

func (service *Service) ListItems(cartRef string) ([]CartEntry, error) {
	cartPath, err := service.resolveCartPath(cartRef)
	if err != nil {
		return nil, err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(cartPath)
	if err != nil {
		return nil, fmt.Errorf("list cart metadata for %q: %w", cartPath, err)
	}

	items := make([]CartEntry, 0)
	for _, avu := range metadata {
		if avu.Name != AVUCartEntryAttribute {
			continue
		}
		if avu.Units != string(EntryTypeFile) && avu.Units != string(EntryTypeCollection) {
			continue
		}
		items = append(items, CartEntry{
			Path: avu.Value,
			Type: EntryType(avu.Units),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Type < items[j].Type
		}
		return items[i].Path < items[j].Path
	})
	return items, nil
}

func (service *Service) resolveCartPath(cartRef string) (string, error) {
	cartRef = strings.TrimSpace(cartRef)
	if cartRef == "" {
		return "", ErrInvalidCartRef
	}

	if strings.HasPrefix(cartRef, "/") {
		if !strings.HasSuffix(cartRef, CartExt) {
			return "", ErrInvalidCartRef
		}
		return path.Clean(cartRef), nil
	}

	if strings.Contains(cartRef, "/") {
		return "", ErrInvalidCartRef
	}

	fileName := cartRef
	if !strings.HasSuffix(fileName, CartExt) {
		fileName += CartExt
	}
	return path.Join(service.categoryPath, fileName), nil
}

func (service *Service) getAssignedName(cartPath string) (string, error) {
	if strings.TrimSpace(cartPath) == "" {
		return "", ErrInvalidMetadataPath
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(cartPath)
	if err != nil {
		return "", fmt.Errorf("list cart metadata for %q: %w", cartPath, err)
	}

	for _, avu := range metadata {
		if avu.Name == AVUCartNameAttribute && avu.Units == AVUCartNameUnit {
			return avu.Value, nil
		}
	}
	return "", nil
}
