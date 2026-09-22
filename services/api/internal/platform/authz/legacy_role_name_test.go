package authz

import "testing"

// Hai hàm này PHẢI nói cùng một điều: `LegacyPermissionsForRole` quyết định một
// cái tên có cấp quyền hay không, `IsLegacyRoleName` quyết định có cấm đặt tên
// ấy hay không. Chúng lệch nhau một lần rồi — kiểm trùng tên so nguyên văn
// trong khi tra quyền viết hoa trước — và chỗ lệch ấy chính là đường leo thang:
// vai tên "Admin" lọt qua phép kiểm trùng rồi nhận `*`.
//
// Cổng này khoá chúng vào chung một nguồn.
func TestIsLegacyRoleName_AgreesWithLegacyPermissionsForRole(t *testing.T) {
	cases := []string{
		"SUPER_ADMIN", "Super_Admin", "super_admin", " SUPER_ADMIN ",
		"ADMIN", "Admin", "admin", "  admin  ",
		"USER", "User", "user",
		"ANALYST", "administrator", "admin2", "", "   ", "ADMIN ROLE",
	}
	for _, name := range cases {
		grants := len(LegacyPermissionsForRole(name)) > 0
		reserved := IsLegacyRoleName(name)
		if grants != reserved {
			t.Errorf("tên %q: cấp quyền=%v nhưng dành riêng=%v — hai hàm đã lệch nhau",
				name, grants, reserved)
		}
	}
}

// Một cái tên chỉ được cấp quyền khi nó nằm trong bảng. Cổng này bắt trường hợp
// ai đó thêm một nhánh cấp quyền mà quên khai vào bảng tên dành riêng.
func TestLegacyRoleNames_AreExactlyThose(t *testing.T) {
	want := map[string]bool{"SUPER_ADMIN": true, "ADMIN": true, "USER": true}
	if len(legacyRolePermissions) != len(want) {
		t.Fatalf("bảng tên dành riêng đổi: có %d mục, mong %d — nếu đây là chủ ý, cập nhật cổng này VÀ rà lại phép chặn tên ở globalrole service",
			len(legacyRolePermissions), len(want))
	}
	for name := range legacyRolePermissions {
		if !want[name] {
			t.Errorf("tên mới %q trong bảng — nó nay là một ĐẶC QUYỀN, phải chặn được ở cửa đặt tên vai", name)
		}
		if name != normalizeLegacyRoleName(name) {
			t.Errorf("khoá %q chưa chuẩn hoá; tra cứu sẽ trượt", name)
		}
	}
}
