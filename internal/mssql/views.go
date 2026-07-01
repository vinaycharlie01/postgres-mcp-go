package mssql

import (
	"context"
	"fmt"
)

// ListViews returns all views, optionally filtered by schema.
func (s *Service) ListViews(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    v.name          AS view_name,
    v.object_id     AS object_id,
    v.create_date   AS create_date,
    v.modify_date   AS modify_date
FROM sys.views v
JOIN sys.schemas sch ON v.schema_id = sch.schema_id
WHERE (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, v.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// objectDefinition returns the T-SQL definition text for a module
// (view/procedure/function/trigger) identified by name.
func (s *Service) objectDefinition(ctx context.Context, name string) (string, error) {
	schema, object := splitObjectName(name, "dbo")
	const sql = `
SELECT m.definition AS definition
FROM sys.sql_modules m
JOIN sys.objects o ON m.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE s.name = @schema AND o.name = @object`
	def, err := s.scalar(ctx, sql, sqlParam("@schema", schema), sqlParam("@object", object))
	if err != nil {
		return "", err
	}
	if def == nil {
		return "", fmt.Errorf("definition for %s.%s not found (object may be encrypted or non-existent)", schema, object)
	}
	str, _ := def.(string)
	return str, nil
}

// GetViewDefinition returns the SQL definition of a view.
func (s *Service) GetViewDefinition(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "dbo")
	def, err := s.objectDefinition(ctx, view)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":     schema,
		"view":       object,
		"definition": def,
	}, nil
}

// AnalyzeView performs static analysis of a view definition: source
// tables/views, columns exposed and dependencies.
func (s *Service) AnalyzeView(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "dbo")
	def, err := s.objectDefinition(ctx, view)
	if err != nil {
		return nil, err
	}
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	columns, err := s.ListColumns(ctx, view)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":        schema,
		"view":          object,
		"columns":       columns,
		"tablesUsed":    analysis["tablesUsed"],
		"viewsUsed":     analysis["viewsUsed"],
		"functionsUsed": analysis["functionsUsed"],
	}, nil
}

// TraceViewSources resolves the underlying base tables of a view, following
// nested views recursively (up to a bounded depth).
func (s *Service) TraceViewSources(ctx context.Context, view string) (map[string]any, error) {
	idx, err := s.buildCatalogIndex(ctx)
	if err != nil {
		return nil, err
	}

	baseTables := map[string]bool{}
	visited := map[string]bool{}

	var trace func(objName string, depth int) error
	trace = func(objName string, depth int) error {
		if depth > 10 || visited[objName] {
			return nil
		}
		visited[objName] = true

		def, err := s.objectDefinition(ctx, objName)
		if err != nil {
			// Non-view or missing: stop this branch.
			return nil
		}
		clean := cleanDefinition(def)
		for _, m := range reFromJoin.FindAllStringSubmatch(clean, -1) {
			bare, full := normalizeRef(m[1])
			if q, ok := resolve(idx.views, full, bare); ok {
				if err := trace(q, depth+1); err != nil {
					return err
				}
			} else if q, ok := resolve(idx.tables, full, bare); ok {
				baseTables[q] = true
			}
		}
		return nil
	}

	if err := trace(view, 0); err != nil {
		return nil, err
	}

	schema, object := splitObjectName(view, "dbo")
	return map[string]any{
		"schema":     schema,
		"view":       object,
		"baseTables": uniqueSorted(baseTables),
	}, nil
}

// ExtractViewDependencies lists all objects the view depends on, using the
// engine's own dependency catalog (sys.sql_expression_dependencies).
func (s *Service) ExtractViewDependencies(ctx context.Context, view string) (map[string]any, error) {
	schema, object := splitObjectName(view, "dbo")
	deps, err := s.objectDependencies(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":       schema,
		"view":         object,
		"dependencies": deps,
	}, nil
}

// resolve looks up a name in a catalog map trying the fully-qualified form first
// then the bare name.
func resolve(m map[string]string, full, bare string) (string, bool) {
	if q, ok := m[full]; ok {
		return q, true
	}
	if q, ok := m[bare]; ok {
		return q, true
	}
	return "", false
}

// objectDependencies returns the referenced entities for an object using
// sys.sql_expression_dependencies.
func (s *Service) objectDependencies(ctx context.Context, schema, object string) ([]map[string]any, error) {
	const sql = `
SELECT DISTINCT
    ISNULL(d.referenced_schema_name, 'dbo')  AS referenced_schema,
    d.referenced_entity_name                 AS referenced_entity,
    o.type_desc                              AS referenced_type
FROM sys.sql_expression_dependencies d
JOIN sys.objects src ON d.referencing_id = src.object_id
JOIN sys.schemas s ON src.schema_id = s.schema_id
LEFT JOIN sys.objects o ON o.name = d.referenced_entity_name
WHERE s.name = @schema AND src.name = @object
ORDER BY referenced_schema, referenced_entity`
	return s.rows(ctx, sql, sqlParam("@schema", schema), sqlParam("@object", object))
}
