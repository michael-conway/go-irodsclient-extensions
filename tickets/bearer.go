package tickets

import (
	"fmt"
	"strings"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/rs/xid"
)

const BearerTokenPrefix = "irods-ticket:"

type AnonymousTicketFilesystem interface {
	CreateTicket(ticketName string, ticketType irodstypes.TicketType, path string) error
	ModifyTicketUseLimit(ticketName string, uses int64) error
	ModifyTicketExpirationTime(ticketName string, expirationTime time.Time) error
	DeleteTicket(ticketName string) error
}

var ticketTimeNow = time.Now

var newTicketID = func() (string, error) {
	return "ticket_" + xid.New().String(), nil
}

// ParseBearerToken extracts an iRODS ticket from a bearer token payload such as
// "irods-ticket:abc123". It returns the normalized ticket value when present.
func ParseBearerToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(strings.ToLower(token), BearerTokenPrefix) {
		return "", false
	}

	ticket := strings.TrimSpace(token[len(BearerTokenPrefix):])
	if ticket == "" {
		return "", false
	}

	return ticket, true
}

func FormatBearerToken(ticketID string) string {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return ""
	}

	return BearerTokenPrefix + ticketID
}

// CreateAnonymousDataObjectBearerToken creates an anonymous read ticket for the
// given iRODS data object path, applies optional use and expiration limits, and
// returns both the ticket id and the formatted bearer token.
func CreateAnonymousDataObjectBearerToken(filesystem AnonymousTicketFilesystem, irodsPath string, maximumUses int64, ticketLifetimeMinutes int) (string, string, error) {
	if filesystem == nil {
		return "", "", fmt.Errorf("missing iRODS filesystem")
	}

	irodsPath = strings.TrimSpace(irodsPath)
	if irodsPath == "" {
		return "", "", fmt.Errorf("missing iRODS path")
	}

	if maximumUses < 0 {
		return "", "", fmt.Errorf("maximum uses must be zero or greater")
	}

	if ticketLifetimeMinutes < 0 {
		return "", "", fmt.Errorf("ticket lifetime minutes must be zero or greater")
	}

	ticketID, err := newTicketID()
	if err != nil {
		return "", "", err
	}

	if err := filesystem.CreateTicket(ticketID, irodstypes.TicketTypeRead, irodsPath); err != nil {
		return "", "", fmt.Errorf("create anonymous read ticket for %q: %w", irodsPath, err)
	}

	cleanupOnError := func(createErr error) (string, string, error) {
		if deleteErr := filesystem.DeleteTicket(ticketID); deleteErr != nil {
			return "", "", fmt.Errorf("%v: cleanup ticket %q: %w", createErr, ticketID, deleteErr)
		}
		return "", "", createErr
	}

	if maximumUses > 0 {
		if err := filesystem.ModifyTicketUseLimit(ticketID, maximumUses); err != nil {
			return cleanupOnError(fmt.Errorf("set use limit on ticket %q: %w", ticketID, err))
		}
	}

	if ticketLifetimeMinutes > 0 {
		expirationTime := ticketTimeNow().Add(time.Duration(ticketLifetimeMinutes) * time.Minute)
		if err := filesystem.ModifyTicketExpirationTime(ticketID, expirationTime); err != nil {
			return cleanupOnError(fmt.Errorf("set expiration on ticket %q: %w", ticketID, err))
		}
	}

	return ticketID, FormatBearerToken(ticketID), nil
}
