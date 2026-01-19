// Package main provides the entry point for the MAIA server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ar4mirez/maia/internal/config"
	"github.com/ar4mirez/maia/internal/server"
	"github.com/ar4mirez/maia/internal/storage"
	"github.com/ar4mirez/maia/internal/storage/badger"
)

// Build-time variables (set via ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize logger
	logger, err := initLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("starting MAIA",
		zap.String("version", Version),
		zap.String("commit", Commit),
		zap.String("build_time", BuildTime),
	)

	// Initialize storage
	store, err := badger.New(&badger.Options{
		DataDir:    cfg.Storage.DataDir,
		SyncWrites: cfg.Storage.SyncWrites,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer func() {
		logger.Info("closing storage")
		if err := store.Close(); err != nil {
			logger.Error("failed to close storage", zap.Error(err))
		}
	}()

	logger.Info("storage initialized",
		zap.String("data_dir", cfg.Storage.DataDir),
	)

	// Ensure default namespace exists
	if err := ensureDefaultNamespace(store, cfg.Memory.DefaultNamespace); err != nil {
		return fmt.Errorf("failed to create default namespace: %w", err)
	}

	// Initialize HTTP server
	srv := server.New(cfg, store, logger)

	// Handle graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			errCh <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownGracePeriod)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}

func initLogger(cfg *config.Config) (*zap.Logger, error) {
	var level zapcore.Level
	switch cfg.Log.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config
	if cfg.Log.Format == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	return zapCfg.Build()
}

func ensureDefaultNamespace(store *badger.Store, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if namespace exists
	_, err := store.GetNamespaceByName(ctx, name)
	if err == nil {
		return nil // Already exists
	}

	// Create default namespace
	_, err = store.CreateNamespace(ctx, &storage.CreateNamespaceInput{
		Name: name,
		Config: storage.NamespaceConfig{
			TokenBudget:       4000,
			InheritFromParent: false,
		},
	})

	return err
}
