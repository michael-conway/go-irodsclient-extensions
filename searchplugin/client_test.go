package searchplugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientListPropertiesAndSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/indexes":
			if request.Method != http.MethodGet {
				http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			_, _ = writer.Write([]byte(`{"indexes":[{"id":"catalog"}]}`))
		case "/v1/attributes/biomed":
			if request.Method != http.MethodGet {
				http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			_, _ = writer.Write([]byte(`{"attributes":[{"attrib_name":"author"}]}`))
		case "/v1/search":
			if request.Method != http.MethodPost {
				http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if request.URL.Query().Get("index_name") != "biomed" || request.URL.Query().Get("search_query") != "genome" {
				http.Error(writer, "unexpected query params", http.StatusBadRequest)
				return
			}
			_, _ = writer.Write([]byte(`{"search_result":[{"title":"Genome Study"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	registry := NewRegistry()
	if err := registry.Register(Endpoint{
		ID:      "catalog",
		BaseURL: server.URL,
		Enabled: true,
		Routes: Routes{
			ListPath:       "/v1/indexes",
			PropertiesPath: "/v1/attributes/{index_name}",
			SearchPath:     "/v1/search",
		},
	}); err != nil {
		t.Fatalf("register endpoint: %v", err)
	}

	client := NewClient(registry)

	listResponse, err := client.List(context.Background(), "catalog", RequestAuthContext{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResponse.StatusCode)
	}

	propertiesResponse, err := client.Properties(context.Background(), "catalog", "biomed", RequestAuthContext{})
	if err != nil {
		t.Fatalf("properties: %v", err)
	}
	if propertiesResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected properties status 200, got %d", propertiesResponse.StatusCode)
	}

	searchResponse, err := client.Search(context.Background(), "catalog", SearchRequest{
		IndexName:   "biomed",
		SearchQuery: "genome",
	}, RequestAuthContext{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if searchResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected search status 200, got %d", searchResponse.StatusCode)
	}
	if !strings.Contains(string(searchResponse.Body), "Genome Study") {
		t.Fatalf("expected search result in response body, got %q", string(searchResponse.Body))
	}
}

func TestClientReturnsHTTPStatusErrorAndCapturedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, `{"error":"bad request"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: server.URL, Enabled: true})

	client := NewClient(registry)
	response, err := client.List(context.Background(), "catalog", RequestAuthContext{})
	if err == nil {
		t.Fatal("expected list request to fail with HTTPStatusError")
	}
	var statusErr HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected HTTPStatusError, got %T", err)
	}
	if response == nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected captured response with status 400, got %+v", response)
	}
}

func TestClientRejectsDisabledEndpoint(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: "https://example.org", Enabled: false})
	client := NewClient(registry)

	_, err := client.List(context.Background(), "catalog", RequestAuthContext{})
	if !errors.Is(err, ErrEndpointDisabled) {
		t.Fatalf("expected ErrEndpointDisabled, got %v", err)
	}
}

func TestPassThroughBearerAuthorizer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer abc123" {
			http.Error(writer, "missing bearer", http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: server.URL, Enabled: true})
	client := NewClient(registry, WithRequestAuthorizer(PassThroughBearerAuthorizer()))

	_, err := client.List(context.Background(), "catalog", RequestAuthContext{BearerToken: "abc123"})
	if err != nil {
		t.Fatalf("expected bearer-authorized request to succeed, got %v", err)
	}
}

func TestPassThroughBasicAuthorizer(t *testing.T) {
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != expected {
			http.Error(writer, "missing basic auth", http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: server.URL, Enabled: true})
	client := NewClient(registry, WithRequestAuthorizer(PassThroughBasicAuthorizer()))

	_, err := client.List(context.Background(), "catalog", RequestAuthContext{BasicUsername: "alice", BasicPassword: "secret"})
	if err != nil {
		t.Fatalf("expected basic-auth request to succeed, got %v", err)
	}
}

func TestSearchAllReturnsPerEndpointErrors(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"results":[{"id":"ok"}]}`))
	}))
	defer okServer.Close()

	errorServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "nope", http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "a", BaseURL: okServer.URL, Enabled: true})
	_ = registry.Register(Endpoint{ID: "b", BaseURL: errorServer.URL, Enabled: true})

	client := NewClient(registry)
	results := client.SearchAll(context.Background(), SearchRequest{IndexName: "x", SearchQuery: "x"}, map[string]RequestAuthContext{})
	if len(results) != 2 {
		t.Fatalf("expected 2 federated search results, got %d", len(results))
	}

	var foundSuccess bool
	var foundFailure bool
	for _, result := range results {
		if result.EndpointID == "a" && result.Error == "" && result.Response != nil && result.Response.StatusCode == http.StatusOK {
			foundSuccess = true
		}
		if result.EndpointID == "b" && result.Error != "" && result.Response != nil && result.Response.StatusCode == http.StatusInternalServerError {
			foundFailure = true
		}
	}

	if !foundSuccess || !foundFailure {
		t.Fatalf("expected one success and one failure, got %+v", results)
	}
}

func TestClientPingUpdatesStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: server.URL, Enabled: true})
	client := NewClient(registry)

	status, err := client.Ping(context.Background(), "catalog", RequestAuthContext{})
	if err != nil {
		t.Fatalf("ping endpoint: %v", err)
	}
	if status.State != PluginStateHealthy {
		t.Fatalf("expected healthy status, got %+v", status)
	}
	if status.LastStatusCode != http.StatusOK {
		t.Fatalf("expected last status code 200, got %+v", status)
	}
}

func TestClientSearchRawBodyValidation(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: "https://example.org", Enabled: true})
	client := NewClient(registry)

	_, err := client.Search(context.Background(), "catalog", SearchRequest{
		IndexName:   "x",
		SearchQuery: "y",
		Raw:         json.RawMessage("{not-json"),
	}, RequestAuthContext{})
	if err == nil {
		t.Fatal("expected invalid raw json body to fail")
	}
}

func TestClientSearchRequiresIndexAndQuery(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(Endpoint{ID: "catalog", BaseURL: "https://example.org", Enabled: true})
	client := NewClient(registry)

	_, err := client.Search(context.Background(), "catalog", SearchRequest{
		SearchQuery: "genome",
	}, RequestAuthContext{})
	if !errors.Is(err, ErrIndexNameRequired) {
		t.Fatalf("expected ErrIndexNameRequired, got %v", err)
	}

	_, err = client.Search(context.Background(), "catalog", SearchRequest{
		IndexName: "biomed",
	}, RequestAuthContext{})
	if !errors.Is(err, ErrSearchQueryRequired) {
		t.Fatalf("expected ErrSearchQueryRequired, got %v", err)
	}
}
