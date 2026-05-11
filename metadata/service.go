package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

var (
	ErrMissingFilesystem = errors.New("missing filesystem")
	ErrInvalidIRODSPath  = errors.New("invalid irods path")
	ErrInvalidOutputPath = errors.New("invalid output path")
	ErrPathStatMissing   = errors.New("path stat missing")
)

// Filesystem is the minimal iRODS API required by the metadata manifest service.
type Filesystem interface {
	Stat(irodsPath string) (*PathStat, error)
	ListMetadata(irodsPath string) ([]AVUStat, error)
	ConnectionInfoForPath(irodsPath string) (ConnectionInfo, error)
}

// Service generates metadata manifests for iRODS paths.
type Service struct {
	filesystem Filesystem
	template   *template.Template
}

type renderData struct {
	Schema      string
	Version     string
	GeneratedAt string
	IRODSPath   string
	IRODSURI    string
	IRODSHost   string
	IRODSPort   int
	IRODSZone   string
	EntryType   EntryType
	Entry       ManifestEntry
	AVUs        []AVU
}

var manifestTemplate = template.Must(template.New("metadata-manifest").Funcs(template.FuncMap{
	"toJSON": func(value interface{}) (string, error) {
		bytesValue, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(bytesValue), nil
	},
}).Parse(`{
  "$schema": {{ toJSON .Schema }},
  "version": {{ toJSON .Version }},
  "generated_at": {{ toJSON .GeneratedAt }},
  "irods_path": {{ toJSON .IRODSPath }},
  "irods_uri": {{ toJSON .IRODSURI }},
  "irods_host": {{ toJSON .IRODSHost }},
  "irods_port": {{ toJSON .IRODSPort }},
  "irods_zone": {{ toJSON .IRODSZone }},
  "entry_type": {{ toJSON .EntryType }},
  "entry": {{ toJSON .Entry }},
  "avus": {{ toJSON .AVUs }}
}`))

// NewService creates a metadata manifest generator service.
func NewService(filesystem Filesystem) (*Service, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	return &Service{
		filesystem: filesystem,
		template:   manifestTemplate,
	}, nil
}

// GenerateManifest builds the typed metadata manifest for an iRODS path.
func (service *Service) GenerateManifest(irodsPath string) (Manifest, error) {
	irodsPath = normalizeIRODSPath(irodsPath)
	if irodsPath == "" {
		return Manifest{}, ErrInvalidIRODSPath
	}

	stat, err := service.filesystem.Stat(irodsPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("stat %q: %w", irodsPath, err)
	}
	if stat == nil {
		return Manifest{}, fmt.Errorf("%w for %q", ErrPathStatMissing, irodsPath)
	}

	metadata, err := service.filesystem.ListMetadata(irodsPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("list metadata for %q: %w", irodsPath, err)
	}
	connectionInfo, err := service.filesystem.ConnectionInfoForPath(irodsPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve connection info for %q: %w", irodsPath, err)
	}

	entryType := EntryTypeDataObject
	if stat.IsCollection {
		entryType = EntryTypeCollection
	}

	manifest := Manifest{
		Schema:      ManifestSchemaURI,
		Version:     ManifestVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		IRODSPath:   irodsPath,
		IRODSURI:    strings.TrimSpace(connectionInfo.IRODSURI),
		IRODSHost:   strings.TrimSpace(connectionInfo.Host),
		IRODSPort:   connectionInfo.Port,
		IRODSZone:   strings.TrimSpace(connectionInfo.Zone),
		EntryType:   entryType,
		Entry:       mapManifestEntry(*stat),
		AVUs:        mapAVUs(metadata),
	}
	return manifest, nil
}

