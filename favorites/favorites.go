package favorites

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/michael-conway/go-irodsclient-extensions/userpersist"
)

const (
	CategoryName      = "favorites"
	FavoritesFileName = "favorites"

	AVUFavoriteAttribute = "iRODS:Favorite"
	AVUFavoriteUnit      = "iRODS:Favorite"
)

var (
	ErrMissingFilesystem   = errors.New("missing filesystem")
	ErrInvalidUserHome     = errors.New("invalid user home path")
	ErrInvalidFavoriteName = errors.New("invalid favorite name")
	ErrInvalidFavoritePath = errors.New("invalid favorite path")
	ErrFavoriteNotFound    = errors.New("favorite not found")
)

type Metadata struct {
	Name  string
	Value string
	Units string
}

type Filesystem interface {
	userpersist.CollectionFilesystem
	CreateDataObject(dataObjectPath string) error
	ListDataObjectMetadata(dataObjectPath string) ([]Metadata, error)
	AddDataObjectMetadata(dataObjectPath string, metadata Metadata) error
	DeleteDataObjectMetadata(dataObjectPath string, metadata Metadata) error
}

type Favorite struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type favoriteAVUValue struct {
	Name         string `json:"name"`
	AbsolutePath string `json:"absolute_path"`
}

type Service struct {
	filesystem   Filesystem
	userHomePath string
	categoryPath string
	filePath     string
}

func NewService(filesystem Filesystem, userHomePath string) (*Service, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	userHomePath = strings.TrimSpace(userHomePath)
	if userHomePath == "" || !strings.HasPrefix(userHomePath, "/") {
		return nil, ErrInvalidUserHome
	}

	categoryPath, err := userpersist.CategoryPath(userHomePath, CategoryName)
	if err != nil {
		return nil, err
	}

	return &Service{
		filesystem:   filesystem,
		userHomePath: path.Clean(userHomePath),
		categoryPath: categoryPath,
		filePath:     path.Join(categoryPath, FavoritesFileName),
	}, nil
}

func (service *Service) UserHomePath() string {
	return service.userHomePath
}

func (service *Service) CollectionPath() string {
	return service.categoryPath
}

func (service *Service) FavoritesPath() string {
	return service.filePath
}

func (service *Service) Ensure() (string, error) {
	categoryPath, err := userpersist.EnsureCategoryCollection(service.filesystem, service.userHomePath, CategoryName)
	if err != nil {
		return "", err
	}
	service.categoryPath = categoryPath
	service.filePath = path.Join(categoryPath, FavoritesFileName)

	if err := service.filesystem.CreateDataObject(service.filePath); err != nil {
		return "", fmt.Errorf("create favorites data object %q: %w", service.filePath, err)
	}

	return service.filePath, nil
}

func (service *Service) AddFavorite(name string, favoritePath string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidFavoriteName
	}

	favoritePath = normalizeFavoritePath(favoritePath)
	if favoritePath == "" {
		return ErrInvalidFavoritePath
	}

	favoritesPath, err := service.Ensure()
	if err != nil {
		return err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(favoritesPath)
	if err != nil {
		return fmt.Errorf("list favorites metadata for %q: %w", favoritesPath, err)
	}

	encodedValue, err := encodeFavoriteValue(Favorite{
		Name: name,
		Path: favoritePath,
	})
	if err != nil {
		return fmt.Errorf("encode favorite metadata for %q: %w", favoritePath, err)
	}

	matching := make([]Metadata, 0)
	hasCanonical := false
	for _, avu := range metadata {
		favorite, ok := decodeFavoriteFromMetadata(avu)
		if !ok || favorite.Path != favoritePath {
			continue
		}

		matching = append(matching, avu)
		if strings.TrimSpace(avu.Value) == encodedValue {
			hasCanonical = true
		}
	}

	if len(matching) == 1 && hasCanonical {
		return nil
	}

	for _, avu := range matching {
		if err := service.filesystem.DeleteDataObjectMetadata(favoritesPath, avu); err != nil {
			return fmt.Errorf("delete existing favorite metadata from %q: %w", favoritesPath, err)
		}
	}

	if err := service.filesystem.AddDataObjectMetadata(favoritesPath, Metadata{
		Name:  AVUFavoriteAttribute,
		Value: encodedValue,
		Units: AVUFavoriteUnit,
	}); err != nil {
		return fmt.Errorf("add favorite metadata on %q: %w", favoritesPath, err)
	}

	return nil
}

