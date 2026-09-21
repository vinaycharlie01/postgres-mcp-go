package postgres

import (
	"context"
	"fmt"
)

// ListViews returns all views, optionally filtered by schema.
func (s *Service) ListViews(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS view_name,
       c.oid     AS object_id
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'v'
  AND  n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, c.relname`, schema)
}

// GetViewDefinition returns the SQL definition of a view.
func (s *Service) GetViewDefinition(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT n.nspname AS schema_name,
       c.relname AS view_name,
       pg_get_viewdef(c.oid, true) AS definition
FROM   pg_catalog.pg_class c
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  c.relkind = 'v' AND n.nspname = $1 AND c.relname = $2`, schema, object)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("view %s.%s not found", schema, object)
	}
	return row, nil
}

// AnalyzeView returns columns exposed by the view plus its definition.
func (s *Service) AnalyzeView(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "public")
	def, err := s.GetViewDefinition(ctx, view)
	if err != nil {
		return nil, err
	}
	cols, err := s.ListColumns(ctx, view)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":     schema,
		"view":       object,
		"definition": def["definition"],
		"columns":    cols,
	}, nil
}

// TraceViewSources resolves the base tables a view depends on.
func (s *Service) TraceViewSources(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "public")
	rows, err := s.rows(ctx, `
SELECT DISTINCT rn.nspname AS schema_name, rc.relname AS table_name
FROM   pg_catalog.pg_depend d
JOIN   pg_catalog.pg_rewrite rw ON rw.oid = d.objid
JOIN   pg_catalog.pg_class c    ON c.oid = rw.ev_class
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN   pg_catalog.pg_class rc   ON rc.oid = d.refobjid AND rc.relkind = 'r'
JOIN   pg_catalog.pg_namespace rn ON rn.oid = rc.relnamespace
WHERE  d.classid = 'pg_rewrite'::regclass
  AND  d.deptype = 'n'
  AND  n.nspname = $1 AND c.relname = $2
ORDER  BY rn.nspname, rc.relname`, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "view": object, "baseTables": rows}, nil
}

// ExtractViewDependencies lists the objects a view depends on.
func (s *Service) ExtractViewDependencies(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "public")
	deps, err := s.objectDependencies(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "view": object, "dependencies": deps}, nil
}

// objectDependencies returns what an object depends on via pg_depend.
func (s *Service) objectDependencies(ctx context.Context, schema, object string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT DISTINCT rn.nspname AS referenced_schema,
               rc.relname  AS referenced_entity,
               rc.relkind  AS referenced_type
FROM   pg_catalog.pg_depend d
JOIN   pg_catalog.pg_class c   ON c.oid = d.objid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN   pg_catalog.pg_class rc  ON rc.oid = d.refobjid
JOIN   pg_catalog.pg_namespace rn ON rn.oid = rc.relnamespace
WHERE  d.deptype = 'n'
  AND  n.nspname = $1 AND c.relname = $2
  AND  rn.nspname NOT IN ('pg_catalog','information_schema')
ORDER  BY rn.nspname, rc.relname`, schema, object)
}

// objectReferencers finds objects that depend on the given object.
func (s *Service) objectReferencers(ctx context.Context, schema, object string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT DISTINCT sn.nspname AS schema_name,
               sc.relname  AS object_name,
               sc.relkind  AS object_type
FROM   pg_catalog.pg_depend d
JOIN   pg_catalog.pg_class rc  ON rc.oid = d.refobjid
JOIN   pg_catalog.pg_namespace rn ON rn.oid = rc.relnamespace
JOIN   pg_catalog.pg_class sc  ON sc.oid = d.objid
JOIN   pg_catalog.pg_namespace sn ON sn.oid = sc.relnamespace
WHERE  d.deptype = 'n'
  AND  rn.nspname = $1 AND rc.relname = $2
  AND  sn.nspname NOT IN ('pg_catalog','information_schema')
ORDER  BY sn.nspname, sc.relname`, schema, object)
}
