package db

import (
	"context"
	"fmt"
	"time"
)

// ExplainUserQuery validates a user-supplied query (read-only policy) and then
// returns its estimated execution plan.
func (c *Connection) ExplainUserQuery(ctx context.Context, query string) (string, error) {
	if err := c.validator.Validate(query); err != nil {
		return "", fmt.Errorf("query rejected: %w", err)
	}
	return c.ExplainQuery(ctx, query)
}

// ExplainQuery returns the estimated XML execution plan for a read-only query
// WITHOUT executing it. It uses SET SHOWPLAN_XML ON, which causes SQL Server to
// return the estimated plan instead of running the statement.
//
// The caller MUST validate the query (read-only) before calling this method.
// ExplainQuery pins a single dedicated connection so the SET option and the
// query run in the same session, and always turns the option back off.
func (c *Connection) ExplainQuery(ctx context.Context, query string) (string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	conn, err := c.db.Conn(queryCtx)
	if err != nil {
		return "", fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(queryCtx, "SET SHOWPLAN_XML ON"); err != nil {
		return "", fmt.Errorf("failed to enable showplan: %w", err)
	}
	// Best-effort restore of the session setting.
	defer func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer restoreCancel()
		_, _ = conn.ExecContext(restoreCtx, "SET SHOWPLAN_XML OFF")
	}()

	rows, err := conn.QueryContext(queryCtx, query)
	if err != nil {
		return "", fmt.Errorf("failed to produce execution plan: %w", err)
	}
	defer rows.Close()

	var plan string
	for rows.Next() {
		var planXML string
		if err := rows.Scan(&planXML); err != nil {
			return "", fmt.Errorf("failed to scan plan: %w", err)
		}
		plan += planXML
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error reading plan: %w", err)
	}
	if plan == "" {
		return "", fmt.Errorf("no execution plan was produced")
	}
	return plan, nil
}
