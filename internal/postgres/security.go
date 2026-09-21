package postgres

import "context"

// ListLogins returns database roles that have LOGIN privileges.
func (s *Service) ListLogins(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT rolname       AS login_name,
       rolcanlogin   AS can_login,
       rolsuper      AS is_superuser,
       rolcreatedb   AS can_create_db,
       rolcreaterole AS can_create_role
FROM   pg_roles
WHERE  rolcanlogin = true
ORDER  BY rolname`)
}

// ListUsers returns database users (roles that can log in).
func (s *Service) ListUsers(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT rolname       AS user_name,
       rolcanlogin   AS can_login,
       rolsuper      AS is_superuser,
       rolcreatedb   AS can_create_db,
       rolcreaterole AS can_create_role,
       rolconnlimit  AS connection_limit
FROM   pg_roles
WHERE  rolcanlogin = true
ORDER  BY rolname`)
}

// ListRoles returns group roles (roles that cannot log in directly).
func (s *Service) ListRoles(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT rolname       AS role_name,
       rolsuper      AS is_superuser,
       rolinherit    AS inherits_privileges,
       rolcreaterole AS can_create_role
FROM   pg_roles
WHERE  rolcanlogin = false
ORDER  BY rolname`)
}

// ListPermissions returns all database-level table permissions.
func (s *Service) ListPermissions(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT grantee,
       table_schema   AS object_schema,
       table_name     AS object_name,
       privilege_type AS permission,
       is_grantable
FROM   information_schema.table_privileges
WHERE  table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER  BY grantee, table_schema, table_name, privilege_type`)
}

// UserPermissions returns permissions granted to a specific user.
func (s *Service) UserPermissions(ctx context.Context, user string) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT grantee,
       table_schema   AS object_schema,
       table_name     AS object_name,
       privilege_type AS permission,
       is_grantable
FROM   information_schema.table_privileges
WHERE  grantee = $1
ORDER  BY table_schema, table_name, privilege_type`, user)
}

// RoleMemberships returns the role membership relationships.
func (s *Service) RoleMemberships(ctx context.Context) ([]map[string]any, error) {
	return s.rows(ctx, `
SELECT r.rolname AS role_name,
       m.rolname AS member_name,
       a.admin_option AS is_admin
FROM   pg_auth_members a
JOIN   pg_roles r ON r.oid = a.roleid
JOIN   pg_roles m ON m.oid = a.member
ORDER  BY r.rolname, m.rolname`)
}

// ObjectPermissions returns permission grants on a specific object.
func (s *Service) ObjectPermissions(ctx context.Context, object string) ([]map[string]any, error) {
	schema, name := splitObjectName(object, "public")
	return s.rows(ctx, `
SELECT grantee,
       table_schema   AS object_schema,
       table_name     AS object_name,
       privilege_type AS permission,
       is_grantable
FROM   information_schema.table_privileges
WHERE  table_schema = $1 AND table_name = $2
ORDER  BY grantee, privilege_type`, schema, name)
}
