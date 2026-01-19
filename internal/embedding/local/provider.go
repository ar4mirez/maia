package local

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/ar4mirez/maia/internal/embedding"
)

// Common errors for local embedding operations.
var (
	ErrModelNotLoaded    = errors.New("model not loaded")
	ErrTokenizerNotReady = errors.New("tokenizer not ready")
	ErrInvalidDimension  = errors.New("invalid embedding dimension")
	ErrEmptyBatch        = errors.New("empty batch")
)

// ProviderConfig holds configuration for the local embedding provider.
type ProviderConfig struct {
	// ModelPath is the path to the ONNX model file.
	ModelPath string

	// VocabPath is the path to the vocabulary JSON file.
	VocabPath string

	// Dimension is the embedding dimension (384 for all-MiniLM-L6-v2).
	Dimension int

	// MaxLength is the maximum sequence length.
	MaxLength int

	// DoLowerCase indicates whether to lowercase input text.
	DoLowerCase bool

	// UseGPU enables GPU acceleration if available.
	UseGPU bool
}

// DefaultProviderConfig returns the default configuration for all-MiniLM-L6-v2.
func DefaultProviderConfig() ProviderConfig {
	return ProviderConfig{
		Dimension:   384,
		MaxLength:   256,
		DoLowerCase: true,
		UseGPU:      false,
	}
}

// Provider implements local embedding generation using ONNX Runtime.
type Provider struct {
	config    ProviderConfig
	tokenizer *WordPieceTokenizer
	session   *ort.DynamicAdvancedSession

	closed bool
	mu     sync.RWMutex
}

// NewProvider creates a new local embedding provider.
func NewProvider(cfg ProviderConfig, vocabJSON []byte) (*Provider, error) {
	if cfg.Dimension <= 0 {
		return nil, ErrInvalidDimension
	}

	// Initialize ONNX Runtime environment
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to initialize ONNX Runtime: %w", err)
	}

	// Create tokenizer
	tokCfg := TokenizerConfig{
		MaxLength:   cfg.MaxLength,
		DoLowerCase: cfg.DoLowerCase,
		PadToken:    DefaultPadToken,
		ClsToken:    DefaultClsToken,
		SepToken:    DefaultSepToken,
		UnkToken:    DefaultUnkToken,
	}

	tokenizer, err := NewWordPieceTokenizer(vocabJSON, tokCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenizer: %w", err)
	}

	// Create ONNX session
	session, err := createSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}

	return &Provider{
		config:    cfg,
		tokenizer: tokenizer,
		session:   session,
	}, nil
}

// createSession creates the ONNX Runtime session.
func createSession(cfg ProviderConfig) (*ort.DynamicAdvancedSession, error) {
	if cfg.ModelPath == "" {
		return nil, errors.New("model path is required")
	}

	// Create session options
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to create session options: %w", err)
	}
	defer func() { _ = options.Destroy() }()

	// Set thread count for CPU
	if err := options.SetIntraOpNumThreads(4); err != nil {
		return nil, fmt.Errorf("failed to set thread count: %w", err)
	}

	// Define input/output names for the BERT-style model
	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	// Create dynamic session for variable batch sizes
	session, err := ort.NewDynamicAdvancedSession(
		cfg.ModelPath,
		inputNames,
		outputNames,
		options,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}

	return session, nil
}

