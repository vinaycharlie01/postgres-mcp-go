package postgres

import "context"

// DatabaseOverview returns high-level object counts for the database.
func (s *Service) DatabaseOverview(ctx context.Context) (map[string]any, error) {
	row, err := s.conn.QueryRow(ctx, `
SELECT current_database() AS database_name,
       (SELECT COUNT(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')) AS table_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'v' AND n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')) AS view_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace WHERE p.prokind = 'p' AND n.nspname NOT IN ('pg_catalog','information_schema')) AS procedure_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace WHERE p.prokind = 'f' AND n.nspname NOT IN ('pg_catalog','information_schema')) AS function_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_trigger tr JOIN pg_catalog.pg_class c ON c.oid = tr.tgrelid JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE NOT tr.tgisinternal AND n.nspname NOT IN ('pg_catalog','information_schema')) AS trigger_count`)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	return row, nil
}

// DatabaseHealthReport returns health indicators for the database.
func (s *Service) DatabaseHealthReport(ctx context.Context) (map[string]any, error) {
	tablesWithoutIndex, err := s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'r'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
  AND  NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_index i WHERE i.indrelid = c.oid
  )
ORDER  BY n.nspname, c.relname`)
	if err != nil {
		return nil, err
	}

	noPK, err := s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'r'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
  AND  NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_constraint con
      WHERE con.conrelid = c.oid AND con.contype = 'p'
  )
ORDER  BY n.nspname, c.relname`)
	if err != nil {
		return nil, err
	}

	unvalidatedConstraints, err := s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name, con.conname AS constraint_name
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  NOT con.convalidated
ORDER  BY n.nspname, c.relname`)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"tablesWithoutIndexes":      tablesWithoutIndex,
		"tablesWithoutIndexesCount": len(tablesWithoutIndex),
		"tablesWithoutPrimaryKey":   noPK,
		"tablesWithoutPKCount":      len(noPK),
		"unvalidatedConstraints":    unvalidatedConstraints,
	}, nil
}

// DatabaseComplexityReport returns metrics describing the structural complexity.
func (s *Service) DatabaseComplexityReport(ctx context.Context) (map[string]any, error) {
	row, err := s.conn.QueryRow(ctx, `
