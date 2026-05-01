package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

var (
	integrationConfigOnce  sync.Once
	integrationConfigValue *ExtensionsTestConfig
	integrationConfigErr   error
)

func RequireIntegrationConfig(t testing.TB) *ExtensionsTestConfig {
	t.Helper()

	cfg := OptionalIntegrationConfig(t)
	if cfg == nil {
		t.Fatalf("integration tests require %s to point at the shared integration config file", ExtensionsIntegrationConfigFileEnvVar)
	}

	return cfg
}

func OptionalIntegrationConfig(t testing.TB) *ExtensionsTestConfig {
	integrationConfigOnce.Do(func() {
		loadIntegrationConfig()
	})

	if integrationConfigErr != nil && t != nil {
		t.Fatalf("%v", integrationConfigErr)
	}

	return integrationConfigValue
}

func NewIntegrationAdminFilesystem(t testing.TB) *irodsfs.FileSystem {
	t.Helper()

	cfg := RequireIntegrationConfig(t)
	requireNonEmptyIntegrationValue(t, "IrodsHost", cfg.IrodsHost)
	if cfg.IrodsPort <= 0 {
		t.Fatalf("integration tests require IrodsPort in %s", ExtensionsIntegrationConfigFileEnvVar)
	}
	requireNonEmptyIntegrationValue(t, "IrodsZone", cfg.IrodsZone)
	requireNonEmptyIntegrationValue(t, "IrodsAdminUser", cfg.IrodsAdminUser)
	requireNonEmptyIntegrationValue(t, "IrodsAdminPassword", cfg.IrodsAdminPassword)
	requireNonEmptyIntegrationValue(t, "IrodsAuthScheme", cfg.IrodsAuthScheme)

	account, err := irodstypes.CreateIRODSAccount(
		cfg.IrodsHost,
		cfg.IrodsPort,
		cfg.IrodsAdminUser,
		cfg.IrodsZone,
		irodstypes.GetAuthScheme(cfg.IrodsAuthScheme),
		cfg.IrodsAdminPassword,
		cfg.IrodsDefaultResource,
	)
	if err != nil {
		t.Fatalf("create iRODS admin account: %v", err)
	}

	filesystem, err := irodsfs.NewFileSystemWithDefault(account, "go-irodsclient-extensions-integration-test")
	if err != nil {
		t.Fatalf("connect to iRODS for integration tests: %v", err)
	}

	return filesystem
}

func NewIntegrationPrimaryTestFilesystem(t testing.TB) *irodsfs.FileSystem {
	t.Helper()

	cfg := RequireIntegrationConfig(t)
	requireNonEmptyIntegrationValue(t, "IrodsHost", cfg.IrodsHost)
	if cfg.IrodsPort <= 0 {
		t.Fatalf("integration tests require IrodsPort in %s", ExtensionsIntegrationConfigFileEnvVar)
	}
	requireNonEmptyIntegrationValue(t, "IrodsZone", cfg.IrodsZone)
	requireNonEmptyIntegrationValue(t, "IrodsAuthScheme", cfg.IrodsAuthScheme)

	account, err := irodstypes.CreateIRODSAccount(
		cfg.IrodsHost,
		cfg.IrodsPort,
		IntegrationPrimaryTestUser(t),
		cfg.IrodsZone,
		irodstypes.GetAuthScheme(cfg.IrodsAuthScheme),
		IntegrationPrimaryTestPassword(t),
		cfg.IrodsDefaultResource,
	)
	if err != nil {
		t.Fatalf("create iRODS primary test account: %v", err)
	}

	filesystem, err := irodsfs.NewFileSystemWithDefault(account, "go-irodsclient-extensions-integration-test-primary")
	if err != nil {
		t.Fatalf("connect to iRODS for primary-user integration tests: %v", err)
	}

	return filesystem
}

func IntegrationPrimaryTestUser(t testing.TB) string {
	t.Helper()

	cfg := RequireIntegrationConfig(t)
	requireNonEmptyIntegrationValue(t, "IrodsPrimaryTestUser", cfg.IrodsPrimaryTestUser)
	return strings.TrimSpace(cfg.IrodsPrimaryTestUser)
}

func IntegrationPrimaryTestPassword(t testing.TB) string {
	t.Helper()

	cfg := RequireIntegrationConfig(t)
	requireNonEmptyIntegrationValue(t, "IrodsPrimaryTestPassword", cfg.IrodsPrimaryTestPassword)
	return strings.TrimSpace(cfg.IrodsPrimaryTestPassword)
}

func IntegrationSecondaryTestUser(t testing.TB) string {
	t.Helper()

	cfg := RequireIntegrationConfig(t)
	requireNonEmptyIntegrationValue(t, "IrodsSecondaryTestUser", cfg.IrodsSecondaryTestUser)
	return strings.TrimSpace(cfg.IrodsSecondaryTestUser)
}

func IntegrationSecondaryTestPassword(t testing.TB) string {
	t.Helper()

	cfg := RequireIntegrationConfig(t)
	requireNonEmptyIntegrationValue(t, "IrodsSecondaryTestPassword", cfg.IrodsSecondaryTestPassword)
	return strings.TrimSpace(cfg.IrodsSecondaryTestPassword)
}

func loadIntegrationConfig() {
	configFile := strings.TrimSpace(os.Getenv(ExtensionsIntegrationConfigFileEnvVar))
	if configFile == "" {
		return
	}

	resolvedPath, err := ResolveIntegrationConfigPath(configFile)
	if err != nil {
		integrationConfigErr = err
		return
	}

	cfg, err := ReadExtensionsTestConfig(resolvedPath)
	if err != nil {
		integrationConfigErr = fmt.Errorf("read integration config from %s=%q: %w", ExtensionsIntegrationConfigFileEnvVar, resolvedPath, err)
		return
	}

	integrationConfigValue = cfg
}

func ResolveIntegrationConfigPath(configFile string) (string, error) {
	configFile = strings.TrimSpace(configFile)
	if configFile == "" {
		return "", fmt.Errorf("empty config file path")
	}

	if filepath.IsAbs(configFile) {
		return configFile, nil
	}

	repoRoot, err := IntegrationRepoRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, configFile), nil
}

func IntegrationRepoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve relative %s path: runtime caller unavailable", ExtensionsIntegrationConfigFileEnvVar)
	}

	testutilDir := filepath.Dir(filename)
	internalDir := filepath.Dir(testutilDir)
	return filepath.Dir(internalDir), nil
}

func requireNonEmptyIntegrationValue(t testing.TB, field string, value string) {
	t.Helper()

	if strings.TrimSpace(value) == "" {
		t.Fatalf("integration tests require %s in %s", field, ExtensionsIntegrationConfigFileEnvVar)
	}
}
