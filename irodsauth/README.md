# irodsauth

`irodsauth` converts application-level authentication decisions into
`go-irodsclient` account values.

The package does not authenticate users itself. Callers validate credentials or
tokens first, then pass the resulting request context into `CreateAccount`.

## Supported Schemes

- `basic`: create a direct account for the supplied username and password.
- `bearer`: create a proxy account for a trusted bearer-token username using
  the configured admin account as proxy.
- `bearer-ticket`: create an admin ticket account for anonymous ticket access.

## Usage

```go
import (
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/irodsauth"
)

account, err := irodsauth.CreateAccount(irodsauth.Request{
	AuthScheme:    irodsauth.AuthSchemeBasic,
	Username:      "alice",
	BasicPassword: "secret",
}, irodsauth.Config{
	Host:            "localhost",
	Port:            1247,
	Zone:            "tempZone",
	DefaultResource: "demoResc",
	RequestAuthScheme: irodstypes.AuthSchemeNative,
	AdminAuthScheme:   irodstypes.AuthSchemeNative,
})
if err != nil {
	return err
}

_ = account
```

Use `ApplyConnectionConfig` when a service needs to apply shared TLS,
negotiation, or connection policy to every generated account.
