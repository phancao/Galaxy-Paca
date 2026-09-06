package jwttoken

import (
	"strings"
	"testing"
	"time"
)

func managers(t *testing.T) (primary, other *Manager) {
	t.Helper()
	base := New("one-secret-shared-by-every-tenant", time.Hour, 24*time.Hour)
	return base.ForTenant("galaxy", true), base.ForTenant("tmo", false)
}

// Cả tiến trình ký bằng MỘT khoá, nên chữ ký hợp lệ chỉ nói "token này của
// chúng ta", không nói "của tenant này". Cửa vào phải nói điều còn lại.
func TestATokenFromOneTenantIsRefusedByAnother(t *testing.T) {
	galaxy, tmo := managers(t)

	tok, err := galaxy.IssueAccess("11111111-1111-1111-1111-111111111111", "an", "USER", "fam", false)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	if _, err := galaxy.Verify(tok); err != nil {
		t.Fatalf("its own tenant must accept it: %v", err)
	}
	err = mustFail(t, tmo, tok)
	if !strings.Contains(err.Error(), "minted for tenant \"galaxy\"") {
		t.Errorf("error should name both tenants, got: %v", err)
	}
}

// Token cũ (chưa có claim) chỉ tenant CHÍNH được nhận. Cho mọi tenant nhận
// thì một nhượng bộ tương thích hoá chiếc chìa vạn năng.
func TestALegacyTokenIsAcceptedOnlyByThePrimary(t *testing.T) {
	base := New("one-secret-shared-by-every-tenant", time.Hour, 24*time.Hour)
	legacy, err := base.IssueAccess("22222222-2222-2222-2222-222222222222", "cu", "USER", "fam", false)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	primary, other := managers(t)
	if _, err := primary.Verify(legacy); err != nil {
		t.Fatalf("primary must still accept sessions minted before the claim existed: %v", err)
	}
	if err := mustFail(t, other, legacy); !strings.Contains(err.Error(), "names no tenant") {
		t.Errorf("want a 'names no tenant' refusal, got: %v", err)
	}
}

// Đổi claim tenant trong token là đổi phần được chữ ký phủ.
func TestForgingTheTenantClaimBreaksTheSignature(t *testing.T) {
	galaxy, tmo := managers(t)
	tok, _ := galaxy.IssueAccess("33333333-3333-3333-3333-333333333333", "an", "USER", "fam", false)

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", tok)
	}
	forged := parts[0] + "." + strings.Replace(parts[1], "a", "b", 1) + "." + parts[2]
	if forged == tok {
		t.Skip("payload had no 'a' to flip")
	}
	mustFail(t, tmo, forged)
	mustFail(t, galaxy, forged)
}

func TestTheClaimTravelsOnTheToken(t *testing.T) {
	galaxy, _ := managers(t)
	tok, _ := galaxy.IssueAccess("44444444-4444-4444-4444-444444444444", "an", "USER", "fam", false)
	claims, err := galaxy.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Tenant != "galaxy" {
		t.Errorf("want tenant claim 'galaxy', got %q", claims.Tenant)
	}
}

func mustFail(t *testing.T, m *Manager, tok string) error {
	t.Helper()
	if _, err := m.Verify(tok); err != nil {
		return err
	}
	t.Fatalf("tenant %q accepted a token it must refuse", m.Tenant())
	return nil
}