SELECT (SELECT COUNT(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')) AS table_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_constraint WHERE contype = 'f') AS foreign_key_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_trigger WHERE NOT tgisinternal) AS trigger_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace WHERE p.prokind = 'p' AND n.nspname NOT IN ('pg_catalog','information_schema')) AS procedure_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace WHERE p.prokind = 'f' AND n.nspname NOT IN ('pg_catalog','information_schema')) AS function_count,
       (SELECT COALESCE(AVG(col_count), 0) FROM (SELECT COUNT(*) AS col_count FROM pg_catalog.pg_attribute WHERE attnum > 0 AND NOT attisdropped GROUP BY attrelid) x) AS avg_columns_per_object,
       (SELECT COALESCE(MAX(LENGTH(prosrc)), 0) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema')) AS largest_module_length`)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	return row, nil
}

// LargeTableAnalysis returns the largest tables by row count and storage.
func (s *Service) LargeTableAnalysis(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
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

// LegacyObjectAnalysis lists database objects by type and schema.
func (s *Service) LegacyObjectAnalysis(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS object_name,
       CASE c.relkind WHEN 'r' THEN 'TABLE' WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' ELSE c.relkind::text END AS object_type
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind IN ('r','v','m')
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
ORDER  BY n.nspname, c.relname`)
}

// DeprecatedFeatureDetection scans function definitions for potential issues.
func (s *Service) DeprecatedFeatureDetection(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       p.proname AS object_name,
       CASE p.prokind WHEN 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END AS object_type,
       'Use of dynamic EXECUTE' AS finding
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.prosrc ILIKE '%EXECUTE %'
  AND  n.nspname NOT IN ('pg_catalog','information_schema')
ORDER  BY n.nspname, p.proname`)
}

// HighRiskObjects lists procedures/functions using complex constructs.
func (s *Service) HighRiskObjects(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       p.proname AS object_name,
       CASE p.prokind WHEN 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END AS object_type,
       LENGTH(p.prosrc) AS definition_length,
       CASE
         WHEN p.prosrc ILIKE '%EXECUTE%' THEN 'Dynamic SQL'
         WHEN p.prosrc ILIKE '%CURSOR%' THEN 'Cursor usage'
         WHEN LENGTH(p.prosrc) > 5000 THEN 'Large module (>5000 chars)'
         ELSE 'Complex logic'
       END AS risk_reason
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  (p.prosrc ILIKE '%EXECUTE%' OR p.prosrc ILIKE '%CURSOR%' OR LENGTH(p.prosrc) > 5000)
  AND  n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER  BY LENGTH(p.prosrc) DESC`)
}

// MigrationComplexityScore computes a weighted complexity score.
func (s *Service) MigrationComplexityScore(ctx context.Context) (map[string]any, error) {
	overview, err := s.DatabaseOverview(ctx)
	if err != nil {
		return nil, err
	}
	highRisk, err := s.HighRiskObjects(ctx)
	if err != nil {
		return nil, err
	}

	tables := toInt64(overview["table_count"])
	views := toInt64(overview["view_count"])
	procs := toInt64(overview["procedure_count"])
	funcs := toInt64(overview["function_count"])
	triggers := toInt64(overview["trigger_count"])
	risky := int64(len(highRisk))

	score := tables*1 + views*2 + procs*5 + funcs*4 + triggers*6 + risky*10

	rating := "Low"
	switch {
	case score > 1000:
		rating = "Very High"
	case score > 500:
		rating = "High"
	case score > 200:
		rating = "Medium"
	}

	return map[string]any{
		"score":           score,
		"rating":          rating,
		"tableCount":      tables,
		"viewCount":       views,
		"procedureCount":  procs,
		"functionCount":   funcs,
		"triggerCount":    triggers,
		"highRiskObjects": risky,
	}, nil
}

// MigrationPriorityOrder proposes an ordering for migrating tables based on FKs.
func (s *Service) MigrationPriorityOrder(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.rows(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS table_name,
       (SELECT COUNT(*) FROM pg_catalog.pg_constraint fk WHERE fk.conrelid = c.oid AND fk.contype = 'f') AS outbound_fk_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_constraint fk WHERE fk.confrelid = c.oid AND fk.contype = 'f') AS inbound_fk_count
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'r'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
ORDER  BY outbound_fk_count ASC, inbound_fk_count DESC, n.nspname, c.relname`)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i]["migration_order"] = i + 1
	}
	return rows, nil
}

// AnalyzeDatabaseForMigration produces a comprehensive migration assessment.
func (s *Service) AnalyzeDatabaseForMigration(ctx context.Context) (map[string]any, error) {
	overview, err := s.DatabaseOverview(ctx)
	if err != nil {
		return nil, err
	}
	tables, err := s.ListTables(ctx, "")
	if err != nil {
		return nil, err
	}
	views, err := s.ListViews(ctx, "")
	if err != nil {
		return nil, err
	}
	procedures, err := s.ListProcedures(ctx, "")
	if err != nil {
		return nil, err
	}
	functions, err := s.ListFunctions(ctx, "")
	if err != nil {
		return nil, err
	}
	triggers, err := s.ListTriggers(ctx, "")
	if err != nil {
		return nil, err
	}
	largeTables, err := s.LargeTableAnalysis(ctx, 20)
	if err != nil {
		return nil, err
	}
	highRisk, err := s.HighRiskObjects(ctx)
	if err != nil {
		return nil, err
	}
	priority, err := s.MigrationPriorityOrder(ctx)
	if err != nil {
		return nil, err
	}
	score, err := s.MigrationComplexityScore(ctx)
	if err != nil {
		return nil, err
	}

	if len(priority) > 50 {
		priority = priority[:50]
	}

	return map[string]any{
		"overview":          overview,
		"tables":            tables,
		"views":             views,
		"procedures":        procedures,
		"functions":         functions,
		"triggers":          triggers,
		"largeTables":       largeTables,
		"highRiskObjects":   highRisk,
		"migrationPriority": priority,
		"complexityScore":   score,
	}, nil
}
