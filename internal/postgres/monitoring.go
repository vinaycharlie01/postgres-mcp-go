package postgres

import "context"

// ConnectionPoolStats returns the current connection pool statistics.
func (s *Service) ConnectionPoolStats(_ context.Context) (map[string]any, error) {
	return s.conn.PoolStats(), nil
}

// DatabaseSizeReport returns the disk usage of the current database.
func (s *Service) DatabaseSizeReport(ctx context.Context) (map[string]any, error) {
	row, err := s.conn.QueryRow(ctx, `
SELECT current_database() AS database_name,
       pg_database_size(current_database()) AS total_bytes,
       pg_size_pretty(pg_database_size(current_database())) AS total_size`)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	return row, nil
}

// TableGrowthReport returns the largest tables by current size as a proxy for growth.
func (s *Service) TableGrowthReport(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS table_name,
       c.reltuples::bigint AS row_count,
       pg_total_relation_size(c.oid) AS total_bytes,
       pg_size_pretty(pg_total_relation_size(c.oid)) AS total_size,
       pg_relation_size(c.oid) AS table_bytes,
       pg_size_pretty(pg_relation_size(c.oid)) AS table_size,
       pg_indexes_size(c.oid) AS index_bytes,
       pg_size_pretty(pg_indexes_size(c.oid)) AS index_size
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'r'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
ORDER  BY pg_total_relation_size(c.oid) DESC
LIMIT  $1`, limit)
}
