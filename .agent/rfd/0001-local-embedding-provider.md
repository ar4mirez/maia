# RFD 0001: Local Embedding Provider Implementation

**Status**: Accepted
**Author**: MAIA Team
**Created**: 2026-01-19
**Updated**: 2026-01-19

---

## Summary

Implement a local embedding provider using ONNX Runtime to generate embeddings without external API dependencies. This enables single-binary deployment and eliminates runtime costs for embedding generation.

---

## Problem Statement

MAIA currently uses a mock embedding provider that generates deterministic but semantically meaningless embeddings. For production use, MAIA needs to generate real semantic embeddings that capture the meaning of text.

**Requirements**:
1. Single-binary deployment (no external dependencies at runtime)
2. No API costs for embedding generation
3. Sub-50ms latency for single embeddings
4. Support batch embedding for efficiency
5. Model: all-MiniLM-L6-v2 (384 dimensions, good quality/speed tradeoff)

---

## Options Considered

### Option A: ONNX Runtime with CGO (Recommended)

Use `onnxruntime-go` bindings to run ONNX models directly in Go.

**Pros**:
- Native integration, no external processes
- Best performance (GPU support possible)
- Well-maintained bindings
- Industry standard for model inference

**Cons**:
- Requires CGO (complicates cross-compilation)
- Platform-specific ONNX Runtime libraries needed
- Larger binary size (~50MB with model embedded)

**Implementation**:
```go
import ort "github.com/yalue/onnxruntime_go"

type LocalProvider struct {
    session   *ort.AdvancedSession
    tokenizer *Tokenizer
    dimension int
}
```

### Option B: Pure Go with Gorgonia

Use Gorgonia/Gonum for pure Go inference.

**Pros**:
- No CGO required
- Simpler cross-compilation
- Smaller binary

**Cons**:
- Significantly slower (10x-100x)
- Complex to implement transformer architecture
- No GPU acceleration
- Would need to reimplement model from scratch

### Option C: Embedded HTTP API (Sidecar)

Bundle a small Rust/Python binary that serves embeddings over localhost.

**Pros**:
- Simpler Go code
- Flexibility in model choice
- Easy to update model independently

**Cons**:
- **Not single-binary** (violates core requirement)
- Additional process management
- IPC overhead
- Deployment complexity

### Option D: Remote API Fallback Only

Only support remote embedding APIs (OpenAI, Voyage, etc.)

**Pros**:
- Simplest implementation
- No binary size increase
- Access to best models

**Cons**:
- **Requires external API** (violates core requirement)
- Runtime costs
- Network latency
- Privacy concerns

---

## Decision

**Selected: Option A (ONNX Runtime with CGO)**

Rationale:
1. Only option that satisfies single-binary + no API dependency
2. Performance is critical for <200ms context assembly target
3. CGO complexity is manageable with proper build documentation
4. ONNX is industry standard with long-term support

---

## Implementation Plan

### Phase 1: Tokenizer Implementation

The all-MiniLM-L6-v2 model uses WordPiece tokenization. We need:

1. Load vocabulary from embedded JSON
2. Implement WordPiece tokenization algorithm
3. Handle special tokens ([CLS], [SEP], [PAD], [UNK])
4. Generate attention masks

### Phase 2: ONNX Runtime Integration

1. Add onnxruntime-go dependency
2. Embed ONNX model file in binary
3. Create session initialization
4. Implement inference pipeline

### Phase 3: Provider Implementation

```go
type LocalProvider struct {
    session    *ort.AdvancedSession
    tokenizer  *WordPieceTokenizer
    dimension  int
    maxLength  int
    closed     bool
    mu         sync.RWMutex
}

func NewLocalProvider(cfg Config) (*LocalProvider, error)
func (p *LocalProvider) Embed(ctx context.Context, text string) ([]float32, error)
func (p *LocalProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
func (p *LocalProvider) Dimension() int
func (p *LocalProvider) Close() error
```

### Phase 4: Model Embedding

Options for including the model:
1. **go:embed** - Embed in binary (preferred, ~90MB binary)
2. **Download on first use** - Smaller binary, requires network
3. **External file** - Configurable path

Decision: Use go:embed for single-binary deployment, with fallback to external file path for development.

---

## Technical Details

### Model Specifications

| Property | Value |
|----------|-------|
| Model | all-MiniLM-L6-v2 |
| Dimension | 384 |
| Max Tokens | 256 |
| Vocab Size | 30522 |
| File Size | ~90MB (ONNX) |

### Tokenization Pipeline

```
Input: "Hello world"
  ↓
Lowercase: "hello world"
  ↓
WordPiece: ["hello", "world"]
  ↓
Add special: ["[CLS]", "hello", "world", "[SEP]"]
  ↓
To IDs: [101, 7592, 2088, 102]
  ↓
Pad to maxLength: [101, 7592, 2088, 102, 0, 0, ...]
  ↓
Attention mask: [1, 1, 1, 1, 0, 0, ...]
```

### Inference Pipeline

```
Tokens + Attention Mask
  ↓
ONNX Runtime Session
  ↓
Mean Pooling (over non-padded tokens)
  ↓
L2 Normalization
  ↓
384-dim embedding vector
```

---

## Testing Strategy

1. **Unit tests**: Tokenizer correctness
2. **Integration tests**: End-to-end embedding generation
3. **Benchmarks**: Latency and throughput
4. **Semantic tests**: Similar texts have similar embeddings

### Acceptance Criteria

- [ ] Single text embedding < 50ms p99
- [ ] Batch embedding (32 texts) < 200ms p99
- [ ] Embeddings match Python implementation within 0.001 cosine similarity
- [ ] No memory leaks under sustained load

---

## Rollout Plan

1. Implement tokenizer with comprehensive tests
2. Add ONNX Runtime integration
3. Create LocalProvider implementation
4. Add configuration to switch between mock/local/remote
5. Update server to use configured provider
6. Add build documentation for CGO requirements

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| CGO cross-compilation complexity | Provide Docker-based build environment |
| ONNX Runtime platform support | Document supported platforms, provide fallback to remote API |
| Model size increases binary | Make model embedding optional via build tags |
| Performance varies by platform | Benchmark on target platforms, set expectations |

---

## Open Questions

1. Should we support GPU acceleration? (Defer to Phase 2)
2. Should we support model quantization for smaller size? (Future optimization)
3. How to handle model updates? (Version in config, rebuild required)

---

## References

- [all-MiniLM-L6-v2 on HuggingFace](https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2)
- [onnxruntime-go](https://github.com/yalue/onnxruntime_go)
- [ONNX Runtime](https://onnxruntime.ai/)
- [WordPiece tokenization](https://huggingface.co/course/chapter6/6)
