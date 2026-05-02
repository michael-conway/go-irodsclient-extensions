package userpersist

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	// RootCollectionName is the default user-scoped extension collection name.
	RootCollectionName = ".irodsext"
)

var (
	ErrMissingFilesystem = errors.New("missing filesystem")
	ErrInvalidUserHome   = errors.New("invalid user home path")
	ErrInvalidCategory   = errors.New("invalid category")
)

// CollectionFilesystem is the minimal collection API required for ensuring
// user-persisted extension collections.
type CollectionFilesystem interface {
	CollectionExists(irodsPath string) (bool, error)
	CreateCollection(irodsPath string, recurse bool) error
}

// RootPath returns the expected ~/.irodsext path for a user home collection path.
func RootPath(userHomePath string) (string, error) {
	normalized, err := normalizeHomePath(userHomePath)
	if err != nil {
		return "", err
	}
	return path.Join(normalized, RootCollectionName), nil
}

// CategoryPath returns the expected path under ~/.irodsext for a category.
func CategoryPath(userHomePath string, category string) (string, error) {
	rootPath, err := RootPath(userHomePath)
	if err != nil {
		return "", err
	}

	category = strings.TrimSpace(category)
	if category == "" || strings.Contains(category, "/") {
		return "", ErrInvalidCategory
	}
	return path.Join(rootPath, category), nil
}

// EnsureRootCollection verifies ~/.irodsext exists and creates it if missing.
func EnsureRootCollection(filesystem CollectionFilesystem, userHomePath string) (string, error) {
	rootPath, err := RootPath(userHomePath)
	if err != nil {
		return "", err
	}

	if err := ensureCollection(filesystem, rootPath); err != nil {
		return "", err
	}
	return rootPath, nil
}

// EnsureCategoryCollection verifies ~/.irodsext/<category> exists and creates
// both parent and category collections if missing.
func EnsureCategoryCollection(filesystem CollectionFilesystem, userHomePath string, category string) (string, error) {
	categoryPath, err := CategoryPath(userHomePath, category)
	if err != nil {
		return "", err
	}

	if _, err := EnsureRootCollection(filesystem, userHomePath); err != nil {
		return "", err
	}

	if err := ensureCollection(filesystem, categoryPath); err != nil {
		return "", err
	}
	return categoryPath, nil
}

func ensureCollection(filesystem CollectionFilesystem, irodsPath string) error {
	if filesystem == nil {
		return ErrMissingFilesystem
	}

	exists, err := filesystem.CollectionExists(irodsPath)
	if err != nil {
		return fmt.Errorf("check collection %q: %w", irodsPath, err)
	}
	if exists {
		return nil
	}

	if err := filesystem.CreateCollection(irodsPath, true); err != nil {
		return fmt.Errorf("create collection %q: %w", irodsPath, err)
	}
	return nil
}

func normalizeHomePath(userHomePath string) (string, error) {
	userHomePath = strings.TrimSpace(userHomePath)
	if userHomePath == "" || !strings.HasPrefix(userHomePath, "/") {
		return "", ErrInvalidUserHome
	}
	return path.Clean(userHomePath), nil
}
