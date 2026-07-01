package mssql

import (
	"context"
)

// ListFunctions returns all user-defined functions (scalar, inline and
// table-valued), optionally filtered by schema.
func (s *Service) ListFunctions(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    o.name          AS function_name,
    o.type_desc     AS function_type,
    o.object_id     AS object_id,
    o.create_date   AS create_date,
    o.modify_date   AS modify_date
FROM sys.objects o
JOIN sys.schemas sch ON o.schema_id = sch.schema_id
WHERE o.type IN ('FN','IF','TF','FS','FT')
  AND (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, o.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// GetFunctionDefinition returns the T-SQL body of a function with its
// parameters.
func (s *Service) GetFunctionDefinition(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "dbo")
	def, err := s.objectDefinition(ctx, function)
	if err != nil {
		return nil, err
	}
	params, err := s.objectParameters(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":     schema,
		"function":   object,
		"parameters": params,
		"definition": def,
	}, nil
}

// AnalyzeFunction performs static analysis of a function definition.
func (s *Service) AnalyzeFunction(ctx context.Context, function string) (map[string]any, error) {
	def, err := s.objectDefinition(ctx, function)
	if err != nil {
		return nil, err
	}
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	schema, object := splitObjectName(function, "dbo")
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

// TraceFunctionUsage finds every object that references the given function.
func (s *Service) TraceFunctionUsage(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "dbo")
	refs, err := s.objectReferencers(ctx, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":      schema,
		"function":    object,
		"referencedBy": refs,
	}, nil
}

// ExtractFunctionDependencies lists the objects the function depends on.
func (s *Service) ExtractFunctionDependencies(ctx context.Context, function string) (map[string]any, error) {
	schema, object := splitObjectName(function, "dbo")
	deps, err := s.objectDependencies(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":       schema,
		"function":     object,
		"dependencies": deps,
	}, nil
}
