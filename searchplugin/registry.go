package searchplugin

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrEndpointIDRequired = errors.New("endpoint id is required")
	ErrEndpointExists     = errors.New("endpoint already exists")
	ErrEndpointNotFound   = errors.New("endpoint not found")
	ErrBaseURLRequired    = errors.New("endpoint base url is required")
	ErrInvalidBaseURL     = errors.New("endpoint base url must be absolute")
	ErrInvalidAuthType    = errors.New("invalid auth type")
	ErrConfigPathNotSet   = errors.New("registry config path not set")
	ErrEndpointDisabled   = errors.New("endpoint is disabled")
)

type Registry struct {
	mutex      sync.RWMutex
	endpoints  map[string]Endpoint
	statuses   map[string]PluginStatus
	configPath string
}

func NewRegistry() *Registry {
	return &Registry{
		endpoints: map[string]Endpoint{},
		statuses:  map[string]PluginStatus{},
	}
}

func (r *Registry) Register(endpoint Endpoint) error {
	normalized, err := normalizeEndpoint(endpoint)
	if err != nil {
		return err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, found := r.endpoints[normalized.ID]; found {
		return fmt.Errorf("%w: %q", ErrEndpointExists, normalized.ID)
	}

	r.endpoints[normalized.ID] = normalized
	r.statuses[normalized.ID] = initialStatusForEndpoint(normalized)
	return nil
}

func (r *Registry) Upsert(endpoint Endpoint) error {
	normalized, err := normalizeEndpoint(endpoint)
	if err != nil {
		return err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.endpoints[normalized.ID] = normalized
	if _, found := r.statuses[normalized.ID]; !found {
		r.statuses[normalized.ID] = initialStatusForEndpoint(normalized)
	}
	if !normalized.Enabled {
		r.statuses[normalized.ID] = initialStatusForEndpoint(normalized)
	}
	return nil
}

func (r *Registry) Unregister(endpointID string) {
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.endpoints, endpointID)
	delete(r.statuses, endpointID)
}

func (r *Registry) Get(endpointID string) (Endpoint, error) {
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return Endpoint{}, ErrEndpointIDRequired
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	endpoint, found := r.endpoints[endpointID]
	if !found {
		return Endpoint{}, fmt.Errorf("%w: %q", ErrEndpointNotFound, endpointID)
	}

	return endpoint, nil
}

func (r *Registry) List() []Endpoint {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if len(r.endpoints) == 0 {
		return []Endpoint{}
	}

	keys := make([]string, 0, len(r.endpoints))
	for key := range r.endpoints {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]Endpoint, 0, len(keys))
	for _, key := range keys {
		result = append(result, r.endpoints[key])
	}
	return result
}

func (r *Registry) ConfigPath() string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.configPath
}

func (r *Registry) LoadConfigFile(configPath string) error {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ErrConfigPathNotSet
	}

	config, err := ReadConfigFile(configPath)
	if err != nil {
		return err
	}
	if err := r.LoadConfig(config); err != nil {
		return err
	}

	r.mutex.Lock()
	r.configPath = configPath
	r.mutex.Unlock()
	return nil
}

func (r *Registry) ReloadConfigFile() error {
	r.mutex.RLock()
	configPath := r.configPath
	r.mutex.RUnlock()
	if strings.TrimSpace(configPath) == "" {
		return ErrConfigPathNotSet
	}
	return r.LoadConfigFile(configPath)
}

func (r *Registry) LoadConfig(config ConfigFile) error {
	nextEndpoints := map[string]Endpoint{}
	nextStatuses := map[string]PluginStatus{}

	for _, plugin := range config.Plugins {
		endpoint := Endpoint{
			ID:       strings.TrimSpace(plugin.Name),
			Name:     strings.TrimSpace(plugin.Name),
			BaseURL:  strings.TrimSpace(plugin.URI),
			AuthType: plugin.AuthType,
			Enabled:  plugin.Enabled,
			Metadata: map[string]string{},
		}

		normalized, err := normalizeEndpoint(endpoint)
		if err != nil {
			return fmt.Errorf("plugin %q: %w", plugin.Name, err)
		}

		if _, found := nextEndpoints[normalized.ID]; found {
			return fmt.Errorf("%w: %q", ErrEndpointExists, normalized.ID)
		}
		nextEndpoints[normalized.ID] = normalized
		nextStatuses[normalized.ID] = initialStatusForEndpoint(normalized)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.endpoints = nextEndpoints
	r.statuses = nextStatuses
	return nil
}

func (r *Registry) Status(endpointID string) (PluginStatus, error) {
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return PluginStatus{}, ErrEndpointIDRequired
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	status, found := r.statuses[endpointID]
	if !found {
		return PluginStatus{}, fmt.Errorf("%w: %q", ErrEndpointNotFound, endpointID)
	}
	return status, nil
}

func (r *Registry) Statuses() []PluginStatus {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if len(r.statuses) == 0 {
		return []PluginStatus{}
	}

	keys := make([]string, 0, len(r.statuses))
	for key := range r.statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]PluginStatus, 0, len(keys))
	for _, key := range keys {
		result = append(result, r.statuses[key])
	}
	return result
}

