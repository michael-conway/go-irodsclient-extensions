package irodsfs

import (
	"strconv"
	"testing"
	"time"

	"github.com/cyverse/go-irodsclient/irods/common"
	"github.com/cyverse/go-irodsclient/irods/connection"
	"github.com/cyverse/go-irodsclient/irods/message"
	"github.com/cyverse/go-irodsclient/irods/types"
	"github.com/cyverse/go-irodsclient/irods/util"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
)

func TestBuildDataObjectEntryQueryRequestUsesScopedAVUConditionsAndMasterReplica(t *testing.T) {
	conn := testConnection(t)
	query, err := metadata.NormalizeEntryQuery(metadata.NewEntryQuery().
		BothKinds().
		Scope("/tempZone/home/alice", metadata.EntryQueryScopeDescendants).
		AVU("foo:bar", "frog*", metadata.AnyUnit).
		Build())
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	request, err := buildDataObjectEntryQueryRequest(conn, query, 0, false)
	if err != nil {
		t.Fatalf("build data object request: %v", err)
	}

	assertCondition(t, request, common.ICAT_COLUMN_COLL_NAME, "like '/tempZone/home/alice/%'")
	assertCondition(t, request, common.ICAT_COLUMN_META_DATA_ATTR_NAME, "= 'foo:bar'")
	assertCondition(t, request, common.ICAT_COLUMN_META_DATA_ATTR_VALUE, "like 'frog%'")
	assertCondition(t, request, common.ICAT_COLUMN_D_REPL_STATUS, "= '1'")
	assertSelect(t, request, common.ICAT_COLUMN_D_DATA_ID)
	assertSelect(t, request, common.ICAT_COLUMN_D_RESC_HIER)
}

func TestBuildCollectionEntryQueryRequestUsesChildrenScope(t *testing.T) {
	conn := testConnection(t)
	query, err := metadata.NormalizeEntryQuery(metadata.NewEntryQuery().
		Collections().
		Scope("/tempZone/home/alice", metadata.EntryQueryScopeChildren).
		Equal(metadata.FieldAVUAttrib, "project").
		Build())
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	request, err := buildCollectionEntryQueryRequest(conn, query, 0, true)
	if err != nil {
		t.Fatalf("build collection request: %v", err)
	}

	assertCondition(t, request, common.ICAT_COLUMN_COLL_PARENT_NAME, "= '/tempZone/home/alice'")
	assertCondition(t, request, common.ICAT_COLUMN_META_COLL_ATTR_NAME, "= 'project'")
	assertSelect(t, request, common.ICAT_COLUMN_COLL_ID)
	assertSelect(t, request, common.ICAT_COLUMN_META_COLL_ATTR_ID)
	assertSelect(t, request, common.ICAT_COLUMN_META_COLL_ATTR_VALUE)
}

func TestBuildCollectionEntryQueryRequestUsesSelfScope(t *testing.T) {
	conn := testConnection(t)
	query, err := metadata.NormalizeEntryQuery(metadata.NewEntryQuery().
		Collections().
		Scope("/tempZone/home/alice", metadata.EntryQueryScopeSelf).
		Equal(metadata.FieldAVUAttrib, "project").
		Build())
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	request, err := buildCollectionEntryQueryRequest(conn, query, 0, true)
	if err != nil {
		t.Fatalf("build collection request: %v", err)
	}

	assertCondition(t, request, common.ICAT_COLUMN_COLL_NAME, "= '/tempZone/home/alice'")
	assertCondition(t, request, common.ICAT_COLUMN_META_COLL_ATTR_NAME, "= 'project'")
}

func TestQueryBranchEnabledSkipsCollectionBranchForDataObjectOnlyFields(t *testing.T) {
	query, err := metadata.NormalizeEntryQuery(metadata.NewEntryQuery().
		BothKinds().
		Equal(metadata.FieldDataType, "generic").
		Build())
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	enabled, err := queryBranchEnabled(query, entryQueryBranchCollections)
	if err != nil {
		t.Fatalf("query branch enabled: %v", err)
	}
	if enabled {
		t.Fatalf("expected collection branch to be skipped for data_type condition")
	}

	enabled, err = queryBranchEnabled(query, entryQueryBranchDataObjects)
	if err != nil {
		t.Fatalf("query branch enabled data objects: %v", err)
	}
	if !enabled {
		t.Fatalf("expected data object branch to remain enabled")
	}
}

func TestQueryBranchEnabledSkipsDataObjectBranchForSelfScope(t *testing.T) {
	query, err := metadata.NormalizeEntryQuery(metadata.NewEntryQuery().
		BothKinds().
		Scope("/tempZone/home/alice", metadata.EntryQueryScopeSelf).
		Build())
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	enabled, err := queryBranchEnabled(query, entryQueryBranchDataObjects)
	if err != nil {
		t.Fatalf("query branch enabled data objects: %v", err)
	}
	if enabled {
		t.Fatalf("expected self scope to skip data object branch")
	}
}

