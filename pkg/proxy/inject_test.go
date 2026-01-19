package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjector_buildQuery(t *testing.T) {
	injector := &Injector{}

	tests := []struct {
		name     string
		messages []ChatMessage
		want     string
	}{
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "What is the weather today?"},
			},
			want: "What is the weather today?",
		},
		{
			name: "multiple user messages",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
				{Role: "user", Content: "What is the weather?"},
			},
			want: "Hello What is the weather?",
		},
		{
			name: "system and user messages",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Tell me about Go"},
			},
			want: "Tell me about Go",
		},
		{
			name:     "empty messages",
			messages: []ChatMessage{},
			want:     "",
		},
		{
			name: "only system message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
			},
			want: "",
		},
		{
			name: "three user messages - takes last 3",
			messages: []ChatMessage{
				{Role: "user", Content: "First"},
				{Role: "assistant", Content: "Response 1"},
				{Role: "user", Content: "Second"},
				{Role: "assistant", Content: "Response 2"},
				{Role: "user", Content: "Third"},
				{Role: "assistant", Content: "Response 3"},
				{Role: "user", Content: "Fourth"},
			},
			want: "Second Third Fourth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injector.buildQuery(tt.messages)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatContext(t *testing.T) {
	content := "User prefers dark mode."
	formatted := formatContext(content)

	assert.Contains(t, formatted, "[Relevant context from memory]")
	assert.Contains(t, formatted, "User prefers dark mode.")
	assert.Contains(t, formatted, "[End of context]")
}

func TestInjectIntoSystem(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		context  string
		checkFn  func(t *testing.T, result []ChatMessage)
	}{
		{
			name: "existing system message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 2)
				assert.Equal(t, "system", result[0].Role)
				content := result[0].GetContentString()
				assert.Contains(t, content, "Context here")
				assert.Contains(t, content, "You are helpful.")
			},
		},
		{
			name: "no system message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 2)
				assert.Equal(t, "system", result[0].Role)
				assert.Equal(t, "Context here", result[0].GetContentString())
				assert.Equal(t, "user", result[1].Role)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectIntoSystem(tt.messages, tt.context)
			tt.checkFn(t, result)
		})
	}
}

func TestInjectIntoFirstUser(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		context  string
		checkFn  func(t *testing.T, result []ChatMessage)
	}{
		{
			name: "prepend to first user",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 2)
				content := result[1].GetContentString()
				assert.Contains(t, content, "Context here")
				assert.Contains(t, content, "Hello")
			},
		},
		{
			name: "no user message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 1)
				assert.Equal(t, "You are helpful.", result[0].GetContentString())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectIntoFirstUser(tt.messages, tt.context)
			tt.checkFn(t, result)
		})
	}
}

func TestInjectBeforeLastUser(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		context  string
		checkFn  func(t *testing.T, result []ChatMessage)
	}{
		{
			name: "insert before last user",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "First question"},
				{Role: "assistant", Content: "First response"},
				{Role: "user", Content: "Second question"},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 5)
				assert.Equal(t, "system", result[0].Role)
				assert.Equal(t, "user", result[1].Role)
				assert.Equal(t, "assistant", result[2].Role)
				assert.Equal(t, "system", result[3].Role)
				assert.Equal(t, "Context here", result[3].GetContentString())
				assert.Equal(t, "user", result[4].Role)
				assert.Equal(t, "Second question", result[4].GetContentString())
			},
		},
		{
			name: "no user message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
			},
			context: "Context here",
			checkFn: func(t *testing.T, result []ChatMessage) {
				assert.Len(t, result, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectBeforeLastUser(tt.messages, tt.context)
			tt.checkFn(t, result)
		})
	}
}

func TestInjector_injectIntoMessages(t *testing.T) {
	injector := &Injector{}

	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	// Test with empty context
	result := injector.injectIntoMessages(messages, "", PositionSystem)
	assert.Len(t, result, 1)

	// Test with context
	result = injector.injectIntoMessages(messages, "Context", PositionSystem)
	assert.Len(t, result, 2)
	assert.Equal(t, "system", result[0].Role)

	// Test different positions
	result = injector.injectIntoMessages(messages, "Context", PositionFirstUser)
	assert.Contains(t, result[0].GetContentString(), "Context")

	// Test default position
	result = injector.injectIntoMessages(messages, "Context", "unknown")
	assert.Equal(t, "system", result[0].Role)
}
