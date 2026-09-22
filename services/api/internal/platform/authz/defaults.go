package authz

import "strings"

// RoleDefinition binds a role name to the permissions it grants.
type RoleDefinition struct {
	Name        string
	Permissions []Permission
}

// DefaultGlobalRoles returns the built-in global role set.
func DefaultGlobalRoles() []RoleDefinition {
	return []RoleDefinition{
		{
			Name:        "SUPER_ADMIN",
			Permissions: []Permission{PermissionAll},
		},
		{
			Name: "ADMIN",
			Permissions: []Permission{
				PermissionUsersAll,
				PermissionGlobalRolesAll,
				PermissionProjectsAll,
			},
		},
		{
			Name: "USER",
			Permissions: []Permission{
				PermissionUsersRead,
			},
		},
	}
}

// DefaultProjectRoles returns built-in project role templates.
func DefaultProjectRoles() []RoleDefinition {
	return []RoleDefinition{
		{
			Name: "PROJECT_OWNER",
			Permissions: []Permission{
				PermissionProjectsAll,
				PermissionProjectMembersAll,
				PermissionProjectRolesAll,
				PermissionTasksAll,
				PermissionSprintsAll,
				PermissionDocsAll,
				PermissionAgentsAll,
				PermissionWorkflowsAll,
			},
		},
		{
			Name: "PROJECT_MANAGER",
			Permissions: []Permission{
				PermissionProjectsRead,
				PermissionProjectsWrite,
				PermissionProjectMembersRead,
				PermissionProjectMembersWrite,
				PermissionTasksAll,
				PermissionSprintsAll,
				PermissionDocsAll,
				PermissionAgentsAll,
				PermissionWorkflowsAll,
			},
		},
		{
			Name: "PROJECT_MEMBER",
			Permissions: []Permission{
				PermissionProjectsRead,
				PermissionProjectMembersRead,
				PermissionProjectRolesRead,
				PermissionTasksRead,
				PermissionTasksWrite,
				PermissionSprintsRead,
				PermissionDocsRead,
				PermissionDocsWrite,
				PermissionAgentsRead,
				PermissionAgentsWrite,
				PermissionWorkflowsRead,
				PermissionWorkflowsWrite,
			},
		},
		{
			Name: "PROJECT_VIEWER",
			Permissions: []Permission{
				PermissionProjectsRead,
				PermissionProjectMembersRead,
				PermissionProjectRolesRead,
				PermissionTasksRead,
				PermissionSprintsRead,
				PermissionDocsRead,
				PermissionAgentsRead,
				PermissionWorkflowsRead,
			},
		},
	}
}

// legacyRolePermissions is the ONE source of truth for "which role names still
// carry permissions by name alone".
//
// Đây là cây cầu tạm bắc từ 28/03/2026 (commit 71f1089e), khi hệ thống quyền
// thật ra đời. Trước đó tên vai CHÍNH LÀ phân quyền — `Policy.Require(role,
// "ADMIN")` — nên cầu này dịch tên cũ sang tập quyền tương đương để các tuyến
// chưa kịp di trú vẫn chạy. Chú thích gốc nói rõ nó sống "until all callers are
// migrated"; cái "until" ấy chưa tới.
//
// Chừng nào cầu còn đứng thì TÊN VAI LÀ MỘT ĐẶC QUYỀN, và `IsLegacyRoleName`
// bên dưới phải được hỏi ở mọi cửa đặt tên vai. Hai chỗ đọc chung một map này
// để chúng không thể lệch nhau — trước đây chúng lệch, và chỗ lệch chính là một
// đường leo thang (xem `IsLegacyRoleName`).
var legacyRolePermissions = map[string][]Permission{
	"SUPER_ADMIN": {PermissionAll},
	"ADMIN":       {PermissionAll},
	"USER":        {PermissionUsersRead},
}

// normalizeLegacyRoleName đưa tên vai về dạng so sánh. MỌI chỗ so tên vai phải
// đi qua đây; một chỗ so nguyên văn còn chỗ kia viết hoa là đủ để mở một lỗ.
func normalizeLegacyRoleName(role string) string {
	return strings.ToUpper(strings.TrimSpace(role))
}

// LegacyPermissionsForRole preserves compatibility with the existing
// users.role claim until all callers are migrated to explicit role assignment.
func LegacyPermissionsForRole(role string) []Permission {
	return legacyRolePermissions[normalizeLegacyRoleName(role)]
}

// IsLegacyRoleName reports whether a role name would be granted permissions by
// its NAME alone, ignoring case and surrounding space.
//
// Vì sao cần: `LegacyPermissionsForRole` viết hoa rồi mới so, trong khi phép
// kiểm trùng tên lúc tạo vai dùng `WHERE name = $1` — phân biệt hoa thường. Hai
// luật khác nhau cho cùng một cái tên, nên một vai tên `"Admin"` (chữ d thường)
// trông như tên MỚI với phép kiểm trùng, rồi được viết hoa thành `"ADMIN"` và
// nhận `*`. Vai ấy không cần một quyền nào trong bảng, nên trần cấp phát —
// vốn chỉ đọc BẢNG quyền — không có gì để phản đối.
//
// Tức là: một vai RỖNG, đặt đúng tên, là chìa khoá vạn năng.
func IsLegacyRoleName(role string) bool {
	_, ok := legacyRolePermissions[normalizeLegacyRoleName(role)]
	return ok
}
