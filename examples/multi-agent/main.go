// Package main demonstrates multi-agent memory sharing with MAIA.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ar4mirez/maia/pkg/maia"
)

var (
	agentType = flag.String("agent", "research", "Agent type: research, code, review")
	namespace = flag.String("namespace", "project:demo", "Project namespace")
	serverURL = flag.String("server", "http://localhost:8080", "MAIA server URL")
)

func main() {
	flag.Parse()

	client := maia.New(
		maia.WithBaseURL(*serverURL),
		maia.WithTimeout(10*time.Second),
	)

	ctx := context.Background()

	// Each agent has its own namespace for storing findings
	agentNamespace := fmt.Sprintf("agent:%s", *agentType)

	fmt.Printf("Starting %s agent\n", *agentType)
	fmt.Printf("Agent namespace: %s\n", agentNamespace)
	fmt.Printf("Project namespace: %s\n\n", *namespace)

	switch *agentType {
	case "research":
		runResearchAgent(ctx, client, agentNamespace, *namespace)
	case "code":
		runCodeAgent(ctx, client, agentNamespace, *namespace)
	case "review":
		runReviewAgent(ctx, client, agentNamespace, *namespace)
	default:
		log.Fatalf("Unknown agent type: %s", *agentType)
	}
}

// runResearchAgent simulates a research agent discovering information.
func runResearchAgent(ctx context.Context, client *maia.Client, agentNS, projectNS string) {
	fmt.Println("=== Research Agent: Discovering Information ===")

	// Research agent discovers facts about the project
	discoveries := []maia.CreateMemoryInput{
		{
			Namespace: agentNS,
			Content:   "The codebase uses React 18 with TypeScript 5.0 and Vite as the build tool",
			Type:      maia.MemoryTypeSemantic,
			Tags:      []string{"tech-stack", "frontend", "build"},
			Source:    maia.MemorySourceExtracted,
		},
		{
			Namespace: agentNS,
			Content:   "Authentication is handled via JWT tokens stored in httpOnly cookies with 24h expiry",
			Type:      maia.MemoryTypeSemantic,
			Tags:      []string{"auth", "security", "architecture"},
			Source:    maia.MemorySourceExtracted,
		},
		{
			Namespace: agentNS,
			Content:   "The API follows REST conventions with OpenAPI 3.0 specification at /api/docs",
			Type:      maia.MemoryTypeSemantic,
			Tags:      []string{"api", "documentation", "rest"},
			Source:    maia.MemorySourceExtracted,
		},
		{
			Namespace: agentNS,
			Content:   "Database is PostgreSQL 15 with Prisma ORM for type-safe queries",
			Type:      maia.MemoryTypeSemantic,
			Tags:      []string{"database", "orm", "backend"},
			Source:    maia.MemorySourceExtracted,
		},
	}

	// Store discoveries in agent namespace
	for _, d := range discoveries {
		mem, err := client.CreateMemory(ctx, &d)
		if err != nil {
			log.Printf("Failed to store discovery: %v", err)
			continue
		}
		fmt.Printf("Discovered: %s\n", truncate(d.Content, 60))
		fmt.Printf("  ID: %s, Tags: %v\n\n", mem.ID[:8], d.Tags)
	}

	// Promote key findings to project namespace
	fmt.Println("=== Promoting Key Findings to Project Namespace ===")

	promotions := []maia.CreateMemoryInput{
		{
			Namespace: projectNS,
			Content:   "Project tech stack: React 18 + TypeScript 5.0 + Vite (frontend), PostgreSQL 15 + Prisma (backend)",
			Type:      maia.MemoryTypeSemantic,
			Tags:      []string{"summary", "research-findings"},
			Source:    maia.MemorySourceInferred,
			Metadata: map[string]interface{}{
				"source":     "research-agent",
				"promoted":   true,
				"created_at": time.Now().Format(time.RFC3339),
			},
		},
		{
			Namespace: projectNS,
			Content:   "Security: JWT auth with httpOnly cookies, 24h token expiry",
			Type:      maia.MemoryTypeSemantic,
			Tags:      []string{"summary", "research-findings"},
			Source:    maia.MemorySourceInferred,
			Metadata: map[string]interface{}{
				"source":     "research-agent",
				"promoted":   true,
				"created_at": time.Now().Format(time.RFC3339),
			},
		},
	}

	for _, p := range promotions {
		_, err := client.CreateMemory(ctx, &p)
		if err != nil {
			log.Printf("Failed to promote: %v", err)
			continue
		}
		fmt.Printf("Promoted: %s\n\n", truncate(p.Content, 70))
	}
}

