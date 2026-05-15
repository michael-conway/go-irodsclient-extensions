package irodsfs

import (
	"errors"
	"fmt"
	"io"
	"strings"

	cyfs "github.com/cyverse/go-irodsclient/fs"
	irodslibfs "github.com/cyverse/go-irodsclient/irods/fs"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	metadataext "github.com/michael-conway/go-irodsclient-extensions/metadata"
	metadatairodsfs "github.com/michael-conway/go-irodsclient-extensions/metadata/irodsfs"
	"github.com/michael-conway/go-irodsclient-extensions/s3admin"
)

const metadataQueryPageSize = 500

// Adapter implements the s3admin filesystem interfaces using go-irodsclient.
type Adapter struct {
	filesystem      *cyfs.FileSystem
	proxyAccount    *irodstypes.IRODSAccount
	applicationName string
}

var _ s3admin.Filesystem = (*Adapter)(nil)
var _ s3admin.UserMappingFilesystem = (*Adapter)(nil)
var _ s3admin.UserKeyAsUserFilesystem = (*Adapter)(nil)

// NewAdapter returns an s3admin filesystem adapter backed by go-irodsclient.
func NewAdapter(filesystem *cyfs.FileSystem) *Adapter {
	return &Adapter{filesystem: filesystem}
}

// NewAdapterWithProxyAccount returns an adapter that can read marked user
// secret objects through an iRODS proxy connection as the marker user.
func NewAdapterWithProxyAccount(filesystem *cyfs.FileSystem, proxyAccount *irodstypes.IRODSAccount, applicationName string) *Adapter {
	adapter := &Adapter{
		filesystem:      filesystem,
		applicationName: strings.TrimSpace(applicationName),
	}
	if proxyAccount != nil {
		accountCopy := *proxyAccount
		adapter.proxyAccount = &accountCopy
	}
	return adapter
}

func (adapter *Adapter) CollectionExists(irodsPath string) (bool, error) {
	entry, err := adapter.filesystem.Stat(irodsPath)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return entry != nil && entry.IsDir(), nil
}

func (adapter *Adapter) CreateCollection(irodsPath string, recurse bool) error {
	return adapter.filesystem.MakeDir(irodsPath, recurse)
}

func (adapter *Adapter) SearchByMeta(metaName string, metaValue string) ([]s3admin.Entry, error) {
	return SearchByMeta(adapter.filesystem, metaName, metaValue)
}

func (adapter *Adapter) QueryCollectionMetadata(metaName string, metaValue string, options s3admin.CollectionMetadataQueryOptions) ([]s3admin.CollectionMetadataMatch, error) {
	scopeMode, err := collectionMetadataScopeMode(options.Scope)
	if err != nil {
		return nil, err
	}

	queryAdapter := metadatairodsfs.NewAdapter(adapter.filesystem)
	builder := metadataext.NewEntryQuery().
		Collections().
		Scope(options.IRODSPath, scopeMode).
		AVU(metaName, metaValue, metadataext.AnyUnit).
		IncludeMatchedAVUs(true).
		Limit(metadataQueryPageSize)

	matchesByPath := map[string][]s3admin.Metadata{}
	paths := []string{}
	var cursor *metadataext.EntryQueryCursor
	for {
		result, err := queryAdapter.QueryEntries(builder.Cursor(cursor).Build())
		if err != nil {
			return nil, err
		}

		for _, entry := range result.Entries {
			if entry == nil || !entry.IsDir() {
				continue
			}

			avus := convertMatchedAVUs(result.MatchedAVUs[entry.Path])
			if len(avus) == 0 {
				continue
			}
			if _, ok := matchesByPath[entry.Path]; !ok {
				paths = append(paths, entry.Path)
			}
			for _, avu := range avus {
				matchesByPath[entry.Path] = appendUniqueMetadata(matchesByPath[entry.Path], avu)
			}
		}

		if !result.Page.HasMore {
			break
		}
		if result.Page.Next == nil {
			return nil, fmt.Errorf("metadata collection query returned has_more without a cursor")
		}
		cursor = result.Page.Next
	}

	matches := make([]s3admin.CollectionMetadataMatch, 0, len(paths))
	for _, irodsPath := range paths {
		matches = append(matches, s3admin.CollectionMetadataMatch{
			IRODSPath: irodsPath,
			Metadata:  matchesByPath[irodsPath],
		})
	}
	return matches, nil
}

