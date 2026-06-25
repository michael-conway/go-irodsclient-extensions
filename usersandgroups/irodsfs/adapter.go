package irodsfs

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	cyfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/cyverse/go-irodsclient/irods/common"
	"github.com/cyverse/go-irodsclient/irods/connection"
	"github.com/cyverse/go-irodsclient/irods/message"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/cyverse/go-irodsclient/irods/util"
	"github.com/michael-conway/go-irodsclient-extensions/usersandgroups"
)

type Adapter struct {
	filesystem *cyfs.FileSystem
}

var _ usersandgroups.Catalog = (*Adapter)(nil)
var _ usersandgroups.Mutator = (*Adapter)(nil)

func NewAdapter(filesystem *cyfs.FileSystem) *Adapter {
	return &Adapter{filesystem: filesystem}
}

func (adapter *Adapter) ListGroupSummaries(ctx context.Context, options usersandgroups.GroupSummaryOptions) ([]usersandgroups.GroupSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	groups, err := adapter.listPrincipals(ctx, principalQueryOptions{
		Zone:   options.Zone,
		Prefix: options.Prefix,
		Type:   irodstypes.IRODSUserRodsGroup,
		Limit:  options.Limit,
	})
	if err != nil {
		return nil, err
	}

	counts, err := adapter.groupMemberCounts(ctx, options.Zone, options.Prefix)
	if err != nil {
		return nil, err
	}

	summaries := make([]usersandgroups.GroupSummary, 0, len(groups))
	for _, group := range groups {
		summaries = append(summaries, usersandgroups.GroupSummary{
			ID:          group.ID,
			Name:        group.Name,
			Zone:        group.Zone,
			Type:        group.Type,
			MemberCount: counts[group.Name],
		})
	}
	return summaries, nil
}

func (adapter *Adapter) ListUserMembershipSummaries(ctx context.Context, options usersandgroups.UserMembershipSummaryOptions) ([]usersandgroups.UserMembershipSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	users, err := adapter.listPrincipals(ctx, principalQueryOptions{
		Zone:        options.Zone,
		Prefix:      options.Prefix,
		Type:        options.Type,
		ExcludeType: irodstypes.IRODSUserRodsGroup,
		Limit:       options.Limit,
	})
	if err != nil {
		return nil, err
	}

	memberships, err := adapter.membershipsByUser(ctx, options.Zone, options.Prefix)
	if err != nil {
		return nil, err
	}

	summaries := make([]usersandgroups.UserMembershipSummary, 0, len(users))
	for _, user := range users {
		summaries = append(summaries, usersandgroups.UserMembershipSummary{
			ID:     user.ID,
			Name:   user.Name,
			Zone:   user.Zone,
			Type:   user.Type,
			Groups: memberships[user.Name],
		})
	}
	return summaries, nil
}

func (adapter *Adapter) ListGroupsForUser(ctx context.Context, options usersandgroups.GroupsForUserOptions) ([]usersandgroups.GroupRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := adapter.queryMembershipRows(ctx, membershipQueryOptions{
		Zone:     options.Zone,
		UserName: options.UserName,
		Limit:    options.Limit,
	})
	if err != nil {
		return nil, err
	}

	groupsByName := map[string]usersandgroups.GroupRef{}
	for _, row := range rows {
		if row.GroupName == "" || row.GroupName == options.UserName {
			continue
		}
		groupsByName[row.GroupName] = usersandgroups.GroupRef{
			ID:   row.GroupID,
			Name: row.GroupName,
			Zone: options.Zone,
			Type: irodstypes.IRODSUserRodsGroup,
		}
	}

	groups := make([]usersandgroups.GroupRef, 0, len(groupsByName))
	for _, group := range groupsByName {
		groups = append(groups, group)
	}
	sortGroupRefs(groups)
	return groups, nil
}

