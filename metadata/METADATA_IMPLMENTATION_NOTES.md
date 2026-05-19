# Metadata File Listing Query Implementation Notes

These notes capture the current development plan for AVU-backed file listing
queries. This is intentionally narrower than the earlier virtual collections
discussion.

## Current Direction

The shared contract is a file listing query contract:

1. A query produces a listing of real iRODS collections and data objects.
2. Results use the common path-entry shape already used by explorer/listing
   views.
3. Rendering of file listing query results should be uniform across query
   types.
4. Query results are discovery views, not storage locations.
5. Users can jump from a query result into the normal Explorer context for the
   real target path.

The term "virtual collection" may still be useful later, but it is not the
primary implementation concept for the current work.

## Layer Responsibilities

### go-irodsclient-extensions

`go-irodsclient-extensions` owns the reusable iRODS service behavior.

For metadata search this means:

- define and validate `EntryQuery` and `EntryQueryDefinition`
- execute AVU/file listing queries against iRODS
- return unified `fs.Entry` results
- support paging through `EntryQueryCursor`
- serialize and deserialize query definitions
- persist saved user queries through `SavedEntryQueryService`
- keep saved query storage user-scoped through `userpersist`
- keep runtime cursor/page state out of persisted query definitions

This layer should not own REST route naming or Starbase UI behavior.

### irods-go-rest

`irods-go-rest` owns HTTP semantics and marshaling.

For metadata search this means:

- keep `/api/v1/path/query` as the execution route for file listing queries
- keep `/api/v1/ext/metadata-queries` as the route family for saved metadata
  queries
- translate REST request payloads and query parameters into
  `go-irodsclient-extensions/metadata` structs
- translate metadata service results into REST response documents and links
- map service errors to HTTP status codes
- document the request and response shapes in OpenAPI

REST should not duplicate query execution, path scoping, cursor handling, or
saved query persistence logic from `go-irodsclient-extensions`.

### Starbase

Starbase owns the user-facing search and listing experience.

For AVU query search this means:

- provide a high-level AVU search page
- render results through a shared file listing query results component
- allow a result row click to move the user into Explorer
- support an "open new page for selections" option so the query results page can
  remain open while selected results are opened in Explorer
- surface saved queries in the left navigation under a divider beneath saved
  favorites
- allow creating and deleting saved AVU queries through the REST saved-query
  routes

## Starbase AVU Search Page

The AVU search page should provide these controls:

- parent collection scope picker using the same finder pattern used by move
  operations
- search scope selector, initially including `children`, `descendants`, and
  other backend-supported scope modes
- kind selector for `data_object`, `collection`, or both
- JSON text area for the query `conditions` portion of the request
- query name, description, scope, and kind/type are separate UI controls, not
  part of the JSON editor
- base example shown in the JSON editor
- Save button
- Save As button
- Revert button
- saved query list
- editable saved query display name and description fields

The first UI should use the JSON conditions editor directly. The JSON editor
accepts only the `conditions` array; it should not accept or expose the full
`EntryQueryDefinition` payload. A graphical query builder can be added later
without changing the backend query contract.

Example editable conditions fragment:

```json
[
  {"field": "avu.attrib", "op": "=", "value": "project"},
  {"field": "avu.value", "op": "like", "value": "frog*"}
]
```

The page composes the full `/api/v1/path/query` request from:

- selected parent collection as `irods_path` or `scope.root`
- selected search scope
- selected kinds
- edited conditions JSON
- paging options
- optional matched AVU inclusion

Result order should follow backend GenQuery defaults within each query branch.
Unified results present collection matches first and data object matches second.
No additional cross-branch sort should be promised while paging remains cursor
based and branch-aware.

## Result Navigation Semantics

Query results are a finder/listing surface.

Default behavior:

- collection result click opens Explorer positioned at that collection
- data object result click opens the data object details page
- if "open new page for selections" is enabled, the Explorer/details view opens
  in a new page while the search result page remains available

Direct operations from query results are intentionally limited:

- data object rows may offer direct download
- all other actions open the target in Explorer for further action
- Explorer handoff respects the "open new page for selections" setting
- matched AVUs are hidden by default and shown through row detail expansion

Mutation semantics remain anchored to the real iRODS path and should not imply
that the query result view is a writable container.

## Saved Query Semantics

Saved queries are exposed through:

```text
/api/v1/ext/metadata-queries
/api/v1/ext/metadata-queries/{query_id}
```

For now, a saved query retains the parent collection context path selected at
design time. That path is stored in the persisted `EntryQueryDefinition` scope.

Saved query behavior:

- list saved queries
- read a saved query
- create a saved query
- update a saved query by fully overwriting the saved definition
- delete a saved query
- duplicate a saved query through a Save As flow that creates a new query ID
- execute a saved query by sending its query definition through
  `/api/v1/path/query`

Saved query identity is the opaque `query_id`, not the display name.
`/api/v1/ext/metadata-queries/{query_id}` addresses an immutable saved query ID.
Saving an existing query fully overwrites the saved definition and display
fields, but preserves the existing query ID. Save As creates a new saved query
with a new query ID. The JSON `name` field is only a user-facing display label;
it does not need to be unique. New saved queries default to the display name
`New Query` and a blank description. Users may change the display name and
description at any time. Saved queries support an optional description, and
Starbase should expose that description in the saved query UI.

Saved query definitions should not persist page tokens, cursors, selected UI
rows, or transient navigation state.

## REST Contract Direction

The immediate REST contract should stay centered on existing routes:

```text
POST /api/v1/path/query
GET  /api/v1/ext/metadata-queries
POST /api/v1/ext/metadata-queries
GET  /api/v1/ext/metadata-queries/{query_id}
PUT  /api/v1/ext/metadata-queries/{query_id}
DELETE /api/v1/ext/metadata-queries/{query_id}
```

Virtual collection routes should not be added until the design proves that the
extra abstraction is needed.

## Implementation Plan

1. Stabilize the file listing query contract in
   `go-irodsclient-extensions/metadata`.
2. Keep AVU query execution and saved query persistence in
   `go-irodsclient-extensions`.
3. Keep `irods-go-rest` focused on route marshaling, OpenAPI, response links,
   and error translation.
4. Build Starbase AVU search around `/api/v1/path/query`.
5. Surface saved metadata queries in Starbase left navigation under saved
   favorites.
6. Use the shared result listing component for AVU query results and later file
   listing query types.
7. Revisit virtual collection terminology only if navigation, persistence, or
   provider abstraction needs exceed the simpler file listing query contract.

## Open Questions

- None at this checkpoint.
