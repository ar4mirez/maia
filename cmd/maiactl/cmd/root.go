// Package cmd provides CLI commands for maiactl.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	serverURL  string
	outputJSON bool
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "maiactl",
	Short: "MAIA CLI - Manage AI memory and context",
	Long: `maiactl is a command-line tool for interacting with the MAIA server.

MAIA (Memory AI Architecture) is an AI-native distributed memory system
for LLM context management.

Use maiactl to:
  - Create, list, and manage memories
  - Manage namespaces
  - Assemble context for queries
  - View server statistics`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", getEnvOrDefault("MAIA_URL", "http://localhost:8080"), "MAIA server URL")
	rootCmd.PersistentFlags().BoolVarP(&outputJSON, "json", "j", false, "Output in JSON format")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(memoryCmd)
	rootCmd.AddCommand(namespaceCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(statsCmd)
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
