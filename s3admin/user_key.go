package s3admin

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"path"
	"regexp"
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/userpersist"
)

const (
	// S3UserKeyContext is the userpersist context collection used for S3 API
	// user key material.
	S3UserKeyContext = "s3admin"
	// S3UserKeyCategory is retained for callers using the original category name.
	S3UserKeyCategory = S3UserKeyContext
	// S3UserKeyFileName is the data object containing the user's S3 API secret.
	S3UserKeyFileName = "irods-s3-api-secret.txt"
	// S3UserSecretKeyLength is the required length for S3 API user secrets.
	S3UserSecretKeyLength = 40
	// AVUSecretAttribute marks an iRODS S3 API secret key data object.
	AVUSecretAttribute = "iRODS:S3:Secret"
)

var (
	ErrMissingAccount                = errors.New("missing account")
	ErrInvalidUserAccount            = errors.New("invalid user account")
	ErrInvalidUserID                 = errors.New("invalid user id")
	ErrInvalidUserSecretKey          = errors.New("invalid s3 user secret key")
	ErrMissingUserKeyService         = errors.New("missing s3 user key service")
	ErrUserSecretMarkerNotFound      = errors.New("s3 user secret marker not found")
	ErrDuplicateUserSecretMarker     = errors.New("duplicate s3 user secret marker")
	ErrInvalidUserSecretKeyIRODSPath = errors.New("invalid s3 user secret key irods path")
)

const s3UserSecretKeyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/+.-_~"

var s3UserSecretKeyPattern = regexp.MustCompile(`^[A-Za-z0-9/+._~-]{40}$`)

// UserKeyFilesystem is the iRODS API surface needed by S3UserKeyService.
type UserKeyFilesystem interface {
	userpersist.FileFilesystem
	ListDataObjectMetadata(dataObjectPath string) ([]Metadata, error)
	AddDataObjectMetadata(dataObjectPath string, metadata Metadata) error
	DeleteDataObjectMetadata(dataObjectPath string, metadata Metadata) error
}

// UserKeyAsUserFilesystem optionally reads a data object through the iRODS user
// identified by a marker AVU.
type UserKeyAsUserFilesystem interface {
	ReadDataObjectAsUser(dataObjectPath string, userID string) ([]byte, error)
}

// S3UserKey describes a user's managed iRODS S3 API secret key.
type S3UserKey struct {
	UserName     string `json:"user_name,omitempty"`
	Zone         string `json:"zone,omitempty"`
	UserHomePath string `json:"user_home_path"`
	IRODSPath    string `json:"irods_path"`
	SecretKey    string `json:"secret_key,omitempty"`
}

// InvalidUserSecretKeyError describes why a supplied S3 user secret key failed
// validation without echoing the secret key back to logs.
type InvalidUserSecretKeyError struct {
	Reason string
}

func (err *InvalidUserSecretKeyError) Error() string {
	if err == nil || err.Reason == "" {
		return ErrInvalidUserSecretKey.Error()
	}
	return fmt.Sprintf("%s: %s", ErrInvalidUserSecretKey, err.Reason)
}

func (err *InvalidUserSecretKeyError) Unwrap() error {
	return ErrInvalidUserSecretKey
}

// S3UserKeyService manages per-user iRODS S3 API secret key files.
type S3UserKeyService struct {
	filesystem UserKeyFilesystem
	files      *userpersist.FileService
}

func NewS3UserKeyService(filesystem UserKeyFilesystem) (*S3UserKeyService, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	files, err := userpersist.NewFileService(filesystem)
	if err != nil {
		return nil, err
	}
	return &S3UserKeyService{
		filesystem: filesystem,
		files:      files,
	}, nil
}

// GenerateS3UserSecretKey generates a secret key that satisfies the iRODS S3 API
// user secret format.
func GenerateS3UserSecretKey() (string, error) {
	builder := strings.Builder{}
	builder.Grow(S3UserSecretKeyLength)

	max := big.NewInt(int64(len(s3UserSecretKeyAlphabet)))
	for i := 0; i < S3UserSecretKeyLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate s3 user secret key: %w", err)
		}
		builder.WriteByte(s3UserSecretKeyAlphabet[n.Int64()])
	}

	return builder.String(), nil
}

// ValidateS3UserSecretKey verifies the supplied key matches the iRODS S3 API
// user secret format.
func ValidateS3UserSecretKey(secretKey string) error {
	if len(secretKey) != S3UserSecretKeyLength {
		return &InvalidUserSecretKeyError{Reason: fmt.Sprintf("secret key must be %d characters", S3UserSecretKeyLength)}
	}
	if !s3UserSecretKeyPattern.MatchString(secretKey) {
		return &InvalidUserSecretKeyError{Reason: "secret key contains unsupported characters"}
	}
	return nil
}

