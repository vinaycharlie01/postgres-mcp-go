package postgres

import (
	"context"
	"fmt"
)

func (s *Service) ListProcedures(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       p.proname AS procedure_name,
       p.oid     AS object_id
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.prokind = 'p'
  AND  n.nspname NOT IN ('pg_catalog','information_schema')
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, p.proname`, schema)
}

func (s *Service) GetProcedureDefinition(ctx context.Context, procedure string) (map[string]any, error) {
	schema, object := splitObjectName(procedure, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT n.nspname AS schema_name,
       p.proname AS procedure_name,
       pg_get_functiondef(p.oid) AS definition,
       pg_get_function_arguments(p.oid) AS parameters
FROM   pg_catalog.pg_proc p
JOIN   pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE  p.prokind = 'p' AND n.nspname = $1 AND p.proname = $2
LIMIT  1`, schema, object)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("procedure %s.%s not found", schema, object)
	}
	return row, nil
}

func (s *Service) AnalyzeStoredProcedure(ctx context.Context, procedure string) (map[string]any, error) {
	schema, object := splitObjectName(procedure, "public")
	defRow, err := s.GetProcedureDefinition(ctx, procedure)
	if err != nil {
		return nil, err
	}
	def, _ := defRow["definition"].(string)
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"schema":           schema,
		"procedure":        object,
		"tablesUsed":       analysis["tablesUsed"],
		"viewsUsed":        analysis["viewsUsed"],
		"functionsUsed":    analysis["functionsUsed"],
		"calledProcedures": analysis["calledProcedures"],
		"validationRules":  analysis["validationRules"],
		"businessRules":    analysis["businessRules"],
		"transactionUsage": analysis["transactionUsage"],
		"dynamicSql":       analysis["dynamicSql"],
		"cursorUsage":      analysis["cursorUsage"],
		"tempTables":       analysis["tempTables"],
		"errorHandling":    analysis["errorHandling"],
	}
	return result, nil
}

// The extract_* tools delegate to AnalyzeStoredProcedure and pick one key.
func (s *Service) extractProcAspect(ctx context.Context, procedure, analysisKey, outputKey string) (map[string]any, error) {
	full, err := s.AnalyzeStoredProcedure(ctx, procedure)
	if err != nil {
		return nil, err
	}
	schema, object := splitObjectName(procedure, "public")
	return map[string]any{"schema": schema, "procedure": object, outputKey: full[analysisKey]}, nil
}

func (s *Service) ExtractTablesUsed(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "tablesUsed", "tablesUsed")
}
func (s *Service) ExtractViewsUsed(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "viewsUsed", "viewsUsed")
}
func (s *Service) ExtractFunctionsUsed(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "functionsUsed", "functionsUsed")
}
func (s *Service) ExtractCalledProcedures(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "calledProcedures", "calledProcedures")
}
func (s *Service) ExtractTransactionUsage(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "transactionUsage", "transactionUsage")
}
func (s *Service) ExtractDynamicSQL(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "dynamicSql", "dynamicSql")
}
func (s *Service) ExtractCursorUsage(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "cursorUsage", "cursorUsage")
}
func (s *Service) ExtractTempTables(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "tempTables", "tempTables")
}
func (s *Service) ExtractErrorHandling(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "errorHandling", "errorHandling")
}
func (s *Service) ExtractValidationRules(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "validationRules", "validationRules")
}
func (s *Service) ExtractBusinessRules(ctx context.Context, p string) (map[string]any, error) {
	return s.extractProcAspect(ctx, p, "businessRules", "businessRules")
}
