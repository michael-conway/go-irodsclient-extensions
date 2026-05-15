package s3admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

const (
	// AVUBucketAttribute marks an iRODS collection as an S3 API bucket root.
	AVUBucketAttribute = "iRODS:S3:Bucket"

	metadataValueWildcard = "%"
)

var (
	ErrMissingFilesystem    = errors.New("missing filesystem")
	ErrMissingMappingFile   = errors.New("missing mapping file")
	ErrInvalidIRODSPath     = errors.New("invalid irods path")
	ErrInvalidScanRoot      = errors.New("invalid scan root")
	ErrInvalidBucketName    = errors.New("invalid bucket name")
	ErrBucketNotFound       = errors.New("bucket not found")
	ErrBucketAlreadySet     = errors.New("bucket already set for irods path")
	ErrDuplicateBucket      = errors.New("duplicate bucket")
	ErrDuplicateUserMapping = errors.New("duplicate user mapping")
)

var bucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Metadata is the AVU shape required by the bucket manager.
type Metadata struct {
	Name  string
	Value string
	Units string
}

// CollectionMetadataQueryScope controls collection AVU search scope.
type CollectionMetadataQueryScope string

const (
	CollectionMetadataQueryScopeSelf        CollectionMetadataQueryScope = "self"
	CollectionMetadataQueryScopeChildren    CollectionMetadataQueryScope = "children"
	CollectionMetadataQueryScopeDescendants CollectionMetadataQueryScope = "descendants"
)

// CollectionMetadataQueryOptions controls collection AVU search.
type CollectionMetadataQueryOptions struct {
	IRODSPath string
	Scope     CollectionMetadataQueryScope
}

// CollectionMetadataMatch contains one collection path and the matched AVUs
// from a collection metadata query.
type CollectionMetadataMatch struct {
	IRODSPath string
	Metadata  []Metadata
}

// DataObjectMetadataMatch contains one data object path and the matched AVUs
// from a data object metadata query.
type DataObjectMetadataMatch struct {
	IRODSPath string
	Metadata  []Metadata
}

// EntryType identifies the kind of filesystem entry returned by metadata search.
type EntryType string

const (
	EntryTypeCollection EntryType = "collection"
	EntryTypeDirectory  EntryType = "directory"
	EntryTypeDataObject EntryType = "data_object"
	EntryTypeFile       EntryType = "file"
)

// Entry is the minimal filesystem entry shape required by the bucket manager.
type Entry struct {
	Path string
	Type EntryType
}

// Filesystem is the collection-level iRODS API required by the bucket manager.
type Filesystem interface {
	CollectionExists(irodsPath string) (bool, error)
	QueryCollectionMetadata(metaName string, metaValue string, options CollectionMetadataQueryOptions) ([]CollectionMetadataMatch, error)
	ListCollectionMetadata(collectionPath string) ([]Metadata, error)
	AddCollectionMetadata(collectionPath string, metadata Metadata) error
	DeleteCollectionMetadata(collectionPath string, metadata Metadata) error
}

// Config controls bucket discovery and the S3 API bucket mapping file.
type Config struct {
	// ScanRootPath is the collection subtree used for duplicate detection and
	// mapping file reconciliation.
	ScanRootPath string
	// MappingFilePath is the JSON file consumed by the iRODS S3 API local-file
	// bucket mapping plugin. The file shape is {"bucket": "/irods/path"}.
	MappingFilePath string
}

// Bucket describes a managed iRODS S3 bucket.
type Bucket struct {
	Name      string `json:"name"`
	IRODSPath string `json:"irods_path"`
}

// MappingRefreshResult describes the buckets written to the S3 API bucket
// mapping file during a refresh operation.
type MappingRefreshResult struct {
	MappingFilePath string   `json:"mapping_file_path"`
	Buckets         []Bucket `json:"buckets"`
}

// UserMappingEntry is the JSON shape consumed by the iRODS S3 API local-file
// user mapping plugin.
type UserMappingEntry struct {
	SecretKey string `json:"secret_key"`
	Username  string `json:"username"`
}

// S3UserMapping describes a managed iRODS S3 API user secret mapping.
type S3UserMapping struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	SecretKey    string `json:"secret_key,omitempty"`
	UserHomePath string `json:"user_home_path,omitempty"`
	IRODSPath    string `json:"irods_path,omitempty"`
}

// UserMappingRefreshResult describes the users written to the S3 API user
// mapping file during a refresh operation.
type UserMappingRefreshResult struct {
	MappingFilePath string          `json:"mapping_file_path"`
	Users           []S3UserMapping `json:"users"`
}

// ListOptions controls bucket listing.
type ListOptions struct {
	// IRODSPath limits discovery to this collection. If empty, ScanRootPath is used.
	IRODSPath string
	// BucketName limits discovery to a specific bucket AVU value.
	BucketName string
	// Recursive controls whether descendants beyond direct children are scanned.
	Recursive bool
}

// DuplicateBucketError identifies a bucket-name collision.
type DuplicateBucketError struct {
	BucketName    string
	ExistingPath  string
	RequestedPath string
}

func (err *DuplicateBucketError) Error() string {
	if err == nil {
		return ErrDuplicateBucket.Error()
	}
	return fmt.Sprintf("duplicate bucket %q already assigned to %q", err.BucketName, err.ExistingPath)
}

func (err *DuplicateBucketError) Unwrap() error {
	return ErrDuplicateBucket
}

