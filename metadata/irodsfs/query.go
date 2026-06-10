package irodsfs

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	cyfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/cyverse/go-irodsclient/irods/common"
	"github.com/cyverse/go-irodsclient/irods/connection"
	"github.com/cyverse/go-irodsclient/irods/message"
	"github.com/cyverse/go-irodsclient/irods/types"
	"github.com/cyverse/go-irodsclient/irods/util"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
)

type entryQueryBranch string

const (
	entryQueryBranchCollections entryQueryBranch = "collections"
	entryQueryBranchDataObjects entryQueryBranch = "data_objects"
)

type branchScanResult struct {
	entries     []*metadata.Entry
	matchedAVUs map[string][]metadata.AVUStat
	returned    int
	scanned     int
	exhausted   bool
}

type collectionQueryRow struct {
	collection *types.IRODSCollection
	avu        *metadata.AVUStat
}

type dataObjectQueryRow struct {
	dataObject *types.IRODSDataObject
	avu        *metadata.AVUStat
}

// QueryEntries executes a metadata entry query against iRODS.
func (adapter *Adapter) QueryEntries(query metadata.EntryQuery) (metadata.EntryQueryResult, error) {
	if adapter == nil || adapter.filesystem == nil {
		return metadata.EntryQueryResult{}, metadata.ErrMissingFilesystem
	}

	normalized, err := metadata.NormalizeEntryQuery(query)
	if err != nil {
		return metadata.EntryQueryResult{}, err
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return metadata.EntryQueryResult{}, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	if conn == nil || !conn.IsConnected() {
		return metadata.EntryQueryResult{}, fmt.Errorf("metadata query connection is nil or disconnected")
	}

	if metrics := conn.GetMetrics(); metrics != nil {
		metrics.IncreaseCounterForSearch(1)
	}

	conn.Lock()
	defer conn.Unlock()

	return queryEntriesWithConnection(conn, normalized)
}

func queryEntriesWithConnection(conn *connection.IRODSConnection, query metadata.EntryQuery) (metadata.EntryQueryResult, error) {
	collectionsEnabled, err := queryBranchEnabled(query, entryQueryBranchCollections)
	if err != nil {
		return metadata.EntryQueryResult{}, err
	}
	dataObjectsEnabled, err := queryBranchEnabled(query, entryQueryBranchDataObjects)
	if err != nil {
		return metadata.EntryQueryResult{}, err
	}

	cursor := metadata.EntryQueryCursor{}
	if query.Cursor != nil {
		cursor = *query.Cursor
	}

	phase := cursor.Phase
	if phase == "" {
		if collectionsEnabled {
			phase = metadata.EntryQueryPhaseCollections
		} else {
			phase = metadata.EntryQueryPhaseDataObjects
		}
	}

	result := metadata.EntryQueryResult{
		Entries: []*metadata.Entry{},
		Page: metadata.EntryQueryPage{
			Limit: query.Limit,
		},
	}
	if query.IncludeMatchedAVUs {
		result.MatchedAVUs = map[string][]metadata.AVUStat{}
	}

	next := cursor
	remaining := query.Limit
	hasMore := false

	if phase == metadata.EntryQueryPhaseDone {
		result.Page.HasMore = false
		return result, nil
	}

	if phase == metadata.EntryQueryPhaseCollections && collectionsEnabled && !cursor.Collections.Exhausted && remaining > 0 {
		scan, err := scanCollectionBranch(conn, query, cursor.Collections.Offset, remaining)
		if err != nil {
			return metadata.EntryQueryResult{}, err
		}

		result.Entries = append(result.Entries, scan.entries...)
		mergeMatchedAVUs(result.MatchedAVUs, scan.matchedAVUs)
		result.Page.Returned.Collections += scan.returned
		result.Page.Scanned.Collections += scan.scanned
		next.Collections.Offset += scan.returned
		remaining -= scan.returned

		if !scan.exhausted {
			hasMore = true
			next.Phase = metadata.EntryQueryPhaseCollections
		} else {
			next.Collections.Exhausted = true
			phase = metadata.EntryQueryPhaseDataObjects
			next.Phase = metadata.EntryQueryPhaseDataObjects
		}
	}

	if !hasMore && phase == metadata.EntryQueryPhaseDataObjects && dataObjectsEnabled && !cursor.DataObjects.Exhausted && remaining > 0 {
		scan, err := scanDataObjectBranch(conn, query, cursor.DataObjects.Offset, remaining)
		if err != nil {
			return metadata.EntryQueryResult{}, err
		}

		result.Entries = append(result.Entries, scan.entries...)
		mergeMatchedAVUs(result.MatchedAVUs, scan.matchedAVUs)
		result.Page.Returned.DataObjects += scan.returned
		result.Page.Scanned.DataObjects += scan.scanned
		next.DataObjects.Offset += scan.returned
		remaining -= scan.returned

		if !scan.exhausted {
			hasMore = true
			next.Phase = metadata.EntryQueryPhaseDataObjects
		} else {
			next.DataObjects.Exhausted = true
			next.Phase = metadata.EntryQueryPhaseDone
		}
	}

	if !hasMore && remaining == 0 && phase == metadata.EntryQueryPhaseDataObjects && dataObjectsEnabled && !next.DataObjects.Exhausted {
		peek, err := scanDataObjectBranch(conn, query, next.DataObjects.Offset, 1)
		if err != nil {
			return metadata.EntryQueryResult{}, err
		}
		result.Page.Scanned.DataObjects += peek.scanned
		if peek.returned > 0 || !peek.exhausted {
			hasMore = true
			next.Phase = metadata.EntryQueryPhaseDataObjects
		} else {
			next.DataObjects.Exhausted = true
			next.Phase = metadata.EntryQueryPhaseDone
		}
	}

	if !hasMore && phase == metadata.EntryQueryPhaseCollections && collectionsEnabled && next.Collections.Exhausted && dataObjectsEnabled && !next.DataObjects.Exhausted {
		peek, err := scanDataObjectBranch(conn, query, next.DataObjects.Offset, 1)
		if err != nil {
			return metadata.EntryQueryResult{}, err
		}
		result.Page.Scanned.DataObjects += peek.scanned
		if peek.returned > 0 || !peek.exhausted {
			hasMore = true
			next.Phase = metadata.EntryQueryPhaseDataObjects
		} else {
			next.DataObjects.Exhausted = true
			next.Phase = metadata.EntryQueryPhaseDone
		}
	}

	if query.IncludeTotals {
		totals := metadata.EntryQueryCounts{}
		if collectionsEnabled {
			count, err := countCollectionBranch(conn, query)
			if err != nil {
				return metadata.EntryQueryResult{}, err
			}
			totals.Collections = count
		}
		if dataObjectsEnabled {
			count, err := countDataObjectBranch(conn, query)
			if err != nil {
				return metadata.EntryQueryResult{}, err
			}
			totals.DataObjects = count
		}
		result.Page.Totals = &totals
	}

	result.Page.HasMore = hasMore
	if hasMore {
		result.Page.Next = &next
	}
	return result, nil
}

func queryBranchEnabled(query metadata.EntryQuery, branch entryQueryBranch) (bool, error) {
	switch branch {
	case entryQueryBranchCollections:
		if !metadata.EntryQueryHasKind(query, metadata.EntryKindCollection) {
			return false, nil
		}
		for _, condition := range query.Conditions {
			if collectionConditionSupported(condition.Field) {
				continue
			}
			if metadata.EntryQueryHasKind(query, metadata.EntryKindDataObject) {
				return false, nil
			}
			return false, fmt.Errorf("%w: field %q cannot be expressed for collection queries without client-side filtering", metadata.ErrInvalidEntryQuery, condition.Field)
		}
		return true, nil
	case entryQueryBranchDataObjects:
		if query.Scope != nil && query.Scope.Mode == metadata.EntryQueryScopeSelf {
			return false, nil
		}
		return metadata.EntryQueryHasKind(query, metadata.EntryKindDataObject), nil
	default:
		return false, fmt.Errorf("%w: unsupported query branch %q", metadata.ErrInvalidEntryQuery, branch)
	}
}

func collectionConditionSupported(field metadata.EntryField) bool {
	switch field {
	case metadata.FieldPath, metadata.FieldOwner, metadata.FieldAVUAttrib, metadata.FieldAVUValue, metadata.FieldAVUUnit:
		return true
	default:
		return false
	}
}

func scanCollectionBranch(conn *connection.IRODSConnection, query metadata.EntryQuery, offset int, limit int) (branchScanResult, error) {
	result := branchScanResult{
		entries:     []*metadata.Entry{},
		matchedAVUs: map[string][]metadata.AVUStat{},
		exhausted:   true,
	}

	seen := map[string]bool{}
	continueIndex := 0
	includeAVUs := query.IncludeMatchedAVUs && metadata.EntryQueryHasAVUConditions(query)

	for {
		request, err := buildCollectionEntryQueryRequest(conn, query, continueIndex, includeAVUs)
		if err != nil {
			return branchScanResult{}, err
		}

		queryResult := message.IRODSMessageQueryResponse{}
		if err := requestGenQuery(conn, request, &queryResult, "collection"); err != nil {
			if isNoRowsError(err) {
				break
			}
			return branchScanResult{}, err
		}

		rows, err := parseCollectionQueryRows(queryResult, includeAVUs)
		if err != nil {
			return branchScanResult{}, err
		}

		for _, row := range rows {
			if row.collection == nil {
				continue
			}
			entry := cyfs.NewEntryFromCollection(row.collection)
			key := entry.Path
			if key == "" {
				key = strconv.FormatInt(entry.ID, 10)
			}

			alreadySeen := seen[key]
			if !alreadySeen {
				seen[key] = true
				result.scanned++
			}

			if alreadySeen {
				if row.avu != nil {
					appendMatchedAVU(result.matchedAVUs, key, *row.avu)
				}
				continue
			}

			logicalIndex := result.scanned - 1
			if logicalIndex < offset {
				continue
			}
			if len(result.entries) >= limit {
				result.exhausted = false
				return result, nil
			}

			result.entries = append(result.entries, entry)
			result.returned++
			if row.avu != nil {
				appendMatchedAVU(result.matchedAVUs, key, *row.avu)
			}
		}

		continueIndex = queryResult.ContinueIndex
		if continueIndex == 0 {
			break
		}
	}

	return result, nil
}

func scanDataObjectBranch(conn *connection.IRODSConnection, query metadata.EntryQuery, offset int, limit int) (branchScanResult, error) {
	result := branchScanResult{
		entries:     []*metadata.Entry{},
		matchedAVUs: map[string][]metadata.AVUStat{},
		exhausted:   true,
	}

	objects := map[int64]*types.IRODSDataObject{}
	objectPaths := map[int64]string{}
	orderedIDs := []int64{}
	matchedByID := map[int64][]metadata.AVUStat{}
	continueIndex := 0
	includeAVUs := query.IncludeMatchedAVUs && metadata.EntryQueryHasAVUConditions(query)

	for {
		request, err := buildDataObjectEntryQueryRequest(conn, query, continueIndex, includeAVUs)
		if err != nil {
			return branchScanResult{}, err
		}

		queryResult := message.IRODSMessageQueryResponse{}
		if err := requestGenQuery(conn, request, &queryResult, "data object"); err != nil {
			if isNoRowsError(err) {
				break
			}
			return branchScanResult{}, err
		}

		rows, err := parseDataObjectQueryRows(queryResult, includeAVUs)
		if err != nil {
			return branchScanResult{}, err
		}

		for _, row := range rows {
			if row.dataObject == nil || len(row.dataObject.Replicas) == 0 {
				continue
			}

			id := row.dataObject.ID
			if id == -1 {
				id = int64(len(orderedIDs) + 1)
			}

			if existing, ok := objects[id]; ok {
				if preferDataObjectReplica(existing, row.dataObject) {
					objects[id] = row.dataObject
					objectPaths[id] = row.dataObject.Path
				}
			} else {
				objects[id] = row.dataObject
				objectPaths[id] = row.dataObject.Path
				orderedIDs = append(orderedIDs, id)
				result.scanned++
			}

			if row.avu != nil {
				matchedByID[id] = appendUniqueAVU(matchedByID[id], *row.avu)
			}
		}

		readyCount := len(orderedIDs)
		if readyCount > offset+limit {
			result.exhausted = false
			break
		}

		continueIndex = queryResult.ContinueIndex
		if continueIndex == 0 {
			break
		}
	}

	for idx, id := range orderedIDs {
		if idx < offset {
			continue
		}
		if len(result.entries) >= limit {
			result.exhausted = false
			break
		}

		object := objects[id]
		if object == nil || len(object.Replicas) == 0 {
			continue
		}
		entry := cyfs.NewEntryFromDataObject(object)
		result.entries = append(result.entries, entry)
		result.returned++
		for _, avu := range matchedByID[id] {
			key := objectPaths[id]
			if key == "" {
				key = entry.Path
			}
			appendMatchedAVU(result.matchedAVUs, key, avu)
		}
	}

	return result, nil
}

func countCollectionBranch(conn *connection.IRODSConnection, query metadata.EntryQuery) (int, error) {
	scan, err := scanCollectionBranch(conn, query, 0, int(^uint(0)>>1))
	if err != nil {
		return 0, err
	}
	return scan.scanned, nil
}

func countDataObjectBranch(conn *connection.IRODSConnection, query metadata.EntryQuery) (int, error) {
	scan, err := scanDataObjectBranch(conn, query, 0, int(^uint(0)>>1))
	if err != nil {
		return 0, err
	}
	return scan.scanned, nil
}

func buildCollectionEntryQueryRequest(conn *connection.IRODSConnection, query metadata.EntryQuery, continueIndex int, includeAVUs bool) (*message.IRODSMessageQueryRequest, error) {
	request := message.NewIRODSMessageQueryRequest(common.MaxQueryRows, continueIndex, 0, 0)
	request.AddKeyVal(common.ZONE_KW, conn.GetAccount().ClientZone)
	request.AddSelect(common.ICAT_COLUMN_COLL_ID, 1)
	request.AddSelect(common.ICAT_COLUMN_COLL_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_COLL_OWNER_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_COLL_CREATE_TIME, 1)
	request.AddSelect(common.ICAT_COLUMN_COLL_MODIFY_TIME, 1)
	if includeAVUs {
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_ID, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_NAME, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_VALUE, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_ATTR_UNITS, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_CREATE_TIME, 1)
		request.AddSelect(common.ICAT_COLUMN_META_COLL_MODIFY_TIME, 1)
	}

	if err := addScopeConditions(request, query.Scope, entryQueryBranchCollections); err != nil {
		return nil, err
	}
	for _, condition := range query.Conditions {
		if err := addCollectionCondition(request, condition); err != nil {
			return nil, err
		}
	}
	return request, nil
}

