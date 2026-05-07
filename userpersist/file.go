package userpersist

import "fmt"

// FileFilesystem is the minimal iRODS API required for managing files under a
// userpersist context.
type FileFilesystem interface {
	CollectionFilesystem
	ReadDataObject(dataObjectPath string) ([]byte, error)
	WriteDataObject(dataObjectPath string, contents []byte) error
	DeleteDataObject(dataObjectPath string, force bool) error
}

// File describes a data object managed under a userpersist context.
type File struct {
	UserHomePath string `json:"user_home_path"`
	Context      string `json:"context"`
	Name         string `json:"name"`
	IRODSPath    string `json:"irods_path"`
	Contents     []byte `json:"contents,omitempty"`
}

// FileService manages files beneath ~/.irodsext/<context>.
type FileService struct {
	filesystem FileFilesystem
}

func NewFileService(filesystem FileFilesystem) (*FileService, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}
	return &FileService{filesystem: filesystem}, nil
}

// Path returns the expected data object path for a file under a userpersist
// context.
func (service *FileService) Path(userHomePath string, persistContext string, fileName string) (string, error) {
	return FilePath(userHomePath, persistContext, fileName)
}

// EnsureContext verifies ~/.irodsext/<context> exists and creates it
// idempotently when missing.
func (service *FileService) EnsureContext(userHomePath string, persistContext string) (string, error) {
	filesystem, err := service.requireFilesystem()
	if err != nil {
		return "", err
	}
	return EnsureCategoryCollection(filesystem, userHomePath, persistContext)
}

// EnsureFileStructure verifies ~/.irodsext/<context> exists and returns the
// expected path for the named file.
func (service *FileService) EnsureFileStructure(userHomePath string, persistContext string, fileName string) (string, error) {
	filePath, err := service.Path(userHomePath, persistContext, fileName)
	if err != nil {
		return "", err
	}
	if _, err := service.EnsureContext(userHomePath, persistContext); err != nil {
		return "", err
	}
	return filePath, nil
}

// AddOrUpdateFile stores or replaces a file under ~/.irodsext/<context>. The
// context collection is created idempotently when missing.
func (service *FileService) AddOrUpdateFile(userHomePath string, persistContext string, fileName string, contents []byte) (File, error) {
	filesystem, err := service.requireFilesystem()
	if err != nil {
		return File{}, err
	}

	filePath, err := service.EnsureFileStructure(userHomePath, persistContext, fileName)
	if err != nil {
		return File{}, err
	}

	if err := filesystem.WriteDataObject(filePath, contents); err != nil {
		return File{}, fmt.Errorf("write userpersist file %q: %w", filePath, err)
	}

	return File{
		UserHomePath: userHomePath,
		Context:      persistContext,
		Name:         fileName,
		IRODSPath:    filePath,
		Contents:     append([]byte(nil), contents...),
	}, nil
}

// AddOrUpdateString stores or replaces a string file under
// ~/.irodsext/<context>.
func (service *FileService) AddOrUpdateString(userHomePath string, persistContext string, fileName string, contents string) (File, error) {
	return service.AddOrUpdateFile(userHomePath, persistContext, fileName, []byte(contents))
}

// GetFile retrieves a file under ~/.irodsext/<context>.
func (service *FileService) GetFile(userHomePath string, persistContext string, fileName string) (File, error) {
	filesystem, err := service.requireFilesystem()
	if err != nil {
		return File{}, err
	}

	filePath, err := service.Path(userHomePath, persistContext, fileName)
	if err != nil {
		return File{}, err
	}

	contents, err := filesystem.ReadDataObject(filePath)
	if err != nil {
		return File{}, fmt.Errorf("read userpersist file %q: %w", filePath, err)
	}

	return File{
		UserHomePath: userHomePath,
		Context:      persistContext,
		Name:         fileName,
		IRODSPath:    filePath,
		Contents:     append([]byte(nil), contents...),
	}, nil
}

// GetString retrieves a string file under ~/.irodsext/<context>.
func (service *FileService) GetString(userHomePath string, persistContext string, fileName string) (string, File, error) {
	file, err := service.GetFile(userHomePath, persistContext, fileName)
	if err != nil {
		return "", File{}, err
	}
	return string(file.Contents), file, nil
}

// DeleteFile removes a file under ~/.irodsext/<context>.
func (service *FileService) DeleteFile(userHomePath string, persistContext string, fileName string, force bool) error {
	filesystem, err := service.requireFilesystem()
	if err != nil {
		return err
	}

	filePath, err := service.Path(userHomePath, persistContext, fileName)
	if err != nil {
		return err
	}

	if err := filesystem.DeleteDataObject(filePath, force); err != nil {
		return fmt.Errorf("delete userpersist file %q: %w", filePath, err)
	}
	return nil
}

func (service *FileService) requireFilesystem() (FileFilesystem, error) {
	if service == nil || service.filesystem == nil {
		return nil, ErrMissingFilesystem
	}
	return service.filesystem, nil
}