// DuplicateUserMappingError identifies more than one secret for the same user.
type DuplicateUserMappingError struct {
	UserID        string
	ExistingPath  string
	RequestedPath string
}

func (err *DuplicateUserMappingError) Error() string {
	if err == nil {
		return ErrDuplicateUserMapping.Error()
	}
	return fmt.Sprintf("duplicate user mapping %q already assigned to %q", err.UserID, err.ExistingPath)
}

func (err *DuplicateUserMappingError) Unwrap() error {
	return ErrDuplicateUserMapping
}

// MappingFile is a mutex-guarded reference to the shared iRODS S3 API bucket
// mapping file.
type MappingFile struct {
	mu       sync.Mutex
	filePath string
}

// UserMappingFile is a mutex-guarded reference to the shared iRODS S3 API user
// mapping file.
type UserMappingFile struct {
	mu       sync.Mutex
	filePath string
}

// NewMappingFile returns a guarded mapping file reference.
func NewMappingFile(filePath string) (*MappingFile, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, ErrMissingMappingFile
	}
	return &MappingFile{filePath: filePath}, nil
}

// Path returns the mapping file path.
func (mappingFile *MappingFile) Path() string {
	if mappingFile == nil {
		return ""
	}
	return mappingFile.filePath
}

// Load reads the current mapping file. A missing file is treated as an empty map.
func (mappingFile *MappingFile) Load() (map[string]string, error) {
	if mappingFile == nil {
		return nil, ErrMissingMappingFile
	}

	mappingFile.mu.Lock()
	defer mappingFile.mu.Unlock()

	return mappingFile.loadLocked()
}

// NewUserMappingFile returns a guarded user mapping file reference.
func NewUserMappingFile(filePath string) (*UserMappingFile, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, ErrMissingMappingFile
	}
	return &UserMappingFile{filePath: filePath}, nil
}

// Path returns the user mapping file path.
func (mappingFile *UserMappingFile) Path() string {
	if mappingFile == nil {
		return ""
	}
	return mappingFile.filePath
}

// Load reads the current user mapping file. A missing file is treated as an
// empty map.
func (mappingFile *UserMappingFile) Load() (map[string]UserMappingEntry, error) {
	if mappingFile == nil {
		return nil, ErrMissingMappingFile
	}

	mappingFile.mu.Lock()
	defer mappingFile.mu.Unlock()

	return mappingFile.loadLocked()
}

// S3Service manages S3 bucket AVUs and keeps the S3 API mapping file in sync.
type S3Service struct {
	filesystem  Filesystem
	scanRoot    string
	mappingFile *MappingFile
}

// UserMappingFilesystem is the iRODS API required for managed S3 user secret
// keys and user mapping file refreshes.
type UserMappingFilesystem interface {
	UserKeyFilesystem
	QueryDataObjectMetadata(metaName string, metaValue string) ([]DataObjectMetadataMatch, error)
}

// S3UserMappingService manages S3 user secret keys and keeps the S3 API user
// mapping file in sync.
type S3UserMappingService struct {
	filesystem  UserMappingFilesystem
	userKeys    *S3UserKeyService
	mappingFile *UserMappingFile
}

// NewS3Service creates a bucket manager service.
func NewS3Service(filesystem Filesystem, cfg Config) (*S3Service, error) {
	mappingFile, err := NewMappingFile(cfg.MappingFilePath)
	if err != nil {
		return nil, err
	}

	return NewS3ServiceWithMappingFile(filesystem, cfg.ScanRootPath, mappingFile)
}

// NewS3ServiceWithMappingFile creates a bucket manager service using a shared
// mapping file reference.
func NewS3ServiceWithMappingFile(filesystem Filesystem, scanRootPath string, mappingFile *MappingFile) (*S3Service, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}
	if mappingFile == nil {
		return nil, ErrMissingMappingFile
	}

	scanRoot := normalizeIRODSPath(scanRootPath)
	if scanRoot == "" {
		return nil, ErrInvalidScanRoot
	}

	return &S3Service{
		filesystem:  filesystem,
		scanRoot:    scanRoot,
		mappingFile: mappingFile,
	}, nil
}

// NewS3UserMappingService creates a user secret key mapping service.
func NewS3UserMappingService(filesystem UserMappingFilesystem, mappingFilePath string) (*S3UserMappingService, error) {
	mappingFile, err := NewUserMappingFile(mappingFilePath)
	if err != nil {
		return nil, err
	}
	return NewS3UserMappingServiceWithMappingFile(filesystem, mappingFile)
}

// NewS3UserMappingServiceWithMappingFile creates a user secret key mapping
// service using a shared user mapping file reference.
func NewS3UserMappingServiceWithMappingFile(filesystem UserMappingFilesystem, mappingFile *UserMappingFile) (*S3UserMappingService, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}
	if mappingFile == nil {
		return nil, ErrMissingMappingFile
	}

	userKeys, err := NewS3UserKeyService(filesystem)
	if err != nil {
		return nil, err
	}

	return &S3UserMappingService{
		filesystem:  filesystem,
		userKeys:    userKeys,
		mappingFile: mappingFile,
	}, nil
}

// ScanRootPath returns the configured duplicate-detection root.
func (service *S3Service) ScanRootPath() string {
	return service.scanRoot
}

// MappingFilePath returns the configured S3 API bucket mapping file.
func (service *S3Service) MappingFilePath() string {
	return service.mappingFile.Path()
}