func buildDataObjectEntryQueryRequest(conn *connection.IRODSConnection, query metadata.EntryQuery, continueIndex int, includeAVUs bool) (*message.IRODSMessageQueryRequest, error) {
	request := message.NewIRODSMessageQueryRequest(common.MaxQueryRows, continueIndex, 0, 0)
	request.AddKeyVal(common.ZONE_KW, conn.GetAccount().ClientZone)
	request.AddSelect(common.ICAT_COLUMN_COLL_ID, 1)
	request.AddSelect(common.ICAT_COLUMN_COLL_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_D_DATA_ID, 1)
	request.AddSelect(common.ICAT_COLUMN_DATA_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_DATA_SIZE, 1)
	request.AddSelect(common.ICAT_COLUMN_DATA_TYPE_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_DATA_REPL_NUM, 1)
	request.AddSelect(common.ICAT_COLUMN_D_OWNER_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_D_DATA_CHECKSUM, 1)
	request.AddSelect(common.ICAT_COLUMN_D_REPL_STATUS, 1)
	request.AddSelect(common.ICAT_COLUMN_D_RESC_NAME, 1)
	request.AddSelect(common.ICAT_COLUMN_D_DATA_PATH, 1)
	request.AddSelect(common.ICAT_COLUMN_D_RESC_HIER, 1)
	request.AddSelect(common.ICAT_COLUMN_D_CREATE_TIME, 1)
	request.AddSelect(common.ICAT_COLUMN_D_MODIFY_TIME, 1)
	if conn.GetVersion() != nil && conn.GetVersion().HasHigherVersionThan(5, 0, 0) {
		request.AddSelect(common.ICAT_COLUMN_D_ACCESS_TIME, 1)
	}
	if includeAVUs {
		request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_ID, 1)
		request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_NAME, 1)
		request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_VALUE, 1)
		request.AddSelect(common.ICAT_COLUMN_META_DATA_ATTR_UNITS, 1)
		request.AddSelect(common.ICAT_COLUMN_META_DATA_CREATE_TIME, 1)
		request.AddSelect(common.ICAT_COLUMN_META_DATA_MODIFY_TIME, 1)
	}

	if err := addScopeConditions(request, query.Scope, entryQueryBranchDataObjects); err != nil {
		return nil, err
	}
	request.AddEqualStringCondition(common.ICAT_COLUMN_D_REPL_STATUS, "1")
	for _, condition := range query.Conditions {
		if err := addDataObjectCondition(request, condition); err != nil {
			return nil, err
		}
	}
	return request, nil
}

