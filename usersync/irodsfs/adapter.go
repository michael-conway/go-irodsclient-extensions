package irodsfs

import (
	"fmt"

	cyfs "github.com/cyverse/go-irodsclient/fs"
	irodscommon "github.com/cyverse/go-irodsclient/irods/common"
	irodsmessage "github.com/cyverse/go-irodsclient/irods/message"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/usersync"
)

// Adapter implements usersync.Filesystem using go-irodsclient.
type Adapter struct {
	filesystem *cyfs.FileSystem
}

var _ usersync.Filesystem = (*Adapter)(nil)

// NewAdapter returns a usersync filesystem adapter backed by go-irodsclient.
func NewAdapter(filesystem *cyfs.FileSystem) *Adapter {
	return &Adapter{filesystem: filesystem}
}

func (adapter *Adapter) GetUser(username string, zoneName string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	return adapter.filesystem.GetUser(username, zoneName, userType)
}

func (adapter *Adapter) ListGroupMembers(zoneName string, groupName string) ([]*irodstypes.IRODSUser, error) {
	return adapter.filesystem.ListGroupMembers(zoneName, groupName)
}

func (adapter *Adapter) ListUserMetadata(username string, zoneName string) ([]*irodstypes.IRODSMeta, error) {
	return adapter.filesystem.ListUserMetadata(username, zoneName)
}

func (adapter *Adapter) AddUserMetadata(username string, zoneName string, attribute string, value string, unit string) error {
	return adapter.filesystem.AddUserMetadata(username, zoneName, attribute, value, unit)
}

func (adapter *Adapter) CreateUser(username string, zoneName string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	return adapter.filesystem.CreateUser(username, zoneName, userType)
}

func (adapter *Adapter) CreateUserGroup(groupName string, zoneName string) (*irodstypes.IRODSUser, error) {
	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return nil, err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	req := irodsmessage.NewIRODSMessageAdminRequest("add", "group", groupName)
	err = func() error {
		conn.Lock()
		defer conn.Unlock()
		return conn.RequestAndCheck(req, &irodsmessage.IRODSMessageAdminResponse{}, nil, conn.GetOperationTimeout())
	}()
	if err != nil {
		if irodstypes.GetIRODSErrorCode(err) == irodscommon.CAT_NO_ROWS_FOUND {
			return nil, fmt.Errorf("received create group error for group %q, zone %q: %w", groupName, zoneName, irodstypes.NewUserNotFoundError(groupName))
		}
		return nil, fmt.Errorf("received create group error for group %q, zone %q: %w", groupName, zoneName, err)
	}

	return adapter.filesystem.GetUser(groupName, zoneName, irodstypes.IRODSUserRodsGroup)
}

func (adapter *Adapter) ChangeUserPassword(username string, zoneName string, newPassword string) error {
	return adapter.filesystem.ChangeUserPassword(username, zoneName, newPassword)
}

func (adapter *Adapter) ChangeUserType(username string, zoneName string, newType irodstypes.IRODSUserType) error {
	return adapter.filesystem.ChangeUserType(username, zoneName, newType)
}

func (adapter *Adapter) RemoveUser(username string, zoneName string, userType irodstypes.IRODSUserType) error {
	return adapter.filesystem.RemoveUser(username, zoneName, userType)
}

func (adapter *Adapter) RemoveUserGroup(groupName string, zoneName string) error {
	conn, err := adapter.filesystem.GetMetadataConnection(true)
	if err != nil {
		return err
	}
	defer adapter.filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	req := irodsmessage.NewIRODSMessageAdminRequest("rm", "group", groupName)
	err = func() error {
		conn.Lock()
		defer conn.Unlock()
		return conn.RequestAndCheck(req, &irodsmessage.IRODSMessageAdminResponse{}, nil, conn.GetOperationTimeout())
	}()
	if err != nil {
		if irodstypes.GetIRODSErrorCode(err) == irodscommon.CAT_NO_ROWS_FOUND {
			return fmt.Errorf("received remove group error for group %q, zone %q: %w", groupName, zoneName, irodstypes.NewUserNotFoundError(groupName))
		}
		return fmt.Errorf("received remove group error for group %q, zone %q: %w", groupName, zoneName, err)
	}

	return nil
}

func (adapter *Adapter) AddGroupMember(groupName string, username string, zoneName string) error {
	return adapter.filesystem.AddGroupMember(groupName, username, zoneName)
}

func (adapter *Adapter) RemoveGroupMember(groupName string, username string, zoneName string) error {
	return adapter.filesystem.RemoveGroupMember(groupName, username, zoneName)
}
