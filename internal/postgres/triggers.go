package postgres

import (
	"context"
	"fmt"
)

// ListTriggers returns all DML triggers with the table they are attached to.
func (s *Service) ListTriggers(ctx context.Context, schema string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT n.nspname AS schema_name,
       tr.tgname AS trigger_name,
       c.relname AS table_name,
       (tr.tgenabled != 'D') AS is_enabled,
       ((tr.tgtype & 64) != 0) AS is_instead_of,
       pg_get_triggerdef(tr.oid) AS definition
FROM   pg_catalog.pg_trigger tr
JOIN   pg_catalog.pg_class c ON c.oid = tr.tgrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  NOT tr.tgisinternal
  AND  n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND  ($1 = '' OR n.nspname = $1)
ORDER  BY n.nspname, c.relname, tr.tgname`, schema)
}

// GetTriggerDefinition returns the definition of a trigger.
func (s *Service) GetTriggerDefinition(ctx context.Context, trigger string) (map[string]any, error) {
	schema, object := splitObjectName(trigger, "public")
	row, err := s.conn.QueryRow(ctx, `
SELECT n.nspname AS schema_name,
       tr.tgname AS trigger_name,
       c.relname AS table_name,
       pg_get_triggerdef(tr.oid) AS definition
FROM   pg_catalog.pg_trigger tr
JOIN   pg_catalog.pg_class c ON c.oid = tr.tgrelid
JOIN   pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE  NOT tr.tgisinternal
  AND  n.nspname = $1 AND tr.tgname = $2
LIMIT  1`, schema, object)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("trigger %s.%s not found", schema, object)
	}
	return row, nil
}

// AnalyzeTrigger performs static analysis of a trigger definition.
func (s *Service) AnalyzeTrigger(ctx context.Context, trigger string) (map[string]any, error) {
	schema, object := splitObjectName(trigger, "public")
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
