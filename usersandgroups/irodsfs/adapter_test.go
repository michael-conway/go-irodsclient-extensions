package irodsfs

import (
	"testing"

	"github.com/cyverse/go-irodsclient/irods/common"
	"github.com/cyverse/go-irodsclient/irods/message"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestParsePrincipalRows(t *testing.T) {
	rows, err := parsePrincipalRows(message.IRODSMessageQueryResponse{
		RowCount:       2,
		AttributeCount: 4,
		SQLResult: []message.IRODSMessageSQLResult{
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ID), Values: []string{"101", "102"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_NAME), Values: []string{"alice", "research-team"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ZONE), Values: []string{"tempZone", "tempZone"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_TYPE), Values: []string{"rodsuser", "rodsgroup"}},
		},
	})
	if err != nil {
		t.Fatalf("parsePrincipalRows returned error: %v", err)
	}
	if rows[0].ID != 101 || rows[0].Name != "alice" || rows[0].Type != irodstypes.IRODSUserRodsUser {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].ID != 102 || rows[1].Name != "research-team" || rows[1].Type != irodstypes.IRODSUserRodsGroup {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestParseMembershipRows(t *testing.T) {
	rows, err := parseMembershipRows(message.IRODSMessageQueryResponse{
		RowCount:       1,
		AttributeCount: 6,
		SQLResult: []message.IRODSMessageSQLResult{
			{AttributeIndex: int(common.ICAT_COLUMN_COLL_USER_GROUP_ID), Values: []string{"201"}},
			{AttributeIndex: int(common.ICAT_COLUMN_COLL_USER_GROUP_NAME), Values: []string{"research-team"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ID), Values: []string{"101"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_NAME), Values: []string{"alice"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ZONE), Values: []string{"tempZone"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_TYPE), Values: []string{"rodsuser"}},
		},
	})
	if err != nil {
		t.Fatalf("parseMembershipRows returned error: %v", err)
	}
	if rows[0].GroupID != 201 || rows[0].GroupName != "research-team" || rows[0].UserID != 101 || rows[0].UserName != "alice" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

func TestParseGroupMemberCountRows(t *testing.T) {
	rows, err := parseGroupMemberCountRows(message.IRODSMessageQueryResponse{
		RowCount:       2,
		AttributeCount: 3,
		SQLResult: []message.IRODSMessageSQLResult{
			{AttributeIndex: int(common.ICAT_COLUMN_COLL_USER_GROUP_ID), Values: []string{"201", "202"}},
			{AttributeIndex: int(common.ICAT_COLUMN_COLL_USER_GROUP_NAME), Values: []string{"research-team", "science-team"}},
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ID), Values: []string{"3", "0"}},
		},
	})
	if err != nil {
		t.Fatalf("parseGroupMemberCountRows returned error: %v", err)
	}
	if rows[0].GroupID != 201 || rows[0].GroupName != "research-team" || rows[0].MemberCount != 3 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].GroupID != 202 || rows[1].GroupName != "science-team" || rows[1].MemberCount != 0 {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestParseRowsRejectMalformedResponses(t *testing.T) {
	_, err := parsePrincipalRows(message.IRODSMessageQueryResponse{
		RowCount:       1,
		AttributeCount: 2,
		SQLResult: []message.IRODSMessageSQLResult{
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ID), Values: []string{"101"}},
		},
	})
	if err == nil {
		t.Fatal("expected malformed principal response error")
	}

	_, err = parseMembershipRows(message.IRODSMessageQueryResponse{
		RowCount:       2,
		AttributeCount: 1,
		SQLResult: []message.IRODSMessageSQLResult{
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ID), Values: []string{"101"}},
		},
	})
	if err == nil {
		t.Fatal("expected malformed membership response error")
	}

	_, err = parseGroupMemberCountRows(message.IRODSMessageQueryResponse{
		RowCount:       1,
		AttributeCount: 1,
		SQLResult: []message.IRODSMessageSQLResult{
			{AttributeIndex: int(common.ICAT_COLUMN_USER_ID), Values: []string{"not-an-int"}},
		},
	})
	if err == nil {
		t.Fatal("expected malformed group member count response error")
	}
}
