package mssql

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// This file contains T-SQL static-analysis helpers used by the stored
// procedure, function, view and trigger analysis tools. The extraction is
// heuristic: it strips comments and string literals, then combines pattern
// matching with a cross-reference against the live catalog so that referenced
// tables/views/functions/procedures are classified accurately.

var (
	reLineComment  = regexp.MustCompile(`--[^\n]*`)
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reStringLit    = regexp.MustCompile(`'(?:[^']|'')*'`)

	reBeginTran   = regexp.MustCompile(`(?i)\bBEGIN\s+TRAN(SACTION)?\b`)
	reCommit      = regexp.MustCompile(`(?i)\bCOMMIT\b`)
	reRollback    = regexp.MustCompile(`(?i)\bROLLBACK\b`)
	reSaveTran    = regexp.MustCompile(`(?i)\bSAVE\s+TRAN(SACTION)?\b`)
	reTryCatch    = regexp.MustCompile(`(?i)\bBEGIN\s+TRY\b`)
	reRaiserror   = regexp.MustCompile(`(?i)\bRAISERROR\b`)
	reThrow       = regexp.MustCompile(`(?i)\bTHROW\b`)
	reErrorFns    = regexp.MustCompile(`(?i)\bERROR_(MESSAGE|NUMBER|SEVERITY|STATE|LINE|PROCEDURE)\s*\(`)
	reCursor      = regexp.MustCompile(`(?i)\bDECLARE\s+([A-Za-z_][A-Za-z0-9_@#$]*)\s+(INSENSITIVE\s+|SCROLL\s+)*CURSOR\b`)
	reTempTable   = regexp.MustCompile(`#{1,2}[A-Za-z_][A-Za-z0-9_]*`)
	reDynExec     = regexp.MustCompile(`(?i)\b(EXEC(UTE)?\s*\(|SP_EXECUTESQL)\b`)
	reIdentifier  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*){0,2}`)
	reFromJoin    = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|APPLY|INTO|UPDATE|MERGE)\s+([A-Za-z_\[][A-Za-z0-9_\].]*)`)
	reExecProc    = regexp.MustCompile(`(?i)\b(?:EXEC|EXECUTE)\s+([A-Za-z_\[][A-Za-z0-9_\].]*)`)
	reValidation  = regexp.MustCompile(`(?i)\bIF\b[^\n]*?\b(RAISERROR|THROW|RETURN)\b[^\n]*`)
	reBizComment  = regexp.MustCompile(`(?i)--\s*(business rule|rule:|note:|important|policy)[^\n]*`)
)

// cleanDefinition removes comments and string literals from a module
// definition so that pattern matching is not confused by their contents.
func cleanDefinition(def string) string {
	out := reBlockComment.ReplaceAllString(def, " ")
	out = reLineComment.ReplaceAllString(out, " ")
	out = reStringLit.ReplaceAllString(out, "''")
	return out
}

// catalogIndex holds lowercased name sets for each object type, used to classify
// identifiers found inside a module definition.
type catalogIndex struct {
	tables     map[string]string // lower bare/qualified name -> "schema.name"
	views      map[string]string
	functions  map[string]string
	procedures map[string]string
}

// buildCatalogIndex loads the names of all tables, views, functions and
// procedures so referenced identifiers can be classified.
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

	if err := load(`SELECT s.name AS schema_name, t.name AS object_name FROM sys.tables t JOIN sys.schemas s ON t.schema_id = s.schema_id`, idx.tables); err != nil {
		return nil, err
	}
	if err := load(`SELECT s.name AS schema_name, v.name AS object_name FROM sys.views v JOIN sys.schemas s ON v.schema_id = s.schema_id`, idx.views); err != nil {
		return nil, err
	}
	if err := load(`SELECT s.name AS schema_name, o.name AS object_name FROM sys.objects o JOIN sys.schemas s ON o.schema_id = s.schema_id WHERE o.type IN ('FN','IF','TF','FS','FT')`, idx.functions); err != nil {
		return nil, err
	}
	if err := load(`SELECT s.name AS schema_name, p.name AS object_name FROM sys.procedures p JOIN sys.schemas s ON p.schema_id = s.schema_id`, idx.procedures); err != nil {
		return nil, err
	}

	return idx, nil
}