func (adapter *Adapter) SearchPrincipals(ctx context.Context, options usersandgroups.PrincipalSearchOptions) ([]usersandgroups.PrincipalSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	remaining := options.Limit
	results := make([]usersandgroups.PrincipalSearchResult, 0, remaining)
	for _, kind := range options.Kinds {
		if remaining <= 0 {
			break
		}
		queryOptions := principalQueryOptions{
			Zone:   options.Zone,
			Prefix: options.Query,
			Limit:  remaining,
		}
		switch kind {
		case usersandgroups.PrincipalKindUser:
			queryOptions.ExcludeType = irodstypes.IRODSUserRodsGroup
		case usersandgroups.PrincipalKindGroup:
			queryOptions.Type = irodstypes.IRODSUserRodsGroup
		default:
			continue
		}

		principals, err := adapter.listPrincipals(ctx, queryOptions)
		if err != nil {
			return nil, err
		}
		for _, principal := range principals {
			results = append(results, usersandgroups.PrincipalSearchResult{
				ID:    principal.ID,
				Name:  principal.Name,
				Zone:  principal.Zone,
				Type:  principal.Type,
				Kind:  kind,
				Score: principalScore(principal.Name, options.Query),
			})
		}
		remaining = options.Limit - len(results)
	}

	sort.SliceStable(results, func(i int, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Kind != results[j].Kind {
			return results[i].Kind < results[j].Kind
		}
		return results[i].Name < results[j].Name
	})
	if len(results) > options.Limit {
		results = results[:options.Limit]
	}
	return results, nil
}