func (r *Registry) Invocation(endpointID string, operation Operation) (EndpointInvocation, error) {
	endpoint, err := r.Get(endpointID)
	if err != nil {
		return EndpointInvocation{}, err
	}

	method, requiredPathParams, requiredQueryParams, err := operationDetails(operation)
	if err != nil {
		return EndpointInvocation{}, err
	}

	urlTemplate, err := composeEndpointURL(endpoint.BaseURL, endpoint.routeFor(operation))
	if err != nil {
		return EndpointInvocation{}, err
	}
	urlTemplate = strings.ReplaceAll(urlTemplate, "%7B", "{")
	urlTemplate = strings.ReplaceAll(urlTemplate, "%7D", "}")

	return EndpointInvocation{
		EndpointID:              endpoint.ID,
		EndpointName:            endpoint.Name,
		Operation:               operation,
		Method:                  method,
		URLTemplate:             urlTemplate,
		RequiredPathParameters:  requiredPathParams,
		RequiredQueryParameters: requiredQueryParams,
		AuthType:                endpoint.AuthType,
		Enabled:                 endpoint.Enabled,
	}, nil
}

func (r *Registry) ListInvocations(endpointID string) ([]EndpointInvocation, error) {
	operations := []Operation{OperationList, OperationProperties, OperationSearch}
	result := make([]EndpointInvocation, 0, len(operations))
	for _, operation := range operations {
		invocation, err := r.Invocation(endpointID, operation)
		if err != nil {
			return nil, err
		}
		result = append(result, invocation)
	}
	return result, nil
}

func (r *Registry) recordHealth(endpointID string, statusCode int, latency time.Duration, requestErr error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	endpoint, found := r.endpoints[endpointID]
	if !found {
		return
	}

	status := r.statuses[endpointID]
	if endpoint.Enabled {
		now := time.Now()
		status.LastCheckedAt = &now
		status.LastStatusCode = statusCode
		status.Latency = latency
		if requestErr != nil {
			status.State = PluginStateUnhealthy
			status.LastError = requestErr.Error()
		} else {
			status.State = PluginStateHealthy
			status.LastError = ""
		}
	} else {
		status = initialStatusForEndpoint(endpoint)
	}

	r.statuses[endpointID] = status
}

func normalizeEndpoint(endpoint Endpoint) (Endpoint, error) {
	endpoint.ID = strings.TrimSpace(endpoint.ID)
	endpoint.Name = strings.TrimSpace(endpoint.Name)
	endpoint.BaseURL = strings.TrimSpace(endpoint.BaseURL)

	if endpoint.ID == "" {
		return Endpoint{}, ErrEndpointIDRequired
	}
	if endpoint.Name == "" {
		endpoint.Name = endpoint.ID
	}
	if endpoint.BaseURL == "" {
		return Endpoint{}, ErrBaseURLRequired
	}

	parsed, err := url.Parse(endpoint.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Endpoint{}, fmt.Errorf("%w: %q", ErrInvalidBaseURL, endpoint.BaseURL)
	}

	if endpoint.AuthType == "" {
		endpoint.AuthType = AuthTypeNone
	}
	if !endpoint.AuthType.Valid() {
		return Endpoint{}, fmt.Errorf("%w: %q", ErrInvalidAuthType, endpoint.AuthType)
	}

	endpoint.BaseURL = strings.TrimRight(parsed.String(), "/")
	endpoint.Routes = endpoint.Routes.withDefaults()
	if endpoint.Metadata == nil {
		endpoint.Metadata = map[string]string{}
	}

	return endpoint, nil
}

func initialStatusForEndpoint(endpoint Endpoint) PluginStatus {
	if endpoint.Enabled {
		return PluginStatus{
			EndpointID: endpoint.ID,
			State:      PluginStateUnknown,
		}
	}
	return PluginStatus{
		EndpointID: endpoint.ID,
		State:      PluginStateDisabled,
	}
}

func operationDetails(operation Operation) (method string, requiredPathParams []string, requiredQueryParams []string, err error) {
	switch operation {
	case OperationList:
		return http.MethodGet, nil, nil, nil
	case OperationProperties:
		return http.MethodGet, []string{"index_name"}, nil, nil
	case OperationSearch:
		return http.MethodPost, nil, []string{"index_name", "search_query"}, nil
	default:
		return "", nil, nil, fmt.Errorf("unsupported operation %q", operation)
	}
}
