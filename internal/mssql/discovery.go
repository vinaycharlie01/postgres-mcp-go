package mssql

import (
	"context"
	"fmt"
)

// ListDatabases returns all databases accessible to the current login.
func (s *Service) ListDatabases(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    d.name                AS database_name,
    d.database_id         AS database_id,
    d.state_desc          AS state,
    d.recovery_model_desc AS recovery_model,
    d.collation_name      AS collation,
    d.create_date         AS create_date,
    d.is_read_only        AS is_read_only
FROM sys.databases d
WHERE HAS_DBACCESS(d.name) = 1
ORDER BY d.name`
	return s.rows(ctx, sql)
}

// ListSchemas returns all schemas in the current database.
func (s *Service) ListSchemas(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    s.name                  AS schema_name,
    s.schema_id             AS schema_id,
    p.name                  AS owner,
    COUNT(o.object_id)      AS object_count
FROM sys.schemas s
LEFT JOIN sys.database_principals p ON s.principal_id = p.principal_id
LEFT JOIN sys.objects o ON o.schema_id = s.schema_id AND o.is_ms_shipped = 0
GROUP BY s.name, s.schema_id, p.name
ORDER BY s.name`
	return s.rows(ctx, sql)
}

// ListTables returns all base tables, optionally filtered by schema.
func (s *Service) ListTables(ctx context.Context, schema string) ([]map[string]any, error) {
	const sql = `
SELECT
    sch.name                                   AS schema_name,
    t.name                                     AS table_name,
    t.object_id                                AS object_id,
    t.create_date                              AS create_date,
    t.modify_date                              AS modify_date,
    p.rows                                     AS row_count,
    (SELECT COUNT(*) FROM sys.columns c WHERE c.object_id = t.object_id) AS column_count
FROM sys.tables t
JOIN sys.schemas sch ON t.schema_id = sch.schema_id
LEFT JOIN sys.partitions p ON p.object_id = t.object_id AND p.index_id IN (0, 1)
WHERE (@schema = '' OR sch.name = @schema)
GROUP BY sch.name, t.name, t.object_id, t.create_date, t.modify_date, p.rows
ORDER BY sch.name, t.name`
	return s.rows(ctx, sql, sqlParam("@schema", schema))
}

// DescribeTable returns the columns, primary keys, foreign keys and indexes for
// a single table.
func (s *Service) DescribeTable(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")

	columns, err := s.ListColumns(ctx, table)
	if err != nil {
		return nil, fmt.Errorf("failed to read columns: %w", err)
	}

	pkRows, err := s.rows(ctx, `
SELECT c.name AS column_name, ic.key_ordinal AS ordinal
FROM sys.indexes i
JOIN sys.index_columns ic ON i.object_id = ic.object_id AND i.index_id = ic.index_id
JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
JOIN sys.tables t ON i.object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE i.is_primary_key = 1 AND s.name = @schema AND t.name = @table
ORDER BY ic.key_ordinal`, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}

	fkRows, err := s.rows(ctx, `
SELECT
    fk.name                       AS constraint_name,
    cpar.name                     AS column_name,
    rs.name                       AS referenced_schema,
    rt.name                       AS referenced_table,
    cref.name                     AS referenced_column
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
JOIN sys.tables pt ON fk.parent_object_id = pt.object_id
JOIN sys.schemas ps ON pt.schema_id = ps.schema_id
JOIN sys.columns cpar ON fkc.parent_object_id = cpar.object_id AND fkc.parent_column_id = cpar.column_id
JOIN sys.tables rt ON fk.referenced_object_id = rt.object_id
JOIN sys.schemas rs ON rt.schema_id = rs.schema_id
JOIN sys.columns cref ON fkc.referenced_object_id = cref.object_id AND fkc.referenced_column_id = cref.column_id
WHERE ps.name = @schema AND pt.name = @table
ORDER BY fk.name, fkc.constraint_column_id`, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}

	idxRows, err := s.rows(ctx, `
SELECT
    i.name                        AS index_name,
    i.type_desc                   AS index_type,
    i.is_unique                   AS is_unique,
    i.is_primary_key              AS is_primary_key,
    STUFF((
        SELECT ', ' + c.name
        FROM sys.index_columns ic
        JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
        WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id AND ic.is_included_column = 0
        ORDER BY ic.key_ordinal
        FOR XML PATH('')), 1, 2, '') AS key_columns
FROM sys.indexes i
JOIN sys.tables t ON i.object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
WHERE i.type > 0 AND s.name = @schema AND t.name = @table
ORDER BY i.is_primary_key DESC, i.name`, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s.%s not found", schema, object)
	}

	return map[string]any{
		"schema":      schema,
		"table":       object,
		"columns":     columns,
		"primaryKeys": pkRows,
		"foreignKeys": fkRows,
		"indexes":     idxRows,
	}, nil
}

