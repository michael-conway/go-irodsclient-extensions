# Search Plugin Package Notes

Use this file for the working rules, decisions, and open issues for
`searchplugin/`.

## Context

This package is the shared client and registry layer for pluggable search
services that implement:

- `searchplugin/data-grid-search-api.yaml`

Primary near-term consumer:

- `starbase` (through `irods-go-rest` proxy integration)

Expected deployment model:

- search plugin services are reachable behind `irods-go-rest`
- public clients call the main API
- `irods-go-rest` may proxy to one or more plugin endpoints

## Current Plan

1. Keep this package focused on reusable registry/client behavior for search plugin implementations.
2. Avoid embedding `irods-go-rest` route semantics directly here.
3. Preserve room for multiple auth forwarding strategies.
4. Support both in-process Go callers and future REST-surface adapters.

## Implemented

- endpoint and operation models aligned to current OpenAPI routes:
  - `GET /indexes`
  - `GET /attributes/{index_name}`
  - `POST /search?index_name=...&search_query=...`
- YAML config format parser and registry load/reload flow:
  - `plugins[].name`
  - `plugins[].uri`
  - `plugins[].auth_type`
  - `plugins[].enabled`
- in-memory registry with validation and per-plugin status tracking
- invocation metadata API to describe how callers should invoke each operation
- HTTP client for list, properties, search, and federated search
- ping support that updates per-plugin health status
- pluggable request authorizer strategies, including per-endpoint auth-type routing

## Authentication Direction (Open)

Auth behavior is intentionally abstracted to a `RequestAuthorizer` hook.

Current built-in auth types:

- `none`
- `bearer_passthrough`
- `basic_passthrough`
- `static_bearer` (placeholder mode in endpoint-driven router)
- `static_basic` (placeholder mode in endpoint-driven router)
- `service_account` (placeholder mode in endpoint-driven router)

Current endpoint-auth router behavior:

- pass-through modes are active
- service-account/static modes are explicitly unsupported until credential source design is finalized

## Config Format

Current YAML shape:

```yaml
plugins:
  - name: "nih-grid-search"
    uri: "https://search.example.org/v1"
    auth_type: "bearer_passthrough"
    enabled: true
```

`name` currently serves as the stable registry id.

## Open Questions

- stable plugin identity field beyond `name` (e.g. explicit `id`)
- service-account credential source and rotation model
- static credential handling model in config (if allowed) vs secure external store
- canonical response normalization for cross-plugin aggregation
- registry-as-library only vs standalone registry/proxy REST service package

## Working Rules

- keep public APIs explicit and small
- keep transport and policy separate
- return raw response payloads alongside typed helpers for forward compatibility
- prioritize deterministic unit tests before integration wiring
