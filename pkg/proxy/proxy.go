package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ar4mirez/maia/internal/config"
	mcontext "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
)

// Header constants for MAIA-specific configuration.
const (
	HeaderNamespace   = "X-MAIA-Namespace"
	HeaderSkipMemory  = "X-MAIA-Skip-Memory"
	HeaderSkipExtract = "X-MAIA-Skip-Extract"
	HeaderTokenBudget = "X-MAIA-Token-Budget"
)

// Proxy handles OpenAI-compatible API proxying with memory integration.
type Proxy struct {
	cfg       *config.ProxyConfig
	client    *Client
	injector  *Injector
	extractor *Extractor
	logger    *zap.Logger

	defaultNamespace string
	defaultBudget    int
}

// ProxyConfig holds configuration for the proxy.
type ProxyConfig struct {
	Backend          string
	AutoRemember     bool
	AutoContext      bool
	ContextPosition  ContextPosition
	TokenBudget      int
	DefaultNamespace string
	Timeout          time.Duration
}

// ProxyDeps holds dependencies for the proxy.
type ProxyDeps struct {
	Store     storage.Store
	Retriever *retrieval.Retriever
	Assembler *mcontext.Assembler
	Logger    *zap.Logger
}

// NewProxy creates a new OpenAI-compatible proxy.
func NewProxy(cfg *ProxyConfig, deps *ProxyDeps) *Proxy {
	client := NewClient(&ClientConfig{
		BaseURL: cfg.Backend,
		Timeout: cfg.Timeout,
	})

	var injector *Injector
	if deps.Retriever != nil && deps.Assembler != nil {
		injector = NewInjector(deps.Retriever, deps.Assembler)
	}

	var extractor *Extractor
	if deps.Store != nil && cfg.AutoRemember {
		extractor = NewExtractor(deps.Store)
	}

	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Proxy{
		client:           client,
		injector:         injector,
		extractor:        extractor,
		logger:           logger,
		defaultNamespace: cfg.DefaultNamespace,
		defaultBudget:    cfg.TokenBudget,
		cfg: &config.ProxyConfig{
			Backend:         cfg.Backend,
			AutoRemember:    cfg.AutoRemember,
			AutoContext:     cfg.AutoContext,
			ContextPosition: string(cfg.ContextPosition),
			TokenBudget:     cfg.TokenBudget,
		},
	}
}

// RegisterRoutes registers the proxy routes on a Gin router.
func (p *Proxy) RegisterRoutes(r *gin.Engine) {
	// OpenAI-compatible routes
	r.POST("/v1/chat/completions", p.handleChatCompletion)
	r.GET("/v1/models", p.handleListModels)

	// Alternative proxy routes
	proxy := r.Group("/proxy")
	proxy.POST("/v1/chat/completions", p.handleChatCompletion)
	proxy.GET("/v1/models", p.handleListModels)
}

// handleChatCompletion handles chat completion requests.
func (p *Proxy) handleChatCompletion(c *gin.Context) {
	// Parse request
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		p.sendError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Extract MAIA-specific options from headers
	opts := p.extractOptions(c)

	// Get authorization header for passthrough
	authHeader := c.GetHeader("Authorization")

	// Inject context if enabled
	messages := req.Messages
	var injectionResult *InjectionResult

	if p.cfg.AutoContext && !opts.SkipMemory && p.injector != nil {
		var err error
		injectionResult, err = p.injector.InjectContext(c.Request.Context(), messages, &InjectionOptions{
			Namespace:   opts.Namespace,
			TokenBudget: opts.TokenBudget,
			Position:    ContextPosition(p.cfg.ContextPosition),
		})
		if err != nil {
			p.logger.Error("context injection failed", zap.Error(err))
			// Continue without injection
		} else {
			messages = injectionResult.Messages
		}
	}

	// Update request with potentially modified messages
	req.Messages = messages

	// Handle streaming vs non-streaming
	if req.Stream {
		p.handleStreamingCompletion(c, &req, authHeader, opts, injectionResult)
	} else {
		p.handleNonStreamingCompletion(c, &req, authHeader, opts, injectionResult)
	}
}

// handleNonStreamingCompletion handles non-streaming chat completion.
func (p *Proxy) handleNonStreamingCompletion(
	c *gin.Context,
	req *ChatCompletionRequest,
	authHeader string,
	opts *requestOptions,
	injection *InjectionResult,
) {
	resp, err := p.client.ChatCompletion(c.Request.Context(), req, authHeader)
	if err != nil {
		p.handleBackendError(c, err)
		return
	}

	// Extract memories from response if enabled
	if p.cfg.AutoRemember && !opts.SkipExtract && p.extractor != nil {
		go p.extractAndStoreMemories(opts.Namespace, req.Messages, resp)
	}

	// Add MAIA metadata if injection occurred
	if injection != nil && injection.MemoriesUsed > 0 {
		c.Header("X-MAIA-Memories-Used", strconv.Itoa(injection.MemoriesUsed))
		c.Header("X-MAIA-Tokens-Injected", strconv.Itoa(injection.TokensInjected))
	}

	c.JSON(http.StatusOK, resp)
}