func addScopeConditions(request *message.IRODSMessageQueryRequest, scope *metadata.EntryQueryScope, branch entryQueryBranch) error {
	if scope == nil || scope.Mode == metadata.EntryQueryScopeAbsolute {
		return nil
	}
	root := strings.TrimRight(scope.Root, "/")
	switch branch {
	case entryQueryBranchCollections:
		switch scope.Mode {
		case metadata.EntryQueryScopeSelf:
			request.AddEqualStringCondition(common.ICAT_COLUMN_COLL_NAME, root)
		case metadata.EntryQueryScopeChildren:
			request.AddEqualStringCondition(common.ICAT_COLUMN_COLL_PARENT_NAME, root)
		case metadata.EntryQueryScopeDescendants:
			request.AddLikeStringCondition(common.ICAT_COLUMN_COLL_NAME, root+"/%")
		default:
			return fmt.Errorf("%w: unsupported collection scope mode %q", metadata.ErrInvalidEntryQuery, scope.Mode)
		}
	case entryQueryBranchDataObjects:
		switch scope.Mode {
		case metadata.EntryQueryScopeSelf:
			return fmt.Errorf("%w: self scope applies to collections, not data object branches", metadata.ErrInvalidEntryQuery)
		case metadata.EntryQueryScopeChildren:
			request.AddEqualStringCondition(common.ICAT_COLUMN_COLL_NAME, root)
		case metadata.EntryQueryScopeDescendants:
			request.AddLikeStringCondition(common.ICAT_COLUMN_COLL_NAME, root+"/%")
		default:
			return fmt.Errorf("%w: unsupported data object scope mode %q", metadata.ErrInvalidEntryQuery, scope.Mode)
		}
	}
	return nil
}