func (adapter *Adapter) CreateRodsUserWithPassword(ctx context.Context, request usersandgroups.CreateRodsUserWithPasswordRequest) (usersandgroups.User, error) {
	if err := ctx.Err(); err != nil {
		return usersandgroups.User{}, err
	}
	if adapter == nil || adapter.filesystem == nil {
		return usersandgroups.User{}, usersandgroups.ErrMissingCatalog
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return usersandgroups.User{}, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	if err := createRodsUserWithPassword(conn, request.Name, request.Zone, request.Password); err != nil {
		return usersandgroups.User{}, err
	}

	user, err := adapter.filesystem.GetUser(request.Name, request.Zone, irodstypes.IRODSUserRodsUser)
	if err != nil {
		return usersandgroups.User{}, err
	}

	return usersandgroups.User{
		ID:   user.ID,
		Name: user.Name,
		Zone: user.Zone,
		Type: user.Type,
	}, nil
}

func (adapter *Adapter) CreateGroup(ctx context.Context, request usersandgroups.GroupRequest) (usersandgroups.GroupRef, error) {
	if err := ctx.Err(); err != nil {
		return usersandgroups.GroupRef{}, err
	}
	if adapter == nil || adapter.filesystem == nil {
		return usersandgroups.GroupRef{}, usersandgroups.ErrMissingCatalog
	}

	if err := adapter.userAdminRequest("mkgroup", request.Name, string(irodstypes.IRODSUserRodsGroup), request.Zone, "", "", "", ""); err != nil {
		return usersandgroups.GroupRef{}, fmt.Errorf("received user-admin mkgroup error for group %q, zone %q: %w", request.Name, request.Zone, err)
	}

	group, err := adapter.filesystem.GetUser(request.Name, request.Zone, irodstypes.IRODSUserRodsGroup)
	if err != nil {
		return usersandgroups.GroupRef{}, err
	}
	return usersandgroups.GroupRef{
		ID:   group.ID,
		Name: group.Name,
		Zone: group.Zone,
		Type: group.Type,
	}, nil
}

func (adapter *Adapter) AddGroupMember(ctx context.Context, request usersandgroups.GroupMemberRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return adapter.userAdminRequest("modify", "group", request.GroupName, "add", request.UserName, request.Zone, "", "")
}

func (adapter *Adapter) RemoveGroupMember(ctx context.Context, request usersandgroups.GroupMemberRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return adapter.userAdminRequest("modify", "group", request.GroupName, "remove", request.UserName, request.Zone, "", "")
}

func createRodsUserWithPassword(conn *connection.IRODSConnection, username string, zone string, password string) error {
	conn.Lock()
	defer conn.Unlock()

	account := conn.GetAccount()
	oldPassword := account.Password
	if account.AuthenticationScheme.IsPAM() {
		oldPassword = conn.GetPAMToken()
	}
	scrambledPassword := util.Scramble(util.GetPasswordPadded(password), oldPassword, "", false)

	req := message.NewIRODSMessageUserAdminRequest("mkuser", username, scrambledPassword, zone, "", "", "", "")
	if err := conn.RequestAndCheck(req, &message.IRODSMessageAdminResponse{}, nil, conn.GetOperationTimeout()); err != nil {
		return fmt.Errorf("received user-admin mkuser error for user %q, zone %q: %w", username, zone, err)
	}
	return nil
}

func (adapter *Adapter) userAdminRequest(action string, args ...string) error {
	if adapter == nil || adapter.filesystem == nil {
		return usersandgroups.ErrMissingCatalog
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	conn.Lock()
	defer conn.Unlock()

	req := message.NewIRODSMessageUserAdminRequest(action, args...)
	return conn.RequestAndCheck(req, &message.IRODSMessageAdminResponse{}, nil, conn.GetOperationTimeout())
}

type principalQueryOptions struct {
	Zone        string
	Prefix      string
	Type        irodstypes.IRODSUserType
	ExcludeType irodstypes.IRODSUserType
	Limit       int
}

type principalRow struct {
	ID   int64
	Name string
	Zone string
	Type irodstypes.IRODSUserType
}

type membershipQueryOptions struct {
	Zone        string
	UserName    string
	UserPrefix  string
	GroupPrefix string
	Limit       int
}

type membershipRow struct {
	GroupID   int64
	GroupName string
	UserID    int64
	UserName  string
	UserZone  string
	UserType  irodstypes.IRODSUserType
}

type groupMemberCountRow struct {
	GroupID     int64
	GroupName   string
	MemberCount int
}

func (adapter *Adapter) listPrincipals(ctx context.Context, options principalQueryOptions) ([]principalRow, error) {
	if adapter == nil || adapter.filesystem == nil {
		return nil, usersandgroups.ErrMissingCatalog
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return nil, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	conn.Lock()
	defer conn.Unlock()

	rows := []principalRow{}
	continueIndex := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		request := message.NewIRODSMessageQueryRequest(common.MaxQueryRows, continueIndex, 0, 0)
		request.AddSelect(common.ICAT_COLUMN_USER_ID)
		request.AddSelect(common.ICAT_COLUMN_USER_NAME)
		request.AddSelect(common.ICAT_COLUMN_USER_TYPE)
		request.AddSelect(common.ICAT_COLUMN_USER_ZONE)
		if strings.TrimSpace(options.Zone) != "" {
			request.AddEqualStringCondition(common.ICAT_COLUMN_USER_ZONE, options.Zone)
		}
		if strings.TrimSpace(options.Prefix) != "" {
			request.AddLikeStringCondition(common.ICAT_COLUMN_USER_NAME, escapeLikePrefix(options.Prefix))
		}
		if options.Type != "" {
			request.AddEqualStringCondition(common.ICAT_COLUMN_USER_TYPE, string(options.Type))
		}
		if options.ExcludeType != "" {
			request.AddCondition(common.ICAT_COLUMN_USER_TYPE, fmt.Sprintf("!= '%s'", options.ExcludeType))
		}

		response := message.IRODSMessageQueryResponse{}
		if err := requestGenQuery(conn, request, &response, "principal"); err != nil {
			return nil, err
		}
		page, err := parsePrincipalRows(response)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page...)
		if options.Limit > 0 && len(rows) >= options.Limit {
			rows = rows[:options.Limit]
			break
		}
		continueIndex = response.ContinueIndex
		if continueIndex == 0 {
			break
		}
	}

	sort.SliceStable(rows, func(i int, j int) bool {
		if rows[i].Zone != rows[j].Zone {
			return rows[i].Zone < rows[j].Zone
		}
		return rows[i].Name < rows[j].Name
	})
	return rows, nil
}

func (adapter *Adapter) groupMemberCounts(ctx context.Context, zone string, groupPrefix string) (map[string]int, error) {
	rows, err := adapter.queryGroupMemberCountRows(ctx, zone, groupPrefix)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, row := range rows {
		if row.GroupName == "" {
			continue
		}
		counts[row.GroupName] = row.MemberCount
	}
	return counts, nil
}

func (adapter *Adapter) membershipsByUser(ctx context.Context, zone string, userPrefix string) (map[string][]usersandgroups.GroupRef, error) {
	rows, err := adapter.queryMembershipRows(ctx, membershipQueryOptions{
		Zone:       zone,
		UserPrefix: userPrefix,
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]map[string]bool{}
	memberships := map[string][]usersandgroups.GroupRef{}
	for _, row := range rows {
		if row.UserName == "" || row.GroupName == "" || row.GroupName == row.UserName {
			continue
		}
		if seen[row.UserName] == nil {
			seen[row.UserName] = map[string]bool{}
		}
		if seen[row.UserName][row.GroupName] {
			continue
		}
		seen[row.UserName][row.GroupName] = true
		memberships[row.UserName] = append(memberships[row.UserName], usersandgroups.GroupRef{
			ID:   row.GroupID,
			Name: row.GroupName,
			Zone: zone,
			Type: irodstypes.IRODSUserRodsGroup,
		})
	}
	for username := range memberships {
		sortGroupRefs(memberships[username])
	}
	return memberships, nil
}

func (adapter *Adapter) queryMembershipRows(ctx context.Context, options membershipQueryOptions) ([]membershipRow, error) {
	if adapter == nil || adapter.filesystem == nil {
		return nil, usersandgroups.ErrMissingCatalog
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return nil, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	conn.Lock()
	defer conn.Unlock()

	rows := []membershipRow{}
	continueIndex := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		request := message.NewIRODSMessageQueryRequest(common.MaxQueryRows, continueIndex, 0, 0)
		request.AddSelect(common.ICAT_COLUMN_COLL_USER_GROUP_ID)
		request.AddSelect(common.ICAT_COLUMN_COLL_USER_GROUP_NAME)
		request.AddSelect(common.ICAT_COLUMN_USER_ID)
		request.AddSelect(common.ICAT_COLUMN_USER_NAME)
		request.AddSelect(common.ICAT_COLUMN_USER_TYPE)
		request.AddSelect(common.ICAT_COLUMN_USER_ZONE)
		if strings.TrimSpace(options.Zone) != "" {
			request.AddEqualStringCondition(common.ICAT_COLUMN_USER_ZONE, options.Zone)
		}
		if strings.TrimSpace(options.UserName) != "" {
			request.AddEqualStringCondition(common.ICAT_COLUMN_USER_NAME, options.UserName)
		}
		if strings.TrimSpace(options.UserPrefix) != "" {
			request.AddLikeStringCondition(common.ICAT_COLUMN_USER_NAME, escapeLikePrefix(options.UserPrefix))
		}
		if strings.TrimSpace(options.GroupPrefix) != "" {
			request.AddLikeStringCondition(common.ICAT_COLUMN_COLL_USER_GROUP_NAME, escapeLikePrefix(options.GroupPrefix))
		}

		response := message.IRODSMessageQueryResponse{}
		if err := requestGenQuery(conn, request, &response, "membership"); err != nil {
			return nil, err
		}
		page, err := parseMembershipRows(response)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page...)
		if options.Limit > 0 && len(rows) >= options.Limit {
			rows = rows[:options.Limit]
			break
		}
		continueIndex = response.ContinueIndex
		if continueIndex == 0 {
			break
		}
	}
	return rows, nil
}

func (adapter *Adapter) queryGroupMemberCountRows(ctx context.Context, zone string, groupPrefix string) ([]groupMemberCountRow, error) {
	if adapter == nil || adapter.filesystem == nil {
		return nil, usersandgroups.ErrMissingCatalog
	}

	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return nil, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	conn.Lock()
	defer conn.Unlock()

	rows := []groupMemberCountRow{}
	continueIndex := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		request := message.NewIRODSMessageQueryRequest(common.MaxQueryRows, continueIndex, 0, 0)
		request.AddSelect(common.ICAT_COLUMN_COLL_USER_GROUP_ID)
		request.AddSelect(common.ICAT_COLUMN_COLL_USER_GROUP_NAME)
		request.AddSelectWithCount(common.ICAT_COLUMN_USER_ID)
		if strings.TrimSpace(zone) != "" {
			request.AddEqualStringCondition(common.ICAT_COLUMN_USER_ZONE, zone)
		}
		if strings.TrimSpace(groupPrefix) != "" {
			request.AddLikeStringCondition(common.ICAT_COLUMN_COLL_USER_GROUP_NAME, escapeLikePrefix(groupPrefix))
		}
		request.AddCondition(common.ICAT_COLUMN_USER_TYPE, fmt.Sprintf("!= '%s'", irodstypes.IRODSUserRodsGroup))

		response := message.IRODSMessageQueryResponse{}
		if err := requestGenQuery(conn, request, &response, "group member count"); err != nil {
			return nil, err
		}
		page, err := parseGroupMemberCountRows(response)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page...)
		continueIndex = response.ContinueIndex
		if continueIndex == 0 {
			break
		}
	}
	return rows, nil
}

