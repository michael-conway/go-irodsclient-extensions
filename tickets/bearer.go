package tickets

import "strings"

const BearerTokenPrefix = "irods-ticket:"

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
