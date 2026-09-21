package mcp

import (
	"log/slog"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vinaycharlie01/postgres-mcp-go/internal/postgres"
)

func TestRegisterTools(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	svc := postgres.NewService(nil, "testdb")
	serverWrapper := NewMCPServer(svc, logger)

	impl := &mcp.Implementation{
		Name:    "postgres-mcp-server",
		Version: "1.0.0",
		Title:   "PostgreSQL MCP Server (Read-Only)",
	}
	serverWrapper.server = mcp.NewServer(impl, nil)

	// Verify registerTools completes without panicking
	serverWrapper.registerTools()

	if serverWrapper.server == nil {
		t.Fatal("expected server to be initialized")
	}
}
