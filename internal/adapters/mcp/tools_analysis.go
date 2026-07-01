package mcp

import "context"

// Additional input types for relationship, profiling, performance, security,
// migration and monitoring tools.

type erdInput struct {
	Table string `json:"table" jsonschema_description:"Root table name, optionally schema-qualified." jsonschema:"required"`
	Depth int    `json:"depth,omitempty" jsonschema_description:"How many foreign-key hops to traverse from the root table (default 2)."`
}

type tableLimitInput struct {
	Table string `json:"table" jsonschema_description:"Table name, optionally schema-qualified." jsonschema:"required"`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum number of rows to return (default 100, hard-capped)."`
}

type columnProfileInput struct {
	Table  string `json:"table" jsonschema_description:"Table name, optionally schema-qualified." jsonschema:"required"`
	Column string `json:"column" jsonschema_description:"Column name." jsonschema:"required"`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum number of distinct values to return (default 100)."`
}

type duplicateRowsInput struct {
	Table   string   `json:"table" jsonschema_description:"Table name, optionally schema-qualified." jsonschema:"required"`
	Columns []string `json:"columns,omitempty" jsonschema_description:"Columns to group by when detecting duplicates. Defaults to all columns."`
	Limit   int      `json:"limit,omitempty" jsonschema_description:"Maximum number of duplicate groups to return."`
}

type limitInput struct {
	Limit int `json:"limit,omitempty" jsonschema_description:"Maximum number of rows to return."`
}

type userInput struct {
	User string `json:"user" jsonschema_description:"Database user name." jsonschema:"required"`
}

