// Package anthropic provides an inference provider for Anthropic's Claude API.
// Anthropic offers Claude models with their own message API format.
package anthropic

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
	defaultBaseURL   = "https://api.anthropic.com/v1"
	defaultTimeout   = 120 * time.Second
	anthropicVersion = "2023-06-01"
)

// Provider implements inference.Provider for Anthropic.
type Provider struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
	models     []string
	closed     bool
	mu         sync.RWMutex
}

// NewProvider creates a new Anthropic provider.
func NewProvider(name string, cfg inference.ProviderConfig) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
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

	// Convert to Anthropic request format
	anthropicReq := convertRequest(req)
	anthropicReq.Stream = false

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/messages",
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

	var result anthropicMessageResponse
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

	// Convert to Anthropic request format
	anthropicReq := convertRequest(req)
	anthropicReq.Stream = true

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/messages",
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
		model:  req.Model,
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
				OwnedBy:  "anthropic",
				Provider: p.name,
			}
		}
		return models, nil
	}

	// Anthropic doesn't have a list models endpoint, return known models
	knownModels := []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}

	models := make([]inference.Model, len(knownModels))
	for i, modelID := range knownModels {
		models[i] = inference.Model{
			ID:       modelID,
			Object:   "model",
			OwnedBy:  "anthropic",
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

	// If models are configured, check against those
	if len(p.models) > 0 {
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

	// Default: support any model starting with "claude"
	return strings.HasPrefix(modelID, "claude")
}

// Health implements inference.Provider.Health.
func (p *Provider) Health(ctx context.Context) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return inference.ErrProviderClosed
	}
	p.mu.RUnlock()

	// Send a minimal request to check if the API is working
	// Anthropic doesn't have a health endpoint, so we send a minimal message
	req := &anthropicMessageRequest{
		Model:     "claude-3-haiku-20240307",
		MaxTokens: 1,
		Messages: []anthropicMessage{
			{Role: "user", Content: "hi"},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal health check request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	// 200 OK means the API is working
	// 400 could mean invalid request but API is reachable
	// 401 means invalid API key
	// 5xx means server issues
	if resp.StatusCode >= 500 {
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

// setHeaders sets the required headers for Anthropic API requests.
func (p *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
}

// handleErrorResponse extracts error information from a non-200 response.
func (p *Provider) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp anthropicErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Type != "" {
		return fmt.Errorf("anthropic error (%d): %s - %s", resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
	}

	return fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, string(body))
}

// Anthropic API types

type anthropicMessageRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessageResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []anthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        anthropicUsage          `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicErrorResponse struct {
	Type  string         `json:"type"`
	Error anthropicError `json:"error"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Streaming types
type anthropicStreamEvent struct {
	Type         string                   `json:"type"`
	Message      *anthropicMessageResponse `json:"message,omitempty"`
	Index        int                      `json:"index,omitempty"`
	ContentBlock *anthropicContentBlock   `json:"content_block,omitempty"`
	Delta        *anthropicDelta          `json:"delta,omitempty"`
	Usage        *anthropicUsage          `json:"usage,omitempty"`
}

type anthropicDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

// Conversion helpers

func convertRequest(req *inference.CompletionRequest) *anthropicMessageRequest {
	var systemMsg string
	messages := make([]anthropicMessage, 0, len(req.Messages))

	for _, m := range req.Messages {
		if m.Role == "system" {
			systemMsg = m.Content
			continue
		}
		messages = append(messages, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	maxTokens := 4096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	return &anthropicMessageRequest{
		Model:         req.Model,
		Messages:      messages,
		System:        systemMsg,
		MaxTokens:     maxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.Stop,
	}
}

func convertResponse(resp *anthropicMessageResponse) *inference.CompletionResponse {
	// Extract text content from content blocks
	var content string
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	finishReason := "stop"
	if resp.StopReason != "" {
		finishReason = resp.StopReason
	}

	return &inference.CompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []inference.Choice{
			{
				Index: 0,
				Message: &inference.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: finishReason,
			},
		},
		Usage: &inference.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// streamReader reads SSE stream from Anthropic.
type streamReader struct {
	reader  *bufio.Reader
	resp    *http.Response
	model   string
	msgID   string
	created int64
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

		// Handle SSE format - Anthropic uses "event:" and "data:" lines
		if strings.HasPrefix(line, "event:") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil {
				s.msgID = event.Message.ID
				s.created = time.Now().Unix()
			}
			continue

		case "content_block_start":
			continue

		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				return &inference.CompletionChunk{
					ID:      s.msgID,
					Object:  "chat.completion.chunk",
					Created: s.created,
					Model:   s.model,
					Choices: []inference.Choice{
						{
							Index: 0,
							Delta: &inference.Message{
								Content: event.Delta.Text,
							},
						},
					},
				}, nil
			}
			continue

		case "content_block_stop":
			continue

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				return &inference.CompletionChunk{
					ID:      s.msgID,
					Object:  "chat.completion.chunk",
					Created: s.created,
					Model:   s.model,
					Choices: []inference.Choice{
						{
							Index:        0,
							FinishReason: event.Delta.StopReason,
						},
					},
				}, nil
			}
			continue

		case "message_stop":
			return nil, io.EOF

		case "error":
			return nil, fmt.Errorf("stream error: received error event")

		default:
			// Unknown event type, skip
			continue
		}
	}
}

// Close implements inference.StreamReader.Close.
func (s *streamReader) Close() error {
	return s.resp.Body.Close()
}
