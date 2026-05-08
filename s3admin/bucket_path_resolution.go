package s3admin

import (
	"fmt"
	"path"
	"strings"
)

// BucketMetadataFilesystem is the minimal metadata API required to resolve
// bucket metadata for a collection path.
type BucketMetadataFilesystem interface {
	ListCollectionMetadata(collectionPath string) ([]Metadata, error)
}

// BucketObjectResolution describes which bucket contains an iRODS data object
// and what its object key is within that bucket.
type BucketObjectResolution struct {
	Bucket               Bucket
	BucketCollectionPath string
	RelativeObjectPath   string
	CandidateBucketCount int
}

// BucketsForPath returns bucket AVU mappings for a collection path.
func BucketsForPath(filesystem BucketMetadataFilesystem, irodsPath string) ([]Bucket, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	irodsPath = normalizeIRODSPath(irodsPath)
	if irodsPath == "" {
		return nil, ErrInvalidIRODSPath
	}

	metadata, err := filesystem.ListCollectionMetadata(irodsPath)
	if err != nil {
		return nil, fmt.Errorf("list bucket metadata for %q: %w", irodsPath, err)
	}

	buckets := make([]Bucket, 0, len(metadata))
	for _, avu := range metadata {
		if !strings.EqualFold(avu.Name, AVUBucketAttribute) {
			continue
		}

		bucketID := strings.TrimSpace(avu.Value)
		if bucketID == "" {
			continue
		}
		// Prefer canonical s3admin normalization when possible, but do not
		// reject non-canonical IDs for read-only resolution use-cases.
		if normalized := normalizeBucketName(bucketID); normalized != "" {
			bucketID = normalized
		}

		buckets = append(buckets, Bucket{
			Name:      bucketID,
			IRODSPath: irodsPath,
		})
	}

	if len(buckets) == 0 {
		return nil, nil
	}

	buckets = sortBuckets(deduplicateBuckets(buckets))
	return buckets, nil
}

// ResolveBucketForDataObjectPath walks upward from the data object's parent
// collection and returns the first ancestor collection that is marked as an S3
// bucket, along with the relative object key within that bucket.
func ResolveBucketForDataObjectPath(filesystem BucketMetadataFilesystem, dataObjectPath string) (BucketObjectResolution, error) {
	if filesystem == nil {
		return BucketObjectResolution{}, ErrMissingFilesystem
	}

	dataObjectPath = normalizeIRODSPath(dataObjectPath)
	if dataObjectPath == "" || dataObjectPath == "/" {
		return BucketObjectResolution{}, ErrInvalidIRODSPath
	}

	current := normalizeIRODSPath(path.Dir(dataObjectPath))
	for current != "" && current != "." && current != "/" {
		buckets, err := BucketsForPath(filesystem, current)
		if err != nil {
			return BucketObjectResolution{}, err
		}
		if len(buckets) > 0 {
			relative := strings.TrimPrefix(dataObjectPath, current+"/")
			relative = strings.TrimPrefix(relative, "/")
			if relative == "" {
				return BucketObjectResolution{}, ErrInvalidIRODSPath
			}

			return BucketObjectResolution{
				Bucket:               buckets[0],
				BucketCollectionPath: current,
				RelativeObjectPath:   relative,
				CandidateBucketCount: len(buckets),
			}, nil
		}

		parent := normalizeIRODSPath(path.Dir(current))
		if parent == current {
			break
		}
		current = parent
	}

	// Check zone root as last candidate.
	if current == "/" {
		buckets, err := BucketsForPath(filesystem, current)
		if err != nil {
			return BucketObjectResolution{}, err
		}
		if len(buckets) > 0 {
			relative := strings.TrimPrefix(dataObjectPath, "/")
			if relative == "" {
				return BucketObjectResolution{}, ErrInvalidIRODSPath
			}
			return BucketObjectResolution{
				Bucket:               buckets[0],
				BucketCollectionPath: "/",
				RelativeObjectPath:   relative,
				CandidateBucketCount: len(buckets),
			}, nil
		}
	}

	return BucketObjectResolution{}, ErrBucketNotFound
}
