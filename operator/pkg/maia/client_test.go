package maia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8080")
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL to be http://localhost:8080, got %s", client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be non-nil")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	client := NewClient("http://localhost:8080",
		WithAPIKey("test-api-key"),
		WithTimeout(60*time.Second),
	)

	if client.apiKey != "test-api-key" {
		t.Errorf("expected apiKey to be test-api-key, got %s", client.apiKey)
	}
	if client.httpClient.Timeout != 60*time.Second {
		t.Errorf("expected timeout to be 60s, got %v", client.httpClient.Timeout)
	}
}

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Health(context.Background())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestHealthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Health(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestCreateTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/tenants" {
			t.Errorf("expected path /admin/tenants, got %s", r.URL.Path)
		}

		var req CreateTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Name != "test-tenant" {
			t.Errorf("expected name test-tenant, got %s", req.Name)
		}

		tenant := Tenant{
			ID:        "tenant-123",
			Name:      req.Name,
			Plan:      req.Plan,
			Status:    "active",
			CreatedAt: time.Now(),
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tenant)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	tenant, err := client.CreateTenant(context.Background(), &CreateTenantRequest{
		Name: "test-tenant",
		Plan: "free",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if tenant.ID != "tenant-123" {
		t.Errorf("expected tenant ID tenant-123, got %s", tenant.ID)
	}
}

func TestGetTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/tenants/tenant-123" {
			t.Errorf("expected path /admin/tenants/tenant-123, got %s", r.URL.Path)
		}

		tenant := Tenant{
			ID:        "tenant-123",
			Name:      "test-tenant",
			Plan:      "free",
			Status:    "active",
			CreatedAt: time.Now(),
		}
		json.NewEncoder(w).Encode(tenant)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	tenant, err := client.GetTenant(context.Background(), "tenant-123")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if tenant == nil {
		t.Error("expected tenant, got nil")
	}
	if tenant.ID != "tenant-123" {
		t.Errorf("expected tenant ID tenant-123, got %s", tenant.ID)
	}
}

func TestGetTenantNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	tenant, err := client.GetTenant(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("expected no error for not found, got %v", err)
	}
	if tenant != nil {
		t.Error("expected nil tenant for not found")
	}
}

func TestUpdateTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		var req UpdateTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		tenant := Tenant{
			ID:        "tenant-123",
			Name:      "test-tenant",
			Plan:      req.Plan,
			Status:    "active",
			UpdatedAt: time.Now(),
		}
		json.NewEncoder(w).Encode(tenant)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	tenant, err := client.UpdateTenant(context.Background(), "tenant-123", &UpdateTenantRequest{
		Plan: "premium",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if tenant.Plan != "premium" {
		t.Errorf("expected plan premium, got %s", tenant.Plan)
	}
}

func TestDeleteTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.DeleteTenant(context.Background(), "tenant-123")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestGetTenantUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		usage := TenantUsage{
			MemoryCount:    100,
			NamespaceCount: 5,
			StorageBytes:   1024000,
			RequestsToday:  500,
		}
		json.NewEncoder(w).Encode(usage)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	usage, err := client.GetTenantUsage(context.Background(), "tenant-123")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if usage.MemoryCount != 100 {
		t.Errorf("expected memory count 100, got %d", usage.MemoryCount)
	}
}

func TestSuspendTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/tenants/tenant-123/suspend" {
			t.Errorf("expected path /admin/tenants/tenant-123/suspend, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.SuspendTenant(context.Background(), "tenant-123", "payment overdue")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestActivateTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/tenants/tenant-123/activate" {
			t.Errorf("expected path /admin/tenants/tenant-123/activate, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.ActivateTenant(context.Background(), "tenant-123")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCreateAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req CreateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		resp := CreateAPIKeyResponse{
			APIKey: &APIKey{
				Key:       "hash123",
				TenantID:  "tenant-123",
				Name:      req.Name,
				Scopes:    req.Scopes,
				CreatedAt: time.Now(),
			},
			Key: "maia_raw_api_key_12345",
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.CreateAPIKey(context.Background(), "tenant-123", &CreateAPIKeyRequest{
		Name:   "test-key",
		Scopes: []string{"read", "write"},
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.Key != "maia_raw_api_key_12345" {
		t.Errorf("expected raw key, got %s", resp.Key)
	}
}

func TestListAPIKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		resp := struct {
			APIKeys []APIKey `json:"api_keys"`
		}{
			APIKeys: []APIKey{
				{Key: "hash1", Name: "key1", TenantID: "tenant-123"},
				{Key: "hash2", Name: "key2", TenantID: "tenant-123"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	keys, err := client.ListAPIKeys(context.Background(), "tenant-123")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestRevokeAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.RevokeAPIKey(context.Background(), "key-hash")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIError{
			Code:    "INVALID_INPUT",
			Message: "Name is required",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.CreateTenant(context.Background(), &CreateTenantRequest{})
	if err == nil {
		t.Error("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Errorf("expected APIError, got %T", err)
	}
	if apiErr.Code != "INVALID_INPUT" {
		t.Errorf("expected code INVALID_INPUT, got %s", apiErr.Code)
	}
}

func TestAPIKeyAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "test-api-key" {
			t.Errorf("expected X-API-Key header to be test-api-key, got %s", apiKey)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, WithAPIKey("test-api-key"))
	err := client.Health(context.Background())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
