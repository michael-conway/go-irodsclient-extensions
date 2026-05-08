package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateManifestForDataObject(t *testing.T) {
	ts := time.Date(2026, 5, 8, 13, 14, 15, 0, time.UTC)
	fs := &testFilesystem{
		stats: map[string]PathStat{
			"/tempZone/home/test1/object.txt": {
				ID:                101,
				Name:              "object.txt",
				Path:              "/tempZone/home/test1/object.txt",
				Owner:             "test1",
				Size:              1536,
				DataType:          "generic",
				CreateTime:        ts,
				ModifyTime:        ts,
				AccessTime:        ts,
				IsCollection:      false,
				ChecksumAlgorithm: "sha2",
				Checksum:          "abcd",
				Replicas: []ReplicaStat{
					{
						Number:            0,
						Owner:             "test1",
						Status:            "1",
						ResourceName:      "demoResc",
						ResourceHierarchy: "demoResc",
						Path:              "/vault/home/test1/object.txt",
						CreateTime:        ts,
						ModifyTime:        ts,
						AccessTime:        ts,
					},
				},
			},
		},
		metadata: map[string][]AVUStat{
			"/tempZone/home/test1/object.txt": {
				{
					Name:       "iRODS:DRS:ID",
					Value:      "7649e941-97a2-4692-8364-469e07166acd",
					Units:      "iRODS:DRS",
					CreateTime: ts,
					ModifyTime: ts,
				},
			},
		},
		connectionInfo: map[string]ConnectionInfo{
			"/tempZone/home/test1/object.txt": {
				IRODSURI: "irods://icat.example.org:1247/tempZone/home/test1/object.txt",
				Host:     "icat.example.org",
				Port:     1247,
				Zone:     "tempZone",
			},
		},
	}

	service, err := NewService(fs)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	manifest, err := service.GenerateManifest("/tempZone/home/test1/object.txt")
	if err != nil {
		t.Fatalf("generate manifest: %v", err)
	}

	if manifest.Version != ManifestVersion {
		t.Fatalf("unexpected version %q", manifest.Version)
	}
	if manifest.EntryType != EntryTypeDataObject {
		t.Fatalf("expected data_object entry type, got %q", manifest.EntryType)
	}
	if manifest.Entry.DisplaySize != "1.5 KiB" {
		t.Fatalf("unexpected display size %q", manifest.Entry.DisplaySize)
	}
	if manifest.IRODSURI != "irods://icat.example.org:1247/tempZone/home/test1/object.txt" {
		t.Fatalf("unexpected irods uri %q", manifest.IRODSURI)
	}
	if manifest.IRODSHost != "icat.example.org" || manifest.IRODSPort != 1247 || manifest.IRODSZone != "tempZone" {
		t.Fatalf("unexpected connection fields host=%q port=%d zone=%q", manifest.IRODSHost, manifest.IRODSPort, manifest.IRODSZone)
	}
	if len(manifest.AVUs) != 1 || manifest.AVUs[0].Name != "iRODS:DRS:ID" {
		t.Fatalf("unexpected avus %+v", manifest.AVUs)
	}
}

func TestGenerateManifestForCollection(t *testing.T) {
	ts := time.Date(2026, 5, 8, 13, 14, 15, 0, time.UTC)
	fs := &testFilesystem{
		stats: map[string]PathStat{
			"/tempZone/home/test1/drscoll": {
				ID:           202,
				Name:         "drscoll",
				Path:         "/tempZone/home/test1/drscoll",
				Owner:        "test1",
				Size:         0,
				CreateTime:   ts,
				ModifyTime:   ts,
				AccessTime:   ts,
				IsCollection: true,
			},
		},
		metadata: map[string][]AVUStat{
			"/tempZone/home/test1/drscoll": {},
		},
		connectionInfo: map[string]ConnectionInfo{
			"/tempZone/home/test1/drscoll": {
				IRODSURI: "irods://icat.example.org:1247/tempZone/home/test1/drscoll",
				Host:     "icat.example.org",
				Port:     1247,
				Zone:     "tempZone",
			},
		},
	}

	service, err := NewService(fs)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	manifest, err := service.GenerateManifest("/tempZone/home/test1/drscoll")
	if err != nil {
		t.Fatalf("generate manifest: %v", err)
	}
	if manifest.EntryType != EntryTypeCollection {
		t.Fatalf("expected collection entry type, got %q", manifest.EntryType)
	}
	if manifest.Entry.Size != 0 || manifest.Entry.DisplaySize != "0 B" {
		t.Fatalf("unexpected collection size fields %+v", manifest.Entry)
	}
}

func TestGenerateManifestFile(t *testing.T) {
	ts := time.Date(2026, 5, 8, 13, 14, 15, 0, time.UTC)
	fs := &testFilesystem{
		stats: map[string]PathStat{
			"/tempZone/home/test1/object.txt": {
				ID:           303,
				Name:         "object.txt",
				Path:         "/tempZone/home/test1/object.txt",
				Owner:        "test1",
				Size:         10,
				CreateTime:   ts,
				ModifyTime:   ts,
				AccessTime:   ts,
				IsCollection: false,
			},
		},
		metadata: map[string][]AVUStat{
			"/tempZone/home/test1/object.txt": {},
		},
		connectionInfo: map[string]ConnectionInfo{
			"/tempZone/home/test1/object.txt": {
				IRODSURI: "irods://icat.example.org:1247/tempZone/home/test1/object.txt",
				Host:     "icat.example.org",
				Port:     1247,
				Zone:     "tempZone",
			},
		},
	}

	service, err := NewService(fs)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "manifests", "object.manifest.json")
	if err := service.GenerateManifestFile("/tempZone/home/test1/object.txt", outputPath); err != nil {
		t.Fatalf("generate manifest file: %v", err)
	}

	bytesValue, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(bytesValue, &manifest); err != nil {
		t.Fatalf("unmarshal output file: %v", err)
	}
	if manifest.Schema != ManifestSchemaURI {
		t.Fatalf("unexpected schema %q", manifest.Schema)
	}
}

type testFilesystem struct {
	stats          map[string]PathStat
	metadata       map[string][]AVUStat
	connectionInfo map[string]ConnectionInfo
}

func (fs *testFilesystem) Stat(irodsPath string) (*PathStat, error) {
	stat, ok := fs.stats[irodsPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	result := stat
	result.Replicas = append([]ReplicaStat(nil), stat.Replicas...)
	return &result, nil
}

func (fs *testFilesystem) ListMetadata(irodsPath string) ([]AVUStat, error) {
	metadata, ok := fs.metadata[irodsPath]
	if !ok {
		return []AVUStat{}, nil
	}
	return append([]AVUStat(nil), metadata...), nil
}

func (fs *testFilesystem) ConnectionInfoForPath(irodsPath string) (ConnectionInfo, error) {
	if fs.connectionInfo != nil {
		if info, ok := fs.connectionInfo[irodsPath]; ok {
			return info, nil
		}
	}
	return ConnectionInfo{}, nil
}
