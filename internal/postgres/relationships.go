package postgres

import (
	"context"
	"sort"
	"strings"
)

// TableRelationships returns the inbound and outbound foreign key relationships
// for a single table.
func (s *Service) TableRelationships(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "public")

	outbound, err := s.rows(ctx, `
SELECT con.conname AS constraint_name,
       a.attname   AS parent_column,
       fn.nspname  AS referenced_schema,
       fc.relname  AS referenced_table,
       fa.attname  AS referenced_column
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class pt  ON pt.oid = con.conrelid
JOIN   pg_catalog.pg_namespace ps ON ps.oid = pt.relnamespace
JOIN   pg_catalog.pg_attribute a  ON a.attrelid = pt.oid  AND a.attnum = ANY(con.conkey)
JOIN   pg_catalog.pg_class fc  ON fc.oid = con.confrelid
JOIN   pg_catalog.pg_namespace fn ON fn.oid = fc.relnamespace
JOIN   pg_catalog.pg_attribute fa ON fa.attrelid = fc.oid AND fa.attnum = ANY(con.confkey)
WHERE  con.contype = 'f' AND ps.nspname = $1 AND pt.relname = $2
ORDER  BY con.conname`, schema, object)
	if err != nil {
		return nil, err
	}

	inbound, err := s.rows(ctx, `
SELECT con.conname AS constraint_name,
       ps.nspname  AS parent_schema,
       pt.relname  AS parent_table,
       a.attname   AS parent_column,
       fa.attname  AS referenced_column
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class pt  ON pt.oid = con.conrelid
JOIN   pg_catalog.pg_namespace ps ON ps.oid = pt.relnamespace
JOIN   pg_catalog.pg_attribute a  ON a.attrelid = pt.oid  AND a.attnum = ANY(con.conkey)
JOIN   pg_catalog.pg_class fc  ON fc.oid = con.confrelid
JOIN   pg_catalog.pg_namespace fn ON fn.oid = fc.relnamespace
JOIN   pg_catalog.pg_attribute fa ON fa.attrelid = fc.oid AND fa.attnum = ANY(con.confkey)
WHERE  con.contype = 'f' AND fn.nspname = $1 AND fc.relname = $2
ORDER  BY con.conname`, schema, object)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"schema":       schema,
		"table":        object,
		"references":   outbound,
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
       ps.nspname || '.' || pt.relname AS from_table,
       fn.nspname || '.' || fc.relname AS to_table
