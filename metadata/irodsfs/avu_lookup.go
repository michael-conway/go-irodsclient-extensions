package irodsfs

import (
	"fmt"
	"path"
	"strconv"

	"github.com/cyverse/go-irodsclient/irods/common"
	"github.com/cyverse/go-irodsclient/irods/message"
	cytypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
)

// GetPathAVUByID returns one AVU attached to an iRODS path using a path-scoped AVU ID query.
func (adapter *Adapter) GetPathAVUByID(irodsPath string, avuID int64) (metadata.AVUStat, error) {
	if adapter == nil || adapter.filesystem == nil {
		return metadata.AVUStat{}, fmt.Errorf("missing go-irodsclient filesystem")
	}
	if avuID <= 0 {
		return metadata.AVUStat{}, fmt.Errorf("%w: id must be positive", metadata.ErrMissingAVUUpdate)
	}

	entry, err := adapter.filesystem.Stat(irodsPath)
	if err != nil {
		return metadata.AVUStat{}, err
	}
	if entry == nil {
		return metadata.AVUStat{}, fmt.Errorf("path stat missing for %q", irodsPath)
	}

	itemType := cytypes.IRODSDataObjectMetaItemType
	if entry.IsDir() {
		itemType = cytypes.IRODSCollectionMetaItemType
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return metadata.AVUStat{}, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	if conn == nil || !conn.IsConnected() {
		return metadata.AVUStat{}, fmt.Errorf("metadata lookup connection is nil or disconnected")
	}

	conn.Lock()
	defer conn.Unlock()

	request := newPathAVUByIDQueryRequest(conn.GetAccount().ClientZone, itemType, irodsPath, avuID)
	response := message.IRODSMessageQueryResponse{}
	if err := requestGenQuery(conn, request, &response, "path AVU ID"); err != nil {
		if isNoRowsError(err) {
			return metadata.AVUStat{}, fmt.Errorf("%w: id %d on path %q", metadata.ErrAVUNotFound, avuID, irodsPath)
		}
		return metadata.AVUStat{}, err
	}

	avu, err := parsePathAVUByIDQueryResult(response, itemType)
	if err != nil {
		return metadata.AVUStat{}, err
	}
	if avu.ID <= 0 {
		return metadata.AVUStat{}, fmt.Errorf("%w: id %d on path %q", metadata.ErrAVUNotFound, avuID, irodsPath)
	}
	return avu, nil
}

func newPathAVUByIDQueryRequest(zone string, itemType cytypes.IRODSMetaItemType, irodsPath string, avuID int64) *message.IRODSMessageQueryRequest {
	request := message.NewIRODSMessageQueryRequest(1, 0, 0, 0)
	request.AddKeyVal(common.ZONE_KW, zone)

	if itemType == cytypes.IRODSCollectionMetaItemType {
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_ID, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_NAME, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_VALUE, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_UNITS, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_CREATE_TIME, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_MODIFY_TIME, 1)
		request.AddEqualIDCondition(common.ICAT_COLUMN_META_COLL_ATTR_ID, avuID)
		request.AddEqualStringCondition(common.ICAT_COLUMN_COLL_NAME, irodsPath)
		return request
	}

	request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_ID, 1)
	request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_VALUE, 1)
	request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_UNITS, 1)
	request.AddSelect(common.ICAT_COLUMN_META_DATA_CREATE_TIME, 1)
	request.AddSelect(common.ICAT_COLUMN_META_DATA_MODIFY_TIME, 1)
	request.AddEqualIDCondition(common.ICAT_COLUMN_META_DATA_ATTR_ID, avuID)
	request.AddEqualStringCondition(common.ICAT_COLUMN_COLL_NAME, path.Dir(irodsPath))
	request.AddEqualStringCondition(common.ICAT_COLUMN_DATA_NAME, path.Base(irodsPath))
	return request
}

