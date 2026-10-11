package postgres

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

var (
	reLineComment  = regexp.MustCompile(`--[^\n]*`)
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reStringLit    = regexp.MustCompile(`'(?:[^']|'')*'`)

	reBeginTran  = regexp.MustCompile(`(?i)\b(BEGIN|START\s+TRANSACTION)\b`)
	reCommit     = regexp.MustCompile(`(?i)\bCOMMIT\b`)
	reRollback   = regexp.MustCompile(`(?i)\bROLLBACK\b`)
	reSaveTran   = regexp.MustCompile(`(?i)\bSAVEPOINT\b`)
	reTryCatch   = regexp.MustCompile(`(?i)\bEXCEPTION\b`)
	reRaiserror  = regexp.MustCompile(`(?i)\bRAISE\s+(EXCEPTION|WARNING|NOTICE|INFO|LOG|DEBUG)\b`)
	reErrorFns   = regexp.MustCompile(`(?i)\b(GET\s+(?:(?:CURRENT|STACKED)\s+)?DIAGNOSTICS|SQLSTATE|SQLERRM)\b`)
	reCursor     = regexp.MustCompile(`(?i)\b(?:DECLARE\s+([A-Za-z_][A-Za-z0-9_]*)\s+CURSOR|OPEN\s+([A-Za-z_][A-Za-z0-9_]*))\b`)
	reTempTable  = regexp.MustCompile(`(?i)\b(?:CREATE\s+(?:TEMP|TEMPORARY)\s+TABLE|TEMPORARY|TEMP)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reDynExec    = regexp.MustCompile(`(?i)\b(?:EXECUTE\s+FORMAT|EXECUTE)\b`)
	reIdentifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*){0,2}`)
	reFromJoin   = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|UPDATE|TABLE)\s+([A-Za-z_"][A-Za-z0-9_".]*)`)
	reExecProc   = regexp.MustCompile(`(?i)\b(?:CALL|PERFORM)\s+([A-Za-z_"][A-Za-z0-9_".]*)`)
	reValidation = regexp.MustCompile(`(?i)\bIF\b[^;]*?\b(RAISE|RETURN)\b[^\n;]*`)
	reBizComment = regexp.MustCompile(`(?i)--\s*(business rule|rule:|note:|important|policy)[^\n]*`)
)

func cleanDefinition(def string) string {
	out := reBlockComment.ReplaceAllString(def, " ")
	out = reLineComment.ReplaceAllString(out, " ")
	out = reStringLit.ReplaceAllString(out, "''")
	return out
}

type catalogIndex struct {
	tables     map[string]string
	views      map[string]string
	functions  map[string]string
	procedures map[string]string
}

func (s *Service) buildCatalogIndex(ctx context.Context) (*catalogIndex, error) {
	idx := &catalogIndex{
		tables:     map[string]string{},
		views:      map[string]string{},
		functions:  map[string]string{},
		procedures: map[string]string{},
	}

	load := func(sql string, dest map[string]string) error {
		rows, err := s.rows(ctx, sql)
		if err != nil {
			return err
		}
		for _, r := range rows {
			schema, _ := r["schema_name"].(string)
			name, _ := r["object_name"].(string)
			if name == "" {
				continue
			}
			qualified := schema + "." + name
			dest[strings.ToLower(name)] = qualified
			dest[strings.ToLower(qualified)] = qualified
		}
		return nil
	}

	if err := load(`SELECT n.nspname AS schema_name, c.relname AS object_name FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog','information_schema')`, idx.tables); err != nil {
		return nil, err
	}
	if err := load(`SELECT n.nspname AS schema_name, c.relname AS object_name FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid WHERE c.relkind IN ('v','m') AND n.nspname NOT IN ('pg_catalog','information_schema')`, idx.views); err != nil {
		return nil, err
	}
	if err := load(`SELECT n.nspname AS schema_name, p.proname AS object_name FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON p.pronamespace = n.oid WHERE p.prokind IN ('f','a','w') AND n.nspname NOT IN ('pg_catalog','information_schema')`, idx.functions); err != nil {
		return nil, err
	}
	if err := load(`SELECT n.nspname AS schema_name, p.proname AS object_name FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON p.pronamespace = n.oid WHERE p.prokind = 'p' AND n.nspname NOT IN ('pg_catalog','information_schema')`, idx.procedures); err != nil {
		return nil, err
	}

	return idx, nil
}

