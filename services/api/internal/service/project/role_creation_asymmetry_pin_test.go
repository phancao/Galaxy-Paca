package projectsvc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	projectdom "github.com/Paca-AI/api/internal/domain/project"
	"github.com/Paca-AI/api/internal/platform/authz"
)

// ---------------------------------------------------------------------------
// GHIM HÀNH VI HIỆN TẠI — KHÔNG PHẢI HÀNH VI MONG MUỐN
//
// Phép kiểm trong tệp này KHÔNG khẳng định hệ thống đang đúng. Nó ghim lại một
// bất đối xứng đã được chứng minh, để nó thôi vô hình và có người canh. Đang
// chờ anh Cao cho hướng.
//
// BẤT ĐỐI XỨNG
//
// Hai đầu của cùng một vòng đời vai project được canh khác nhau:
//
//   • TẠO vai — project_role_service.go:23-52 (CreateRole): KHÔNG có ceiling,
//     KHÔNG kiểm khoá quyền so với từ vựng. Nhận thẳng map người gọi đưa vào,
//     kể cả khoá rác như {"read": true} hay {"toan_quyen": true}.
//   • GÁN vai — project_member_service.go:19-27 (enforceRoleGrantCeiling, PACA-4):
//     CÓ ceiling. Người gọi phải BAO được mọi quyền của vai.
//
// Hệ quả: tạo được một vai mà sau đó KHÔNG AI ngoài người giữ "*" gán nổi. Vai
// chết từ lúc sinh ra, im lặng — API trả 201, vai hiện trong danh sách, và chỉ
// khi ai đó thử gán mới nhận 403 không giải thích được. Không có cảnh báo lúc
// tạo, không log, không cổng nào đỏ.
//
// Đây đúng là thứ đã làm phép kiểm E2E TestE2EProjectMembers_FullLifecycle đỏ
// suốt: helper tạo vai với {"read": true} rồi ngạc nhiên vì không gán được.
//
// HƯỚNG CÓ THỂ ĐI (cần quyết định, chưa làm gì):
//   - kiểm khoá quyền lúc TẠO vai, từ chối khoá ngoài từ vựng
//     (internal/platform/authz/permissions.go);
//   - hoặc áp luôn ceiling lúc tạo, để không tạo được vai rộng hơn chính mình;
//   - hoặc giữ nguyên và cảnh báo ở tầng giao diện.
// Lưu ý: hai hướng đầu SIẾT LẠI, nên phải rà dữ liệu vai đang có trước khi bật.
// ---------------------------------------------------------------------------

// TestCreateRole_AcceptsJunkPermissionKeys_ThenNobodyCanAssign ghim cả hai đầu
// trong MỘT phép kiểm, để thấy rõ chúng bất đối xứng.
func TestCreateRole_AcceptsJunkPermissionKeys_ThenNobodyCanAssign(t *testing.T) {
	projectID := uuid.New()
	ctx := context.Background()

	repo := &memberServiceRepoMock{
		findByID: func(_ context.Context, id uuid.UUID) (*projectdom.Project, error) {
			return &projectdom.Project{ID: id}, nil
		},
	}
	svc := New(repo, nil)

	// ĐẦU 1 — TẠO: khoá "read" không có trong từ vựng quyền (mọi khoá thật đều
	// có không gian tên: projects.read, project.members.read …). Vẫn được nhận.
	role, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
		RoleName:    "vai-rac",
		Permissions: map[string]any{"read": true},
	})
	assert.NoError(t, err, "HÀNH VI ĐÃ ĐỔI: CreateRole giờ từ chối khoá rác. "+
		"Nếu đây là bản vá có chủ ý cho bất đối xứng mô tả ở đầu tệp, hãy xoá "+
		"phép kiểm này và ghi lại quyết định.")
	assert.NotNil(t, role, "vai phải được tạo")
	assert.Equal(t, map[string]any{"read": true}, role.Permissions,
		"khoá rác được lưu nguyên vẹn, không chuẩn hoá, không cảnh báo")

	// ĐẦU 2 — GÁN: cùng vai ấy, người gọi giữ TOÀN BỘ từ vựng quyền project vẫn
	// không gán nổi, vì "read" không nằm trong bất kỳ khoá thật nào.
	repo.findRoleByID = func(_ context.Context, id uuid.UUID) (*projectdom.ProjectRole, error) {
		r := *role
		r.ID = id
		return &r, nil
	}
	added := false
	repo.addMember = func(_ context.Context, _ *projectdom.ProjectMember) error {
		added = true
		return nil
	}

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
	_, err = svc.AddMember(ctx, projectID, projectdom.AddMemberInput{
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
