package mssql

import (
	"context"
)

// DatabaseOverview returns high-level object counts for the database.
func (s *Service) DatabaseOverview(ctx context.Context) (map[string]any, error) {
	const sql = `
SELECT
    DB_NAME()                                                          AS database_name,
    (SELECT COUNT(*) FROM sys.tables)                                  AS table_count,
    (SELECT COUNT(*) FROM sys.views)                                   AS view_count,
    (SELECT COUNT(*) FROM sys.procedures)                              AS procedure_count,
    (SELECT COUNT(*) FROM sys.objects WHERE type IN ('FN','IF','TF','FS','FT')) AS function_count,
    (SELECT COUNT(*) FROM sys.triggers WHERE is_ms_shipped = 0)        AS trigger_count`
	row, err := s.conn.QueryRow(ctx, sql)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	return row, nil
}

// DatabaseHealthReport returns assorted health indicators for the database.
func (s *Service) DatabaseHealthReport(ctx context.Context) (map[string]any, error) {
	heapTables, err := s.rows(ctx, `
SELECT s.name AS schema_name, t.name AS table_name
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE NOT EXISTS (SELECT 1 FROM sys.indexes i WHERE i.object_id = t.object_id AND i.type IN (1))
ORDER BY s.name, t.name`)
	if err != nil {
		return nil, err
	}

	noPK, err := s.rows(ctx, `
SELECT s.name AS schema_name, t.name AS table_name
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE NOT EXISTS (SELECT 1 FROM sys.key_constraints kc WHERE kc.parent_object_id = t.object_id AND kc.type = 'PK')
ORDER BY s.name, t.name`)
	if err != nil {
		return nil, err
	}

	untrustedFKs, err := s.rows(ctx, `
SELECT s.name AS schema_name, t.name AS table_name, fk.name AS constraint_name
FROM sys.foreign_keys fk
JOIN sys.tables t ON fk.parent_object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE fk.is_not_trusted = 1
ORDER BY s.name, t.name`)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"heapTables":              heapTables,
		"heapTableCount":          len(heapTables),
		"tablesWithoutPrimaryKey": noPK,
		"tablesWithoutPKCount":    len(noPK),
		"untrustedForeignKeys":    untrustedFKs,
	}, nil
}

// DatabaseComplexityReport returns metrics describing the structural complexity
// of the schema.
func (s *Service) DatabaseComplexityReport(ctx context.Context) (map[string]any, error) {
	const sql = `
SELECT
    (SELECT COUNT(*) FROM sys.tables)                                       AS table_count,
    (SELECT COUNT(*) FROM sys.foreign_keys)                                 AS foreign_key_count,
    (SELECT COUNT(*) FROM sys.triggers WHERE is_ms_shipped = 0)             AS trigger_count,
    (SELECT COUNT(*) FROM sys.procedures)                                   AS procedure_count,
    (SELECT COUNT(*) FROM sys.objects WHERE type IN ('FN','IF','TF','FS','FT')) AS function_count,
    (SELECT AVG(col_count * 1.0) FROM (SELECT COUNT(*) AS col_count FROM sys.columns GROUP BY object_id) x) AS avg_columns_per_object,
    (SELECT MAX(LEN(definition)) FROM sys.sql_modules) AS largest_module_length`
	row, err := s.conn.QueryRow(ctx, sql)
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
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sql := `
SELECT TOP (@limit)
    s.name                              AS schema_name,
    t.name                              AS table_name,
    SUM(CASE WHEN p.index_id IN (0,1) THEN p.rows ELSE 0 END) AS row_count,
    SUM(a.total_pages) * 8              AS total_space_kb
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
JOIN sys.partitions p ON p.object_id = t.object_id
JOIN sys.allocation_units a ON p.partition_id = a.container_id
GROUP BY s.name, t.name
ORDER BY SUM(a.total_pages) DESC`
	return s.rows(ctx, sql, sqlParam("@limit", limit))
}