func (service *S3UserKeyService) GenerateS3UserSecretKey() (string, error) {
	return GenerateS3UserSecretKey()
}

// S3UserKeyPath returns the iRODS path to the user's S3 API secret key file.
func S3UserKeyPath(account *irodstypes.IRODSAccount) (string, error) {
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return "", err
	}
	return S3UserKeyPathForHome(userHomePath)
}

// S3UserKeyPathForHome returns the iRODS path to the S3 API secret key file for
// an explicit user home collection path.
func S3UserKeyPathForHome(userHomePath string) (string, error) {
	return userpersist.FilePath(userHomePath, S3UserKeyContext, S3UserKeyFileName)
}

// EnsureS3UserKeyStructure verifies the user's S3 API secret key collection
// exists, creating it idempotently when needed.
func (service *S3UserKeyService) EnsureS3UserKeyStructure(account *irodstypes.IRODSAccount) (string, error) {
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return "", err
	}
	return service.EnsureS3UserKeyStructureForHome(userHomePath)
}

// EnsureS3UserKeyStructureForHome verifies the S3 API secret key collection
// exists for an explicit user home collection path, creating it idempotently
// when needed. It returns the expected secret key data object path.
func (service *S3UserKeyService) EnsureS3UserKeyStructureForHome(userHomePath string) (string, error) {
	files, err := service.requireUserFileService()
	if err != nil {
		return "", err
	}
	return files.EnsureFileStructure(userHomePath, S3UserKeyContext, S3UserKeyFileName)
}

// StoreS3UserKey stores or replaces the user's S3 API secret key.
func (service *S3UserKeyService) StoreS3UserKey(account *irodstypes.IRODSAccount, secretKey string) (S3UserKey, error) {
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return S3UserKey{}, err
	}
	userID, err := s3UserIDFromAccount(account)
	if err != nil {
		return S3UserKey{}, err
	}

	userKey, err := service.StoreS3UserKeyForHomeAndUser(userHomePath, userID, secretKey)
	if err != nil {
		return S3UserKey{}, err
	}
	userKey.UserName = account.ClientUser
	userKey.Zone = account.ClientZone
	return userKey, nil
}

// StoreS3UserKeyForHome stores or replaces the S3 API secret key for an explicit
// user home collection path.
func (service *S3UserKeyService) StoreS3UserKeyForHome(userHomePath string, secretKey string) (S3UserKey, error) {
	return service.StoreS3UserKeyForHomeAndUser(userHomePath, userIDFromHomePath(userHomePath), secretKey)
}

// StoreS3UserKeyForHomeAndUser stores or replaces the S3 API secret key for an
// explicit user home collection path and marker user ID.
func (service *S3UserKeyService) StoreS3UserKeyForHomeAndUser(userHomePath string, userID string, secretKey string) (S3UserKey, error) {
	if err := ValidateS3UserSecretKey(secretKey); err != nil {
		return S3UserKey{}, err
	}
	userID = normalizeUserID(userID)
	if userID == "" {
		return S3UserKey{}, ErrInvalidUserID
	}

	files, err := service.requireUserFileService()
	if err != nil {
		return S3UserKey{}, err
	}

	file, err := files.AddOrUpdateString(userHomePath, S3UserKeyContext, S3UserKeyFileName, secretKey)
	if err != nil {
		return S3UserKey{}, fmt.Errorf("store s3 user secret key: %w", err)
	}

	if err := service.replaceSecretMarkerAVUs(file.IRODSPath, userID); err != nil {
		return S3UserKey{}, err
	}

	return S3UserKey{
		UserName:     userID,
		UserHomePath: userHomePath,
		IRODSPath:    file.IRODSPath,
		SecretKey:    secretKey,
	}, nil
}

// GenerateAndStoreS3UserKey generates, stores, and returns a new S3 API secret
// key for the user.
func (service *S3UserKeyService) GenerateAndStoreS3UserKey(account *irodstypes.IRODSAccount) (S3UserKey, error) {
	secretKey, err := GenerateS3UserSecretKey()
	if err != nil {
		return S3UserKey{}, err
	}
	return service.StoreS3UserKey(account, secretKey)
}

// GenerateAndStoreS3UserKeyForHome generates, stores, and returns a new S3 API
// secret key for an explicit user home collection path.
func (service *S3UserKeyService) GenerateAndStoreS3UserKeyForHome(userHomePath string) (S3UserKey, error) {
	return service.GenerateAndStoreS3UserKeyForHomeAndUser(userHomePath, userIDFromHomePath(userHomePath))
}

