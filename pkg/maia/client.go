package maia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// DefaultTimeout is the default HTTP client timeout.
	DefaultTimeout = 30 * time.Second
	// DefaultBaseURL is the default MAIA server URL.
	DefaultBaseURL = "http://localhost:8080"
)

// Client is the MAIA SDK client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// ClientOption is a function that configures a Client.
type ClientOption func(*Client)

// WithBaseURL sets the base URL for the client.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithHeader adds a custom header to all requests.
func WithHeader(key, value string) ClientOption {
	return func(c *Client) {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers[key] = value
	}
}

// New creates a new MAIA client with the given options.
func New(opts ...ClientOption) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		headers: make(map[string]string),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// do performs an HTTP request and decodes the response.
func (c *Client) do(ctx context.Context, method, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		apiErr.StatusCode = resp.StatusCode
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return &apiErr
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("server error: %s", string(respBody)),
		}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Health checks if the server is healthy.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.do(ctx, http.MethodGet, "/health", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Ready checks if the server is ready to serve requests.
func (c *Client) Ready(ctx context.Context) error {
	var resp struct {
		Status string `json:"status"`
	}
	return c.do(ctx, http.MethodGet, "/ready", nil, &resp)
}

// Stats returns storage statistics.
func (c *Client) Stats(ctx context.Context) (*Stats, error) {
	var stats Stats
	if err := c.do(ctx, http.MethodGet, "/v1/stats", nil, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// Memory operations

// CreateMemory creates a new memory.
func (c *Client) CreateMemory(ctx context.Context, input *CreateMemoryInput) (*Memory, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if input.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	var mem Memory
	if err := c.do(ctx, http.MethodPost, "/v1/memories", input, &mem); err != nil {
		return nil, err
	}
	return &mem, nil
}

// GetMemory retrieves a memory by ID.
func (c *Client) GetMemory(ctx context.Context, id string) (*Memory, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	var mem Memory
	if err := c.do(ctx, http.MethodGet, "/v1/memories/"+url.PathEscape(id), nil, &mem); err != nil {
		return nil, err
	}
	return &mem, nil
}

// UpdateMemory updates an existing memory.
func (c *Client) UpdateMemory(ctx context.Context, id string, input *UpdateMemoryInput) (*Memory, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}

	var mem Memory
	if err := c.do(ctx, http.MethodPut, "/v1/memories/"+url.PathEscape(id), input, &mem); err != nil {
		return nil, err
	}
	return &mem, nil
}

// DeleteMemory deletes a memory by ID.
func (c *Client) DeleteMemory(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	var resp DeleteResponse
	return c.do(ctx, http.MethodDelete, "/v1/memories/"+url.PathEscape(id), nil, &resp)
}

// SearchMemories searches for memories based on the given criteria.
func (c *Client) SearchMemories(ctx context.Context, input *SearchMemoriesInput) (*ListResponse[*SearchResult], error) {
	if input == nil {
		input = &SearchMemoriesInput{}
	}

	var resp ListResponse[*SearchResult]
	if err := c.do(ctx, http.MethodPost, "/v1/memories/search", input, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Namespace operations

// CreateNamespace creates a new namespace.
func (c *Client) CreateNamespace(ctx context.Context, input *CreateNamespaceInput) (*Namespace, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	var ns Namespace
	if err := c.do(ctx, http.MethodPost, "/v1/namespaces", input, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

// GetNamespace retrieves a namespace by ID or name.
func (c *Client) GetNamespace(ctx context.Context, idOrName string) (*Namespace, error) {
	if idOrName == "" {
		return nil, fmt.Errorf("id or name is required")
	}

	var ns Namespace
	if err := c.do(ctx, http.MethodGet, "/v1/namespaces/"+url.PathEscape(idOrName), nil, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

// UpdateNamespace updates a namespace configuration.
func (c *Client) UpdateNamespace(ctx context.Context, id string, input *UpdateNamespaceInput) (*Namespace, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}

	var ns Namespace
	if err := c.do(ctx, http.MethodPut, "/v1/namespaces/"+url.PathEscape(id), input, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

// DeleteNamespace deletes a namespace by ID.
func (c *Client) DeleteNamespace(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	var resp DeleteResponse
	return c.do(ctx, http.MethodDelete, "/v1/namespaces/"+url.PathEscape(id), nil, &resp)
}

// ListNamespaces lists all namespaces.
func (c *Client) ListNamespaces(ctx context.Context, opts *ListOptions) (*ListResponse[*Namespace], error) {
	path := "/v1/namespaces"
	if opts != nil {
		params := url.Values{}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	var resp ListResponse[*Namespace]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListNamespaceMemories lists memories in a namespace.
func (c *Client) ListNamespaceMemories(ctx context.Context, namespace string, opts *ListOptions) (*ListResponse[*Memory], error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	path := "/v1/namespaces/" + url.PathEscape(namespace) + "/memories"
	if opts != nil {
		params := url.Values{}
		if opts.Limit > 0 {
			params.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", strconv.Itoa(opts.Offset))
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	var resp ListResponse[*Memory]
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Context operations

// GetContext assembles context for a query.
func (c *Client) GetContext(ctx context.Context, input *GetContextInput) (*ContextResponse, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	var resp ContextResponse
	if err := c.do(ctx, http.MethodPost, "/v1/context", input, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Convenience methods

// Remember is a convenience method to create a semantic memory.
func (c *Client) Remember(ctx context.Context, namespace, content string) (*Memory, error) {
	return c.CreateMemory(ctx, &CreateMemoryInput{
		Namespace:  namespace,
		Content:    content,
		Type:       MemoryTypeSemantic,
		Source:     MemorySourceUser,
		Confidence: 1.0,
	})
}

// Recall is a convenience method to get context for a query.
func (c *Client) Recall(ctx context.Context, query string, opts ...RecallOption) (*ContextResponse, error) {
	input := &GetContextInput{
		Query: query,
	}

	for _, opt := range opts {
		opt(input)
	}

	return c.GetContext(ctx, input)
}

// RecallOption is an option for the Recall method.
type RecallOption func(*GetContextInput)

// WithNamespace sets the namespace for recall.
func WithNamespace(namespace string) RecallOption {
	return func(input *GetContextInput) {
		input.Namespace = namespace
	}
}

// WithTokenBudget sets the token budget for recall.
func WithTokenBudget(budget int) RecallOption {
	return func(input *GetContextInput) {
		input.TokenBudget = budget
	}
}

// WithSystemPrompt sets the system prompt for recall.
func WithSystemPrompt(prompt string) RecallOption {
	return func(input *GetContextInput) {
		input.SystemPrompt = prompt
	}
}

// WithMinScore sets the minimum score for recall.
func WithMinScore(score float64) RecallOption {
	return func(input *GetContextInput) {
		input.MinScore = score
	}
}

// WithScores includes scores in the recall response.
func WithScores() RecallOption {
	return func(input *GetContextInput) {
		input.IncludeScores = true
	}
}

// Forget deletes a memory by ID.
func (c *Client) Forget(ctx context.Context, id string) error {
	return c.DeleteMemory(ctx, id)
}
