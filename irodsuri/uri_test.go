package irodsuri

import (
	"net/url"
	"testing"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestParseParsesIRODSURIWithUserZoneAndPassword(t *testing.T) {
	parsed, err := Parse("irods://rods%23tempZone:secret@icat.example.org:1247/tempZone/home/rods/file.txt")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if parsed.Host != "icat.example.org" {
		t.Fatalf("expected host icat.example.org, got %q", parsed.Host)
	}
	if parsed.Port != 1247 {
		t.Fatalf("expected port 1247, got %d", parsed.Port)
	}
	if parsed.Path != "/tempZone/home/rods/file.txt" {
		t.Fatalf("expected path, got %q", parsed.Path)
	}
	if parsed.UserInfo == nil {
		t.Fatal("expected user info")
	}
	if parsed.UserInfo.UserName != "rods" || parsed.UserInfo.Zone != "tempZone" || parsed.UserInfo.Password != "secret" {
		t.Fatalf("unexpected user info %+v", parsed.UserInfo)
	}
	if parsed.Ticket != "" {
		t.Fatalf("expected empty ticket, got %q", parsed.Ticket)
	}
}

func TestBuildBuildsIRODSURI(t *testing.T) {
	uri, err := Build("icat.example.org", 1247, &UserInfo{
		UserName: "rods",
		Zone:     "tempZone",
		Password: "secret",
	}, "/tempZone/home/rods/file.txt")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if got := uri.String(); got != "irods://rods%23tempZone:secret@icat.example.org:1247/tempZone/home/rods/file.txt" {
		t.Fatalf("unexpected URI %q", got)
	}
}

func TestBuildWithTicketBuildsIRODSURI(t *testing.T) {
	uri, err := BuildWithTicket("icat.example.org", 1247, &UserInfo{
		UserName: "rods",
		Zone:     "tempZone",
	}, "/tempZone/home/rods/file.txt", "ticket_abc123")
	if err != nil {
		t.Fatalf("BuildWithTicket returned error: %v", err)
	}

	if got := uri.String(); got != "irods://rods%23tempZone@icat.example.org:1247/tempZone/home/rods/file.txt?ticket=ticket_abc123" {
		t.Fatalf("unexpected URI %q", got)
	}
}

func TestBuildForAccountBuildsAbsoluteURI(t *testing.T) {
	account, err := irodstypes.CreateIRODSAccount(
		"icat.example.org",
		1247,
		"rods",
		"tempZone",
		irodstypes.AuthSchemeNative,
		"secret",
		"",
	)
	if err != nil {
		t.Fatalf("CreateIRODSAccount returned error: %v", err)
	}

	uri, err := BuildForAccount(account, "file.txt", true)
	if err != nil {
		t.Fatalf("BuildForAccount returned error: %v", err)
	}

	if got := uri.String(); got != "irods://rods%23tempZone:secret@icat.example.org:1247/tempZone/home/rods/file.txt" {
		t.Fatalf("unexpected URI %q", got)
	}
}

func TestBuildForAccountWithoutUserInfoBuildsAnonymousStyleURI(t *testing.T) {
	account, err := irodstypes.CreateIRODSAccount(
		"icat.example.org",
		1247,
		"rods",
		"tempZone",
		irodstypes.AuthSchemeNative,
		"secret",
		"",
	)
	if err != nil {
		t.Fatalf("CreateIRODSAccount returned error: %v", err)
	}

	uri, err := BuildForAccountWithoutUserInfo(account, "/tempZone/home/rods/file.txt")
	if err != nil {
		t.Fatalf("BuildForAccountWithoutUserInfo returned error: %v", err)
	}

	if got := uri.String(); got != "irods://icat.example.org:1247/tempZone/home/rods/file.txt" {
		t.Fatalf("unexpected URI %q", got)
	}
}

func TestBuildForTicketAccountBuildsTicketURI(t *testing.T) {
	account, err := irodstypes.CreateIRODSAccountForTicket(
		"icat.example.org",
		1247,
		"rods",
		"tempZone",
		irodstypes.AuthSchemeNative,
		"",
		"ticket_abc123",
		"",
	)
	if err != nil {
		t.Fatalf("CreateIRODSAccountForTicket returned error: %v", err)
	}

	uri, err := BuildForTicketAccount(account, "file.txt")
	if err != nil {
		t.Fatalf("BuildForTicketAccount returned error: %v", err)
	}

	if got := uri.String(); got != "irods://rods%23tempZone@icat.example.org:1247/tempZone/home/rods/file.txt?ticket=ticket_abc123" {
		t.Fatalf("unexpected URI %q", got)
	}
}

func TestAccountFromURLBuildsIRODSAccount(t *testing.T) {
	uri, err := url.Parse("irods://rods%23tempZone:secret@icat.example.org:1247/tempZone/home/rods/file.txt")
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}

	account, irodsPath, err := AccountFromURL(uri)
	if err != nil {
		t.Fatalf("AccountFromURL returned error: %v", err)
	}

	if account.Host != "icat.example.org" || account.Port != 1247 {
		t.Fatalf("unexpected host/port %+v", account)
	}
	if account.ClientUser != "rods" || account.ClientZone != "tempZone" {
		t.Fatalf("unexpected user fields %+v", account)
	}
	if account.Password != "secret" {
		t.Fatalf("expected password secret, got %q", account.Password)
	}
	if irodsPath != "/tempZone/home/rods/file.txt" {
		t.Fatalf("expected path, got %q", irodsPath)
	}
}

func TestTicketAccountFromURLBuildsIRODSTicketAccount(t *testing.T) {
	uri, err := url.Parse("irods://rods%23tempZone@icat.example.org:1247/tempZone/home/rods/file.txt?ticket=ticket_abc123")
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}

	account, irodsPath, err := TicketAccountFromURL(uri)
	if err != nil {
		t.Fatalf("TicketAccountFromURL returned error: %v", err)
	}

	if account.Host != "icat.example.org" || account.Port != 1247 {
		t.Fatalf("unexpected host/port %+v", account)
	}
	if account.ClientUser != "rods" || account.ClientZone != "tempZone" {
		t.Fatalf("unexpected user fields %+v", account)
	}
	if account.Ticket != "ticket_abc123" {
		t.Fatalf("expected ticket ticket_abc123, got %q", account.Ticket)
	}
	if irodsPath != "/tempZone/home/rods/file.txt" {
		t.Fatalf("expected path, got %q", irodsPath)
	}
}
