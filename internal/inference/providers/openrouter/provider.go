// Package openrouter provides an inference provider for OpenRouter.
// OpenRouter is an API gateway that provides access to 100+ LLM models
// through a single, OpenAI-compatible API.
package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ar4mirez/maia/internal/inference"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	defaultTimeout = 120 * time.Second
)

// Provider implements inference.Provider for OpenRouter.
type Provider struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
	models     []string
	closed     bool
	mu         sync.RWMutex
}

// NewProvider creates a new OpenRouter provider.
func NewProvider(name string, cfg inference.ProviderConfig) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenRouter API key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Provider{
		name:    name,
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		models: cfg.Models,
	}, nil
}

// Complete implements inference.Provider.Complete.
func (p *Provider) Complete(ctx context.Context, req *inference.CompletionRequest) (*inference.CompletionResponse, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, inference.ErrProviderClosed
	}
	p.mu.RUnlock()

	if len(req.Messages) == 0 {
		return nil, inference.ErrEmptyMessages
	}

	// Convert to OpenRouter request format
	orReq := convertRequest(req)
	orReq.Stream = false

	body, err := json.Marshal(orReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, p.handleErrorResponse(resp)
	}

	var result openRouterCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return convertResponse(&result), nil
}

// Stream implements inference.Provider.Stream.
func (p *Provider) Stream(ctx context.Context, req *inference.CompletionRequest) (inference.StreamReader, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, inference.ErrProviderClosed
	}
	p.mu.RUnlock()

	if len(req.Messages) == 0 {
		return nil, inference.ErrEmptyMessages
	}

	// Convert to OpenRouter request format
	orReq := convertRequest(req)
	orReq.Stream = true

	body, err := json.Marshal(orReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, p.handleErrorResponse(resp)
	}

	return &streamReader{
		reader: bufio.NewReader(resp.Body),
		resp:   resp,
	}, nil
}

// ListModels implements inference.Provider.ListModels.
func (p *Provider) ListModels(ctx context.Context) ([]inference.Model, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, inference.ErrProviderClosed
	}
	configuredModels := p.models
	p.mu.RUnlock()

	// If models are configured, return those
	if len(configuredModels) > 0 {
		models := make([]inference.Model, len(configuredModels))
		for i, modelID := range configuredModels {
			models[i] = inference.Model{
				ID:       modelID,
				Object:   "model",
				OwnedBy:  "openrouter",
				Provider: p.name,
			}
		}
		return models, nil
	}

	// Otherwise, query OpenRouter for available models
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, p.handleErrorResponse(resp)
	}

	var result openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	models := make([]inference.Model, len(result.Data))
	for i, m := range result.Data {
		models[i] = inference.Model{
			ID:       m.ID,
			Object:   "model",
			OwnedBy:  "openrouter",
			Provider: p.name,
		}
	}
	return models, nil
}

// Name implements inference.Provider.Name.
func (p *Provider) Name() string {
	return p.name
}

// SupportsModel implements inference.Provider.SupportsModel.
func (p *Provider) SupportsModel(modelID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// If no models configured, assume support for any model
	// OpenRouter supports 100+ models, so this is reasonable
	if len(p.models) == 0 {
		return true
	}

	for _, model := range p.models {
		if model == modelID {
			return true
		}
		// Support wildcard matching
		if strings.HasSuffix(model, "*") {
			prefix := strings.TrimSuffix(model, "*")
			if strings.HasPrefix(modelID, prefix) {
				return true
			}
		}
	}
	return false
}

// Health implements inference.Provider.Health.
func (p *Provider) Health(ctx context.Context) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return inference.ErrProviderClosed
	}
	p.mu.RUnlock()

	// Try to list models as a health check
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// Close implements inference.Provider.Close.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// setHeaders sets the required headers for OpenRouter API requests.
func (p *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/ar4mirez/maia")
	req.Header.Set("X-Title", "MAIA")
}

// handleErrorResponse extracts error information from a non-200 response.
func (p *Provider) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp openRouterErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		return fmt.Errorf("openrouter error (%d): %s - %s", resp.StatusCode, errResp.Error.Code, errResp.Error.Message)
	}

	return fmt.Errorf("openrouter error (%d): %s", resp.StatusCode, string(body))
}

// OpenRouter API types

type openRouterCompletionRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Temperature *float64            `json:"temperature,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Stream      bool                `json:"stream"`
	Stop        []string            `json:"stop,omitempty"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterCompletionResponse struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	Created           int64              `json:"created"`
	Model             string             `json:"model"`
	SystemFingerprint string             `json:"system_fingerprint,omitempty"`
	Choices           []openRouterChoice `json:"choices"`
	Usage             *openRouterUsage   `json:"usage,omitempty"`
}

type openRouterChoice struct {
	Index        int                `json:"index"`
	Message      *openRouterMessage `json:"message,omitempty"`
	Delta        *openRouterMessage `json:"delta,omitempty"`
	FinishReason string             `json:"finish_reason,omitempty"`
}

type openRouterUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type openRouterErrorResponse struct {
	Error *openRouterError `json:"error"`
}

type openRouterError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Conversion helpers

func convertRequest(req *inference.CompletionRequest) *openRouterCompletionRequest {
	messages := make([]openRouterMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openRouterMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	return &openRouterCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
	}
}

func convertResponse(resp *openRouterCompletionResponse) *inference.CompletionResponse {
	choices := make([]inference.Choice, len(resp.Choices))
	for i, c := range resp.Choices {
		var msg *inference.Message
		if c.Message != nil {
			msg = &inference.Message{
				Role:    c.Message.Role,
				Content: c.Message.Content,
			}
		}
		choices[i] = inference.Choice{
			Index:        c.Index,
			Message:      msg,
			FinishReason: c.FinishReason,
		}
	}

	var usage *inference.Usage
	if resp.Usage != nil {
		usage = &inference.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return &inference.CompletionResponse{
		ID:                resp.ID,
		Object:            resp.Object,
		Created:           resp.Created,
		Model:             resp.Model,
		SystemFingerprint: resp.SystemFingerprint,
		Choices:           choices,
		Usage:             usage,
	}
}

// streamReader reads SSE stream from OpenRouter.
type streamReader struct {
	reader *bufio.Reader
	resp   *http.Response
}

// Read implements inference.StreamReader.Read.
func (s *streamReader) Read() (*inference.CompletionChunk, error) {
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("read stream: %w", err)
		}

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle SSE format
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			return nil, io.EOF
		}

		var chunk openRouterCompletionResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("unmarshal chunk: %w", err)
		}

		return convertChunk(&chunk), nil
	}
}

// Close implements inference.StreamReader.Close.
func (s *streamReader) Close() error {
	return s.resp.Body.Close()
}

func convertChunk(chunk *openRouterCompletionResponse) *inference.CompletionChunk {
	choices := make([]inference.Choice, len(chunk.Choices))
	for i, c := range chunk.Choices {
		var delta *inference.Message
		if c.Delta != nil {
			delta = &inference.Message{
				Role:    c.Delta.Role,
				Content: c.Delta.Content,
			}
		}
		choices[i] = inference.Choice{
			Index:        c.Index,
			Delta:        delta,
			FinishReason: c.FinishReason,
		}
	}

	return &inference.CompletionChunk{
		ID:                chunk.ID,
		Object:            chunk.Object,
		Created:           chunk.Created,
		Model:             chunk.Model,
		SystemFingerprint: chunk.SystemFingerprint,
		Choices:           choices,
	}
}