// UserMappingFilePath returns the configured S3 API user mapping file.
func (service *S3UserMappingService) UserMappingFilePath() string {
	return service.mappingFile.Path()
}

// AddBucket marks an iRODS collection as an S3 bucket and refreshes the mapping file.
func (service *S3Service) AddBucket(irodsPath string, bucketName string) (Bucket, error) {
	irodsPath = normalizeIRODSPath(irodsPath)
	if irodsPath == "" {
		return Bucket{}, ErrInvalidIRODSPath
	}

	bucketName = normalizeBucketName(bucketName)
	if bucketName == "" {
		return Bucket{}, ErrInvalidBucketName
	}

	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	if err := service.ensureCollection(irodsPath); err != nil {
		return Bucket{}, err
	}

	existingForPath, err := service.bucketsForPath(irodsPath)
	if err != nil {
		return Bucket{}, err
	}

	duplicates, err := service.searchBucketsByName(bucketName, ListOptions{IRODSPath: service.scanRoot, Recursive: true})
	if err != nil {
		return Bucket{}, err
	}
	if duplicate, ok := bucketByName(duplicates, bucketName); ok && duplicate.IRODSPath != irodsPath {
		return Bucket{}, &DuplicateBucketError{
			BucketName:    bucketName,
			ExistingPath:  duplicate.IRODSPath,
			RequestedPath: irodsPath,
		}
	}

	for _, existing := range existingForPath {
		if existing.Name == bucketName {
			if err := service.writeDiscoveredMappingLocked(bucketName); err != nil {
				return Bucket{}, err
			}
			return Bucket{Name: bucketName, IRODSPath: irodsPath}, nil
		}
		return Bucket{}, ErrBucketAlreadySet
	}

	if err := service.filesystem.AddCollectionMetadata(irodsPath, Metadata{
		Name:  AVUBucketAttribute,
		Value: bucketName,
	}); err != nil {
		return Bucket{}, fmt.Errorf("add bucket metadata on %q: %w", irodsPath, err)
	}

	if err := service.writeDiscoveredMappingLocked(bucketName); err != nil {
		return Bucket{}, err
	}

	return Bucket{Name: bucketName, IRODSPath: irodsPath}, nil
}

// UpdateBucket changes the bucket name assigned to an iRODS collection.
func (service *S3Service) UpdateBucket(irodsPath string, bucketName string) (Bucket, error) {
	irodsPath = normalizeIRODSPath(irodsPath)
	if irodsPath == "" {
		return Bucket{}, ErrInvalidIRODSPath
	}

	bucketName = normalizeBucketName(bucketName)
	if bucketName == "" {
		return Bucket{}, ErrInvalidBucketName
	}

	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	if err := service.ensureCollection(irodsPath); err != nil {
		return Bucket{}, err
	}

	existingForPath, err := service.bucketsForPath(irodsPath)
	if err != nil {
		return Bucket{}, err
	}
	if len(existingForPath) == 0 {
		return Bucket{}, ErrBucketNotFound
	}

	duplicates, err := service.searchBucketsByName(bucketName, ListOptions{IRODSPath: service.scanRoot, Recursive: true})
	if err != nil {
		return Bucket{}, err
	}
	if duplicate, ok := bucketByName(duplicates, bucketName); ok && duplicate.IRODSPath != irodsPath {
		return Bucket{}, &DuplicateBucketError{
			BucketName:    bucketName,
			ExistingPath:  duplicate.IRODSPath,
			RequestedPath: irodsPath,
		}
	}

	if err := service.replaceBucketAVUs(irodsPath, bucketName); err != nil {
		return Bucket{}, err
	}

	if err := service.writeDiscoveredMappingLocked(bucketName); err != nil {
		return Bucket{}, err
	}

	return Bucket{Name: bucketName, IRODSPath: irodsPath}, nil
}

// ModifyBucket is an alias for UpdateBucket.
func (service *S3Service) ModifyBucket(irodsPath string, bucketName string) (Bucket, error) {
	return service.UpdateBucket(irodsPath, bucketName)
}

// DeleteBucket removes bucket AVUs from an iRODS collection and refreshes the mapping file.
func (service *S3Service) DeleteBucket(irodsPath string) error {
	irodsPath = normalizeIRODSPath(irodsPath)
	if irodsPath == "" {
		return ErrInvalidIRODSPath
	}

	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	if err := service.ensureCollection(irodsPath); err != nil {
		return err
	}

	bucketAVUs, err := service.bucketAVUs(irodsPath)
	if err != nil {
		return err
	}
	if len(bucketAVUs) == 0 {
		return ErrBucketNotFound
	}

	for _, avu := range bucketAVUs {
		if err := service.filesystem.DeleteCollectionMetadata(irodsPath, avu); err != nil {
			return fmt.Errorf("delete bucket metadata from %q: %w", irodsPath, err)
		}
	}

	return service.writeDiscoveredMappingLocked()
}

// ListBuckets lists buckets beneath the requested iRODS path.
func (service *S3Service) ListBuckets(options ListOptions) ([]Bucket, error) {
	return service.listBuckets(options)
}

// SearchBuckets lists buckets matching a bucket name beneath the requested iRODS path.
func (service *S3Service) SearchBuckets(bucketName string, options ListOptions) ([]Bucket, error) {
	options.BucketName = bucketName
	return service.listBuckets(options)
}

