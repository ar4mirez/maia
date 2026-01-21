// Package main provides a CLI tool for MAIA data migrations.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Define commands
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("no command specified")
	}

	command := os.Args[1]

	switch command {
	case "export":
		return runExport(os.Args[2:])
	case "import":
		return runImport(os.Args[2:])
	case "migrate-to-tenant":
		return runMigrateToTenant(os.Args[2:])
	case "copy-between-tenants":
		return runCopyBetweenTenants(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Println(`MAIA Data Migration Tool

Usage:
  migrate <command> [options]

Commands:
  export              Export data from a MAIA database to JSON
  import              Import data from JSON to a MAIA database
  migrate-to-tenant   Migrate single-tenant data to a specific tenant
  copy-between-tenants Copy data from one tenant to another
  help                Show this help message

Examples:
  # Export all data from default namespace
  migrate export --data-dir ./data --output backup.json

  # Export data from specific namespace
  migrate export --data-dir ./data --namespace myapp --output backup.json

  # Import data to a new namespace
  migrate import --data-dir ./data --input backup.json --namespace restored

  # Migrate existing data to a tenant
  migrate migrate-to-tenant --data-dir ./data --tenant-id acme-corp

  # Copy data between tenants
  migrate copy-between-tenants --data-dir ./data --from-tenant tenant-a --to-tenant tenant-b`)
}

// ExportData represents exported MAIA data.
type ExportData struct {
	Version    string               `json:"version"`
	ExportedAt time.Time            `json:"exported_at"`
	Namespace  string               `json:"namespace,omitempty"`
	TenantID   string               `json:"tenant_id,omitempty"`
	Memories   []*storage.Memory    `json:"memories"`
	Namespaces []*storage.Namespace `json:"namespaces"`
	Statistics map[string]any       `json:"statistics"`
}

func openStore(dataDir string) (*badger.Store, error) {
	return badger.New(&badger.Options{
		DataDir:    dataDir,
		SyncWrites: false,
	})
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dataDir := fs.String("data-dir", "./data", "Path to MAIA data directory")
	output := fs.String("output", "", "Output file (default: stdout)")
	namespace := fs.String("namespace", "", "Filter by namespace (empty for all)")
	tenantID := fs.String("tenant-id", "", "Filter by tenant ID (empty for all)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Open storage
	store, err := openStore(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// List memories (namespace filter applied here)
	targetNS := ""
	if *namespace != "" {
		targetNS = *namespace
	}
	if *tenantID != "" && targetNS != "" {
		targetNS = *tenantID + "::" + targetNS
	}

	memories, err := store.ListMemories(ctx, targetNS, nil) // nil = no pagination
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	// If only tenant filter (no namespace), filter memories by tenant prefix
	if *tenantID != "" && *namespace == "" {
		prefix := *tenantID + "::"
		var filtered []*storage.Memory
		for _, m := range memories {
			if strings.HasPrefix(m.Namespace, prefix) {
				filtered = append(filtered, m)
			}
		}
		memories = filtered
	}

	// List namespaces
	namespaces, err := store.ListNamespaces(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	// Filter namespaces if namespace specified
	if *namespace != "" {
		var filtered []*storage.Namespace
		for _, ns := range namespaces {
			if ns.ID == *namespace || ns.Name == *namespace {
				filtered = append(filtered, ns)
			}
		}
		namespaces = filtered
	}

	// Build export data
	export := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now().UTC(),
		Namespace:  *namespace,
		TenantID:   *tenantID,
		Memories:   memories,
		Namespaces: namespaces,
		Statistics: map[string]any{
			"memory_count":    len(memories),
			"namespace_count": len(namespaces),
		},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal export data: %w", err)
	}

	// Write output
	var writer io.Writer = os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		writer = f
	}

	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if *output != "" {
		fmt.Fprintf(os.Stderr, "Exported %d memories and %d namespaces to %s\n",
			len(memories), len(namespaces), *output)
	}

	return nil
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dataDir := fs.String("data-dir", "./data", "Path to MAIA data directory")
	input := fs.String("input", "", "Input file (required)")
	namespace := fs.String("namespace", "", "Target namespace (overrides source)")
	tenantID := fs.String("tenant-id", "", "Target tenant ID")
	skipExisting := fs.Bool("skip-existing", false, "Skip memories that already exist")
	dryRun := fs.Bool("dry-run", false, "Show what would be imported without making changes")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *input == "" {
		return fmt.Errorf("--input is required")
	}

	// Read input file
	data, err := os.ReadFile(*input)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Parse export data
	var export ExportData
	if err := json.Unmarshal(data, &export); err != nil {
		return fmt.Errorf("failed to parse export data: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Loaded export with %d memories and %d namespaces\n",
		len(export.Memories), len(export.Namespaces))

	if *dryRun {
		fmt.Fprintf(os.Stderr, "[DRY RUN] Would import:\n")
		for _, m := range export.Memories {
			targetNS := m.Namespace
			if *namespace != "" {
				targetNS = *namespace
			}
			if *tenantID != "" {
				targetNS = *tenantID + "::" + targetNS
			}
			fmt.Fprintf(os.Stderr, "  - Memory %s in namespace %s\n", m.ID, targetNS)
		}
		return nil
	}

	// Open storage
	store, err := openStore(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Import namespaces first
	nsCreated := 0
	for _, ns := range export.Namespaces {
		targetNS := ns.Name
		if *namespace != "" {
			targetNS = *namespace
		}
		if *tenantID != "" {
			targetNS = *tenantID + "::" + targetNS
		}

		// Check if namespace exists
		_, err := store.GetNamespace(ctx, targetNS)
		if err == nil {
			fmt.Fprintf(os.Stderr, "Namespace %s already exists, skipping\n", targetNS)
			continue
		}

		// Create namespace
		_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
			Name:   targetNS,
			Config: ns.Config,
		})
		if err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", targetNS, err)
		}
		nsCreated++
	}

	// Import memories
	memCreated := 0
	memSkipped := 0
	for _, m := range export.Memories {
		targetNS := m.Namespace
		if *namespace != "" {
			targetNS = *namespace
		}
		if *tenantID != "" {
			targetNS = *tenantID + "::" + targetNS
		}

		// Check if memory with same content exists
		if *skipExisting {
			existing, _ := store.ListMemories(ctx, targetNS, &storage.ListOptions{Limit: 1000})
			found := false
			for _, e := range existing {
				if e.Content == m.Content {
					found = true
					break
				}
			}
			if found {
				memSkipped++
				continue
			}
		}

		// Build memory input
		memInput := &storage.CreateMemoryInput{
			Namespace:  targetNS,
			Content:    m.Content,
			Type:       m.Type,
			Embedding:  m.Embedding,
			Metadata:   m.Metadata,
			Tags:       m.Tags,
			Confidence: m.Confidence,
			Source:     m.Source,
			Relations:  m.Relations,
		}

		_, err := store.CreateMemory(ctx, memInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create memory: %v\n", err)
			continue
		}
		memCreated++
	}

	fmt.Fprintf(os.Stderr, "Import complete: %d namespaces created, %d memories created, %d skipped\n",
		nsCreated, memCreated, memSkipped)

	return nil
}

func runMigrateToTenant(args []string) error {
	fs := flag.NewFlagSet("migrate-to-tenant", flag.ExitOnError)
	dataDir := fs.String("data-dir", "./data", "Path to MAIA data directory")
	targetTenantID := fs.String("tenant-id", "", "Target tenant ID (required)")
	sourceNamespace := fs.String("namespace", "", "Source namespace (empty for all)")
	dryRun := fs.Bool("dry-run", false, "Show what would be migrated without making changes")
	deleteSource := fs.Bool("delete-source", false, "Delete source data after migration")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *targetTenantID == "" {
		return fmt.Errorf("--tenant-id is required")
	}

	// Open storage
	store, err := openStore(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// List all memories without tenant prefix
	memories, err := store.ListMemories(ctx, *sourceNamespace, nil)
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	// Filter out memories that already have tenant prefix
	var toMigrate []*storage.Memory
	for _, m := range memories {
		// Skip if already has a tenant prefix (contains ::)
		if strings.Contains(m.Namespace, "::") {
			continue
		}
		toMigrate = append(toMigrate, m)
	}

	fmt.Fprintf(os.Stderr, "Found %d memories to migrate to tenant %s\n",
		len(toMigrate), *targetTenantID)

	if *dryRun {
		fmt.Fprintf(os.Stderr, "[DRY RUN] Would migrate:\n")
		for _, m := range toMigrate {
			newNS := *targetTenantID + "::" + m.Namespace
			fmt.Fprintf(os.Stderr, "  - Memory %s: %s -> %s\n", m.ID, m.Namespace, newNS)
		}
		return nil
	}

	// Perform migration
	migrated := 0
	for _, m := range toMigrate {
		newNamespace := *targetTenantID + "::" + m.Namespace

		// Create new memory with tenant prefix
		newMemory := &storage.CreateMemoryInput{
			Namespace:  newNamespace,
			Content:    m.Content,
			Type:       m.Type,
			Embedding:  m.Embedding,
			Metadata:   m.Metadata,
			Tags:       m.Tags,
			Confidence: m.Confidence,
			Source:     m.Source,
			Relations:  m.Relations,
		}

		_, err := store.CreateMemory(ctx, newMemory)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create memory in new namespace: %v\n", err)
			continue
		}

		// Delete original if requested
		if *deleteSource {
			if err := store.DeleteMemory(ctx, m.ID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete original memory %s: %v\n", m.ID, err)
			}
		}

		migrated++
	}

	fmt.Fprintf(os.Stderr, "Migration complete: %d memories migrated\n", migrated)

	return nil
}

func runCopyBetweenTenants(args []string) error {
	fs := flag.NewFlagSet("copy-between-tenants", flag.ExitOnError)
	dataDir := fs.String("data-dir", "./data", "Path to MAIA data directory")
	fromTenant := fs.String("from-tenant", "", "Source tenant ID (required)")
	toTenant := fs.String("to-tenant", "", "Target tenant ID (required)")
	namespace := fs.String("namespace", "", "Specific namespace to copy (empty for all)")
	dryRun := fs.Bool("dry-run", false, "Show what would be copied without making changes")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *fromTenant == "" || *toTenant == "" {
		return fmt.Errorf("--from-tenant and --to-tenant are required")
	}

	// Open storage
	store, err := openStore(*dataDir)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// List all memories
	memories, err := store.ListMemories(ctx, "", nil)
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	// Filter memories belonging to source tenant
	fromPrefix := *fromTenant + "::"
	var toCopy []*storage.Memory
	for _, m := range memories {
		if strings.HasPrefix(m.Namespace, fromPrefix) {
			// Check namespace filter
			originalNS := strings.TrimPrefix(m.Namespace, fromPrefix)
			if *namespace != "" && originalNS != *namespace {
				continue
			}
			toCopy = append(toCopy, m)
		}
	}

	fmt.Fprintf(os.Stderr, "Found %d memories to copy from %s to %s\n",
		len(toCopy), *fromTenant, *toTenant)

	if *dryRun {
		fmt.Fprintf(os.Stderr, "[DRY RUN] Would copy:\n")
		for _, m := range toCopy {
			originalNS := strings.TrimPrefix(m.Namespace, fromPrefix)
			newNS := *toTenant + "::" + originalNS
			fmt.Fprintf(os.Stderr, "  - Memory %s: %s -> %s\n", m.ID, m.Namespace, newNS)
		}
		return nil
	}

	// Perform copy
	copied := 0
	for _, m := range toCopy {
		originalNS := strings.TrimPrefix(m.Namespace, fromPrefix)
		newNamespace := *toTenant + "::" + originalNS

		// Create new memory with target tenant prefix
		newMemory := &storage.CreateMemoryInput{
			Namespace:  newNamespace,
			Content:    m.Content,
			Type:       m.Type,
			Embedding:  m.Embedding,
			Metadata:   m.Metadata,
			Tags:       m.Tags,
			Confidence: m.Confidence,
			Source:     m.Source,
			Relations:  m.Relations,
		}

		_, err := store.CreateMemory(ctx, newMemory)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to copy memory: %v\n", err)
			continue
		}

		copied++
	}

	fmt.Fprintf(os.Stderr, "Copy complete: %d memories copied\n", copied)

	return nil
}
