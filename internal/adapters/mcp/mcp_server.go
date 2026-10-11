// Package mcp implements the MCP server adapter: it owns the server lifecycle
// and registers the read-only PostgreSQL tools.
package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vinaycharlie01/postgres-mcp-go/internal/postgres"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the MCP SDK server and the PostgreSQL service.
type MCPServer struct {
	server  *mcp.Server
	service *postgres.Service
	logger  *slog.Logger
}

// NewMCPServer creates a new MCP server backed by the given service.
func NewMCPServer(service *postgres.Service, logger *slog.Logger) *MCPServer {
	return &MCPServer{
		service: service,
		logger:  logger,
	}
}

// Start initialises the MCP server, registers tools and runs the stdio
// transport until the context is cancelled or stdin closes.
func (s *MCPServer) Start(ctx context.Context) error {
	impl := &mcp.Implementation{
		Name:    "postgres-mcp-server",
		Version: "1.0.0",
		Title:   "PostgreSQL MCP Server (Read-Only)",
	}

	server := mcp.NewServer(impl, nil)
	s.server = server

	s.registerTools()
	s.logger.Info("registered MCP tools (read-only mode)")

	transport := &mcp.StdioTransport{}
	if err := server.Run(ctx, transport); err != nil {
		return fmt.Errorf("failed to run MCP server: %w", err)
	}

	return nil
}

// Stop performs any cleanup required when shutting down.
func (s *MCPServer) Stop(_ context.Context) error {
	s.logger.Info("stopping MCP server")
	return nil
}

// registerTool is a small generic helper that adds a tool whose handler returns
// an arbitrary serialisable result plus an error.
func registerTool[In any](srv *mcp.Server, name, description string, fn func(context.Context, In) (any, error)) {
	tool := &mcp.Tool{Name: name, Description: description}
	mcp.AddTool(srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		result, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return nil, result, nil
	})
}
