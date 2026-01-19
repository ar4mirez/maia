// Package local provides local embedding generation using ONNX Runtime.
package local

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

// Common errors for tokenizer operations.
var (
	ErrEmptyVocab       = errors.New("vocabulary is empty")
	ErrUnknownToken     = errors.New("unknown token not in vocabulary")
	ErrInvalidMaxLength = errors.New("max length must be positive")
)

// Special token IDs for BERT-style models.
const (
	DefaultPadToken = "[PAD]"
	DefaultClsToken = "[CLS]"
	DefaultSepToken = "[SEP]"
	DefaultUnkToken = "[UNK]"
	DefaultMaskToken = "[MASK]"

	// WordPiece continuation prefix
	WordPiecePrefix = "##"
)

// TokenizerConfig holds configuration for the tokenizer.
type TokenizerConfig struct {
	// MaxLength is the maximum sequence length (including special tokens).
	MaxLength int

	// DoLowerCase indicates whether to lowercase input text.
	DoLowerCase bool

	// Special tokens
	PadToken  string
	ClsToken  string
	SepToken  string
	UnkToken  string
}

// DefaultTokenizerConfig returns the default configuration for BERT-style models.
func DefaultTokenizerConfig() TokenizerConfig {
	return TokenizerConfig{
		MaxLength:   256,
		DoLowerCase: true,
		PadToken:    DefaultPadToken,
		ClsToken:    DefaultClsToken,
		SepToken:    DefaultSepToken,
		UnkToken:    DefaultUnkToken,
	}
}

// WordPieceTokenizer implements BERT-style WordPiece tokenization.
type WordPieceTokenizer struct {
	vocab     map[string]int
	idToToken map[int]string
	config    TokenizerConfig

	// Cached special token IDs
	padID int
	clsID int
	sepID int
	unkID int
}

// NewWordPieceTokenizer creates a new tokenizer from vocabulary JSON.
func NewWordPieceTokenizer(vocabJSON []byte, cfg TokenizerConfig) (*WordPieceTokenizer, error) {
	if cfg.MaxLength <= 0 {
		return nil, ErrInvalidMaxLength
	}

	var vocab map[string]int
	if err := json.Unmarshal(vocabJSON, &vocab); err != nil {
		return nil, err
	}

	if len(vocab) == 0 {
		return nil, ErrEmptyVocab
	}

	// Build reverse mapping
	idToToken := make(map[int]string, len(vocab))
	for token, id := range vocab {
		idToToken[id] = token
	}

	// Get special token IDs
	padID, ok := vocab[cfg.PadToken]
	if !ok {
		padID = 0
	}

	clsID, ok := vocab[cfg.ClsToken]
	if !ok {
		return nil, errors.New("CLS token not found in vocabulary")
	}

	sepID, ok := vocab[cfg.SepToken]
	if !ok {
		return nil, errors.New("SEP token not found in vocabulary")
	}

	unkID, ok := vocab[cfg.UnkToken]
	if !ok {
		return nil, ErrUnknownToken
	}

	return &WordPieceTokenizer{
		vocab:     vocab,
		idToToken: idToToken,
		config:    cfg,
		padID:     padID,
		clsID:     clsID,
		sepID:     sepID,
		unkID:     unkID,
	}, nil
}

// EncodedInput represents the tokenized input for the model.
type EncodedInput struct {
	InputIDs      []int64
	AttentionMask []int64
	TokenTypeIDs  []int64
}

// Encode tokenizes text and returns model inputs.
func (t *WordPieceTokenizer) Encode(text string) *EncodedInput {
	// Preprocess
	if t.config.DoLowerCase {
		text = strings.ToLower(text)
	}

	// Basic tokenization (split on whitespace and punctuation)
	basicTokens := t.basicTokenize(text)

	// WordPiece tokenization
	var tokens []string
	for _, token := range basicTokens {
		subTokens := t.wordPieceTokenize(token)
		tokens = append(tokens, subTokens...)
	}

	// Truncate if needed (leave room for [CLS] and [SEP])
	maxTokens := t.config.MaxLength - 2
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	// Build input IDs with special tokens
	inputIDs := make([]int64, t.config.MaxLength)
	attentionMask := make([]int64, t.config.MaxLength)
	tokenTypeIDs := make([]int64, t.config.MaxLength)

	// [CLS] token
	inputIDs[0] = int64(t.clsID)
	attentionMask[0] = 1

	// Content tokens
	for i, token := range tokens {
		id, ok := t.vocab[token]
		if !ok {
			id = t.unkID
		}
		inputIDs[i+1] = int64(id)
		attentionMask[i+1] = 1
	}

	// [SEP] token
	sepPos := len(tokens) + 1
	inputIDs[sepPos] = int64(t.sepID)
	attentionMask[sepPos] = 1

	// Padding (already zero-initialized for inputIDs and tokenTypeIDs)
	// PAD token ID is typically 0
	for i := sepPos + 1; i < t.config.MaxLength; i++ {
		inputIDs[i] = int64(t.padID)
		// attentionMask and tokenTypeIDs are already 0
	}

	return &EncodedInput{
		InputIDs:      inputIDs,
		AttentionMask: attentionMask,
		TokenTypeIDs:  tokenTypeIDs,
	}
}

