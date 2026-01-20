package proxy

import (
	"bufio"
	"context"
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
	"github.com/ar4mirez/maia/internal/inference"
	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
)

// Header constants for MAIA-specific configuration.
const (
	HeaderNamespace         = "X-MAIA-Namespace"
	HeaderSkipMemory        = "X-MAIA-Skip-Memory"
	HeaderSkipExtract       = "X-MAIA-Skip-Extract"
	HeaderTokenBudget       = "X-MAIA-Token-Budget"
	HeaderInferenceProvider = "X-MAIA-Inference-Provider"
)

// Proxy handles OpenAI-compatible API proxying with memory integration.
type Proxy struct {
	cfg       *config.ProxyConfig
	client    *Client
	injector  *Injector
	extractor *Extractor
	logger    *zap.Logger

	// inferenceRouter is an optional inference router for MAIA-managed inference.
	// When set, the proxy uses the router instead of the client for inference.
	inferenceRouter *inference.DefaultRouter

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
	Store           storage.Store
	Retriever       *retrieval.Retriever
	Assembler       *mcontext.Assembler
	Logger          *zap.Logger
	InferenceRouter *inference.DefaultRouter
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
		inferenceRouter:  deps.InferenceRouter,
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

// SetInferenceRouter sets the inference router for MAIA-managed inference.
func (p *Proxy) SetInferenceRouter(router *inference.DefaultRouter) {
	p.inferenceRouter = router
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
	var resp *ChatCompletionResponse
	var err error
	var providerUsed string

	// Use inference router if available, otherwise fall back to client
	if p.inferenceRouter != nil {
		resp, providerUsed, err = p.completeViaInference(c.Request.Context(), req, opts.InferenceProvider)
		if providerUsed != "" {
			c.Header("X-MAIA-Inference-Provider-Used", providerUsed)
		}
	} else {
		resp, err = p.client.ChatCompletion(c.Request.Context(), req, authHeader)
	}

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

// completeViaInference routes a completion request through the inference router.
// Returns the response, the provider name used, and any error.
func (p *Proxy) completeViaInference(ctx context.Context, req *ChatCompletionRequest, explicitProvider string) (*ChatCompletionResponse, string, error) {
	// Convert proxy request to inference request
	infReq := &inference.CompletionRequest{
		Model:       req.Model,
		Messages:    convertToInferenceMessages(req.Messages),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
		Stop:        req.Stop,
	}

	// Route to get the provider (for reporting which one was used)
	provider, err := p.inferenceRouter.RouteWithOptions(ctx, req.Model, explicitProvider)
	if err != nil {
		return nil, "", err
	}

	infResp, err := provider.Complete(ctx, infReq)
	if err != nil {
		return nil, provider.Name(), err
	}

	// Convert inference response back to proxy response
	return convertFromInferenceResponse(infResp), provider.Name(), nil
}

// convertToInferenceMessages converts proxy messages to inference messages.
func convertToInferenceMessages(messages []ChatMessage) []inference.Message {
	result := make([]inference.Message, len(messages))
	for i, m := range messages {
		result[i] = inference.Message{
			Role:    m.Role,
			Content: m.GetContentString(),
			Name:    m.Name,
		}
	}
	return result
}

// convertFromInferenceResponse converts an inference response to a proxy response.
func convertFromInferenceResponse(resp *inference.CompletionResponse) *ChatCompletionResponse {
	choices := make([]Choice, len(resp.Choices))
	for i, c := range resp.Choices {
		var msg *ChatMessage
		if c.Message != nil {
			msg = &ChatMessage{
				Role:    c.Message.Role,
				Content: c.Message.Content,
				Name:    c.Message.Name,
			}
		}
		choices[i] = Choice{
			Index:        c.Index,
			Message:      msg,
			FinishReason: c.FinishReason,
		}
	}

	var usage *Usage
	if resp.Usage != nil {
		usage = &Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return &ChatCompletionResponse{
		ID:                resp.ID,
		Object:            resp.Object,
		Created:           resp.Created,
		Model:             resp.Model,
		SystemFingerprint: resp.SystemFingerprint,
		Choices:           choices,
		Usage:             usage,
	}
}

// handleStreamingCompletion handles streaming chat completion.
func (p *Proxy) handleStreamingCompletion(
	c *gin.Context,
	req *ChatCompletionRequest,
	authHeader string,
	opts *requestOptions,
	injection *InjectionResult,
) {
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

	// Use inference router if available, otherwise fall back to client
	if p.inferenceRouter != nil {
		// Get provider name for header before flushing
		provider, err := p.inferenceRouter.RouteWithOptions(c.Request.Context(), req.Model, opts.InferenceProvider)
		if err == nil && provider != nil {
			c.Header("X-MAIA-Inference-Provider-Used", provider.Name())
		}
		c.Writer.Flush()
		p.streamViaInference(c, req, opts)
	} else {
		c.Writer.Flush()
		p.streamViaClient(c, req, authHeader, opts)
	}
}

// streamViaClient streams completions through the HTTP client.
func (p *Proxy) streamViaClient(
	c *gin.Context,
	req *ChatCompletionRequest,
	authHeader string,
	opts *requestOptions,
) {
	stream, err := p.client.ChatCompletionStream(c.Request.Context(), req, authHeader)
	if err != nil {
		p.handleBackendError(c, err)
		return
	}
	defer stream.Close()

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

// streamViaInference streams completions through the inference router.
func (p *Proxy) streamViaInference(
	c *gin.Context,
	req *ChatCompletionRequest,
	opts *requestOptions,
) {
	// Convert proxy request to inference request
	infReq := &inference.CompletionRequest{
		Model:       req.Model,
		Messages:    convertToInferenceMessages(req.Messages),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
		Stop:        req.Stop,
	}

	// Route to get the provider
	provider, err := p.inferenceRouter.RouteWithOptions(c.Request.Context(), req.Model, opts.InferenceProvider)
	if err != nil {
		p.handleBackendError(c, err)
		return
	}

	stream, err := provider.Stream(c.Request.Context(), infReq)
	if err != nil {
		p.handleBackendError(c, err)
		return
	}
	defer stream.Close()

	// Accumulate response for memory extraction
	infAccumulator := inference.NewAccumulator()

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
		infAccumulator.Add(chunk)

		// Convert to proxy chunk format and forward to client
		proxyChunk := convertFromInferenceChunk(chunk)
		data, err := json.Marshal(proxyChunk)
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
		go p.extractAndStoreMemoriesFromInferenceAccumulator(opts.Namespace, req.Messages, infAccumulator)
	}
}

// convertFromInferenceChunk converts an inference chunk to a proxy chunk.
func convertFromInferenceChunk(chunk *inference.CompletionChunk) *ChatCompletionChunk {
	choices := make([]Choice, len(chunk.Choices))
	for i, c := range chunk.Choices {
		var delta *ChatMessage
		if c.Delta != nil {
			delta = &ChatMessage{
				Role:    c.Delta.Role,
				Content: c.Delta.Content,
				Name:    c.Delta.Name,
			}
		}
		choices[i] = Choice{
			Index:        c.Index,
			Delta:        delta,
			FinishReason: c.FinishReason,
		}
	}

	return &ChatCompletionChunk{
		ID:                chunk.ID,
		Object:            chunk.Object,
		Created:           chunk.Created,
		Model:             chunk.Model,
		SystemFingerprint: chunk.SystemFingerprint,
		Choices:           choices,
	}
}

// extractAndStoreMemoriesFromInferenceAccumulator extracts memories from inference accumulator.
func (p *Proxy) extractAndStoreMemoriesFromInferenceAccumulator(
	namespace string,
	messages []ChatMessage,
	acc *inference.Accumulator,
) {
	assistantContent := acc.GetContent()
	if assistantContent == "" {
		return
	}

	userMessages := make([]string, 0)
	for _, msg := range messages {
		if msg.Role == "user" {
			userMessages = append(userMessages, msg.GetContentString())
		}
	}

	result, err := p.extractor.Extract(context.Background(), assistantContent, userMessages)
	if err != nil {
		p.logger.Error("memory extraction failed", zap.Error(err))
		return
	}

	if len(result.Memories) > 0 {
		if err := p.extractor.Store(context.Background(), namespace, result.Memories); err != nil {
			p.logger.Error("memory storage failed", zap.Error(err))
		} else {
			p.logger.Debug("stored extracted memories",
				zap.Int("count", len(result.Memories)),
				zap.String("namespace", namespace),
			)
		}
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
	Namespace         string
	TokenBudget       int
	SkipMemory        bool
	SkipExtract       bool
	InferenceProvider string
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

	if provider := c.GetHeader(HeaderInferenceProvider); provider != "" {
		opts.InferenceProvider = provider
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

	result, err := p.extractor.Extract(context.Background(), assistantContent, userMessages)
	if err != nil {
		p.logger.Error("memory extraction failed", zap.Error(err))
		return
	}

	if len(result.Memories) > 0 {
		if err := p.extractor.Store(context.Background(), namespace, result.Memories); err != nil {
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

	result, err := p.extractor.Extract(context.Background(), assistantContent, userMessages)
	if err != nil {
		p.logger.Error("memory extraction failed", zap.Error(err))
		return
	}

	if len(result.Memories) > 0 {
		if err := p.extractor.Store(context.Background(), namespace, result.Memories); err != nil {
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
