package searchplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultUserAgent = "go-irodsclient-extensions/searchplugin"

var (
	ErrIndexNameRequired   = errors.New("index_name is required")
	ErrSearchQueryRequired = errors.New("search_query is required")
)

type HTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

type Client struct {
	registry   *Registry
	httpClient HTTPClient
	authorizer RequestAuthorizer
	userAgent  string
}

type ClientOption func(client *Client)

func WithHTTPClient(httpClient HTTPClient) ClientOption {
	return func(client *Client) {
		client.httpClient = httpClient
	}
}

func WithRequestAuthorizer(authorizer RequestAuthorizer) ClientOption {
	return func(client *Client) {
		client.authorizer = authorizer
	}
}

func WithUserAgent(userAgent string) ClientOption {
	return func(client *Client) {
		client.userAgent = strings.TrimSpace(userAgent)
	}
}

func NewClient(registry *Registry, options ...ClientOption) *Client {
	client := &Client{
		registry:   registry,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		authorizer: ByEndpointAuthTypeAuthorizer(),
		userAgent:  defaultUserAgent,
	}

	for _, option := range options {
		option(client)
	}

	if client.authorizer == nil {
		client.authorizer = ByEndpointAuthTypeAuthorizer()
	}
	if strings.TrimSpace(client.userAgent) == "" {
		client.userAgent = defaultUserAgent
	}

	return client
}

func (client *Client) List(ctx context.Context, endpointID string, authContext RequestAuthContext) (*CapturedResponse, error) {
	return client.perform(ctx, endpointID, OperationList, http.MethodGet, "", nil, nil, authContext)
}

func (client *Client) Properties(ctx context.Context, endpointID string, indexName string, authContext RequestAuthContext) (*CapturedResponse, error) {
	pathParams := map[string]string{
		"index_name": strings.TrimSpace(indexName),
	}
	return client.perform(ctx, endpointID, OperationProperties, http.MethodGet, "", pathParams, nil, authContext)
}