func normalizeRef(raw string) (bare, full string) {
	raw = strings.Trim(raw, " \t\r\n\"")
	raw = strings.ReplaceAll(raw, `"`, "")
	parts := strings.Split(raw, ".")
	switch len(parts) {
	case 1:
		return strings.ToLower(parts[0]), strings.ToLower(parts[0])
	case 2:
		return strings.ToLower(parts[1]), strings.ToLower(parts[0] + "." + parts[1])
	case 3:
		return strings.ToLower(parts[2]), strings.ToLower(parts[1] + "." + parts[2])
	default:
		return strings.ToLower(raw), strings.ToLower(raw)
	}
}

func (s *Service) analyzeDefinition(ctx context.Context, def string) (map[string]any, error) {
	idx, err := s.buildCatalogIndex(ctx)
	if err != nil {
		return nil, err
	}
	clean := cleanDefinition(def)

	tablesUsed := map[string]bool{}
	viewsUsed := map[string]bool{}
	functionsUsed := map[string]bool{}
	calledProcs := map[string]bool{}

	for _, m := range reFromJoin.FindAllStringSubmatch(clean, -1) {
		bare, full := normalizeRef(m[1])
		if q, ok := idx.tables[full]; ok {
			tablesUsed[q] = true
		} else if q, ok := idx.tables[bare]; ok {
			tablesUsed[q] = true
		} else if q, ok := idx.views[full]; ok {
			viewsUsed[q] = true
		} else if q, ok := idx.views[bare]; ok {
			viewsUsed[q] = true
		}
	}

	for _, m := range reExecProc.FindAllStringSubmatch(clean, -1) {
		bare, full := normalizeRef(m[1])
		if q, ok := idx.procedures[full]; ok {
			calledProcs[q] = true
		} else if q, ok := idx.procedures[bare]; ok {
			calledProcs[q] = true
		}
	}

	for _, m := range reIdentifier.FindAllString(clean, -1) {
		bare, full := normalizeRef(m)
		if q, ok := idx.functions[full]; ok {
			functionsUsed[q] = true
		} else if q, ok := idx.functions[bare]; ok {
			functionsUsed[q] = true
		}
		if q, ok := idx.views[full]; ok {
			viewsUsed[q] = true
		} else if q, ok := idx.tables[full]; ok {
			tablesUsed[q] = true
		}
	}

	return map[string]any{
		"tablesUsed":       uniqueSorted(tablesUsed),
		"viewsUsed":        uniqueSorted(viewsUsed),
		"functionsUsed":    uniqueSorted(functionsUsed),
		"calledProcedures": uniqueSorted(calledProcs),
		"validationRules":  extractValidationRules(clean),
		"businessRules":    extractBusinessRules(def),
		"transactionUsage": extractTransactionUsage(clean),
		"dynamicSql":       extractDynamicSQL(clean),
		"cursorUsage":      extractCursors(clean),
		"tempTables":       extractTempTables(clean),
		"errorHandling":    extractErrorHandling(clean),
	}, nil
}

func extractTransactionUsage(clean string) []string {
	var out []string
	if reBeginTran.MatchString(clean) {
		out = append(out, "BEGIN/START TRANSACTION")
	}
	if reCommit.MatchString(clean) {
		out = append(out, "COMMIT")
	}
	if reRollback.MatchString(clean) {
		out = append(out, "ROLLBACK")
	}
	if reSaveTran.MatchString(clean) {
		out = append(out, "SAVEPOINT")
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func extractDynamicSQL(clean string) []string {
	matches := reDynExec.FindAllString(clean, -1)
	set := map[string]bool{}
	for _, m := range matches {
		set[strings.ToUpper(strings.TrimSpace(m))] = true
	}
	return uniqueSorted(set)
}

func extractCursors(clean string) []string {
	set := map[string]bool{}
	for _, m := range reCursor.FindAllStringSubmatch(clean, -1) {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				set[m[i]] = true
			}
		}
	}
	return uniqueSorted(set)
}

