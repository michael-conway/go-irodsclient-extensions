package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/michael-conway/go-irodsclient-extensions/userpersist"
	"github.com/rs/xid"
)

const (
	SavedEntryQueryVersion  = "metadata.saved_entry_query.v1"
	SavedEntryQueryCategory = "metadata_queries"
	SavedEntryQueryFileExt  = ".entry-query.json"
)

var (
	ErrInvalidUserHome            = errors.New("invalid user home path")
	ErrInvalidSavedEntryQueryID   = errors.New("invalid saved entry query id")
	ErrInvalidSavedEntryQueryName = errors.New("invalid saved entry query name")
	ErrInvalidSavedEntryQuery     = errors.New("invalid saved entry query")
)

// SavedEntryQueryFilesystem is the minimal iRODS API required for managing
// user-scoped saved metadata entry queries.
type SavedEntryQueryFilesystem interface {
	userpersist.FileFilesystem
	ListDataObjects(collectionPath string) ([]string, error)
}

// SavedEntryQuery stores one user-managed metadata entry query definition.
type SavedEntryQuery struct {
	Version     string               `json:"version"`
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Query       EntryQueryDefinition `json:"query"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// SavedEntryQuerySummary is the lightweight listing shape for saved queries.
type SavedEntryQuerySummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	FileName    string    `json:"file_name"`
	IRODSPath   string    `json:"irods_path"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SavedEntryQueryUpdate fully replaces the user-facing fields and query
// definition for an existing saved query while preserving CreatedAt.
type SavedEntryQueryUpdate struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Query       EntryQueryDefinition `json:"query"`
}

// SavedEntryQueryService manages saved metadata entry queries beneath a
// user-scoped userpersist category.
type SavedEntryQueryService struct {
	filesystem     SavedEntryQueryFilesystem
	files          *userpersist.FileService
	userHomePath   string
	collectionPath string
}

var savedEntryQueryNow = func() time.Time {
	return time.Now().UTC()
}

// NewSavedEntryQueryService creates a user-scoped saved-query service.
func NewSavedEntryQueryService(filesystem SavedEntryQueryFilesystem, userHomePath string) (*SavedEntryQueryService, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	userHomePath = normalizeSavedEntryQueryHomePath(userHomePath)
	if userHomePath == "" {
		return nil, ErrInvalidUserHome
	}

	fileService, err := userpersist.NewFileService(filesystem)
	if err != nil {
		return nil, err
	}

	collectionPath, err := userpersist.CategoryPath(userHomePath, SavedEntryQueryCategory)
	if err != nil {
		return nil, err
	}

	return &SavedEntryQueryService{
		filesystem:     filesystem,
		files:          fileService,
		userHomePath:   userHomePath,
		collectionPath: collectionPath,
	}, nil
}

// UserHomePath returns the normalized user home path used by the service.
func (service *SavedEntryQueryService) UserHomePath() string {
	return service.userHomePath
}

// CollectionPath returns the expected saved-query collection path.
func (service *SavedEntryQueryService) CollectionPath() string {
	return service.collectionPath
}

// Ensure verifies the saved-query collection exists.
func (service *SavedEntryQueryService) Ensure() (string, error) {
	if service == nil || service.files == nil {
		return "", ErrMissingFilesystem
	}

	collectionPath, err := service.files.EnsureContext(service.userHomePath, SavedEntryQueryCategory)
	if err != nil {
		return "", err
	}
	service.collectionPath = collectionPath
	return collectionPath, nil
}

// Path returns the expected data object path for a saved query ID.
func (service *SavedEntryQueryService) Path(id string) (string, error) {
	id, err := normalizeSavedEntryQueryID(id)
	if err != nil {
		return "", err
	}
	return path.Join(service.collectionPath, savedEntryQueryFileName(id)), nil
}

// CreateSavedQuery stores a new saved query with an auto-generated ID.
func (service *SavedEntryQueryService) CreateSavedQuery(name string, definition EntryQueryDefinition) (SavedEntryQuery, error) {
	return service.CreateSavedQueryWithDescription(name, "", definition)
}