// RefreshMapping rewrites the mapping file from all bucket AVUs beneath ScanRootPath.
func (service *S3Service) RefreshMapping() error {
	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	return service.writeDiscoveredMappingLocked()
}

// RebuildMappingFromAVUs wipes and rewrites the mapping file from bucket AVUs
// currently discoverable beneath ScanRootPath. Existing mapping file contents
// are not used as a discovery aid.
func (service *S3Service) RebuildMappingFromAVUs() (MappingRefreshResult, error) {
	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	buckets, err := service.listBuckets(ListOptions{IRODSPath: service.scanRoot, Recursive: true})
	if err != nil {
		return MappingRefreshResult{}, err
	}
	buckets = sortBuckets(deduplicateBuckets(buckets))

	if err := service.writeBucketMappingLocked(buckets); err != nil {
		return MappingRefreshResult{}, err
	}

	return MappingRefreshResult{
		MappingFilePath: service.mappingFile.Path(),
		Buckets:         buckets,
	}, nil
}

// AddUserSecretKey stores a user S3 API secret key and refreshes the user
// mapping file. It behaves as an add-or-update operation for the user.
func (service *S3UserMappingService) AddUserSecretKey(account *irodstypes.IRODSAccount, secretKey string) (S3UserMapping, error) {
	return service.StoreUserSecretKey(account, secretKey)
}

// UpdateUserSecretKey stores a replacement S3 API secret key and refreshes the
// user mapping file.
func (service *S3UserMappingService) UpdateUserSecretKey(account *irodstypes.IRODSAccount, secretKey string) (S3UserMapping, error) {
	return service.StoreUserSecretKey(account, secretKey)
}

// UpdateUserSecretKeyForHomeAndUser stores a replacement S3 API secret key for
// an explicit user home path and marker user ID, then refreshes the user mapping
// file.
func (service *S3UserMappingService) UpdateUserSecretKeyForHomeAndUser(userHomePath string, userID string, secretKey string) (S3UserMapping, error) {
	return service.StoreUserSecretKeyForHomeAndUser(userHomePath, userID, secretKey)
}

// StoreUserSecretKey stores or replaces a user S3 API secret key and refreshes
// the user mapping file.
func (service *S3UserMappingService) StoreUserSecretKey(account *irodstypes.IRODSAccount, secretKey string) (S3UserMapping, error) {
	userID, err := s3UserIDFromAccount(account)
	if err != nil {
		return S3UserMapping{}, err
	}
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return S3UserMapping{}, err
	}

	return service.StoreUserSecretKeyForHomeAndUser(userHomePath, userID, secretKey)
}

// StoreUserSecretKeyForHomeAndUser stores or replaces a user S3 API secret key
// for an explicit user home path and marker user ID, then refreshes the user
// mapping file.
func (service *S3UserMappingService) StoreUserSecretKeyForHomeAndUser(userHomePath string, userID string, secretKey string) (S3UserMapping, error) {
	userID = normalizeUserID(userID)
	if userID == "" {
		return S3UserMapping{}, ErrInvalidUserID
	}

	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	userKey, err := service.userKeys.StoreS3UserKeyForHomeAndUser(userHomePath, userID, secretKey)
	if err != nil {
		return S3UserMapping{}, err
	}

	mapping, err := service.mappingFile.loadLocked()
	if err != nil {
		return S3UserMapping{}, err
	}
	mapping[userID] = UserMappingEntry{
		SecretKey: userKey.SecretKey,
		Username:  userID,
	}

	if err := service.mappingFile.replaceLocked(mapping); err != nil {
		return S3UserMapping{}, err
	}

	return s3UserMappingFromKey(userKey, userID), nil
}

// GenerateAndStoreUserSecretKey generates, stores, and maps a new user S3 API
// secret key.
func (service *S3UserMappingService) GenerateAndStoreUserSecretKey(account *irodstypes.IRODSAccount) (S3UserMapping, error) {
	userID, err := s3UserIDFromAccount(account)
	if err != nil {
		return S3UserMapping{}, err
	}
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return S3UserMapping{}, err
	}
	return service.GenerateAndStoreUserSecretKeyForHomeAndUser(userHomePath, userID)
}

// GenerateAndStoreUserSecretKeyForHomeAndUser generates, stores, and maps a new
// user S3 API secret key for an explicit user home path and marker user ID.
func (service *S3UserMappingService) GenerateAndStoreUserSecretKeyForHomeAndUser(userHomePath string, userID string) (S3UserMapping, error) {
	secretKey, err := GenerateS3UserSecretKey()
	if err != nil {
		return S3UserMapping{}, err
	}
	return service.StoreUserSecretKeyForHomeAndUser(userHomePath, userID, secretKey)
}

// GetUserSecretKey retrieves the user's current S3 API secret key from iRODS.
func (service *S3UserMappingService) GetUserSecretKey(account *irodstypes.IRODSAccount) (S3UserMapping, error) {
	userID, err := s3UserIDFromAccount(account)
	if err != nil {
		return S3UserMapping{}, err
	}
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return S3UserMapping{}, err
	}

	return service.GetUserSecretKeyForHome(userHomePath, userID)
}