func extractTempTables(clean string) []string {
	set := map[string]bool{}
	for _, m := range reTempTable.FindAllStringSubmatch(clean, -1) {
		if len(m) > 1 && m[1] != "" {
			set[m[1]] = true
		}
	}
	return uniqueSorted(set)
}

func extractErrorHandling(clean string) []string {
	var out []string
	if reTryCatch.MatchString(clean) {
		out = append(out, "EXCEPTION block")
	}
	if reRaiserror.MatchString(clean) {
		out = append(out, "RAISE")
	}
	if reErrorFns.MatchString(clean) {
		out = append(out, "GET DIAGNOSTICS / SQLSTATE / SQLERRM")
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func extractValidationRules(clean string) []string {
	set := map[string]bool{}
	for _, m := range reValidation.FindAllString(clean, -1) {
		rule := strings.Join(strings.Fields(m), " ")
		if len(rule) > 200 {
			rule = rule[:200] + "..."
		}
		set[rule] = true
	}
	return uniqueSorted(set)
}

func extractBusinessRules(def string) []string {
	set := map[string]bool{}
	for _, m := range reBizComment.FindAllString(def, -1) {
		set[strings.TrimSpace(strings.TrimPrefix(m, "--"))] = true
	}
	return uniqueSorted(set)
}

func uniqueSorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DependencyGraph builds a recursive dependency graph for a database object.
func (s *Service) DependencyGraph(ctx context.Context, object string) (map[string]any, error) {
	schema, name := splitObjectName(object, "public")
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
		"schema": schema,
		"object": name,
		"edges":  edges,
	}, nil
}

// FindColumnUsage finds views and modules referencing a specific table column.
func (s *Service) FindColumnUsage(ctx context.Context, table, column string) ([]map[string]any, error) {
	schema, object := splitObjectName(table, "public")
	return s.rows(ctx, `
SELECT DISTINCT view_schema AS schema_name,
       view_name   AS object_name,
       'VIEW'      AS object_type
FROM   information_schema.view_column_usage
WHERE  table_schema = $1 AND table_name = $2 AND column_name = $3
ORDER  BY view_schema, view_name`, schema, object, column)
}

// FindObjectDependencies returns the direct dependencies of an object.
func (s *Service) FindObjectDependencies(ctx context.Context, object string) (map[string]any, error) {
	schema, name := splitObjectName(object, "public")
	deps, err := s.objectDependencies(ctx, schema, name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":       schema,
		"object":       name,
		"dependencies": deps,
	}, nil
}

// FindObjectReferences returns objects that reference the given object.
func (s *Service) FindObjectReferences(ctx context.Context, object string) (map[string]any, error) {
	schema, name := splitObjectName(object, "public")
	refs, err := s.objectReferencers(ctx, schema, name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":       schema,
		"object":       name,
		"referencedBy": refs,
	}, nil
}

// FindTableUsage returns all objects referencing a table.
func (s *Service) FindTableUsage(ctx context.Context, table string) (map[string]any, error) {
	schema, name := splitObjectName(table, "public")
	refs, err := s.objectReferencers(ctx, schema, name)
	if err != nil {
		return nil, err
	}
	var procedures, views, functions, triggers []map[string]any
	for _, r := range refs {
		typ, _ := r["object_type"].(string)
		entry := map[string]any{"schema": r["schema_name"], "name": r["object_name"]}
		switch typ {
		case "r":
			// table
		case "v", "m":
			views = append(views, entry)
		default:
			typUpper := strings.ToUpper(typ)
			switch {
			case strings.Contains(typUpper, "PROC"):
				procedures = append(procedures, entry)
			case strings.Contains(typUpper, "TRIG"):
				triggers = append(triggers, entry)
			default:
				functions = append(functions, entry)
			}
		}
	}
	if procedures == nil {
		procedures = []map[string]any{}
	}
	if views == nil {
		views = []map[string]any{}
	}
	if functions == nil {
		functions = []map[string]any{}
	}
	if triggers == nil {
		triggers = []map[string]any{}
	}
	return map[string]any{
		"schema":     schema,
		"table":      name,
		"procedures": procedures,
		"views":      views,
		"functions":  functions,
		"triggers":   triggers,
		"references": refs,
	}, nil
}
