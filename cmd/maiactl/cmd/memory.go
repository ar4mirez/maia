package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// Memory represents a memory from the API.
type Memory struct {
	ID          string                 `json:"id"`
	Namespace   string                 `json:"namespace"`
	Content     string                 `json:"content"`
	Type        string                 `json:"type"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Confidence  float64                `json:"confidence"`
	AccessCount int                    `json:"access_count"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	LastAccess  time.Time              `json:"last_access"`
}

// ListResponse represents a paginated list response.
type ListResponse struct {
	Data   []Memory `json:"data"`
	Count  int      `json:"count"`
	Offset int      `json:"offset"`
	Limit  int      `json:"limit"`
}

var memoryCmd = &cobra.Command{
	Use:     "memory",
	Aliases: []string{"mem", "m"},
	Short:   "Manage memories",
	Long:    `Create, list, get, update, and delete memories in MAIA.`,
}

var memoryCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new memory",
	Long:  `Create a new memory with the specified content and options.`,
	Example: `  # Create a simple memory
  maiactl memory create --namespace default --content "User prefers dark mode"

  # Create a memory with type and tags
  maiactl memory create -n default -c "API key is stored in env" -t procedural --tags security,config`,
	RunE: runMemoryCreate,
}

var memoryListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List memories",
	Long:    `List memories in a namespace with optional filtering.`,
	Example: `  # List all memories in default namespace
  maiactl memory list --namespace default

  # List with filters
  maiactl memory list -n default --type semantic --limit 20`,
	RunE: runMemoryList,
}

var memoryGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a memory by ID",
	Long:  `Retrieve a specific memory by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runMemoryGet,
}

var memoryUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a memory",
	Long:  `Update the content or metadata of an existing memory.`,
	Args:  cobra.ExactArgs(1),
	Example: `  # Update memory content
  maiactl memory update <id> --content "Updated preference"

  # Add tags to a memory
  maiactl memory update <id> --tags important,reviewed`,
	RunE: runMemoryUpdate,
}

var memoryDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"rm"},
	Short:   "Delete a memory",
	Long:    `Delete a memory by its ID.`,
	Args:    cobra.ExactArgs(1),
	RunE:    runMemoryDelete,
}

var memorySearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search memories",
	Long:  `Search memories using various criteria.`,
	Example: `  # Search by query
  maiactl memory search --query "user preferences"

  # Search with filters
  maiactl memory search -q "api" --namespace default --type procedural`,
	RunE: runMemorySearch,
}

// Flags for memory commands
var (
	memNamespace  string
	memContent    string
	memType       string
	memTags       []string
	memConfidence float64
	memLimit      int
	memOffset     int
	memQuery      string
)

func init() {
	// Create command flags
	memoryCreateCmd.Flags().StringVarP(&memNamespace, "namespace", "n", "default", "Namespace for the memory")
	memoryCreateCmd.Flags().StringVarP(&memContent, "content", "c", "", "Memory content (required)")
	memoryCreateCmd.Flags().StringVarP(&memType, "type", "t", "semantic", "Memory type (semantic, episodic, working, procedural)")
	memoryCreateCmd.Flags().StringSliceVar(&memTags, "tags", nil, "Tags for the memory")
	memoryCreateCmd.Flags().Float64Var(&memConfidence, "confidence", 1.0, "Confidence score (0.0-1.0)")
	_ = memoryCreateCmd.MarkFlagRequired("content")

	// List command flags
	memoryListCmd.Flags().StringVarP(&memNamespace, "namespace", "n", "default", "Namespace to list")
	memoryListCmd.Flags().StringVarP(&memType, "type", "t", "", "Filter by memory type")
	memoryListCmd.Flags().IntVarP(&memLimit, "limit", "l", 20, "Maximum number of results")
	memoryListCmd.Flags().IntVarP(&memOffset, "offset", "o", 0, "Offset for pagination")

	// Update command flags
	memoryUpdateCmd.Flags().StringVarP(&memContent, "content", "c", "", "New content")
	memoryUpdateCmd.Flags().StringSliceVar(&memTags, "tags", nil, "New tags")
	memoryUpdateCmd.Flags().Float64Var(&memConfidence, "confidence", -1, "New confidence score")

	// Search command flags
	memorySearchCmd.Flags().StringVarP(&memQuery, "query", "q", "", "Search query")
	memorySearchCmd.Flags().StringVarP(&memNamespace, "namespace", "n", "", "Namespace to search")
	memorySearchCmd.Flags().StringVarP(&memType, "type", "t", "", "Filter by memory type")
	memorySearchCmd.Flags().IntVarP(&memLimit, "limit", "l", 20, "Maximum results")
	memorySearchCmd.Flags().IntVarP(&memOffset, "offset", "o", 0, "Offset for pagination")

	// Add subcommands
	memoryCmd.AddCommand(memoryCreateCmd)
	memoryCmd.AddCommand(memoryListCmd)
	memoryCmd.AddCommand(memoryGetCmd)
	memoryCmd.AddCommand(memoryUpdateCmd)
	memoryCmd.AddCommand(memoryDeleteCmd)
	memoryCmd.AddCommand(memorySearchCmd)
}

func runMemoryCreate(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	body := map[string]interface{}{
		"namespace":  memNamespace,
		"content":    memContent,
		"type":       memType,
		"confidence": memConfidence,
	}
	if len(memTags) > 0 {
		body["tags"] = memTags
	}

	var mem Memory
	if err := client.Post("/v1/memories", body, &mem); err != nil {
		return fmt.Errorf("failed to create memory: %w", err)
	}

	if outputJSON {
		return PrintJSON(mem)
	}

	fmt.Printf("Created memory: %s\n", mem.ID)
	fmt.Printf("  Namespace: %s\n", mem.Namespace)
	fmt.Printf("  Type:      %s\n", FormatMemoryType(mem.Type))
	fmt.Printf("  Content:   %s\n", Truncate(mem.Content, 60))
	return nil
}

func runMemoryList(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	path := fmt.Sprintf("/v1/namespaces/%s/memories?limit=%d&offset=%d", memNamespace, memLimit, memOffset)

	var resp ListResponse
	if err := client.Get(path, &resp); err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	if outputJSON {
		return PrintJSON(resp)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No memories found.")
		return nil
	}

	headers := []string{"ID", "TYPE", "CONTENT", "CONFIDENCE", "CREATED"}
	rows := make([][]string, len(resp.Data))
	for i, m := range resp.Data {
		rows[i] = []string{
			m.ID[:8] + "...",
			FormatMemoryType(m.Type),
			Truncate(m.Content, 40),
			fmt.Sprintf("%.2f", m.Confidence),
			m.CreatedAt.Format("2006-01-02 15:04"),
		}
	}
	PrintTable(headers, rows)

	if resp.Count >= resp.Limit {
		fmt.Printf("\nShowing %d of potentially more results. Use --offset to paginate.\n", resp.Count)
	}
	return nil
}

func runMemoryGet(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var mem Memory
	if err := client.Get("/v1/memories/"+args[0], &mem); err != nil {
		return fmt.Errorf("failed to get memory: %w", err)
	}

	if outputJSON {
		return PrintJSON(mem)
	}

	fmt.Printf("ID:          %s\n", mem.ID)
	fmt.Printf("Namespace:   %s\n", mem.Namespace)
	fmt.Printf("Type:        %s\n", FormatMemoryType(mem.Type))
	fmt.Printf("Confidence:  %.2f\n", mem.Confidence)
	fmt.Printf("Access Count: %d\n", mem.AccessCount)
	fmt.Printf("Created:     %s\n", mem.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:     %s\n", mem.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Last Access: %s\n", mem.LastAccess.Format(time.RFC3339))
	if len(mem.Tags) > 0 {
		fmt.Printf("Tags:        %v\n", mem.Tags)
	}
	fmt.Printf("\nContent:\n%s\n", mem.Content)
	return nil
}

func runMemoryUpdate(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	body := make(map[string]interface{})
	if memContent != "" {
		body["content"] = memContent
	}
	if len(memTags) > 0 {
		body["tags"] = memTags
	}
	if memConfidence >= 0 {
		body["confidence"] = memConfidence
	}

	if len(body) == 0 {
		return fmt.Errorf("no updates specified")
	}

	var mem Memory
	if err := client.Put("/v1/memories/"+args[0], body, &mem); err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	if outputJSON {
		return PrintJSON(mem)
	}

	fmt.Printf("Updated memory: %s\n", mem.ID)
	return nil
}

func runMemoryDelete(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var result map[string]interface{}
	if err := client.Delete("/v1/memories/"+args[0], &result); err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	if outputJSON {
		return PrintJSON(result)
	}

	fmt.Printf("Deleted memory: %s\n", args[0])
	return nil
}

func runMemorySearch(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	body := map[string]interface{}{
		"limit":  memLimit,
		"offset": memOffset,
	}
	if memQuery != "" {
		body["query"] = memQuery
	}
	if memNamespace != "" {
		body["namespace"] = memNamespace
	}
	if memType != "" {
		body["types"] = []string{memType}
	}

	var resp ListResponse
	if err := client.Post("/v1/memories/search", body, &resp); err != nil {
		return fmt.Errorf("failed to search memories: %w", err)
	}

	if outputJSON {
		return PrintJSON(resp)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No memories found matching your search.")
		return nil
	}

	headers := []string{"ID", "TYPE", "CONTENT", "CONFIDENCE"}
	rows := make([][]string, len(resp.Data))
	for i, m := range resp.Data {
		rows[i] = []string{
			m.ID[:8] + "...",
			FormatMemoryType(m.Type),
			Truncate(m.Content, 50),
			fmt.Sprintf("%.2f", m.Confidence),
		}
	}
	PrintTable(headers, rows)
	return nil
}
