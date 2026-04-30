# Developer Notes

`cmdcues` is intentionally lightweight.

Scope:

- build user-facing cue structures containing both `gocmd` and `icommand` strings
- support `put` and `get`

Non-scope:

- no iRODS API calls
- no command execution
- no filesystem checks

Current API direction:

- strongly typed `Operation`
- one generic builder plus convenience helpers
- strict path validation only for absolute iRODS path expectations
- `put` cues render a local path placeholder (`<LOCAL_PATH>`) for copy/paste guidance
- `get` cues render a destination placeholder (`<DESTINATION_PATH>`) for copy/paste guidance