// CreateSavedQueryWithDescription stores a new saved query with an
// auto-generated ID and description.
func (service *SavedEntryQueryService) CreateSavedQueryWithDescription(name string, description string, definition EntryQueryDefinition) (SavedEntryQuery, error) {
	now := savedEntryQueryNow()
	saved := SavedEntryQuery{
		Version:     SavedEntryQueryVersion,
		ID:          xid.New().String(),
		Name:        name,
		Description: description,
		Query:       definition,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := service.writeSavedQuery(saved); err != nil {
		return SavedEntryQuery{}, err
	}
	return canonicalSavedEntryQuery(saved)
}

// PutSavedQuery replaces an existing saved query while preserving its CreatedAt
// timestamp.
func (service *SavedEntryQueryService) PutSavedQuery(id string, update SavedEntryQueryUpdate) (SavedEntryQuery, error) {
	existing, err := service.GetSavedQuery(id)
	if err != nil {
		return SavedEntryQuery{}, err
	}

	saved := SavedEntryQuery{
		Version:     SavedEntryQueryVersion,
		ID:          existing.ID,
		Name:        update.Name,
		Description: update.Description,
		Query:       update.Query,
		CreatedAt:   existing.CreatedAt,
		UpdatedAt:   savedEntryQueryNow(),
	}
	if err := service.writeSavedQuery(saved); err != nil {
		return SavedEntryQuery{}, err
	}
	return canonicalSavedEntryQuery(saved)
}

// GetSavedQuery reads and validates a saved query by ID.
func (service *SavedEntryQueryService) GetSavedQuery(id string) (SavedEntryQuery, error) {
	id, err := normalizeSavedEntryQueryID(id)
	if err != nil {
		return SavedEntryQuery{}, err
	}

	fileName := savedEntryQueryFileName(id)
	file, err := service.files.GetFile(service.userHomePath, SavedEntryQueryCategory, fileName)
	if err != nil {
		return SavedEntryQuery{}, fmt.Errorf("read saved entry query %q: %w", id, err)
	}

	saved, err := ParseSavedEntryQuery(file.Contents)
	if err != nil {
		return SavedEntryQuery{}, fmt.Errorf("parse saved entry query %q: %w", file.IRODSPath, err)
	}
	if saved.ID != id {
		return SavedEntryQuery{}, fmt.Errorf("%w: saved query file %q contains id %q", ErrInvalidSavedEntryQuery, file.IRODSPath, saved.ID)
	}
	return saved, nil
}

// ListSavedQueries lists saved queries in deterministic display order.
func (service *SavedEntryQueryService) ListSavedQueries() ([]SavedEntryQuerySummary, error) {
	collectionPath, err := service.Ensure()
	if err != nil {
		return nil, err
	}

	dataObjectPaths, err := service.filesystem.ListDataObjects(collectionPath)
	if err != nil {
		return nil, fmt.Errorf("list saved entry query objects from %q: %w", collectionPath, err)
	}

	summaries := make([]SavedEntryQuerySummary, 0, len(dataObjectPaths))
	for _, dataObjectPath := range dataObjectPaths {
		fileName := path.Base(strings.TrimSpace(dataObjectPath))
		if !strings.HasSuffix(fileName, SavedEntryQueryFileExt) {
			continue
		}

		id := strings.TrimSuffix(fileName, SavedEntryQueryFileExt)
		saved, err := service.GetSavedQuery(id)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, SavedEntryQuerySummary{
			ID:          saved.ID,
			Name:        saved.Name,
			Description: saved.Description,
			FileName:    fileName,
			IRODSPath:   dataObjectPath,
			CreatedAt:   saved.CreatedAt,
			UpdatedAt:   saved.UpdatedAt,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Name == summaries[j].Name {
			return summaries[i].ID < summaries[j].ID
		}
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}

// DeleteSavedQuery removes a saved query by ID.
func (service *SavedEntryQueryService) DeleteSavedQuery(id string, force bool) error {
	id, err := normalizeSavedEntryQueryID(id)
	if err != nil {
		return err
	}

	if err := service.files.DeleteFile(service.userHomePath, SavedEntryQueryCategory, savedEntryQueryFileName(id), force); err != nil {
		return fmt.Errorf("delete saved entry query %q: %w", id, err)
	}
	return nil
}

// ToEntryQuery reads a saved query and converts it into an executable query.
func (service *SavedEntryQueryService) ToEntryQuery(id string, options EntryQueryExecutionOptions) (EntryQuery, error) {
	saved, err := service.GetSavedQuery(id)
	if err != nil {
		return EntryQuery{}, err
	}
	return saved.Query.ToEntryQuery(options)
}

// MarshalSavedEntryQuery returns canonical saved-query JSON.
func MarshalSavedEntryQuery(saved SavedEntryQuery) ([]byte, error) {
	canonical, err := canonicalSavedEntryQuery(saved)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(canonical, "", "  ")
}

// ParseSavedEntryQuery parses and validates saved-query JSON.
func ParseSavedEntryQuery(data []byte) (SavedEntryQuery, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var payload struct {
		Version     string                `json:"version"`
		ID          string                `json:"id"`
		Name        string                `json:"name"`
		Description string                `json:"description,omitempty"`
		Query       *EntryQueryDefinition `json:"query"`
		CreatedAt   time.Time             `json:"created_at"`
		UpdatedAt   time.Time             `json:"updated_at"`
	}
	if err := decoder.Decode(&payload); err != nil {
		return SavedEntryQuery{}, fmt.Errorf("%w: parse saved entry query: %w", ErrInvalidSavedEntryQuery, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return SavedEntryQuery{}, fmt.Errorf("%w: multiple JSON values in saved entry query", ErrInvalidSavedEntryQuery)
	}
	if payload.Query == nil {
		return SavedEntryQuery{}, fmt.Errorf("%w: query is required", ErrInvalidSavedEntryQuery)
	}

	return canonicalSavedEntryQuery(SavedEntryQuery{
		Version:     payload.Version,
		ID:          payload.ID,
		Name:        payload.Name,
		Description: payload.Description,
		Query:       *payload.Query,
		CreatedAt:   payload.CreatedAt,
		UpdatedAt:   payload.UpdatedAt,
	})
}

func (service *SavedEntryQueryService) writeSavedQuery(saved SavedEntryQuery) error {
	if service == nil || service.files == nil {
		return ErrMissingFilesystem
	}

	canonical, err := canonicalSavedEntryQuery(saved)
	if err != nil {
		return err
	}

	data, err := MarshalSavedEntryQuery(canonical)
	if err != nil {
		return err
	}

	fileName := savedEntryQueryFileName(canonical.ID)
	file, err := service.files.AddOrUpdateFile(service.userHomePath, SavedEntryQueryCategory, fileName, data)
	if err != nil {
		return fmt.Errorf("write saved entry query %q: %w", canonical.ID, err)
	}
	service.collectionPath = path.Dir(file.IRODSPath)
	return nil
}

func canonicalSavedEntryQuery(saved SavedEntryQuery) (SavedEntryQuery, error) {
	if saved.Version == "" {
		saved.Version = SavedEntryQueryVersion
	}
	if saved.Version != SavedEntryQueryVersion {
		return SavedEntryQuery{}, fmt.Errorf("%w: unsupported saved entry query version %q", ErrInvalidSavedEntryQuery, saved.Version)
	}

	id, err := normalizeSavedEntryQueryID(saved.ID)
	if err != nil {
		return SavedEntryQuery{}, err
	}

	name := strings.TrimSpace(saved.Name)
	if name == "" {
		return SavedEntryQuery{}, ErrInvalidSavedEntryQueryName
	}

	query, err := canonicalEntryQueryDefinition(saved.Query)
	if err != nil {
		return SavedEntryQuery{}, err
	}

	return SavedEntryQuery{
		Version:     SavedEntryQueryVersion,
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(saved.Description),
		Query:       query,
		CreatedAt:   saved.CreatedAt.UTC(),
		UpdatedAt:   saved.UpdatedAt.UTC(),
	}, nil
}

func normalizeSavedEntryQueryHomePath(userHomePath string) string {
	userHomePath = strings.TrimSpace(userHomePath)
	if userHomePath == "" || !strings.HasPrefix(userHomePath, "/") {
		return ""
	}
	return path.Clean(userHomePath)
}

func normalizeSavedEntryQueryID(id string) (string, error) {
	id = strings.TrimSpace(id)
	id = strings.TrimSuffix(id, SavedEntryQueryFileExt)
	if id == "" || strings.Contains(id, "/") || id == "." || id == ".." || strings.Contains(id, "..") {
		return "", ErrInvalidSavedEntryQueryID
	}
	return id, nil
}

func savedEntryQueryFileName(id string) string {
	return id + SavedEntryQueryFileExt
}
