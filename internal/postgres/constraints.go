package postgres

import "context"

func (s *Service) ListPrimaryKeys(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname  AS schema_name,
       t.relname  AS table_name,
       con.conname AS constraint_name,
       (SELECT string_agg(a.attname, ', ' ORDER BY array_position(con.conkey, a.attnum))
        FROM   pg_catalog.pg_attribute a
        WHERE  a.attrelid = t.oid AND a.attnum = ANY(con.conkey)
       ) AS key_columns
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class t   ON t.oid = con.conrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = t.relnamespace
WHERE  con.contype = 'p'
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, t.relname`, schema)
}

func (s *Service) ListForeignKeys(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT con.conname    AS constraint_name,
       pn.nspname     AS parent_schema,
       pt.relname     AS parent_table,
       a.attname      AS parent_column,
       fn.nspname     AS referenced_schema,
       ft.relname     AS referenced_table,
       fa.attname     AS referenced_column,
       CASE con.confupdtype
         WHEN 'a' THEN 'NO ACTION' WHEN 'r' THEN 'RESTRICT'
         WHEN 'c' THEN 'CASCADE'   WHEN 'n' THEN 'SET NULL'
         WHEN 'd' THEN 'SET DEFAULT' END AS on_update,
       CASE con.confdeltype
         WHEN 'a' THEN 'NO ACTION' WHEN 'r' THEN 'RESTRICT'
         WHEN 'c' THEN 'CASCADE'   WHEN 'n' THEN 'SET NULL'
         WHEN 'd' THEN 'SET DEFAULT' END AS on_delete
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class pt  ON pt.oid = con.conrelid
JOIN   pg_catalog.pg_namespace pn ON pn.oid = pt.relnamespace
JOIN   pg_catalog.pg_attribute a  ON a.attrelid = pt.oid  AND a.attnum = ANY(con.conkey)
JOIN   pg_catalog.pg_class ft  ON ft.oid = con.confrelid
JOIN   pg_catalog.pg_namespace fn ON fn.oid = ft.relnamespace
JOIN   pg_catalog.pg_attribute fa ON fa.attrelid = ft.oid AND fa.attnum = ANY(con.confkey)
WHERE  con.contype = 'f'
  AND  ($1 = '' OR pn.nspname = $1)
ORDER  BY pn.nspname, pt.relname, con.conname`, schema)
}

func (s *Service) ListUniqueConstraints(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname   AS schema_name,
       t.relname   AS table_name,
       con.conname  AS constraint_name,
       (SELECT string_agg(a.attname, ', ' ORDER BY array_position(con.conkey, a.attnum))
        FROM   pg_catalog.pg_attribute a
        WHERE  a.attrelid = t.oid AND a.attnum = ANY(con.conkey)
       ) AS key_columns
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class t   ON t.oid = con.conrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = t.relnamespace
WHERE  con.contype = 'u'
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, t.relname`, schema)
}

func (s *Service) ListCheckConstraints(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname   AS schema_name,
       t.relname   AS table_name,
       con.conname  AS constraint_name,
       pg_get_constraintdef(con.oid) AS definition,
       NOT con.convalidated AS is_not_validated
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class t   ON t.oid = con.conrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = t.relnamespace
WHERE  con.contype = 'c'
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, t.relname, con.conname`, schema)
}

func (s *Service) ListDefaultConstraints(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS table_name,
       a.attname AS column_name,
       pg_get_expr(ad.adbin, ad.adrelid) AS default_definition
FROM   pg_catalog.pg_attrdef ad
JOIN   pg_catalog.pg_attribute a ON a.attrelid = ad.adrelid AND a.attnum = ad.adnum
JOIN   pg_catalog.pg_class c ON c.oid = ad.adrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  a.attnum > 0 AND NOT a.attisdropped
  AND  c.relkind = 'r'
  AND  n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, c.relname, a.attnum`, schema)
}
