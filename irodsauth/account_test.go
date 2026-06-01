package irodsauth

import (
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestCreateAccountBasic(t *testing.T) {
	account, err := CreateAccount(Request{
		AuthScheme:    AuthSchemeBasic,
		Username:      "alice",
		BasicPassword: "secret",
	}, testConfig())
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}

	if account.ClientUser != "alice" || account.ProxyUser != "alice" {
		t.Fatalf("expected alice direct account, got client=%q proxy=%q", account.ClientUser, account.ProxyUser)
	}
	if account.AuthenticationScheme != irodstypes.AuthSchemePAM {
		t.Fatalf("expected request auth scheme PAM, got %q", account.AuthenticationScheme)
	}
}

func TestCreateAccountBearer(t *testing.T) {
	account, err := CreateAccount(Request{
		AuthScheme: AuthSchemeBearer,
		Username:   "alice",
	}, testConfig())
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}

	if account.ClientUser != "alice" || account.ProxyUser != "rods" {
		t.Fatalf("expected proxy account client=alice proxy=rods, got client=%q proxy=%q", account.ClientUser, account.ProxyUser)
	}
	if account.AuthenticationScheme != irodstypes.AuthSchemeNative {
		t.Fatalf("expected admin auth scheme native, got %q", account.AuthenticationScheme)
	}
}

func TestCreateAccountBearerTicket(t *testing.T) {
	account, err := CreateAccount(Request{
		AuthScheme: AuthSchemeBearerTicket,
		Ticket:     "ticket-1",
	}, testConfig())
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}

	if account.ClientUser != "rods" || account.ProxyUser != "rods" {
		t.Fatalf("expected admin ticket account, got client=%q proxy=%q", account.ClientUser, account.ProxyUser)
	}
	if account.Ticket != "ticket-1" {
		t.Fatalf("expected ticket to be carried through, got %q", account.Ticket)
	}
}

func TestCreateAccountAppliesConnectionConfig(t *testing.T) {
	called := false
	account, err := CreateAccount(Request{
		AuthScheme:    AuthSchemeBasic,
		Username:      "alice",
		BasicPassword: "secret",
	}, Config{
		Host:              "irods.local",
		Port:              1247,
		Zone:              "tempZone",
		DefaultResource:   "demoResc",
		AdminUser:         "rods",
		AdminPassword:     "rods",
		RequestAuthScheme: irodstypes.AuthSchemePAM,
		AdminAuthScheme:   irodstypes.AuthSchemeNative,
		ApplyConnectionConfig: func(account *irodstypes.IRODSAccount) *irodstypes.IRODSAccount {
			called = true
			account.ClientServerNegotiation = true
			account.CSNegotiationPolicy = irodstypes.CSNegotiationPolicyRequestSSL
			return account
		},
	})
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	if !called {
		t.Fatal("expected ApplyConnectionConfig to be called")
	}
	if !account.ClientServerNegotiation || account.CSNegotiationPolicy != irodstypes.CSNegotiationPolicyRequestSSL {
		t.Fatalf("expected account connection policy to be applied, got negotiation=%v policy=%q", account.ClientServerNegotiation, account.CSNegotiationPolicy)
	}
}

func TestCreateAccountErrors(t *testing.T) {
	_, err := CreateAccount(Request{AuthScheme: AuthSchemeBearer}, testConfig())
	if err == nil {
		t.Fatal("expected bearer missing username error")
	}

	_, err = CreateAccount(Request{AuthScheme: AuthSchemeBearerTicket}, testConfig())
	if err == nil {
		t.Fatal("expected bearer-ticket missing ticket error")
	}

	_, err = CreateAccount(Request{AuthScheme: "unknown"}, testConfig())
	if err == nil {
		t.Fatal("expected unsupported auth scheme error")
	}
}

func testConfig() Config {
	return Config{
		Host:              "irods.local",
		Port:              1247,
		Zone:              "tempZone",
		DefaultResource:   "demoResc",
		AdminUser:         "rods",
		AdminPassword:     "rods",
		RequestAuthScheme: irodstypes.AuthSchemePAM,
		AdminAuthScheme:   irodstypes.AuthSchemeNative,
	}
}