// normalizeRef strips brackets from a captured identifier and returns the bare
// object name (last component) and the lowercased full reference.
func normalizeRef(ref string) (bare string, full string) {
	ref = strings.TrimSpace(ref)
	ref = strings.ReplaceAll(ref, "[", "")
	ref = strings.ReplaceAll(ref, "]", "")
	full = strings.ToLower(ref)
	if idx := strings.LastIndex(ref, "."); idx >= 0 {
		bare = strings.ToLower(ref[idx+1:])
		// keep at most schema.object (drop database prefix)
		parts := strings.Split(ref, ".")
		if len(parts) >= 2 {
			full = strings.ToLower(parts[len(parts)-2] + "." + parts[len(parts)-1])
		}
	} else {
		bare = strings.ToLower(ref)
	}
	return bare, full
}

// uniqueSorted returns the de-duplicated, sorted values of a set.
func uniqueSorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// analyzeDefinition performs the full static analysis of a module definition.
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

	// Classify identifiers that follow FROM/JOIN/INTO/etc.
	for _, m := range reFromJoin.FindAllStringSubmatch(clean, -1) {
		bare, full := normalizeRef(m[1])
		if q, ok := idx.views[full]; ok {
			viewsUsed[q] = true
		} else if q, ok := idx.views[bare]; ok {
			viewsUsed[q] = true
		} else if q, ok := idx.tables[full]; ok {
			tablesUsed[q] = true
		} else if q, ok := idx.tables[bare]; ok {
			tablesUsed[q] = true
		}
	}

	// Called procedures via EXEC.
	for _, m := range reExecProc.FindAllStringSubmatch(clean, -1) {
		bare, full := normalizeRef(m[1])
		if q, ok := idx.procedures[full]; ok {
			calledProcs[q] = true
		} else if q, ok := idx.procedures[bare]; ok {
			calledProcs[q] = true
		}
	}

	// Functions: scan every identifier and intersect with the function catalog.
	for _, m := range reIdentifier.FindAllString(clean, -1) {
		bare, full := normalizeRef(m)
		if q, ok := idx.functions[full]; ok {
			functionsUsed[q] = true
		} else if q, ok := idx.functions[bare]; ok {
			functionsUsed[q] = true
		}
		// Also catch tables/views referenced without a FROM (e.g. in subqueries).
		if q, ok := idx.views[full]; ok {
			viewsUsed[q] = true
		} else if q, ok := idx.tables[full]; ok {
			tablesUsed[q] = true
		}
	}

	return map[string]any{
		"tablesUsed":        uniqueSorted(tablesUsed),
		"viewsUsed":         uniqueSorted(viewsUsed),
		"functionsUsed":     uniqueSorted(functionsUsed),
		"calledProcedures":  uniqueSorted(calledProcs),
		"validationRules":   extractValidationRules(clean),
		"businessRules":     extractBusinessRules(def),
		"transactionUsage":  extractTransactionUsage(clean),
		"dynamicSql":        extractDynamicSQL(clean),
		"cursorUsage":       extractCursors(clean),
		"tempTables":        extractTempTables(clean),
		"errorHandling":     extractErrorHandling(clean),
	}, nil
}

func extractTransactionUsage(clean string) []string {
	var out []string
	if reBeginTran.MatchString(clean) {
		out = append(out, "BEGIN TRANSACTION")
	}
	if reCommit.MatchString(clean) {
		out = append(out, "COMMIT")
	}
	if reRollback.MatchString(clean) {
		out = append(out, "ROLLBACK")
	}
	if reSaveTran.MatchString(clean) {
		out = append(out, "SAVE TRANSACTION")
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
		set[strings.ToUpper(strings.TrimSpace(strings.TrimRight(m, "(")))] = true
	}
	return uniqueSorted(set)
}

func extractCursors(clean string) []string {
	set := map[string]bool{}
	for _, m := range reCursor.FindAllStringSubmatch(clean, -1) {
		set[m[1]] = true
	}
	return uniqueSorted(set)
}

func extractTempTables(clean string) []string {
	set := map[string]bool{}
	for _, m := range reTempTable.FindAllString(clean, -1) {
		set[m] = true
	}
	return uniqueSorted(set)
}

func extractErrorHandling(clean string) []string {
	var out []string
	if reTryCatch.MatchString(clean) {
		out = append(out, "TRY/CATCH")
	}
	if reRaiserror.MatchString(clean) {
		out = append(out, "RAISERROR")
	}
	if reThrow.MatchString(clean) {
		out = append(out, "THROW")
	}
	if reErrorFns.MatchString(clean) {
		out = append(out, "ERROR_* functions")
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

// extractBusinessRules works on the original definition so it can pick up
// rule-bearing comments that cleanDefinition would have removed.
func extractBusinessRules(def string) []string {
	set := map[string]bool{}
	for _, m := range reBizComment.FindAllString(def, -1) {
		set[strings.TrimSpace(strings.TrimPrefix(m, "--"))] = true
	}
	return uniqueSorted(set)
}
