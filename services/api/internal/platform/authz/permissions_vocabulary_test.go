package authz_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Paca-AI/api/internal/platform/authz"
)

// TestAllPermissions_CoversEveryDeclaredConstant is the gate that keeps
// authz.AllPermissions() from falling behind the constants above it.
//
// It parses permissions.go itself and collects every `Xxx Permission = "..."`
// constant, then asserts the exported vocabulary contains exactly those values.
// Adding a constant without adding it to allPermissions turns this test red —
// which matters because the project-role validator (and anything else that asks
// "is this a real permission key?") answers from AllPermissions(): a forgotten
// constant would be rejected as a junk key at role creation time.
func TestAllPermissions_CoversEveryDeclaredConstant(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "permissions.go", nil, 0)
	require.NoError(t, err, "phải đọc được permissions.go để đếm hằng")

	declared := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Permission" {
				continue
			}
			for _, val := range vs.Values {
				lit, ok := val.(*ast.BasicLit)
				require.True(t, ok, "hằng Permission phải là literal chuỗi")
				require.Equal(t, token.STRING, lit.Kind)
				// lit.Value keeps its quotes; strip them.
				declared[lit.Value[1:len(lit.Value)-1]] = true
			}
		}
	}
	require.NotEmpty(t, declared, "không tìm thấy hằng Permission nào — phép kiểm này đã mù")

	exported := map[string]bool{}
	for _, p := range authz.AllPermissions() {
		assert.False(t, exported[string(p)], "AllPermissions() lặp khoá %q", p)
		exported[string(p)] = true
	}

	for key := range declared {
		assert.True(t, exported[key],
			"hằng Permission %q được khai nhưng THIẾU trong allPermissions — "+
				"thêm nó vào, nếu không vai project mang khoá này sẽ bị từ chối 400", key)
		assert.True(t, authz.IsBuiltinPermission(authz.Permission(key)),
			"IsBuiltinPermission(%q) phải đúng", key)
	}
	for key := range exported {
		assert.True(t, declared[key],
			"allPermissions chứa %q nhưng không có hằng Permission nào khai nó", key)
	}
	assert.Equal(t, len(declared), len(exported),
		"số khoá trong allPermissions phải bằng số hằng Permission đếm được")
}