func addCollectionCondition(request *message.IRODSMessageQueryRequest, condition metadata.EntryCondition) error {
	switch condition.Field {
	case metadata.FieldPath:
		addStringCondition(request, common.ICAT_COLUMN_COLL_NAME, condition)
	case metadata.FieldOwner:
		addStringCondition(request, common.ICAT_COLUMN_COLL_OWNER_NAME, condition)
	case metadata.FieldAVUAttrib:
		addStringCondition(request, common.ICAT_COLUMN_META_COLL_ATTR_NAME, condition)
	case metadata.FieldAVUValue:
		addStringCondition(request, common.ICAT_COLUMN_META_COLL_ATTR_VALUE, condition)
	case metadata.FieldAVUUnit:
		addStringCondition(request, common.ICAT_COLUMN_META_COLL_ATTR_UNITS, condition)
	default:
		return fmt.Errorf("%w: field %q cannot be expressed for collection queries without client-side filtering", metadata.ErrInvalidEntryQuery, condition.Field)
	}
	return nil
}

func addDataObjectCondition(request *message.IRODSMessageQueryRequest, condition metadata.EntryCondition) error {
	switch condition.Field {
	case metadata.FieldPath:
		return addDataObjectPathCondition(request, condition)
	case metadata.FieldName:
		addStringCondition(request, common.ICAT_COLUMN_DATA_NAME, condition)
	case metadata.FieldOwner:
		addStringCondition(request, common.ICAT_COLUMN_D_OWNER_NAME, condition)
	case metadata.FieldDataType:
		addStringCondition(request, common.ICAT_COLUMN_DATA_TYPE_NAME, condition)
	case metadata.FieldResource:
		addStringCondition(request, common.ICAT_COLUMN_D_RESC_NAME, condition)
	case metadata.FieldChecksum:
		addStringCondition(request, common.ICAT_COLUMN_D_DATA_CHECKSUM, condition)
	case metadata.FieldAVUAttrib:
		addStringCondition(request, common.ICAT_COLUMN_META_DATA_ATTR_NAME, condition)
	case metadata.FieldAVUValue:
		addStringCondition(request, common.ICAT_COLUMN_META_DATA_ATTR_VALUE, condition)
	case metadata.FieldAVUUnit:
		addStringCondition(request, common.ICAT_COLUMN_META_DATA_ATTR_UNITS, condition)
	default:
		return fmt.Errorf("%w: field %q cannot be expressed for data object queries", metadata.ErrInvalidEntryQuery, condition.Field)
	}
	return nil
}