func (adapter *Adapter) QueryDataObjectMetadata(metaName string, metaValue string) ([]s3admin.DataObjectMetadataMatch, error) {
	queryAdapter := metadatairodsfs.NewAdapter(adapter.filesystem)
	builder := metadataext.NewEntryQuery().
		DataObjects().
		AVU(metaName, metaValue, metadataext.AnyUnit).
		IncludeMatchedAVUs(true).
		Limit(metadataQueryPageSize)

	matchesByPath := map[string][]s3admin.Metadata{}
	paths := []string{}
	var cursor *metadataext.EntryQueryCursor
	for {
		result, err := queryAdapter.QueryEntries(builder.Cursor(cursor).Build())
		if err != nil {
			return nil, err
		}

		for _, entry := range result.Entries {
			if entry == nil || entry.IsDir() {
				continue
			}

			avus := convertMatchedAVUs(result.MatchedAVUs[entry.Path])
			if len(avus) == 0 {
				continue
			}
			if _, ok := matchesByPath[entry.Path]; !ok {
				paths = append(paths, entry.Path)
			}
			for _, avu := range avus {
				matchesByPath[entry.Path] = appendUniqueMetadata(matchesByPath[entry.Path], avu)
			}
		}

		if !result.Page.HasMore {
			break
		}
		if result.Page.Next == nil {
			return nil, fmt.Errorf("metadata data object query returned has_more without a cursor")
		}
		cursor = result.Page.Next
	}

	matches := make([]s3admin.DataObjectMetadataMatch, 0, len(paths))
	for _, irodsPath := range paths {
		matches = append(matches, s3admin.DataObjectMetadataMatch{
			IRODSPath: irodsPath,
			Metadata:  matchesByPath[irodsPath],
		})
	}
	return matches, nil
}

func (adapter *Adapter) ListCollectionMetadata(collectionPath string) ([]s3admin.Metadata, error) {
	return listMetadata(adapter.filesystem, collectionPath)
}

func (adapter *Adapter) AddCollectionMetadata(collectionPath string, metadata s3admin.Metadata) error {
	return adapter.filesystem.AddMetadata(collectionPath, metadata.Name, metadata.Value, metadata.Units)
}

func (adapter *Adapter) DeleteCollectionMetadata(collectionPath string, metadata s3admin.Metadata) error {
	return deleteMetadata(adapter.filesystem, collectionPath, metadata)
}

func (adapter *Adapter) ReadDataObject(dataObjectPath string) ([]byte, error) {
	return readDataObject(adapter.filesystem, dataObjectPath)
}

func (adapter *Adapter) ReadDataObjectAsUser(dataObjectPath string, userID string) ([]byte, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || adapter.proxyAccount == nil {
		return adapter.ReadDataObject(dataObjectPath)
	}

	account := *adapter.proxyAccount
	account.ClientUser = userID
	if strings.TrimSpace(account.ClientZone) == "" {
		account.ClientZone = account.ProxyZone
	}
	account.FixAuthConfiguration()

	applicationName := adapter.applicationName
	if applicationName == "" {
		applicationName = "s3admin-irodsfs-as-user"
	}

	filesystem, err := cyfs.NewFileSystemWithDefault(&account, applicationName)
	if err != nil {
		return nil, err
	}
	defer filesystem.Release()

	return readDataObject(filesystem, dataObjectPath)
}

