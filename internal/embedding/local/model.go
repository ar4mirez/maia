package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Model file URLs and checksums for all-MiniLM-L6-v2
const (
	// HuggingFace model URLs
	ModelURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx"
	VocabURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"

	// Expected file sizes (approximate, for verification)
	ExpectedModelSize = 90_000_000 // ~90MB
	ExpectedVocabSize = 230_000    // ~230KB

	// Default model directory
	DefaultModelDir = ".maia/models/minilm-l6-v2"
)

// ModelFiles holds paths to the model files.
type ModelFiles struct {
	ModelPath string
	VocabPath string
}

// GetModelDir returns the model directory path.
func GetModelDir() (string, error) {
	// Check for environment variable override
	if dir := os.Getenv("MAIA_MODEL_DIR"); dir != "" {
		return dir, nil
	}

	// Use user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, DefaultModelDir), nil
}

// EnsureModelFiles ensures the model files are available, downloading if needed.
func EnsureModelFiles() (*ModelFiles, error) {
	modelDir, err := GetModelDir()
	if err != nil {
		return nil, err
	}

	// Create model directory if needed
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create model directory: %w", err)
	}

	modelPath := filepath.Join(modelDir, "model.onnx")
	vocabPath := filepath.Join(modelDir, "vocab.txt")

	files := &ModelFiles{
		ModelPath: modelPath,
		VocabPath: vocabPath,
	}

	// Check if files exist
	if fileExists(modelPath) && fileExists(vocabPath) {
		return files, nil
	}

	// Download missing files
	if !fileExists(vocabPath) {
		fmt.Println("Downloading vocabulary...")
		if err := downloadFile(VocabURL, vocabPath); err != nil {
			return nil, fmt.Errorf("failed to download vocabulary: %w", err)
		}
	}

	if !fileExists(modelPath) {
		fmt.Println("Downloading model (this may take a while)...")
		if err := downloadFile(ModelURL, modelPath); err != nil {
			return nil, fmt.Errorf("failed to download model: %w", err)
		}
	}

	return files, nil
}

// LoadVocab loads the vocabulary from the vocab.txt file.
// The vocab.txt format is one token per line, where the line number is the token ID.
func LoadVocab(vocabPath string) ([]byte, error) {
	data, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vocabulary file: %w", err)
	}
	return data, nil
}

// ParseVocabTxt converts vocab.txt format to JSON map.
func ParseVocabTxt(vocabTxt []byte) ([]byte, error) {
	vocab := make(map[string]int)

	// Parse line by line
	tokenID := 0
	start := 0
	for i := 0; i <= len(vocabTxt); i++ {
		if i == len(vocabTxt) || vocabTxt[i] == '\n' {
			if i > start {
				token := string(vocabTxt[start:i])
				// Remove carriage return if present
				if len(token) > 0 && token[len(token)-1] == '\r' {
					token = token[:len(token)-1]
				}
				if token != "" {
					vocab[token] = tokenID
					tokenID++
				}
			}
			start = i + 1
		}
	}

	// Convert to JSON bytes
	// Simple JSON encoding without external dependencies
	return vocabToJSON(vocab), nil
}

// vocabToJSON converts vocab map to JSON bytes.
func vocabToJSON(vocab map[string]int) []byte {
	// Build JSON string
	var result []byte
	result = append(result, '{')
	first := true
	for token, id := range vocab {
		if !first {
			result = append(result, ',')
		}
		first = false
		// Escape special characters in token
		escaped := escapeJSONString(token)
		result = append(result, fmt.Sprintf("%q:%d", escaped, id)...)
	}
	result = append(result, '}')
	return result
}

// escapeJSONString escapes special characters for JSON.
func escapeJSONString(s string) string {
	var result []byte
	for _, r := range s {
		switch r {
		case '"':
			result = append(result, '\\', '"')
		case '\\':
			result = append(result, '\\', '\\')
		case '\n':
			result = append(result, '\\', 'n')
		case '\r':
			result = append(result, '\\', 'r')
		case '\t':
			result = append(result, '\\', 't')
		default:
			if r < 32 {
				result = append(result, fmt.Sprintf("\\u%04x", r)...)
			} else {
				result = append(result, string(r)...)
			}
		}
	}
	return string(result)
}

// downloadFile downloads a file from URL to the specified path.
func downloadFile(url, destPath string) error {
	// Create temporary file
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		out.Close()
		os.Remove(tmpPath) // Clean up on error
	}()

	// Download
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Copy with progress
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Close before rename
	out.Close()

	// Verify size (basic check)
	if written == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	// Rename to final path
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ComputeFileHash computes SHA256 hash of a file.
func ComputeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
