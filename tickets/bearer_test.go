package tickets

import (
	"errors"
	"strings"
	"testing"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestParseBearerToken(t *testing.T) {
	ticket, ok := ParseBearerToken("irods-ticket:ticket123")
	if !ok {
		t.Fatal("expected ticket bearer token to parse")
	}
	if ticket != "ticket123" {
		t.Fatalf("expected parsed ticket ticket123, got %q", ticket)
	}
}

func TestParseBearerTokenTrimsWhitespace(t *testing.T) {
	ticket, ok := ParseBearerToken("  irods-ticket:ticket123  ")
	if !ok {
		t.Fatal("expected whitespace-padded ticket bearer token to parse")
	}
	if ticket != "ticket123" {
		t.Fatalf("expected parsed ticket ticket123, got %q", ticket)
	}
}

func TestParseBearerTokenAcceptsCaseInsensitivePrefix(t *testing.T) {
	ticket, ok := ParseBearerToken("IRODS-TICKET:ticket123")
	if !ok {
		t.Fatal("expected case-insensitive ticket bearer token to parse")
	}
	if ticket != "ticket123" {
		t.Fatalf("expected parsed ticket ticket123, got %q", ticket)
	}
}

func TestParseBearerTokenRejectsNonTicketBearer(t *testing.T) {
	if ticket, ok := ParseBearerToken("token123"); ok || ticket != "" {
		t.Fatalf("expected non-ticket bearer token to be rejected, got %q", ticket)
	}
}

func TestParseBearerTokenRejectsEmptyTicket(t *testing.T) {
	if ticket, ok := ParseBearerToken("irods-ticket:   "); ok || ticket != "" {
		t.Fatalf("expected empty ticket bearer token to be rejected, got %q", ticket)
	}
}

func TestFormatBearerToken(t *testing.T) {
	token := FormatBearerToken("ticket123")
	if token != "irods-ticket:ticket123" {
		t.Fatalf("expected formatted bearer token, got %q", token)
	}
}

func TestCreateAnonymousDataObjectBearerToken(t *testing.T) {
	oldTicketID := newTicketID
	oldNow := ticketTimeNow
	newTicketID = func() (string, error) { return "ticket123", nil }
	ticketTimeNow = func() time.Time { return time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC) }
	defer func() {
		newTicketID = oldTicketID
		ticketTimeNow = oldNow
	}()

	fs := &testAnonymousTicketFilesystem{}
	ticketID, bearerToken, err := CreateAnonymousDataObjectBearerToken(fs, "/tempZone/home/test1/file.txt", 50, 720)
	if err != nil {
		t.Fatalf("expected ticket creation to succeed, got %v", err)
	}

	if ticketID != "ticket123" {
		t.Fatalf("expected ticket id ticket123, got %q", ticketID)
	}
	if bearerToken != "irods-ticket:ticket123" {
		t.Fatalf("expected formatted bearer token, got %q", bearerToken)
	}
	if fs.createdTicketName != "ticket123" {
		t.Fatalf("expected created ticket name ticket123, got %q", fs.createdTicketName)
	}
	if fs.createdTicketType != irodstypes.TicketTypeRead {
		t.Fatalf("expected read ticket type, got %q", fs.createdTicketType)
	}
	if fs.createdPath != "/tempZone/home/test1/file.txt" {
		t.Fatalf("expected created path to match input, got %q", fs.createdPath)
	}
	if fs.modifiedUses != 50 {
		t.Fatalf("expected use limit 50, got %d", fs.modifiedUses)
	}

	expectedExpiration := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	if !fs.modifiedExpiration.Equal(expectedExpiration) {
		t.Fatalf("expected expiration %v, got %v", expectedExpiration, fs.modifiedExpiration)
	}
}

