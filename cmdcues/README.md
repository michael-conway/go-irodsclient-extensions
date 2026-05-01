# cmdcues

`cmdcues` is a helper package for cueing users in interface workflows with the
proper `icommand` or `gocmd` formulation for common operations.

Current focus:

- `iput`/`iget` or `gocmd put`/`gocmd get`
- storage commands (`iphymv`/`irep` and `gocmd phymove`/`gocmd replicate`)

This package provides methods that:

- build a cue list (`[]CueEntry`) for collection and data object contexts
- return command entries containing:
  - `operation`
  - `gocmd`
  - `icommand`

Behavior details:

- collection cue sets include recursive `put`, `get`, `phymove`, and `replicate`
- data object cue sets include `put`, `get`, `phymove`, and `replicate`
- `put` uses `<LOCAL_PATH>`
- `get` uses `<DESTINATION_PATH>`
- storage commands use `icommand` syntax with `-S <srcResource>` and `-R <targetResource>`
- storage entries intentionally omit `gocmd`

This package does not perform iRODS calls. It only formats documentation-style
command cues.

cmdcues is a helper package for cueing users in various interfaces of the proper icommand or gocmd formulatin
for certain operations. This implementation is focused on:

iinit or gocmd init
iput/iget or gocmd put/get

This package will provide methods for the following:

- accept a flag for type gocmd or icommand
- accept a command as init, put, or get along with relevant file paths
  - for a put, it will take the current collection absolute path and formulate a put command to that collection that will show the user how to transfer a file
  - for a get, it will take the current absolute path (it must be an irods object) and return a get command to download the file via a command tool

This api will not implement any irods calls, it simply provides documentation that will be inserted into the file or collection details 