// GetUserSecretKeyForHome retrieves a user's current S3 API secret key from an
// explicit user home path.
func (service *S3UserMappingService) GetUserSecretKeyForHome(userHomePath string, expectedUserID string) (S3UserMapping, error) {
	userKey, err := service.userKeys.GetS3UserKeyForHome(userHomePath)
	if err != nil {
		return S3UserMapping{}, err
	}
	userID := normalizeUserID(userKey.UserName)
	if userID == "" {
		userID = normalizeUserID(expectedUserID)
	}
	if userID == "" {
		return S3UserMapping{}, ErrInvalidUserID
	}
	return s3UserMappingFromKey(userKey, userID), nil
}

// ListUserSecretMappingsFromAVUs returns user secret mappings discovered from
// current iRODS:S3:Secret marker AVUs without rewriting the mapping file.
func (service *S3UserMappingService) ListUserSecretMappingsFromAVUs() ([]S3UserMapping, error) {
	return service.discoverUserSecretMappings()
}

// DeleteUserSecretKey deletes the user's S3 API secret key and removes it from
// the user mapping file.
func (service *S3UserMappingService) DeleteUserSecretKey(account *irodstypes.IRODSAccount) error {
	userID, err := s3UserIDFromAccount(account)
	if err != nil {
		return err
	}
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return err
	}

	return service.DeleteUserSecretKeyForHomeAndUser(userHomePath, userID)
}

// DeleteUserSecretKeyForHomeAndUser deletes a user's S3 API secret key for an
// explicit user home path and removes the user ID from the mapping file.
func (service *S3UserMappingService) DeleteUserSecretKeyForHomeAndUser(userHomePath string, userID string) error {
	userID = normalizeUserID(userID)
	if userID == "" {
		return ErrInvalidUserID
	}

	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	if err := service.userKeys.DeleteS3UserKeyForHome(userHomePath); err != nil {
		return err
	}

	mapping, err := service.mappingFile.loadLocked()
	if err != nil {
		return err
	}
	delete(mapping, userID)
	return service.mappingFile.replaceLocked(mapping)
}

// RefreshUserMapping rewrites the user mapping file from all marked S3 user
// secret key data objects.
func (service *S3UserMappingService) RefreshUserMapping() error {
	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	return service.writeDiscoveredUserMappingLocked()
}

// RebuildUserMappingFromAVUs wipes and rewrites the user mapping file from
// marker AVUs currently discoverable by metadata search. Existing mapping file
// contents are not used as a discovery aid.
func (service *S3UserMappingService) RebuildUserMappingFromAVUs() (UserMappingRefreshResult, error) {
	service.mappingFile.mu.Lock()
	defer service.mappingFile.mu.Unlock()

	users, err := service.discoverUserSecretMappings()
	if err != nil {
		return UserMappingRefreshResult{}, err
	}
	users = sortUserMappings(deduplicateUserMappings(users))

	if err := service.writeUserMappingLocked(users); err != nil {
		return UserMappingRefreshResult{}, err
	}

	return UserMappingRefreshResult{
		MappingFilePath: service.mappingFile.Path(),
		Users:           users,
	}, nil
}

func (service *S3Service) listBuckets(options ListOptions) ([]Bucket, error) {
	startPath := normalizeIRODSPath(options.IRODSPath)
	if startPath == "" {
		startPath = service.scanRoot
	}

	if err := service.ensureCollection(startPath); err != nil {
		return nil, err
	}

	bucketName := normalizeOptionalBucketName(options.BucketName)
	if strings.TrimSpace(options.BucketName) != "" && bucketName == "" {
		return nil, ErrInvalidBucketName
	}

	if bucketName != "" {
		return service.searchBucketsByName(bucketName, ListOptions{
			IRODSPath: startPath,
			Recursive: options.Recursive,
		})
	}

	buckets, err := service.searchBucketsByMetadataValue(metadataValueWildcard, ListOptions{
		IRODSPath: startPath,
		Recursive: options.Recursive,
	})
	return sortBuckets(deduplicateBuckets(buckets)), err
}

func (service *S3Service) writeDiscoveredMappingLocked(extraBucketNames ...string) error {
	buckets, err := service.listBuckets(ListOptions{IRODSPath: service.scanRoot, Recursive: true})
	if err != nil {
		return err
	}

	knownMapping, err := service.mappingFile.loadLocked()
	if err != nil {
		return err
	}

	bucketNames := make([]string, 0, len(knownMapping)+len(extraBucketNames))
	for bucketName := range knownMapping {
		bucketNames = append(bucketNames, bucketName)
	}
	for _, bucketName := range extraBucketNames {
		bucketName = normalizeBucketName(bucketName)
		if bucketName != "" {
			bucketNames = append(bucketNames, bucketName)
		}
	}

	for _, bucketName := range bucketNames {
		matchingBuckets, err := service.searchBucketsByName(bucketName, ListOptions{IRODSPath: service.scanRoot, Recursive: true})
		if err != nil {
			return err
		}
		buckets = append(buckets, matchingBuckets...)
	}

	buckets = sortBuckets(deduplicateBuckets(buckets))

	return service.writeBucketMappingLocked(buckets)
}

func (service *S3Service) writeBucketMappingLocked(buckets []Bucket) error {
	mapping := map[string]string{}
	for _, bucket := range buckets {
		if existingPath, ok := mapping[bucket.Name]; ok && existingPath != bucket.IRODSPath {
			return &DuplicateBucketError{
				BucketName:    bucket.Name,
				ExistingPath:  existingPath,
				RequestedPath: bucket.IRODSPath,
			}
		}
		mapping[bucket.Name] = bucket.IRODSPath
	}

	return service.mappingFile.replaceLocked(mapping)
}

