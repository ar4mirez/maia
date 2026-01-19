// Package proxy provides an OpenAI-compatible proxy with automatic memory management.
package proxy

import (
	"encoding/json"
	"time"
)

// ChatCompletionRequest represents an OpenAI chat completion request.
type ChatCompletionRequest struct {
	Model            string                 `json:"model"`
	Messages         []ChatMessage          `json:"messages"`
	Temperature      *float64               `json:"temperature,omitempty"`
	TopP             *float64               `json:"top_p,omitempty"`
	N                *int                   `json:"n,omitempty"`
	Stream           bool                   `json:"stream,omitempty"`
	Stop             []string               `json:"stop,omitempty"`
	MaxTokens        *int                   `json:"max_tokens,omitempty"`
	PresencePenalty  *float64               `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64               `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int         `json:"logit_bias,omitempty"`
	User             string                 `json:"user,omitempty"`
	Functions        []Function             `json:"functions,omitempty"`
	FunctionCall     interface{}            `json:"function_call,omitempty"`
	Tools            []Tool                 `json:"tools,omitempty"`
	ToolChoice       interface{}            `json:"tool_choice,omitempty"`
	ResponseFormat   *ResponseFormat        `json:"response_format,omitempty"`
	Seed             *int                   `json:"seed,omitempty"`
	Extra            map[string]interface{} `json:"-"`
}

// ChatMessage represents a message in a chat completion request/response.
type ChatMessage struct {
	Role         string        `json:"role"`
	Content      interface{}   `json:"content"`
	Name         string        `json:"name,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
}

// GetContentString returns the content as a string.
func (m *ChatMessage) GetContentString() string {
	switch v := m.Content.(type) {
	case string:
		return v
	case []interface{}:
		// Handle multi-modal content (text + images)
		for _, part := range v {
			if p, ok := part.(map[string]interface{}); ok {
				if p["type"] == "text" {
					if text, ok := p["text"].(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}

// SetContent sets the content from a string.
func (m *ChatMessage) SetContent(content string) {
	m.Content = content
}

// Function represents a function definition for function calling.
type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// FunctionCall represents a function call in a message.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool represents a tool definition.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// ToolCall represents a tool call in a message.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// ResponseFormat specifies the format of the response.
type ResponseFormat struct {
	Type string `json:"type"`
}

// ChatCompletionResponse represents an OpenAI chat completion response.
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
}

// Choice represents a choice in a chat completion response.
type Choice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason string       `json:"finish_reason,omitempty"`
	Logprobs     interface{}  `json:"logprobs,omitempty"`
}

// Usage represents token usage in a response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionChunk represents a streaming chunk response.
type ChatCompletionChunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
}

// ErrorResponse represents an OpenAI API error response.
type ErrorResponse struct {
	Error *APIError `json:"error"`
}

// APIError represents an API error.
type APIError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param,omitempty"`
	Code    *string `json:"code,omitempty"`
}

// Model represents an OpenAI model.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsResponse represents a list of models.
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// ProxyRequest wraps a chat completion request with MAIA-specific metadata.
type ProxyRequest struct {
	*ChatCompletionRequest
	Namespace   string `json:"-"`
	TokenBudget int    `json:"-"`
	SkipMemory  bool   `json:"-"`
	SkipExtract bool   `json:"-"`
}

// ProxyResponse wraps a chat completion response with MAIA-specific metadata.
type ProxyResponse struct {
	*ChatCompletionResponse
	MemoriesUsed     int           `json:"_maia_memories_used,omitempty"`
	TokensInjected   int           `json:"_maia_tokens_injected,omitempty"`
	ContextQueryTime time.Duration `json:"_maia_context_query_time,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for ChatCompletionRequest
// to handle unknown fields.
func (r *ChatCompletionRequest) UnmarshalJSON(data []byte) error {
	type Alias ChatCompletionRequest
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Unmarshal again to capture extra fields
	var extra map[string]interface{}
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}

	// Remove known fields
	knownFields := []string{
		"model", "messages", "temperature", "top_p", "n", "stream",
		"stop", "max_tokens", "presence_penalty", "frequency_penalty",
		"logit_bias", "user", "functions", "function_call", "tools",
		"tool_choice", "response_format", "seed",
	}
	for _, f := range knownFields {
		delete(extra, f)
	}

	if len(extra) > 0 {
		r.Extra = extra
	}

	return nil
}

// MarshalJSON implements custom marshaling for ChatCompletionRequest
// to include extra fields.
func (r *ChatCompletionRequest) MarshalJSON() ([]byte, error) {
	type Alias ChatCompletionRequest
	data, err := json.Marshal((*Alias)(r))
	if err != nil {
		return nil, err
	}

	if len(r.Extra) == 0 {
		return data, nil
	}

	// Merge extra fields
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for k, v := range r.Extra {
		m[k] = v
	}

	return json.Marshal(m)
}
