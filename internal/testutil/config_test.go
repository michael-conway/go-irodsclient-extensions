package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadExtensionsTestConfig(t *testing.T) {
	dir := t.TempDir()
	passwordPath := filepath.Join(dir, "irods-admin-password.txt")
	if err := os.WriteFile(passwordPath, []byte("rods\n"), 0600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	configPath := filepath.Join(dir, "extensions-test-config.yaml")
	configBody := "" +
		"IrodsHost: localhost\n" +
		"IrodsPort: 1247\n" +
		"IrodsZone: tempZone\n" +
		"IrodsAdminUser: rods\n" +
		"IrodsAdminPasswordFile: irods-admin-password.txt\n" +
		"IrodsAuthScheme: native\n" +
		"IrodsDefaultResource: demoResc\n" +
		"IrodsPrimaryTestUser: test1\n" +
		"IrodsPrimaryTestPassword: test\n" +
		"IrodsSecondaryTestUser: test2\n" +
		"IrodsSecondaryTestPassword: test2\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := ReadExtensionsTestConfig(configPath)
	if err != nil {
		t.Fatalf("read test config: %v", err)
	}

	if cfg.IrodsHost != "localhost" {
		t.Fatalf("expected host localhost, got %q", cfg.IrodsHost)
	}
	if cfg.IrodsPort != 1247 {
		t.Fatalf("expected port 1247, got %d", cfg.IrodsPort)
	}
	if cfg.IrodsZone != "tempZone" {
		t.Fatalf("expected zone tempZone, got %q", cfg.IrodsZone)
	}
	if cfg.IrodsAdminUser != "rods" {
		t.Fatalf("expected admin user rods, got %q", cfg.IrodsAdminUser)
	}
	if cfg.IrodsAdminPassword != "rods" {
		t.Fatalf("expected password to resolve from file, got %q", cfg.IrodsAdminPassword)
	}
	if cfg.IrodsAuthScheme != "native" {
		t.Fatalf("expected auth scheme native, got %q", cfg.IrodsAuthScheme)
	}
	if cfg.IrodsDefaultResource != "demoResc" {
		t.Fatalf("expected default resource demoResc, got %q", cfg.IrodsDefaultResource)
	}
	if cfg.IrodsPrimaryTestUser != "test1" {
		t.Fatalf("expected primary test user test1, got %q", cfg.IrodsPrimaryTestUser)
	}
	if cfg.IrodsPrimaryTestPassword != "test" {
		t.Fatalf("expected primary test password test, got %q", cfg.IrodsPrimaryTestPassword)
	}
	if cfg.IrodsSecondaryTestUser != "test2" {
		t.Fatalf("expected secondary test user test2, got %q", cfg.IrodsSecondaryTestUser)
	}
	if cfg.IrodsSecondaryTestPassword != "test2" {
		t.Fatalf("expected secondary test password test2, got %q", cfg.IrodsSecondaryTestPassword)
	}
}