// ListColumns returns the columns of a table or view.
func (s *Service) ListColumns(ctx context.Context, table string) ([]map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")
	const sql = `
SELECT
    c.column_id                               AS ordinal,
    c.name                                    AS column_name,
    ty.name                                   AS data_type,
    c.max_length                              AS max_length,
    c.precision                               AS [precision],
    c.scale                                   AS scale,
    c.is_nullable                             AS is_nullable,
    c.is_identity                             AS is_identity,
    c.is_computed                             AS is_computed,
    dc.definition                             AS default_definition,
    c.collation_name                          AS collation
FROM sys.columns c
JOIN sys.objects o ON c.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
JOIN sys.types ty ON c.user_type_id = ty.user_type_id
LEFT JOIN sys.default_constraints dc ON c.default_object_id = dc.object_id
WHERE s.name = @schema AND o.name = @table
ORDER BY c.column_id`
	return s.rows(ctx, sql, sqlParam("@schema", schema), sqlParam("@table", object))
}

// DescribeColumn returns detailed metadata for a single column.
func (s *Service) DescribeColumn(ctx context.Context, table, column string) (map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")
	const sql = `
SELECT
    s.name                                    AS schema_name,
    o.name                                    AS table_name,
    c.name                                    AS column_name,
    c.column_id                               AS ordinal,
    ty.name                                   AS data_type,
    c.max_length                              AS max_length,
    c.precision                               AS [precision],
    c.scale                                   AS scale,
    c.is_nullable                             AS is_nullable,
    c.is_identity                             AS is_identity,
    c.is_computed                             AS is_computed,
    c.collation_name                          AS collation,
    dc.definition                             AS default_definition,
    cc.definition                             AS computed_definition
FROM sys.columns c
JOIN sys.objects o ON c.object_id = o.object_id
JOIN sys.schemas s ON o.schema_id = s.schema_id
JOIN sys.types ty ON c.user_type_id = ty.user_type_id
LEFT JOIN sys.default_constraints dc ON c.default_object_id = dc.object_id
LEFT JOIN sys.computed_columns cc ON c.object_id = cc.object_id AND c.column_id = cc.column_id
WHERE s.name = @schema AND o.name = @table AND c.name = @column`
	row, err := s.conn.QueryRow(ctx, sql, sqlParam("@schema", schema), sqlParam("@table", object), sqlParam("@column", column))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("column %s not found on %s.%s", column, schema, object)
	}
	return row, nil
}

// TableRowCount returns an accurate row count for a table using catalog
// metadata (no scan of the base data).
func (s *Service) TableRowCount(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")
	const sql = `
SELECT
    s.name              AS schema_name,
    t.name              AS table_name,
    SUM(p.rows)         AS row_count
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
JOIN sys.partitions p ON p.object_id = t.object_id AND p.index_id IN (0, 1)
WHERE s.name = @schema AND t.name = @table
GROUP BY s.name, t.name`
	row, err := s.conn.QueryRow(ctx, sql, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("table %s.%s not found", schema, object)
	}
	return row, nil
}

// TableStorageInfo returns storage statistics (space used, index space) for a
// table.
func (s *Service) TableStorageInfo(ctx context.Context, table string) (map[string]any, error) {
	schema, object := splitObjectName(table, "dbo")
	const sql = `
SELECT
    s.name                                          AS schema_name,
    t.name                                          AS table_name,
    SUM(CASE WHEN p.index_id IN (0,1) THEN p.rows ELSE 0 END) AS row_count,
    SUM(a.total_pages) * 8                           AS total_space_kb,
    SUM(a.used_pages) * 8                            AS used_space_kb,
    (SUM(a.total_pages) - SUM(a.used_pages)) * 8     AS unused_space_kb,
    SUM(CASE WHEN p.index_id > 1 THEN a.used_pages ELSE 0 END) * 8 AS index_space_kb
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
JOIN sys.partitions p ON p.object_id = t.object_id
JOIN sys.allocation_units a ON p.partition_id = a.container_id
WHERE s.name = @schema AND t.name = @table
GROUP BY s.name, t.name`
	row, err := s.conn.QueryRow(ctx, sql, sqlParam("@schema", schema), sqlParam("@table", object))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("table %s.%s not found", schema, object)
	}
	return row, nil
}
