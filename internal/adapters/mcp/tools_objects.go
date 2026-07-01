package mcp

import "context"

// Input types for stored procedure, view, function and trigger tools.

type procedureInput struct {
	Procedure string `json:"procedure" jsonschema_description:"Stored procedure name, optionally schema-qualified (e.g. dbo.MyProc)." jsonschema:"required"`
}

type viewInput struct {
	View string `json:"view" jsonschema_description:"View name, optionally schema-qualified." jsonschema:"required"`
}

type functionInput struct {
	Function string `json:"function" jsonschema_description:"Function name, optionally schema-qualified." jsonschema:"required"`
}

type triggerInput struct {
	Trigger string `json:"trigger" jsonschema_description:"Trigger name, optionally schema-qualified." jsonschema:"required"`
}

type keywordInput struct {
	Keyword string `json:"keyword" jsonschema_description:"Keyword to search for." jsonschema:"required"`
}

type objectInput struct {
	Object string `json:"object" jsonschema_description:"Database object name (procedure/view/function/table), optionally schema-qualified." jsonschema:"required"`
}

// registerObjectTools registers the stored procedure, view, function and trigger
// tools: definitions, search and static analysis (tables/views/functions/
// procedures used, transactions, dynamic SQL, cursors, temp tables, error
// handling, validation and business rules).
func (s *MCPServer) registerObjectTools() {
	svc := s.service
	srv := s.server

	// --- Stored procedures ---

	registerTool(srv, "list_procedures",
		"List all stored procedures, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListProcedures(ctx, in.Schema)
			return list("procedures", rows), err
		})

	registerTool(srv, "get_procedure_definition",
		"Return the full T-SQL definition and parameters of a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.GetProcedureDefinition(ctx, in.Procedure)
		})

	registerTool(srv, "search_procedures",
		"Find stored procedures whose definition contains the given keyword.",
		func(ctx context.Context, in keywordInput) (any, error) {
			rows, err := svc.SearchProcedures(ctx, in.Keyword)
			return list("procedures", rows), err
		})

	registerTool(srv, "analyze_stored_procedure",
		"Statically analyze a stored procedure: tables/views/functions used, called procedures, validation and business rules, transaction usage, dynamic SQL, cursors, temp tables and error handling.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.AnalyzeStoredProcedure(ctx, in.Procedure)
		})

	registerTool(srv, "extract_tables_used",
		"Extract the tables referenced by a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractTablesUsed(ctx, in.Procedure)
		})

	registerTool(srv, "extract_views_used",
		"Extract the views referenced by a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractViewsUsed(ctx, in.Procedure)
		})

	registerTool(srv, "extract_functions_used",
		"Extract the functions referenced by a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractFunctionsUsed(ctx, in.Procedure)
		})

	registerTool(srv, "extract_called_procedures",
		"Extract the procedures called (via EXEC) by a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractCalledProcedures(ctx, in.Procedure)
		})

	registerTool(srv, "extract_transaction_usage",
		"Extract transaction control usage (BEGIN/COMMIT/ROLLBACK/SAVE) from a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractTransactionUsage(ctx, in.Procedure)
		})

	registerTool(srv, "extract_dynamic_sql",
		"Detect dynamic SQL usage (EXEC(...) / sp_executesql) in a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractDynamicSQL(ctx, in.Procedure)
		})

	registerTool(srv, "extract_cursor_usage",
		"Extract cursor declarations from a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractCursorUsage(ctx, in.Procedure)
		})

	registerTool(srv, "extract_temp_tables",
		"Extract temp table (#temp / ##global) usage from a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractTempTables(ctx, in.Procedure)
		})

	registerTool(srv, "extract_error_handling",
		"Extract error handling constructs (TRY/CATCH, RAISERROR, THROW, ERROR_* functions) from a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractErrorHandling(ctx, in.Procedure)
		})

	registerTool(srv, "extract_validation_rules",
		"Extract validation rules (IF ... RAISERROR/THROW/RETURN) from a stored procedure.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractValidationRules(ctx, in.Procedure)
		})

	registerTool(srv, "extract_business_rules",
		"Extract business-rule comments from a stored procedure definition.",
		func(ctx context.Context, in procedureInput) (any, error) {
			return svc.ExtractBusinessRules(ctx, in.Procedure)
		})

	// --- Views ---

	registerTool(srv, "list_views",
		"List all views, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListViews(ctx, in.Schema)
			return list("views", rows), err
		})

	registerTool(srv, "get_view_definition",
		"Return the SQL definition of a view.",
		func(ctx context.Context, in viewInput) (any, error) {
			return svc.GetViewDefinition(ctx, in.View)
		})

	registerTool(srv, "analyze_view",
		"Analyze a view: exposed columns and the tables/views/functions it uses.",
		func(ctx context.Context, in viewInput) (any, error) {
			return svc.AnalyzeView(ctx, in.View)
		})

	registerTool(srv, "trace_view_sources",
		"Resolve the underlying base tables of a view, following nested views recursively.",
		func(ctx context.Context, in viewInput) (any, error) {
			return svc.TraceViewSources(ctx, in.View)
		})

	registerTool(srv, "extract_view_dependencies",
		"List the objects a view depends on (from the engine dependency catalog).",
		func(ctx context.Context, in viewInput) (any, error) {
			return svc.ExtractViewDependencies(ctx, in.View)
		})

	// --- Functions ---

	registerTool(srv, "list_functions",
		"List all user-defined functions (scalar, inline and table-valued), optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListFunctions(ctx, in.Schema)
			return list("functions", rows), err
		})

	registerTool(srv, "get_function_definition",
		"Return the T-SQL definition and parameters of a function.",
		func(ctx context.Context, in functionInput) (any, error) {
			return svc.GetFunctionDefinition(ctx, in.Function)
		})

	registerTool(srv, "analyze_function",
		"Statically analyze a function: tables/views/functions used and error handling.",
		func(ctx context.Context, in functionInput) (any, error) {
			return svc.AnalyzeFunction(ctx, in.Function)
		})

	registerTool(srv, "trace_function_usage",
		"Find every object that references the given function.",
		func(ctx context.Context, in functionInput) (any, error) {
			return svc.TraceFunctionUsage(ctx, in.Function)
		})

	registerTool(srv, "extract_function_dependencies",
		"List the objects a function depends on.",
		func(ctx context.Context, in functionInput) (any, error) {
			return svc.ExtractFunctionDependencies(ctx, in.Function)
		})

	// --- Triggers ---

	registerTool(srv, "list_triggers",
		"List all DML triggers with the table they are attached to, optionally filtered by schema.",
		func(ctx context.Context, in schemaInput) (any, error) {
			rows, err := svc.ListTriggers(ctx, in.Schema)
			return list("triggers", rows), err
		})

	registerTool(srv, "get_trigger_definition",
		"Return the T-SQL definition of a trigger.",
		func(ctx context.Context, in triggerInput) (any, error) {
			return svc.GetTriggerDefinition(ctx, in.Trigger)
		})

	registerTool(srv, "analyze_trigger",
		"Statically analyze a trigger: tables/views/functions used, transaction usage and error handling.",
		func(ctx context.Context, in triggerInput) (any, error) {
			return svc.AnalyzeTrigger(ctx, in.Trigger)
		})

	// --- Object dependency & search helpers ---

	registerTool(srv, "find_object_dependencies",
		"List the objects a given database object depends on.",
		func(ctx context.Context, in objectInput) (any, error) {
			return svc.FindObjectDependencies(ctx, in.Object)
		})

	registerTool(srv, "find_object_references",
		"List the objects that reference a given database object.",
		func(ctx context.Context, in objectInput) (any, error) {
			return svc.FindObjectReferences(ctx, in.Object)
		})

	registerTool(srv, "find_table_usage",
		"Find every procedure, view, function and trigger that references a table.",
		func(ctx context.Context, in tableInput) (any, error) {
			return svc.FindTableUsage(ctx, in.Table)
		})

	registerTool(srv, "search_objects_by_keyword",
		"Search every object name and module definition for a keyword.",
		func(ctx context.Context, in keywordInput) (any, error) {
			rows, err := svc.SearchObjectsByKeyword(ctx, in.Keyword)
			return list("objects", rows), err
		})
}
