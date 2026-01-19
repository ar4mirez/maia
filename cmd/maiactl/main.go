// Package main provides the entry point for the maiactl CLI tool.
package main

import (
	"fmt"
	"os"

	"github.com/ar4mirez/maia/cmd/maiactl/cmd"
)

// Build-time variables (set via ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	cmd.SetVersionInfo(Version, Commit, BuildTime)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