// runCodeAgent simulates a code agent using research and storing implementations.
func runCodeAgent(ctx context.Context, client *maia.Client, agentNS, projectNS string) {
	fmt.Println("=== Code Agent: Gathering Context ===")

	// First, recall what the research agent found
	queries := []string{
		"What is the tech stack?",
		"How does authentication work?",
	}

	for _, query := range queries {
		result, err := client.Recall(ctx, query,
			maia.WithNamespace("agent:research"))
		if err != nil {
			// Try project namespace if agent namespace doesn't exist
			result, err = client.Recall(ctx, query,
				maia.WithNamespace(projectNS))
		}
		if err != nil {
			log.Printf("Failed to recall: %v", err)
			continue
		}

		fmt.Printf("Query: %s\n", query)
		fmt.Printf("Context (%d tokens, %d memories):\n", result.TokenCount, len(result.Memories))
		for _, line := range strings.Split(result.Content, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				fmt.Printf("  %s\n", truncate(line, 70))
			}
		}
		fmt.Println()
	}

	// Store implementation details
	fmt.Println("=== Code Agent: Recording Implementations ===")

	implementations := []maia.CreateMemoryInput{
		{
			Namespace: agentNS,
			Content:   "Created AuthProvider component at src/providers/AuthProvider.tsx with useAuth hook",
			Type:      maia.MemoryTypeEpisodic,
			Tags:      []string{"implementation", "auth", "react"},
			Source:    maia.MemorySourceUser,
		},
		{
			Namespace: agentNS,
			Content:   "Added JWT refresh logic in src/lib/auth.ts - auto-refresh 5 min before expiry",
			Type:      maia.MemoryTypeEpisodic,
			Tags:      []string{"implementation", "auth", "security"},
			Source:    maia.MemorySourceUser,
		},
		{
			Namespace: agentNS,
			Content:   "Implemented protected routes using AuthGuard HOC at src/components/AuthGuard.tsx",
			Type:      maia.MemoryTypeEpisodic,
			Tags:      []string{"implementation", "routing", "auth"},
			Source:    maia.MemorySourceUser,
		},
	}

	for _, impl := range implementations {
		mem, err := client.CreateMemory(ctx, &impl)
		if err != nil {
			log.Printf("Failed to store: %v", err)
			continue
		}
		fmt.Printf("Implemented: %s\n", truncate(impl.Content, 60))
		fmt.Printf("  ID: %s\n\n", mem.ID[:8])
	}

	// Also store in project namespace for visibility
	_, err := client.CreateMemory(ctx, &maia.CreateMemoryInput{
		Namespace: projectNS,
		Content:   "Auth system implemented: AuthProvider, useAuth hook, AuthGuard, JWT refresh",
		Type:      maia.MemoryTypeEpisodic,
		Tags:      []string{"implementation", "auth", "milestone"},
		Source:    maia.MemorySourceUser,
		Metadata: map[string]interface{}{
			"source": "code-agent",
		},
	})
	if err != nil {
		log.Printf("Failed to store project update: %v", err)
	}
}

// runReviewAgent simulates a review agent gathering context from all sources.
func runReviewAgent(ctx context.Context, client *maia.Client, agentNS, projectNS string) {
	fmt.Println("=== Review Agent: Gathering Full Context ===")

	// Review agent needs context from all sources
	result, err := client.Recall(ctx,
		"Review the authentication implementation including research and code",
		maia.WithNamespace(projectNS),
		maia.WithTokenBudget(4000))

	if err != nil {
		log.Printf("Failed to recall context: %v", err)
	} else {
		fmt.Printf("Project context (%d tokens, %d memories):\n", result.TokenCount, len(result.Memories))
		fmt.Println(result.Content)
		fmt.Println()
	}

	// Try to get research agent context
	researchCtx, err := client.Recall(ctx, "What was discovered about the project?",
		maia.WithNamespace("agent:research"),
		maia.WithTokenBudget(2000))
	if err == nil && len(researchCtx.Memories) > 0 {
		fmt.Printf("Research findings (%d memories):\n", len(researchCtx.Memories))
		for _, line := range strings.Split(researchCtx.Content, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				fmt.Printf("  %s\n", truncate(line, 70))
			}
		}
		fmt.Println()
	}

	// Try to get code agent context
	codeCtx, err := client.Recall(ctx, "What was implemented?",
		maia.WithNamespace("agent:code"),
		maia.WithTokenBudget(2000))
	if err == nil && len(codeCtx.Memories) > 0 {
		fmt.Printf("Code implementations (%d memories):\n", len(codeCtx.Memories))
		for _, line := range strings.Split(codeCtx.Content, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				fmt.Printf("  %s\n", truncate(line, 70))
			}
		}
		fmt.Println()
	}

	// Store review notes
	fmt.Println("=== Review Agent: Recording Notes ===")

	notes := []string{
		"REVIEW: Auth implementation follows security best practices - httpOnly cookies, token refresh",
		"REVIEW: Code structure is clean with proper separation - providers, hooks, guards",
		"REVIEW: Consider adding rate limiting to auth endpoints (TODO)",
	}

	for _, note := range notes {
		_, err := client.CreateMemory(ctx, &maia.CreateMemoryInput{
			Namespace: agentNS,
			Content:   note,
			Type:      maia.MemoryTypeEpisodic,
			Tags:      []string{"review", "auth", "notes"},
			Source:    maia.MemorySourceUser,
		})
		if err != nil {
			log.Printf("Failed to store note: %v", err)
			continue
		}
		fmt.Printf("Note: %s\n", note)
	}
}

func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}
