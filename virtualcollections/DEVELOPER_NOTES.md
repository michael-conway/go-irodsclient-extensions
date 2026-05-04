# Virtual Collections Package Notes

Use this file for the working rules, decisions, and open issues for
`virtualcollections/`.

## Context

This package is intended to provide lower-level support for virtual collections
in `go-irodsclient-extensions`.

Virtual collections are query/provider-backed views that return entries that
look like normal iRODS listings. Entries are always real iRODS objects and
collections identified by canonical absolute iRODS paths.

## Core Semantics

1. A virtual collection is a listing view, not a storage location.
2. Returned rows must always be resolvable to canonical `absolute_path`.
3. Virtual collections support navigation.
4. Virtual collections do not support adding/uploading directly to the virtual
   root.
5. File and collection operations are performed against target iRODS paths
   outside virtual collection semantics.

## Path-Hintable and `stay_virtual`

`stay_virtual` maps to path-hinted virtual collection execution.

- `path_hintable = true`:
  - provider accepts a path hint under the virtual collection root/scope
  - provider reapplies the virtual collection query constrained by the path hint
  - user remains in virtual collection context while navigating
- `path_hintable = false`:
  - `stay_virtual` is not supported
  - clients should either show a disabled state or switch to explicit
    `follow_path` handoff

Expected guardrails:

- reject path hints outside the allowed root/scope
- normalize and validate path hints before query execution
- keep deterministic ordering for stable paging

## Intended Lower-Level Design

Planned package responsibilities:

- typed definition model for virtual collections
- provider interface for different backing strategies
- registry of providers by type
- resolver that normalizes results into a common listing shape
- capability flags (for example, `path_hintable`)

Initial provider targets:

- `collection_path` (baseline)
- `genquery`
- `avu_query`
- `external_index` (if it resolves to canonical iRODS paths)

## Proposed API Concepts (Go Package)

Proposed concepts for implementation:

- `Definition`
  - id, name, type, enabled
  - optional scope root
  - provider-specific query/config payload
- `ResolveRequest`
  - pagination/sort inputs
  - optional `path_hint`
  - navigation mode (`stay_virtual` or `follow_path` intent from caller)
- `Result`
  - normalized list of iRODS entries
  - total/matched count metadata
  - provider/capability metadata
- `Provider` interface
  - validate definition
  - resolve listing
  - expose capabilities

## Proposed REST Paths (Future, Not Implemented Here)

This package does not implement REST. These are proposed consumer paths for
`irods-go-rest`.

- `GET /api/v1/ext/virtual-collections`
  - list available virtual collection definitions and capabilities
- `GET /api/v1/ext/virtual-collections/{id}`
  - return definition metadata and top-level links
- `GET /api/v1/ext/virtual-collections/{id}/children`
  - resolve listing
  - query inputs may include:
    - `path_hint`
    - paging/sort fields
    - optional name pattern filter
    - navigation mode hint where needed

Proposed response behavior:

- list rows always include canonical iRODS `absolute_path`
- links support virtual navigation and explicit handoff to explorer path views
- mutation links are intentionally absent from virtual listing responses

## Proposed Starbase Behavior (Future, Not Implemented Here)

Virtual collections should be rendered as a separate page/surface that is
visually similar to explorer, but functionally separate.

Recommended behavior:

1. Left navigation contains virtual collection entries.
2. Selecting a virtual collection opens Virtual Collections view.
3. The Virtual Collections view supports navigation only.
4. Row-level action to "Open in Explorer" hands off to normal path explorer.
5. All mutation operations (rename/move/copy/delete/upload/ACL/metadata) occur
   in Explorer, not in Virtual Collections view.

This separation avoids mixed semantics while still allowing users to work from
query-derived discovery into normal iRODS operations.

## Open Questions

- definition persistence location and format (in-memory, YAML, `.irodsext`, or
  service-managed)
- whether `follow_path` should be represented in this package, or only in
  consuming UI/service layers
- provider-specific query schema standardization vs opaque payloads
- caching and refresh policy for expensive providers