func (service *S3UserMappingService) writeDiscoveredUserMappingLocked() error {
	users, err := service.discoverUserSecretMappings()
	if err != nil {
		return err
	}
	users = sortUserMappings(deduplicateUserMappings(users))
	return service.writeUserMappingLocked(users)
}

func (service *S3UserMappingService) writeUserMappingLocked(users []S3UserMapping) error {
	mapping := map[string]UserMappingEntry{}
	pathByUserID := map[string]string{}
	for _, user := range users {
		userID := normalizeUserID(user.UserID)
		if userID == "" {
			userID = normalizeUserID(user.Username)
		}
		if userID == "" || strings.TrimSpace(user.SecretKey) == "" {
			continue
		}
		if existingPath, ok := pathByUserID[userID]; ok && existingPath != user.IRODSPath {
			return &DuplicateUserMappingError{
				UserID:        userID,
				ExistingPath:  existingPath,
				RequestedPath: user.IRODSPath,
			}
		}
		pathByUserID[userID] = user.IRODSPath
		mapping[userID] = UserMappingEntry{
			SecretKey: user.SecretKey,
			Username:  userID,
		}
	}

	return service.mappingFile.replaceLocked(mapping)
}

func (service *S3UserMappingService) discoverUserSecretMappings() ([]S3UserMapping, error) {
	matches, err := service.filesystem.QueryDataObjectMetadata(AVUSecretAttribute, metadataValueWildcard)
	if err != nil {
		return nil, fmt.Errorf("search s3 user secret metadata %q=%q: %w", AVUSecretAttribute, metadataValueWildcard, err)
	}

	users := make([]S3UserMapping, 0, len(matches))
	for _, match := range matches {
		secretPath := normalizeIRODSPath(match.IRODSPath)
		if secretPath == "" {
			continue
		}

		userID, err := userIDFromSecretMarkerAVUs(match.Metadata)
		if err != nil {
			return nil, err
		}

		contents, err := readDataObjectForUser(service.filesystem, secretPath, userID)
		if err != nil {
			return nil, fmt.Errorf("read s3 user secret key %q: %w", secretPath, err)
		}

		secretKey := strings.TrimSpace(string(contents))
		if err := ValidateS3UserSecretKey(secretKey); err != nil {
			return nil, fmt.Errorf("stored s3 user secret key %q: %w", secretPath, err)
		}

		users = append(users, s3UserMappingFromKey(S3UserKey{
			UserName:     userID,
			UserHomePath: userHomePathFromSecretKeyPath(secretPath),
			IRODSPath:    secretPath,
			SecretKey:    secretKey,
		}, userID))
	}
	return sortUserMappings(deduplicateUserMappings(users)), nil
}

func (service *S3Service) searchBucketsByName(bucketName string, options ListOptions) ([]Bucket, error) {
	bucketName = normalizeBucketName(bucketName)
	if bucketName == "" {
		return nil, ErrInvalidBucketName
	}
	options.BucketName = bucketName
	return service.searchBucketsByMetadataValue(bucketName, options)
}

func (service *S3Service) searchBucketsByMetadataValue(metaValue string, options ListOptions) ([]Bucket, error) {
	startPath := normalizeIRODSPath(options.IRODSPath)
	if startPath == "" {
		startPath = service.scanRoot
	}

	matches, err := service.queryBucketMetadata(metaValue, startPath, options.Recursive)
	if err != nil {
		return nil, err
	}

	bucketNameFilter := normalizeOptionalBucketName(options.BucketName)
	buckets := make([]Bucket, 0, len(matches))
	for _, match := range matches {
		collectionPath := normalizeIRODSPath(match.IRODSPath)
		if collectionPath == "" {
			continue
		}

		for _, avu := range match.Metadata {
			if avu.Name != AVUBucketAttribute {
				continue
			}
			bucketName := normalizeBucketName(avu.Value)
			if bucketName == "" || (bucketNameFilter != "" && bucketName != bucketNameFilter) {
				continue
			}
			buckets = append(buckets, Bucket{
				Name:      bucketName,
				IRODSPath: collectionPath,
			})
		}
	}

	return sortBuckets(deduplicateBuckets(buckets)), nil
}

func (service *S3Service) queryBucketMetadata(metaValue string, startPath string, recursive bool) ([]CollectionMetadataMatch, error) {
	scopes := []CollectionMetadataQueryScope{
		CollectionMetadataQueryScopeSelf,
		CollectionMetadataQueryScopeChildren,
	}
	if recursive {
		scopes[1] = CollectionMetadataQueryScopeDescendants
	}

	metadataByPath := map[string][]Metadata{}
	paths := []string{}
	for _, scope := range scopes {
		matches, err := service.filesystem.QueryCollectionMetadata(AVUBucketAttribute, metaValue, CollectionMetadataQueryOptions{
			IRODSPath: startPath,
			Scope:     scope,
		})
		if err != nil {
			return nil, fmt.Errorf("search bucket metadata %q=%q path=%q scope=%q: %w", AVUBucketAttribute, metaValue, startPath, scope, err)
		}
		for _, match := range matches {
			collectionPath := normalizeIRODSPath(match.IRODSPath)
			if collectionPath == "" {
				continue
			}
			if _, ok := metadataByPath[collectionPath]; !ok {
				paths = append(paths, collectionPath)
			}
			for _, avu := range match.Metadata {
				metadataByPath[collectionPath] = appendUniqueMetadata(metadataByPath[collectionPath], avu)
			}
		}
	}

	sort.Strings(paths)
	result := make([]CollectionMetadataMatch, 0, len(paths))
	for _, collectionPath := range paths {
		result = append(result, CollectionMetadataMatch{
			IRODSPath: collectionPath,
			Metadata:  metadataByPath[collectionPath],
		})
	}
	return result, nil
}

