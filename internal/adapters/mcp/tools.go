package mcp

import (
	"context"
)

// Input types for the registered tools. Field tags drive the generated JSON
// schema that MCP clients see.

type emptyInput struct{}

type schemaInput struct {
	Schema string `json:"schema,omitempty" jsonschema_description:"Optional schema name to filter by (e.g. dbo). Leave empty for all schemas."`
}

type tableInput struct {
	Table string `json:"table" jsonschema_description:"Table name, optionally schema-qualified (e.g. dbo.Users or Users)." jsonschema:"required"`
}

type columnInput struct {
	Table  string `json:"table" jsonschema_description:"Table name, optionally schema-qualified." jsonschema:"required"`
	Column string `json:"column" jsonschema_description:"Column name." jsonschema:"required"`
}

type queryInput struct {
	Query string `json:"query" jsonschema_description:"A read-only SELECT or WITH (CTE) query. Any data- or schema-modifying statement is rejected." jsonschema:"required"`
}

// list wraps a slice result in an object with a count, matching the MCP
// convention of returning structured objects.
func list(key string, items []map[string]any) any {
	if items == nil {
		items = []map[string]any{}
	}

	return map[string]any{
		key:     items,
		"count": len(items),
	}
}

// registerTools registers the focused, read-only tool set: query/health, schema
// discovery, table design (columns + data types), and foreign-key relationships.
func (s *MCPServer) registerTools() {
	svc := s.service
	srv := s.server

	// --- Core query tools ---

	registerTool(srv, "execute_query",
		"Execute a strictly read-only SELECT/WITH query and return columns, rows and row count. Dangerous statements are rejected.",
		func(ctx context.Context, in queryInput) (any, error) {
			return svc.Conn().ExecuteQuery(ctx, in.Query)
		})

	registerTool(srv, "health_check",
		"Check database connectivity and return connection pool statistics.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			status := "healthy"
			database := "connected"
			ping := "success"
			if err := svc.Conn().Ping(ctx); err != nil {
				status = "unhealthy"
				database = "disconnected"
				ping = err.Error()
			}
			return map[string]any{
				"status":    status,
				"database":  database,
				"ping":      ping,
				"poolStats": svc.Conn().PoolStats(),
			}, nil
		})

	// --- Database discovery ---

	registerTool(srv, "list_databases",
		"List all databases accessible to the current login.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.ListDatabases(ctx)
			return list("databases", rows), err
		})

	registerTool(srv, "list_schemas",
		"List all schemas in the current database.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.ListSchemas(ctx)
			return list("schemas", rows), err
		})

	registerTool(srv, "list_tables",
		"List all tables, optionally filtered by schema. Includes row counts and column counts.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListTables(ctx, in.Schema)
			return list("tables", rows), err
		})

	registerTool(srv, "describe_table",
		"Describe a table's design: columns with data types, primary keys, foreign keys (links to other tables) and indexes.",
		func(ctx context.Context, in tableInput) (any, error) {
			return svc.DescribeTable(ctx, in.Table)
		})

	registerTool(srv, "list_columns",
		"List the columns of a table or view with their data types and nullability.",
		func(ctx context.Context, in tableInput) (any, error) {
			rows, err := svc.ListColumns(ctx, in.Table)
			return list("columns", rows), err
		})

	registerTool(srv, "describe_column",
		"Return detailed metadata for a single column (data type, precision, default, computed definition).",
		func(ctx context.Context, in columnInput) (any, error) {
			return svc.DescribeColumn(ctx, in.Table, in.Column)
		})

	registerTool(srv, "table_row_count",
		"Return the row count of a table using catalog metadata (no table scan).",
		func(ctx context.Context, in tableInput) (any, error) {
			return svc.TableRowCount(ctx, in.Table)
		})

	// --- Relationships & keys ---

	registerTool(srv, "table_relationships",
		"Show how a table links to other tables: its outbound foreign keys (references) and inbound foreign keys (referenced by).",
		func(ctx context.Context, in tableInput) (any, error) {
			return svc.TableRelationships(ctx, in.Table)
		})

	registerTool(srv, "list_foreign_keys",
		"List foreign key relationships, optionally filtered by schema, including referenced tables and columns.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListForeignKeys(ctx, in.Schema)
			return list("foreignKeys", rows), err
		})

	registerTool(srv, "list_primary_keys",
		"List primary key constraints and their key columns, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListPrimaryKeys(ctx, in.Schema)
			return list("primaryKeys", rows), err
		})

	registerTool(srv, "foreign_key_graph",
		"Return the full foreign-key graph of the database as nodes and edges.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.ForeignKeyGraph(ctx)
		})

	// Stored procedure, view, function and trigger tools.
	s.registerObjectTools()

	// Relationship graphs, search, profiling, performance, security, migration
	// and monitoring tools.
	s.registerAnalysisTools()
}