func addDataObjectPathCondition(request *message.IRODSMessageQueryRequest, condition metadata.EntryCondition) error {
	value := strings.TrimSpace(condition.Value)
	if value == "" || !strings.HasPrefix(value, "/") {
		return fmt.Errorf("%w: data object path conditions require an absolute logical path", metadata.ErrInvalidEntryQuery)
	}

	if condition.Op == metadata.OpEqual {
		normalized := metadata.NormalizeLikePattern(value)
		request.AddEqualStringCondition(common.ICAT_COLUMN_COLL_NAME, path.Dir(normalized))
		request.AddEqualStringCondition(common.ICAT_COLUMN_DATA_NAME, path.Base(normalized))
		return nil
	}

	pattern := metadata.NormalizeLikePattern(value)
	request.AddLikeStringCondition(common.ICAT_COLUMN_COLL_NAME, path.Dir(pattern))
	request.AddLikeStringCondition(common.ICAT_COLUMN_DATA_NAME, path.Base(pattern))
	return nil
}

func addStringCondition(request *message.IRODSMessageQueryRequest, column common.ICATColumnNumber, condition metadata.EntryCondition) {
	if condition.Op == metadata.OpLike {
		request.AddLikeStringCondition(column, metadata.NormalizeLikePattern(condition.Value))
		return
	}
	request.AddEqualStringCondition(column, condition.Value)
}

