package mssql

import (
	"context"
	"sort"
	"strings"
)

// TableRelationships returns the inbound and outbound foreign key relationships
// for a single table.
func (s *Service) TableRelationships(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")

	outbound, err := s.rows(ctx, `
SELECT
    fk.name             AS constraint_name,
    cpar.name           AS parent_column,
    rs.name             AS referenced_schema,
    rt.name             AS referenced_table,
    cref.name           AS referenced_column
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
JOIN sys.tables pt ON fk.parent_object_id = pt.object_id
JOIN sys.schemas ps ON pt.schema_id = ps.schema_id
JOIN sys.columns cpar ON fkc.parent_object_id = cpar.object_id AND fkc.parent_column_id = cpar.column_id
JOIN sys.tables rt ON fk.referenced_object_id = rt.object_id
JOIN sys.schemas rs ON rt.schema_id = rs.schema_id
JOIN sys.columns cref ON fkc.referenced_object_id = cref.object_id AND fkc.referenced_column_id = cref.column_id
WHERE ps.name = @schema AND pt.name = @table
ORDER BY fk.name`, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}

	inbound, err := s.rows(ctx, `
SELECT
    fk.name             AS constraint_name,
    ps.name             AS parent_schema,
    pt.name             AS parent_table,
    cpar.name           AS parent_column,
    cref.name           AS referenced_column
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
JOIN sys.tables pt ON fk.parent_object_id = pt.object_id
JOIN sys.schemas ps ON pt.schema_id = ps.schema_id
JOIN sys.columns cpar ON fkc.parent_object_id = cpar.object_id AND fkc.parent_column_id = cpar.column_id
JOIN sys.tables rt ON fk.referenced_object_id = rt.object_id
JOIN sys.schemas rs ON rt.schema_id = rs.schema_id
JOIN sys.columns cref ON fkc.referenced_object_id = cref.object_id AND fkc.referenced_column_id = cref.column_id
WHERE rs.name = @schema AND rt.name = @table
ORDER BY fk.name`, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"schema":   schema,
		"table":    object,
		"references": outbound,
		"referencedBy": inbound,
	}, nil
}

// fkEdge is a single foreign-key edge between two tables.
type fkEdge struct {
	From string
	To   string
}

// allForeignKeyEdges returns every table->referenced-table edge in the database.
func (s *Service) allForeignKeyEdges(ctx context.Context) ([]fkEdge, error) {
	rows, err := s.rows(ctx, `
SELECT DISTINCT
    ps.name + '.' + pt.name AS from_table,
    rs.name + '.' + rt.name AS to_table
FROM sys.foreign_keys fk
JOIN sys.tables pt ON fk.parent_object_id = pt.object_id
JOIN sys.schemas ps ON pt.schema_id = ps.schema_id
JOIN sys.tables rt ON fk.referenced_object_id = rt.object_id
JOIN sys.schemas rs ON rt.schema_id = rs.schema_id`)
	if err != nil {
		return nil, err
	}
	edges := make([]fkEdge, 0, len(rows))
	for _, r := range rows {
		from, _ := r["from_table"].(string)
		to, _ := r["to_table"].(string)
		edges = append(edges, fkEdge{From: from, To: to})
	}
	return edges, nil
}

// ForeignKeyGraph returns the complete foreign-key graph of the database as a
// list of edges.
func (s *Service) ForeignKeyGraph(ctx context.Context) (map[string]any, error) {
	edges, err := s.allForeignKeyEdges(ctx)
	if err != nil {
		return nil, err
	}
	edgeList := make([]map[string]any, 0, len(edges))
	nodes := map[string]bool{}
	for _, e := range edges {
		edgeList = append(edgeList, map[string]any{"from": e.From, "to": e.To})
		nodes[e.From] = true
		nodes[e.To] = true
	}
	return map[string]any{
		"nodes": uniqueSortedBool(nodes),
		"edges": edgeList,
	}, nil
}

