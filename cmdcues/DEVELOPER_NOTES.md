# Developer Notes

`cmdcues` is intentionally lightweight.

Scope:

- build user-facing cue lists containing both `gocmd` and `icommand` strings
- support transfer (`put`/`get`) and storage (`phymove`/`replicate`) guidance

Non-scope:

- no iRODS API calls
- no command execution
- no filesystem checks

Current API direction:

- strongly typed `Operation`
- `Cue` is a slice of `CueEntry`
- builders produce consolidated command lists for collections and data objects
- strict path validation only for absolute iRODS path expectations
- `put` cues render a local path placeholder (`<LOCAL_PATH>`) for copy/paste guidance
- `get` cues render a destination placeholder (`<DESTINATION_PATH>`) for copy/paste guidance
- storage cues render `-S <srcResource>` and `-R <targetResource>` placeholders
- storage cues intentionally provide `icommand` only (no `gocmd` entry)