func requestGenQuery(conn *connection.IRODSConnection, request *message.IRODSMessageQueryRequest, response *message.IRODSMessageQueryResponse, name string) error {
	if conn == nil || !conn.IsConnected() {
		return fmt.Errorf("%s query connection is nil or disconnected", name)
	}
	if err := conn.Request(request, response, nil, conn.GetLongResponseOperationTimeout()); err != nil {
		if irodstypes.GetIRODSErrorCode(err) == common.CAT_NO_ROWS_FOUND {
			return nil
		}
		return fmt.Errorf("failed to receive %s query result: %w", name, err)
	}
	if err := response.CheckError(); err != nil {
		if irodstypes.GetIRODSErrorCode(err) == common.CAT_NO_ROWS_FOUND {
			return nil
		}
		return fmt.Errorf("received %s query error: %w", name, err)
	}
	return nil
}

func parsePrincipalRows(response message.IRODSMessageQueryResponse) ([]principalRow, error) {
	if response.AttributeCount > len(response.SQLResult) {
		return nil, fmt.Errorf("principal query returned %d attributes but only %d result columns", response.AttributeCount, len(response.SQLResult))
	}
	rows := make([]principalRow, response.RowCount)
	for attr := 0; attr < response.AttributeCount; attr++ {
		sqlResult := response.SQLResult[attr]
		if len(sqlResult.Values) != response.RowCount {
			return nil, fmt.Errorf("principal query returned %d rows but attribute %d has %d values", response.RowCount, sqlResult.AttributeIndex, len(sqlResult.Values))
		}
		for row := 0; row < response.RowCount; row++ {
			switch sqlResult.AttributeIndex {
			case int(common.ICAT_COLUMN_USER_ID):
				id, err := parseInt64(sqlResult.Values[row], "user id")
				if err != nil {
					return nil, err
				}
				rows[row].ID = id
			case int(common.ICAT_COLUMN_USER_NAME):
				rows[row].Name = sqlResult.Values[row]
			case int(common.ICAT_COLUMN_USER_ZONE):
				rows[row].Zone = sqlResult.Values[row]
			case int(common.ICAT_COLUMN_USER_TYPE):
				rows[row].Type = irodstypes.IRODSUserType(sqlResult.Values[row])
			}
		}
	}
	return rows, nil
}

