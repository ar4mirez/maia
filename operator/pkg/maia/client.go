// Package maia provides a client for the MAIA Admin API.
package maia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the MAIA Admin API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// ClientOption configures the client.
type ClientOption func(*Client)

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// NewClient creates a new MAIA Admin API client.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Tenant represents a MAIA tenant.
type Tenant struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Plan      string            `json:"plan"`
	Status    string            `json:"status"`
	Config    *TenantConfig     `json:"config,omitempty"`
	Quotas    *TenantQuotas     `json:"quotas,omitempty"`
	Usage     *TenantUsage      `json:"usage,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// TenantConfig represents tenant configuration.
type TenantConfig struct {
	DefaultTokenBudget     int      `json:"default_token_budget,omitempty"`
	MaxTokenBudget         int      `json:"max_token_budget,omitempty"`
	AllowedEmbeddingModels []string `json:"allowed_embedding_models,omitempty"`
	DedicatedStorage       bool     `json:"dedicated_storage,omitempty"`
}

// TenantQuotas represents tenant quotas.
type TenantQuotas struct {
	MaxMemories       int64 `json:"max_memories,omitempty"`
	MaxStorageBytes   int64 `json:"max_storage_bytes,omitempty"`
	MaxNamespaces     int   `json:"max_namespaces,omitempty"`
	RequestsPerMinute int   `json:"requests_per_minute,omitempty"`
	RequestsPerDay    int64 `json:"requests_per_day,omitempty"`
}

// TenantUsage represents tenant usage.
type TenantUsage struct {
	MemoryCount    int   `json:"memory_count"`
	NamespaceCount int   `json:"namespace_count"`
	StorageBytes   int64 `json:"storage_bytes"`
	RequestsToday  int64 `json:"requests_today"`
}

// CreateTenantRequest is the request to create a tenant.
type CreateTenantRequest struct {
	Name     string            `json:"name"`
	Plan     string            `json:"plan,omitempty"`
	Config   *TenantConfig     `json:"config,omitempty"`
	Quotas   *TenantQuotas     `json:"quotas,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// UpdateTenantRequest is the request to update a tenant.
type UpdateTenantRequest struct {
	Plan     string            `json:"plan,omitempty"`
	Config   *TenantConfig     `json:"config,omitempty"`
	Quotas   *TenantQuotas     `json:"quotas,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// APIKey represents a MAIA API key.
type APIKey struct {
	Key        string     `json:"key"`
	TenantID   string     `json:"tenant_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// CreateAPIKeyRequest is the request to create an API key.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKeyResponse is the response when creating an API key.
type CreateAPIKeyResponse struct {
	APIKey *APIKey `json:"api_key"`
	Key    string  `json:"key"` // Raw key, only returned once
}

// APIError represents an API error response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Health checks the MAIA server health.
func (c *Client) Health(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}

// CreateTenant creates a new tenant.
func (c *Client) CreateTenant(ctx context.Context, req *CreateTenantRequest) (*Tenant, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/admin/tenants", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var tenant Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tenant, nil
}

// GetTenant retrieves a tenant by ID.
func (c *Client) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/admin/tenants/%s", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Tenant not found
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var tenant Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tenant, nil
}

// UpdateTenant updates a tenant.
func (c *Client) UpdateTenant(ctx context.Context, id string, req *UpdateTenantRequest) (*Tenant, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/admin/tenants/%s", id), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var tenant Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tenant, nil
}

// DeleteTenant deletes a tenant.
func (c *Client) DeleteTenant(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/admin/tenants/%s", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// GetTenantUsage retrieves tenant usage.
func (c *Client) GetTenantUsage(ctx context.Context, id string) (*TenantUsage, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/admin/tenants/%s/usage", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var usage TenantUsage
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &usage, nil
}

// SuspendTenant suspends a tenant.
func (c *Client) SuspendTenant(ctx context.Context, id string, reason string) error {
	body := map[string]string{"reason": reason}
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/admin/tenants/%s/suspend", id), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// ActivateTenant activates a suspended tenant.
func (c *Client) ActivateTenant(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/admin/tenants/%s/activate", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// CreateAPIKey creates a new API key for a tenant.
func (c *Client) CreateAPIKey(ctx context.Context, tenantID string, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/admin/tenants/%s/apikeys", tenantID), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var response CreateAPIKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// ListAPIKeys lists API keys for a tenant.
func (c *Client) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/admin/tenants/%s/apikeys", tenantID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var response struct {
		APIKeys []APIKey `json:"api_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.APIKeys, nil
}

// RevokeAPIKey revokes an API key.
func (c *Client) RevokeAPIKey(ctx context.Context, key string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/admin/apikeys/%s", key), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	return c.httpClient.Do(req)
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		return &apiErr
	}

	return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
}
