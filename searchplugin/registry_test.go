package searchplugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()

	err := registry.Register(Endpoint{
		ID:      "catalog",
		BaseURL: "https://example.org/plugins/catalog",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("register endpoint: %v", err)
	}

	endpoint, err := registry.Get("catalog")
	if err != nil {
		t.Fatalf("get endpoint: %v", err)
	}
	if endpoint.ID != "catalog" {
		t.Fatalf("expected endpoint id catalog, got %q", endpoint.ID)
	}
	if endpoint.Routes.ListPath == "" || endpoint.Routes.PropertiesPath == "" || endpoint.Routes.SearchPath == "" {
		t.Fatalf("expected default routes to be applied, got %+v", endpoint.Routes)
	}
	if endpoint.AuthType != AuthTypeNone {
		t.Fatalf("expected default auth type none, got %q", endpoint.AuthType)
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(Endpoint{
		ID:      "catalog",
		BaseURL: "https://example.org/plugins/catalog",
		Enabled: true,
	})

	err := registry.Register(Endpoint{
		ID:      "catalog",
		BaseURL: "https://example.org/plugins/catalog-v2",
		Enabled: true,
	})
	if err == nil {
		t.Fatal("expected duplicate endpoint registration to fail")
	}
	if !errors.Is(err, ErrEndpointExists) {
		t.Fatalf("expected ErrEndpointExists, got %v", err)
	}
}

func TestRegistryRejectsInvalidEndpoint(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(Endpoint{BaseURL: "https://example.org"}); !errors.Is(err, ErrEndpointIDRequired) {
		t.Fatalf("expected ErrEndpointIDRequired, got %v", err)
	}

	if err := registry.Register(Endpoint{ID: "x"}); !errors.Is(err, ErrBaseURLRequired) {
		t.Fatalf("expected ErrBaseURLRequired, got %v", err)
	}

	if err := registry.Register(Endpoint{ID: "x", BaseURL: "/relative"}); !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("expected ErrInvalidBaseURL, got %v", err)
	}

	if err := registry.Register(Endpoint{ID: "x", BaseURL: "https://example.org", AuthType: "nonesuch"}); !errors.Is(err, ErrInvalidAuthType) {
		t.Fatalf("expected ErrInvalidAuthType, got %v", err)
	}
}

func TestRegistryListSortedByID(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Upsert(Endpoint{ID: "zeta", BaseURL: "https://example.org/zeta", Enabled: true})
	_ = registry.Upsert(Endpoint{ID: "alpha", BaseURL: "https://example.org/alpha", Enabled: true})
	_ = registry.Upsert(Endpoint{ID: "beta", BaseURL: "https://example.org/beta", Enabled: true})

	endpoints := registry.List()
	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].ID != "alpha" || endpoints[1].ID != "beta" || endpoints[2].ID != "zeta" {
		t.Fatalf("expected endpoints sorted by id, got %+v", endpoints)
	}
}

func TestRegistryLoadAndReloadConfig(t *testing.T) {
	registry := NewRegistry()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "plugins.yaml")

	first := []byte(`
plugins:
  - name: catalog
    uri: https://example.org/catalog
    auth_type: bearer_passthrough
    enabled: true
`)
	if err := os.WriteFile(configPath, first, 0600); err != nil {
		t.Fatalf("write first config: %v", err)
	}

	if err := registry.LoadConfigFile(configPath); err != nil {
		t.Fatalf("load config file: %v", err)
	}
	endpoints := registry.List()
	if len(endpoints) != 1 || endpoints[0].ID != "catalog" || endpoints[0].AuthType != AuthTypeBearerPassThrough || !endpoints[0].Enabled {
		t.Fatalf("unexpected endpoints after first load: %+v", endpoints)
	}

	second := []byte(`
plugins:
  - name: disabled-plugin
    uri: https://example.org/disabled
    auth_type: none
    enabled: false
`)
	if err := os.WriteFile(configPath, second, 0600); err != nil {
		t.Fatalf("write second config: %v", err)
	}

	if err := registry.ReloadConfigFile(); err != nil {
		t.Fatalf("reload config file: %v", err)
	}
	reloaded := registry.List()
	if len(reloaded) != 1 || reloaded[0].ID != "disabled-plugin" || reloaded[0].Enabled {
		t.Fatalf("unexpected endpoints after reload: %+v", reloaded)
	}
	status, err := registry.Status("disabled-plugin")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status.State != PluginStateDisabled {
		t.Fatalf("expected disabled state, got %+v", status)
	}
}

func TestRegistryInvocationInfo(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Endpoint{
		ID:       "catalog",
		Name:     "Catalog",
		BaseURL:  "https://example.org/api/v1",
		AuthType: AuthTypeBearerPassThrough,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register endpoint: %v", err)
	}

	propertiesInvocation, err := registry.Invocation("catalog", OperationProperties)
	if err != nil {
		t.Fatalf("invocation: %v", err)
	}
	if propertiesInvocation.Method != "GET" {
		t.Fatalf("expected GET method, got %+v", propertiesInvocation)
	}
	if propertiesInvocation.URLTemplate != "https://example.org/attributes/{index_name}" {
		t.Fatalf("unexpected url template: %+v", propertiesInvocation)
	}
	if len(propertiesInvocation.RequiredPathParameters) != 1 || propertiesInvocation.RequiredPathParameters[0] != "index_name" {
		t.Fatalf("unexpected path params: %+v", propertiesInvocation.RequiredPathParameters)
	}
}
