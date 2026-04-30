// Package irodsuri provides helpers for parsing and building irods:// URIs.
//
// The package is intended to centralize iRODS URI handling shared by higher-
// level clients. It supports:
//
// - parsing irods:// URIs into host, port, path, and optional user information
// - building irods:// URIs from discrete fields
// - converting between go-irodsclient IRODSAccount values and irods:// URIs
//
// The canonical user-info form used by this package is:
//
//	username#zone[:password]
//
// so a full URI may look like:
//
//	irods://rods%23tempZone:secret@icat.example.org:1247/tempZone/home/rods/file.txt
//
// When ticket-based access needs to be represented, this package uses an
// explicit query parameter instead of overloading the password position:
//
//	irods://rods%23tempZone@icat.example.org:1247/tempZone/home/rods/file.txt?ticket=ticket_abc123
//
// Use this package when a client needs a stable serialized form for iRODS
// connection and logical-path information, or when translating between iRODS
// accounts and externally visible irods:// URIs.
package irodsuri
