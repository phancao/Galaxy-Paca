package projectsvc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectdom "github.com/Paca-AI/api/internal/domain/project"
	"github.com/Paca-AI/api/internal/platform/authz"
)

// ---------------------------------------------------------------------------
// BẤT ĐỐI XỨNG ĐÃ ĐƯỢC VÁ — tệp này giờ ghim HÀNH VI MỚI
//
// Giữ lại phần mô tả vì nó là lý do bản vá tồn tại.
//
// BẤT ĐỐI XỨNG (hành vi CŨ, trước bản vá)
//
// Hai đầu của cùng một vòng đời vai project được canh khác nhau:
//
//   • TẠO vai — project_role_service.go (CreateRole): KHÔNG có ceiling, KHÔNG
//     kiểm khoá quyền so với từ vựng. Nhận thẳng map người gọi đưa vào, kể cả
//     khoá rác như {"read": true} hay {"toan_quyen": true}.
//   • GÁN vai — project_member_service.go (enforceRoleGrantCeiling, PACA-4):
//     CÓ ceiling. Người gọi phải BAO được mọi quyền của vai.
//
// Hệ quả: tạo được một vai mà sau đó KHÔNG AI ngoài người giữ "*" gán nổi. Vai
// chết từ lúc sinh ra, im lặng — API trả 201, vai hiện trong danh sách, và chỉ
// khi ai đó thử gán mới nhận 403 không giải thích được. Không có cảnh báo lúc
// tạo, không log, không cổng nào đỏ. Đây đúng là thứ đã làm phép kiểm E2E
// TestE2EProjectMembers_FullLifecycle đỏ suốt.
//
// QUYẾT ĐỊNH (đã làm)
//
// Siết ở đầu TẠO/SỬA: khoá quyền phải nằm trong một từ vựng có thật — 35 hằng
// dựng sẵn (internal/platform/authz/permissions.go) HỢP với khoá do plugin
// đang cài khai (PluginManifest.CustomPermissions) — nếu không, 400 kèm TÊN
// khoá sai. KHÔNG đụng tới ceiling và KHÔNG nới trần: đây là siết thêm.
//
// Ceiling vẫn giữ nguyên, nên nửa sau của phép kiểm dưới đây vẫn đúng: một vai
// CŨ đã lỡ mang khoá rác (ghi trước bản vá, nằm sẵn trong DB) vẫn không ai gán
// nổi. Bản vá chặn nguồn, nó không dọn dữ liệu cũ.
// ---------------------------------------------------------------------------

// TestCreateRole_RejectsJunkPermissionKeys ghim đầu TẠO: khoá rác giờ bị từ
// chối, và lỗi phải GỌI TÊN khoá sai chứ không im lặng.
func TestCreateRole_RejectsJunkPermissionKeys(t *testing.T) {
	projectID := uuid.New()
	ctx := context.Background()

	repo := &memberServiceRepoMock{
		findByID: func(_ context.Context, id uuid.UUID) (*projectdom.Project, error) {
			return &projectdom.Project{ID: id}, nil
		},
	}
	svc := New(repo, nil)

	// "read" trần không có trong từ vựng quyền: mọi khoá thật đều có không gian
	// tên (projects.read, project.members.read …).
	role, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
		RoleName:    "vai-rac",
		Permissions: map[string]any{"read": true},
	})

	assert.Nil(t, role, "không được tạo vai mang khoá rác")
	require.ErrorIs(t, err, projectdom.ErrRolePermissionsInvalid)

	var unknown *projectdom.UnknownPermissionsError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, []string{"read"}, unknown.Keys,
		"lỗi phải gọi tên đúng khoá sai, để người dùng sửa được")
	assert.Contains(t, err.Error(), "read",
		"thông điệp trả về client phải chứa tên khoá sai")
}

// TestLegacyJunkRole_StillCannotBeAssigned ghim đầu GÁN: ceiling KHÔNG đổi.
// Một vai cũ đã nằm trong DB với khoá rác (ghi trước bản vá — bản vá chặn
// nguồn, không dọn dữ liệu) vẫn không gán nổi, trừ người giữ "*".
func TestLegacyJunkRole_StillCannotBeAssigned(t *testing.T) {
	projectID := uuid.New()
	ctx := context.Background()

	// Vai này được dựng THẲNG như một hàng DB cũ, không qua CreateRole — vì
	// CreateRole giờ đã từ chối nó.
	legacy := &projectdom.ProjectRole{
		ID:          uuid.New(),
		ProjectID:   &projectID,
		RoleName:    "vai-rac-cu",
		Permissions: map[string]any{"read": true},
	}

	added := false
	repo := &memberServiceRepoMock{
		findByID: func(_ context.Context, id uuid.UUID) (*projectdom.Project, error) {
			return &projectdom.Project{ID: id}, nil
		},
		findRoleByID: func(_ context.Context, id uuid.UUID) (*projectdom.ProjectRole, error) {
			r := *legacy
			r.ID = id
			return &r, nil
		},
		addMember: func(_ context.Context, _ *projectdom.ProjectMember) error {
			added = true
			return nil
		},
	}
	svc := New(repo, nil)

	richCaller := callerWith(
		authz.PermissionProjectsAll,
		authz.PermissionProjectMembersAll,
		authz.PermissionProjectRolesAll,
		authz.PermissionTasksAll,
		authz.PermissionSprintsAll,
		authz.PermissionDocsAll,
		authz.PermissionAgentsAll,
		authz.PermissionWorkflowsAll,
	)
	_, err := svc.AddMember(ctx, projectID, projectdom.AddMemberInput{
		UserID:        uuid.New(),
		ProjectRoleID: uuid.New(),
	}, richCaller)

	assert.ErrorIs(t, err, projectdom.ErrPermissionCeilingExceeded,
		"vai chứa khoá rác phải không gán nổi kể cả với người gọi rất rộng quyền")
	assert.False(t, added, "không được ghi thành viên khi ceiling chặn")

	// Chỉ người giữ "*" mới qua — đó là lối thoát DUY NHẤT cho một vai đã chết.
	repo.findMember = func(_ context.Context, _, _ uuid.UUID) (*projectdom.ProjectMember, error) {
		return nil, projectdom.ErrMemberNotFound
	}
	_, _ = svc.AddMember(ctx, projectID, projectdom.AddMemberInput{
		UserID:        uuid.New(),
		ProjectRoleID: uuid.New(),
	}, callerWith(authz.PermissionAll))
	assert.True(t, added,
		`người giữ "*" bỏ qua được ceiling và GHI ĐƯỢC thành viên — lối thoát `+
			`duy nhất cho một vai đã chết`)
}
