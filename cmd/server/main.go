package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vinaycharlie01/postgres-mcp-go/internal/adapters/mcp"
	"github.com/vinaycharlie01/postgres-mcp-go/internal/db"
	"github.com/vinaycharlie01/postgres-mcp-go/internal/infrastructure"
	"github.com/vinaycharlie01/postgres-mcp-go/internal/postgres"
)

func main() {
	// Load and validate configuration.
	config, err := infrastructure.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := config.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	// Initialise the logger. For stdio transport logs go to stderr so they do
	// not corrupt the MCP protocol stream on stdout.
	logger := infrastructure.NewLogger(config)
	slog.SetDefault(logger)
	logger.Info("starting MCP PostgreSQL Server (read-only)",
		"version", "1.0.0",
		"transport", config.MCP.Transport,
		"database", config.Database.Database,
	)

	// Root context cancelled on shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish the singleton connection pool.
	conn, err := db.GetConnection(ctx, config, logger)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		cancel()
		os.Exit(1) //nolint:gocritic // cancel called explicitly above
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			logger.Error("error closing database connection", "error", cerr)
		}
	}()

	// Build the service and MCP server.
	service := postgres.NewService(conn, config.Database.Database)
	mcpServer := mcp.NewMCPServer(service, logger)

	// Signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if err := mcpServer.Start(ctx); err != nil {
			errStr := err.Error()
			if errors.Is(err, io.EOF) ||
				errStr == "server is closing: EOF" ||
				errStr == "failed to run MCP server: server is closing: EOF" {
				logger.Info("MCP stdin closed, shutting down gracefully")
				errChan <- nil
			} else {
				errChan <- fmt.Errorf("MCP server error: %w", err)
			}
		} else {
			errChan <- nil
		}
	}()

	select {
	case <-sigChan:
		logger.Info("received shutdown signal")
	case err := <-errChan:
		if err != nil {
			logger.Error("server error", "error", err)
		}
	}

	logger.Info("shutting down...")
	cancel()
	if err := mcpServer.Stop(context.Background()); err != nil {
		logger.Error("error during shutdown", "error", err)
	}
	logger.Info("server stopped")
}
