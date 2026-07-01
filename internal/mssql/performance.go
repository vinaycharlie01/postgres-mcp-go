package mssql

import (
	"context"
	"regexp"
	"strconv"
)

var reEstimatedCost = regexp.MustCompile(`StatementSubTreeCost="([0-9.eE+-]+)"`)

// ExplainQuery returns the estimated XML execution plan for a read-only query.
func (s *Service) ExplainQuery(ctx context.Context, query string) (map[string]any, error) {
	plan, err := s.conn.ExplainUserQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":        query,
		"executionPlan": plan,
	}, nil
}

// EstimateQueryCost returns the estimated subtree cost parsed from the query's
// estimated execution plan.
func (s *Service) EstimateQueryCost(ctx context.Context, query string) (map[string]any, error) {
	plan, err := s.conn.ExplainUserQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	var maxCost float64
	for _, m := range reEstimatedCost.FindAllStringSubmatch(plan, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > maxCost {
			maxCost = v
		}
	}
	return map[string]any{
		"query":              query,
		"estimatedSubtreeCost": maxCost,
	}, nil
}

// AnalyzeExecutionPlan returns the plan plus a short summary of operators used.
func (s *Service) AnalyzeExecutionPlan(ctx context.Context, query string) (map[string]any, error) {
	plan, err := s.conn.ExplainUserQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	physicalOps := regexp.MustCompile(`PhysicalOp="([^"]+)"`)
	opCounts := map[string]int{}
	for _, m := range physicalOps.FindAllStringSubmatch(plan, -1) {
		opCounts[m[1]]++
	}
	var maxCost float64
	for _, m := range reEstimatedCost.FindAllStringSubmatch(plan, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > maxCost {
			maxCost = v
		}
	}
	return map[string]any{
		"query":                query,
		"estimatedSubtreeCost": maxCost,
		"operators":            opCounts,
		"executionPlan":        plan,
	}, nil
}

// FindMissingIndexes reports indexes the optimizer believes would help, from the
// missing-index DMVs.
func (s *Service) FindMissingIndexes(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT TOP (100)
    DB_NAME(mid.database_id)                       AS database_name,
    OBJECT_SCHEMA_NAME(mid.object_id, mid.database_id) AS schema_name,
    OBJECT_NAME(mid.object_id, mid.database_id)    AS table_name,
    migs.avg_user_impact                           AS avg_user_impact,
    migs.user_seeks + migs.user_scans              AS demand,
    mid.equality_columns                           AS equality_columns,
    mid.inequality_columns                         AS inequality_columns,
    mid.included_columns                           AS included_columns
FROM sys.dm_db_missing_index_groups mig
JOIN sys.dm_db_missing_index_group_stats migs ON mig.index_group_handle = migs.group_handle
JOIN sys.dm_db_missing_index_details mid ON mig.index_handle = mid.index_handle
WHERE mid.database_id = DB_ID()
ORDER BY migs.avg_user_impact DESC`
	return s.rows(ctx, sql)
}

// FindUnusedIndexes reports non-primary indexes that have never been read but
// incur write maintenance.
func (s *Service) FindUnusedIndexes(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name                                  AS schema_name,
    t.name                                  AS table_name,
    i.name                                  AS index_name,
    ISNULL(us.user_seeks, 0)                AS user_seeks,
    ISNULL(us.user_scans, 0)                AS user_scans,
    ISNULL(us.user_lookups, 0)              AS user_lookups,
    ISNULL(us.user_updates, 0)              AS user_updates
FROM sys.indexes i
JOIN sys.tables t ON i.object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
LEFT JOIN sys.dm_db_index_usage_stats us
       ON i.object_id = us.object_id AND i.index_id = us.index_id AND us.database_id = DB_ID()
WHERE i.type_desc IN ('NONCLUSTERED', 'CLUSTERED')
  AND i.is_primary_key = 0
  AND i.is_unique_constraint = 0
  AND ISNULL(us.user_seeks, 0) + ISNULL(us.user_scans, 0) + ISNULL(us.user_lookups, 0) = 0
ORDER BY us.user_updates DESC`
	return s.rows(ctx, sql)
}

