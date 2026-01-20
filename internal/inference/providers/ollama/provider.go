// Package ollama provides an inference provider for Ollama.
// Ollama runs local LLMs and exposes an OpenAI-compatible API.
package ollama

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
	defaultBaseURL = "http://localhost:11434/v1"
	defaultTimeout = 60 * time.Second
)

// Provider implements inference.Provider for Ollama.
type Provider struct {
	name       string
	baseURL    string
	httpClient *http.Client
	models     []string
	closed     bool
	mu         sync.RWMutex
}

// NewProvider creates a new Ollama provider.
func NewProvider(name string, cfg inference.ProviderConfig) (*Provider, error) {
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

	// Convert to Ollama request format
	ollamaReq := convertRequest(req)
	ollamaReq.Stream = false

	body, err := json.Marshal(ollamaReq)
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
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, p.handleErrorResponse(resp)
	}

	var result ollamaCompletionResponse
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

	// Convert to Ollama request format
	ollamaReq := convertRequest(req)
	ollamaReq.Stream = true

	body, err := json.Marshal(ollamaReq)
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
	httpReq.Header.Set("Content-Type", "application/json")
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
				OwnedBy:  "ollama",
				Provider: p.name,
			}
		}
		return models, nil
	}

	// Otherwise, query Ollama for available models
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, p.handleErrorResponse(resp)
	}

	var result ollamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	models := make([]inference.Model, len(result.Data))
	for i, m := range result.Data {
		models[i] = inference.Model{
			ID:       m.ID,
			Object:   m.Object,
			Created:  m.Created,
			OwnedBy:  m.OwnedBy,
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

// handleErrorResponse extracts error information from a non-200 response.
func (p *Provider) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp ollamaErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		return fmt.Errorf("ollama error (%d): %s", resp.StatusCode, errResp.Error.Message)
	}

	return fmt.Errorf("ollama error (%d): %s", resp.StatusCode, string(body))
}

// Ollama API types

type ollamaCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []ollamaMessage `json:"messages"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream"`
	Stop        []string        `json:"stop,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaCompletionResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
	Choices           []ollamaChoice `json:"choices"`
	Usage             *ollamaUsage   `json:"usage,omitempty"`
}

type ollamaChoice struct {
	Index        int            `json:"index"`
	Message      *ollamaMessage `json:"message,omitempty"`
	Delta        *ollamaMessage `json:"delta,omitempty"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

type ollamaUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ollamaModelsResponse struct {
	Object string        `json:"object"`
	Data   []ollamaModel `json:"data"`
}

type ollamaModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ollamaErrorResponse struct {
	Error *ollamaError `json:"error"`
}

type ollamaError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Conversion helpers

func convertRequest(req *inference.CompletionRequest) *ollamaCompletionRequest {
	messages := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ollamaMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	return &ollamaCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
	}
}

func convertResponse(resp *ollamaCompletionResponse) *inference.CompletionResponse {
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

// streamReader reads SSE stream from Ollama.
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

		var chunk ollamaCompletionResponse
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

func convertChunk(chunk *ollamaCompletionResponse) *inference.CompletionChunk {
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
