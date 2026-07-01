package mssql

import "context"

// SearchTables finds tables whose name matches the keyword.
func (s *Service) SearchTables(ctx context.Context, keyword string) ([]map[string]any, error) {
	const sql = `
SELECT s.name AS schema_name, t.name AS table_name, t.object_id AS object_id
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE t.name LIKE '%' + @keyword + '%'
ORDER BY s.name, t.name`
	return s.rows(ctx, sql, sqlParam("@keyword", keyword))
}

// SearchColumns finds columns whose name matches the keyword, across all tables.
func (s *Service) SearchColumns(ctx context.Context, keyword string) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name  AS schema_name,
    o.name  AS table_name,
    c.name  AS column_name,
    ty.name AS data_type
FROM sys.columns c
JOIN sys.objects o ON c.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
JOIN sys.types ty ON c.user_type_id = ty.user_type_id
WHERE o.type IN ('U','V') AND c.name LIKE '%' + @keyword + '%'
ORDER BY s.name, o.name, c.name`
	return s.rows(ctx, sql, sqlParam("@keyword", keyword))
}

// SearchViews finds views whose name matches the keyword.
func (s *Service) SearchViews(ctx context.Context, keyword string) ([]map[string]any, error) {
	const sql = `
SELECT s.name AS schema_name, v.name AS view_name
FROM sys.views v
JOIN sys.schemas s ON v.schema_id = s.schema_id
WHERE v.name LIKE '%' + @keyword + '%'
ORDER BY s.name, v.name`
	return s.rows(ctx, sql, sqlParam("@keyword", keyword))
}

// SearchFunctions finds functions whose name matches the keyword.
func (s *Service) SearchFunctions(ctx context.Context, keyword string) ([]map[string]any, error) {
	const sql = `
SELECT s.name AS schema_name, o.name AS function_name, o.type_desc AS function_type
FROM sys.objects o
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE o.type IN ('FN','IF','TF','FS','FT') AND o.name LIKE '%' + @keyword + '%'
ORDER BY s.name, o.name`
	return s.rows(ctx, sql, sqlParam("@keyword", keyword))
}

// SearchObjectsByKeyword searches every object name and module definition for
// the keyword, returning a unified result set.
func (s *Service) SearchObjectsByKeyword(ctx context.Context, keyword string) ([]map[string]any, error) {
	const sql = `
SELECT s.name AS schema_name, o.name AS object_name, o.type_desc AS object_type, 'name' AS matched_on
FROM sys.objects o
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE o.is_ms_shipped = 0 AND o.name LIKE '%' + @keyword + '%'
UNION
SELECT s.name, o.name, o.type_desc, 'definition'
FROM sys.sql_modules m
JOIN sys.objects o ON m.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE o.is_ms_shipped = 0 AND m.definition LIKE '%' + @keyword + '%'
ORDER BY object_type, schema_name, object_name`
	return s.rows(ctx, sql, sqlParam("@keyword", keyword))
}
