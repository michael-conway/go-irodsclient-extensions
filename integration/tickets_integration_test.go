//go:build integration
// +build integration

package integration

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
	"github.com/michael-conway/go-irodsclient-extensions/tickets"
	"github.com/rs/xid"
)

func TestTicketsLifecycleSanityIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()
	adminFilesystem := testutil.NewIntegrationAdminFilesystem(t)
	defer adminFilesystem.Release()

	homePath := strings.TrimSpace(filesystem.GetHomeDirPath())
	if homePath == "" {
		t.Fatalf("expected non-empty primary user home path")
	}

	fixtureRoot := path.Join(homePath, ".goext-tickets-integration-"+xid.New().String())
	if err := filesystem.MakeDir(fixtureRoot, true); err != nil {
		t.Fatalf("create fixture root %q: %v", fixtureRoot, err)
	}
	defer func() {
		if err := filesystem.RemoveDir(fixtureRoot, true, true); err != nil && filesystem.Exists(fixtureRoot) {
			t.Errorf("cleanup ticket fixture root %q: %v", fixtureRoot, err)
		}
	}()

	filePath := path.Join(fixtureRoot, "ticket-object.txt")
	if _, err := filesystem.UploadFileFromBuffer(bytes.NewBufferString("ticket integration payload\n"), filePath, "", false, true, nil); err != nil {
		t.Fatalf("upload ticket integration object %q: %v", filePath, err)
	}

	ticketID, bearerToken, err := tickets.CreateAnonymousDataObjectBearerToken(filesystem, filePath, 5, 30)
	if err != nil {
		t.Fatalf("create anonymous ticket bearer token: %v", err)
	}

	visibleTicket, err := waitForTicketVisibility(adminFilesystem, ticketID, filePath, 3*time.Second)
	if err != nil {
		t.Fatalf("lookup created ticket %q: %v", ticketID, err)
	}
	defer func() {
		if err := filesystem.DeleteTicket(visibleTicket.Name); err != nil {
			t.Errorf("cleanup ticket %q: %v", visibleTicket.Name, err)
		}
	}()

	if bearerToken != tickets.FormatBearerToken(ticketID) {
		t.Fatalf("expected bearer token %q, got %q", tickets.FormatBearerToken(ticketID), bearerToken)
	}
	parsedTicket, ok := tickets.ParseBearerToken(bearerToken)
	if !ok {
		t.Fatal("expected generated bearer token to parse")
	}
	if parsedTicket != ticketID {
		t.Fatalf("expected parsed ticket id %q, got %q", ticketID, parsedTicket)
	}
	if visibleTicket.Name != ticketID {
		t.Fatalf("expected ticket name %q, got %q", ticketID, visibleTicket.Name)
	}
	if visibleTicket.Path != filePath {
		t.Fatalf("expected ticket path %q, got %q", filePath, visibleTicket.Path)
	}
	if visibleTicket.UsesLimit != 5 {
		t.Fatalf("expected ticket uses limit 5, got %d", visibleTicket.UsesLimit)
	}
	if visibleTicket.ExpirationTime.IsZero() {
		t.Fatal("expected non-zero ticket expiration time")
	}
}

func waitForTicketVisibility(filesystem interface {
	ListTickets() ([]*irodstypes.IRODSTicket, error)
}, ticketName string, objectPath string, timeout time.Duration) (*irodstypes.IRODSTicket, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ticketsList, err := filesystem.ListTickets()
		if err != nil {
			return nil, err
		}

		for _, ticket := range ticketsList {
			if ticket == nil {
				continue
			}
			if ticket.Name == ticketName && ticket.Path == objectPath {
				return ticket, nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("ticket %q for %q not visible after %s", ticketName, objectPath, timeout)
}
