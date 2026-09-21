package postgres

import "context"

func (s *Service) SearchTables(ctx context.Context, keyword string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name, c.oid AS object_id
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'r' AND c.relname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
ORDER  BY n.nspname, c.relname`, keyword)
}

func (s *Service) SearchColumns(ctx context.Context, keyword string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS table_name,
       a.attname AS column_name,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type
FROM   pg_catalog.pg_attribute a
JOIN   pg_catalog.pg_class c     ON c.oid = a.attrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind IN ('r','v') AND a.attnum > 0 AND NOT a.attisdropped
  AND  a.attname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
ORDER  BY n.nspname, c.relname, a.attname`, keyword)
}

func (s *Service) SearchViews(ctx context.Context, keyword string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS view_name
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'v' AND c.relname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
ORDER  BY n.nspname, c.relname`, keyword)
}

func (s *Service) SearchFunctions(ctx context.Context, keyword string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name, p.proname AS function_name,
       CASE p.prokind WHEN 'f' THEN 'FUNCTION' WHEN 'p' THEN 'PROCEDURE'
                      WHEN 'a' THEN 'AGGREGATE' ELSE 'WINDOW' END AS function_type
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.proname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema')
ORDER  BY n.nspname, p.proname`, keyword)
}

func (s *Service) SearchObjectsByKeyword(ctx context.Context, keyword string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name, c.relname AS object_name,
       CASE c.relkind WHEN 'r' THEN 'TABLE' WHEN 'v' THEN 'VIEW'
                      WHEN 'm' THEN 'MATERIALIZED VIEW' ELSE c.relkind::text END AS object_type,
       'name' AS matched_on
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
UNION
SELECT n.nspname, p.proname, 'FUNCTION', 'name'
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.proname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema')
ORDER  BY object_type, schema_name, object_name`, keyword)
}

func (s *Service) SearchProcedures(ctx context.Context, keyword string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name, p.proname AS procedure_name
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.prokind = 'p' AND p.proname ILIKE '%' || $1 || '%'
  AND  n.nspname NOT IN ('pg_catalog','information_schema')
ORDER  BY n.nspname, p.proname`, keyword)
}