// HighRiskObjects identifies modules that use migration-risky constructs
// (dynamic SQL, cursors, deprecated features, very large bodies).
func (s *Service) HighRiskObjects(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name              AS schema_name,
    o.name              AS object_name,
    o.type_desc         AS object_type,
    LEN(m.definition)   AS definition_length,
    CASE WHEN m.definition LIKE '%sp_executesql%' OR m.definition LIKE '%EXEC(%' OR m.definition LIKE '%EXECUTE(%' THEN 1 ELSE 0 END AS uses_dynamic_sql,
    CASE WHEN m.definition LIKE '%CURSOR%' THEN 1 ELSE 0 END AS uses_cursor,
    CASE WHEN m.definition LIKE '%xp_cmdshell%' THEN 1 ELSE 0 END AS uses_xp_cmdshell,
    CASE WHEN m.definition LIKE '%OPENQUERY%' OR m.definition LIKE '%OPENROWSET%' THEN 1 ELSE 0 END AS uses_openquery
FROM sys.sql_modules m
JOIN sys.objects o ON m.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE o.is_ms_shipped = 0
  AND (m.definition LIKE '%sp_executesql%'
       OR m.definition LIKE '%EXEC(%'
       OR m.definition LIKE '%EXECUTE(%'
       OR m.definition LIKE '%CURSOR%'
       OR m.definition LIKE '%xp_cmdshell%'
       OR m.definition LIKE '%OPENQUERY%'
       OR m.definition LIKE '%OPENROWSET%'
       OR LEN(m.definition) > 8000)
ORDER BY LEN(m.definition) DESC`
	return s.rows(ctx, sql)
}

// deprecatedFeatures are legacy/deprecated T-SQL constructs we flag.
var deprecatedFeatures = []struct {
	Name    string
	Pattern string
}{
	{"text/ntext/image data types", "%[^a-z]text[^a-z]%"},
	{"old-style outer join (*=)", "%*=%"},
	{"old-style outer join (=*)", "%=*%"},
	{"sp_dboption", "%sp_dboption%"},
	{"SETUSER", "%SETUSER%"},
	{"WRITETEXT", "%WRITETEXT%"},
	{"READTEXT", "%READTEXT%"},
	{"UPDATETEXT", "%UPDATETEXT%"},
	{"FASTFIRSTROW hint", "%FASTFIRSTROW%"},
}

// DeprecatedFeatureDetection scans module definitions for deprecated T-SQL
// features that need attention before migrating off SQL Server.
func (s *Service) DeprecatedFeatureDetection(ctx context.Context) ([]map[string]any, error) {
	var findings []map[string]any
	for _, f := range deprecatedFeatures {
		rows, err := s.rows(ctx, `
SELECT s.name AS schema_name, o.name AS object_name, o.type_desc AS object_type
FROM sys.sql_modules m
JOIN sys.objects o ON m.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE o.is_ms_shipped = 0 AND m.definition LIKE @pattern`, sqlParam("@pattern", f.Pattern))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			findings = append(findings, map[string]any{
				"feature":    f.Name,
				"schema":     r["schema_name"],
				"object":     r["object_name"],
				"objectType": r["object_type"],
			})
		}
	}
	if findings == nil {
		findings = []map[string]any{}
	}
	return findings, nil
}

// LegacyObjectAnalysis returns objects together with how long since they were
// last modified, surfacing stale/legacy code.
func (s *Service) LegacyObjectAnalysis(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name          AS schema_name,
    o.name          AS object_name,
    o.type_desc     AS object_type,
    o.create_date   AS create_date,
    o.modify_date   AS modify_date,
    DATEDIFF(DAY, o.modify_date, GETDATE()) AS days_since_modified
FROM sys.objects o
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE o.is_ms_shipped = 0
  AND o.type IN ('U','V','P','FN','IF','TF','TR')
ORDER BY o.modify_date ASC`
	return s.rows(ctx, sql)
}

// MigrationComplexityScore computes a weighted complexity score for the database
// based on object counts and risk factors.
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

	// Weighted score: procedures and triggers are the most expensive to migrate.
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

// MigrationPriorityOrder proposes an ordering for migrating tables: independent
// tables (no outbound foreign keys) first.
func (s *Service) MigrationPriorityOrder(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name      AS schema_name,
    t.name      AS table_name,
    (SELECT COUNT(*) FROM sys.foreign_keys fk WHERE fk.parent_object_id = t.object_id)      AS outbound_fk_count,
    (SELECT COUNT(*) FROM sys.foreign_keys fk WHERE fk.referenced_object_id = t.object_id)  AS inbound_fk_count
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
ORDER BY outbound_fk_count ASC, inbound_fk_count DESC, s.name, t.name`
	rows, err := s.rows(ctx, sql)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i]["migration_order"] = i + 1
	}
	return rows, nil
}

// AnalyzeDatabaseForMigration produces the comprehensive migration assessment
// described in the task specification.
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