// FindFragmentedIndexes reports indexes with high average fragmentation.
func (s *Service) FindFragmentedIndexes(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name                                      AS schema_name,
    t.name                                      AS table_name,
    i.name                                      AS index_name,
    ips.avg_fragmentation_in_percent            AS avg_fragmentation_percent,
    ips.page_count                              AS page_count
FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'LIMITED') ips
JOIN sys.indexes i ON ips.object_id = i.object_id AND ips.index_id = i.index_id
JOIN sys.tables t ON i.object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE ips.avg_fragmentation_in_percent > 10 AND ips.page_count > 100 AND i.name IS NOT NULL
ORDER BY ips.avg_fragmentation_in_percent DESC`
	return s.rows(ctx, sql)
}

// IndexUsageReport returns read/write usage statistics for all indexes.
func (s *Service) IndexUsageReport(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name                          AS schema_name,
    t.name                          AS table_name,
    i.name                          AS index_name,
    i.type_desc                     AS index_type,
    ISNULL(us.user_seeks, 0)        AS user_seeks,
    ISNULL(us.user_scans, 0)        AS user_scans,
    ISNULL(us.user_lookups, 0)      AS user_lookups,
    ISNULL(us.user_updates, 0)      AS user_updates,
    us.last_user_seek               AS last_user_seek,
    us.last_user_scan               AS last_user_scan
FROM sys.indexes i
JOIN sys.tables t ON i.object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
LEFT JOIN sys.dm_db_index_usage_stats us
       ON i.object_id = us.object_id AND i.index_id = us.index_id AND us.database_id = DB_ID()
WHERE i.name IS NOT NULL
ORDER BY (ISNULL(us.user_seeks,0) + ISNULL(us.user_scans,0) + ISNULL(us.user_lookups,0)) DESC`
	return s.rows(ctx, sql)
}

// SlowQueryAnalysis returns the most expensive cached queries by average
// elapsed time.
func (s *Service) SlowQueryAnalysis(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT TOP (50)
    qs.execution_count                                              AS execution_count,
    qs.total_elapsed_time / 1000                                    AS total_elapsed_ms,
    (qs.total_elapsed_time / qs.execution_count) / 1000             AS avg_elapsed_ms,
    qs.total_logical_reads                                          AS total_logical_reads,
    qs.total_worker_time / 1000                                     AS total_cpu_ms,
    SUBSTRING(st.text, (qs.statement_start_offset/2)+1,
        ((CASE qs.statement_end_offset WHEN -1 THEN DATALENGTH(st.text)
            ELSE qs.statement_end_offset END - qs.statement_start_offset)/2)+1) AS query_text
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
ORDER BY (qs.total_elapsed_time / qs.execution_count) DESC`
	return s.rows(ctx, sql)
}

// QueryPerformanceReport returns aggregate query performance statistics from the
// plan cache, ordered by total worker time.
func (s *Service) QueryPerformanceReport(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT TOP (50)
    qs.execution_count                                  AS execution_count,
    qs.total_worker_time / 1000                         AS total_cpu_ms,
    qs.total_logical_reads                              AS total_logical_reads,
    qs.total_logical_writes                             AS total_logical_writes,
    qs.total_elapsed_time / 1000                        AS total_elapsed_ms,
    qs.creation_time                                    AS creation_time,
    qs.last_execution_time                              AS last_execution_time,
    SUBSTRING(st.text, (qs.statement_start_offset/2)+1,
        ((CASE qs.statement_end_offset WHEN -1 THEN DATALENGTH(st.text)
            ELSE qs.statement_end_offset END - qs.statement_start_offset)/2)+1) AS query_text
FROM sys.dm_exec_query_stats qs
CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) st
ORDER BY qs.total_worker_time DESC`
	return s.rows(ctx, sql)
}
