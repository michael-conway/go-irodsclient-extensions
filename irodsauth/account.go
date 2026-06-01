package irodsauth

import (
	"fmt"
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

const (
	AuthSchemeBasic        = "basic"
	AuthSchemeBearer       = "bearer"
	AuthSchemeBearerTicket = "bearer-ticket"
)

type Request struct {
	AuthScheme    string
	Username      string
	BasicPassword string
	Ticket        string
}

type Config struct {
	Host                  string
	Port                  int
	Zone                  string
	DefaultResource       string
	AdminUser             string
	AdminPassword         string
	RequestAuthScheme     irodstypes.AuthScheme
	AdminAuthScheme       irodstypes.AuthScheme
	ApplyConnectionConfig func(*irodstypes.IRODSAccount) *irodstypes.IRODSAccount
}

func CreateAccount(request Request, config Config) (*irodstypes.IRODSAccount, error) {
	authScheme := strings.TrimSpace(request.AuthScheme)
	username := strings.TrimSpace(request.Username)
	ticket := strings.TrimSpace(request.Ticket)

	var account *irodstypes.IRODSAccount
	var err error

	switch authScheme {
	case AuthSchemeBasic:
		if username == "" {
			return nil, fmt.Errorf("missing username for basic auth")
		}
		account, err = irodstypes.CreateIRODSAccount(
			config.Host,
			config.Port,
			username,
			config.Zone,
			config.RequestAuthScheme,
			request.BasicPassword,
			config.DefaultResource,
		)
		if err != nil {
			return nil, fmt.Errorf("create iRODS account: %w", err)
		}
	case AuthSchemeBearerTicket:
		if ticket == "" {
			return nil, fmt.Errorf("missing iRODS ticket")
		}
		account, err = irodstypes.CreateIRODSAccountForTicket(
			config.Host,
			config.Port,
			config.AdminUser,
			config.Zone,
			config.AdminAuthScheme,
			config.AdminPassword,
			ticket,
			config.DefaultResource,
		)
		if err != nil {
			return nil, fmt.Errorf("create iRODS ticket account: %w", err)
		}
	case AuthSchemeBearer:
		if username == "" {
			return nil, fmt.Errorf("missing trusted bearer username")
		}
		account, err = irodstypes.CreateIRODSProxyAccount(
			config.Host,
			config.Port,
			username,
			config.Zone,
			config.AdminUser,
			config.Zone,
			config.AdminAuthScheme,
			config.AdminPassword,
			config.DefaultResource,
		)
		if err != nil {
			return nil, fmt.Errorf("create iRODS proxy account: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported auth scheme %q", request.AuthScheme)
	}

	if config.ApplyConnectionConfig == nil {
		return account, nil
	}

	applied := config.ApplyConnectionConfig(account)
	if applied == nil {
		return account, nil
	}
	return applied, nil
}