// GenerateAndStoreS3UserKeyForHomeAndUser generates, stores, and returns a new
// S3 API secret key for an explicit user home collection path and marker user
// ID.
func (service *S3UserKeyService) GenerateAndStoreS3UserKeyForHomeAndUser(userHomePath string, userID string) (S3UserKey, error) {
	secretKey, err := GenerateS3UserSecretKey()
	if err != nil {
		return S3UserKey{}, err
	}
	return service.StoreS3UserKeyForHomeAndUser(userHomePath, userID, secretKey)
}

// DeleteS3UserKey removes the user's S3 API secret key file.
func (service *S3UserKeyService) DeleteS3UserKey(account *irodstypes.IRODSAccount) error {
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return err
	}
	return service.deleteS3UserKeyForHome(userHomePath)
}

// DeleteS3UserKeyForHome removes the S3 API secret key file for an explicit
// user home collection path.
func (service *S3UserKeyService) DeleteS3UserKeyForHome(userHomePath string) error {
	return service.deleteS3UserKeyForHome(userHomePath)
}

// GetS3UserKey retrieves the user's S3 API secret key.
func (service *S3UserKeyService) GetS3UserKey(account *irodstypes.IRODSAccount) (S3UserKey, error) {
	userHomePath, err := s3UserHomePath(account)
	if err != nil {
		return S3UserKey{}, err
	}

	userKey, err := service.GetS3UserKeyForHome(userHomePath)
	if err != nil {
		return S3UserKey{}, err
	}
	userKey.UserName = account.ClientUser
	userKey.Zone = account.ClientZone
	return userKey, nil
}

// GetS3UserKeyForHome retrieves the S3 API secret key for an explicit user home
// collection path.
func (service *S3UserKeyService) GetS3UserKeyForHome(userHomePath string) (S3UserKey, error) {
	files, err := service.requireUserFileService()
	if err != nil {
		return S3UserKey{}, err
	}

	contents, file, err := files.GetString(userHomePath, S3UserKeyContext, S3UserKeyFileName)
	if err != nil {
		return S3UserKey{}, fmt.Errorf("read s3 user secret key: %w", err)
	}

	secretKey := strings.TrimSpace(contents)
	if err := ValidateS3UserSecretKey(secretKey); err != nil {
		return S3UserKey{}, fmt.Errorf("stored s3 user secret key %q: %w", file.IRODSPath, err)
	}

	userID, err := service.userIDForSecretKeyPath(file.IRODSPath)
	if err != nil {
		return S3UserKey{}, err
	}

	return S3UserKey{
		UserName:     userID,
		UserHomePath: userHomePath,
		IRODSPath:    file.IRODSPath,
		SecretKey:    secretKey,
	}, nil
}

// GetS3UserKeyAtPath retrieves an S3 API secret key from an explicit iRODS data
// object path and reads the marker AVU to identify the user.
func (service *S3UserKeyService) GetS3UserKeyAtPath(secretPath string) (S3UserKey, error) {
	filesystem, err := service.requireUserKeyFilesystem()
	if err != nil {
		return S3UserKey{}, err
	}

	secretPath = normalizeIRODSPath(secretPath)
	if secretPath == "" {
		return S3UserKey{}, ErrInvalidUserSecretKeyIRODSPath
	}

	userID, err := service.userIDForSecretKeyPath(secretPath)
	if err != nil {
		return S3UserKey{}, err
	}

	contents, err := readDataObjectForUser(filesystem, secretPath, userID)
	if err != nil {
		return S3UserKey{}, fmt.Errorf("read s3 user secret key %q: %w", secretPath, err)
	}

	secretKey := strings.TrimSpace(string(contents))
	if err := ValidateS3UserSecretKey(secretKey); err != nil {
		return S3UserKey{}, fmt.Errorf("stored s3 user secret key %q: %w", secretPath, err)
	}

	return S3UserKey{
		UserName:     userID,
		UserHomePath: userHomePathFromSecretKeyPath(secretPath),
		IRODSPath:    secretPath,
		SecretKey:    secretKey,
	}, nil
}

func (service *S3UserKeyService) deleteS3UserKeyForHome(userHomePath string) error {
	files, err := service.requireUserFileService()
	if err != nil {
		return err
	}

	secretPath, pathErr := S3UserKeyPathForHome(userHomePath)
	if pathErr != nil {
		return pathErr
	}

	if err := files.DeleteFile(userHomePath, S3UserKeyContext, S3UserKeyFileName, true); err != nil {
		return fmt.Errorf("delete s3 user secret key %q: %w", secretPath, err)
	}
	return nil
}

