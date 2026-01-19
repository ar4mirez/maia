package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVocabTxt(t *testing.T) {
	tests := []struct {
		name     string
		vocabTxt string
		wantErr  bool
	}{
		{
			name:     "simple vocabulary",
			vocabTxt: "[PAD]\n[UNK]\n[CLS]\n[SEP]\nhello\nworld\n",
			wantErr:  false,
		},
		{
			name:     "with windows line endings",
			vocabTxt: "[PAD]\r\n[UNK]\r\n[CLS]\r\n[SEP]\r\n",
			wantErr:  false,
		},
		{
			name:     "empty vocabulary",
			vocabTxt: "",
			wantErr:  false,
		},
		{
			name:     "special characters",
			vocabTxt: "[PAD]\n##ing\n##ed\n[MASK]\n",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := ParseVocabTxt([]byte(tt.vocabTxt))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, jsonData)

			// Verify it's valid for tokenizer
			if tt.name == "simple vocabulary" {
				cfg := TokenizerConfig{
					MaxLength:   8,
					DoLowerCase: true,
					PadToken:    "[PAD]",
					ClsToken:    "[CLS]",
					SepToken:    "[SEP]",
					UnkToken:    "[UNK]",
				}
				tok, err := NewWordPieceTokenizer(jsonData, cfg)
				require.NoError(t, err)
				assert.NotNil(t, tok)

				// Test encoding
				encoded := tok.Encode("hello world")
				assert.NotNil(t, encoded)
			}
		})
	}
}

func TestVocabToJSON(t *testing.T) {
	vocab := map[string]int{
		"hello": 0,
		"world": 1,
	}

	jsonBytes := vocabToJSON(vocab)
	assert.Contains(t, string(jsonBytes), `"hello":0`)
	assert.Contains(t, string(jsonBytes), `"world":1`)
}

func TestEscapeJSONString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "hello", want: "hello"},
		{input: `with"quote`, want: `with\"quote`},
		{input: "with\\slash", want: "with\\\\slash"},
		{input: "with\nnewline", want: "with\\nnewline"},
		{input: "with\ttab", want: "with\\ttab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeJSONString(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFileExists(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	// File doesn't exist yet
	assert.False(t, fileExists(tmpFile))

	// Create the file
	err := os.WriteFile(tmpFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Now it exists
	assert.True(t, fileExists(tmpFile))
}

func TestGetModelDir(t *testing.T) {
	// Test with environment variable
	t.Run("with env var", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("MAIA_MODEL_DIR", tmpDir)

		dir, err := GetModelDir()
		require.NoError(t, err)
		assert.Equal(t, tmpDir, dir)
	})

	// Test without environment variable (uses home dir)
	t.Run("without env var", func(t *testing.T) {
		t.Setenv("MAIA_MODEL_DIR", "")

		dir, err := GetModelDir()
		require.NoError(t, err)
		assert.Contains(t, dir, DefaultModelDir)
	})
}

func TestComputeFileHash(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	// Create file with known content
	content := []byte("hello world")
	err := os.WriteFile(tmpFile, content, 0644)
	require.NoError(t, err)

	hash, err := ComputeFileHash(tmpFile)
	require.NoError(t, err)

	// SHA256 of "hello world"
	expectedHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	assert.Equal(t, expectedHash, hash)
}

func TestComputeFileHash_NotFound(t *testing.T) {
	_, err := ComputeFileHash("/nonexistent/file")
	require.Error(t, err)
}
