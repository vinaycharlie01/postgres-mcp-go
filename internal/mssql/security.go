package mssql

import "context"

// ListLogins returns server-level logins (requires VIEW ANY DEFINITION / server
// permissions; may be restricted for low-privilege accounts).
func (s *Service) ListLogins(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    sp.name             AS login_name,
    sp.type_desc        AS login_type,
    sp.is_disabled      AS is_disabled,
    sp.create_date      AS create_date,
    sp.modify_date      AS modify_date,
    sp.default_database_name AS default_database
FROM sys.server_principals sp
WHERE sp.type IN ('S','U','G')
ORDER BY sp.name`
	return s.rows(ctx, sql)
}

// ListUsers returns database-level users.
func (s *Service) ListUsers(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    dp.name             AS user_name,
    dp.type_desc        AS user_type,
    dp.authentication_type_desc AS authentication_type,
    dp.create_date      AS create_date,
    dp.default_schema_name AS default_schema
FROM sys.database_principals dp
WHERE dp.type IN ('S','U','G','E','X')
  AND dp.name NOT IN ('guest','INFORMATION_SCHEMA','sys')
ORDER BY dp.name`
	return s.rows(ctx, sql)
}

// ListRoles returns database roles.
func (s *Service) ListRoles(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    dp.name         AS role_name,
    dp.type_desc    AS role_type,
    dp.is_fixed_role AS is_fixed_role,
    dp.create_date  AS create_date
FROM sys.database_principals dp
WHERE dp.type = 'R'
ORDER BY dp.name`
	return s.rows(ctx, sql)
}

// ListPermissions returns all database-level permission grants.
func (s *Service) ListPermissions(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    dp.name                 AS principal_name,
    perm.class_desc         AS class,
    perm.permission_name    AS permission,
    perm.state_desc         AS state,
    ISNULL(OBJECT_SCHEMA_NAME(perm.major_id), '') AS object_schema,
    ISNULL(OBJECT_NAME(perm.major_id), '')        AS object_name
FROM sys.database_permissions perm
JOIN sys.database_principals dp ON perm.grantee_principal_id = dp.principal_id
ORDER BY dp.name, perm.permission_name`
	return s.rows(ctx, sql)
}

// UserPermissions returns the effective permissions granted to a specific user.
func (s *Service) UserPermissions(ctx context.Context, user string) ([]map[string]any, error) {
	const sql = `
SELECT
    perm.class_desc         AS class,
    perm.permission_name    AS permission,
    perm.state_desc         AS state,
    ISNULL(OBJECT_SCHEMA_NAME(perm.major_id), '') AS object_schema,
    ISNULL(OBJECT_NAME(perm.major_id), '')        AS object_name
FROM sys.database_permissions perm
JOIN sys.database_principals dp ON perm.grantee_principal_id = dp.principal_id
WHERE dp.name = @user
ORDER BY perm.permission_name`
	return s.rows(ctx, sql, sqlParam("@user", user))
}

// RoleMemberships returns the role membership graph (which principals belong to
// which roles).
func (s *Service) RoleMemberships(ctx context.Context) ([]map[string]any, error) {
	const sql = `
SELECT
    r.name      AS role_name,
    m.name      AS member_name,
    m.type_desc AS member_type
FROM sys.database_role_members rm
JOIN sys.database_principals r ON rm.role_principal_id = r.principal_id
JOIN sys.database_principals m ON rm.member_principal_id = m.principal_id
ORDER BY r.name, m.name`
	return s.rows(ctx, sql)
}

// ObjectPermissions returns permission grants on a specific object.
func (s *Service) ObjectPermissions(ctx context.Context, object string) ([]map[string]any, error) {
	schema, name := splitObjectName(object, "dbo")
	const sql = `
SELECT
    dp.name                 AS principal_name,
    perm.permission_name    AS permission,
    perm.state_desc         AS state
FROM sys.database_permissions perm
JOIN sys.database_principals dp ON perm.grantee_principal_id = dp.principal_id
WHERE perm.major_id = OBJECT_ID(QUOTENAME(@schema) + '.' + QUOTENAME(@object))
ORDER BY dp.name, perm.permission_name`
	return s.rows(ctx, sql, sqlParam("@schema", schema), sqlParam("@object", name))
}