// Embed generates an embedding for a single text.
func (p *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, embedding.ErrProviderClosed
	}
	p.mu.RUnlock()

	if text == "" {
		return nil, embedding.ErrEmptyText
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Tokenize
	encoded := p.tokenizer.Encode(text)

	// Run inference
	embeddings, err := p.runInference([][]int64{encoded.InputIDs},
		[][]int64{encoded.AttentionMask}, [][]int64{encoded.TokenTypeIDs})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, errors.New("no embeddings returned")
	}

	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (p *Provider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, embedding.ErrProviderClosed
	}
	p.mu.RUnlock()

	if len(texts) == 0 {
		return nil, ErrEmptyBatch
	}

	for _, text := range texts {
		if text == "" {
			return nil, embedding.ErrEmptyText
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Tokenize all texts
	batch := p.tokenizer.EncodeBatch(texts)

	// Run inference
	return p.runInference(batch.InputIDs, batch.AttentionMask, batch.TokenTypeIDs)
}

// runInference executes the ONNX model.
func (p *Provider) runInference(inputIDs, attentionMask, tokenTypeIDs [][]int64) ([][]float32, error) {
	batchSize := len(inputIDs)
	seqLen := p.config.MaxLength

	// Flatten inputs
	flatInputIDs := flatten2D(inputIDs)
	flatAttentionMask := flatten2D(attentionMask)
	flatTokenTypeIDs := flatten2D(tokenTypeIDs)

	// Create input tensors
	inputShape := ort.NewShape(int64(batchSize), int64(seqLen))

	inputIDsTensor, err := ort.NewTensor(inputShape, flatInputIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer func() { _ = inputIDsTensor.Destroy() }()

	attentionMaskTensor, err := ort.NewTensor(inputShape, flatAttentionMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer func() { _ = attentionMaskTensor.Destroy() }()

	tokenTypeIDsTensor, err := ort.NewTensor(inputShape, flatTokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
	}
	defer func() { _ = tokenTypeIDsTensor.Destroy() }()

	// Create output tensor
	outputShape := ort.NewShape(int64(batchSize), int64(seqLen), int64(p.config.Dimension))
	outputData := make([]float32, batchSize*seqLen*p.config.Dimension)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer func() { _ = outputTensor.Destroy() }()

	// Run inference
	err = p.session.Run(
		[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
		[]ort.ArbitraryTensor{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to run inference: %w", err)
	}

	// Apply mean pooling over non-padded tokens
	embeddings := make([][]float32, batchSize)
	for i := 0; i < batchSize; i++ {
		embeddings[i] = p.meanPooling(outputData, attentionMask[i], i, seqLen)
	}

	return embeddings, nil
}

// meanPooling applies mean pooling over the token embeddings.
func (p *Provider) meanPooling(hiddenStates []float32, attentionMask []int64, batchIdx, seqLen int) []float32 {
	dim := p.config.Dimension
	result := make([]float32, dim)

	// Count non-padded tokens
	var tokenCount float32
	for _, mask := range attentionMask {
		tokenCount += float32(mask)
	}
	if tokenCount == 0 {
		tokenCount = 1 // Avoid division by zero
	}

	// Sum embeddings for non-padded tokens
	batchOffset := batchIdx * seqLen * dim
	for tokenIdx := 0; tokenIdx < seqLen; tokenIdx++ {
		if attentionMask[tokenIdx] == 0 {
			continue
		}
		tokenOffset := batchOffset + tokenIdx*dim
		for d := 0; d < dim; d++ {
			result[d] += hiddenStates[tokenOffset+d]
		}
	}

	// Compute mean
	for d := 0; d < dim; d++ {
		result[d] /= tokenCount
	}

	// Normalize to unit length
	return embedding.Normalize(result)
}

// Dimension returns the embedding dimension.
func (p *Provider) Dimension() int {
	return p.config.Dimension
}

// Close releases resources.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	if p.session != nil {
		if err := p.session.Destroy(); err != nil {
			return fmt.Errorf("failed to destroy session: %w", err)
		}
	}

	// Cleanup ONNX Runtime environment
	if err := ort.DestroyEnvironment(); err != nil {
		return fmt.Errorf("failed to destroy ONNX Runtime environment: %w", err)
	}

	return nil
}

// flatten2D flattens a 2D slice into a 1D slice.
func flatten2D(data [][]int64) []int64 {
	if len(data) == 0 {
		return nil
	}

	totalLen := len(data) * len(data[0])
	result := make([]int64, totalLen)

	for i, row := range data {
		copy(result[i*len(row):], row)
	}

	return result
}

// Ensure Provider implements embedding.Provider interface.
var _ embedding.Provider = (*Provider)(nil)
