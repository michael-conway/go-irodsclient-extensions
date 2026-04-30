package cmdcues

import (
	"fmt"
	"strings"
)

type Operation string

const (
	OperationPut Operation = "put"
	OperationGet Operation = "get"
)

type Cue struct {
	Operation Operation `json:"operation"`
	GoCmd     string    `json:"gocmd"`
	ICommand  string    `json:"icommand"`
}

type CueRequest struct {
	Operation           Operation
	CollectionIRODSPath string
	ObjectIRODSPath     string
}

func BuildCue(request CueRequest) (Cue, error) {
	switch request.Operation {
	case OperationPut:
		return BuildPutCue(request.CollectionIRODSPath)
	case OperationGet:
		return BuildGetCue(request.ObjectIRODSPath)
	default:
		return Cue{}, fmt.Errorf("unsupported operation %q", request.Operation)
	}
}

func BuildPutCue(collectionIRODSPath string) (Cue, error) {
	collectionIRODSPath = strings.TrimSpace(collectionIRODSPath)

	if !isAbsoluteIRODSPath(collectionIRODSPath) {
		return Cue{}, fmt.Errorf("collection iRODS path must be absolute")
	}

	return Cue{
		Operation: OperationPut,
		GoCmd:     fmt.Sprintf("gocmd put %s %s", "<LOCAL_PATH>", quote(collectionIRODSPath)),
		ICommand:  fmt.Sprintf("iput %s %s", "<LOCAL_PATH>", quote(collectionIRODSPath)),
	}, nil
}

func BuildGetCue(objectIRODSPath string) (Cue, error) {
	objectIRODSPath = strings.TrimSpace(objectIRODSPath)

	if !isAbsoluteIRODSPath(objectIRODSPath) {
		return Cue{}, fmt.Errorf("object iRODS path must be absolute")
	}
	if strings.HasSuffix(objectIRODSPath, "/") {
		return Cue{}, fmt.Errorf("object iRODS path must reference a data object, not a collection")
	}

	return Cue{
		Operation: OperationGet,
		GoCmd:     fmt.Sprintf("gocmd get %s %s", quote(objectIRODSPath), "<DESTINATION_PATH>"),
		ICommand:  fmt.Sprintf("iget %s %s", quote(objectIRODSPath), "<DESTINATION_PATH>"),
	}, nil
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