func requestGenQuery(conn *connection.IRODSConnection, request *message.IRODSMessageQueryRequest, response *message.IRODSMessageQueryResponse, branchName string) error {
	if err := conn.Request(request, response, nil, conn.GetLongResponseOperationTimeout()); err != nil {
		if types.GetIRODSErrorCode(err) == common.CAT_NO_ROWS_FOUND {
			return err
		}
		return fmt.Errorf("failed to receive %s query result message: %w", branchName, err)
	}
	if err := response.CheckError(); err != nil {
		if types.GetIRODSErrorCode(err) == common.CAT_NO_ROWS_FOUND {
			return err
		}
		return fmt.Errorf("received %s query error: %w", branchName, err)
	}
	return nil
}

func isNoRowsError(err error) bool {
	return types.GetIRODSErrorCode(err) == common.CAT_NO_ROWS_FOUND
}

func parseCollectionQueryRows(queryResult message.IRODSMessageQueryResponse, includeAVUs bool) ([]collectionQueryRow, error) {
	if queryResult.RowCount == 0 {
		return nil, nil
	}
	if queryResult.AttributeCount > len(queryResult.SQLResult) {
		return nil, fmt.Errorf("failed to receive collection attributes - requires %d, but received %d attributes", queryResult.AttributeCount, len(queryResult.SQLResult))
	}

	rows := make([]collectionQueryRow, queryResult.RowCount)
	for attr := 0; attr < queryResult.AttributeCount; attr++ {
		sqlResult := queryResult.SQLResult[attr]
		if len(sqlResult.Values) != queryResult.RowCount {
			return nil, fmt.Errorf("failed to receive collection rows - requires %d, but received %d attributes", queryResult.RowCount, len(sqlResult.Values))
		}

		for row := 0; row < queryResult.RowCount; row++ {
			if rows[row].collection == nil {
				rows[row].collection = &types.IRODSCollection{
					ID:         -1,
					Path:       "",
					Name:       "",
					Owner:      "",
					CreateTime: time.Time{},
					ModifyTime: time.Time{},
				}
			}
			if includeAVUs && rows[row].avu == nil {
				rows[row].avu = &metadata.AVUStat{}
			}

			value := sqlResult.Values[row]
			switch sqlResult.AttributeIndex {
			case int(common.ICAT_COLUMN_COLL_ID):
				id, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection id %q: %w", value, err)
				}
				rows[row].collection.ID = id
			case int(common.ICAT_COLUMN_COLL_NAME):
				rows[row].collection.Path = value
				rows[row].collection.Name = util.GetIRODSPathFileName(value)
			case int(common.ICAT_COLUMN_COLL_OWNER_NAME):
				rows[row].collection.Owner = value
			case int(common.ICAT_COLUMN_COLL_CREATE_TIME):
				createTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection create time %q: %w", value, err)
				}
				rows[row].collection.CreateTime = createTime
			case int(common.ICAT_COLUMN_COLL_MODIFY_TIME):
				modifyTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection modify time %q: %w", value, err)
				}
				rows[row].collection.ModifyTime = modifyTime
			case int(common.ICAT_COLUMN_META_COLL_ATTR_ID):
				id, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection AVU id %q: %w", value, err)
				}
				rows[row].avu.ID = id
			case int(common.ICAT_COLUMN_META_COLL_ATTR_NAME):
				rows[row].avu.Name = value
			case int(common.ICAT_COLUMN_META_COLL_ATTR_VALUE):
				rows[row].avu.Value = value
			case int(common.ICAT_COLUMN_META_COLL_ATTR_UNITS):
				rows[row].avu.Units = value
			case int(common.ICAT_COLUMN_META_COLL_CREATE_TIME):
				createTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection AVU create time %q: %w", value, err)
				}
				rows[row].avu.CreateTime = createTime
			case int(common.ICAT_COLUMN_META_COLL_MODIFY_TIME):
				modifyTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection AVU modify time %q: %w", value, err)
				}
				rows[row].avu.ModifyTime = modifyTime
			}
		}
	}
	return rows, nil
}