// handleStreamingCompletion handles streaming chat completion.
func (p *Proxy) handleStreamingCompletion(
	c *gin.Context,
	req *ChatCompletionRequest,
	authHeader string,
	opts *requestOptions,
	injection *InjectionResult,
) {
	stream, err := p.client.ChatCompletionStream(c.Request.Context(), req, authHeader)
	if err != nil {
		p.handleBackendError(c, err)
		return
	}
	defer stream.Close()

	// Set headers for SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Add MAIA metadata if injection occurred
	if injection != nil && injection.MemoriesUsed > 0 {
		c.Header("X-MAIA-Memories-Used", strconv.Itoa(injection.MemoriesUsed))
		c.Header("X-MAIA-Tokens-Injected", strconv.Itoa(injection.TokensInjected))
	}

	c.Writer.Flush()

	// Accumulate response for memory extraction
	accumulator := NewAccumulator()

	// Create a writer that flushes after each write
	writer := bufio.NewWriter(c.Writer)

	for {
		chunk, err := stream.Read()
		if err == io.EOF {
			// Write final done message
			_, _ = writer.WriteString("data: [DONE]\n\n")
			writer.Flush()
			break
		}
		if err != nil {
			p.logger.Error("stream read error", zap.Error(err))
			break
		}

		// Accumulate for extraction
		accumulator.Add(chunk)

		// Forward chunk to client
		data, err := json.Marshal(chunk)
		if err != nil {
			p.logger.Error("marshal chunk error", zap.Error(err))
			continue
		}

		_, err = writer.WriteString("data: " + string(data) + "\n\n")
		if err != nil {
			p.logger.Error("write chunk error", zap.Error(err))
			break
		}
		writer.Flush()
	}

	// Extract memories from accumulated response
	if p.cfg.AutoRemember && !opts.SkipExtract && p.extractor != nil {
		go p.extractAndStoreMemoriesFromAccumulator(opts.Namespace, req.Messages, accumulator)
	}
}

// handleListModels handles the list models endpoint.
func (p *Proxy) handleListModels(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")

	resp, err := p.client.ListModels(c.Request.Context(), authHeader)
	if err != nil {
		p.handleBackendError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// requestOptions holds per-request options.
type requestOptions struct {
	Namespace   string
	TokenBudget int
	SkipMemory  bool
	SkipExtract bool
}

// extractOptions extracts MAIA-specific options from request headers.
func (p *Proxy) extractOptions(c *gin.Context) *requestOptions {
	opts := &requestOptions{
		Namespace:   p.defaultNamespace,
		TokenBudget: p.defaultBudget,
	}

	if ns := c.GetHeader(HeaderNamespace); ns != "" {
		opts.Namespace = ns
	}

	if skip := c.GetHeader(HeaderSkipMemory); skip == "true" || skip == "1" {
		opts.SkipMemory = true
	}

	if skip := c.GetHeader(HeaderSkipExtract); skip == "true" || skip == "1" {
		opts.SkipExtract = true
	}

	if budget := c.GetHeader(HeaderTokenBudget); budget != "" {
		if b, err := strconv.Atoi(budget); err == nil && b > 0 {
			opts.TokenBudget = b
		}
	}

	return opts
}

// extractAndStoreMemories extracts and stores memories from a response.
func (p *Proxy) extractAndStoreMemories(
	namespace string,
	messages []ChatMessage,
	resp *ChatCompletionResponse,
) {
	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return
	}

	assistantContent := resp.Choices[0].Message.GetContentString()

	// Extract user messages for context
	userMessages := make([]string, 0)
	for _, msg := range messages {
		if msg.Role == "user" {
			userMessages = append(userMessages, msg.GetContentString())
		}
	}

	result, err := p.extractor.Extract(nil, assistantContent, userMessages)
	if err != nil {
		p.logger.Error("memory extraction failed", zap.Error(err))
		return
	}

	if len(result.Memories) > 0 {
		if err := p.extractor.Store(nil, namespace, result.Memories); err != nil {
			p.logger.Error("memory storage failed", zap.Error(err))
		} else {
			p.logger.Debug("stored extracted memories",
				zap.Int("count", len(result.Memories)),
				zap.String("namespace", namespace),
			)
		}
	}
}

// extractAndStoreMemoriesFromAccumulator extracts memories from streaming accumulator.
func (p *Proxy) extractAndStoreMemoriesFromAccumulator(
	namespace string,
	messages []ChatMessage,
	acc *Accumulator,
) {
	assistantContent := acc.GetAssistantContent()
	if assistantContent == "" {
		return
	}

	userMessages := make([]string, 0)
	for _, msg := range messages {
		if msg.Role == "user" {
			userMessages = append(userMessages, msg.GetContentString())
		}
	}

	result, err := p.extractor.Extract(nil, assistantContent, userMessages)
	if err != nil {
		p.logger.Error("memory extraction failed", zap.Error(err))
		return
	}

	if len(result.Memories) > 0 {
		if err := p.extractor.Store(nil, namespace, result.Memories); err != nil {
			p.logger.Error("memory storage failed", zap.Error(err))
		} else {
			p.logger.Debug("stored extracted memories",
				zap.Int("count", len(result.Memories)),
				zap.String("namespace", namespace),
			)
		}
	}
}

// sendError sends an OpenAI-compatible error response.
func (p *Proxy) sendError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, ErrorResponse{
		Error: &APIError{
			Type:    errType,
			Message: message,
		},
	})
}

// handleBackendError handles errors from the backend.
func (p *Proxy) handleBackendError(c *gin.Context, err error) {
	if be, ok := err.(*BackendError); ok {
		p.sendError(c, be.StatusCode, be.Type, be.Message)
		return
	}

	// Check for context errors
	if strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "context canceled") {
		p.sendError(c, http.StatusGatewayTimeout, "timeout", "Request to backend timed out")
		return
	}

	p.logger.Error("backend error", zap.Error(err))
	p.sendError(c, http.StatusBadGateway, "backend_error",
		fmt.Sprintf("Backend request failed: %v", err))
}
