package postgres

import (
	"context"
	"regexp"
	"strconv"
)

var (
	reTotalCost = regexp.MustCompile(`"Total Cost":\s*([0-9.eE+-]+)`)
	reNodeType  = regexp.MustCompile(`"Node Type":\s*"([^"]+)"`)
)

// ExplainQuery returns the estimated JSON execution plan for a read-only query.
func (s *Service) ExplainQuery(ctx context.Context, query string) (map[string]any, error) {
	plan, err := s.conn.ExplainUserQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":         query,
		"executionPlan": plan,
	}, nil
}

// EstimateQueryCost returns the estimated total cost parsed from the execution plan.
func (s *Service) EstimateQueryCost(ctx context.Context, query string) (map[string]any, error) {
	plan, err := s.conn.ExplainUserQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	var maxCost float64
	for _, m := range reTotalCost.FindAllStringSubmatch(plan, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > maxCost {
			maxCost = v
		}
	}
	return map[string]any{
		"query":              query,
		"estimatedTotalCost": maxCost,
	}, nil
}

// AnalyzeExecutionPlan returns the plan plus a summary of operators used.
func (s *Service) AnalyzeExecutionPlan(ctx context.Context, query string) (map[string]any, error) {
	plan, err := s.conn.ExplainUserQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	opCounts := map[string]int{}
	for _, m := range reNodeType.FindAllStringSubmatch(plan, -1) {
		opCounts[m[1]]++
	}
	var maxCost float64
	for _, m := range reTotalCost.FindAllStringSubmatch(plan, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > maxCost {
			maxCost = v
		}
	}
	return map[string]any{
		"query":              query,
		"estimatedTotalCost": maxCost,
		"operators":          opCounts,
		"executionPlan":      plan,
	}, nil
}

// FindMissingIndexes reports tables with high sequential scans where indexes may help.
func (s *Service) FindMissingIndexes(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT schemaname AS schema_name,
       relname    AS table_name,
       seq_scan,
       seq_tup_read,
       idx_scan,
       idx_tup_fetch,
       n_live_tup AS row_count
FROM   pg_stat_user_tables
WHERE  seq_scan > 0
ORDER  BY seq_tup_read DESC
LIMIT  100`)
}

// FindUnusedIndexes reports non-primary, non-unique indexes that have never been scanned.
func (s *Service) FindUnusedIndexes(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT ui.schemaname AS schema_name,
       ui.relname    AS table_name,
       ui.indexrelname AS index_name,
       ui.idx_scan,
       ui.idx_tup_read,
       ui.idx_tup_fetch,
       pg_size_pretty(pg_relation_size(ui.indexrelid)) AS index_size
FROM   pg_stat_user_indexes ui
JOIN   pg_index i ON i.indexrelid = ui.indexrelid
WHERE  NOT i.indisprimary AND NOT i.indisunique AND ui.idx_scan = 0
ORDER  BY pg_relation_size(ui.indexrelid) DESC`)
}

// FindIndexFragmentation reports index sizes and disk usage across user tables.
func (s *Service) FindIndexFragmentation(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS table_name,
       i.relname AS index_name,
       pg_size_pretty(pg_relation_size(i.oid)) AS index_size,
       pg_relation_size(i.oid) AS index_bytes
FROM   pg_catalog.pg_class i
JOIN   pg_catalog.pg_index ix ON ix.indexrelid = i.oid
JOIN   pg_catalog.pg_class c  ON c.oid = ix.indrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER  BY pg_relation_size(i.oid) DESC
LIMIT  100`)
}

// FindFragmentedIndexes reports indexes with high disk usage / potential fragmentation.
func (s *Service) FindFragmentedIndexes(ctx context.Context) ([]map[string]any, error) {
	return s.FindIndexFragmentation(ctx)
}

// IndexUsageReport returns read/write usage statistics for all user indexes.
func (s *Service) IndexUsageReport(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT schemaname AS schema_name,
       relname    AS table_name,
       indexrelname AS index_name,
       idx_scan,
       idx_tup_read,
       idx_tup_fetch,
       pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
FROM   pg_stat_user_indexes
ORDER  BY idx_scan DESC`)
}

// SlowQueryAnalysis returns expensive cached queries or running queries.
func (s *Service) SlowQueryAnalysis(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.rows(ctx, `
SELECT query,
       calls AS execution_count,
       total_exec_time AS total_elapsed_ms,
       mean_exec_time AS avg_elapsed_ms,
       rows AS total_rows
FROM   pg_stat_statements
ORDER  BY mean_exec_time DESC
LIMIT  50`)
	if err == nil {
		return rows, nil
	}
	return s.rows(ctx, `
SELECT query,
       1 AS execution_count,
       EXTRACT(EPOCH FROM (now() - query_start)) * 1000 AS total_elapsed_ms,
       EXTRACT(EPOCH FROM (now() - query_start)) * 1000 AS avg_elapsed_ms,
       state,
       usename AS user_name
FROM   pg_stat_activity
WHERE  state != 'idle' AND query NOT LIKE '%pg_stat_activity%'
ORDER  BY query_start ASC
LIMIT  50`)
}

// QueryPerformanceReport returns aggregate query performance statistics.
func (s *Service) QueryPerformanceReport(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.rows(ctx, `
SELECT query,
       calls AS execution_count,
       total_exec_time AS total_time_ms,
       mean_exec_time AS avg_time_ms,
       rows AS total_rows
FROM   pg_stat_statements
ORDER  BY total_exec_time DESC
LIMIT  50`)
	if err == nil {
		return rows, nil
	}
	return s.rows(ctx, `
SELECT query,
       1 AS execution_count,
       EXTRACT(EPOCH FROM (now() - query_start)) * 1000 AS total_time_ms,
       state,
       usename AS user_name
FROM   pg_stat_activity
WHERE  state != 'idle' AND query NOT LIKE '%pg_stat_activity%'
ORDER  BY query_start ASC
LIMIT  50`)
}