func readDataObject(filesystem *cyfs.FileSystem, dataObjectPath string) ([]byte, error) {
	entry, err := filesystem.Stat(dataObjectPath)
	if err != nil {
		return nil, err
	}
	if entry == nil || entry.IsDir() {
		return nil, irodstypes.NewFileNotFoundError(dataObjectPath)
	}
	if entry.Size == 0 {
		return []byte{}, nil
	}

	handle, err := filesystem.OpenFile(dataObjectPath, "", "r")
	if err != nil {
		return nil, err
	}
	defer handle.Close() //nolint

	buffer := make([]byte, entry.Size)
	read, err := handle.ReadAt(buffer, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buffer[:read], nil
}

func (adapter *Adapter) WriteDataObject(dataObjectPath string, contents []byte) error {
	if entry, err := adapter.filesystem.Stat(dataObjectPath); err == nil {
		if entry == nil || entry.IsDir() {
			return irodstypes.NewFileNotFoundError(dataObjectPath)
		}
		if err := adapter.filesystem.RemoveFile(dataObjectPath, true); err != nil {
			return err
		}
	} else if !isNotFound(err) {
		return err
	}

	handle, err := adapter.filesystem.CreateFile(dataObjectPath, "", "w")
	if err != nil {
		return err
	}
	if len(contents) > 0 {
		if _, err := handle.Write(contents); err != nil {
			handle.Close() //nolint
			return err
		}
	}
	return handle.Close()
}

func (adapter *Adapter) DeleteDataObject(dataObjectPath string, force bool) error {
	return adapter.filesystem.RemoveFile(dataObjectPath, force)
}

func (adapter *Adapter) ListDataObjectMetadata(dataObjectPath string) ([]s3admin.Metadata, error) {
	return listMetadata(adapter.filesystem, dataObjectPath)
}

func (adapter *Adapter) AddDataObjectMetadata(dataObjectPath string, metadata s3admin.Metadata) error {
	return adapter.filesystem.AddMetadata(dataObjectPath, metadata.Name, metadata.Value, metadata.Units)
}

func (adapter *Adapter) DeleteDataObjectMetadata(dataObjectPath string, metadata s3admin.Metadata) error {
	return deleteMetadata(adapter.filesystem, dataObjectPath, metadata)
}

func collectionMetadataScopeMode(scope s3admin.CollectionMetadataQueryScope) (metadataext.EntryQueryScopeMode, error) {
	switch scope {
	case s3admin.CollectionMetadataQueryScopeSelf:
		return metadataext.EntryQueryScopeSelf, nil
	case s3admin.CollectionMetadataQueryScopeChildren:
		return metadataext.EntryQueryScopeChildren, nil
	case s3admin.CollectionMetadataQueryScopeDescendants:
		return metadataext.EntryQueryScopeDescendants, nil
	default:
		return "", fmt.Errorf("unsupported collection metadata query scope %q", scope)
	}
}

func convertMatchedAVUs(avus []metadataext.AVUStat) []s3admin.Metadata {
	result := make([]s3admin.Metadata, 0, len(avus))
	for _, avu := range avus {
		result = append(result, s3admin.Metadata{
			Name:  avu.Name,
			Value: avu.Value,
			Units: avu.Units,
		})
	}
	return result
}

func appendUniqueMetadata(existing []s3admin.Metadata, avu s3admin.Metadata) []s3admin.Metadata {
	for _, candidate := range existing {
		if candidate == avu {
			return existing
		}
	}
	return append(existing, avu)
}

// SearchByMeta searches iRODS collections and data objects by AVU metadata.
func SearchByMeta(filesystem *cyfs.FileSystem, metaName string, metaValue string) ([]s3admin.Entry, error) {
	if strings.ContainsAny(metaValue, "%_") {
		return searchByMetaWildcard(filesystem, metaName, metaValue)
	}

	entries, err := filesystem.SearchByMeta(metaName, metaValue)
	if err != nil {
		return nil, err
	}

	result := make([]s3admin.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}

		entryType := s3admin.EntryTypeFile
		if entry.IsDir() {
			entryType = s3admin.EntryTypeDirectory
		}
		result = append(result, s3admin.Entry{
			Path: entry.Path,
			Type: entryType,
		})
	}
	return result, nil
}

func searchByMetaWildcard(filesystem *cyfs.FileSystem, metaName string, metaValue string) ([]s3admin.Entry, error) {
	conn, err := filesystem.GetMetadataConnection(true)
	if err != nil {
		return nil, err
	}
	defer filesystem.ReturnMetadataConnection(conn) //nolint:errcheck

	collections, err := irodslibfs.SearchCollectionsByMetaWildcard(conn, metaName, metaValue)
	if err != nil {
		return nil, err
	}

	dataObjects, err := irodslibfs.SearchDataObjectsMasterReplicaByMetaWildcard(conn, metaName, metaValue)
	if err != nil {
		return nil, err
	}

	entries := make([]s3admin.Entry, 0, len(collections)+len(dataObjects))
	for _, collection := range collections {
		if collection == nil {
			continue
		}
		entries = append(entries, s3admin.Entry{
			Path: collection.Path,
			Type: s3admin.EntryTypeCollection,
		})
	}
	for _, dataObject := range dataObjects {
		if dataObject == nil {
			continue
		}
		entries = append(entries, s3admin.Entry{
			Path: dataObject.Path,
			Type: s3admin.EntryTypeDataObject,
		})
	}
	return entries, nil
}

func listMetadata(filesystem *cyfs.FileSystem, irodsPath string) ([]s3admin.Metadata, error) {
	metadata, err := filesystem.ListMetadata(irodsPath)
	if err != nil {
		return nil, err
	}

	result := make([]s3admin.Metadata, 0, len(metadata))
	for _, avu := range metadata {
		if avu == nil {
			continue
		}
		result = append(result, s3admin.Metadata{
			Name:  avu.Name,
			Value: avu.Value,
			Units: avu.Units,
		})
	}
	return result, nil
}

func deleteMetadata(filesystem *cyfs.FileSystem, irodsPath string, metadata s3admin.Metadata) error {
	metadataList, err := filesystem.ListMetadata(irodsPath)
	if err != nil {
		return err
	}

	for _, avu := range metadataList {
		if avu == nil {
			continue
		}
		if avu.Name == metadata.Name && avu.Value == metadata.Value && avu.Units == metadata.Units {
			return filesystem.DeleteMetadata(irodsPath, avu.AVUID)
		}
	}
	return irodstypes.NewFileNotFoundError(irodsPath)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
