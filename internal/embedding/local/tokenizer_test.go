package local

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testVocab creates a minimal vocabulary for testing.
func testVocab() map[string]int {
	return map[string]int{
		"[PAD]":  0,
		"[UNK]":  100,
		"[CLS]":  101,
		"[SEP]":  102,
		"[MASK]": 103,
		"hello":  7592,
		"world":  2088,
		"the":    1996,
		"quick":  4248,
		"brown":  2829,
		"fox":    4419,
		"##s":    2015,
		"##ed":   2098,
		"##ing":  2075,
		"test":   3231,
		"##er":   2121,
		".":      1012,
		",":      1010,
		"!":      999,
		"?":      1029,
	}
}

func testVocabJSON(t *testing.T) []byte {
	vocab := testVocab()
	data, err := json.Marshal(vocab)
	require.NoError(t, err)
	return data
}

func TestNewWordPieceTokenizer(t *testing.T) {
	tests := []struct {
		name      string
		vocabJSON []byte
		config    TokenizerConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid vocabulary",
			vocabJSON: testVocabJSON(t),
			config:    DefaultTokenizerConfig(),
			wantErr:   false,
		},
		{
			name:      "invalid JSON",
			vocabJSON: []byte("not json"),
			config:    DefaultTokenizerConfig(),
			wantErr:   true,
		},
		{
			name:      "empty vocabulary",
			vocabJSON: []byte("{}"),
			config:    DefaultTokenizerConfig(),
			wantErr:   true,
			errMsg:    "vocabulary is empty",
		},
		{
			name:      "invalid max length",
			vocabJSON: testVocabJSON(t),
			config: TokenizerConfig{
				MaxLength:   0,
				DoLowerCase: true,
				PadToken:    DefaultPadToken,
				ClsToken:    DefaultClsToken,
				SepToken:    DefaultSepToken,
				UnkToken:    DefaultUnkToken,
			},
			wantErr: true,
			errMsg:  "max length must be positive",
		},
		{
			name:      "missing CLS token",
			vocabJSON: []byte(`{"[PAD]": 0, "[SEP]": 102, "[UNK]": 100, "hello": 1}`),
			config:    DefaultTokenizerConfig(),
			wantErr:   true,
			errMsg:    "CLS token not found",
		},
		{
			name:      "missing SEP token",
			vocabJSON: []byte(`{"[PAD]": 0, "[CLS]": 101, "[UNK]": 100, "hello": 1}`),
			config:    DefaultTokenizerConfig(),
			wantErr:   true,
			errMsg:    "SEP token not found",
		},
		{
			name:      "missing UNK token",
			vocabJSON: []byte(`{"[PAD]": 0, "[CLS]": 101, "[SEP]": 102, "hello": 1}`),
			config:    DefaultTokenizerConfig(),
			wantErr:   true,
			errMsg:    "unknown token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := NewWordPieceTokenizer(tt.vocabJSON, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, tok)
		})
	}
}

func TestWordPieceTokenizer_Encode(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), DefaultTokenizerConfig())
	require.NoError(t, err)

	tests := []struct {
		name              string
		text              string
		wantFirstTokens   []int64 // First few tokens (after [CLS])
		wantAttentionOnes int     // Number of 1s in attention mask
	}{
		{
			name:              "simple text",
			text:              "hello world",
			wantFirstTokens:   []int64{101, 7592, 2088, 102}, // [CLS] hello world [SEP]
			wantAttentionOnes: 4,
		},
		{
			name:              "with lowercase",
			text:              "HELLO WORLD",
			wantFirstTokens:   []int64{101, 7592, 2088, 102}, // Should be lowercased
			wantAttentionOnes: 4,
		},
		{
			name:              "with punctuation",
			text:              "hello, world!",
			wantFirstTokens:   []int64{101, 7592, 1010, 2088, 999, 102}, // [CLS] hello , world ! [SEP]
			wantAttentionOnes: 6,
		},
		{
			name:              "unknown word",
			text:              "hello xyz",
			wantFirstTokens:   []int64{101, 7592, 100, 102}, // [CLS] hello [UNK] [SEP]
			wantAttentionOnes: 4,
		},
		{
			name:              "empty text",
			text:              "",
			wantFirstTokens:   []int64{101, 102}, // [CLS] [SEP]
			wantAttentionOnes: 2,
		},
		{
			name:              "whitespace only",
			text:              "   ",
			wantFirstTokens:   []int64{101, 102}, // [CLS] [SEP]
			wantAttentionOnes: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tok.Encode(tt.text)

			// Check structure
			assert.Len(t, encoded.InputIDs, 256)
			assert.Len(t, encoded.AttentionMask, 256)
			assert.Len(t, encoded.TokenTypeIDs, 256)

			// Check first tokens
			for i, want := range tt.wantFirstTokens {
				assert.Equal(t, want, encoded.InputIDs[i],
					"token %d: want %d, got %d", i, want, encoded.InputIDs[i])
			}

			// Check attention mask count
			ones := 0
			for _, v := range encoded.AttentionMask {
				if v == 1 {
					ones++
				}
			}
			assert.Equal(t, tt.wantAttentionOnes, ones)

			// All token type IDs should be 0 for single sequence
			for _, v := range encoded.TokenTypeIDs {
				assert.Equal(t, int64(0), v)
			}
		})
	}
}

