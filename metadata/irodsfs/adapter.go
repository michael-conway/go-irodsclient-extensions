package irodsfs

import (
	"encoding/hex"
	"strings"

	cyfs "github.com/cyverse/go-irodsclient/fs"
	irodsuri "github.com/michael-conway/go-irodsclient-extensions/irodsuri"
	"github.com/michael-conway/go-irodsclient-extensions/metadata"
)

// Adapter implements metadata.Filesystem using go-irodsclient.
type Adapter struct {
	filesystem *cyfs.FileSystem
}

var _ metadata.Filesystem = (*Adapter)(nil)

// NewAdapter returns a metadata filesystem adapter backed by go-irodsclient.
func NewAdapter(filesystem *cyfs.FileSystem) *Adapter {
	return &Adapter{filesystem: filesystem}
}

func (adapter *Adapter) Stat(irodsPath string) (*metadata.PathStat, error) {
	entry, err := adapter.filesystem.Stat(irodsPath)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	pathStat := &metadata.PathStat{
		ID:           entry.ID,
		Name:         entry.Name,
		Path:         entry.Path,
		Owner:        entry.Owner,
		Size:         entry.Size,
		DataType:     entry.DataType,
		CreateTime:   entry.CreateTime,
		ModifyTime:   entry.ModifyTime,
		AccessTime:   entry.AccessTime,
		IsCollection: entry.IsDir(),
	}
	if entry.CheckSumAlgorithm != "" {
		pathStat.ChecksumAlgorithm = string(entry.CheckSumAlgorithm)
	}
	if len(entry.CheckSum) > 0 {
		pathStat.Checksum = hex.EncodeToString(entry.CheckSum)
	}

	replicas := make([]metadata.ReplicaStat, 0, len(entry.IRODSReplicas))
	for _, replica := range entry.IRODSReplicas {
		mappedReplica := metadata.ReplicaStat{
			Number:            replica.Number,
			Owner:             replica.Owner,
			Status:            replica.Status,
			ResourceName:      replica.ResourceName,
			ResourceHierarchy: replica.ResourceHierarchy,
			Path:              replica.Path,
			CreateTime:        replica.CreateTime,
			ModifyTime:        replica.ModifyTime,
			AccessTime:        replica.AccessTime,
		}
		if replica.Checksum != nil {
			mappedReplica.ChecksumAlgorithm = string(replica.Checksum.Algorithm)
			if len(replica.Checksum.Checksum) > 0 {
				mappedReplica.Checksum = hex.EncodeToString(replica.Checksum.Checksum)
			}
		}
		replicas = append(replicas, mappedReplica)
	}
	pathStat.Replicas = replicas
	return pathStat, nil
}

func (adapter *Adapter) ListMetadata(irodsPath string) ([]metadata.AVUStat, error) {
	metadataList, err := adapter.filesystem.ListMetadata(irodsPath)
	if err != nil {
		return nil, err
	}

	result := make([]metadata.AVUStat, 0, len(metadataList))
	for _, avu := range metadataList {
		if avu == nil {
			continue
		}

		result = append(result, metadata.AVUStat{
			ID:         avu.AVUID,
			Name:       avu.Name,
			Value:      avu.Value,
			Units:      avu.Units,
			CreateTime: avu.CreateTime,
			ModifyTime: avu.ModifyTime,
		})
	}

	return result, nil
}

func (adapter *Adapter) ConnectionInfoForPath(irodsPath string) (metadata.ConnectionInfo, error) {
	account := adapter.filesystem.GetAccount()
	if account == nil {
		return metadata.ConnectionInfo{
			Zone: zoneFromPath(irodsPath),
		}, nil
	}

	zone := strings.TrimSpace(account.ClientZone)
	if zone == "" {
		zone = zoneFromPath(irodsPath)
	}
	if zone == "" {
		zone = strings.TrimSpace(account.ProxyZone)
	}

	connectionInfo := metadata.ConnectionInfo{
		Host: strings.TrimSpace(account.Host),
		Port: account.Port,
		Zone: zone,
	}

	uri, err := irodsuri.BuildForAccountWithoutUserInfo(account, irodsPath)
	if err == nil && uri != nil {
		connectionInfo.IRODSURI = uri.String()
	}

	return connectionInfo, nil
}

func zoneFromPath(irodsPath string) string {
	irodsPath = strings.TrimSpace(irodsPath)
	if !strings.HasPrefix(irodsPath, "/") {
		return ""
	}

	trimmed := strings.TrimPrefix(irodsPath, "/")
	if trimmed == "" {
		return ""
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

// ListPathAVUs returns AVUs attached to an iRODS path.
func (adapter *Adapter) ListPathAVUs(irodsPath string) ([]metadata.AVUStat, error) {
	return adapter.ListMetadata(irodsPath)
}