func appendUniqueMetadata(existing []Metadata, avu Metadata) []Metadata {
	for _, candidate := range existing {
		if candidate == avu {
			return existing
		}
	}
	return append(existing, avu)
}

func (service *S3Service) replaceBucketAVUs(irodsPath string, bucketName string) error {
	bucketAVUs, err := service.bucketAVUs(irodsPath)
	if err != nil {
		return err
	}
	for _, avu := range bucketAVUs {
		if err := service.filesystem.DeleteCollectionMetadata(irodsPath, avu); err != nil {
			return fmt.Errorf("delete existing bucket metadata from %q: %w", irodsPath, err)
		}
	}

	if err := service.filesystem.AddCollectionMetadata(irodsPath, Metadata{
		Name:  AVUBucketAttribute,
		Value: bucketName,
	}); err != nil {
		return fmt.Errorf("add bucket metadata on %q: %w", irodsPath, err)
	}

	return nil
}

func (service *S3Service) bucketsForPath(irodsPath string) ([]Bucket, error) {
	bucketAVUs, err := service.bucketAVUs(irodsPath)
	if err != nil {
		return nil, err
	}

	buckets := make([]Bucket, 0, len(bucketAVUs))
	for _, avu := range bucketAVUs {
		bucketName := normalizeBucketName(avu.Value)
		if bucketName == "" {
			continue
		}
		buckets = append(buckets, Bucket{
			Name:      bucketName,
			IRODSPath: irodsPath,
		})
	}
	return sortBuckets(deduplicateBuckets(buckets)), nil
}

func (service *S3Service) bucketAVUs(irodsPath string) ([]Metadata, error) {
	metadata, err := service.filesystem.ListCollectionMetadata(irodsPath)
	if err != nil {
		return nil, fmt.Errorf("list bucket metadata for %q: %w", irodsPath, err)
	}

	bucketAVUs := make([]Metadata, 0)
	for _, avu := range metadata {
		if avu.Name == AVUBucketAttribute && normalizeBucketName(avu.Value) != "" {
			bucketAVUs = append(bucketAVUs, avu)
		}
	}
	return bucketAVUs, nil
}

func (entry Entry) IsCollection() bool {
	return entry.Type == EntryTypeCollection || entry.Type == EntryTypeDirectory
}

func (entry Entry) IsDataObject() bool {
	return entry.Type == EntryTypeDataObject || entry.Type == EntryTypeFile
}

func (service *S3Service) ensureCollection(irodsPath string) error {
	exists, err := service.filesystem.CollectionExists(irodsPath)
	if err != nil {
		return fmt.Errorf("check collection %q: %w", irodsPath, err)
	}
	if !exists {
		return ErrInvalidIRODSPath
	}
	return nil
}

func (mappingFile *MappingFile) loadLocked() (map[string]string, error) {
	contents, err := os.ReadFile(mappingFile.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read mapping file %q: %w", mappingFile.filePath, err)
	}

	if len(strings.TrimSpace(string(contents))) == 0 {
		return map[string]string{}, nil
	}

	mapping := map[string]string{}
	if err := json.Unmarshal(contents, &mapping); err != nil {
		return nil, fmt.Errorf("decode mapping file %q: %w", mappingFile.filePath, err)
	}

	normalized := map[string]string{}
	for bucketName, irodsPath := range mapping {
		bucketName = normalizeBucketName(bucketName)
		irodsPath = normalizeIRODSPath(irodsPath)
		if bucketName == "" || irodsPath == "" {
			continue
		}
		normalized[bucketName] = irodsPath
	}
	return normalized, nil
}

func (mappingFile *MappingFile) replaceLocked(mapping map[string]string) error {
	normalized := map[string]string{}
	for bucketName, irodsPath := range mapping {
		bucketName = normalizeBucketName(bucketName)
		irodsPath = normalizeIRODSPath(irodsPath)
		if bucketName == "" || irodsPath == "" {
			continue
		}
		normalized[bucketName] = irodsPath
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(mappingFile.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mapping file directory %q: %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, ".s3-bucket-mapping-*.json")
	if err != nil {
		return fmt.Errorf("create temporary mapping file in %q: %w", dir, err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath) //nolint

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close() //nolint
		return fmt.Errorf("write temporary mapping file %q: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary mapping file %q: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, mappingFile.filePath); err != nil {
		return fmt.Errorf("replace mapping file %q: %w", mappingFile.filePath, err)
	}

	return nil
}

func (mappingFile *UserMappingFile) loadLocked() (map[string]UserMappingEntry, error) {
	contents, err := os.ReadFile(mappingFile.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]UserMappingEntry{}, nil
		}
		return nil, fmt.Errorf("read user mapping file %q: %w", mappingFile.filePath, err)
	}

	if len(strings.TrimSpace(string(contents))) == 0 {
		return map[string]UserMappingEntry{}, nil
	}

	mapping := map[string]UserMappingEntry{}
	if err := json.Unmarshal(contents, &mapping); err != nil {
		return nil, fmt.Errorf("decode user mapping file %q: %w", mappingFile.filePath, err)
	}

	normalized := map[string]UserMappingEntry{}
	for userID, entry := range mapping {
		userID = normalizeUserID(userID)
		username := normalizeUserID(entry.Username)
		secretKey := strings.TrimSpace(entry.SecretKey)
		if userID == "" {
			userID = username
		}
		if username == "" {
			username = userID
		}
		if userID == "" || username == "" || secretKey == "" {
			continue
		}
		normalized[userID] = UserMappingEntry{
			SecretKey: secretKey,
			Username:  username,
		}
	}
	return normalized, nil
}

