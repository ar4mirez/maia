package proxy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatMessage_GetContentString(t *testing.T) {
	tests := []struct {
		name     string
		content  interface{}
		expected string
	}{
		{
			name:     "string content",
			content:  "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "nil content",
			content:  nil,
			expected: "",
		},
		{
			name: "multimodal content with text",
			content: []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Describe this image",
				},
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "data:image/png;base64,..."},
				},
			},
			expected: "Describe this image",
		},
		{
			name: "multimodal content without text",
			content: []interface{}{
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "data:image/png;base64,..."},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := ChatMessage{Content: tt.content}
			assert.Equal(t, tt.expected, msg.GetContentString())
		})
	}
}

func TestChatMessage_SetContent(t *testing.T) {
	msg := ChatMessage{}
	msg.SetContent("New content")
	assert.Equal(t, "New content", msg.Content)
}

func TestChatCompletionRequest_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		request  ChatCompletionRequest
		checkFn  func(t *testing.T, data []byte)
	}{
		{
			name: "basic request",
			request: ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			checkFn: func(t *testing.T, data []byte) {
				assert.Contains(t, string(data), `"model":"gpt-4"`)
				assert.Contains(t, string(data), `"messages"`)
			},
		},
		{
			name: "request with extra fields",
			request: ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
				Extra: map[string]interface{}{
					"custom_field": "custom_value",
				},
			},
			checkFn: func(t *testing.T, data []byte) {
				assert.Contains(t, string(data), `"custom_field":"custom_value"`)
			},
		},
		{
			name: "request with optional fields",
			request: ChatCompletionRequest{
				Model:  "gpt-4",
				Stream: true,
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			checkFn: func(t *testing.T, data []byte) {
				assert.Contains(t, string(data), `"stream":true`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(&tt.request)
			require.NoError(t, err)
			tt.checkFn(t, data)
		})
	}
}

func TestChatCompletionRequest_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		checkFn func(t *testing.T, req *ChatCompletionRequest)
	}{
		{
			name: "basic request",
			json: `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`,
			checkFn: func(t *testing.T, req *ChatCompletionRequest) {
				assert.Equal(t, "gpt-4", req.Model)
				assert.Len(t, req.Messages, 1)
				assert.Equal(t, "user", req.Messages[0].Role)
			},
		},
		{
			name: "request with extra fields",
			json: `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"custom_field":"value"}`,
			checkFn: func(t *testing.T, req *ChatCompletionRequest) {
				assert.NotNil(t, req.Extra)
				assert.Equal(t, "value", req.Extra["custom_field"])
			},
		},
		{
			name: "request with all known fields",
			json: `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"temperature":0.7,"stream":true}`,
			checkFn: func(t *testing.T, req *ChatCompletionRequest) {
				assert.Nil(t, req.Extra)
				assert.True(t, req.Stream)
				require.NotNil(t, req.Temperature)
				assert.Equal(t, 0.7, *req.Temperature)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req ChatCompletionRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			require.NoError(t, err)
			tt.checkFn(t, &req)
		})
	}
}

func TestChatCompletionResponse_JSON(t *testing.T) {
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1677652288,
		Model:   "gpt-4",
		Choices: []Choice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role:    "assistant",
					Content: "Hello! How can I help you?",
				},
				FinishReason: "stop",
			},
		},
		Usage: &Usage{
			PromptTokens:     9,
			CompletionTokens: 12,
			TotalTokens:      21,
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ChatCompletionResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resp.ID, decoded.ID)
	assert.Equal(t, resp.Model, decoded.Model)
	assert.Len(t, decoded.Choices, 1)
	assert.Equal(t, "assistant", decoded.Choices[0].Message.Role)
}

func TestErrorResponse_JSON(t *testing.T) {
	code := "invalid_api_key"
	errResp := ErrorResponse{
		Error: &APIError{
			Message: "Invalid API key",
			Type:    "invalid_request_error",
			Code:    &code,
		},
	}

	data, err := json.Marshal(errResp)
	require.NoError(t, err)

	var decoded ErrorResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "Invalid API key", decoded.Error.Message)
	assert.Equal(t, "invalid_request_error", decoded.Error.Type)
	require.NotNil(t, decoded.Error.Code)
	assert.Equal(t, "invalid_api_key", *decoded.Error.Code)
}

func TestModelsResponse_JSON(t *testing.T) {
	resp := ModelsResponse{
		Object: "list",
		Data: []Model{
			{
				ID:      "gpt-4",
				Object:  "model",
				Created: 1687882410,
				OwnedBy: "openai",
			},
			{
				ID:      "gpt-3.5-turbo",
				Object:  "model",
				Created: 1677610602,
				OwnedBy: "openai",
			},
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ModelsResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "list", decoded.Object)
	assert.Len(t, decoded.Data, 2)
	assert.Equal(t, "gpt-4", decoded.Data[0].ID)
}
