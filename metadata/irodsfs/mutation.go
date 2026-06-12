package irodsfs

import (
	"fmt"

	"github.com/cyverse/go-irodsclient/irods/connection"
	"github.com/cyverse/go-irodsclient/irods/message"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
)

var _ metadata.AVUMutator = (*Adapter)(nil)

// ReplacePathAVU replaces one AVU on an iRODS collection or data object.
func (adapter *Adapter) ReplacePathAVU(irodsPath string, replacement metadata.AVUReplacement) (metadata.AVUStat, error) {
	if adapter == nil || adapter.filesystem == nil {
		return metadata.AVUStat{}, fmt.Errorf("missing go-irodsclient filesystem")
	}

	normalized, err := metadata.NormalizeAVUReplacement(replacement)
	if err != nil {
		return metadata.AVUStat{}, err
	}

	entry, err := adapter.filesystem.Stat(irodsPath)
	if err != nil {
		return metadata.AVUStat{}, err
	}
	if entry == nil {
		return metadata.AVUStat{}, fmt.Errorf("path stat missing for %q", irodsPath)
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return metadata.AVUStat{}, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	itemType := irodstypes.IRODSDataObjectMetaItemType
	if entry.IsDir() {
		itemType = irodstypes.IRODSCollectionMetaItemType
	}

	oldMetadata := irodsMetaFromAVUStat(normalized.From)
	newMetadata := irodsMetaFromAVUStat(normalized.To)
	if err := replaceMetadata(conn, itemType, irodsPath, oldMetadata, newMetadata); err != nil {
		return metadata.AVUStat{}, err
	}
	adapter.filesystem.InvalidateCacheForPath(irodsPath)

	metadataList, err := adapter.ListMetadata(irodsPath)
	if err != nil {
		return metadata.AVUStat{}, err
	}
	if updated, ok := findAVUStat(metadataList, normalized.To); ok {
		return updated, nil
	}

	return metadata.AVUStat{}, fmt.Errorf("metadata replacement completed but replacement AVU was not found for path %q target %+v metadata %+v", irodsPath, normalized.To, metadataList)
}

func replaceMetadata(conn *connection.IRODSConnection, itemType irodstypes.IRODSMetaItemType, itemName string, oldMetadata *irodstypes.IRODSMeta, newMetadata *irodstypes.IRODSMeta) error {
	if conn == nil || !conn.IsConnected() {
		return fmt.Errorf("connection is nil or disconnected")
	}

	conn.Lock()
	defer conn.Unlock()

	request := newReplaceMetadataRequest(itemType, itemName, oldMetadata, newMetadata)
	response := message.IRODSMessageModifyMetadataResponse{}
	if err := conn.RequestAndCheck(request, &response, nil, conn.GetOperationTimeout()); err != nil {
		return fmt.Errorf("replace metadata: %w", err)
	}
	return nil
}

func irodsMetaFromAVUStat(avu metadata.AVUStat) *irodstypes.IRODSMeta {
	return &irodstypes.IRODSMeta{
		Name:  avu.Name,
		Value: avu.Value,
		Units: avu.Units,
	}
}

func findAVUStat(metadataList []metadata.AVUStat, target metadata.AVUStat) (metadata.AVUStat, bool) {
	for _, avu := range metadataList {
		if avu.Name == target.Name && avu.Value == target.Value && avu.Units == target.Units {
			return avu, true
		}
	}
	return metadata.AVUStat{}, false
}

func newReplaceMetadataRequest(itemType irodstypes.IRODSMetaItemType, itemName string, oldMetadata *irodstypes.IRODSMeta, newMetadata *irodstypes.IRODSMeta) *message.IRODSMessageModifyMetadataRequest {
	request := message.NewIRODSMessageReplaceMetadataRequest(itemType, itemName, oldMetadata, newMetadata)
	request.NewAttrName = "n:" + newMetadata.Name
	request.NewAttrValue = "v:" + newMetadata.Value
	request.NewAttrUnits = "u:" + newMetadata.Units
	return request
}
