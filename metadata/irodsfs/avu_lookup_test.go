package irodsfs

import (
	"testing"
	"time"

	"github.com/cyverse/go-irodsclient/irods/common"
	"github.com/cyverse/go-irodsclient/irods/message"
	cytypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestPathAVUByIDQueryRequestUsesDataObjectPathAndID(t *testing.T) {
	request := newPathAVUByIDQueryRequest("tempZone", cytypes.IRODSDataObjectMetaItemType, "/tempZone/home/alice/object.txt", 42)

	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_ID)
	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_NAME)
	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_VALUE)
	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_UNITS)
	assertCondition(t, request, common.ICAT_COLUMN_META_DATA_ATTR_ID, "= '42'")
	assertCondition(t, request, common.ICAT_COLUMN_COLL_NAME, "= '/tempZone/home/alice'")
	assertCondition(t, request, common.ICAT_COLUMN_DATA_NAME, "= 'object.txt'")
}

func TestPathAVUByIDQueryRequestUsesCollectionPathAndID(t *testing.T) {
	request := newPathAVUByIDQueryRequest("tempZone", cytypes.IRODSCollectionMetaItemType, "/tempZone/home/alice", 43)

	assertSelect(t, request, common.ICAT_COLUMN_META_COLL_ATTR_ID)
	assertSelect(t, request, common.ICAT_COLUMN_META_COLL_ATTR_NAME)
	assertSelect(t, request, common.ICAT_COLUMN_META_COLL_ATTR_VALUE)
	assertSelect(t, request, common.ICAT_COLUMN_META_COLL_ATTR_UNITS)
	assertCondition(t, request, common.ICAT_COLUMN_META_COLL_ATTR_ID, "= '43'")
	assertCondition(t, request, common.ICAT_COLUMN_COLL_NAME, "= '/tempZone/home/alice'")
}

func TestParsePathAVUByIDQueryResultMapsDataObjectAVU(t *testing.T) {
	created := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	modified := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	queryResult := message.IRODSMessageQueryResponse{
		RowCount:       1,
		AttributeCount: 6,
		SQLResult: []message.IRODSMessageSQLResult{
			sqlResult(common.ICAT_COLUMN_META_DATA_ATTR_ID, "42"),
			sqlResult(common.ICAT_COLUMN_META_DATA_ATTR_NAME, "source"),
			sqlResult(common.ICAT_COLUMN_META_DATA_ATTR_VALUE, "before"),
			sqlResult(common.ICAT_COLUMN_META_DATA_ATTR_UNITS, "fixture"),
			sqlResult(common.ICAT_COLUMN_META_DATA_CREATE_TIME, irodsTime(created)),
			sqlResult(common.ICAT_COLUMN_META_DATA_MODIFY_TIME, irodsTime(modified)),
		},
	}

	avu, err := parsePathAVUByIDQueryResult(queryResult, cytypes.IRODSDataObjectMetaItemType)
	if err != nil {
		t.Fatalf("parse AVU query result: %v", err)
	}
	if avu.ID != 42 || avu.Name != "source" || avu.Value != "before" || avu.Units != "fixture" {
		t.Fatalf("unexpected AVU: %+v", avu)
	}
	if !avu.CreateTime.Equal(created) || !avu.ModifyTime.Equal(modified) {
		t.Fatalf("unexpected AVU times: %+v", avu)
	}
}