func parseMembershipRows(response message.IRODSMessageQueryResponse) ([]membershipRow, error) {
	if response.AttributeCount > len(response.SQLResult) {
		return nil, fmt.Errorf("membership query returned %d attributes but only %d result columns", response.AttributeCount, len(response.SQLResult))
	}
	rows := make([]membershipRow, response.RowCount)
	for attr := 0; attr < response.AttributeCount; attr++ {
		sqlResult := response.SQLResult[attr]
		if len(sqlResult.Values) != response.RowCount {
			return nil, fmt.Errorf("membership query returned %d rows but attribute %d has %d values", response.RowCount, sqlResult.AttributeIndex, len(sqlResult.Values))
		}
		for row := 0; row < response.RowCount; row++ {
			switch sqlResult.AttributeIndex {
			case int(common.ICAT_COLUMN_COLL_USER_GROUP_ID):
				id, err := parseInt64(sqlResult.Values[row], "group id")
				if err != nil {
					return nil, err
				}
				rows[row].GroupID = id
			case int(common.ICAT_COLUMN_COLL_USER_GROUP_NAME):
				rows[row].GroupName = sqlResult.Values[row]
			case int(common.ICAT_COLUMN_USER_ID):
				id, err := parseInt64(sqlResult.Values[row], "user id")
				if err != nil {
					return nil, err
				}
				rows[row].UserID = id
			case int(common.ICAT_COLUMN_USER_NAME):
				rows[row].UserName = sqlResult.Values[row]
			case int(common.ICAT_COLUMN_USER_ZONE):
				rows[row].UserZone = sqlResult.Values[row]
			case int(common.ICAT_COLUMN_USER_TYPE):
				rows[row].UserType = irodstypes.IRODSUserType(sqlResult.Values[row])
			}
		}
	}
	return rows, nil
}

