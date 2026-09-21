package postgres

import (
	"context"
	"fmt"
)

// ListFunctions lists all user-defined functions and procedures.
func (s *Service) ListFunctions(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       p.proname AS function_name,
       CASE p.prokind WHEN 'f' THEN 'FUNCTION' WHEN 'p' THEN 'PROCEDURE'
                      WHEN 'a' THEN 'AGGREGATE' ELSE 'WINDOW' END AS function_type,
       p.oid     AS object_id
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.prokind IN ('f','p')
  AND  n.nspname NOT IN ('pg_catalog','information_schema')
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, p.proname`, schema)
}

// GetFunctionDefinition returns the body and parameters of a function.
func (s *Service) GetFunctionDefinition(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT n.nspname AS schema_name,
       p.proname AS function_name,
       pg_get_functiondef(p.oid) AS definition,
       pg_get_function_arguments(p.oid) AS parameters,
       pg_get_function_result(p.oid) AS return_type
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  n.nspname = $1 AND p.proname = $2
LIMIT  1`, schema, object)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("function %s.%s not found", schema, object)
	}
	return row, nil
}

// AnalyzeFunction returns definition and static analysis info for a function.
func (s *Service) AnalyzeFunction(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "public")
	defRow, err := s.GetFunctionDefinition(ctx, function)
	if err != nil {
		return nil, err
	}
	def, _ := defRow["definition"].(string)
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":           schema,
		"function":         object,
		"tablesUsed":       analysis["tablesUsed"],
		"viewsUsed":        analysis["viewsUsed"],
		"functionsUsed":    analysis["functionsUsed"],
		"calledProcedures": analysis["calledProcedures"],
		"errorHandling":    analysis["errorHandling"],
	}, nil
}

// TraceFunctionUsage finds objects that reference this function.
func (s *Service) TraceFunctionUsage(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "public")
	refs, err := s.objectReferencers(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "function": object, "referencedBy": refs}, nil
}

// ExtractFunctionDependencies lists what a function depends on.
func (s *Service) ExtractFunctionDependencies(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "public")
	deps, err := s.objectDependencies(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "function": object, "dependencies": deps}, nil
}