func (client *Client) Search(ctx context.Context, endpointID string, request SearchRequest, authContext RequestAuthContext) (*CapturedResponse, error) {
	if strings.TrimSpace(request.IndexName) == "" {
		return nil, ErrIndexNameRequired
	}
	if strings.TrimSpace(request.SearchQuery) == "" {
		return nil, ErrSearchQueryRequired
	}

	queryParams := map[string]string{
		"index_name":   strings.TrimSpace(request.IndexName),
		"search_query": strings.TrimSpace(request.SearchQuery),
	}
	for key, value := range request.Parameters {
		queryParams[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	var body []byte
	if len(request.Raw) > 0 {
		body = request.Raw
		if !json.Valid(body) {
			return nil, fmt.Errorf("search raw payload is not valid json")
		}
	}

	return client.perform(ctx, endpointID, OperationSearch, http.MethodPost, "", nil, requestWithBody(queryParams, body), authContext)
}

func requestWithBody(queryParams map[string]string, body []byte) map[string]any {
	return map[string]any{
		"query": queryParams,
		"body":  body,
	}
}

func (client *Client) SearchAll(ctx context.Context, request SearchRequest, authContextByEndpoint map[string]RequestAuthContext) []FederatedSearchResult {
	endpoints := client.registry.List()
	if len(endpoints) == 0 {
		return []FederatedSearchResult{}
	}

	results := make([]FederatedSearchResult, 0, len(endpoints))
	for _, endpoint := range endpoints {
		authContext, ok := authContextByEndpoint[endpoint.ID]
		if !ok {
			authContext = RequestAuthContext{}
		}

		response, err := client.Search(ctx, endpoint.ID, request, authContext)
		result := FederatedSearchResult{
			EndpointID: endpoint.ID,
			Response:   response,
		}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	return results
}

func (client *Client) Ping(ctx context.Context, endpointID string, authContext RequestAuthContext) (PluginStatus, error) {
	_, err := client.List(ctx, endpointID, authContext)
	status, statusErr := client.registry.Status(endpointID)
	if statusErr != nil {
		return PluginStatus{}, statusErr
	}
	return status, err
}

func (client *Client) perform(ctx context.Context, endpointID string, operation Operation, method string, operationPath string, pathParams map[string]string, requestShape map[string]any, authContext RequestAuthContext) (*CapturedResponse, error) {
	endpoint, err := client.registry.Get(endpointID)
	if err != nil {
		return nil, err
	}
	if !endpoint.Enabled {
		client.registry.recordHealth(endpoint.ID, 0, 0, ErrEndpointDisabled)
		return nil, fmt.Errorf("%w: %q", ErrEndpointDisabled, endpoint.ID)
	}

	if operationPath == "" {
		operationPath = endpoint.routeFor(operation)
	}
	operationPath = resolvePathParams(operationPath, pathParams)

	requestURL, err := composeEndpointURL(endpoint.BaseURL, operationPath)
	if err != nil {
		client.registry.recordHealth(endpoint.ID, 0, 0, err)
		return nil, err
	}

	queryParams, requestBody, err := extractRequestShape(requestShape)
	if err != nil {
		client.registry.recordHealth(endpoint.ID, 0, 0, err)
		return nil, err
	}
	requestURL, err = appendQueryParams(requestURL, queryParams)
	if err != nil {
		client.registry.recordHealth(endpoint.ID, 0, 0, err)
		return nil, err
	}

	var bodyReader io.Reader
	if len(requestBody) > 0 {
		bodyReader = bytes.NewReader(requestBody)
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		client.registry.recordHealth(endpoint.ID, 0, 0, err)
		return nil, err
	}

	request.Header.Set("Accept", "application/json")
	if client.userAgent != "" {
		request.Header.Set("User-Agent", client.userAgent)
	}
	if len(requestBody) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}

	if err := client.authorizer.Authorize(request, endpoint, authContext); err != nil {
		client.registry.recordHealth(endpoint.ID, 0, 0, err)
		return nil, err
	}

	startedAt := time.Now()
	response, err := client.httpClient.Do(request)
	if err != nil {
		client.registry.recordHealth(endpoint.ID, 0, time.Since(startedAt), err)
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		client.registry.recordHealth(endpoint.ID, response.StatusCode, time.Since(startedAt), err)
		return nil, err
	}

	captured := &CapturedResponse{
		EndpointID: endpoint.ID,
		Operation:  operation,
		StatusCode: response.StatusCode,
		Headers:    response.Header.Clone(),
		Body:       json.RawMessage(body),
		ReceivedAt: time.Now(),
		Duration:   time.Since(startedAt),
	}

	if response.StatusCode >= http.StatusBadRequest {
		statusErr := HTTPStatusError{
			EndpointID: endpoint.ID,
			Operation:  operation,
			StatusCode: response.StatusCode,
			Body:       string(body),
		}
		client.registry.recordHealth(endpoint.ID, response.StatusCode, captured.Duration, statusErr)
		return captured, statusErr
	}

	client.registry.recordHealth(endpoint.ID, response.StatusCode, captured.Duration, nil)
	return captured, nil
}

func resolvePathParams(operationPath string, pathParams map[string]string) string {
	path := operationPath
	for key, value := range pathParams {
		path = strings.ReplaceAll(path, "{"+strings.TrimSpace(key)+"}", url.PathEscape(strings.TrimSpace(value)))
	}
	return path
}

func extractRequestShape(shape map[string]any) (map[string]string, []byte, error) {
	if shape == nil {
		return nil, nil, nil
	}

	queryParams := map[string]string{}
	queryAny, ok := shape["query"]
	if ok {
		typed, ok := queryAny.(map[string]string)
		if !ok {
			return nil, nil, fmt.Errorf("query shape must be map[string]string")
		}
		for key, value := range typed {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" {
				continue
			}
			queryParams[key] = value
		}
	}

	var body []byte
	bodyAny, ok := shape["body"]
	if ok {
		typed, ok := bodyAny.([]byte)
		if !ok {
			return nil, nil, fmt.Errorf("body shape must be []byte")
		}
		body = typed
	}
	return queryParams, body, nil
}

func appendQueryParams(baseURL string, queryParams map[string]string) (string, error) {
	if len(queryParams) == 0 {
		return baseURL, nil
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for key, value := range queryParams {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func composeEndpointURL(base string, relativePath string) (string, error) {
	base = strings.TrimSpace(base)
	relativePath = strings.TrimSpace(relativePath)
	if base == "" {
		return "", ErrBaseURLRequired
	}
	if relativePath == "" {
		return "", fmt.Errorf("operation path is required")
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	pathURL, err := url.Parse(relativePath)
	if err != nil {
		return "", err
	}

	return baseURL.ResolveReference(pathURL).String(), nil
}

type HTTPStatusError struct {
	EndpointID string
	Operation  Operation
	StatusCode int
	Body       string
}

func (err HTTPStatusError) Error() string {
	return fmt.Sprintf("search plugin request failed: endpoint=%q operation=%q status=%d body=%q", err.EndpointID, err.Operation, err.StatusCode, strings.TrimSpace(err.Body))
}
