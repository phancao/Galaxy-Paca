package authz

// Permission is a stable machine-readable permission key.
type Permission string

// Stable permission keys used by the authorization system.
const (
	PermissionAll Permission = "*"

	PermissionUsersRead   Permission = "users.read"
	PermissionUsersWrite  Permission = "users.write"
	PermissionUsersDelete Permission = "users.delete"
	PermissionUsersAll    Permission = "users.*"

	PermissionGlobalRolesRead   Permission = "global_roles.read"
	PermissionGlobalRolesWrite  Permission = "global_roles.write"
	PermissionGlobalRolesAssign Permission = "global_roles.assign"
	PermissionGlobalRolesAll    Permission = "global_roles.*"

	PermissionProjectsRead   Permission = "projects.read"
	PermissionProjectsWrite  Permission = "projects.write"
	PermissionProjectsCreate Permission = "projects.create"
	PermissionProjectsDelete Permission = "projects.delete"
	PermissionProjectsAll    Permission = "projects.*"

	PermissionProjectMembersRead  Permission = "project.members.read"
	PermissionProjectMembersWrite Permission = "project.members.write"
	PermissionProjectMembersAll   Permission = "project.members.*"

	PermissionProjectRolesRead  Permission = "project.roles.read"
	PermissionProjectRolesWrite Permission = "project.roles.write"
	PermissionProjectRolesAll   Permission = "project.roles.*"

	PermissionTasksRead  Permission = "tasks.read"
	PermissionTasksWrite Permission = "tasks.write"
	PermissionTasksAll   Permission = "tasks.*"

	PermissionSprintsRead  Permission = "sprints.read"
	PermissionSprintsWrite Permission = "sprints.write"
	PermissionSprintsAll   Permission = "sprints.*"

	PermissionDocsRead  Permission = "docs.read"
	PermissionDocsWrite Permission = "docs.write"
	PermissionDocsAll   Permission = "docs.*"

	PermissionAgentsRead  Permission = "agents.read"
	PermissionAgentsWrite Permission = "agents.write"
	PermissionAgentsAll   Permission = "agents.*"

	PermissionWorkflowsRead  Permission = "workflows.read"
	PermissionWorkflowsWrite Permission = "workflows.write"
	PermissionWorkflowsAll   Permission = "workflows.*"
)

// allPermissions is the complete built-in permission vocabulary. It is the
// single source of truth for "is this a permission key the host knows about?".
//
// NOTE: every Permission constant declared above must appear here. The gate
// test TestAllPermissions_CoversEveryDeclaredConstant parses this file's AST
// and fails when a constant is declared but missing from this slice, so the
// list cannot silently fall behind.
var allPermissions = []Permission{
	PermissionAll,

	PermissionUsersRead,
	PermissionUsersWrite,
	PermissionUsersDelete,
	PermissionUsersAll,

	PermissionGlobalRolesRead,
	PermissionGlobalRolesWrite,
	PermissionGlobalRolesAssign,
	PermissionGlobalRolesAll,

	PermissionProjectsRead,
	PermissionProjectsWrite,
	PermissionProjectsCreate,
	PermissionProjectsDelete,
	PermissionProjectsAll,

	PermissionProjectMembersRead,
	PermissionProjectMembersWrite,
	PermissionProjectMembersAll,

	PermissionProjectRolesRead,
	PermissionProjectRolesWrite,
	PermissionProjectRolesAll,

	PermissionTasksRead,
	PermissionTasksWrite,
	PermissionTasksAll,

	PermissionSprintsRead,
	PermissionSprintsWrite,
	PermissionSprintsAll,

	PermissionDocsRead,
	PermissionDocsWrite,
	PermissionDocsAll,

	PermissionAgentsRead,
	PermissionAgentsWrite,
	PermissionAgentsAll,

	PermissionWorkflowsRead,
	PermissionWorkflowsWrite,
	PermissionWorkflowsAll,
}

// builtinPermissionIndex is the lookup form of allPermissions.
var builtinPermissionIndex = func() map[Permission]struct{} {
	m := make(map[Permission]struct{}, len(allPermissions))
	for _, p := range allPermissions {
		m[p] = struct{}{}
	}
	return m
}()

// AllPermissions returns a copy of the built-in permission vocabulary.
// The copy keeps callers from mutating the package-level source of truth.
func AllPermissions() []Permission {
	out := make([]Permission, len(allPermissions))
	copy(out, allPermissions)
	return out
}

// IsBuiltinPermission reports whether key is one of the host's built-in
// permission keys. It is an exact-membership test: it does NOT expand
// wildcards, so "projects.*" is known (it is a declared constant) while
// "projects.anything" is not.
func IsBuiltinPermission(key Permission) bool {
	_, ok := builtinPermissionIndex[key]
	return ok
}
