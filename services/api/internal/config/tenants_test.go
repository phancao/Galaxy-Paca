package config

import (
	"os"
	"testing"
)

// Bất biến quan trọng nhất của cả thiết kế: bật đa tenant KHÔNG được dời dữ
// liệu của tenant đang chạy. Tenant chính phải giữ nguyên xi ba giá trị cũ.
func TestPrimaryTenantKeepsTheValuesTheDeploymentAlreadyRunsOn(t *testing.T) {
	t.Setenv("PACA_TENANTS", "galaxy,tmo,hdbank")
	base := "postgres://paca:pw@postgres:5432/paca?sslmode=disable"

	got, errs := buildTenants("galaxy", base, "redis://valkey:6379/0", "paca")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 tenants, got %d: %+v", len(got), got)
	}
	if got[0].Code != "galaxy" {
		t.Fatalf("primary must be first, got %q", got[0].Code)
	}
	if got[0].DSN != base {
		t.Errorf("primary DSN changed:\n got %q\nwant %q", got[0].DSN, base)
	}
	if got[0].RedisURL != "redis://valkey:6379/0" {
		t.Errorf("primary redis changed: %q", got[0].RedisURL)
	}
	if got[0].Bucket != "paca" {
		t.Errorf("primary bucket changed: %q", got[0].Bucket)
	}
}

func TestAdditionalTenantsGetTheirOwnDatabaseIndexAndBucket(t *testing.T) {
	t.Setenv("PACA_TENANTS", "galaxy,tmo,hdbank")
	got, errs := buildTenants("galaxy",
		"postgres://paca:pw@postgres:5432/paca?sslmode=disable",
		"redis://valkey:6379/0", "paca")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []TenantConfig{
		{Code: "tmo", DSN: "postgres://paca:pw@postgres:5432/paca_tmo?sslmode=disable", RedisURL: "redis://valkey:6379/1", Bucket: "paca-tmo"},
		{Code: "hdbank", DSN: "postgres://paca:pw@postgres:5432/paca_hdbank?sslmode=disable", RedisURL: "redis://valkey:6379/2", Bucket: "paca-hdbank"},
	}
	for i, w := range want {
		g := got[i+1]
		if g != w {
			t.Errorf("tenant %d:\n got %+v\nwant %+v", i+1, g, w)
		}
	}
}

// Không tenant nào được dùng chung kết nối với tenant khác — đó là điều duy
// nhất thiết kế này tồn tại để ngăn, nên cấu hình sai phải chết ở lúc khởi
// động chứ không phải lặng lẽ trùng kho.
func TestNoTwoTenantsShareAConnection(t *testing.T) {
	t.Setenv("PACA_TENANTS", "galaxy,tmo,hdbank,spaxe,8verse")
	got, errs := buildTenants("galaxy",
		"postgres://paca:pw@postgres:5432/paca?sslmode=disable",
		"redis://valkey:6379/0", "paca")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, field := range []struct {
		name string
		get  func(TenantConfig) string
	}{
		{"DSN", func(c TenantConfig) string { return c.DSN }},
		{"RedisURL", func(c TenantConfig) string { return c.RedisURL }},
		{"Bucket", func(c TenantConfig) string { return c.Bucket }},
	} {
		seen := map[string]string{}
		for _, tc := range got {
			if prev, dup := seen[field.get(tc)]; dup {
				t.Errorf("%s collision: %q and %q both use %q", field.name, prev, tc.Code, field.get(tc))
			}
			seen[field.get(tc)] = tc.Code
		}
	}
}

func TestTenantOverridesWin(t *testing.T) {
	t.Setenv("PACA_TENANTS", "galaxy,tmo")
	t.Setenv("PACA_TENANT_DSN_TMO", "postgres://other:pw@elsewhere:5432/tasks_tmo")
	t.Setenv("PACA_TENANT_BUCKET_TMO", "sovico-tasks")
	got, errs := buildTenants("galaxy",
		"postgres://paca:pw@postgres:5432/paca?sslmode=disable",
		"redis://valkey:6379/0", "paca")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got[1].DSN != "postgres://other:pw@elsewhere:5432/tasks_tmo" {
		t.Errorf("DSN override ignored: %q", got[1].DSN)
	}
	if got[1].Bucket != "sovico-tasks" {
		t.Errorf("bucket override ignored: %q", got[1].Bucket)
	}
}

// Hết chỉ số Valkey thì phải nói ra ở lúc khởi động, không phải lặng lẽ đưa
// hai tenant về cùng một kho.
func TestMoreTenantsThanValkeyIndexesIsAStartupError(t *testing.T) {
	codes := "galaxy"
	for i := 1; i <= 16; i++ {
		codes += ",t" + string(rune('a'+i-1))
	}
	t.Setenv("PACA_TENANTS", codes)
	_, errs := buildTenants("galaxy",
		"postgres://paca:pw@postgres:5432/paca?sslmode=disable",
		"redis://valkey:6379/0", "paca")
	if len(errs) == 0 {
		t.Fatal("want a startup error when tenants outnumber Valkey databases, got none")
	}
}

func TestPrimaryIsFirstEvenWhenListedLast(t *testing.T) {
	t.Setenv("PACA_TENANTS", "tmo,hdbank,galaxy")
	got, _ := buildTenants("galaxy",
		"postgres://paca:pw@postgres:5432/paca?sslmode=disable",
		"redis://valkey:6379/0", "paca")
	if got[0].Code != "galaxy" {
		t.Fatalf("primary must lead the list, got %q", got[0].Code)
	}
	if got[0].DSN != "postgres://paca:pw@postgres:5432/paca?sslmode=disable" {
		t.Errorf("primary lost its bare DSN: %q", got[0].DSN)
	}
}

func TestUnsetListFallsBackToTheSingleOidcTenant(t *testing.T) {
	os.Unsetenv("PACA_TENANTS")
	got, errs := buildTenants("galaxy",
		"postgres://paca:pw@postgres:5432/paca?sslmode=disable",
		"redis://valkey:6379/0", "paca")
	if len(errs) != 0 || len(got) != 1 || got[0].Code != "galaxy" {
		t.Fatalf("want exactly the galaxy tenant, got %+v errs=%v", got, errs)
	}
}
