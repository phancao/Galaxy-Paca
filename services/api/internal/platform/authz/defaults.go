// Package authz provides authorization helpers.
package authz

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

// Vì sao ở đây KHÔNG còn hàm nào dịch TÊN vai ra quyền.
//
// Từ 28/03/2026 đến 22/09/2026 tệp này giữ `LegacyPermissionsForRole`: một
// bảng tra biến tên vai thành quyền ("ADMIN" -> `*`). Nó là cây cầu tạm bắc
// khi hệ thống quyền thật ra đời, để các tuyến chưa kịp di trú vẫn chạy —
// chú thích gốc nói rõ nó sống "until all callers are migrated to explicit
// role assignment".
//
// Cây cầu ấy biến CÁI TÊN thành một đặc quyền, và trần cấp phát (PACA-3/4)
// chỉ đọc BẢNG quyền nên không bao giờ nhìn thấy nó. Hai đường lách thực tế:
//   - một vai toàn cục RỖNG tên "Admin" (kiểm trùng tên so nguyên văn, tra
//     quyền thì viết hoa) nhận `*`;
//   - một AGENT trong vai project tên "Admin" nhận `*` toàn nền tảng, mà tạo
//     vai project chỉ cần `project.roles.write`.
//
// Quyền nay CHỈ đến từ bảng. Đừng bắc lại cây cầu này: nếu một vai cần quyền
// gì thì ghi vào `global_roles.permissions` / `project_roles.permissions`.
// Cổng `no_permissions_from_role_name_test.go` canh điều đó.
