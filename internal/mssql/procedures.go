package mssql

import (
	"context"
	"fmt"
)

// ListProcedures returns all stored procedures, optionally filtered by schema.
func (s *Service) ListProcedures(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    p.name          AS procedure_name,
    p.object_id     AS object_id,
    p.create_date   AS create_date,
    p.modify_date   AS modify_date
FROM sys.procedures p
JOIN sys.schemas sch ON p.schema_id = sch.schema_id
WHERE (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, p.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// GetProcedureDefinition returns the T-SQL body of a stored procedure together
// with its parameters.
func (s *Service) GetProcedureDefinition(ctx context.Context, procedure string) (map[string]any, error) {
	schema, object := splitObjectName(procedure, "dbo")
	def, err := s.objectDefinition(ctx, procedure)
	if err != nil {
		return nil, err
	}
	params, err := s.objectParameters(ctx, schema, object)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":     schema,
		"procedure":  object,
		"parameters": params,
		"definition": def,
	}, nil
}

// objectParameters returns the parameters of a procedure or function.
func (s *Service) objectParameters(ctx context.Context, schema, object string) ([]map[string]any, error) {
	const sql = `
SELECT
    pa.name                 AS parameter_name,
    ty.name                 AS data_type,
    pa.max_length           AS max_length,
    pa.precision            AS [precision],
    pa.scale                AS scale,
    pa.is_output            AS is_output,
    pa.parameter_id         AS ordinal
FROM sys.parameters pa
JOIN sys.objects o ON pa.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
JOIN sys.types ty ON pa.user_type_id = ty.user_type_id
WHERE s.name = @schema AND o.name = @object
ORDER BY pa.parameter_id`
	return s.rows(ctx, sql, sqlParam("@schema", schema), sqlParam("@object", object))
}

// SearchProcedures finds stored procedures whose definition contains the given
// keyword.
func (s *Service) SearchProcedures(ctx context.Context, keyword string) ([]map[string]any, error) {
	if keyword == "" {
		return nil, fmt.Errorf("keyword must not be empty")
	}
	const sql = `
SELECT
    sch.name        AS schema_name,
    p.name          AS procedure_name,
    p.modify_date   AS modify_date
FROM sys.procedures p
JOIN sys.schemas sch ON p.schema_id = sch.schema_id
JOIN sys.sql_modules m ON p.object_id = m.object_id
WHERE m.definition LIKE '%' + @keyword + '%'
ORDER BY sch.name, p.name`
	return s.rows(ctx, sql, sqlParam("@keyword", keyword))
}

// AnalyzeStoredProcedure performs a full static analysis of a stored procedure,
// returning the consolidated report shape described in the task specification.
func (s *Service) AnalyzeStoredProcedure(ctx context.Context, procedure string) (map[string]any, error) {
	def, err := s.objectDefinition(ctx, procedure)
	if err != nil {
		return nil, err
	}
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	schema, object := splitObjectName(procedure, "dbo")
	result := map[string]any{
		"schema":            schema,
		"procedure":         object,
		"tablesUsed":        analysis["tablesUsed"],
		"viewsUsed":         analysis["viewsUsed"],
		"functionsUsed":     analysis["functionsUsed"],
		"calledProcedures":  analysis["calledProcedures"],
		"validationRules":   analysis["validationRules"],
		"businessRules":     analysis["businessRules"],
		"transactionUsage":  analysis["transactionUsage"],
		"dynamicSql":        analysis["dynamicSql"],
		"cursorUsage":       analysis["cursorUsage"],
		"tempTables":        analysis["tempTables"],
		"errorHandling":     analysis["errorHandling"],
	}
	return result, nil
}

// extractAspect runs the full analysis and returns one aspect under the given
// output key. It is shared by all the focused extract_* tools.
func (s *Service) extractAspect(ctx context.Context, procedure, analysisKey, outputKey string) (map[string]any, error) {
	def, err := s.objectDefinition(ctx, procedure)
	if err != nil {
		return nil, err
	}
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	schema, object := splitObjectName(procedure, "dbo")
	return map[string]any{
		"schema":    schema,
		"procedure": object,
		outputKey:   analysis[analysisKey],
	}, nil
}

// The focused extraction tools below all delegate to extractAspect.

func (s *Service) ExtractTablesUsed(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "tablesUsed", "tablesUsed")
}

func (s *Service) ExtractViewsUsed(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "viewsUsed", "viewsUsed")
}

func (s *Service) ExtractFunctionsUsed(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "functionsUsed", "functionsUsed")
}

func (s *Service) ExtractCalledProcedures(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "calledProcedures", "calledProcedures")
}

func (s *Service) ExtractTransactionUsage(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "transactionUsage", "transactionUsage")
}

func (s *Service) ExtractDynamicSQL(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "dynamicSql", "dynamicSql")
}

func (s *Service) ExtractCursorUsage(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "cursorUsage", "cursorUsage")
}

func (s *Service) ExtractTempTables(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "tempTables", "tempTables")
}

func (s *Service) ExtractErrorHandling(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "errorHandling", "errorHandling")
}

func (s *Service) ExtractValidationRules(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "validationRules", "validationRules")
}

func (s *Service) ExtractBusinessRules(ctx context.Context, procedure string) (map[string]any, error) {
	return s.extractAspect(ctx, procedure, "businessRules", "businessRules")
}
