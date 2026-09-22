package authz_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/platform/authz"
)

// ---------------------------------------------------------------------------
// CỔNG: quyền KHÔNG bao giờ được suy ra từ TÊN vai.
//
// Từ 28/03/2026 đến 22/09/2026 gói này có `LegacyPermissionsForRole`, một bảng
// tra biến tên vai thành quyền ("ADMIN" -> `*`). Trần cấp phát (PACA-3/4) chỉ
// đọc BẢNG quyền nên không nhìn thấy đường ấy, và hai lối leo thang có thật đi
// qua nó: một vai toàn cục RỖNG tên "Admin", và một AGENT trong vai project
// tên "Admin".
//
// Cây cầu đã gỡ. Ba phép kiểm dưới đây canh để nó không mọc lại: một đo HÀNH
// VI, một đo CHỮ KÝ hàm, một đo MÃ NGUỒN. Phép cuối tự kiểm chứng bằng cách
// đòi bộ dò của chính nó phải bắt được tệp mồi — một cổng không bắt được gì
// thì xanh vô nghĩa.
// ---------------------------------------------------------------------------

// nameOnlyStore không cấp quyền nào. Nó đại diện cho trường hợp thật: một
// người (hoặc agent) mang vai TÊN "ADMIN"/"SUPER_ADMIN" mà cột permissions
// của vai ấy rỗng.
type nameOnlyStore struct{}

func (nameOnlyStore) ListGlobalPermissions(context.Context, uuid.UUID) ([]authz.Permission, error) {
	return nil, nil
}

func (nameOnlyStore) ListProjectPermissions(context.Context, uuid.UUID, uuid.UUID) ([]authz.Permission, error) {
	return nil, nil
}

func (nameOnlyStore) ListAgentProjectPermissions(context.Context, uuid.UUID, uuid.UUID) ([]authz.Permission, error) {
	return nil, nil
}

// TestBangRongThiKhongCoQuyenNao: bảng rỗng thì không quyền nào được cấp, kể
// cả cho người lẫn agent. Trước bản vá, một vai tên "ADMIN" với bảng rỗng vẫn
// nhận `*` ở đây.
func TestBangRongThiKhongCoQuyenNao(t *testing.T) {
	ctx := context.Background()
	a := authz.NewAuthorizer(nameOnlyStore{})
	userID, agentID, projectID := uuid.New(), uuid.New(), uuid.New()

	for _, want := range []authz.Permission{
		authz.PermissionAll,
		authz.PermissionUsersRead,
		authz.PermissionTasksRead,
	} {
		ok, err := a.HasPermissions(ctx, userID, nil, want)
		if err != nil {
			t.Fatalf("HasPermissions(%s): %v", want, err)
		}
		if ok {
			t.Errorf("người dùng có bảng quyền RỖNG lại được cấp %s", want)
		}

		ok, err = a.HasPermissionsForAgent(ctx, agentID, projectID, want)
		if err != nil {
			t.Fatalf("HasPermissionsForAgent(%s): %v", want, err)
		}
		if ok {
			t.Errorf("agent có bảng quyền RỖNG lại được cấp %s", want)
		}
	}

	set, err := a.EffectivePermissions(ctx, userID, &projectID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}
	if len(set) != 0 {
		t.Errorf("tập quyền hiệu dụng phải rỗng, nhận %v", set)
	}
}

// TestCuaVaoKhongNhanTenVai: không cửa nào của Authorizer còn nhận một chuỗi.
// Đây là rào cứng nhất — còn tham số string thì cái tên còn đường chui vào.
func TestCuaVaoKhongNhanTenVai(t *testing.T) {
	a := authz.NewAuthorizer(nameOnlyStore{})
	v := reflect.ValueOf(a)

	for _, ten := range []string{
		"HasPermissions",
		"HasPermissionsForAgent",
		"EffectivePermissions",
		"EffectivePermissionsForAgent",
	} {
		m := v.MethodByName(ten)
		if !m.IsValid() {
			t.Fatalf("không còn hàm %s — nếu đổi tên thì sửa cổng này, đừng bỏ", ten)
		}
		ft := m.Type()
		for i := range ft.NumIn() {
			if ft.In(i).Kind() == reflect.String {
				t.Errorf("%s nhận tham số string thứ %d — tên vai đang có đường quay lại", ten, i)
			}
		}
	}
}

// TestChiTepDinhNghiaDuocGanTenVaiVoiQuyen: ngoài `defaults.go` (nơi khai vai
// dựng sẵn để GHI VÀO BẢNG lúc seed), không tệp sản phẩm nào được đặt một tên
// vai dựng sẵn cạnh một giá trị Permission.
func TestChiTepDinhNghiaDuocGanTenVaiVoiQuyen(t *testing.T) {
	goc, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	moi := filepath.Join(goc, "internal/platform/authz/defaults.go")

	var dinhNghiaBiBat bool
	var viPham []string

	err = filepath.WalkDir(filepath.Join(goc, "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dong := strings.Split(string(b), "\n")
		for i := range dong {
			if !ganTenVoiQuyen(dong, i) {
				continue
			}
			if p == moi {
				dinhNghiaBiBat = true
				continue
			}
			rel, _ := filepath.Rel(goc, p)
			viPham = append(viPham, rel+":"+itoa(i+1)+": "+strings.TrimSpace(dong[i]))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("duyệt cây: %v", err)
	}

	// Tự kiểm chứng: bộ dò phải bắt được tệp mồi. Nếu `defaults.go` đổi hình
	// dạng, cổng này xanh mà không đo gì — đỏ ở đây là để buộc nhìn lại.
	if !dinhNghiaBiBat {
		t.Fatalf("bộ dò không bắt được chính %s — cổng đang xanh vô nghĩa, sửa hàm ganTenVoiQuyen", moi)
	}

	if len(viPham) > 0 {
		t.Errorf("tên vai đang được gán quyền ngoài tệp định nghĩa (%d chỗ):\n  %s",
			len(viPham), strings.Join(viPham, "\n  "))
	}
}

// ganTenVoiQuyen báo dòng thứ i có đặt một tên vai dựng sẵn CẠNH một giá trị
// Permission hay không. "Cạnh" là trong cùng cửa sổ vaiQuyenGan dòng, vì một
// khai báo vai trải ra nhiều dòng — tên ở dòng này, quyền ở dòng sau.
const vaiQuyenGan = 3

func ganTenVoiQuyen(dong []string, i int) bool {
	coTen := false
	for _, ten := range []string{`"SUPER_ADMIN"`, `"ADMIN"`, `"USER"`} {
		if strings.Contains(dong[i], ten) {
			coTen = true
			break
		}
	}
	if !coTen {
		return false
	}
	for j := max(0, i-vaiQuyenGan); j <= min(len(dong)-1, i+vaiQuyenGan); j++ {
		if strings.Contains(dong[j], "Permission") {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
