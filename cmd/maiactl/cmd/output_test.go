package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"very short max", "hello", 3, "hel"},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatMemoryType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"semantic", "Semantic"},
		{"episodic", "Episodic"},
		{"working", "Working"},
		{"procedural", "Procedural"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := FormatMemoryType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrintJSON(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	data := map[string]string{"key": "value"}
	err := PrintJSON(data)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "\"key\": \"value\"")
}

func TestPrintTable(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	headers := []string{"NAME", "VALUE"}
	rows := [][]string{
		{"key1", "value1"},
		{"key2", "value2"},
	}
	PrintTable(headers, rows)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "VALUE")
	assert.Contains(t, output, "key1")
	assert.Contains(t, output, "value1")
	assert.Contains(t, output, "key2")
	assert.Contains(t, output, "value2")
}

func TestPrintKeyValue(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	pairs := map[string]string{
		"Name":  "test",
		"Value": "123",
	}
	PrintKeyValue(pairs)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	output := buf.String()
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "test")
}
