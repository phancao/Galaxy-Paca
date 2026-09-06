package bootstrap

import "testing"

// Postgres mặc định cho 100 kết nối. Một pool 25 là đúng khi nó ở một mình;
// bảy pool 25 là 175, và cái đó không hỏng lúc khởi động — nó hỏng lúc có tải,
// bằng "too many connections", vào một ngày không ai đang nhìn.
func TestTheApiNeverAsksForMoreConnectionsThanTheServerHas(t *testing.T) {
	const serverMax = 100
	for tenants := 1; tenants <= 16; tenants++ {
		open, idle := poolBudget(tenants)
		total := open * tenants
		if total > serverMax-20 {
			t.Errorf("tenants=%d: %d pools × %d = %d, quá sát trần %d (phải chừa chỗ cho bridge, backup, psql)",
				tenants, tenants, open, total, serverMax)
		}
		if open < 4 {
			t.Errorf("tenants=%d: pool %d quá nhỏ, yêu cầu sẽ xếp hàng", tenants, open)
		}
		if idle > open {
			t.Errorf("tenants=%d: idle %d > open %d", tenants, idle, open)
		}
	}
}

// Một tenant thì phải y như trước khi có chuyện đa tenant.
func TestOneTenantKeepsTheOriginalPool(t *testing.T) {
	open, idle := poolBudget(1)
	if open != 25 {
		t.Errorf("want the original 25 open conns for a single tenant, got %d", open)
	}
	if idle < 2 {
		t.Errorf("idle too small: %d", idle)
	}
}