func parseDataObjectQueryRows(queryResult message.IRODSMessageQueryResponse, includeAVUs bool) ([]dataObjectQueryRow, error) {
	if queryResult.RowCount == 0 {
		return nil, nil
	}
	if queryResult.AttributeCount > len(queryResult.SQLResult) {
		return nil, fmt.Errorf("failed to receive data object attributes - requires %d, but received %d attributes", queryResult.AttributeCount, len(queryResult.SQLResult))
	}

	rows := make([]dataObjectQueryRow, queryResult.RowCount)
	for attr := 0; attr < queryResult.AttributeCount; attr++ {
		sqlResult := queryResult.SQLResult[attr]
		if len(sqlResult.Values) != queryResult.RowCount {
			return nil, fmt.Errorf("failed to receive data object rows - requires %d, but received %d attributes", queryResult.RowCount, len(sqlResult.Values))
		}

		for row := 0; row < queryResult.RowCount; row++ {
			if rows[row].dataObject == nil {
				replica := &types.IRODSReplica{
					Number:            -1,
					Owner:             "",
					Checksum:          nil,
					Status:            "",
					ResourceName:      "",
					Path:              "",
					ResourceHierarchy: "",
					CreateTime:        time.Time{},
					ModifyTime:        time.Time{},
					AccessTime:        time.Time{},
				}
				rows[row].dataObject = &types.IRODSDataObject{
					ID:           -1,
					CollectionID: -1,
					Path:         "",
					Name:         "",
					Size:         0,
					DataType:     "",
					Replicas:     []*types.IRODSReplica{replica},
				}
			}
			if includeAVUs && rows[row].avu == nil {
				rows[row].avu = &metadata.AVUStat{}
			}

			value := sqlResult.Values[row]
			switch sqlResult.AttributeIndex {
			case int(common.ICAT_COLUMN_COLL_ID):
				id, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse collection id %q: %w", value, err)
				}
				rows[row].dataObject.CollectionID = id
			case int(common.ICAT_COLUMN_COLL_NAME):
				if rows[row].dataObject.Path != "" {
					rows[row].dataObject.Path = util.MakeIRODSPath(value, rows[row].dataObject.Path)
				} else {
					rows[row].dataObject.Path = value
				}
			case int(common.ICAT_COLUMN_D_DATA_ID):
				id, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object id %q: %w", value, err)
				}
				rows[row].dataObject.ID = id
			case int(common.ICAT_COLUMN_DATA_NAME):
				if rows[row].dataObject.Path != "" {
					rows[row].dataObject.Path = util.MakeIRODSPath(rows[row].dataObject.Path, value)
				} else {
					rows[row].dataObject.Path = value
				}
				rows[row].dataObject.Name = value
			case int(common.ICAT_COLUMN_DATA_SIZE):
				size, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object size %q: %w", value, err)
				}
				rows[row].dataObject.Size = size
			case int(common.ICAT_COLUMN_DATA_TYPE_NAME):
				rows[row].dataObject.DataType = value
			case int(common.ICAT_COLUMN_DATA_REPL_NUM):
				number, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object replica number %q: %w", value, err)
				}
				rows[row].dataObject.Replicas[0].Number = number
			case int(common.ICAT_COLUMN_D_OWNER_NAME):
				rows[row].dataObject.Replicas[0].Owner = value
			case int(common.ICAT_COLUMN_D_DATA_CHECKSUM):
				checksum, err := types.CreateIRODSChecksum(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object checksum %q: %w", value, err)
				}
				rows[row].dataObject.Replicas[0].Checksum = checksum
			case int(common.ICAT_COLUMN_D_REPL_STATUS):
				rows[row].dataObject.Replicas[0].Status = value
			case int(common.ICAT_COLUMN_D_RESC_NAME):
				rows[row].dataObject.Replicas[0].ResourceName = value
			case int(common.ICAT_COLUMN_D_DATA_PATH):
				rows[row].dataObject.Replicas[0].Path = value
			case int(common.ICAT_COLUMN_D_RESC_HIER):
				rows[row].dataObject.Replicas[0].ResourceHierarchy = value
			case int(common.ICAT_COLUMN_D_CREATE_TIME):
				createTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object create time %q: %w", value, err)
				}
				rows[row].dataObject.Replicas[0].CreateTime = createTime
			case int(common.ICAT_COLUMN_D_MODIFY_TIME):
				modifyTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object modify time %q: %w", value, err)
				}
				rows[row].dataObject.Replicas[0].ModifyTime = modifyTime
			case int(common.ICAT_COLUMN_D_ACCESS_TIME):
				accessTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object access time %q: %w", value, err)
				}
				rows[row].dataObject.Replicas[0].AccessTime = accessTime
			case int(common.ICAT_COLUMN_META_DATA_ATTR_ID):
				id, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object AVU id %q: %w", value, err)
				}
				rows[row].avu.ID = id
			case int(common.ICAT_COLUMN_META_DATA_ATTR_NAME):
				rows[row].avu.Name = value
			case int(common.ICAT_COLUMN_META_DATA_ATTR_VALUE):
				rows[row].avu.Value = value
			case int(common.ICAT_COLUMN_META_DATA_ATTR_UNITS):
				rows[row].avu.Units = value
			case int(common.ICAT_COLUMN_META_DATA_CREATE_TIME):
				createTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object AVU create time %q: %w", value, err)
				}
				rows[row].avu.CreateTime = createTime
			case int(common.ICAT_COLUMN_META_DATA_MODIFY_TIME):
				modifyTime, err := parseIRODSTime(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse data object AVU modify time %q: %w", value, err)
				}
				rows[row].avu.ModifyTime = modifyTime
			}
		}
	}
	return rows, nil
}

