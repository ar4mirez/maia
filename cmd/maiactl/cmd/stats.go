package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// StatsResponse represents server statistics.
type StatsResponse struct {
	MemoryCount    int   `json:"memory_count"`
	NamespaceCount int   `json:"namespace_count"`
	TotalSize      int64 `json:"total_size"`
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show server statistics",
	Long:  `Display statistics about the MAIA server including memory counts and storage size.`,
	RunE:  runStats,
}

func runStats(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var stats StatsResponse
	if err := client.Get("/v1/stats", &stats); err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	if outputJSON {
		return PrintJSON(stats)
	}

	fmt.Println("MAIA Server Statistics")
	fmt.Println("──────────────────────")
	fmt.Printf("Server:      %s\n", serverURL)
	fmt.Printf("Memories:    %d\n", stats.MemoryCount)
	fmt.Printf("Namespaces:  %d\n", stats.NamespaceCount)
	fmt.Printf("Storage:     %s\n", formatBytes(stats.TotalSize))

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