// EncodeBatch tokenizes multiple texts and returns batched model inputs.
func (t *WordPieceTokenizer) EncodeBatch(texts []string) *BatchEncodedInput {
	batch := &BatchEncodedInput{
		InputIDs:      make([][]int64, len(texts)),
		AttentionMask: make([][]int64, len(texts)),
		TokenTypeIDs:  make([][]int64, len(texts)),
	}

	for i, text := range texts {
		encoded := t.Encode(text)
		batch.InputIDs[i] = encoded.InputIDs
		batch.AttentionMask[i] = encoded.AttentionMask
		batch.TokenTypeIDs[i] = encoded.TokenTypeIDs
	}

	return batch
}

// BatchEncodedInput represents batched tokenized inputs.
type BatchEncodedInput struct {
	InputIDs      [][]int64
	AttentionMask [][]int64
	TokenTypeIDs  [][]int64
}

// Flatten returns flattened arrays for ONNX input.
func (b *BatchEncodedInput) Flatten() (inputIDs, attentionMask, tokenTypeIDs []int64) {
	batchSize := len(b.InputIDs)
	if batchSize == 0 {
		return nil, nil, nil
	}

	seqLen := len(b.InputIDs[0])
	totalLen := batchSize * seqLen

	inputIDs = make([]int64, totalLen)
	attentionMask = make([]int64, totalLen)
	tokenTypeIDs = make([]int64, totalLen)

	for i := 0; i < batchSize; i++ {
		offset := i * seqLen
		copy(inputIDs[offset:], b.InputIDs[i])
		copy(attentionMask[offset:], b.AttentionMask[i])
		copy(tokenTypeIDs[offset:], b.TokenTypeIDs[i])
	}

	return inputIDs, attentionMask, tokenTypeIDs
}

// basicTokenize splits text on whitespace and punctuation.
func (t *WordPieceTokenizer) basicTokenize(text string) []string {
	// Normalize whitespace and clean
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Insert spaces around punctuation
	var result strings.Builder
	for _, r := range text {
		if unicode.IsPunct(r) {
			result.WriteRune(' ')
			result.WriteRune(r)
			result.WriteRune(' ')
		} else {
			result.WriteRune(r)
		}
	}

	// Split on whitespace
	tokens := strings.Fields(result.String())

	return tokens
}

// wordPieceTokenize applies WordPiece algorithm to a single token.
func (t *WordPieceTokenizer) wordPieceTokenize(token string) []string {
	if len(token) == 0 {
		return nil
	}

	// Check if whole token is in vocabulary
	if _, ok := t.vocab[token]; ok {
		return []string{token}
	}

	// Apply WordPiece algorithm
	var subTokens []string
	start := 0

	for start < len(token) {
		end := len(token)
		var curSubstr string
		found := false

		for start < end {
			substr := token[start:end]
			if start > 0 {
				substr = WordPiecePrefix + substr
			}

			if _, ok := t.vocab[substr]; ok {
				curSubstr = substr
				found = true
				break
			}

			// Try shorter substring (character by character from end)
			end = t.prevCharBoundary(token, end)
		}

		if !found {
			// If we can't find any subword, use UNK for the remaining part
			subTokens = append(subTokens, t.config.UnkToken)
			break
		}

		subTokens = append(subTokens, curSubstr)
		start = end
		if start > 0 && !strings.HasPrefix(curSubstr, WordPiecePrefix) {
			start = end
		} else if strings.HasPrefix(curSubstr, WordPiecePrefix) {
			start = end
		}
	}

	return subTokens
}

// prevCharBoundary returns the byte index of the previous UTF-8 character.
func (t *WordPieceTokenizer) prevCharBoundary(s string, pos int) int {
	if pos <= 0 || pos > len(s) {
		return 0
	}
	pos--
	for pos > 0 && !isUTF8Start(s[pos]) {
		pos--
	}
	return pos
}

// isUTF8Start returns true if the byte is the start of a UTF-8 character.
func isUTF8Start(b byte) bool {
	return b&0xC0 != 0x80
}

// VocabSize returns the vocabulary size.
func (t *WordPieceTokenizer) VocabSize() int {
	return len(t.vocab)
}

// MaxLength returns the maximum sequence length.
func (t *WordPieceTokenizer) MaxLength() int {
	return t.config.MaxLength
}

// Decode converts token IDs back to text (for debugging).
func (t *WordPieceTokenizer) Decode(ids []int64) string {
	var tokens []string
	for _, id := range ids {
		if token, ok := t.idToToken[int(id)]; ok {
			// Skip special tokens
			if token == t.config.PadToken || token == t.config.ClsToken ||
			   token == t.config.SepToken {
				continue
			}
			tokens = append(tokens, token)
		}
	}

	// Join tokens, handling WordPiece prefixes
	var result strings.Builder
	for i, token := range tokens {
		if strings.HasPrefix(token, WordPiecePrefix) {
			result.WriteString(strings.TrimPrefix(token, WordPiecePrefix))
		} else {
			if i > 0 {
				result.WriteRune(' ')
			}
			result.WriteString(token)
		}
	}

	return result.String()
}
