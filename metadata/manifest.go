package metadata

const (
	// ManifestVersion is the current metadata-manifest format version.
	ManifestVersion = "1.0.0"

	// ManifestSchemaURI points to the JSON schema identifier for the manifest.
	ManifestSchemaURI = "https://github.com/michael-conway/go-irodsclient-extensions/metadata/manifest.schema.json"
)

// EntryType identifies whether a manifest is for a collection or data object.
type EntryType string

const (
	EntryTypeCollection EntryType = "collection"
	EntryTypeDataObject EntryType = "data_object"
)

// Manifest describes the metadata-manifest JSON output.
type Manifest struct {
	Schema      string        `json:"$schema"`
	Version     string        `json:"version"`
	GeneratedAt string        `json:"generated_at"`
	IRODSPath   string        `json:"irods_path"`
	IRODSURI    string        `json:"irods_uri,omitempty"`
	IRODSHost   string        `json:"irods_host,omitempty"`
	IRODSPort   int           `json:"irods_port,omitempty"`
	IRODSZone   string        `json:"irods_zone,omitempty"`
	EntryType   EntryType     `json:"entry_type"`
	Entry       ManifestEntry `json:"entry"`
	AVUs        []AVU         `json:"avus"`
}

// ManifestEntry contains path details included in the manifest.
type ManifestEntry struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Path              string    `json:"path"`
	Owner             string    `json:"owner"`
	Size              int64     `json:"size"`
	DisplaySize       string    `json:"display_size"`
	DataType          string    `json:"data_type,omitempty"`
	CreateTime        string    `json:"create_time,omitempty"`
	ModifyTime        string    `json:"modify_time,omitempty"`
	AccessTime        string    `json:"access_time,omitempty"`
	ChecksumAlgorithm string    `json:"checksum_algorithm,omitempty"`
	Checksum          string    `json:"checksum,omitempty"`
	Replicas          []Replica `json:"replicas,omitempty"`
}

// Replica captures replica details for data objects.
type Replica struct {
	Number            int64  `json:"number"`
	Owner             string `json:"owner,omitempty"`
	Status            string `json:"status,omitempty"`
	ResourceName      string `json:"resource_name,omitempty"`
	ResourceHierarchy string `json:"resource_hierarchy,omitempty"`
	Path              string `json:"path,omitempty"`
	CreateTime        string `json:"create_time,omitempty"`
	ModifyTime        string `json:"modify_time,omitempty"`
	AccessTime        string `json:"access_time,omitempty"`
	ChecksumAlgorithm string `json:"checksum_algorithm,omitempty"`
	Checksum          string `json:"checksum,omitempty"`
}

// AVU captures attached metadata values for a path.
type AVU struct {
	Name       string `json:"name"`
	Value      string `json:"value"`
	Units      string `json:"units,omitempty"`
	CreateTime string `json:"create_time,omitempty"`
	ModifyTime string `json:"modify_time,omitempty"`
}
