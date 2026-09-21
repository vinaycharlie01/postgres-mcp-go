package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/vinaycharlie01/postgres-mcp-go/internal/infrastructure"

	// Register the PostgreSQL driver.
	_ "github.com/lib/pq"
)

// driverName is the database/sql driver name registered by lib/pq.
const driverName = "postgres"

// Connection is a thin, strictly read-only wrapper around a *sql.DB connection
// pool. It enforces query validation, row limits and per-query timeouts.
//
// A single Connection instance is created per process (see GetConnection) and is
// safe for concurrent use by multiple goroutines.
type Connection struct {
	db        *sql.DB
	validator *Validator
	logger    *slog.Logger

	maxRows      int
	queryTimeout time.Duration
}

var (
	instance   *Connection
	instanceMu sync.Mutex
)

// QueryResult is the canonical shape returned by every read query. Columns
// preserves the column order; Rows holds one map per row keyed by column name.
type QueryResult struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"rowCount"`
}

// GetConnection returns the singleton database connection, creating and
// verifying it on first use. Subsequent calls return the same instance.
func GetConnection(ctx context.Context, cfg *infrastructure.Config, logger *slog.Logger) (*Connection, error) {
	instanceMu.Lock()
	defer instanceMu.Unlock()

	if instance != nil {
		return instance, nil
	}

	conn, err := newConnection(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	instance = conn
	return instance, nil
}

// newConnection builds the connection pool, configures it and verifies
// connectivity with a ping.
func newConnection(ctx context.Context, cfg *infrastructure.Config, logger *slog.Logger) (*Connection, error) {
	dsn := buildDSN(cfg)

	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure the pool per the production requirements.
	sqlDB.SetMaxOpenConns(cfg.Pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Pool.ConnMaxLifetime.AsDuration())

	conn := &Connection{
		db:           sqlDB,
		validator:    NewValidator(cfg.Query.MaxLength),
		logger:       logger,
		maxRows:      cfg.Query.MaxRows,
		queryTimeout: cfg.Query.Timeout.AsDuration(),
	}

	// Verify connectivity with a short, bounded ping (with one retry) so the
	// server fails fast on misconfiguration.
	if err := conn.pingWithRetry(ctx, 2); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	logger.Info("database connection pool established",
		slog.String("host", cfg.Database.Host),
		slog.Int("port", cfg.Database.Port),
		slog.String("database", cfg.Database.Database),
		slog.Int("max_open_conns", cfg.Pool.MaxOpenConns),
		slog.Int("max_idle_conns", cfg.Pool.MaxIdleConns),
	)

	return conn, nil
}

// buildDSN constructs the lib/pq PostgreSQL connection string (DSN).
// The application session is pinned to read-only validation via the Validator;
// true enforcement is performed there.
func buildDSN(cfg *infrastructure.Config) string {
	q := url.Values{}
	q.Set("dbname", cfg.Database.Database)
	q.Set("sslmode", cfg.Database.SSLMode)
	q.Set("connect_timeout", fmt.Sprintf("%d", cfg.Database.ConnectionTimeout))
	q.Set("application_name", "mcp-postgres-go")
	if cfg.Database.SearchPath != "" {
		q.Set("search_path", cfg.Database.SearchPath)
	}

	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.Database.User, cfg.Database.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Database.Host, cfg.Database.Port),
		RawQuery: q.Encode(),
	}
	return u.String()
}

// pingWithRetry pings the database, retrying a bounded number of times with a
// short backoff to tolerate transient startup races.
func (c *Connection) pingWithRetry(ctx context.Context, attempts int) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		c.logger.Warn("database ping failed, retrying",
			slog.Int("attempt", i+1),
			slog.String("error", err.Error()),
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * time.Second):
		}
	}
	return lastErr
}

// Ping verifies the database is reachable within a bounded timeout.
func (c *Connection) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.db.PingContext(pingCtx)
}

// Stats returns the current connection pool statistics.
func (c *Connection) Stats() sql.DBStats {
	return c.db.Stats()
}

// PoolStats returns the connection pool statistics as a serialisable map.
func (c *Connection) PoolStats() map[string]any {
	s := c.db.Stats()
	return map[string]any{
		"maxOpenConnections": s.MaxOpenConnections,
		"openConnections":    s.OpenConnections,
		"inUse":              s.InUse,
		"idle":               s.Idle,
		"waitCount":          s.WaitCount,
		"waitDuration":       s.WaitDuration.String(),
		"maxIdleClosed":      s.MaxIdleClosed,
		"maxIdleTimeClosed":  s.MaxIdleTimeClosed,
		"maxLifetimeClosed":  s.MaxLifetimeClosed,
	}
}

// MaxRows returns the configured maximum number of rows returned by a query.
func (c *Connection) MaxRows() int {
	return c.maxRows
}

// Close gracefully closes the underlying connection pool.
func (c *Connection) Close() error {
	if c.db == nil {
		return nil
	}
	c.logger.Info("closing database connection pool")
	return c.db.Close()
}

// ExecuteQuery validates and runs a user-supplied read-only query, enforcing the
// configured row limit and execution timeout.
func (c *Connection) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	if err := c.validator.Validate(query); err != nil {
		return nil, fmt.Errorf("query rejected: %w", err)
	}
	return c.Query(ctx, query)
}

// Query runs an already-trusted SQL statement (built internally by the server's
// metadata tools) and returns the rows. It still enforces the row limit and
// execution timeout. User-supplied queries must go through ExecuteQuery so they
// are validated first.
func (c *Connection) Query(ctx context.Context, query string, args ...any) (*QueryResult, error) {
	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	start := time.Now()
	rows, err := c.db.QueryContext(queryCtx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	result, err := scanRows(rows, c.maxRows)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("query executed",
		slog.Int("rows", result.RowCount),
		slog.Duration("duration", time.Since(start)),
	)

	return result, nil
}

// QueryRow runs a query expected to return a single row and returns it as a map.
// When no rows are returned it returns (nil, nil).
func (c *Connection) QueryRow(ctx context.Context, query string, args ...any) (map[string]any, error) {
	result, err := c.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if result.RowCount == 0 {
		return nil, nil
	}
	return result.Rows[0], nil
}

// scanRows converts a *sql.Rows into a QueryResult, capping the number of rows
// at maxRows. Byte slices are converted to strings so the result serialises to
// readable JSON.
func scanRows(rows *sql.Rows, maxRows int) (*QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to read columns: %w", err)
	}

	result := &QueryResult{
		Columns: columns,
		Rows:    make([]map[string]any, 0),
	}

	for rows.Next() {
		if maxRows > 0 && result.RowCount >= maxRows {
			// Stop reading additional rows beyond the configured cap.
			break
		}

		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			rowMap[col] = normalizeValue(values[i])
		}
		result.Rows = append(result.Rows, rowMap)
		result.RowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}

// normalizeValue converts driver values into JSON-friendly Go values.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return val
	}
}