// GenerateManifestBytes renders a metadata-manifest JSON document.
func (service *Service) GenerateManifestBytes(irodsPath string) ([]byte, error) {
	manifest, err := service.GenerateManifest(irodsPath)
	if err != nil {
		return nil, err
	}

	buffer := bytes.NewBuffer(nil)
	templateData := renderData{
		Schema:      manifest.Schema,
		Version:     manifest.Version,
		GeneratedAt: manifest.GeneratedAt,
		IRODSPath:   manifest.IRODSPath,
		IRODSURI:    manifest.IRODSURI,
		IRODSHost:   manifest.IRODSHost,
		IRODSPort:   manifest.IRODSPort,
		IRODSZone:   manifest.IRODSZone,
		EntryType:   manifest.EntryType,
		Entry:       manifest.Entry,
		AVUs:        manifest.AVUs,
	}

	if err := service.template.Execute(buffer, templateData); err != nil {
		return nil, fmt.Errorf("render metadata manifest: %w", err)
	}

	formatted := bytes.NewBuffer(nil)
	if err := json.Indent(formatted, buffer.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("format metadata manifest json: %w", err)
	}
	formatted.WriteByte('\n')

	return formatted.Bytes(), nil
}

// GenerateManifestFile writes a metadata-manifest JSON file for an iRODS path.
func (service *Service) GenerateManifestFile(irodsPath string, outputPath string) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return ErrInvalidOutputPath
	}

	bytesValue, err := service.GenerateManifestBytes(irodsPath)
	if err != nil {
		return err
	}

	directory := filepath.Dir(outputPath)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create output directory %q: %w", directory, err)
		}
	}

	if err := os.WriteFile(outputPath, bytesValue, 0o644); err != nil {
		return fmt.Errorf("write metadata manifest file %q: %w", outputPath, err)
	}

	return nil
}

func mapManifestEntry(stat PathStat) ManifestEntry {
	replicas := make([]Replica, 0, len(stat.Replicas))
	for _, replica := range stat.Replicas {
		replicas = append(replicas, Replica{
			Number:            replica.Number,
			Owner:             replica.Owner,
			Status:            replica.Status,
			ResourceName:      replica.ResourceName,
			ResourceHierarchy: replica.ResourceHierarchy,
			Path:              replica.Path,
			CreateTime:        formatTime(replica.CreateTime),
			ModifyTime:        formatTime(replica.ModifyTime),
			AccessTime:        formatTime(replica.AccessTime),
			ChecksumAlgorithm: replica.ChecksumAlgorithm,
			Checksum:          replica.Checksum,
		})
	}

	return ManifestEntry{
		ID:                stat.ID,
		Name:              stat.Name,
		Path:              stat.Path,
		Owner:             stat.Owner,
		Size:              stat.Size,
		DisplaySize:       formatDisplaySize(stat.Size),
		DataType:          stat.DataType,
		CreateTime:        formatTime(stat.CreateTime),
		ModifyTime:        formatTime(stat.ModifyTime),
		AccessTime:        formatTime(stat.AccessTime),
		ChecksumAlgorithm: stat.ChecksumAlgorithm,
		Checksum:          stat.Checksum,
		Replicas:          replicas,
	}
}

func mapAVUs(metadata []AVUStat) []AVU {
	avus := make([]AVU, 0, len(metadata))
	for _, avu := range metadata {
		avus = append(avus, AVU{
			Name:       avu.Name,
			Value:      avu.Value,
			Units:      avu.Units,
			CreateTime: formatTime(avu.CreateTime),
			ModifyTime: formatTime(avu.ModifyTime),
		})
	}
	return avus
}

func normalizeIRODSPath(irodsPath string) string {
	irodsPath = strings.TrimSpace(irodsPath)
	if irodsPath == "" || !strings.HasPrefix(irodsPath, "/") {
		return ""
	}
	if irodsPath == "/" {
		return "/"
	}
	return strings.TrimRight(irodsPath, "/")
}

func formatTime(timestamp time.Time) string {
	if timestamp.IsZero() {
		return ""
	}
	return timestamp.UTC().Format(time.RFC3339)
}

func formatDisplaySize(size int64) string {
	if size < 0 {
		return "unknown"
	}
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	unitIndex := 0
	for value >= 1024 && unitIndex < len(units)-1 {
		value = value / 1024
		unitIndex++
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}
