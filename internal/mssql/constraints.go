package mssql

import "context"

// ListPrimaryKeys returns all primary key constraints with their key columns.
func (s *Service) ListPrimaryKeys(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    t.name          AS table_name,
    kc.name         AS constraint_name,
    STUFF((
        SELECT ', ' + c.name
        FROM sys.index_columns ic
        JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
        WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id
        ORDER BY ic.key_ordinal
        FOR XML PATH('')), 1, 2, '') AS key_columns
FROM sys.key_constraints kc
JOIN sys.tables t ON kc.parent_object_id = t.object_id
JOIN sys.schemas sch ON t.schema_id = sch.schema_id
JOIN sys.indexes i ON kc.unique_index_id = i.index_id AND kc.parent_object_id = i.object_id
WHERE kc.type = 'PK'
  AND (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, t.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// ListForeignKeys returns all foreign key relationships.
func (s *Service) ListForeignKeys(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    fk.name             AS constraint_name,
    ps.name             AS parent_schema,
    pt.name             AS parent_table,
    cpar.name           AS parent_column,
    rs.name             AS referenced_schema,
    rt.name             AS referenced_table,
    cref.name           AS referenced_column,
    fk.delete_referential_action_desc AS on_delete,
    fk.update_referential_action_desc AS on_update
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
JOIN sys.tables pt ON fk.parent_object_id = pt.object_id
JOIN sys.schemas ps ON pt.schema_id = ps.schema_id
JOIN sys.columns cpar ON fkc.parent_object_id = cpar.object_id AND fkc.parent_column_id = cpar.column_id
JOIN sys.tables rt ON fk.referenced_object_id = rt.object_id
JOIN sys.schemas rs ON rt.schema_id = rs.schema_id
JOIN sys.columns cref ON fkc.referenced_object_id = cref.object_id AND fkc.referenced_column_id = cref.column_id
WHERE (@schema = '' OR ps.name = @schema)
ORDER BY ps.name, pt.name, fk.name, fkc.constraint_column_id`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// ListUniqueConstraints returns all unique constraints.
func (s *Service) ListUniqueConstraints(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    t.name          AS table_name,
    kc.name         AS constraint_name,
    STUFF((
        SELECT ', ' + c.name
        FROM sys.index_columns ic
        JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
        WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id
        ORDER BY ic.key_ordinal
        FOR XML PATH('')), 1, 2, '') AS key_columns
FROM sys.key_constraints kc
JOIN sys.tables t ON kc.parent_object_id = t.object_id
JOIN sys.schemas sch ON t.schema_id = sch.schema_id
JOIN sys.indexes i ON kc.unique_index_id = i.index_id AND kc.parent_object_id = i.object_id
WHERE kc.type = 'UQ'
  AND (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, t.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// ListCheckConstraints returns all CHECK constraints with their definitions.
func (s *Service) ListCheckConstraints(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    t.name          AS table_name,
    cc.name         AS constraint_name,
    cc.definition   AS definition,
    cc.is_disabled  AS is_disabled
FROM sys.check_constraints cc
JOIN sys.tables t ON cc.parent_object_id = t.object_id
JOIN sys.schemas sch ON t.schema_id = sch.schema_id
WHERE (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, t.name, cc.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// ListDefaultConstraints returns all DEFAULT constraints with their definitions.
func (s *Service) ListDefaultConstraints(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name        AS schema_name,
    t.name          AS table_name,
    dc.name         AS constraint_name,
    col.name        AS column_name,
    dc.definition   AS definition
FROM sys.default_constraints dc
JOIN sys.tables t ON dc.parent_object_id = t.object_id
JOIN sys.schemas sch ON t.schema_id = sch.schema_id
JOIN sys.columns col ON dc.parent_object_id = col.object_id AND dc.parent_column_id = col.column_id
WHERE (@schema = '' OR sch.name = @schema)
ORDER BY sch.name, t.name, col.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}
