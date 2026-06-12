//go:build integration
// +build integration

package integration

import (
	"fmt"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func createFileWithContents(filesystem *irodsfs.FileSystem, irodsPath string, contents string) error {
	handle, err := filesystem.CreateFile(irodsPath, "", "w")
	if err != nil {
		return err
	}

	if len(contents) > 0 {
		if _, err := handle.Write([]byte(contents)); err != nil {
			_ = handle.Close()
			return err
		}
	}

	if err := handle.Close(); err != nil {
		return fmt.Errorf("close file %q: %w", irodsPath, err)
	}
	return nil
}

func hasIRODSAVU(metadataList []*irodstypes.IRODSMeta, name string, value string, units string) bool {
	for _, avu := range metadataList {
		if avu != nil && avu.Name == name && avu.Value == value && avu.Units == units {
			return true
		}
	}
	return false
}
