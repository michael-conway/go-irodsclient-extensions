package cmdcues

import (
	"fmt"
	"strings"
)

type Operation string

const (
	OperationPut       Operation = "put"
	OperationGet       Operation = "get"
	OperationPhyMove   Operation = "phymove"
	OperationReplicate Operation = "replicate"
)

type CueEntry struct {
	Operation Operation `json:"operation"`
	GoCmd     string    `json:"gocmd"`
	ICommand  string    `json:"icommand"`
}

// Cue is a list of user-facing command cues.
type Cue []CueEntry

type CueRequest struct {
	OperationSet        string
	CollectionIRODSPath string
	ObjectIRODSPath     string
	IsCollection        bool
}

func BuildCue(request CueRequest) (Cue, error) {
	switch strings.TrimSpace(strings.ToLower(request.OperationSet)) {
	case "put/get":
		if request.IsCollection {
			return buildCollectionPutGetCues(request.CollectionIRODSPath)
		}
		return buildDataObjectPutGetCues(request.ObjectIRODSPath)
	case "storage":
		if request.IsCollection {
			return buildCollectionStorageCues(request.CollectionIRODSPath)
		}
		return buildDataObjectStorageCues(request.ObjectIRODSPath)
	case "":
		if request.IsCollection {
			return BuildCollectionCues(request.CollectionIRODSPath)
		}
		return BuildDataObjectCues(request.ObjectIRODSPath)
	default:
		return nil, fmt.Errorf("unsupported operation set %q", request.OperationSet)
	}
}

func BuildCollectionCues(collectionIRODSPath string) (Cue, error) {
	putGet, err := buildCollectionPutGetCues(collectionIRODSPath)
	if err != nil {
		return nil, err
	}

	storage, err := buildCollectionStorageCues(collectionIRODSPath)
	if err != nil {
		return nil, err
	}

	return append(putGet, storage...), nil
}

func BuildDataObjectCues(objectIRODSPath string) (Cue, error) {
	putGet, err := buildDataObjectPutGetCues(objectIRODSPath)
	if err != nil {
		return nil, err
	}

	storage, err := buildDataObjectStorageCues(objectIRODSPath)
	if err != nil {
		return nil, err
	}

	return append(putGet, storage...), nil
}

func BuildPutCue(collectionIRODSPath string) (CueEntry, error) {
	collectionIRODSPath = strings.TrimSpace(collectionIRODSPath)

	if !isAbsoluteIRODSPath(collectionIRODSPath) {
		return CueEntry{}, fmt.Errorf("collection iRODS path must be absolute")
	}

	return CueEntry{
		Operation: OperationPut,
		GoCmd:     fmt.Sprintf("gocmd put %s %s", "<LOCAL_PATH>", quote(collectionIRODSPath)),
		ICommand:  fmt.Sprintf("iput %s %s", "<LOCAL_PATH>", quote(collectionIRODSPath)),
	}, nil
}

func BuildGetCue(objectIRODSPath string) (CueEntry, error) {
	objectIRODSPath = strings.TrimSpace(objectIRODSPath)

	if !isAbsoluteIRODSPath(objectIRODSPath) {
		return CueEntry{}, fmt.Errorf("object iRODS path must be absolute")
	}
	if strings.HasSuffix(objectIRODSPath, "/") {
		return CueEntry{}, fmt.Errorf("object iRODS path must reference a data object, not a collection")
	}

	return CueEntry{
		Operation: OperationGet,
		GoCmd:     fmt.Sprintf("gocmd get %s %s", quote(objectIRODSPath), "<DESTINATION_PATH>"),
		ICommand:  fmt.Sprintf("iget %s %s", quote(objectIRODSPath), "<DESTINATION_PATH>"),
	}, nil
}

func buildCollectionPutGetCues(collectionIRODSPath string) (Cue, error) {
	collectionIRODSPath = strings.TrimSpace(collectionIRODSPath)
	if !isAbsoluteIRODSPath(collectionIRODSPath) {
		return nil, fmt.Errorf("collection iRODS path must be absolute")
	}

	return Cue{
		{
			Operation: OperationPut,
			GoCmd:     fmt.Sprintf("gocmd put -r %s %s", "<LOCAL_PATH>", quote(collectionIRODSPath)),
			ICommand:  fmt.Sprintf("iput -r %s %s", "<LOCAL_PATH>", quote(collectionIRODSPath)),
		},
		{
			Operation: OperationGet,
			GoCmd:     fmt.Sprintf("gocmd get -r %s %s", quote(collectionIRODSPath), "<DESTINATION_PATH>"),
			ICommand:  fmt.Sprintf("iget -r %s %s", quote(collectionIRODSPath), "<DESTINATION_PATH>"),
		},
	}, nil
}

func buildDataObjectPutGetCues(objectIRODSPath string) (Cue, error) {
	putCue, err := BuildPutCue(objectParentPath(objectIRODSPath))
	if err != nil {
		return nil, err
	}
	getCue, err := BuildGetCue(objectIRODSPath)
	if err != nil {
		return nil, err
	}

	return Cue{putCue, getCue}, nil
}

func buildCollectionStorageCues(collectionIRODSPath string) (Cue, error) {
	collectionIRODSPath = strings.TrimSpace(collectionIRODSPath)
	if !isAbsoluteIRODSPath(collectionIRODSPath) {
		return nil, fmt.Errorf("collection iRODS path must be absolute")
	}

	return Cue{
		{
			Operation: OperationPhyMove,
			ICommand:  fmt.Sprintf("iphymv -r -S %s -R %s %s", "<srcResource>", "<targetResource>", quote(collectionIRODSPath)),
		},
		{
			Operation: OperationReplicate,
			ICommand:  fmt.Sprintf("irepl -r -S %s -R %s %s", "<srcResource>", "<targetResource>", quote(collectionIRODSPath)),
		},
	}, nil
}

func buildDataObjectStorageCues(objectIRODSPath string) (Cue, error) {
	objectIRODSPath = strings.TrimSpace(objectIRODSPath)
	if !isAbsoluteIRODSPath(objectIRODSPath) {
		return nil, fmt.Errorf("object iRODS path must be absolute")
	}
	if strings.HasSuffix(objectIRODSPath, "/") {
		return nil, fmt.Errorf("object iRODS path must reference a data object, not a collection")
	}

	return Cue{
		{
			Operation: OperationPhyMove,
			ICommand:  fmt.Sprintf("iphymv -S %s -R %s %s", "<srcResource>", "<targetResource>", quote(objectIRODSPath)),
		},
		{
			Operation: OperationReplicate,
			ICommand:  fmt.Sprintf("irepl -S %s -R %s %s", "<srcResource>", "<targetResource>", quote(objectIRODSPath)),
		},
	}, nil
}

func objectParentPath(objectIRODSPath string) string {
	objectIRODSPath = strings.TrimSpace(objectIRODSPath)
	if objectIRODSPath == "" {
		return objectIRODSPath
	}
	lastSlash := strings.LastIndex(objectIRODSPath, "/")
	if lastSlash <= 0 {
		return "/"
	}
	return objectIRODSPath[:lastSlash]
}

func isAbsoluteIRODSPath(path string) bool {
	return strings.HasPrefix(strings.TrimSpace(path), "/")
}

func quote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	escaped := strings.ReplaceAll(value, `'`, `'"'"'`)
	return "'" + escaped + "'"
}