func (mappingFile *UserMappingFile) replaceLocked(mapping map[string]UserMappingEntry) error {
	normalized := map[string]UserMappingEntry{}
	for userID, entry := range mapping {
		userID = normalizeUserID(userID)
		username := normalizeUserID(entry.Username)
		secretKey := strings.TrimSpace(entry.SecretKey)
		if userID == "" {
			userID = username
		}
		if username == "" {
			username = userID
		}
		if userID == "" || username == "" || secretKey == "" {
			continue
		}
		normalized[userID] = UserMappingEntry{
			SecretKey: secretKey,
			Username:  username,
		}
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(mappingFile.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create user mapping file directory %q: %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, ".s3-user-mapping-*.json")
	if err != nil {
		return fmt.Errorf("create temporary user mapping file in %q: %w", dir, err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath) //nolint

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close() //nolint
		return fmt.Errorf("write temporary user mapping file %q: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary user mapping file %q: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, mappingFile.filePath); err != nil {
		return fmt.Errorf("replace user mapping file %q: %w", mappingFile.filePath, err)
	}

	return nil
}

func bucketByName(buckets []Bucket, bucketName string) (Bucket, bool) {
	for _, bucket := range buckets {
		if bucket.Name == bucketName {
			return bucket, true
		}
	}
	return Bucket{}, false
}

func sortBuckets(buckets []Bucket) []Bucket {
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Name == buckets[j].Name {
			return buckets[i].IRODSPath < buckets[j].IRODSPath
		}
		return buckets[i].Name < buckets[j].Name
	})
	return buckets
}

func deduplicateBuckets(buckets []Bucket) []Bucket {
	seen := map[Bucket]struct{}{}
	result := make([]Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.Name == "" || bucket.IRODSPath == "" {
			continue
		}
		if _, ok := seen[bucket]; ok {
			continue
		}
		seen[bucket] = struct{}{}
		result = append(result, bucket)
	}
	return result
}

func s3UserMappingFromKey(userKey S3UserKey, userID string) S3UserMapping {
	userID = normalizeUserID(userID)
	if userID == "" {
		userID = normalizeUserID(userKey.UserName)
	}
	return S3UserMapping{
		UserID:       userID,
		Username:     userID,
		SecretKey:    userKey.SecretKey,
		UserHomePath: userKey.UserHomePath,
		IRODSPath:    userKey.IRODSPath,
	}
}

func sortUserMappings(users []S3UserMapping) []S3UserMapping {
	sort.Slice(users, func(i, j int) bool {
		if users[i].UserID == users[j].UserID {
			return users[i].IRODSPath < users[j].IRODSPath
		}
		return users[i].UserID < users[j].UserID
	})
	return users
}

func deduplicateUserMappings(users []S3UserMapping) []S3UserMapping {
	seen := map[S3UserMapping]struct{}{}
	result := make([]S3UserMapping, 0, len(users))
	for _, user := range users {
		if user.UserID == "" || user.SecretKey == "" {
			continue
		}
		if _, ok := seen[user]; ok {
			continue
		}
		seen[user] = struct{}{}
		result = append(result, user)
	}
	return result
}

func pathWithinScope(candidatePath string, startPath string, recursive bool) bool {
	candidatePath = normalizeIRODSPath(candidatePath)
	startPath = normalizeIRODSPath(startPath)
	if candidatePath == "" || startPath == "" {
		return false
	}
	if candidatePath == startPath {
		return true
	}
	if recursive {
		return strings.HasPrefix(candidatePath, strings.TrimSuffix(startPath, "/")+"/")
	}
	return path.Dir(candidatePath) == startPath
}

func normalizeIRODSPath(irodsPath string) string {
	irodsPath = strings.TrimSpace(irodsPath)
	if irodsPath == "" || !strings.HasPrefix(irodsPath, "/") {
		return ""
	}
	return path.Clean(irodsPath)
}

func normalizeOptionalBucketName(bucketName string) string {
	if strings.TrimSpace(bucketName) == "" {
		return ""
	}
	return normalizeBucketName(bucketName)
}

func normalizeBucketName(bucketName string) string {
	bucketName = strings.TrimSpace(strings.ToLower(bucketName))
	if !isValidBucketName(bucketName) {
		return ""
	}
	return bucketName
}

func isValidBucketName(bucketName string) bool {
	if !bucketNamePattern.MatchString(bucketName) {
		return false
	}
	if strings.Contains(bucketName, "..") || strings.Contains(bucketName, ".-") || strings.Contains(bucketName, "-.") {
		return false
	}
	return net.ParseIP(bucketName) == nil
}
