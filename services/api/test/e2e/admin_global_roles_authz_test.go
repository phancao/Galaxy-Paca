package e2e_test

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/google/uuid"

	globalroledom "github.com/Paca-AI/api/internal/domain/globalrole"
)

func TestAdminGlobalRolesAuthorization(t *testing.T) {
	env := newE2EEnv(t)

	const username = "rolecheck"
	const password = "supersecret"

	seedUser(t, env, username, password, "Role Check")

	readOnlyName := "READ_ONLY_" + uuid.NewString()
	if err := env.roleRepo.Create(env.ctx, &globalroledom.GlobalRole{
		ID:          uuid.New(),
		Name:        readOnlyName,
		Permissions: map[string]any{"global_roles.read": true},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("create read-only role: %v", err)
	}

	writeOnlyName := "WRITE_ONLY_" + uuid.NewString()
	if err := env.roleRepo.Create(env.ctx, &globalroledom.GlobalRole{
		ID:          uuid.New(),
		Name:        writeOnlyName,
		Permissions: map[string]any{"global_roles.write": true},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("create write-only role: %v", err)
	}

	assignOnlyName := "ASSIGN_ONLY_" + uuid.NewString()
	if err := env.roleRepo.Create(env.ctx, &globalroledom.GlobalRole{
		ID:          uuid.New(),
		Name:        assignOnlyName,
		Permissions: map[string]any{"global_roles.assign": true},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("create assign-only role: %v", err)
	}

	t.Run("without_global_role_permissions_read_is_forbidden", func(t *testing.T) {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

		assignGlobalRolesByName(t, env, username)
		loginResp := login(env.ctx, t, client, env.base, username, password)
		_ = loginResp.Body.Close()

		req := mustRequest(env.ctx, t, http.MethodGet, env.base+"/api/v1/admin/global-roles", nil)
		resp := mustDo(t, client, req)
		defer func() { _ = resp.Body.Close() }()

		assertStatus(t, resp, http.StatusForbidden)
		assertErrorCode(t, resp, "FORBIDDEN")
	})

	t.Run("read_permission_allows_list_but_not_write", func(t *testing.T) {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

		assignGlobalRolesByName(t, env, username, readOnlyName)
		loginResp := login(env.ctx, t, client, env.base, username, password)
		_ = loginResp.Body.Close()

		listReq := mustRequest(env.ctx, t, http.MethodGet, env.base+"/api/v1/admin/global-roles", nil)
		listResp := mustDo(t, client, listReq)
		defer func() { _ = listResp.Body.Close() }()
		assertStatus(t, listResp, http.StatusOK)

		createBody := jsonBody(t, map[string]any{
			"name":        "READ_ONLY_CANNOT_CREATE_" + uuid.NewString(),
			"permissions": map[string]any{"global_roles.read": true},
		})
		createReq := mustRequest(env.ctx, t, http.MethodPost, env.base+"/api/v1/admin/global-roles", createBody)
		createReq.Header.Set("Content-Type", "application/json")
		createResp := mustDo(t, client, createReq)
		defer func() { _ = createResp.Body.Close() }()

		assertStatus(t, createResp, http.StatusForbidden)
		assertErrorCode(t, createResp, "FORBIDDEN")
	})

	t.Run("write_permission_cannot_assign_roles", func(t *testing.T) {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

		assignGlobalRolesByName(t, env, username, writeOnlyName)
		loginResp := login(env.ctx, t, client, env.base, username, password)
		_ = loginResp.Body.Close()

		target, err := env.userRepo.FindByUsername(env.ctx, username)
		if err != nil {
			t.Fatalf("find target user: %v", err)
		}

		assignReq := mustRequest(
			env.ctx,
			t,
			http.MethodPut,
			env.base+"/api/v1/admin/users/"+target.ID.String()+"/global-roles",
			jsonBody(t, map[string]any{"role_ids": []string{}}),
		)
		assignReq.Header.Set("Content-Type", "application/json")

		assignResp := mustDo(t, client, assignReq)
		defer func() { _ = assignResp.Body.Close() }()

		assertStatus(t, assignResp, http.StatusForbidden)
		assertErrorCode(t, assignResp, "FORBIDDEN")
	})

	// PACA-3, CHIỀU CHẶN: global_roles.assign một mình KHÔNG đủ để phát vai.
	//
	// Người gọi chỉ giữ global_roles.assign. Vai đích USER cấp users.read
	// (migrations/000001_init.sql:123), thứ người gọi KHÔNG có — nên ceiling
	// PACA-3 (global_role_service.go:132-143) từ chối.
	//
	// Mắt xích dễ đọc nhầm nằm ở claims.Role: User.Role là TÊN vai lấy từ JOIN
	// global_roles (domain/user/entity.go:26-28) và được đúc thẳng vào JWT
	// (auth_service.go:87). Gán vai tên "ASSIGN_ONLY_<uuid>" làm
	// LegacyPermissionsForRole rơi vào default: nil (authz/defaults.go:100-112),
	// nên người gọi MẤT users.read mà vai USER trước đó ngầm cấp. Xem
	// TestLegacyRoleNameFallback_SilentlyDropsImplicitPermissions.
	//
	// Phép kiểm này từng kỳ vọng 200 — viết 28/03 (71f1089e), ceiling thêm
	// 18/07 (79cc561d), không ai chỉnh theo. 200 là kỳ vọng SAI: nó cho phép
	// đúng kiểu tự leo thang mà PACA-3 sinh ra để chặn.
	t.Run("assign_permission_alone_cannot_grant_a_broader_role", func(t *testing.T) {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

		assignGlobalRolesByName(t, env, username, assignOnlyName)
		loginResp := login(env.ctx, t, client, env.base, username, password)
		_ = loginResp.Body.Close()

		target, err := env.userRepo.FindByUsername(env.ctx, username)
		if err != nil {
			t.Fatalf("find target user: %v", err)
		}

		userRole, err := env.roleRepo.FindByName(env.ctx, "USER")
		if err != nil {
			t.Fatalf("find USER role: %v", err)
		}

		assignReq := mustRequest(
			env.ctx,
			t,
			http.MethodPut,
			env.base+"/api/v1/admin/users/"+target.ID.String()+"/global-roles",
			jsonBody(t, map[string]any{"role_ids": []string{userRole.ID.String()}}),
		)
		assignReq.Header.Set("Content-Type", "application/json")

		assignResp := mustDo(t, client, assignReq)
		defer func() { _ = assignResp.Body.Close() }()

		assertStatus(t, assignResp, http.StatusForbidden)
		assertErrorCode(t, assignResp, "FORBIDDEN")
	})

	// PACA-3, CHIỀU CHO QUA: vai bao đủ thì assign được.
	//
	// Ca đối chứng của ca trên. Không nới ceiling và không cấp thêm quyền cho
	// ASSIGN_ONLY: dùng một vai KHÁC, giữ đúng hai quyền cần thiết —
	// global_roles.assign để qua cửa tuyến, và users.read để BAO được vai đích.
	// Thiếu ca này, ca chặn ở trên vẫn xanh kể cả khi ceiling chặn nhầm tất cả.
	t.Run("assign_permission_plus_coverage_allows_role_assignment", func(t *testing.T) {
		coveringName := "ASSIGN_AND_USERS_READ_" + uuid.NewString()
		if err := env.roleRepo.Create(env.ctx, &globalroledom.GlobalRole{
			ID:   uuid.New(),
			Name: coveringName,
			Permissions: map[string]any{
				"global_roles.assign": true,
				"users.read":          true,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("create covering role: %v", err)
		}

		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

		assignGlobalRolesByName(t, env, username, coveringName)
		loginResp := login(env.ctx, t, client, env.base, username, password)
		_ = loginResp.Body.Close()

		target, err := env.userRepo.FindByUsername(env.ctx, username)
		if err != nil {
			t.Fatalf("find target user: %v", err)
		}

		userRole, err := env.roleRepo.FindByName(env.ctx, "USER")
		if err != nil {
			t.Fatalf("find USER role: %v", err)
		}

		assignReq := mustRequest(
			env.ctx,
			t,
			http.MethodPut,
			env.base+"/api/v1/admin/users/"+target.ID.String()+"/global-roles",
			jsonBody(t, map[string]any{"role_ids": []string{userRole.ID.String()}}),
		)
		assignReq.Header.Set("Content-Type", "application/json")

		assignResp := mustDo(t, client, assignReq)
		defer func() { _ = assignResp.Body.Close() }()

		assertStatus(t, assignResp, http.StatusOK)
	})
}