func parsePathAVUByIDQueryResult(queryResult message.IRODSMessageQueryResponse, itemType cytypes.IRODSMetaItemType) (metadata.AVUStat, error) {
	if queryResult.RowCount == 0 {
		return metadata.AVUStat{}, metadata.ErrAVUNotFound
	}
	if queryResult.RowCount > 1 {
		return metadata.AVUStat{}, fmt.Errorf("expected one AVU row, got %d", queryResult.RowCount)
	}
	if queryResult.AttributeCount > len(queryResult.SQLResult) {
		return metadata.AVUStat{}, fmt.Errorf("failed to receive AVU attributes - requires %d, but received %d attributes", queryResult.AttributeCount, len(queryResult.SQLResult))
	}

	avu := metadata.AVUStat{}
	for attr := 0; attr < queryResult.AttributeCount; attr++ {
		sqlResult := queryResult.SQLResult[attr]
		if len(sqlResult.Values) != queryResult.RowCount {
			return metadata.AVUStat{}, fmt.Errorf("failed to receive AVU rows - requires %d, but received %d attributes", queryResult.RowCount, len(sqlResult.Values))
		}

		value := sqlResult.Values[0]
		switch sqlResult.AttributeIndex {
		case int(avuIDColumn(itemType)):
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return metadata.AVUStat{}, fmt.Errorf("failed to parse AVU id %q: %w", value, err)
			}
			avu.ID = id
		case int(avuNameColumn(itemType)):
			avu.Name = value
		case int(avuValueColumn(itemType)):
			avu.Value = value
		case int(avuUnitsColumn(itemType)):
			avu.Units = value
		case int(avuCreateTimeColumn(itemType)):
			createTime, err := parseIRODSTime(value)
			if err != nil {
				return metadata.AVUStat{}, fmt.Errorf("failed to parse AVU create time %q: %w", value, err)
			}
			avu.CreateTime = createTime
		case int(avuModifyTimeColumn(itemType)):
			modifyTime, err := parseIRODSTime(value)
			if err != nil {
				return metadata.AVUStat{}, fmt.Errorf("failed to parse AVU modify time %q: %w", value, err)
			}
			avu.ModifyTime = modifyTime
		}
	}

	return avu, nil
}

func avuIDColumn(itemType cytypes.IRODSMetaItemType) common.ICATColumnNumber {
	if itemType == cytypes.IRODSCollectionMetaItemType {
		return common.ICAT_COLUMN_META_COLL_ATTR_ID
	}
	return common.ICAT_COLUMN_META_DATA_ATTR_ID
}

func avuNameColumn(itemType cytypes.IRODSMetaItemType) common.ICATColumnNumber {
	if itemType == cytypes.IRODSCollectionMetaItemType {
		return common.ICAT_COLUMN_META_COLL_ATTR_NAME
	}
	return common.ICAT_COLUMN_META_DATA_ATTR_NAME
}

func avuValueColumn(itemType cytypes.IRODSMetaItemType) common.ICATColumnNumber {
	if itemType == cytypes.IRODSCollectionMetaItemType {
		return common.ICAT_COLUMN_META_COLL_ATTR_VALUE
	}
	return common.ICAT_COLUMN_META_DATA_ATTR_VALUE
}

func avuUnitsColumn(itemType cytypes.IRODSMetaItemType) common.ICATColumnNumber {
	if itemType == cytypes.IRODSCollectionMetaItemType {
		return common.ICAT_COLUMN_META_COLL_ATTR_UNITS
	}
	return common.ICAT_COLUMN_META_DATA_ATTR_UNITS
}

func avuCreateTimeColumn(itemType cytypes.IRODSMetaItemType) common.ICATColumnNumber {
	if itemType == cytypes.IRODSCollectionMetaItemType {
		return common.ICAT_COLUMN_META_COLL_CREATE_TIME
	}
	return common.ICAT_COLUMN_META_DATA_CREATE_TIME
}

func avuModifyTimeColumn(itemType cytypes.IRODSMetaItemType) common.ICATColumnNumber {
	if itemType == cytypes.IRODSCollectionMetaItemType {
		return common.ICAT_COLUMN_META_COLL_MODIFY_TIME
	}
	return common.ICAT_COLUMN_META_DATA_MODIFY_TIME
}