func parseGroupMemberCountRows(response message.IRODSMessageQueryResponse) ([]groupMemberCountRow, error) {
	if response.AttributeCount > len(response.SQLResult) {
		return nil, fmt.Errorf("group member count query returned %d attributes but only %d result columns", response.AttributeCount, len(response.SQLResult))
	}
	rows := make([]groupMemberCountRow, response.RowCount)
	for attr := 0; attr < response.AttributeCount; attr++ {
		sqlResult := response.SQLResult[attr]
		if len(sqlResult.Values) != response.RowCount {
			return nil, fmt.Errorf("group member count query returned %d rows but attribute %d has %d values", response.RowCount, sqlResult.AttributeIndex, len(sqlResult.Values))
		}
		for row := 0; row < response.RowCount; row++ {
			switch sqlResult.AttributeIndex {
			case int(common.ICAT_COLUMN_COLL_USER_GROUP_ID):
				id, err := parseInt64(sqlResult.Values[row], "group id")
				if err != nil {
					return nil, err
				}
				rows[row].GroupID = id
			case int(common.ICAT_COLUMN_COLL_USER_GROUP_NAME):
				rows[row].GroupName = sqlResult.Values[row]
			case int(common.ICAT_COLUMN_USER_ID):
				count, err := parseInt64(sqlResult.Values[row], "group member count")
				if err != nil {
					return nil, err
				}
				rows[row].MemberCount = int(count)
			}
		}
	}
	return rows, nil
}

func parseInt64(value string, field string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s %q: %w", field, value, err)
	}
	return parsed, nil
}

func escapeLikePrefix(prefix string) string {
	return strings.ReplaceAll(strings.TrimSpace(prefix), "'", "''") + "%"
}

func principalScore(name string, query string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	query = strings.ToLower(strings.TrimSpace(query))
	switch {
	case name == query:
		return 100
	case strings.HasPrefix(name, query):
		return 80
	case strings.Contains(name, query):
		return 50
	default:
		return 0
	}
}

func sortGroupRefs(groups []usersandgroups.GroupRef) {
	sort.SliceStable(groups, func(i int, j int) bool {
		if groups[i].Zone != groups[j].Zone {
			return groups[i].Zone < groups[j].Zone
		}
		return groups[i].Name < groups[j].Name
	})
}
