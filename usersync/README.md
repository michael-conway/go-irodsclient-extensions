# usersync

`usersync` provides desired-state helpers for iRODS users, groups, and group
membership. It is intended for sync controllers, REST services, and
administrative tools that need idempotent create/update/delete behavior without
reimplementing iRODS catalog edge-case handling.

The package keeps transport and authorization policy out of the shared layer.
Callers supply a small filesystem interface, decide whether an operation should
run, and map returned sentinel errors to their own API surface.

## Reconcile Operations

- `EnsureUser`
- `UpdateUser`
- `EnsureUserAbsent`
- `EnsureGroup`
- `EnsureGroupAbsent`
- `EnsureGroupMember`
- `EnsureGroupMemberAbsent`

Each operation returns an `Outcome` value such as `created`, `updated`,
`already_exists`, `already_absent`, or `already_member`.

Use `usersync/irodsfs` when the backing filesystem is `go-irodsclient/fs`.

## Sync Policy

The default policy is intentionally narrower than iRODS catalog authority. The
caller supplies an already-authorized iRODS filesystem, but `usersync` still
refuses to use sync workflows for iRODS administrative privilege management.

- sync may create and manage only `rodsuser` user principals
- `groupadmin` and `rodsadmin` users are protected
- groups are valid sync targets as `rodsgroup`
- created users and groups are marked with `iRODS:USER_SYNCH:MANAGED=true`
- existing users/groups are not claimed unless `WithClaimExisting(true)` is set
- delete-class operations require managed AVUs unless explicitly relaxed by
  policy options

Managed AVU attributes use the `iRODS:USER_SYNCH:<field>` family. Current
constants include:

- `iRODS:USER_SYNCH:MANAGED`
- `iRODS:USER_SYNCH:SOURCE`
- `iRODS:USER_SYNCH:REALM`
- `iRODS:USER_SYNCH:EXTERNAL_ID`
- `iRODS:USER_SYNCH:LAST_SYNC_AT`
- `iRODS:USER_SYNCH:LAST_PLAN_ID`
