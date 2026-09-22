package projectsvc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	plugindom "github.com/Paca-AI/api/internal/domain/plugin"
	projectdom "github.com/Paca-AI/api/internal/domain/project"
	"github.com/Paca-AI/api/internal/platform/authz"
)

// pluginSourceStub feeds the validator a fixed list of installed plugins.
type pluginSourceStub struct {
	plugins []*plugindom.Plugin
	err     error
	calls   int
}

func (p *pluginSourceStub) List(context.Context) ([]*plugindom.Plugin, error) {
	p.calls++
	return p.plugins, p.err
}

// timeLoggingPlugin mirrors a real installed plugin declaring a custom
// permission. plugindom.PluginManifest.Validate() forces the key under the
// plugin's own namespace, so "time_logging.manage_all" is what a manifest for
// "com.paca.time-logging" is allowed to declare.
func timeLoggingPlugin() *plugindom.Plugin {
	return &plugindom.Plugin{
		ID:      uuid.New(),
		Name:    "com.paca.time-logging",
		Enabled: true,
		Manifest: plugindom.PluginManifest{
			ID:          "com.paca.time-logging",
			DisplayName: "Time logging",
			Version:     "1.0.0",
			CustomPermissions: []plugindom.CustomPermission{
				{Key: "time_logging.manage_all", Label: "Manage all time logs", Scope: "project"},
			},
		},
	}
}

func roleRepoStub() *memberServiceRepoMock {
	return &memberServiceRepoMock{
		findByID: func(_ context.Context, id uuid.UUID) (*projectdom.Project, error) {
			return &projectdom.Project{ID: id}, nil
		},
	}
}

func TestCreateRole_PermissionKeyValidation(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	t.Run("khoa_dung_san_qua_duoc", func(t *testing.T) {
		svc := New(roleRepoStub(), nil)
		role, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName: "nguoi-xem",
			Permissions: map[string]any{
				string(authz.PermissionProjectMembersRead): true,
				string(authz.PermissionTasksRead):          true,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, role)
		assert.Len(t, role.Permissions, 2)
	})

	t.Run("wildcard_dung_san_qua_duoc", func(t *testing.T) {
		svc := New(roleRepoStub(), nil)
		role, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName:    "chu-du-an",
			Permissions: map[string]any{string(authz.PermissionProjectsAll): true},
		})
		require.NoError(t, err)
		require.NotNil(t, role)
	})

	t.Run("khoa_plugin_dang_cai_qua_duoc", func(t *testing.T) {
		src := &pluginSourceStub{plugins: []*plugindom.Plugin{timeLoggingPlugin()}}
		svc := New(roleRepoStub(), nil).WithPluginPermissions(src)
		role, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName: "quan-ly-gio",
			Permissions: map[string]any{
				"time_logging.manage_all":         true,
				string(authz.PermissionTasksRead): true,
			},
		})
		require.NoError(t, err, "khoá do plugin đang cài khai KHÔNG được coi là rác")
		require.NotNil(t, role)
		assert.Positive(t, src.calls, "phải thật sự hỏi danh sách plugin")
	})

	t.Run("wildcard_khong_gian_ten_plugin_qua_duoc", func(t *testing.T) {
		src := &pluginSourceStub{plugins: []*plugindom.Plugin{timeLoggingPlugin()}}
		svc := New(roleRepoStub(), nil).WithPluginPermissions(src)
		_, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName:    "toan-quyen-gio",
			Permissions: map[string]any{"time_logging.*": true},
		})
		require.NoError(t, err)
	})

	t.Run("khoa_plugin_chua_cai_bi_tu_choi", func(t *testing.T) {
		svc := New(roleRepoStub(), nil).WithPluginPermissions(&pluginSourceStub{})
		_, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName:    "quan-ly-gio",
			Permissions: map[string]any{"time_logging.manage_all": true},
		})
		require.ErrorIs(t, err, projectdom.ErrRolePermissionsInvalid)
	})

	t.Run("nhieu_khoa_rac_duoc_goi_ten_het_va_sap_xep", func(t *testing.T) {
		svc := New(roleRepoStub(), nil)
		_, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName: "vai-lung-tung",
			Permissions: map[string]any{
				"toan_quyen":                      true,
				"read":                            true,
				string(authz.PermissionTasksRead): true,
			},
		})
		var unknown *projectdom.UnknownPermissionsError
		require.ErrorAs(t, err, &unknown)
		assert.Equal(t, []string{"read", "toan_quyen"}, unknown.Keys,
			"mọi khoá sai phải được liệt kê, theo thứ tự ổn định")
	})

	t.Run("khoa_tat_khong_bi_soi", func(t *testing.T) {
		// {"x": false} không cấp gì, nên ceiling không đòi nó và validator
		// cũng không đụng tới — hai bên phải đọc map giống hệt nhau.
		svc := New(roleRepoStub(), nil)
		_, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName:    "vai-rong",
			Permissions: map[string]any{"read": false},
		})
		require.NoError(t, err)
	})

	t.Run("nguon_plugin_hong_thi_bao_loi_chu_khong_doan", func(t *testing.T) {
		boom := errors.New("plugin store down")
		svc := New(roleRepoStub(), nil).WithPluginPermissions(&pluginSourceStub{err: boom})
		_, err := svc.CreateRole(ctx, projectID, projectdom.CreateRoleInput{
			RoleName:    "quan-ly-gio",
			Permissions: map[string]any{"time_logging.manage_all": true},
		})
		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, projectdom.ErrRolePermissionsInvalid,
			"đọc hụt từ vựng KHÔNG được biến thành 'khoá này sai'")
	})
}

