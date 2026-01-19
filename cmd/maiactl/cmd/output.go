package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// PrintJSON outputs data as formatted JSON.
func PrintJSON(data interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// PrintTable outputs data in a table format.
func PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print separator
	sep := make([]string, len(headers))
	for i, h := range headers {
		sep[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(w, strings.Join(sep, "\t"))

	// Print rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
}

// PrintKeyValue outputs key-value pairs.
func PrintKeyValue(pairs map[string]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for k, v := range pairs {
		fmt.Fprintf(w, "%s:\t%s\n", k, v)
	}
	w.Flush()
}

// Truncate shortens a string to the specified length.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FormatMemoryType returns a human-readable memory type.
func FormatMemoryType(t string) string {
	switch t {
	case "semantic":
		return "Semantic"
	case "episodic":
		return "Episodic"
	case "working":
		return "Working"
	case "procedural":
		return "Procedural"
	default:
		return t
	}
}