func TestCreateAnonymousDataObjectBearerTokenRejectsInvalidInput(t *testing.T) {
	if _, _, err := CreateAnonymousDataObjectBearerToken(nil, "/tempZone/home/test1/file.txt", 1, 1); !errors.Is(err, ErrMissingFilesystem) {
		t.Fatalf("expected ErrMissingFilesystem, got %v", err)
	}
	if _, _, err := CreateAnonymousDataObjectBearerToken(&testAnonymousTicketFilesystem{}, "   ", 1, 1); !errors.Is(err, ErrInvalidIRODSPath) {
		t.Fatalf("expected ErrInvalidIRODSPath, got %v", err)
	}
	if _, _, err := CreateAnonymousDataObjectBearerToken(&testAnonymousTicketFilesystem{}, "/tempZone/home/test1/file.txt", -1, 1); !errors.Is(err, ErrInvalidMaximumUses) {
		t.Fatalf("expected ErrInvalidMaximumUses, got %v", err)
	}
	if _, _, err := CreateAnonymousDataObjectBearerToken(&testAnonymousTicketFilesystem{}, "/tempZone/home/test1/file.txt", 1, -1); !errors.Is(err, ErrInvalidTicketExpiry) {
		t.Fatalf("expected ErrInvalidTicketExpiry, got %v", err)
	}
}

func TestCreateAnonymousDataObjectBearerTokenRollsBackOnRestrictionFailure(t *testing.T) {
	oldTicketID := newTicketID
	newTicketID = func() (string, error) { return "ticket123", nil }
	defer func() { newTicketID = oldTicketID }()

	fs := &testAnonymousTicketFilesystem{
		modifyUsesErr: errors.New("boom"),
	}

	_, _, err := CreateAnonymousDataObjectBearerToken(fs, "/tempZone/home/test1/file.txt", 50, 720)
	if err == nil {
		t.Fatal("expected restriction failure to return an error")
	}
	if fs.deletedTicketName != "ticket123" {
		t.Fatalf("expected created ticket to be deleted on failure, got %q", fs.deletedTicketName)
	}
}

func TestCreateAnonymousDataObjectBearerTokenErrorDoesNotLeakTicketID(t *testing.T) {
	oldTicketID := newTicketID
	newTicketID = func() (string, error) { return "ticket123", nil }
	defer func() { newTicketID = oldTicketID }()

	fs := &testAnonymousTicketFilesystem{
		modifyUsesErr: errors.New("boom"),
	}

	_, _, err := CreateAnonymousDataObjectBearerToken(fs, "/tempZone/home/test1/file.txt", 50, 720)
	if err == nil {
		t.Fatal("expected restriction failure to return an error")
	}

	message := err.Error()
	if strings.Contains(message, "ticket123") {
		t.Fatalf("expected error to avoid ticket id leakage, got %q", message)
	}
	if strings.Contains(message, "irods-ticket:") {
		t.Fatalf("expected error to avoid bearer token leakage, got %q", message)
	}
}

type testAnonymousTicketFilesystem struct {
	createdTicketName  string
	createdTicketType  irodstypes.TicketType
	createdPath        string
	modifiedUses       int64
	modifiedExpiration time.Time
	deletedTicketName  string
	createErr          error
	modifyUsesErr      error
	modifyExpiryErr    error
	deleteErr          error
}

func (fs *testAnonymousTicketFilesystem) CreateTicket(ticketName string, ticketType irodstypes.TicketType, path string) error {
	if fs.createErr != nil {
		return fs.createErr
	}
	fs.createdTicketName = ticketName
	fs.createdTicketType = ticketType
	fs.createdPath = path
	return nil
}

func (fs *testAnonymousTicketFilesystem) ModifyTicketUseLimit(ticketName string, uses int64) error {
	if fs.modifyUsesErr != nil {
		return fs.modifyUsesErr
	}
	fs.modifiedUses = uses
	return nil
}

func (fs *testAnonymousTicketFilesystem) ModifyTicketExpirationTime(ticketName string, expirationTime time.Time) error {
	if fs.modifyExpiryErr != nil {
		return fs.modifyExpiryErr
	}
	fs.modifiedExpiration = expirationTime
	return nil
}

func (fs *testAnonymousTicketFilesystem) DeleteTicket(ticketName string) error {
	if fs.deleteErr != nil {
		return fs.deleteErr
	}
	fs.deletedTicketName = ticketName
	return nil
}
