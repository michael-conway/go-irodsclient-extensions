// Package s3admin manages iRODS S3 API bucket mappings backed by collection AVUs.
//
// A managed bucket is represented by an AVU on the bucket root collection where
// the attribute is "iRODS:S3:Bucket" and the value is the S3 bucket name. The
// service treats those AVUs as the source of truth and rewrites the iRODS S3 API
// local-file bucket mapping JSON from discovered bucket AVUs after each
// successful add, update, or delete.
package s3admin