func preferDataObjectReplica(existing *types.IRODSDataObject, candidate *types.IRODSDataObject) bool {
	if existing == nil || len(existing.Replicas) == 0 {
		return true
	}
	if candidate == nil || len(candidate.Replicas) == 0 {
		return false
	}

	existingReplica := existing.Replicas[0]
	candidateReplica := candidate.Replicas[0]
	if existingReplica.Status != "1" && candidateReplica.Status == "1" {
		return true
	}
	if existingReplica.Status == "1" && candidateReplica.Status != "1" {
		return false
	}
	if existingReplica.CreateTime.IsZero() {
		return true
	}
	if candidateReplica.CreateTime.IsZero() {
		return false
	}
	return existingReplica.CreateTime.After(candidateReplica.CreateTime)
}

func parseIRODSTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return util.GetIRODSDateTime(value)
}

func mergeMatchedAVUs(target map[string][]metadata.AVUStat, source map[string][]metadata.AVUStat) {
	if target == nil || source == nil {
		return
	}
	for pathKey, avus := range source {
		for _, avu := range avus {
			appendMatchedAVU(target, pathKey, avu)
		}
	}
}

func appendMatchedAVU(target map[string][]metadata.AVUStat, pathKey string, avu metadata.AVUStat) {
	if target == nil || pathKey == "" || avu.Name == "" {
		return
	}
	target[pathKey] = appendUniqueAVU(target[pathKey], avu)
}

func appendUniqueAVU(existing []metadata.AVUStat, avu metadata.AVUStat) []metadata.AVUStat {
	for _, candidate := range existing {
		if candidate.Name == avu.Name && candidate.Value == avu.Value && candidate.Units == avu.Units {
			return existing
		}
	}
	return append(existing, avu)
}
