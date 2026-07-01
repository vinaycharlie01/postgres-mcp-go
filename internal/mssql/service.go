// Package mssql implements the read-only metadata, discovery, dependency,
// profiling and migration-assessment operations exposed by the MCP server.
//
// Every method in this package is strictly read-only. Queries against
// user-supplied object names are constructed using validated, bracket-quoted
// identifiers and parameterised filter values to prevent SQL injection, and the
// underlying db.Connection enforces row limits and execution timeouts.
package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/vinaykumar-v4_PLIBGIT/mssql-mcp-go/internal/db"
)

// sqlParam builds a named SQL parameter. The go-mssqldb driver matches named
// arguments to "@name" placeholders in the query text. A leading "@" in name is
// accepted and stripped.
func sqlParam(name string, value any) sql.NamedArg {
	return sql.Named(strings.TrimPrefix(name, "@"), value)
}

// Service provides all read-only SQL Server analysis operations.
type Service struct {
	conn     *db.Connection
	database string
}

// NewService creates a new Service backed by the given connection.
func NewService(conn *db.Connection, database string) *Service {
	return &Service{conn: conn, database: database}
}

// Conn exposes the underlying connection (used by the health check tool).
func (s *Service) Conn() *db.Connection {
	return s.conn
}

// Database returns the configured default database name.
func (s *Service) Database() string {
	return s.database
}

// identifierPattern matches a single valid, unquoted SQL Server identifier
// component. We deliberately keep this conservative.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$#@]*$`)

// quoteName validates a single identifier component and returns it wrapped in
// square brackets, escaping any embedded closing bracket. It returns an error
// for identifiers that do not look like safe SQL Server identifiers.
func quoteName(ident string) (string, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return "", fmt.Errorf("identifier must not be empty")
	}
	// Allow already-bracketed input by stripping a single pair of brackets.
	ident = strings.TrimPrefix(ident, "[")
	ident = strings.TrimSuffix(ident, "]")
	if !identifierPattern.MatchString(ident) {
		return "", fmt.Errorf("invalid identifier %q: only letters, digits and _ $ # @ are allowed", ident)
	}
	return "[" + strings.ReplaceAll(ident, "]", "]]") + "]", nil
}

// splitObjectName splits a possibly schema-qualified object name ("schema.obj"
// or "obj") into its schema and object parts. When no schema is supplied the
// provided default is used.
func splitObjectName(name, defaultSchema string) (schema, object string) {
	name = strings.TrimSpace(name)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		schema = strings.Trim(name[:idx], "[] ")
		object = strings.Trim(name[idx+1:], "[] ")
	} else {
		schema = defaultSchema
		object = strings.Trim(name, "[] ")
	}
	if schema == "" {
		schema = defaultSchema
	}
	return schema, object
}

// quoteQualified validates schema and object identifiers and returns the
// bracket-quoted "[schema].[object]" form.
func quoteQualified(schema, object string) (string, error) {
	qs, err := quoteName(schema)
	if err != nil {
		return "", fmt.Errorf("invalid schema: %w", err)
	}
	qo, err := quoteName(object)
	if err != nil {
		return "", fmt.Errorf("invalid object name: %w", err)
	}
	return qs + "." + qo, nil
}

// query is a convenience wrapper around the connection's Query method.
func (s *Service) query(ctx context.Context, sql string, args ...any) (*db.QueryResult, error) {
	return s.conn.Query(ctx, sql, args...)
}

// rows runs a query and returns just the row maps, which is the most common
// shape consumed by tools.
func (s *Service) rows(ctx context.Context, sql string, args ...any) ([]map[string]any, error) {
	res, err := s.query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return res.Rows, nil
}

// scalar runs a query expected to return a single value in the first column of
// the first row.
func (s *Service) scalar(ctx context.Context, sql string, args ...any) (any, error) {
	res, err := s.query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if res.RowCount == 0 || len(res.Columns) == 0 {
		return nil, nil
	}
	return res.Rows[0][res.Columns[0]], nil
}