FROM   pg_catalog.pg_constraint con
JOIN   pg_catalog.pg_class pt  ON pt.oid = con.conrelid
JOIN   pg_catalog.pg_namespace ps ON ps.oid = pt.relnamespace
JOIN   pg_catalog.pg_class fc  ON fc.oid = con.confrelid
JOIN   pg_catalog.pg_namespace fn ON fn.oid = fc.relnamespace
WHERE  con.contype = 'f'
  AND  ps.nspname NOT IN ('pg_catalog', 'information_schema')
  AND  fn.nspname NOT IN ('pg_catalog', 'information_schema')`)
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
	edgeList := make([]map[string]string, 0, len(edges))
	tableSet := map[string]bool{}
	for _, e := range edges {
		edgeList = append(edgeList, map[string]string{
			"from": e.From,
			"to":   e.To,
		})
		tableSet[e.From] = true
		tableSet[e.To] = true
	}
	tables := uniqueSortedBool(tableSet)
	return map[string]any{
		"tables":     tables,
		"edges":      edgeList,
		"tableCount": len(tables),
		"edgeCount":  len(edgeList),
	}, nil
}

// GenerateERD builds a schema ERD description centered on table, traversing up
// to depth hops away through foreign keys.
func (s *Service) GenerateERD(ctx context.Context, table string, depth int) (map[string]any, error) {
	if depth <= 0 {
		depth = 2
	}
	schema, object := splitObjectName(table, "public")
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

	visited := map[string]bool{root: true}
	frontier := []string{root}
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, node := range frontier {
			for _, neighbor := range adj[node] {
				if !visited[neighbor] {
					visited[neighbor] = true
					next = append(next, neighbor)
				}
			}
		}
		frontier = next
	}

	var relevantEdges []map[string]string
	for _, e := range edges {
		if visited[e.From] && visited[e.To] {
			relevantEdges = append(relevantEdges, map[string]string{
				"from": e.From,
				"to":   e.To,
			})
		}
	}
	if relevantEdges == nil {
		relevantEdges = []map[string]string{}
	}

	entities := make([]map[string]any, 0, len(visited))
	for _, t := range uniqueSortedBool(visited) {
		cols, err := s.ListColumns(ctx, t)
		if err != nil {
			return nil, err
		}
		entities = append(entities, map[string]any{"table": t, "columns": cols})
	}

	return map[string]any{
		"rootTable":     root,
		"depth":         depth,
		"tables":        uniqueSortedBool(visited),
		"entities":      entities,
		"relationships": relevantEdges,
	}, nil
}

// FindUnusedTables returns tables that have no inbound foreign keys and no
// tracked module references (candidate dead tables).
func (s *Service) FindUnusedTables(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT s.nspname AS schema_name, t.relname AS table_name
FROM   pg_catalog.pg_class t
JOIN   pg_catalog.pg_namespace s ON t.relnamespace = s.oid
WHERE  t.relkind = 'r'
  AND  s.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND  NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_constraint fk
      WHERE fk.confrelid = t.oid AND fk.contype = 'f'
  )
  AND  NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_depend d
      WHERE d.refobjid = t.oid AND d.deptype = 'n'
  )
ORDER  BY s.nspname, t.relname`)
}

// FindUnusedColumns returns non-identity columns of a table with no detected
// tracked dependencies.
func (s *Service) FindUnusedColumns(ctx context.Context, table string) ([]map[string]any, error) {
	schema, object := splitObjectName(table, "public")
	return s.rows(ctx, `
SELECT a.attname AS column_name,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type
FROM   pg_catalog.pg_attribute a
JOIN   pg_catalog.pg_class c ON a.attrelid = c.oid
JOIN   pg_catalog.pg_namespace n ON c.relnamespace = n.oid
WHERE  n.nspname = $1 AND c.relname = $2
  AND  a.attnum > 0 AND NOT a.attisdropped
  AND  a.attidentity = ''
  AND  NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_depend d
      WHERE d.refobjid = c.oid AND d.refobjsubid = a.attnum AND d.deptype = 'n'
  )
ORDER  BY a.attnum`, schema, object)
}

// FindOrphanTables returns tables that have no foreign keys at all (neither
// inbound nor outbound), i.e. tables not connected to the relational graph.
func (s *Service) FindOrphanTables(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT s.nspname AS schema_name,
       t.relname AS table_name,
       t.reltuples::bigint AS row_count
FROM   pg_catalog.pg_class t
JOIN   pg_catalog.pg_namespace s ON t.relnamespace = s.oid
WHERE  t.relkind = 'r'
  AND  s.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND  NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_constraint fk
      WHERE (fk.conrelid = t.oid OR fk.confrelid = t.oid) AND fk.contype = 'f'
  )
ORDER  BY s.nspname, t.relname`)
}

// FindCircularDependencies detects cycles in the foreign-key graph.
func (s *Service) FindCircularDependencies(ctx context.Context) (map[string]any, error) {
	edges, err := s.allForeignKeyEdges(ctx)
	if err != nil {
		return nil, err
	}
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}

	var cycles [][]string
	color := map[string]int{} // 0 unvisited, 1 in-stack, 2 done
	var stack []string

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = 1
		stack = append(stack, node)
		for _, nb := range adj[node] {
			switch color[nb] {
			case 1:
				cycle := captureCycle(stack, nb)
				if len(cycle) > 0 {
					cycles = append(cycles, cycle)
				}
			case 0:
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

	deduped := dedupeCycles(cycles)
	return map[string]any{
		"cycles":     deduped,
		"cycleCount": len(deduped),
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