func (service *S3UserKeyService) replaceSecretMarkerAVUs(secretPath string, userID string) error {
	filesystem, err := service.requireUserKeyFilesystem()
	if err != nil {
		return err
	}

	secretPath = normalizeIRODSPath(secretPath)
	if secretPath == "" {
		return ErrInvalidUserSecretKeyIRODSPath
	}

	markers, err := service.secretMarkerAVUs(secretPath)
	if err != nil {
		return err
	}
	for _, marker := range markers {
		if err := filesystem.DeleteDataObjectMetadata(secretPath, marker); err != nil {
			return fmt.Errorf("delete existing s3 secret marker from %q: %w", secretPath, err)
		}
	}

	if err := filesystem.AddDataObjectMetadata(secretPath, Metadata{
		Name:  AVUSecretAttribute,
		Value: userID,
	}); err != nil {
		return fmt.Errorf("add s3 secret marker to %q: %w", secretPath, err)
	}
	return nil
}

func (service *S3UserKeyService) userIDForSecretKeyPath(secretPath string) (string, error) {
	markers, err := service.secretMarkerAVUs(secretPath)
	if err != nil {
		return "", err
	}
	if len(markers) == 0 {
		return "", ErrUserSecretMarkerNotFound
	}

	userIDs := map[string]struct{}{}
	for _, marker := range markers {
		userID := normalizeUserID(marker.Value)
		if userID != "" {
			userIDs[userID] = struct{}{}
		}
	}
	if len(userIDs) == 0 {
		return "", ErrUserSecretMarkerNotFound
	}
	if len(userIDs) > 1 {
		return "", ErrDuplicateUserSecretMarker
	}
	for userID := range userIDs {
		return userID, nil
	}
	return "", ErrUserSecretMarkerNotFound
}

func readDataObjectForUser(filesystem UserKeyFilesystem, dataObjectPath string, userID string) ([]byte, error) {
	if filesystemWithUser, ok := filesystem.(UserKeyAsUserFilesystem); ok {
		return filesystemWithUser.ReadDataObjectAsUser(dataObjectPath, userID)
	}
	return filesystem.ReadDataObject(dataObjectPath)
}

func (service *S3UserKeyService) secretMarkerAVUs(secretPath string) ([]Metadata, error) {
	filesystem, err := service.requireUserKeyFilesystem()
	if err != nil {
		return nil, err
	}

	metadata, err := filesystem.ListDataObjectMetadata(secretPath)
	if err != nil {
		return nil, fmt.Errorf("list s3 user secret marker metadata for %q: %w", secretPath, err)
	}

	markers := make([]Metadata, 0)
	for _, avu := range metadata {
		if avu.Name == AVUSecretAttribute {
			markers = append(markers, avu)
		}
	}
	return markers, nil
}

func (service *S3UserKeyService) requireUserFileService() (*userpersist.FileService, error) {
	if service == nil {
		return nil, ErrMissingUserKeyService
	}
	if service.files == nil {
		return nil, ErrMissingFilesystem
	}
	return service.files, nil
}

func (service *S3UserKeyService) requireUserKeyFilesystem() (UserKeyFilesystem, error) {
	if service == nil {
		return nil, ErrMissingUserKeyService
	}
	if service.filesystem == nil {
		return nil, ErrMissingFilesystem
	}
	return service.filesystem, nil
}

func s3UserHomePath(account *irodstypes.IRODSAccount) (string, error) {
	if account == nil {
		return "", ErrMissingAccount
	}
	if strings.TrimSpace(account.ClientUser) == "" || strings.TrimSpace(account.ClientZone) == "" {
		return "", ErrInvalidUserAccount
	}

	userHomePath := account.GetHomeDirPath()
	if strings.TrimSpace(userHomePath) == "" {
		return "", ErrInvalidUserAccount
	}
	return userHomePath, nil
}

func s3UserIDFromAccount(account *irodstypes.IRODSAccount) (string, error) {
	if account == nil {
		return "", ErrMissingAccount
	}
	userID := normalizeUserID(account.ClientUser)
	if userID == "" {
		return "", ErrInvalidUserID
	}
	return userID, nil
}

func normalizeUserID(userID string) string {
	return strings.TrimSpace(userID)
}

func userIDFromHomePath(userHomePath string) string {
	userHomePath = normalizeIRODSPath(userHomePath)
	if userHomePath == "" {
		return ""
	}
	return normalizeUserID(path.Base(userHomePath))
}

func userHomePathFromSecretKeyPath(secretPath string) string {
	secretPath = normalizeIRODSPath(secretPath)
	if secretPath == "" || path.Base(secretPath) != S3UserKeyFileName {
		return ""
	}

	categoryPath := path.Dir(secretPath)
	if path.Base(categoryPath) != S3UserKeyContext {
		return ""
	}

	rootPath := path.Dir(categoryPath)
	if path.Base(rootPath) != userpersist.RootCollectionName {
		return ""
	}

	return path.Dir(rootPath)
}
