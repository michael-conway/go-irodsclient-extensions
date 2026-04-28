package tickets

import "testing"

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
