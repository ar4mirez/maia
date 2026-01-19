package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client handles HTTP communication with the backend LLM provider.
type Client struct {
	httpClient *http.Client
	baseURL    string
	timeout    time.Duration
}

// ClientConfig holds configuration for the backend client.
type ClientConfig struct {
	BaseURL string
	Timeout time.Duration
}

// NewClient creates a new backend client.
func NewClient(cfg *ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
		timeout: timeout,
	}
}

// ChatCompletion sends a chat completion request and returns the response.
func (c *Client) ChatCompletion(
	ctx context.Context,
	req *ChatCompletionRequest,
	authHeader string,
) (*ChatCompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// ChatCompletionStream sends a streaming chat completion request.
func (c *Client) ChatCompletionStream(
	ctx context.Context,
	req *ChatCompletionRequest,
	authHeader string,
) (*StreamReader, error) {
	// Ensure streaming is enabled
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.handleErrorResponse(resp)
	}

	return &StreamReader{
		reader: bufio.NewReader(resp.Body),
		resp:   resp,
	}, nil
}

// ListModels lists available models from the backend.
func (c *Client) ListModels(ctx context.Context, authHeader string) (*ModelsResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var result ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// handleErrorResponse extracts error information from a non-200 response.
func (c *Client) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		return &BackendError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
	}

	return &BackendError{
		StatusCode: resp.StatusCode,
		Message:    string(body),
	}
}

// BackendError represents an error from the backend provider.
type BackendError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *BackendError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("backend error (%d): %s - %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("backend error (%d): %s", e.StatusCode, e.Message)
}

// StreamReader reads SSE stream from the backend.
type StreamReader struct {
	reader *bufio.Reader
	resp   *http.Response
}

// Read reads the next chunk from the stream.
// Returns io.EOF when the stream is done.
func (s *StreamReader) Read() (*ChatCompletionChunk, error) {
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

		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("unmarshal chunk: %w", err)
		}

		return &chunk, nil
	}
}

// Close closes the stream.
func (s *StreamReader) Close() error {
	return s.resp.Body.Close()
}

// Accumulator accumulates streaming chunks into a complete response.
type Accumulator struct {
	ID                string
	Object            string
	Created           int64
	Model             string
	SystemFingerprint string
	Contents          []string
	FinishReasons     []string
	ToolCalls         [][]ToolCall
}

// NewAccumulator creates a new accumulator.
func NewAccumulator() *Accumulator {
	return &Accumulator{
		Contents:      []string{},
		FinishReasons: []string{},
		ToolCalls:     [][]ToolCall{},
	}
}

// Add adds a chunk to the accumulator.
func (a *Accumulator) Add(chunk *ChatCompletionChunk) {
	// Copy metadata from first chunk
	if a.ID == "" {
		a.ID = chunk.ID
		a.Object = chunk.Object
		a.Created = chunk.Created
		a.Model = chunk.Model
		a.SystemFingerprint = chunk.SystemFingerprint
	}

	for i, choice := range chunk.Choices {
		// Expand slices if needed
		for len(a.Contents) <= i {
			a.Contents = append(a.Contents, "")
			a.FinishReasons = append(a.FinishReasons, "")
			a.ToolCalls = append(a.ToolCalls, []ToolCall{})
		}

		if choice.Delta != nil {
			content := choice.Delta.GetContentString()
			a.Contents[i] += content

			// Accumulate tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					// Find or create tool call
					found := false
					for j := range a.ToolCalls[i] {
						if a.ToolCalls[i][j].ID == tc.ID || (tc.ID == "" && j < len(a.ToolCalls[i])) {
							// Append to existing
							a.ToolCalls[i][j].Function.Arguments += tc.Function.Arguments
							found = true
							break
						}
					}
					if !found && tc.ID != "" {
						a.ToolCalls[i] = append(a.ToolCalls[i], tc)
					}
				}
			}
		}

		if choice.FinishReason != "" {
			a.FinishReasons[i] = choice.FinishReason
		}
	}
}

// ToResponse converts the accumulated data to a ChatCompletionResponse.
func (a *Accumulator) ToResponse() *ChatCompletionResponse {
	choices := make([]Choice, len(a.Contents))
	for i := range choices {
		choices[i] = Choice{
			Index: i,
			Message: &ChatMessage{
				Role:    "assistant",
				Content: a.Contents[i],
			},
			FinishReason: a.FinishReasons[i],
		}
		if len(a.ToolCalls[i]) > 0 {
			choices[i].Message.ToolCalls = a.ToolCalls[i]
		}
	}

	return &ChatCompletionResponse{
		ID:                a.ID,
		Object:            "chat.completion",
		Created:           a.Created,
		Model:             a.Model,
		SystemFingerprint: a.SystemFingerprint,
		Choices:           choices,
	}
}

// GetAssistantContent returns the accumulated assistant content for all choices.
func (a *Accumulator) GetAssistantContent() string {
	if len(a.Contents) == 0 {
		return ""
	}
	return a.Contents[0]
}
