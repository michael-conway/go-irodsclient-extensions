//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/michael-conway/go-irodsclient-extensions/internal/testutil"
)

func TestIntegrationConfigIsPresent(t *testing.T) {
	cfg := testutil.RequireIntegrationConfig(t)
	if cfg == nil {
		t.Fatalf("expected integration config")
	}
}
