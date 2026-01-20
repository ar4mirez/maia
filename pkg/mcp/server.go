// Package mcp provides a Model Context Protocol (MCP) server implementation for MAIA.
// It exposes MAIA's memory capabilities as MCP tools, resources, and prompts.
package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ar4mirez/maia/internal/config"
	ctxpkg "github.com/ar4mirez/maia/internal/context"
	"github.com/ar4mirez/maia/internal/embedding"
	"github.com/ar4mirez/maia/internal/index/fulltext"
	"github.com/ar4mirez/maia/internal/index/vector"
	"github.com/ar4mirez/maia/internal/inference"
	"github.com/ar4mirez/maia/internal/retrieval"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

// Server wraps the MCP server with MAIA-specific functionality.
type Server struct {
	server          *mcp.Server
	store           storage.Store
	retriever       *retrieval.Retriever
	assembler       *ctxpkg.Assembler
	provider        embedding.Provider
	cfg             *config.Config
	inferenceRouter *inference.DefaultRouter
}

// Options configures the MCP server.
type Options struct {
	Config          *config.Config
	Store           storage.Store
	Provider        embedding.Provider
	VectorIndex     vector.Index
	TextIndex       fulltext.Index
	InferenceRouter *inference.DefaultRouter
}

// NewServer creates a new MCP server for MAIA.
func NewServer(opts *Options) (*Server, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	// Create MCP server
	impl := &mcp.Implementation{
		Name:    "maia",
		Version: "1.0.0",
	}
	mcpServer := mcp.NewServer(impl, nil)

	// Create components if not provided
	var store storage.Store = opts.Store
	var err error
	if store == nil && opts.Config != nil {
		store, err = badger.NewWithPath(opts.Config.Storage.DataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create store: %w", err)
		}
	}

	// Create embedding provider
	provider := opts.Provider
	if provider == nil {
		provider = embedding.NewMockProvider(384)
	}

	// Create indices if not provided
	vectorIndex := opts.VectorIndex
	if vectorIndex == nil {
		vectorIndex = vector.NewHNSWIndex(vector.DefaultConfig(384))
	}

	var textIndex fulltext.Index = opts.TextIndex
	if textIndex == nil {
		textIndex, err = fulltext.NewBleveIndex(fulltext.Config{InMemory: true})
		if err != nil {
			return nil, fmt.Errorf("failed to create text index: %w", err)
		}
	}

	// Create retriever
	retriever := retrieval.NewRetriever(
		store,
		vectorIndex,
		textIndex,
		provider,
		retrieval.DefaultConfig(),
	)

	// Create assembler
	assembler := ctxpkg.NewAssembler(ctxpkg.DefaultAssemblerConfig())

	s := &Server{
		server:          mcpServer,
		store:           store,
		retriever:       retriever,
		assembler:       assembler,
		provider:        provider,
		cfg:             opts.Config,
		inferenceRouter: opts.InferenceRouter,
	}

	// Register tools
	s.registerTools()

	// Register resources
	s.registerResources()

	// Register prompts
	s.registerPrompts()

	return s, nil
}

// Run starts the MCP server using stdio transport.
func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

// Close cleans up server resources.
func (s *Server) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

// MCPServer returns the underlying MCP server for testing.
func (s *Server) MCPServer() *mcp.Server {
	return s.server
}
