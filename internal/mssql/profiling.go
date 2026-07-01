package mssql

import (
	"context"
	"fmt"
	"strings"
)

// defaultSampleLimit caps the number of rows returned by sampling tools.
const defaultSampleLimit = 100

// TableStatistics returns row count, column count and storage statistics for a
// table.
func (s *Service) TableStatistics(ctx context.Context, table string) (map[string]any, error) {
	storage, err := s.TableStorageInfo(ctx, table)
	if err != nil {
		return nil, err
	}
	columns, err := s.ListColumns(ctx, table)
	if err != nil {
		return nil, err
	}
	storage["columnCount"] = len(columns)
	return storage, nil
}

// ColumnStatistics computes min/max/avg/distinct/null statistics for a column.
func (s *Service) ColumnStatistics(ctx context.Context, table, column string) (map[string]any, error) {
	qualified, err := s.qualifiedTable(table)
	if err != nil {
		return nil, err
	}
	qcol, err := quoteName(column)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf(`
SELECT
    COUNT(*)                                   AS total_rows,
    COUNT(%[1]s)                               AS non_null_count,
    COUNT(*) - COUNT(%[1]s)                    AS null_count,
    COUNT(DISTINCT %[1]s)                      AS distinct_count,
    CONVERT(NVARCHAR(4000), MIN(%[1]s))        AS min_value,
    CONVERT(NVARCHAR(4000), MAX(%[1]s))        AS max_value
FROM %[2]s`, qcol, qualified)
	row, err := s.conn.QueryRow(ctx, sql)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	row["table"] = table
	row["column"] = column
	return row, nil
}

// ColumnDistinctValues returns the most frequent distinct values for a column.
func (s *Service) ColumnDistinctValues(ctx context.Context, table, column string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > defaultSampleLimit {
		limit = defaultSampleLimit
	}
	qualified, err := s.qualifiedTable(table)
	if err != nil {
		return nil, err
	}
	qcol, err := quoteName(column)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf(`
SELECT TOP (%[3]d)
    CONVERT(NVARCHAR(4000), %[1]s) AS value,
    COUNT(*) AS frequency
FROM %[2]s
GROUP BY %[1]s
ORDER BY COUNT(*) DESC`, qcol, qualified, limit)
	return s.rows(ctx, sql)
}

// ColumnNullAnalysis reports the null count and percentage for a column.
func (s *Service) ColumnNullAnalysis(ctx context.Context, table, column string) (map[string]any, error) {
	qualified, err := s.qualifiedTable(table)
	if err != nil {
		return nil, err
	}
	qcol, err := quoteName(column)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf(`
SELECT
    COUNT(*)                AS total_rows,
    COUNT(*) - COUNT(%[1]s) AS null_count,
    CASE WHEN COUNT(*) = 0 THEN 0
         ELSE CAST((COUNT(*) - COUNT(%[1]s)) * 100.0 / COUNT(*) AS DECIMAL(5,2))
    END                     AS null_percentage
FROM %[2]s`, qcol, qualified)
	row, err := s.conn.QueryRow(ctx, sql)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = map[string]any{}
	}
	row["table"] = table
	row["column"] = column
	return row, nil
}

// DataDistribution returns the value-frequency distribution of a column (top N).
func (s *Service) DataDistribution(ctx context.Context, table, column string, limit int) (map[string]any, error) {
	values, err := s.ColumnDistinctValues(ctx, table, column, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"table":        table,
		"column":       column,
		"distribution": values,
	}, nil
}

// SampleTableData returns a small sample of rows from a table.
func (s *Service) SampleTableData(ctx context.Context, table string, limit int) (*queryResultLike, error) {
	if limit <= 0 || limit > defaultSampleLimit {
		limit = defaultSampleLimit
	}
	qualified, err := s.qualifiedTable(table)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf(`SELECT TOP (%d) * FROM %s`, limit, qualified)
	res, err := s.query(ctx, sql)
	if err != nil {
		return nil, err
	}
	return &queryResultLike{Columns: res.Columns, Rows: res.Rows, RowCount: res.RowCount}, nil
}

// FindDuplicateRows finds groups of duplicate rows based on the given columns
// (or all columns when none are supplied).
func (s *Service) FindDuplicateRows(ctx context.Context, table string, columns []string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > defaultSampleLimit {
		limit = defaultSampleLimit
	}
	qualified, err := s.qualifiedTable(table)
	if err != nil {
		return nil, err
	}

	if len(columns) == 0 {
		cols, err := s.ListColumns(ctx, table)
		if err != nil {
			return nil, err
		}
		for _, c := range cols {
			if name, ok := c["column_name"].(string); ok {
				columns = append(columns, name)
			}
		}
	}

	quoted := make([]string, 0, len(columns))
	for _, c := range columns {
		qc, err := quoteName(c)
		if err != nil {
			return nil, err
		}
		quoted = append(quoted, qc)
	}
	colList := strings.Join(quoted, ", ")

	sql := fmt.Sprintf(`
SELECT TOP (%[3]d) %[1]s, COUNT(*) AS duplicate_count
FROM %[2]s
GROUP BY %[1]s
HAVING COUNT(*) > 1
ORDER BY COUNT(*) DESC`, colList, qualified, limit)
	return s.rows(ctx, sql)
}

// FindCandidateKeys checks each non-nullable column to see whether it is unique
// across the table, identifying single-column candidate keys.
func (s *Service) FindCandidateKeys(ctx context.Context, table string) (map[string]any, error) {
	qualified, err := s.qualifiedTable(table)
	if err != nil {
		return nil, err
	}
	cols, err := s.ListColumns(ctx, table)
	if err != nil {
		return nil, err
	}

	totalAny, err := s.scalar(ctx, fmt.Sprintf(`SELECT COUNT(*) AS c FROM %s`, qualified))
	if err != nil {
		return nil, err
	}
	total := toInt64(totalAny)

	var candidates []string
	for _, c := range cols {
		name, _ := c["column_name"].(string)
		if name == "" {
			continue
		}
		qc, err := quoteName(name)
		if err != nil {
			continue
		}
		distinctAny, err := s.scalar(ctx, fmt.Sprintf(`SELECT COUNT(DISTINCT %s) FROM %s WHERE %s IS NOT NULL`, qc, qualified, qc))
		if err != nil {
			return nil, err
		}
		nullsAny, err := s.scalar(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s IS NULL`, qualified, qc))
		if err != nil {
			return nil, err
		}
		if toInt64(distinctAny) == total && toInt64(nullsAny) == 0 && total > 0 {
			candidates = append(candidates, name)
		}
	}
	if candidates == nil {
		candidates = []string{}
	}
	return map[string]any{
		"table":          table,
		"rowCount":       total,
		"candidateKeys":  candidates,
	}, nil
}

// qualifiedTable validates and returns the bracket-quoted schema.table form.
func (s *Service) qualifiedTable(table string) (string, error) {
	schema, object := splitObjectName(table, "dbo")
	return quoteQualified(schema, object)
}

// queryResultLike mirrors db.QueryResult so profiling tools can return the same
// columns/rows/rowCount shape without importing the db package's concrete type
// into tool signatures.
type queryResultLike struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"rowCount"`
}

// toInt64 best-effort converts a driver numeric value to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
