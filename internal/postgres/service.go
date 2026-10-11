// Package postgres implements the read-only PostgreSQL metadata, discovery,
// analysis and monitoring operations exposed by the MCP server.
//
// Every method is strictly read-only. Identifiers supplied by callers are
// validated by splitObjectName before use; all filter values are passed as
// positional parameters ($1, $2, …) to prevent SQL injection. The underlying
// db.Connection enforces row limits and execution timeouts.
package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vinaycharlie01/postgres-mcp-go/internal/db"
)

// Service provides all read-only PostgreSQL analysis operations.
type Service struct {
	conn     *db.Connection
	database string
}

// NewService creates a new Service backed by the given connection.
func NewService(conn *db.Connection, database string) *Service {
	return &Service{conn: conn, database: database}
}

// Conn exposes the underlying connection (used by the health-check tool).
func (s *Service) Conn() *db.Connection { return s.conn }

// Database returns the configured default database name.
func (s *Service) Database() string { return s.database }

// identifierPattern accepts conservative PostgreSQL unquoted identifiers.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// quoteName validates one identifier component and returns it double-quoted.
func quoteName(ident string) (string, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return "", fmt.Errorf("identifier must not be empty")
	}
	if len(ident) >= 2 && ident[0] == '"' && ident[len(ident)-1] == '"' {
		ident = ident[1 : len(ident)-1]
	}
	if !identifierPattern.MatchString(ident) {
		return "", fmt.Errorf("invalid identifier %q", ident)
	}
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`, nil
}

// splitObjectName splits "schema.object" → (schema, object).
// Falls back to defaultSchema when no schema prefix is present.
func splitObjectName(name, defaultSchema string) (schema, object string) { //nolint:unparam // defaultSchema kept for API consistency
	name = strings.TrimSpace(name)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		schema = strings.Trim(name[:idx], `" `)
		object = strings.Trim(name[idx+1:], `" `)
	} else {
		schema = defaultSchema
		object = strings.Trim(name, `" `)
	}
	if schema == "" {
		schema = defaultSchema
	}
	return schema, object
}

// quoteQualified returns the fully double-quoted "schema"."object" form.
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

// rows executes sql and returns the row maps.
func (s *Service) rows(ctx context.Context, sql string, args ...any) ([]map[string]any, error) {
	res, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return res.Rows, nil
}

// scalar executes sql and returns the first column of the first row.
func (s *Service) scalar(ctx context.Context, sql string, args ...any) (any, error) {
	res, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if res.RowCount == 0 || len(res.Columns) == 0 {
		return nil, nil
	}
	return res.Rows[0][res.Columns[0]], nil
}

// toInt64 converts common driver numeric types to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