func (service *Service) RenameFavorite(favoritePath string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidFavoriteName
	}

	favoritePath = normalizeFavoritePath(favoritePath)
	if favoritePath == "" {
		return ErrInvalidFavoritePath
	}

	favoritesPath, err := service.Ensure()
	if err != nil {
		return err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(favoritesPath)
	if err != nil {
		return fmt.Errorf("list favorites metadata for %q: %w", favoritesPath, err)
	}

	existingCount := 0
	for _, avu := range metadata {
		favorite, ok := decodeFavoriteFromMetadata(avu)
		if !ok || favorite.Path != favoritePath {
			continue
		}

		if err := service.filesystem.DeleteDataObjectMetadata(favoritesPath, avu); err != nil {
			return fmt.Errorf("delete existing favorite metadata from %q: %w", favoritesPath, err)
		}
		existingCount++
	}

	if existingCount == 0 {
		return ErrFavoriteNotFound
	}

	encodedValue, err := encodeFavoriteValue(Favorite{
		Name: name,
		Path: favoritePath,
	})
	if err != nil {
		return fmt.Errorf("encode favorite metadata for %q: %w", favoritePath, err)
	}

	if err := service.filesystem.AddDataObjectMetadata(favoritesPath, Metadata{
		Name:  AVUFavoriteAttribute,
		Value: encodedValue,
		Units: AVUFavoriteUnit,
	}); err != nil {
		return fmt.Errorf("add renamed favorite metadata on %q: %w", favoritesPath, err)
	}

	return nil
}

func (service *Service) RemoveFavorite(favoritePath string) error {
	favoritePath = normalizeFavoritePath(favoritePath)
	if favoritePath == "" {
		return ErrInvalidFavoritePath
	}

	favoritesPath, err := service.Ensure()
	if err != nil {
		return err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(favoritesPath)
	if err != nil {
		return fmt.Errorf("list favorites metadata for %q: %w", favoritesPath, err)
	}

	for _, avu := range metadata {
		favorite, ok := decodeFavoriteFromMetadata(avu)
		if !ok || favorite.Path != favoritePath {
			continue
		}

		if err := service.filesystem.DeleteDataObjectMetadata(favoritesPath, avu); err != nil {
			return fmt.Errorf("delete favorite metadata from %q: %w", favoritesPath, err)
		}
	}

	return nil
}

func (service *Service) ListFavorites() ([]Favorite, error) {
	favoritesPath, err := service.Ensure()
	if err != nil {
		return nil, err
	}

	metadata, err := service.filesystem.ListDataObjectMetadata(favoritesPath)
	if err != nil {
		return nil, fmt.Errorf("list favorites metadata for %q: %w", favoritesPath, err)
	}

	favoritesByPath := map[string]Favorite{}
	for _, avu := range metadata {
		favorite, ok := decodeFavoriteFromMetadata(avu)
		if !ok {
			continue
		}

		favoritesByPath[favorite.Path] = favorite
	}

	favorites := make([]Favorite, 0, len(favoritesByPath))
	for _, favorite := range favoritesByPath {
		favorites = append(favorites, favorite)
	}

	sort.Slice(favorites, func(i, j int) bool {
		if favorites[i].Name == favorites[j].Name {
			return favorites[i].Path < favorites[j].Path
		}
		return favorites[i].Name < favorites[j].Name
	})

	return favorites, nil
}

func normalizeFavoritePath(favoritePath string) string {
	favoritePath = strings.TrimSpace(favoritePath)
	if favoritePath == "" || !strings.HasPrefix(favoritePath, "/") {
		return ""
	}
	return path.Clean(favoritePath)
}

func encodeFavoriteValue(favorite Favorite) (string, error) {
	favorite.Path = normalizeFavoritePath(favorite.Path)
	favorite.Name = strings.TrimSpace(favorite.Name)
	if favorite.Path == "" {
		return "", ErrInvalidFavoritePath
	}
	if favorite.Name == "" {
		return "", ErrInvalidFavoriteName
	}

	value, err := json.Marshal(favoriteAVUValue{
		Name:         favorite.Name,
		AbsolutePath: favorite.Path,
	})
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func decodeFavoriteFromMetadata(metadata Metadata) (Favorite, bool) {
	if metadata.Name != AVUFavoriteAttribute || metadata.Units != AVUFavoriteUnit {
		return Favorite{}, false
	}

	var value favoriteAVUValue
	if err := json.Unmarshal([]byte(strings.TrimSpace(metadata.Value)), &value); err != nil {
		return Favorite{}, false
	}

	favoritePath := normalizeFavoritePath(value.AbsolutePath)
	if favoritePath == "" {
		return Favorite{}, false
	}

	name := strings.TrimSpace(value.Name)
	if name == "" {
		name = path.Base(favoritePath)
	}

	return Favorite{
		Name: name,
		Path: favoritePath,
	}, true
}