// registerAnalysisTools registers the relationship-graph, search, data
// profiling, performance, security, migration-discovery and monitoring tools.
func (s *MCPServer) registerAnalysisTools() {
	svc := s.service
	srv := s.server

	// --- Relationship graphs & dependency analysis ---

	registerTool(srv, "generate_erd",
		"Generate an entity-relationship description rooted at a table, traversing foreign keys outward to the given depth.",
		func(ctx context.Context, in erdInput) (any, error) {
			return svc.GenerateERD(ctx, in.Table, in.Depth)
		})

	registerTool(srv, "dependency_graph",
		"Build a recursive dependency graph for a database object using the engine dependency catalog.",
		func(ctx context.Context, in objectInput) (any, error) {
			return svc.DependencyGraph(ctx, in.Object)
		})

	registerTool(srv, "find_column_usage",
		"Find the modules that reference a specific column.",
		func(ctx context.Context, in columnInput) (any, error) {
			return svc.FindColumnUsage(ctx, in.Table, in.Column)
		})

	registerTool(srv, "find_unused_tables",
		"Find tables not referenced by any module and with no inbound foreign keys (candidate dead tables).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.FindUnusedTables(ctx)
			return list("tables", rows), err
		})

	registerTool(srv, "find_unused_columns",
		"Find columns of a table not referenced by any tracked module dependency (heuristic).",
		func(ctx context.Context, in tableInput) (any, error) {
			rows, err := svc.FindUnusedColumns(ctx, in.Table)
			return list("columns", rows), err
		})

	registerTool(srv, "find_orphan_tables",
		"Find tables with no foreign keys at all (not connected to the relational graph).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.FindOrphanTables(ctx)
			return list("tables", rows), err
		})

	registerTool(srv, "find_circular_dependencies",
		"Detect cycles in the foreign-key graph.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.FindCircularDependencies(ctx)
		})

	// --- Constraints ---

	registerTool(srv, "list_unique_constraints",
		"List unique constraints and their key columns, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListUniqueConstraints(ctx, in.Schema)
			return list("uniqueConstraints", rows), err
		})

	registerTool(srv, "list_check_constraints",
		"List CHECK constraints and their definitions, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListCheckConstraints(ctx, in.Schema)
			return list("checkConstraints", rows), err
		})

	registerTool(srv, "list_default_constraints",
		"List DEFAULT constraints and their definitions, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListDefaultConstraints(ctx, in.Schema)
			return list("defaultConstraints", rows), err
		})

	// --- Search ---

	registerTool(srv, "search_tables",
		"Find tables whose name matches a keyword.",
		func(ctx context.Context, in keywordInput) (any, error) {
			rows, err := svc.SearchTables(ctx, in.Keyword)
			return list("tables", rows), err
		})

	registerTool(srv, "search_columns",
		"Find columns whose name matches a keyword, across all tables and views.",
		func(ctx context.Context, in keywordInput) (any, error) {
			rows, err := svc.SearchColumns(ctx, in.Keyword)
			return list("columns", rows), err
		})

	registerTool(srv, "search_views",
		"Find views whose name matches a keyword.",
		func(ctx context.Context, in keywordInput) (any, error) {
			rows, err := svc.SearchViews(ctx, in.Keyword)
			return list("views", rows), err
		})

	registerTool(srv, "search_functions",
		"Find functions whose name matches a keyword.",
		func(ctx context.Context, in keywordInput) (any, error) {
			rows, err := svc.SearchFunctions(ctx, in.Keyword)
			return list("functions", rows), err
		})

	// --- Data profiling ---

	registerTool(srv, "table_statistics",
		"Return row count, column count and storage statistics for a table.",
		func(ctx context.Context, in tableInput) (any, error) {
			return svc.TableStatistics(ctx, in.Table)
		})

	registerTool(srv, "column_statistics",
		"Compute min/max/distinct/null statistics for a column.",
		func(ctx context.Context, in columnInput) (any, error) {
			return svc.ColumnStatistics(ctx, in.Table, in.Column)
		})

	registerTool(srv, "column_distinct_values",
		"Return the most frequent distinct values for a column with their frequencies.",
		func(ctx context.Context, in columnProfileInput) (any, error) {
			rows, err := svc.ColumnDistinctValues(ctx, in.Table, in.Column, in.Limit)
			return list("values", rows), err
		})

	registerTool(srv, "column_null_analysis",
		"Report the null count and percentage for a column.",
		func(ctx context.Context, in columnInput) (any, error) {
			return svc.ColumnNullAnalysis(ctx, in.Table, in.Column)
		})

	registerTool(srv, "data_distribution",
		"Return the value-frequency distribution of a column (top N).",
		func(ctx context.Context, in columnProfileInput) (any, error) {
			return svc.DataDistribution(ctx, in.Table, in.Column, in.Limit)
		})

	registerTool(srv, "sample_table_data",
		"Return a small sample of rows from a table (read-only, row-capped).",
		func(ctx context.Context, in tableLimitInput) (any, error) {
			return svc.SampleTableData(ctx, in.Table, in.Limit)
		})

	registerTool(srv, "find_duplicate_rows",
		"Find groups of duplicate rows based on the given columns (or all columns).",
		func(ctx context.Context, in duplicateRowsInput) (any, error) {
			rows, err := svc.FindDuplicateRows(ctx, in.Table, in.Columns, in.Limit)
			return list("duplicates", rows), err
		})

	registerTool(srv, "find_candidate_keys",
		"Identify single-column candidate keys by testing each column for uniqueness.",
		func(ctx context.Context, in tableInput) (any, error) {
			return svc.FindCandidateKeys(ctx, in.Table)
		})

	// --- Performance analysis ---

	registerTool(srv, "explain_query",
		"Return the estimated XML execution plan for a read-only query (does not execute it).",
		func(ctx context.Context, in queryInput) (any, error) {
			return svc.ExplainQuery(ctx, in.Query)
		})

	registerTool(srv, "estimate_query_cost",
		"Return the estimated subtree cost of a read-only query from its execution plan.",
		func(ctx context.Context, in queryInput) (any, error) {
			return svc.EstimateQueryCost(ctx, in.Query)
		})

	registerTool(srv, "analyze_execution_plan",
		"Return the estimated execution plan plus a summary of the physical operators used.",
		func(ctx context.Context, in queryInput) (any, error) {
			return svc.AnalyzeExecutionPlan(ctx, in.Query)
		})

	registerTool(srv, "find_missing_indexes",
		"Report indexes the optimizer believes would help, from the missing-index DMVs.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.FindMissingIndexes(ctx)
			return list("missingIndexes", rows), err
		})

	registerTool(srv, "find_unused_indexes",
		"Report non-key indexes that are never read but incur write maintenance.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.FindUnusedIndexes(ctx)
			return list("unusedIndexes", rows), err
		})

	registerTool(srv, "find_fragmented_indexes",
		"Report indexes with high average fragmentation.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.FindFragmentedIndexes(ctx)
			return list("fragmentedIndexes", rows), err
		})

	registerTool(srv, "index_usage_report",
		"Return read/write usage statistics for all indexes.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.IndexUsageReport(ctx)
			return list("indexes", rows), err
		})

	registerTool(srv, "slow_query_analysis",
		"Return the most expensive cached queries by average elapsed time.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.SlowQueryAnalysis(ctx)
			return list("queries", rows), err
		})

	registerTool(srv, "query_performance_report",
		"Return aggregate query performance statistics from the plan cache.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.QueryPerformanceReport(ctx)
			return list("queries", rows), err
		})

	// --- Security analysis ---

	registerTool(srv, "list_logins",
		"List server-level logins (may require elevated permissions).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.ListLogins(ctx)
			return list("logins", rows), err
		})

	registerTool(srv, "list_users",
		"List database-level users.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.ListUsers(ctx)
			return list("users", rows), err
		})

	registerTool(srv, "list_roles",
		"List database roles.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.ListRoles(ctx)
			return list("roles", rows), err
		})

	registerTool(srv, "list_permissions",
		"List all database-level permission grants.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.ListPermissions(ctx)
			return list("permissions", rows), err
		})

	registerTool(srv, "user_permissions",
		"List the permissions granted to a specific database user.",
		func(ctx context.Context, in userInput) (any, error) {
			rows, err := svc.UserPermissions(ctx, in.User)
			return list("permissions", rows), err
		})

	registerTool(srv, "role_memberships",
		"Return the database role-membership graph.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.RoleMemberships(ctx)
			return list("memberships", rows), err
		})

	registerTool(srv, "object_permissions",
		"List permission grants on a specific object.",
		func(ctx context.Context, in objectInput) (any, error) {
			rows, err := svc.ObjectPermissions(ctx, in.Object)
			return list("permissions", rows), err
		})

	// --- Migration discovery ---

	registerTool(srv, "database_overview",
		"Return high-level object counts for the database.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.DatabaseOverview(ctx)
		})

	registerTool(srv, "database_health_report",
		"Return health indicators: heap tables, tables without primary keys and untrusted foreign keys.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.DatabaseHealthReport(ctx)
		})

	registerTool(srv, "database_complexity_report",
		"Return structural complexity metrics for the schema.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.DatabaseComplexityReport(ctx)
		})

	registerTool(srv, "analyze_database_for_migration",
		"Produce a comprehensive migration assessment: object inventory, large tables, high-risk objects, priority order and complexity score.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.AnalyzeDatabaseForMigration(ctx)
		})

	registerTool(srv, "migration_complexity_score",
		"Compute a weighted migration complexity score and rating.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.MigrationComplexityScore(ctx)
		})

	registerTool(srv, "migration_priority_order",
		"Propose an ordering for migrating tables based on foreign-key dependencies.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.MigrationPriorityOrder(ctx)
			return list("tables", rows), err
		})

	registerTool(srv, "high_risk_objects",
		"List modules using migration-risky constructs (dynamic SQL, cursors, xp_cmdshell, OPENQUERY, very large bodies).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.HighRiskObjects(ctx)
			return list("objects", rows), err
		})

	registerTool(srv, "large_table_analysis",
		"List the largest tables by storage and row count.",
		func(ctx context.Context, in limitInput) (any, error) {
			rows, err := svc.LargeTableAnalysis(ctx, in.Limit)
			return list("tables", rows), err
		})

	registerTool(srv, "legacy_object_analysis",
		"List objects with how long since they were last modified, surfacing stale/legacy code.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.LegacyObjectAnalysis(ctx)
			return list("objects", rows), err
		})

	registerTool(srv, "deprecated_feature_detection",
		"Scan module definitions for deprecated T-SQL features that need attention before migration.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			rows, err := svc.DeprecatedFeatureDetection(ctx)
			return list("findings", rows), err
		})

	// --- Monitoring ---

	registerTool(srv, "connection_pool_stats",
		"Return the current connection pool statistics.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.ConnectionPoolStats(ctx)
		})

	registerTool(srv, "database_size_report",
		"Return the data/log file sizes of the current database.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return svc.DatabaseSizeReport(ctx)
		})

	registerTool(srv, "table_growth_report",
		"Return the largest tables by current size (proxy for growth analysis).",
		func(ctx context.Context, in limitInput) (any, error) {
			rows, err := svc.TableGrowthReport(ctx, in.Limit)
			return list("tables", rows), err
		})
}
