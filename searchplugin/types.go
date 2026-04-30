package searchplugin

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type Operation string

const (
	OperationList       Operation = "list"
	OperationProperties Operation = "properties"
	OperationSearch     Operation = "search"
)

type AuthType string

const (
	AuthTypeNone              AuthType = "none"
	AuthTypeBearerPassThrough AuthType = "bearer_passthrough"
	AuthTypeBasicPassThrough  AuthType = "basic_passthrough"
	AuthTypeStaticBearer      AuthType = "static_bearer"
	AuthTypeStaticBasic       AuthType = "static_basic"
	AuthTypeServiceAccount    AuthType = "service_account"
)

func (authType AuthType) Valid() bool {
	switch authType {
	case AuthTypeNone, AuthTypeBearerPassThrough, AuthTypeBasicPassThrough, AuthTypeStaticBearer, AuthTypeStaticBasic, AuthTypeServiceAccount:
		return true
	default:
		return false
	}
}

type Endpoint struct {
	ID       string            `json:"id"`
	Name     string            `json:"name,omitempty"`
	BaseURL  string            `json:"base_url"`
	AuthType AuthType          `json:"auth_type"`
	Enabled  bool              `json:"enabled"`
	Routes   Routes            `json:"routes,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Routes struct {
	ListPath       string `json:"list_path,omitempty"`
	PropertiesPath string `json:"properties_path,omitempty"`
	SearchPath     string `json:"search_path,omitempty"`
}

func (r Routes) withDefaults() Routes {
	if r.ListPath == "" {
		r.ListPath = "/indexes"
	}
	if r.PropertiesPath == "" {
		r.PropertiesPath = "/attributes/{index_name}"
	}
	if r.SearchPath == "" {
		r.SearchPath = "/search"
	}
	return r
}

func (e Endpoint) routeFor(operation Operation) string {
	routes := e.Routes.withDefaults()
	switch operation {
	case OperationList:
		return routes.ListPath
	case OperationProperties:
		return routes.PropertiesPath
	case OperationSearch:
		return routes.SearchPath
	default:
		return ""
	}
}

type RequestAuthContext struct {
	BearerToken   string
	BasicUsername string
	BasicPassword string
}

type SearchRequest struct {
	IndexName   string            `json:"index_name,omitempty"`
	SearchQuery string            `json:"search_query,omitempty"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Raw         json.RawMessage
}

type CapturedResponse struct {
	EndpointID string          `json:"endpoint_id"`
	Operation  Operation       `json:"operation"`
	StatusCode int             `json:"status_code"`
	Headers    http.Header     `json:"headers"`
	Body       json.RawMessage `json:"body"`
	ReceivedAt time.Time       `json:"received_at"`
	Duration   time.Duration   `json:"duration"`
}

func (r CapturedResponse) Decode(target any) error {
	return json.Unmarshal(r.Body, target)
}

type FederatedSearchResult struct {
	EndpointID string            `json:"endpoint_id"`
	Response   *CapturedResponse `json:"response,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type PluginState string

const (
	PluginStateUnknown   PluginState = "unknown"
	PluginStateHealthy   PluginState = "healthy"
	PluginStateUnhealthy PluginState = "unhealthy"
	PluginStateDisabled  PluginState = "disabled"
)

type PluginStatus struct {
	EndpointID     string        `json:"endpoint_id"`
	State          PluginState   `json:"state"`
	LastCheckedAt  *time.Time    `json:"last_checked_at,omitempty"`
	LastStatusCode int           `json:"last_status_code,omitempty"`
	LastError      string        `json:"last_error,omitempty"`
	Latency        time.Duration `json:"latency,omitempty"`
}

type EndpointInvocation struct {
	EndpointID              string    `json:"endpoint_id"`
	EndpointName            string    `json:"endpoint_name,omitempty"`
	Operation               Operation `json:"operation"`
	Method                  string    `json:"method"`
	URLTemplate             string    `json:"url_template"`
	RequiredPathParameters  []string  `json:"required_path_parameters,omitempty"`
	RequiredQueryParameters []string  `json:"required_query_parameters,omitempty"`
	AuthType                AuthType  `json:"auth_type"`
	Enabled                 bool      `json:"enabled"`
}

func (invocation EndpointInvocation) ResolveURL(pathParams map[string]string) string {
	url := invocation.URLTemplate
	for key, value := range pathParams {
		url = strings.ReplaceAll(url, "{"+key+"}", value)
	}
	return url
}
