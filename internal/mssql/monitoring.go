package mssql

import "context"

// ConnectionPoolStats returns the current connection pool statistics.
func (s *Service) ConnectionPoolStats(_ context.Context) (map[string]any, error) {
	return s.conn.PoolStats(), nil
}

// DatabaseSizeReport returns the data/log file sizes of the current database.
func (s *Service) DatabaseSizeReport(ctx context.Context) (map[string]any, error) {
	files, err := s.rows(ctx, `
SELECT
    df.type_desc                                            AS file_type,
    df.name                                                 AS logical_name,
    df.physical_name                                        AS physical_name,
    df.size * 8                                             AS size_kb,
    CASE df.max_size WHEN -1 THEN -1 ELSE df.max_size * 8 END AS max_size_kb,
    df.growth                                               AS growth,
    df.is_percent_growth                                    AS is_percent_growth
FROM sys.database_files df
ORDER BY df.type_desc, df.name`)
	if err != nil {
		return nil, err
	}

	total, err := s.scalar(ctx, `SELECT SUM(size) * 8 AS total_kb FROM sys.database_files`)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"database":   s.database,
		"files":      files,
		"totalSizeKb": total,
	}, nil
}

// TableGrowthReport returns the largest tables by current size, used as a proxy
// for growth analysis (SQL Server does not retain per-table growth history by
// default).
func (s *Service) TableGrowthReport(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const sql = `
SELECT TOP (@limit)
    s.name                                                  AS schema_name,
    t.name                                                  AS table_name,
    SUM(CASE WHEN p.index_id IN (0,1) THEN p.rows ELSE 0 END) AS row_count,
    SUM(a.total_pages) * 8                                  AS total_space_kb,
    SUM(a.used_pages) * 8                                   AS used_space_kb,
    t.create_date                                           AS create_date,
    t.modify_date                                           AS modify_date
FROM sys.tables t
JOIN sys.schemas s ON t.schema_id = s.schema_id
JOIN sys.partitions p ON p.object_id = t.object_id
JOIN sys.allocation_units a ON p.partition_id = a.container_id
GROUP BY s.name, t.name, t.create_date, t.modify_date
ORDER BY SUM(a.total_pages) DESC`
	return s.rows(ctx, sql, sqlParam("@limit", limit))
}
