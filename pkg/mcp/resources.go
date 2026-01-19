package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ar4mirez/maia/internal/storage"
)

// registerResources registers all MAIA resources with the MCP server.
func (s *Server) registerResources() {
	// Register resource templates
	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "maia://namespaces",
		Name:        "Namespaces",
		Description: "List all MAIA namespaces",
		MIMEType:    "application/json",
	}, s.handleNamespacesResource)

	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "maia://namespace/{namespace}/memories",
		Name:        "Namespace Memories",
		Description: "List memories in a specific namespace",
		MIMEType:    "application/json",
	}, s.handleMemoriesResource)

	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "maia://memory/{id}",
		Name:        "Memory",
		Description: "Get a specific memory by ID",
		MIMEType:    "application/json",
	}, s.handleMemoryResource)

	s.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "maia://stats",
		Name:        "Statistics",
		Description: "Get MAIA storage statistics",
		MIMEType:    "application/json",
	}, s.handleStatsResource)
}

func (s *Server) handleNamespacesResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	namespaces, err := s.store.ListNamespaces(ctx, &storage.ListOptions{Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	// Convert to simpler format
	type nsInfo struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Parent    string `json:"parent,omitempty"`
		CreatedAt string `json:"created_at"`
	}

	infos := make([]nsInfo, 0, len(namespaces))
	for _, ns := range namespaces {
		infos = append(infos, nsInfo{
			ID:        ns.ID,
			Name:      ns.Name,
			Parent:    ns.Parent,
			CreatedAt: ns.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	data, err := json.MarshalIndent(infos, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal namespaces: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func (s *Server) handleMemoriesResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	// Extract namespace from URI
	// URI format: maia://namespace/{namespace}/memories
	namespace := extractURIParam(req.Params.URI, "namespace")
	if namespace == "" {
		namespace = "default"
	}

	memories, err := s.store.ListMemories(ctx, namespace, &storage.ListOptions{Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("failed to list memories: %w", err)
	}

	// Convert to simpler format
	type memInfo struct {
		ID         string   `json:"id"`
		Content    string   `json:"content"`
		Type       string   `json:"type"`
		Tags       []string `json:"tags,omitempty"`
		CreatedAt  string   `json:"created_at"`
		AccessedAt string   `json:"accessed_at"`
	}

	infos := make([]memInfo, 0, len(memories))
	for _, m := range memories {
		infos = append(infos, memInfo{
			ID:         m.ID,
			Content:    truncate(m.Content, 200),
			Type:       string(m.Type),
			Tags:       m.Tags,
			CreatedAt:  m.CreatedAt.Format("2006-01-02T15:04:05Z"),
			AccessedAt: m.AccessedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	data, err := json.MarshalIndent(infos, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal memories: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func (s *Server) handleMemoryResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	// Extract ID from URI
	// URI format: maia://memory/{id}
	id := extractURIParam(req.Params.URI, "id")
	if id == "" {
		return nil, fmt.Errorf("memory ID is required")
	}

	memory, err := s.store.GetMemory(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	data, err := json.MarshalIndent(memory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal memory: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func (s *Server) handleStatsResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	stats, err := s.store.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal stats: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

// extractURIParam extracts a parameter value from a URI.
// For URI like "maia://namespace/myns/memories", extractURIParam(uri, "namespace") returns "myns".
func extractURIParam(uri, param string) string {
	// Simple parsing - could be improved with a proper URI template library
	switch param {
	case "namespace":
		// URI: maia://namespace/{namespace}/memories
		if len(uri) > len("maia://namespace/") {
			rest := uri[len("maia://namespace/"):]
			for i, c := range rest {
				if c == '/' {
					return rest[:i]
				}
			}
			return rest
		}
	case "id":
		// URI: maia://memory/{id}
		if len(uri) > len("maia://memory/") {
			return uri[len("maia://memory/"):]
		}
	}
	return ""
}