func TestUpdateRole_PermissionKeyValidation(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	roleID := uuid.New()

	// legacyRole là hàng DB ghi TRƯỚC bản vá: nó mang khoá rác "read".
	newRepo := func(perms map[string]any) *memberServiceRepoMock {
		repo := roleRepoStub()
		repo.findRoleByID = func(_ context.Context, id uuid.UUID) (*projectdom.ProjectRole, error) {
			return &projectdom.ProjectRole{
				ID:          id,
				ProjectID:   &projectID,
				RoleName:    "vai-cu",
				Permissions: perms,
			}, nil
		}
		return repo
	}

	t.Run("them_khoa_rac_moi_bi_tu_choi", func(t *testing.T) {
		svc := New(newRepo(map[string]any{string(authz.PermissionTasksRead): true}), nil)
		_, err := svc.UpdateRole(ctx, projectID, roleID, projectdom.UpdateRoleInput{
			RoleName: "vai-cu",
			Permissions: map[string]any{
				string(authz.PermissionTasksRead): true,
				"write":                           true,
			},
		})
		var unknown *projectdom.UnknownPermissionsError
		require.ErrorAs(t, err, &unknown)
		assert.Equal(t, []string{"write"}, unknown.Keys)
	})

	t.Run("khoa_rac_CU_duoc_giu_lai_van_sua_duoc", func(t *testing.T) {
		// Quyết định (ràng buộc 4): chỉ kiểm khoá MỚI thêm. Vai cũ vẫn đổi được
		// tên, vẫn thêm được khoá hợp lệ, mà không bị khoá cứng vì một khoá rác
		// có sẵn. Nếu áp cho cả bản ghi, vai ấy sẽ không sửa nổi — kể cả để dọn.
		svc := New(newRepo(map[string]any{"read": true}), nil)
		updated, err := svc.UpdateRole(ctx, projectID, roleID, projectdom.UpdateRoleInput{
			RoleName: "vai-cu-doi-ten",
			Permissions: map[string]any{
				"read":                            true,
				string(authz.PermissionTasksRead): true,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "vai-cu-doi-ten", updated.RoleName)
	})

	t.Run("don_khoa_rac_CU_di_thi_qua", func(t *testing.T) {
		svc := New(newRepo(map[string]any{"read": true}), nil)
		updated, err := svc.UpdateRole(ctx, projectID, roleID, projectdom.UpdateRoleInput{
			RoleName:    "vai-da-don",
			Permissions: map[string]any{string(authz.PermissionTasksRead): true},
		})
		require.NoError(t, err)
		assert.NotContains(t, updated.Permissions, "read")
	})
}
