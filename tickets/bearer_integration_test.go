//go:build integration
// +build integration

package tickets

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/cyverse/go-irodsclient/irods/util"
	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
)

func TestCreateAnonymousDataObjectBearerTokenIntegration(t *testing.T) {
	filesystem := testutil.NewIntegrationPrimaryTestFilesystem(t)
	defer filesystem.Release()
	adminFilesystem := testutil.NewIntegrationAdminFilesystem(t)
	defer adminFilesystem.Release()

	cfg := testutil.RequireIntegrationConfig(t)
	rootPath := util.GetCorrectIRODSPath("/" + strings.TrimSpace(cfg.IrodsZone) + "/home/" + testutil.IntegrationPrimaryTestUser(t) + "/extensions-ticket-integration-" + time.Now().UTC().Format("20060102150405.000000000"))
	if err := filesystem.MakeDir(rootPath, true); err != nil {
		t.Fatalf("create integration ticket root %q: %v", rootPath, err)
	}
	defer func() {
		if err := filesystem.RemoveDir(rootPath, true, true); err != nil && filesystem.Exists(rootPath) {
			t.Errorf("cleanup integration ticket root %q: %v", rootPath, err)
		}
	}()

	objectPath := util.GetCorrectIRODSPath(rootPath + "/object.txt")
	if _, err := filesystem.UploadFileFromBuffer(bytes.NewBufferString("extensions ticket integration payload\n"), objectPath, "", false, true, false, false, nil); err != nil {
		t.Fatalf("upload integration ticket object %q: %v", objectPath, err)
	}

	ticketID, bearerToken, err := CreateAnonymousDataObjectBearerToken(filesystem, objectPath, 5, 30)
	if err != nil {
		t.Fatalf("create anonymous ticket bearer token: %v", err)
	}
	visibleTicket, err := waitForTicket(adminFilesystem, ticketID, objectPath, 3*time.Second)
	if err != nil {
		t.Fatalf("lookup created ticket %q: %v", ticketID, err)
	}
	defer func() {
		if err := filesystem.DeleteTicket(visibleTicket.Name); err != nil {
			t.Errorf("cleanup integration ticket %q: %v", visibleTicket.Name, err)
		}
	}()

	if bearerToken != FormatBearerToken(ticketID) {
		t.Fatalf("expected bearer token %q, got %q", FormatBearerToken(ticketID), bearerToken)
	}

	parsedTicket, ok := ParseBearerToken(bearerToken)
	if !ok {
		t.Fatal("expected created bearer token to parse")
	}
	if parsedTicket != ticketID {
		t.Fatalf("expected parsed ticket id %q, got %q", ticketID, parsedTicket)
	}

	if visibleTicket.Name != ticketID {
		t.Fatalf("expected ticket name %q, got %q", ticketID, visibleTicket.Name)
	}
	if visibleTicket.Path != objectPath {
		t.Fatalf("expected ticket path %q, got %q", objectPath, visibleTicket.Path)
	}
	if visibleTicket.UsesLimit != 5 {
		t.Fatalf("expected ticket uses limit 5, got %d", visibleTicket.UsesLimit)
	}
	if visibleTicket.ExpirationTime.IsZero() {
		t.Fatal("expected non-zero ticket expiration time")
	}
}

func waitForTicket(filesystem interface {
	ListTickets() ([]*irodstypes.IRODSTicket, error)
}, ticketName string, objectPath string, timeout time.Duration) (*irodstypes.IRODSTicket, error) {
	deadline := time.Now().Add(timeout)
	var lastNames []string

	for time.Now().Before(deadline) {
		tickets, err := filesystem.ListTickets()
		if err != nil {
			return nil, err
		}

		lastNames = lastNames[:0]
		for _, ticket := range tickets {
			if ticket == nil {
				continue
			}

			lastNames = append(lastNames, fmt.Sprintf("%s:%s", ticket.Name, ticket.Path))
			if ticket.Name == ticketName && ticket.Path == objectPath {
				return ticket, nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("ticket not visible after %s; visible tickets: %s", timeout, strings.Join(lastNames, ", "))
}
