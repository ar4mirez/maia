// Package main demonstrates basic MAIA usage with the Go SDK.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ar4mirez/maia/pkg/maia"
)

func main() {
	// Create a new MAIA client
	client := maia.New(
		maia.WithBaseURL("http://localhost:8080"),
		maia.WithTimeout(10*time.Second),
	)

	ctx := context.Background()
	namespace := "example"

	// Check server health
	fmt.Println("=== Checking Server Health ===")
	health, err := client.Health(ctx)
	if err != nil {
		log.Fatalf("Failed to check health: %v", err)
	}
	fmt.Printf("Server status: %s\n\n", health.Status)

	// Create a namespace (ignore error if it already exists)
	fmt.Println("=== Creating Namespace ===")
	ns, err := client.CreateNamespace(ctx, &maia.CreateNamespaceInput{
		Name: namespace,
		Config: maia.NamespaceConfig{
			TokenBudget: 4000,
		},
	})
	if err != nil {
		fmt.Printf("Namespace may already exist: %v\n", err)
	} else {
		fmt.Printf("Created namespace: %s (ID: %s)\n", ns.Name, ns.ID)
	}
	fmt.Println()

	// Store some memories using CreateMemory
	fmt.Println("=== Storing Memories ===")

	inputs := []maia.CreateMemoryInput{
		{
			Namespace:  namespace,
			Content:    "The user prefers dark mode for all applications. They find it easier on the eyes, especially when working late at night.",
			Type:       maia.MemoryTypeSemantic,
			Tags:       []string{"preference", "ui", "accessibility"},
			Confidence: 0.95,
			Source:     maia.MemorySourceUser,
		},
		{
			Namespace:  namespace,
			Content:    "User's timezone is America/New_York (Eastern Time). They typically work from 9 AM to 6 PM.",
			Type:       maia.MemoryTypeSemantic,
			Tags:       []string{"preference", "timezone", "schedule"},
			Confidence: 0.9,
			Source:     maia.MemorySourceUser,
		},
		{
			Namespace:  namespace,
			Content:    "Currently working on a React project called 'dashboard-v2'. Using TypeScript and Tailwind CSS.",
			Type:       maia.MemoryTypeEpisodic,
			Tags:       []string{"project", "context", "react"},
			Confidence: 0.85,
			Source:     maia.MemorySourceUser,
		},
		{
			Namespace:  namespace,
			Content:    "The user mentioned they have a meeting with the design team tomorrow at 2 PM to discuss the new color palette.",
			Type:       maia.MemoryTypeWorking,
			Tags:       []string{"meeting", "schedule", "design"},
			Confidence: 0.8,
			Source:     maia.MemorySourceUser,
		},
	}

	var storedMemories []*maia.Memory
	for _, input := range inputs {
		mem, err := client.CreateMemory(ctx, &input)
		if err != nil {
			log.Printf("Failed to store memory: %v", err)
			continue
		}
		storedMemories = append(storedMemories, mem)
		fmt.Printf("Stored: %s... (ID: %s, Type: %s)\n",
			truncate(mem.Content, 50), mem.ID[:8], mem.Type)
	}
	fmt.Println()

	// List all memories in the namespace
	fmt.Println("=== Listing All Memories ===")
	allMemories, err := client.ListNamespaceMemories(ctx, namespace, nil)
	if err != nil {
		log.Printf("Failed to list memories: %v", err)
	} else {
		fmt.Printf("Found %d memories in namespace '%s'\n", len(allMemories.Data), namespace)
		for i, mem := range allMemories.Data {
			fmt.Printf("  %d. [%s] %s\n", i+1, mem.Type, truncate(mem.Content, 60))
		}
	}
	fmt.Println()

	// Search memories by tag
	fmt.Println("=== Searching Memories ===")
	fmt.Println("Searching for memories with tag 'preference'...")
	searchResults, err := client.SearchMemories(ctx, &maia.SearchMemoriesInput{
		Namespace: namespace,
		Tags:      []string{"preference"},
	})
	if err != nil {
		log.Printf("Failed to search memories: %v", err)
	} else {
		fmt.Printf("Found %d memories:\n", len(searchResults.Data))
		for _, result := range searchResults.Data {
			fmt.Printf("  - %s (score: %.2f)\n", truncate(result.Memory.Content, 60), result.Score)
		}
	}
	fmt.Println()

	// Recall context for different queries using the convenience method
	fmt.Println("=== Recalling Context ===")

	queries := []string{
		"What are the user's UI preferences?",
		"What project is the user working on?",
		"What meetings does the user have scheduled?",
	}

	for _, query := range queries {
		fmt.Printf("\nQuery: %q\n", query)
		result, err := client.Recall(ctx, query,
			maia.WithNamespace(namespace),
			maia.WithTokenBudget(1000),
		)
		if err != nil {
			log.Printf("Failed to recall: %v", err)
			continue
		}

		fmt.Printf("  Tokens used: %d/%d\n", result.TokenCount, result.TokenBudget)
		fmt.Printf("  Memories used: %d\n", len(result.Memories))
		fmt.Printf("  Context preview: %s...\n", truncate(result.Content, 100))
	}
	fmt.Println()

	// Get server statistics
	fmt.Println("=== Server Statistics ===")
	stats, err := client.Stats(ctx)
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
	} else {
		fmt.Printf("Total memories: %d\n", stats.TotalMemories)
		fmt.Printf("Total namespaces: %d\n", stats.TotalNamespaces)
		fmt.Printf("Storage size: %d bytes\n", stats.StorageSizeBytes)
	}
	fmt.Println()

	// Cleanup: Delete the memories we created
	fmt.Println("=== Cleanup ===")
	for _, mem := range storedMemories {
		if err := client.Forget(ctx, mem.ID); err != nil {
			log.Printf("Failed to delete memory %s: %v", mem.ID[:8], err)
		} else {
			fmt.Printf("Deleted memory: %s\n", mem.ID[:8])
		}
	}

	fmt.Println("\nDone!")
}

// truncate truncates a string to the specified length.
func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}
