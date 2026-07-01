# 🛠️ Complete Tool Reference

This document provides a comprehensive reference for all tools available in the MSSQL MCP Server.

## 📑 Table of Contents

- [Discovery Tools](#discovery-tools)
- [Analysis Tools](#analysis-tools)
- [Query Tools](#query-tools)
- [Relationship Tools](#relationship-tools)
- [Security Tools](#security-tools)
- [Performance Tools](#performance-tools)
- [Migration Tools](#migration-tools)
- [Search Tools](#search-tools)
- [Utility Tools](#utility-tools)

---

## Discovery Tools

### `list_tables`
Lists all tables in the database with metadata.

**Parameters:**
- `schema` (optional): Filter by schema name

**Returns:**
- Table name, schema, row count, column count, creation date

**Example:**
```json
{
  "schema": "dbo"
}
```

---

### `list_views`
Lists all views in the database.

**Parameters:**
- `schema` (optional): Filter by schema name

**Returns:**
- View name, schema, definition, creation date

---

### `list_procedures`
Lists all stored procedures.

**Parameters:**
- `schema` (optional): Filter by schema name

**Returns:**
- Procedure name, schema, parameter count, dates

---

### `list_functions`
Lists all user-defined functions.

**Parameters:**
- `schema` (optional): Filter by schema name

**Returns:**
- Function name, type, schema, creation date

---

### `list_triggers`
Lists all DML triggers.

**Parameters:**
- `schema` (optional): Filter by schema name

**Returns:**
- Trigger name, table, type, events

---

### `describe_table`
Get detailed structure of a table.

**Parameters:**
- `table` (required): Table name

**Returns:**
- Columns, primary keys, foreign keys, indexes, constraints

---

### `describe_column`
Get detailed metadata for a single column.

**Parameters:**
- `table` (required): Table name
- `column` (required): Column name

**Returns:**
- Data type, precision, nullability, default value

---

## Analysis Tools

### `analyze_database_for_migration`
Comprehensive migration assessment.

**Returns:**
- Object inventory
- Large tables
- High-risk objects
- Migration order
- Complexity score

---

### `database_complexity_report`
Structural complexity metrics.

**Returns:**
- Object counts
- Relationship density
- Complexity indicators

---

### `database_health_report`
Health indicators and issues.

**Returns:**
- Heap tables
- Tables without PKs
- Untrusted FKs
- Disabled indexes

---

### `analyze_stored_procedure`
Static analysis of a stored procedure.

**Parameters:**
- `procedure` (required): Procedure name

**Returns:**
- Dependencies
- Business rules
- Transaction usage
- Dynamic SQL detection

---

### `analyze_function`
Static analysis of a function.

**Parameters:**
- `function` (required): Function name

**Returns:**
- Dependencies
- Error handling
- Complexity metrics

---

### `analyze_trigger`
Static analysis of a trigger.

**Parameters:**
- `trigger` (required): Trigger name

**Returns:**
- Dependencies
- Transaction usage
- Error handling

---

### `analyze_view`
Analyze a view definition.

**Parameters:**
- `view` (required): View name

**Returns:**
- Exposed columns
- Dependencies
- Complexity

---

## Query Tools

### `execute_query`
Execute a read-only SELECT query.

**Parameters:**
- `query` (required): SQL SELECT statement

**Returns:**
- Column names, rows, row count

**Safety:**
- Only SELECT/WITH allowed
- Dangerous operations blocked
- Query timeout enforced

---

### `sample_table_data`
Get sample rows from a table.

**Parameters:**
- `table` (required): Table name
- `limit` (optional): Number of rows (default: 10, max: 100)

**Returns:**
- Sample rows with metadata

---

### `column_statistics`
Compute statistics for a column.

**Parameters:**
- `table` (required): Table name
- `column` (required): Column name

**Returns:**
- Min, max, distinct count, null count

---

### `column_null_analysis`
Analyze null values in a column.

**Parameters:**
- `table` (required): Table name
- `column` (required): Column name

**Returns:**
- Total rows, null count, null percentage

---

### `column_distinct_values`
Get most frequent distinct values.

**Parameters:**
- `table` (required): Table name
- `column` (required): Column name
- `limit` (optional): Number of values (default: 20)

**Returns:**
- Value, frequency, percentage

---

### `data_distribution`
Analyze value distribution.

**Parameters:**
- `table` (required): Table name
- `column` (required): Column name
- `limit` (optional): Number of buckets (default: 10)

**Returns:**
- Value ranges, frequency distribution

---

## Relationship Tools

### `foreign_key_graph`
Get complete FK relationship graph.

**Returns:**
- Nodes (tables), edges (FK relationships)

---

### `dependency_graph`
Build recursive dependency graph for an object.

**Parameters:**
- `object` (required): Object name

**Returns:**
- Dependency tree, depth

---

### `table_relationships`
Show how a table connects to others.

**Parameters:**
- `table` (required): Table name

**Returns:**
- Outbound FKs, inbound FKs

---

### `find_circular_dependencies`
Detect cycles in FK graph.

**Returns:**
- Circular dependency chains

---

### `find_object_dependencies`
List objects a given object depends on.

**Parameters:**
- `object` (required): Object name

**Returns:**
- Dependent objects with types

---

### `find_object_references`
List objects that reference a given object.

**Parameters:**
- `object` (required): Object name

**Returns:**
- Referencing objects with types

---

## Security Tools

### `list_users`
List database users.

**Returns:**
- User name, type, creation date, default schema

---

### `list_roles`
List database roles.

**Returns:**
- Role name, type, members

---

### `list_permissions`
List all permission grants.

**Returns:**
- Grantee, permission type, object name

---

### `user_permissions`
List permissions for a specific user.

**Parameters:**
- `user` (required): User name

**Returns:**
- Permission grants, role memberships

---

### `object_permissions`
List permissions on a specific object.

**Parameters:**
- `object` (required): Object name

**Returns:**
- Users with access, permission types

---

## Performance Tools

### `analyze_execution_plan`
Get estimated execution plan.

**Parameters:**
- `query` (required): SQL query

**Returns:**
- XML execution plan, operators, costs

---

### `estimate_query_cost`
Get estimated subtree cost.

**Parameters:**
- `query` (required): SQL query

**Returns:**
- Estimated cost, breakdown

---

### `find_missing_indexes`
Identify missing indexes.

**Returns:**
- Table, columns, impact score, improvement estimate

---

### `find_unused_indexes`
Find indexes never used.

**Returns:**
- Index name, table, size, write cost

---

### `find_fragmented_indexes`
Report highly fragmented indexes.

**Returns:**
- Index name, fragmentation %, recommended action

---

### `index_usage_report`
Get index usage statistics.

**Returns:**
- Index name, seeks, scans, lookups, updates

---

### `query_performance_report`
Aggregate query performance from plan cache.

**Returns:**
- Query text, execution count, duration, CPU time

---

### `slow_query_analysis`
Find most expensive cached queries.

**Returns:**
- Query text, elapsed time, execution count

---

## Migration Tools

### `migration_priority_order`
Propose table migration order based on FK dependencies.

**Returns:**
- Migration waves, table order, dependencies

---

### `migration_complexity_score`
Compute weighted migration complexity.

**Returns:**
- Overall score (0-100), rating, factors

---

### `high_risk_objects`
List objects with migration-risky constructs.

**Returns:**
- Object name, risk factors, severity

---

### `large_table_analysis`
List largest tables by storage and rows.

**Parameters:**
- `limit` (optional): Number of tables (default: 20)

**Returns:**
- Table name, row count, sizes

---

### `deprecated_feature_detection`
Scan for deprecated T-SQL features.

**Returns:**
- Object name, deprecated feature, replacement

---

## Search Tools

### `search_tables`
Find tables by name pattern.

**Parameters:**
- `keyword` (required): Search keyword

**Returns:**
- Matching tables with metadata

---

### `search_columns`
Find columns by name pattern.

**Parameters:**
- `keyword` (required): Search keyword

**Returns:**
- Table, column, data type

---

### `search_procedures`
Find procedures by name or content.

**Parameters:**
- `keyword` (required): Search keyword

**Returns:**
- Procedure name, match location

---

### `search_objects_by_keyword`
Search all objects for keyword.

**Parameters:**
- `keyword` (required): Search keyword

**Returns:**
- Object name, type, match context

---

## Utility Tools

### `health_check`
Check database connectivity and pool stats.

**Returns:**
- Connection status, pool statistics, server version

---

### `connection_pool_stats`
Get current connection pool statistics.

**Returns:**
- Open/idle/in-use connections, wait stats

---

### `database_size_report`
Get data and log file sizes.

**Returns:**
- Data file size, log file size, free space

---

### `database_overview`
High-level object counts.

**Returns:**
- Counts by object type

---

## Tool Usage Patterns

### Pattern 1: Schema Exploration
```
1. database_overview → Get high-level counts
2. list_tables → See all tables
3. describe_table → Examine specific table
4. table_relationships → Understand connections
```

### Pattern 2: Performance Investigation
```
1. slow_query_analysis → Find slow queries
2. analyze_execution_plan → Understand query plan
3. find_missing_indexes → Get recommendations
4. find_unused_indexes → Remove waste
```

### Pattern 3: Migration Planning
```
1. analyze_database_for_migration → Overall assessment
2. large_table_analysis → Identify big tables
3. high_risk_objects → Find problematic code
4. migration_priority_order → Plan migration waves
5. deprecated_feature_detection → Find issues
```

### Pattern 4: Data Quality Analysis
```
1. sample_table_data → Preview data
2. column_statistics → Get basic stats
3. column_null_analysis → Check completeness
4. data_distribution → Understand patterns
```

---

## Best Practices

### 1. Start Broad, Then Narrow
- Use overview tools first
- Drill down into specific areas
- Use search tools to find relevant objects

### 2. Combine Tools
- Use multiple tools for comprehensive analysis
- Cross-reference results
- Build complete picture

### 3. Performance Considerations
- Use `limit` parameters for large result sets
- Sample data instead of full scans
- Use specific schemas when possible

### 4. Safety
- All tools are read-only
- No schema modifications possible
- Query timeouts enforced
- Connection pooling prevents resource exhaustion

---

## Tool Categories Summary

| Category | Tool Count | Primary Use |
|----------|------------|-------------|
| Discovery | 15+ | Schema exploration |
| Analysis | 20+ | Code and structure analysis |
| Query | 10+ | Data exploration |
| Relationship | 8+ | Dependency mapping |
| Security | 6+ | Access control review |
| Performance | 12+ | Optimization |
| Migration | 8+ | Migration planning |
| Search | 5+ | Finding objects |
| Utility | 5+ | Health and monitoring |

**Total: 89+ tools available**

---

## Need Help?

- 📖 [Main README](../README.md)
- 🐛 [Report Issues](https://github.com/yourusername/mssql-mcp-go/issues)
- 💬 [Discussions](https://github.com/yourusername/mssql-mcp-go/discussions)