package mssql

import (
	"context"
)

// ListTriggers returns all DML triggers with the table they are attached to.
func (s *Service) ListTriggers(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    tr.name         AS trigger_name,
    t.name          AS table_name,
    tr.is_disabled  AS is_disabled,
    tr.is_instead_of_trigger AS is_instead_of,
    tr.create_date  AS create_date,
    tr.modify_date  AS modify_date
FROM sys.triggers tr
JOIN sys.tables t ON tr.parent_id = t.object_id
JOIN sys.schemas sch ON t.schema_id = sch.schema_id
WHERE tr.is_ms_shipped = 0
  AND (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, t.name, tr.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// GetTriggerDefinition returns the T-SQL body of a trigger.
func (s *Service) GetTriggerDefinition(ctx context.Context, trigger string) (map[string]any, error) {
	schema, object := splitObjectName(trigger, "dbo")
	const sql = `
SELECT m.definition AS definition
FROM sys.sql_modules m
JOIN sys.triggers tr ON m.object_id = tr.object_id
JOIN sys.tables t ON tr.parent_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE tr.name = @trigger`
	def, err := s.scalar(ctx, sql, sqlParam("@trigger", object))
	if err != nil {
		return nil, err
	}
	defStr, _ := def.(string)
	return map[string]any{
		"schema":     schema,
		"trigger":    object,
		"definition": defStr,
	}, nil
}

// AnalyzeTrigger performs static analysis of a trigger definition.
func (s *Service) AnalyzeTrigger(ctx context.Context, trigger string) (map[string]any, error) {
	schema, object := splitObjectName(trigger, "dbo")
	res, err := s.GetTriggerDefinition(ctx, trigger)
	if err != nil {
		return nil, err
	}
	def, _ := res["definition"].(string)
	analysis, err := s.analyzeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema":           schema,
		"trigger":          object,
		"tablesUsed":       analysis["tablesUsed"],
		"viewsUsed":        analysis["viewsUsed"],
		"functionsUsed":    analysis["functionsUsed"],
		"calledProcedures": analysis["calledProcedures"],
		"transactionUsage": analysis["transactionUsage"],
		"errorHandling":    analysis["errorHandling"],
	}, nil
}
