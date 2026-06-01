# cmdcues

`cmdcues` builds copy/paste command hints for interfaces that need to show
equivalent `icommand` and `gocmd` operations.

Supported cue families:

- `iput`/`iget` or `gocmd put`/`gocmd get`
- storage commands (`iphymv`/`irep` and `gocmd phymove`/`gocmd replicate`)

The package:

- build a cue list (`[]CueEntry`) for collection and data object contexts
- returns command entries containing `operation`, `gocmd`, and `icommand`

Behavior details:

- collection cue sets include recursive `put`, `get`, `phymove`, and `replicate`
- data object cue sets include `put`, `get`, `phymove`, and `replicate`
- `put` uses `<LOCAL_PATH>`
- `get` uses `<DESTINATION_PATH>`
- storage commands use `icommand` syntax with `-S <srcResource>` and `-R <targetResource>`
- storage entries intentionally omit `gocmd`

This package does not perform iRODS calls. It only formats documentation-style
command cues.
