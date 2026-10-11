package db

import (
	"context"
	"fmt"
)

// ExplainUserQuery validates a user-supplied query (read-only policy) and then
// returns its estimated execution plan.
func (c *Connection) ExplainUserQuery(ctx context.Context, query string) (string, error) {
	if err := c.validator.Validate(query); err != nil {
		return "", fmt.Errorf("query rejected: %w", err)
	}
	return c.ExplainQuery(ctx, query)
}

// ExplainQuery returns the estimated JSON execution plan for a read-only query
// WITHOUT executing it. It uses EXPLAIN (FORMAT JSON, ANALYZE FALSE), which
// causes PostgreSQL to return the plan instead of running the statement.
//
// The caller MUST validate the query (read-only) before calling this method.
func (c *Connection) ExplainQuery(ctx context.Context, query string) (string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	explainSQL := "EXPLAIN (FORMAT JSON, ANALYZE FALSE, VERBOSE FALSE) " + query //nolint:gosec // fixed prefix, query is validated by caller
	rows, err := c.db.QueryContext(queryCtx, explainSQL)
	if err != nil {
		return "", fmt.Errorf("failed to produce execution plan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var plan string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", fmt.Errorf("failed to scan plan: %w", err)
		}
		plan += line
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error reading plan: %w", err)
	}
	if plan == "" {
		return "", fmt.Errorf("no execution plan was produced")
	}
	return plan, nil
}