func TestWordPieceTokenizer_EncodeBatch(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), DefaultTokenizerConfig())
	require.NoError(t, err)

	texts := []string{"hello world", "the quick brown fox"}
	batch := tok.EncodeBatch(texts)

	assert.Len(t, batch.InputIDs, 2)
	assert.Len(t, batch.AttentionMask, 2)
	assert.Len(t, batch.TokenTypeIDs, 2)

	// Check first sequence
	assert.Equal(t, int64(101), batch.InputIDs[0][0])  // [CLS]
	assert.Equal(t, int64(7592), batch.InputIDs[0][1]) // hello
	assert.Equal(t, int64(2088), batch.InputIDs[0][2]) // world
	assert.Equal(t, int64(102), batch.InputIDs[0][3])  // [SEP]

	// Check second sequence
	assert.Equal(t, int64(101), batch.InputIDs[1][0])  // [CLS]
	assert.Equal(t, int64(1996), batch.InputIDs[1][1]) // the
	assert.Equal(t, int64(4248), batch.InputIDs[1][2]) // quick
}

func TestBatchEncodedInput_Flatten(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), TokenizerConfig{
		MaxLength:   8, // Small for easier testing
		DoLowerCase: true,
		PadToken:    DefaultPadToken,
		ClsToken:    DefaultClsToken,
		SepToken:    DefaultSepToken,
		UnkToken:    DefaultUnkToken,
	})
	require.NoError(t, err)

	texts := []string{"hello", "world"}
	batch := tok.EncodeBatch(texts)

	inputIDs, attentionMask, tokenTypeIDs := batch.Flatten()

	// Should be batchSize * seqLen = 2 * 8 = 16
	assert.Len(t, inputIDs, 16)
	assert.Len(t, attentionMask, 16)
	assert.Len(t, tokenTypeIDs, 16)

	// First sequence: [CLS] hello [SEP] [PAD] [PAD] [PAD] [PAD] [PAD]
	assert.Equal(t, int64(101), inputIDs[0])  // [CLS]
	assert.Equal(t, int64(7592), inputIDs[1]) // hello
	assert.Equal(t, int64(102), inputIDs[2])  // [SEP]
	assert.Equal(t, int64(0), inputIDs[3])    // [PAD]

	// Second sequence starts at index 8
	assert.Equal(t, int64(101), inputIDs[8])  // [CLS]
	assert.Equal(t, int64(2088), inputIDs[9]) // world
	assert.Equal(t, int64(102), inputIDs[10]) // [SEP]
}

func TestBatchEncodedInput_Flatten_Empty(t *testing.T) {
	batch := &BatchEncodedInput{}
	inputIDs, attentionMask, tokenTypeIDs := batch.Flatten()

	assert.Nil(t, inputIDs)
	assert.Nil(t, attentionMask)
	assert.Nil(t, tokenTypeIDs)
}

