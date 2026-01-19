# RFD 0002: OpenAI-Compatible Proxy

**Status**: Accepted
**Created**: 2026-01-19
**Author**: AI Principal Engineer

---

## Context

MAIA needs to provide a seamless way for applications to use memory-enhanced LLM interactions without requiring code changes. The OpenAI API is the de facto standard for LLM interactions, and many applications already use it.

## Problem Statement

1. Applications using OpenAI-compatible APIs cannot benefit from MAIA's memory without code modifications
2. Manual memory management adds friction to LLM-powered applications
3. Context injection requires explicit API calls and coordination

## Proposal

Implement an OpenAI-compatible proxy that:
1. Accepts standard OpenAI chat completion requests
2. Automatically retrieves relevant memories based on the conversation
3. Injects assembled context into the request before forwarding
4. Extracts and stores relevant information from responses
5. Forwards to any OpenAI-compatible backend (OpenAI, Anthropic, Ollama, etc.)

## Design

### Architecture

```
┌─────────────┐     ┌─────────────────────────┐     ┌─────────────┐
│  Client     │────▶│    MAIA Proxy           │────▶│  Backend    │
│  (OpenAI    │     │  ┌─────────────────┐    │     │  (OpenAI,   │
│   SDK)      │◀────│  │ Memory Inject   │    │◀────│   Ollama)   │
└─────────────┘     │  │ Response Extract│    │     └─────────────┘
                    │  └─────────────────┘    │
                    │           │             │
                    │           ▼             │
                    │  ┌─────────────────┐    │
                    │  │   MAIA Store    │    │
                    │  └─────────────────┘    │
                    └─────────────────────────┘
```

### Request Flow

1. **Receive Request**: Accept OpenAI-compatible chat completion request
2. **Extract Namespace**: From header `X-MAIA-Namespace` or infer from API key
3. **Retrieve Context**: Use last user message + conversation history as query
4. **Inject Context**: Add assembled context based on `context_position` config:
   - `system`: Prepend to system message or create one
   - `first_user`: Prepend to first user message
   - `before_last`: Insert before last user message
5. **Forward Request**: Send modified request to backend
6. **Stream/Return Response**: Return response to client
7. **Extract Memories**: Asynchronously extract facts from assistant response (if auto_remember enabled)

### API Endpoints

```
POST /v1/chat/completions     # Main proxy endpoint
POST /proxy/v1/chat/completions  # Alternative path
GET  /v1/models               # List available models (passthrough)
```

### Request Headers

| Header | Description | Default |
|--------|-------------|---------|
| `X-MAIA-Namespace` | Target namespace for memory operations | from config |
| `X-MAIA-Skip-Memory` | Skip memory retrieval for this request | false |
| `X-MAIA-Skip-Extract` | Skip memory extraction from response | false |
| `X-MAIA-Token-Budget` | Override token budget for context | from config |
| `Authorization` | Bearer token for backend (passed through) | required |

### Configuration

```yaml
proxy:
  backend: "https://api.openai.com/v1"  # Backend URL
  auto_remember: true                    # Extract memories from responses
  auto_context: true                     # Inject context into requests
  context_position: "system"             # Where to inject context
  token_budget: 4000                     # Default token budget
  default_namespace: "default"           # Default namespace if not specified
  rate_limit_rps: 100                    # Requests per second limit
  timeout: 60s                           # Backend request timeout
  stream_buffer_size: 4096               # Buffer size for streaming
```

### Memory Extraction Strategy

For `auto_remember`, we extract memories from assistant responses using:

1. **Heuristic extraction**: Look for patterns like:
   - "I'll remember that..."
   - "You mentioned..."
   - "Your preference is..."
   - Statements about user facts

2. **Structured extraction** (future): Use an LLM to extract structured facts

### Context Injection Format

```json
{
  "role": "system",
  "content": "[Previous context from MAIA]\n\n{assembled_context}\n\n[End of context]\n\n{original_system_prompt}"
}
```

### Streaming Support

The proxy must support SSE streaming for chat completions:
1. Buffer the initial response headers
2. Stream data chunks as they arrive from backend
3. Accumulate response for memory extraction (async)
4. Handle connection errors gracefully

## Options Considered

### Option A: Full Proxy (Selected)

Proxy all requests through MAIA, injecting context and extracting memories.

**Pros**:
- Transparent to clients
- Full control over request/response
- Can support streaming

**Cons**:
- Added latency
- Requires careful error handling

### Option B: Sidecar Pattern

Run MAIA as a sidecar that applications call before/after LLM requests.

**Pros**:
- Simpler implementation
- No proxy latency

**Cons**:
- Requires application changes
- Doesn't support existing applications

### Option C: Middleware Library

Provide SDK middleware that wraps OpenAI clients.

**Pros**:
- Language-specific optimizations
- No network hop to proxy

**Cons**:
- Requires code changes
- Multiple implementations needed

## Decision

**Selected: Option A - Full Proxy**

Rationale:
- Zero code changes for existing applications
- Consistent behavior across all clients
- Streaming complexity is manageable with proper buffering
- Latency is acceptable given memory retrieval value

## Implementation Plan

### Phase 1: Basic Proxy

1. Create `pkg/proxy/proxy.go` - Main proxy handler
2. Create `pkg/proxy/types.go` - OpenAI API types
3. Create `pkg/proxy/client.go` - Backend client with streaming
4. Implement basic request forwarding
5. Add context injection (without extraction)

### Phase 2: Memory Integration

1. Implement memory retrieval using existing retriever
2. Implement context assembly integration
3. Add header-based configuration
4. Add namespace resolution

### Phase 3: Response Extraction

1. Implement response accumulation for streaming
2. Add heuristic memory extraction
3. Add async memory storage
4. Handle extraction errors gracefully

### Phase 4: Production Hardening

1. Add rate limiting middleware
2. Add comprehensive error handling
3. Add metrics and logging
4. Add retry logic for backend failures

## File Structure

```
pkg/proxy/
├── proxy.go       # Main proxy handler
├── types.go       # OpenAI API types (ChatCompletionRequest, etc.)
├── client.go      # HTTP client for backend
├── inject.go      # Context injection logic
├── extract.go     # Memory extraction from responses
├── stream.go      # SSE streaming handler
└── proxy_test.go  # Tests
```

## Test Strategy

1. Unit tests for each component
2. Integration tests with mock backend
3. Streaming tests with SSE verification
4. End-to-end tests with actual MAIA storage

## Success Criteria

- [ ] Chat completions proxied successfully
- [ ] Streaming works correctly
- [ ] Context injection improves response quality
- [ ] Memory extraction captures relevant facts
- [ ] Latency < 100ms overhead (excluding backend)
- [ ] Test coverage > 80%

## References

- [OpenAI API Reference](https://platform.openai.com/docs/api-reference/chat)
- [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)
