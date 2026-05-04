# virtualcollections

`virtualcollections` is a planned extension package for building and resolving
virtual iRODS collection views.

A virtual collection is a listing assembled by query or provider logic, but all
returned entries are canonical iRODS collections or data objects.

## Core Rule

- virtual collection results must map to real iRODS paths
- virtual collections are navigation surfaces, not storage/mutation surfaces

## Design Scope

- define and validate virtual collection definitions
- provide provider interfaces for query-backed listing sources
- resolve listings in a shape that can be rendered like `/path/children`
- support path-hinted navigation for `stay_virtual` behavior

Out of scope for initial package work:

- direct REST endpoint implementation
- UI implementation
- mutation operations on virtual collection roots

See [DEVELOPER_NOTES.md](./DEVELOPER_NOTES.md) for design details and
proposed REST and Starbase integration behavior.
