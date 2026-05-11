package tickets

import (
	"errors"
	"fmt"
	"strings"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	"github.com/rs/xid"
)

const BearerTokenPrefix = "irods-ticket:"

var (
	ErrMissingFilesystem   = errors.New("missing iRODS filesystem")
	ErrInvalidIRODSPath    = errors.New("invalid iRODS path")
	ErrInvalidMaximumUses  = errors.New("invalid maximum uses")
	ErrInvalidTicketExpiry = errors.New("invalid ticket lifetime minutes")
)

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
		return "", "", ErrMissingFilesystem
	}

	irodsPath = strings.TrimSpace(irodsPath)
	if irodsPath == "" {
		return "", "", ErrInvalidIRODSPath
	}

	if maximumUses < 0 {
		return "", "", ErrInvalidMaximumUses
	}

	if ticketLifetimeMinutes < 0 {
		return "", "", ErrInvalidTicketExpiry
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
			return "", "", errors.Join(createErr, fmt.Errorf("cleanup ticket: %w", deleteErr))
		}
		return "", "", createErr
	}

	if maximumUses > 0 {
		if err := filesystem.ModifyTicketUseLimit(ticketID, maximumUses); err != nil {
			return cleanupOnError(fmt.Errorf("set use limit on created ticket: %w", err))
		}
	}

	if ticketLifetimeMinutes > 0 {
		expirationTime := ticketTimeNow().Add(time.Duration(ticketLifetimeMinutes) * time.Minute)
		if err := filesystem.ModifyTicketExpirationTime(ticketID, expirationTime); err != nil {
			return cleanupOnError(fmt.Errorf("set expiration on created ticket: %w", err))
		}
	}

	return ticketID, FormatBearerToken(ticketID), nil
}