func TestWordPieceTokenizer_Truncation(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), TokenizerConfig{
		MaxLength:   6, // Very short for testing truncation
		DoLowerCase: true,
		PadToken:    DefaultPadToken,
		ClsToken:    DefaultClsToken,
		SepToken:    DefaultSepToken,
		UnkToken:    DefaultUnkToken,
	})
	require.NoError(t, err)

	// This text would be: [CLS] the quick brown fox [SEP] = 6 tokens
	// But maxLength=6 with 2 reserved for [CLS]/[SEP] means only 4 content tokens
	text := "the quick brown fox"
	encoded := tok.Encode(text)

	// Should be exactly maxLength
	assert.Len(t, encoded.InputIDs, 6)

	// Should have: [CLS] the quick brown fox [SEP] - exactly fits!
	assert.Equal(t, int64(101), encoded.InputIDs[0])  // [CLS]
	assert.Equal(t, int64(1996), encoded.InputIDs[1]) // the
	assert.Equal(t, int64(4248), encoded.InputIDs[2]) // quick
	assert.Equal(t, int64(2829), encoded.InputIDs[3]) // brown
	assert.Equal(t, int64(4419), encoded.InputIDs[4]) // fox
	assert.Equal(t, int64(102), encoded.InputIDs[5])  // [SEP]
}

func TestWordPieceTokenizer_BasicTokenize(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), DefaultTokenizerConfig())
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple words",
			input: "hello world",
			want:  []string{"hello", "world"},
		},
		{
			name:  "with punctuation",
			input: "hello, world!",
			want:  []string{"hello", ",", "world", "!"},
		},
		{
			name:  "multiple spaces",
			input: "hello   world",
			want:  []string{"hello", "world"},
		},
		{
			name:  "leading/trailing spaces",
			input: "  hello world  ",
			want:  []string{"hello", "world"},
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.basicTokenize(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWordPieceTokenizer_Decode(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), DefaultTokenizerConfig())
	require.NoError(t, err)

	tests := []struct {
		name string
		ids  []int64
		want string
	}{
		{
			name: "simple tokens",
			ids:  []int64{101, 7592, 2088, 102}, // [CLS] hello world [SEP]
			want: "hello world",
		},
		{
			name: "with wordpiece",
			ids:  []int64{101, 3231, 2121, 102}, // [CLS] test ##er [SEP]
			want: "tester",
		},
		{
			name: "with padding",
			ids:  []int64{101, 7592, 102, 0, 0, 0}, // [CLS] hello [SEP] [PAD] [PAD] [PAD]
			want: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.Decode(tt.ids)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWordPieceTokenizer_VocabSize(t *testing.T) {
	tok, err := NewWordPieceTokenizer(testVocabJSON(t), DefaultTokenizerConfig())
	require.NoError(t, err)

	assert.Equal(t, len(testVocab()), tok.VocabSize())
}

func TestWordPieceTokenizer_MaxLength(t *testing.T) {
	cfg := DefaultTokenizerConfig()
	cfg.MaxLength = 128

	tok, err := NewWordPieceTokenizer(testVocabJSON(t), cfg)
	require.NoError(t, err)

	assert.Equal(t, 128, tok.MaxLength())
}

func TestDefaultTokenizerConfig(t *testing.T) {
	cfg := DefaultTokenizerConfig()

	assert.Equal(t, 256, cfg.MaxLength)
	assert.True(t, cfg.DoLowerCase)
	assert.Equal(t, "[PAD]", cfg.PadToken)
	assert.Equal(t, "[CLS]", cfg.ClsToken)
	assert.Equal(t, "[SEP]", cfg.SepToken)
	assert.Equal(t, "[UNK]", cfg.UnkToken)
}

func BenchmarkWordPieceTokenizer_Encode(b *testing.B) {
	vocabJSON, _ := json.Marshal(testVocab())
	tok, _ := NewWordPieceTokenizer(vocabJSON, DefaultTokenizerConfig())

	text := "the quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tok.Encode(text)
	}
}

func BenchmarkWordPieceTokenizer_EncodeBatch(b *testing.B) {
	vocabJSON, _ := json.Marshal(testVocab())
	tok, _ := NewWordPieceTokenizer(vocabJSON, DefaultTokenizerConfig())

	texts := make([]string, 32)
	for i := range texts {
		texts[i] = "the quick brown fox jumps over the lazy dog"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tok.EncodeBatch(texts)
	}
}
