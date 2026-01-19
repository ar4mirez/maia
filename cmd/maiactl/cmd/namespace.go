package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// Namespace represents a namespace from the API.
type Namespace struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Parent    string          `json:"parent,omitempty"`
	Template  string          `json:"template,omitempty"`
	Config    NamespaceConfig `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// NamespaceConfig represents namespace configuration.
type NamespaceConfig struct {
	TokenBudget       int  `json:"token_budget"`
	InheritFromParent bool `json:"inherit_from_parent"`
}

// NamespaceListResponse represents a list of namespaces.
type NamespaceListResponse struct {
	Data   []Namespace `json:"data"`
	Count  int         `json:"count"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
}

var namespaceCmd = &cobra.Command{
	Use:     "namespace",
	Aliases: []string{"ns"},
	Short:   "Manage namespaces",
	Long:    `Create, list, get, update, and delete namespaces in MAIA.`,
}

var namespaceCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new namespace",
	Long:  `Create a new namespace with optional configuration.`,
	Args:  cobra.ExactArgs(1),
	Example: `  # Create a simple namespace
  maiactl namespace create my-project

  # Create with parent and token budget
  maiactl namespace create my-project/dev --parent my-project --token-budget 8000`,
	RunE: runNamespaceCreate,
}

var namespaceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List namespaces",
	Long:    `List all namespaces.`,
	RunE:    runNamespaceList,
}

var namespaceGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a namespace by name or ID",
	Long:  `Retrieve a specific namespace by its name or ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runNamespaceGet,
}

var namespaceUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update a namespace",
	Long:  `Update namespace configuration.`,
	Args:  cobra.ExactArgs(1),
	Example: `  # Update token budget
  maiactl namespace update my-project --token-budget 10000`,
	RunE: runNamespaceUpdate,
}

var namespaceDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm"},
	Short:   "Delete a namespace",
	Long:    `Delete a namespace by name or ID.`,
	Args:    cobra.ExactArgs(1),
	RunE:    runNamespaceDelete,
}

// Flags for namespace commands
var (
	nsParent      string
	nsTemplate    string
	nsTokenBudget int
	nsInherit     bool
	nsLimit       int
	nsOffset      int
)

func init() {
	// Create command flags
	namespaceCreateCmd.Flags().StringVar(&nsParent, "parent", "", "Parent namespace")
	namespaceCreateCmd.Flags().StringVar(&nsTemplate, "template", "", "Configuration template")
	namespaceCreateCmd.Flags().IntVar(&nsTokenBudget, "token-budget", 4000, "Token budget for context assembly")
	namespaceCreateCmd.Flags().BoolVar(&nsInherit, "inherit", false, "Inherit configuration from parent")

	// List command flags
	namespaceListCmd.Flags().IntVarP(&nsLimit, "limit", "l", 100, "Maximum number of results")
	namespaceListCmd.Flags().IntVarP(&nsOffset, "offset", "o", 0, "Offset for pagination")

	// Update command flags
	namespaceUpdateCmd.Flags().IntVar(&nsTokenBudget, "token-budget", 0, "New token budget")
	namespaceUpdateCmd.Flags().BoolVar(&nsInherit, "inherit", false, "Inherit configuration from parent")

	// Add subcommands
	namespaceCmd.AddCommand(namespaceCreateCmd)
	namespaceCmd.AddCommand(namespaceListCmd)
	namespaceCmd.AddCommand(namespaceGetCmd)
	namespaceCmd.AddCommand(namespaceUpdateCmd)
	namespaceCmd.AddCommand(namespaceDeleteCmd)
}

func runNamespaceCreate(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	body := map[string]interface{}{
		"name": args[0],
		"config": map[string]interface{}{
			"token_budget":        nsTokenBudget,
			"inherit_from_parent": nsInherit,
		},
	}
	if nsParent != "" {
		body["parent"] = nsParent
	}
	if nsTemplate != "" {
		body["template"] = nsTemplate
	}

	var ns Namespace
	if err := client.Post("/v1/namespaces", body, &ns); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	if outputJSON {
		return PrintJSON(ns)
	}

	fmt.Printf("Created namespace: %s\n", ns.Name)
	fmt.Printf("  ID:           %s\n", ns.ID)
	fmt.Printf("  Token Budget: %d\n", ns.Config.TokenBudget)
	if ns.Parent != "" {
		fmt.Printf("  Parent:       %s\n", ns.Parent)
	}
	return nil
}

func runNamespaceList(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	path := fmt.Sprintf("/v1/namespaces?limit=%d&offset=%d", nsLimit, nsOffset)

	var resp NamespaceListResponse
	if err := client.Get(path, &resp); err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	if outputJSON {
		return PrintJSON(resp)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No namespaces found.")
		return nil
	}

	headers := []string{"NAME", "ID", "PARENT", "TOKEN BUDGET", "CREATED"}
	rows := make([][]string, len(resp.Data))
	for i, ns := range resp.Data {
		parent := "-"
		if ns.Parent != "" {
			parent = ns.Parent
		}
		rows[i] = []string{
			ns.Name,
			ns.ID[:8] + "...",
			parent,
			fmt.Sprintf("%d", ns.Config.TokenBudget),
			ns.CreatedAt.Format("2006-01-02"),
		}
	}
	PrintTable(headers, rows)
	return nil
}

func runNamespaceGet(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var ns Namespace
	if err := client.Get("/v1/namespaces/"+args[0], &ns); err != nil {
		return fmt.Errorf("failed to get namespace: %w", err)
	}

	if outputJSON {
		return PrintJSON(ns)
	}

	fmt.Printf("Name:         %s\n", ns.Name)
	fmt.Printf("ID:           %s\n", ns.ID)
	if ns.Parent != "" {
		fmt.Printf("Parent:       %s\n", ns.Parent)
	}
	if ns.Template != "" {
		fmt.Printf("Template:     %s\n", ns.Template)
	}
	fmt.Printf("Token Budget: %d\n", ns.Config.TokenBudget)
	fmt.Printf("Inherit:      %t\n", ns.Config.InheritFromParent)
	fmt.Printf("Created:      %s\n", ns.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:      %s\n", ns.UpdatedAt.Format(time.RFC3339))
	return nil
}

func runNamespaceUpdate(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	config := map[string]interface{}{}
	if cmd.Flags().Changed("token-budget") {
		config["token_budget"] = nsTokenBudget
	}
	if cmd.Flags().Changed("inherit") {
		config["inherit_from_parent"] = nsInherit
	}

	if len(config) == 0 {
		return fmt.Errorf("no updates specified")
	}

	body := map[string]interface{}{
		"config": config,
	}

	var ns Namespace
	if err := client.Put("/v1/namespaces/"+args[0], body, &ns); err != nil {
		return fmt.Errorf("failed to update namespace: %w", err)
	}

	if outputJSON {
		return PrintJSON(ns)
	}

	fmt.Printf("Updated namespace: %s\n", ns.Name)
	return nil
}

func runNamespaceDelete(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var result map[string]interface{}
	if err := client.Delete("/v1/namespaces/"+args[0], &result); err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	if outputJSON {
		return PrintJSON(result)
	}

	fmt.Printf("Deleted namespace: %s\n", args[0])
	return nil
}
