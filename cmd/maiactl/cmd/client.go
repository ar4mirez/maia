package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client provides HTTP client functionality for the CLI.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Error represents an API error response.
type Error struct {
	Message string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	return e.Message
}

// Do performs an HTTP request and decodes the response.
func (c *Client) Do(method, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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
		var apiErr Error
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return &apiErr
		}
		return fmt.Errorf("server error: %s (status %d)", string(respBody), resp.StatusCode)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request.
func (c *Client) Get(path string, result interface{}) error {
	return c.Do(http.MethodGet, path, nil, result)
}

// Post performs a POST request.
func (c *Client) Post(path string, body, result interface{}) error {
	return c.Do(http.MethodPost, path, body, result)
}

// Put performs a PUT request.
func (c *Client) Put(path string, body, result interface{}) error {
	return c.Do(http.MethodPut, path, body, result)
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string, result interface{}) error {
	return c.Do(http.MethodDelete, path, nil, result)
}
