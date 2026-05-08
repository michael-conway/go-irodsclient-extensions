package metadata

import "time"

// PathStat is the minimal filesystem stat shape required by the manifest service.
type PathStat struct {
	ID                int64
	Name              string
	Path              string
	Owner             string
	Size              int64
	DataType          string
	CreateTime        time.Time
	ModifyTime        time.Time
	AccessTime        time.Time
	IsCollection      bool
	ChecksumAlgorithm string
	Checksum          string
	Replicas          []ReplicaStat
}

// ReplicaStat contains replica metadata for a data object.
type ReplicaStat struct {
	Number            int64
	Owner             string
	Status            string
	ResourceName      string
	ResourceHierarchy string
	Path              string
	CreateTime        time.Time
	ModifyTime        time.Time
	AccessTime        time.Time
	ChecksumAlgorithm string
	Checksum          string
}

// AVUStat contains AVU details read from iRODS metadata.
type AVUStat struct {
	Name       string
	Value      string
	Units      string
	CreateTime time.Time
	ModifyTime time.Time
}

// ConnectionInfo contains iRODS connection and zone context for a path.
type ConnectionInfo struct {
	IRODSURI string
	Host     string
	Port     int
	Zone     string
}
