package s3admin

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
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
)

var (
	ErrMissingAccount        = errors.New("missing account")
	ErrInvalidUserAccount    = errors.New("invalid user account")
	ErrInvalidUserSecretKey  = errors.New("invalid s3 user secret key")
	ErrMissingUserKeyService = errors.New("missing s3 user key service")
)

const s3UserSecretKeyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/+.-_~"

var s3UserSecretKeyPattern = regexp.MustCompile(`^[A-Za-z0-9/+._~-]{40}$`)

// UserKeyFilesystem is the iRODS API surface needed by S3UserKeyService.
type UserKeyFilesystem = userpersist.FileFilesystem

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
	files *userpersist.FileService
}

func NewS3UserKeyService(filesystem UserKeyFilesystem) (*S3UserKeyService, error) {
	files, err := userpersist.NewFileService(filesystem)
	if err != nil {
		return nil, err
	}
	return &S3UserKeyService{files: files}, nil
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

	userKey, err := service.StoreS3UserKeyForHome(userHomePath, secretKey)
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
	if err := ValidateS3UserSecretKey(secretKey); err != nil {
		return S3UserKey{}, err
	}

	files, err := service.requireUserFileService()
	if err != nil {
		return S3UserKey{}, err
	}

	file, err := files.AddOrUpdateString(userHomePath, S3UserKeyContext, S3UserKeyFileName, secretKey)
	if err != nil {
		return S3UserKey{}, fmt.Errorf("store s3 user secret key: %w", err)
	}

	return S3UserKey{
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
	secretKey, err := GenerateS3UserSecretKey()
	if err != nil {
		return S3UserKey{}, err
	}
	return service.StoreS3UserKeyForHome(userHomePath, secretKey)
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

	return S3UserKey{
		UserHomePath: userHomePath,
		IRODSPath:    file.IRODSPath,
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

func (service *S3UserKeyService) requireUserFileService() (*userpersist.FileService, error) {
	if service == nil {
		return nil, ErrMissingUserKeyService
	}
	if service.files == nil {
		return nil, ErrMissingFilesystem
	}
	return service.files, nil
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