func TestParseDataObjectRowsSelectsPreferredReplica(t *testing.T) {
	createNewer := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	createOlder := time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC)
	accessTime := time.Date(2026, 5, 15, 11, 0, 0, 0, time.UTC)
	queryResult := message.IRODSMessageQueryResponse{
		RowCount:       2,
		AttributeCount: 11,
		SQLResult: []message.IRODSMessageSQLResult{
			sqlResult(common.ICAT_COLUMN_COLL_ID, "20", "20"),
			sqlResult(common.ICAT_COLUMN_COLL_NAME, "/tempZone/home/alice", "/tempZone/home/alice"),
			sqlResult(common.ICAT_COLUMN_D_DATA_ID, "100", "100"),
			sqlResult(common.ICAT_COLUMN_DATA_NAME, "object.txt", "object.txt"),
			sqlResult(common.ICAT_COLUMN_DATA_SIZE, "5", "5"),
			sqlResult(common.ICAT_COLUMN_DATA_TYPE_NAME, "generic", "generic"),
			sqlResult(common.ICAT_COLUMN_DATA_REPL_NUM, "1", "0"),
			sqlResult(common.ICAT_COLUMN_D_OWNER_NAME, "alice", "alice"),
			sqlResult(common.ICAT_COLUMN_D_REPL_STATUS, "1", "1"),
			sqlResult(common.ICAT_COLUMN_D_CREATE_TIME, irodsTime(createNewer), irodsTime(createOlder)),
			sqlResult(common.ICAT_COLUMN_D_ACCESS_TIME, irodsTime(accessTime), irodsTime(accessTime)),
		},
	}

	rows, err := parseDataObjectQueryRows(queryResult, false)
	if err != nil {
		t.Fatalf("parse data object rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 raw rows, got %d", len(rows))
	}

	if !preferDataObjectReplica(rows[0].dataObject, rows[1].dataObject) {
		t.Fatalf("expected older good replica to be preferred")
	}
	if !rows[0].dataObject.Replicas[0].AccessTime.Equal(accessTime) {
		t.Fatalf("expected access time %s, got %s", accessTime, rows[0].dataObject.Replicas[0].AccessTime)
	}
}

func testConnection(t *testing.T) *connection.IRODSConnection {
	t.Helper()

	account, err := types.CreateIRODSAccount("localhost", 1247, "alice", "tempZone", types.AuthSchemeNative, "test", "demoResc")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	conn, err := connection.NewIRODSConnection(account, &connection.IRODSConnectionConfig{
		ApplicationName: "metadata-query-test",
		ConnectTimeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}
	return conn
}

func assertCondition(t *testing.T, request *message.IRODSMessageQueryRequest, column common.ICATColumnNumber, value string) {
	t.Helper()

	escapedValue := util.EscapeXMLSpecialChars(value)
	for idx, key := range request.Conditions.Keys {
		if key == int(column) && request.Conditions.Values[idx].Value == escapedValue {
			return
		}
	}
	t.Fatalf("missing condition column=%d value=%q in %+v", column, value, request.Conditions)
}

func assertSelect(t *testing.T, request *message.IRODSMessageQueryRequest, column common.ICATColumnNumber) {
	t.Helper()

	for _, key := range request.Selects.Keys {
		if key == int(column) {
			return
		}
	}
	t.Fatalf("missing select column=%d in %+v", column, request.Selects)
}

func sqlResult(column common.ICATColumnNumber, values ...string) message.IRODSMessageSQLResult {
	return message.IRODSMessageSQLResult{
		AttributeIndex: int(column),
		ResultLen:      len(values),
		Values:         values,
	}
}

func irodsTime(value time.Time) string {
	return strconv.FormatInt(value.Unix(), 10)
}

func TestBuildDataObjectEntryQueryRequestIncludesMatchedAVUID(t *testing.T) {
	conn := testConnection(t)
	query, err := metadata.NormalizeEntryQuery(metadata.NewEntryQuery().
		DataObjects().
		Scope("/tempZone/home/alice", metadata.EntryQueryScopeDescendants).
		AVU("source", "test", metadata.AnyUnit).
		IncludeMatchedAVUs(true).
		Build())
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	request, err := buildDataObjectEntryQueryRequest(conn, query, 0, true)
	if err != nil {
		t.Fatalf("build data object request: %v", err)
	}

	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_ID)
	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_NAME)
	assertSelect(t, request, common.ICAT_COLUMN_META_DATA_ATTR_VALUE)
}
