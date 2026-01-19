// Command mcp-server provides a Model Context Protocol (MCP) server for MAIA.
// It can be used with Claude, Cursor, and other MCP-compatible clients.
//
// Usage:
//
//	mcp-server [flags]
//
// Flags:
//
//	-data-dir string
//	      Data directory for storage (default "./data")
//	-help
//	      Show help
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/pkg/mcp"
)

var (
	dataDir = flag.String("data-dir", "./data", "Data directory for storage")
	help    = flag.Bool("help", false, "Show help")
)

func main() {
	flag.Parse()

	if *help {
		printUsage()
		os.Exit(0)
	}

	// Create config
	cfg := &config.Config{
		Storage: config.StorageConfig{
			DataDir: *dataDir,
		},
		Embedding: config.EmbeddingConfig{
			Model:      "local",
			Dimensions: 384,
		},
	}

	// Create server
	server, err := mcp.NewServer(&mcp.Options{
		Config: cfg,
	})
	if err != nil {
		log.Fatalf("Failed to create MCP server: %v", err)
	}
	defer server.Close()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	// Run the server
	if err := server.Run(ctx); err != nil {
		if ctx.Err() == nil {
			log.Fatalf("MCP server error: %v", err)
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `MAIA MCP Server

A Model Context Protocol server for MAIA memory system.

Usage:
  mcp-server [flags]

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Environment Variables:
  MAIA_STORAGE_DATA_DIR    Data directory for storage
  MAIA_EMBEDDING_MODEL     Embedding model (local, openai, voyage)

Example:
  # Start with default settings
  mcp-server

  # Start with custom data directory
  mcp-server -data-dir /path/to/data

  # For use with Claude Desktop, add to config:
  {
    "mcpServers": {
      "maia": {
        "command": "/path/to/mcp-server",
        "args": ["-data-dir", "/path/to/data"]
      }
    }
  }
`)
}
