package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ContextResponse represents the context assembly response.
type ContextResponse struct {
	Content     string           `json:"content"`
	Memories    []ContextMemory  `json:"memories"`
	TokenCount  int              `json:"token_count"`
	TokenBudget int              `json:"token_budget"`
	Truncated   bool             `json:"truncated"`
	ZoneStats   *ZoneStats       `json:"zone_stats,omitempty"`
	QueryTime   string           `json:"query_time"`
}

// ContextMemory represents a memory in context.
type ContextMemory struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Type       string  `json:"type"`
	Score      float64 `json:"score,omitempty"`
	Position   string  `json:"position"`
	TokenCount int     `json:"token_count"`
	Truncated  bool    `json:"truncated"`
}

// ZoneStats represents context zone statistics.
type ZoneStats struct {
	CriticalUsed   int `json:"critical_used"`
	CriticalBudget int `json:"critical_budget"`
	MiddleUsed     int `json:"middle_used"`
	MiddleBudget   int `json:"middle_budget"`
	RecencyUsed    int `json:"recency_used"`
	RecencyBudget  int `json:"recency_budget"`
}

var contextCmd = &cobra.Command{
	Use:     "context",
	Aliases: []string{"ctx"},
	Short:   "Assemble context from memories",
	Long: `Query MAIA for relevant memories and assemble them into
a context block optimized for LLM consumption.

The context is assembled using position-aware ordering:
- Critical zone (first 15%): Most relevant information
- Middle zone (65%): Supporting context
- Recency zone (last 20%): Recent/temporal information`,
	Example: `  # Get context for a query
  maiactl context "What are the user's preferences?"

  # Get context with custom token budget
  maiactl context "API usage" --token-budget 2000 -n my-project

  # Include system prompt
  maiactl context "help me debug" --system "You are a helpful assistant"`,
	Args: cobra.ExactArgs(1),
	RunE: runContext,
}

// Flags for context command
var (
	ctxNamespace    string
	ctxTokenBudget  int
	ctxSystemPrompt string
	ctxShowScores   bool
	ctxMinScore     float64
	ctxRaw          bool
)

func init() {
	contextCmd.Flags().StringVarP(&ctxNamespace, "namespace", "n", "default", "Namespace to query")
	contextCmd.Flags().IntVarP(&ctxTokenBudget, "token-budget", "b", 4000, "Maximum tokens for context")
	contextCmd.Flags().StringVar(&ctxSystemPrompt, "system", "", "System prompt to prepend")
	contextCmd.Flags().BoolVar(&ctxShowScores, "scores", false, "Include relevance scores")
	contextCmd.Flags().Float64Var(&ctxMinScore, "min-score", 0.0, "Minimum relevance score (0.0-1.0)")
	contextCmd.Flags().BoolVarP(&ctxRaw, "raw", "r", false, "Output only the assembled content (no metadata)")
}

func runContext(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	body := map[string]interface{}{
		"query":          args[0],
		"namespace":      ctxNamespace,
		"token_budget":   ctxTokenBudget,
		"include_scores": ctxShowScores,
	}
	if ctxSystemPrompt != "" {
		body["system_prompt"] = ctxSystemPrompt
	}
	if ctxMinScore > 0 {
		body["min_score"] = ctxMinScore
	}

	var resp ContextResponse
	if err := client.Post("/v1/context", body, &resp); err != nil {
		return fmt.Errorf("failed to get context: %w", err)
	}

	// Raw mode - just output the content
	if ctxRaw {
		fmt.Print(resp.Content)
		return nil
	}

	if outputJSON {
		return PrintJSON(resp)
	}

	// Human-readable output
	fmt.Printf("Query: %s\n", args[0])
	fmt.Printf("Namespace: %s\n", ctxNamespace)
	fmt.Println()

	fmt.Printf("Token Usage: %d / %d", resp.TokenCount, resp.TokenBudget)
	if resp.Truncated {
		fmt.Print(" (truncated)")
	}
	fmt.Println()
	fmt.Printf("Query Time: %s\n", resp.QueryTime)
	fmt.Println()

	if resp.ZoneStats != nil {
		fmt.Println("Zone Statistics:")
		fmt.Printf("  Critical: %d / %d tokens\n", resp.ZoneStats.CriticalUsed, resp.ZoneStats.CriticalBudget)
		fmt.Printf("  Middle:   %d / %d tokens\n", resp.ZoneStats.MiddleUsed, resp.ZoneStats.MiddleBudget)
		fmt.Printf("  Recency:  %d / %d tokens\n", resp.ZoneStats.RecencyUsed, resp.ZoneStats.RecencyBudget)
		fmt.Println()
	}

	fmt.Printf("Memories (%d):\n", len(resp.Memories))
	headers := []string{"#", "ID", "TYPE", "POSITION", "TOKENS"}
	if ctxShowScores {
		headers = append(headers, "SCORE")
	}
	headers = append(headers, "CONTENT")

	rows := make([][]string, len(resp.Memories))
	for i, m := range resp.Memories {
		row := []string{
			fmt.Sprintf("%d", i+1),
			m.ID[:8] + "...",
			FormatMemoryType(m.Type),
			m.Position,
			fmt.Sprintf("%d", m.TokenCount),
		}
		if ctxShowScores {
			row = append(row, fmt.Sprintf("%.3f", m.Score))
		}
		content := Truncate(m.Content, 30)
		if m.Truncated {
			content += " [...]"
		}
		row = append(row, content)
		rows[i] = row
	}
	PrintTable(headers, rows)

	fmt.Println()
	fmt.Println("Assembled Content:")
	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Println(resp.Content)
	fmt.Println("─────────────────────────────────────────────────────")

	return nil
}
