module github.com/michael-conway/go-irodsclient-extensions

go 1.24.9

require (
	github.com/cyverse/go-irodsclient v0.0.0
	github.com/rs/xid v1.3.0
)

require (
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
)

require (
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/cyverse/go-irodsclient => ../go-irodsclient
