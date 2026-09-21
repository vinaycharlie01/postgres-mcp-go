package postgres

import (
	"context"
	"fmt"
)

func (s *Service) DescribeTable(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "public")

	columns, err := s.ListColumns(ctx, table)
	if err != nil {
		return nil, fmt.Errorf("failed to read columns: %w", err)
	}

	pkRows, err := s.rows(ctx, `
SELECT a.attname    AS column_name, ia.indkeyidx AS ordinal
FROM   pg_catalog.pg_index i
JOIN   pg_catalog.pg_class c  ON c.oid = i.indrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN   LATERAL unnest(i.indkey) WITH ORDINALITY AS ia(attnum, indkeyidx) ON true
JOIN   pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attnum = ia.attnum
WHERE  i.indisprimary AND n.nspname = $1 AND c.relname = $2
ORDER  BY ia.indkeyidx`, schema, object)
	if err != nil {
		return nil, err
	}

	fkRows, err := s.rows(ctx, `
SELECT con.conname AS constraint_name, a.attname AS column_name,
       fn.nspname  AS referenced_schema, fc.relname AS referenced_table,
       fa.attname  AS referenced_column
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class t  ON t.oid = con.conrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = t.relnamespace
JOIN   pg_catalog.pg_attribute a  ON a.attrelid = t.oid  AND a.attnum = ANY(con.conkey)
JOIN   pg_catalog.pg_class fc ON fc.oid = con.confrelid
JOIN   pg_catalog.pg_namespace fn ON fn.oid = fc.relnamespace
JOIN   pg_catalog.pg_attribute fa ON fa.attrelid = fc.oid AND fa.attnum = ANY(con.confkey)
WHERE  con.contype = 'f' AND n.nspname = $1 AND t.relname = $2
ORDER  BY con.conname`, schema, object)
	if err != nil {
		return nil, err
	}

	idxRows, err := s.rows(ctx, `
SELECT i.relname AS index_name, ix.indisunique AS is_unique, ix.indisprimary AS is_primary,
       pg_get_indexdef(ix.indexrelid) AS index_definition
FROM   pg_catalog.pg_index ix
JOIN   pg_catalog.pg_class i ON i.oid = ix.indexrelid
JOIN   pg_catalog.pg_class t ON t.oid = ix.indrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = t.relnamespace
WHERE  n.nspname = $1 AND t.relname = $2
ORDER  BY i.relname`, schema, object)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"schema": schema, "table": object,
		"columns": columns, "primaryKeys": pkRows,
		"foreignKeys": fkRows, "indexes": idxRows,
	}, nil
}

func (s *Service) ListDatabases(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT datname AS database_name,
       oid     AS database_id,
       pg_encoding_to_char(encoding) AS encoding,
       datcollate    AS collation,
       datistemplate AS is_template,
       datallowconn  AS allow_connections
FROM   pg_database
WHERE  datallowconn = true
ORDER  BY datname`)
}

func (s *Service) ListSchemas(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname                              AS schema_name,
       n.oid                                  AS schema_id,
       pg_catalog.pg_get_userbyid(n.nspowner) AS owner,
       COUNT(c.oid)                           AS object_count
FROM   pg_catalog.pg_namespace n
LEFT   JOIN pg_catalog.pg_class c
       ON   c.relnamespace = n.oid AND c.relkind IN ('r','v','m','f')
WHERE  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
  AND  n.nspname NOT LIKE 'pg_temp_%'
  AND  n.nspname NOT LIKE 'pg_toast_temp_%'
GROUP  BY n.nspname, n.oid, n.nspowner
ORDER  BY n.nspname`)
}

func (s *Service) ListTables(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname          AS schema_name,
       c.relname          AS table_name,
       c.oid              AS object_id,
       c.reltuples::bigint AS row_count,
       (SELECT COUNT(*) FROM pg_catalog.pg_attribute a
        WHERE  a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
       ) AS column_count
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'r'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, c.relname`, schema)
}

func (s *Service) ListColumns(ctx context.Context, table string) ([]map[string]any, error) {
	schema, object := splitObjectName(table, "public")
	return s.rows(ctx, `
SELECT a.attname AS column_name,
       a.attnum  AS ordinal,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
       NOT a.attnotnull    AS is_nullable,
       a.attidentity != ''  AS is_identity,
       a.attgenerated != '' AS is_computed,
       pg_get_expr(ad.adbin, ad.adrelid) AS default_definition
FROM   pg_catalog.pg_attribute a
JOIN   pg_catalog.pg_class c     ON c.oid = a.attrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
LEFT   JOIN pg_catalog.pg_attrdef ad
            ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
WHERE  n.nspname = $1 AND c.relname = $2
  AND  a.attnum > 0 AND NOT a.attisdropped
ORDER  BY a.attnum`, schema, object)
}

func (s *Service) DescribeColumn(ctx context.Context, table, column string) (map[string]any, error) {
	schema, object := splitObjectName(table, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT a.attname AS column_name,
       a.attnum  AS ordinal,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
       NOT a.attnotnull    AS is_nullable,
       a.attidentity != ''  AS is_identity,
       a.attgenerated != '' AS is_computed,
       pg_get_expr(ad.adbin, ad.adrelid) AS default_definition
FROM   pg_catalog.pg_attribute a
JOIN   pg_catalog.pg_class c     ON c.oid = a.attrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
LEFT   JOIN pg_catalog.pg_attrdef ad
            ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
WHERE  n.nspname = $1 AND c.relname = $2 AND a.attname = $3
  AND  a.attnum > 0 AND NOT a.attisdropped`, schema, object, column)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("column %s not found on %s.%s", column, schema, object)
	}
	return row, nil
}

func (s *Service) TableRowCount(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT n.nspname        AS schema_name,
       c.relname        AS table_name,
       c.reltuples::bigint AS row_count
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  n.nspname = $1 AND c.relname = $2`, schema, object)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("table %s.%s not found", schema, object)
	}
	return row, nil
}

func (s *Service) TableStorageInfo(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT n.nspname                     AS schema_name,
       c.relname                     AS table_name,
       c.reltuples::bigint           AS row_count,
       pg_total_relation_size(c.oid) AS total_bytes,
       pg_relation_size(c.oid)       AS table_bytes,
       pg_indexes_size(c.oid)        AS index_bytes
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  n.nspname = $1 AND c.relname = $2 AND c.relkind = 'r'`, schema, object)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("table %s.%s not found", schema, object)
	}
	return row, nil
}
