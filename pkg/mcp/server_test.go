package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/internal/embedding"
	"github.com/ar4mirez/maia/internal/index/fulltext"
	"github.com/ar4mirez/maia/internal/index/vector"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create temporary store
	tmpDir := t.TempDir()
	store, err := badger.New(&badger.Options{DataDir: tmpDir})
	require.NoError(t, err)

	// Create mock provider
	provider := embedding.NewMockProvider(384)

	// Create vector index
	vectorIndex := vector.NewHNSWIndex(vector.DefaultConfig(384))

	// Create text index
	textIndex, err := fulltext.NewBleveIndex(fulltext.Config{InMemory: true})
	require.NoError(t, err)

	// Create server
	server, err := NewServer(&Options{
		Config:      &config.Config{},
		Store:       store,
		Provider:    provider,
		VectorIndex: vectorIndex,
		TextIndex:   textIndex,
	})
	require.NoError(t, err)

	cleanup := func() {
		server.Close()
		textIndex.Close()
	}

	return server, cleanup
}

func TestNewServer_WithNilOptions(t *testing.T) {
	_, err := NewServer(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "options cannot be nil")
}

func TestNewServer_WithValidOptions(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	assert.NotNil(t, server)
	assert.NotNil(t, server.MCPServer())
}

func TestServer_RememberAndRecall(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a memory directly using the store
	mem, err := server.store.CreateMemory(ctx, &storage.CreateMemoryInput{
		Namespace:  "default",
		Content:    "Test memory content for MCP server",
		Type:       storage.MemoryTypeSemantic,
		Confidence: 1.0,
		Source:     storage.MemorySourceUser,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	// List memories
	memories, err := server.store.ListMemories(ctx, "default", &storage.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, memories, 1)
	assert.Equal(t, "Test memory content for MCP server", memories[0].Content)
}

func TestServer_Close(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	err := server.Close()
	assert.NoError(t, err)

	// Close again should be safe (idempotent)
	err = server.Close()
	assert.NoError(t, err)
}