// GenerateERD builds an entity-relationship description rooted at a table,
// traversing foreign keys outward up to the given depth.
func (s *Service) GenerateERD(ctx context.Context, table string, depth int) (map[string]any, error) {
	if depth <= 0 {
		depth = 2
	}
	schema, object := splitObjectName(table, "dbo")
	root := schema + "." + object

	edges, err := s.allForeignKeyEdges(ctx)
	if err != nil {
		return nil, err
	}
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}

	included := map[string]bool{root: true}
	frontier := []string{root}
	for d := 0; d < depth; d++ {
		var next []string
		for _, node := range frontier {
			for _, nb := range adj[node] {
				if !included[nb] {
					included[nb] = true
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}

	// Collect edges whose both endpoints are included.
	var erdEdges []map[string]any
	for _, e := range edges {
		if included[e.From] && included[e.To] {
			erdEdges = append(erdEdges, map[string]any{"from": e.From, "to": e.To})
		}
	}

	// Include the columns of each entity so the ERD is usable.
	entities := make([]map[string]any, 0, len(included))
	for _, t := range uniqueSortedBool(included) {
		cols, err := s.ListColumns(ctx, t)
		if err != nil {
			return nil, err
		}
		entities = append(entities, map[string]any{"table": t, "columns": cols})
	}

	return map[string]any{
		"root":     root,
		"depth":    depth,
		"entities": entities,
		"edges":    erdEdges,
	}, nil
}

// DependencyGraph builds a recursive dependency graph for a database object
// using sys.sql_expression_dependencies.
func (s *Service) DependencyGraph(ctx context.Context, object string) (map[string]any, error) {
	schema, name := splitObjectName(object, "dbo")
	visited := map[string]bool{}
	var edges []map[string]any

	var walk func(sch, obj string, depth int) error
	walk = func(sch, obj string, depth int) error {
		key := strings.ToLower(sch + "." + obj)
		if depth > 10 || visited[key] {
			return nil
		}
		visited[key] = true

		deps, err := s.objectDependencies(ctx, sch, obj)
		if err != nil {
			return nil
		}
		for _, d := range deps {
			refSchema, _ := d["referenced_schema"].(string)
			refName, _ := d["referenced_entity"].(string)
			if refName == "" {
				continue
			}
			edges = append(edges, map[string]any{
				"from": sch + "." + obj,
				"to":   refSchema + "." + refName,
				"type": d["referenced_type"],
			})
			if err := walk(refSchema, refName, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(schema, name, 0); err != nil {
		return nil, err
	}
	if edges == nil {
		edges = []map[string]any{}
	}
	return map[string]any{
		"object": schema + "." + name,
		"edges":  edges,
	}, nil
}

// FindObjectDependencies returns the objects the given object depends on.
func (s *Service) FindObjectDependencies(ctx context.Context, object string) (map[string]any, error) {
	schema, name := splitObjectName(object, "dbo")
	deps, err := s.objectDependencies(ctx, schema, name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"object":       schema + "." + name,
		"dependencies": deps,
	}, nil
}

// FindObjectReferences returns the objects that reference the given object.
func (s *Service) FindObjectReferences(ctx context.Context, object string) (map[string]any, error) {
	_, name := splitObjectName(object, "dbo")
	refs, err := s.objectReferencers(ctx, name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"object":       name,
		"referencedBy": refs,
	}, nil
}

// objectReferencers returns the modules that reference a named entity.
func (s *Service) objectReferencers(ctx context.Context, entityName string) ([]map[string]any, error) {
	const sql = `
SELECT DISTINCT
    s.name          AS schema_name,
    o.name          AS object_name,
    o.type_desc     AS object_type
FROM sys.sql_expression_dependencies d
JOIN sys.objects o ON d.referencing_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE d.referenced_entity_name = @entity
ORDER BY o.type_desc, s.name, o.name`
	return s.rows(ctx, sql, sqlParam("@entity", entityName))
}

// FindTableUsage returns every procedure, view, function and trigger that
// references the table.
func (s *Service) FindTableUsage(ctx context.Context, table string) (map[string]any, error) {
	_, object := splitObjectName(table, "dbo")
	refs, err := s.objectReferencers(ctx, object)
	if err != nil {
		return nil, err
	}

	procedures := []map[string]any{}
	views := []map[string]any{}
	functions := []map[string]any{}
	triggers := []map[string]any{}
	for _, r := range refs {
		typ, _ := r["object_type"].(string)
		entry := map[string]any{"schema": r["schema_name"], "name": r["object_name"]}
		switch {
		case strings.Contains(typ, "PROCEDURE"):
			procedures = append(procedures, entry)
		case strings.Contains(typ, "VIEW"):
			views = append(views, entry)
		case strings.Contains(typ, "FUNCTION"):
			functions = append(functions, entry)
		case strings.Contains(typ, "TRIGGER"):
			triggers = append(triggers, entry)
		}
	}

	return map[string]any{
		"table":      object,
		"procedures": procedures,
		"views":      views,
		"functions":  functions,
		"triggers":   triggers,
	}, nil
}

// FindColumnUsage returns the modules that reference a specific column.
func (s *Service) FindColumnUsage(ctx context.Context, table, column string) (map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")
	const sql = `
SELECT DISTINCT
    s.name          AS schema_name,
    o.name          AS object_name,
    o.type_desc     AS object_type
FROM sys.sql_expression_dependencies d
JOIN sys.objects o ON d.referencing_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
WHERE d.referenced_entity_name = @table
  AND d.referenced_minor_id = (
        SELECT c.column_id FROM sys.columns c
        JOIN sys.objects t ON c.object_id = t.object_id
        JOIN sys.schemas ts ON t.schema_id = ts.schema_id
        WHERE ts.name = @schema AND t.name = @table AND c.name = @column)
ORDER BY o.type_desc, s.name, o.name`
	rows, err := s.rows(ctx, sql, sqlParam("@schema", schema), sqlParam("@table", object), sqlParam("@column", column))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"table":        object,
		"column":       column,
		"referencedBy": rows,
	}, nil
}

// FindUnusedTables returns tables that are not referenced by any module and have
// no inbound foreign keys (candidate dead tables).
func (s *Service) FindUnusedTables(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name AS schema_name,
    t.name AS table_name,
    p.rows AS row_count
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
LEFT JOIN sys.partitions p ON p.object_id = t.object_id AND p.index_id IN (0,1)
WHERE NOT EXISTS (
        SELECT 1 FROM sys.sql_expression_dependencies d
        WHERE d.referenced_entity_name = t.name)
  AND NOT EXISTS (
        SELECT 1 FROM sys.foreign_keys fk
        WHERE fk.referenced_object_id = t.object_id)
ORDER BY s.name, t.name`
	return s.rows(ctx, sql)
}

// FindUnusedColumns returns columns not referenced by any module dependency.
// This is heuristic: dynamic SQL and application code are not visible to the
// engine's dependency tracking.
func (s *Service) FindUnusedColumns(ctx context.Context, table string) ([]map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")
	const sql = `
SELECT
    c.name AS column_name,
    ty.name AS data_type
FROM sys.columns c
JOIN sys.objects o ON c.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
JOIN sys.types ty ON c.user_type_id = ty.user_type_id
WHERE s.name = @schema AND o.name = @table
  AND NOT EXISTS (
        SELECT 1 FROM sys.sql_expression_dependencies d
        WHERE d.referenced_entity_name = o.name
          AND d.referenced_minor_id = c.column_id)
  AND c.is_identity = 0
ORDER BY c.column_id`
	return s.rows(ctx, sql, sqlParam("@schema", schema), sqlParam("@table", object))
}

// FindOrphanTables returns tables that have no foreign keys at all (neither
// inbound nor outbound), i.e. tables not connected to the relational graph.
func (s *Service) FindOrphanTables(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name AS schema_name,
    t.name AS table_name,
    p.rows AS row_count
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
LEFT JOIN sys.partitions p ON p.object_id = t.object_id AND p.index_id IN (0,1)
WHERE NOT EXISTS (SELECT 1 FROM sys.foreign_keys fk WHERE fk.parent_object_id = t.object_id)
  AND NOT EXISTS (SELECT 1 FROM sys.foreign_keys fk WHERE fk.referenced_object_id = t.object_id)
ORDER BY s.name, t.name`
	return s.rows(ctx, sql)
}

// FindCircularDependencies detects cycles in the foreign-key graph.
func (s *Service) FindCircularDependencies(ctx context.Context) (map[string]any, error) {
	edges, err := s.allForeignKeyEdges(ctx)
	if err != nil {
		return nil, err
	}
	adj := map[string][]string{}
	for _, e := range edges {
		if e.From == e.To {
			// self reference is a trivial cycle handled separately below
		}
		adj[e.From] = append(adj[e.From], e.To)
	}

	cycles := [][]string{}
	color := map[string]int{} // 0 unvisited, 1 in-stack, 2 done
	var stack []string

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = 1
		stack = append(stack, node)
		for _, nb := range adj[node] {
			if color[nb] == 1 {
				// found a back edge; capture the cycle from nb in the stack
				cycle := captureCycle(stack, nb)
				if len(cycle) > 0 {
					cycles = append(cycles, cycle)
				}
			} else if color[nb] == 0 {
				dfs(nb)
			}
		}
		stack = stack[:len(stack)-1]
		color[node] = 2
	}

	nodes := map[string]bool{}
	for _, e := range edges {
		nodes[e.From] = true
		nodes[e.To] = true
	}
	for _, n := range uniqueSortedBool(nodes) {
		if color[n] == 0 {
			dfs(n)
		}
	}

	return map[string]any{
		"cycles":     dedupeCycles(cycles),
		"cycleCount": len(dedupeCycles(cycles)),
	}, nil
}

func captureCycle(stack []string, start string) []string {
	for i, n := range stack {
		if n == start {
			cyc := append([]string{}, stack[i:]...)
			cyc = append(cyc, start)
			return cyc
		}
	}
	return nil
}

func dedupeCycles(cycles [][]string) [][]string {
	seen := map[string]bool{}
	var out [][]string
	for _, c := range cycles {
		key := strings.Join(c, "->")
		if !seen[key] {
			seen[key] = true
			out = append(out, c)
		}
	}
	if out == nil {
		out = [][]string{}
	}
	return out
}

func uniqueSortedBool(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
