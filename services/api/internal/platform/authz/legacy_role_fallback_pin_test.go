package authz_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/platform/authz"
)

// ---------------------------------------------------------------------------
// GHIM HÀNH VI HIỆN TẠI — KHÔNG PHẢI HÀNH VI MONG MUỐN
//
// Phép kiểm trong tệp này KHÔNG khẳng định hệ thống đang đúng. Nó ghim lại một
// cái bẫy đã được chứng minh, để nó thôi vô hình và có người canh: nếu ai đó
// đổi hành vi, phép kiểm này đỏ và buộc mở lại cuộc bàn thay vì để thay đổi
// trôi qua im lặng. Đang chờ anh Cao cho hướng.
//
// CÁI BẪY
//
// Gán cho người dùng một vai toàn cục có TÊN TUỲ Ý sẽ âm thầm THU HỒI quyền
// ngầm mà vai cũ cấp. Chuỗi nhân quả:
//
//  1. internal/domain/user/entity.go:26-28 — User.Role không phải cột riêng
//     trong bảng users; nó là TÊN vai lấy từ JOIN global_roles.
//  2. internal/service/auth/auth_service.go:87 — tên ấy được đúc thẳng vào JWT
//     làm claims.Role.
//  3. internal/platform/authz/defaults.go:100-112 — LegacyPermissionsForRole
//     chỉ nhận đúng ba tên SUPER_ADMIN / ADMIN / USER; mọi tên khác rơi vào
//     `default: return nil`.
//
// Hệ quả: người vận hành tạo vai tên "ANALYST" rồi gán cho một người đang ở vai
// USER sẽ làm người ấy MẤT users.read — trừ khi vai mới tự khai lại quyền đó.
// Không có cảnh báo, không có log, không có cổng nào đỏ. Người dùng chỉ thấy
// 403 ở những chỗ hôm qua vẫn vào được.
//
// Đây chính là nguyên nhân làm phép kiểm E2E
// TestAdminGlobalRolesAuthorization/assign_permission_* trông như lỗi sản phẩm
// trong khi ceiling PACA-3 đang chặn hoàn toàn đúng luật.
//
// HƯỚNG CÓ THỂ ĐI (cần quyết định, chưa làm gì):
//   - để nguyên và ghi vào tài liệu vận hành, coi quyền ngầm của vai dựng sẵn
//     là thứ sắp bỏ;
//   - hoặc buộc mọi vai phải khai quyền tường minh và bỏ hẳn nhánh legacy;
//   - hoặc cho vai tự khai "kế thừa" một vai dựng sẵn.
// ---------------------------------------------------------------------------

// TestLegacyRoleNameFallback_SilentlyDropsImplicitPermissions ghim bước 3 của
// chuỗi trên: chỉ ĐÚNG ba tên được nhận, mọi tên khác cho về rỗng.
func TestLegacyRoleNameFallback_SilentlyDropsImplicitPermissions(t *testing.T) {
	// Ba tên dựng sẵn mang quyền ngầm.
	for _, tc := range []struct {
		role string
		want authz.Permission
	}{
		{"SUPER_ADMIN", authz.PermissionAll},
		{"ADMIN", authz.PermissionAll},
		{"USER", authz.PermissionUsersRead},
	} {
		got := authz.LegacyPermissionsForRole(tc.role)
		if len(got) == 0 {
			t.Fatalf("vai dựng sẵn %q phải mang quyền ngầm, got rỗng", tc.role)
		}
		if got[0] != tc.want {
			t.Errorf("vai %q: want %v, got %v", tc.role, tc.want, got[0])
		}
	}

	// Mọi tên khác — kể cả tên do người vận hành đặt rất hợp lý — cho về rỗng.
	for _, role := range []string{
		"ANALYST",
		"PROJECT_ADMIN",
		"ASSIGN_ONLY_9a12",
		"USER_READONLY", // gần giống USER, vẫn rỗng
		"",
	} {
		if got := authz.LegacyPermissionsForRole(role); len(got) != 0 {
			t.Errorf("vai tuỳ ý %q: HÀNH VI ĐÃ ĐỔI, want rỗng, got %v "+
				"— nếu đây là chủ ý, xem lại chú thích đầu tệp này", role, got)
		}
	}
}

// TestLegacyRoleRename_RevokesImplicitPermission_EndToEnd ghim HỆ QUẢ thật sự
// đau: cùng một người, cùng một vai toàn cục trong DB, chỉ khác cái TÊN mà
// claims.Role mang theo — và quyền users.read biến mất.
func TestLegacyRoleRename_RevokesImplicitPermission_EndToEnd(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	// Vai mới trong DB chỉ cấp global_roles.assign. Nó KHÔNG khai users.read,
	// vì người vận hành tưởng users.read đã có sẵn từ vai USER.
	store := &stubPermissionStore{
		globalPerms: []authz.Permission{authz.PermissionGlobalRolesAssign},
	}
	a := authz.NewAuthorizer(store)

	// Trước khi đổi vai: claims.Role = "USER" → legacy cấp users.read.
	before, err := a.HasPermissions(ctx, userID, nil, "USER", authz.PermissionUsersRead)
	if err != nil {
		t.Fatalf("before: %v", err)
	}
	if !before {
		t.Fatal("trước khi đổi vai, users.read phải có (quyền ngầm của USER)")
	}

	// Sau khi gán vai tên tuỳ ý: claims.Role = "ANALYST" → legacy rỗng.
	after, err := a.HasPermissions(ctx, userID, nil, "ANALYST", authz.PermissionUsersRead)
	if err != nil {
		t.Fatalf("after: %v", err)
	}
	if after {
		t.Fatal("HÀNH VI ĐÃ ĐỔI: vai tên tuỳ ý giờ giữ được quyền ngầm của USER. " +
			"Nếu đây là bản vá có chủ ý cho cái bẫy mô tả ở đầu tệp, hãy xoá phép " +
			"kiểm này và ghi lại quyết định.")
	}

	// Chốt: quyền do CHÍNH vai mới khai thì vẫn còn — mất mát chỉ ở phần ngầm.
	kept, err := a.HasPermissions(ctx, userID, nil, "ANALYST", authz.PermissionGlobalRolesAssign)
	if err != nil {
		t.Fatalf("kept: %v", err)
	}
	if !kept {
		t.Fatal("quyền vai mới tự khai phải còn nguyên")
	}
}
