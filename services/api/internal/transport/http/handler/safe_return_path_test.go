package handler

import "testing"

// An open redirect through our own login is the failure mode worth a test:
// the value arrives from a query string a stranger can craft.
func TestSafeReturnPath(t *testing.T) {
	keep := []string{"/", "/home", "/projects/abc/tasks/def?x=1#frag"}
	for _, v := range keep {
		if got := safeReturnPath(v); got != v {
			t.Fatalf("safeReturnPath(%q) = %q, muốn giữ nguyên", v, got)
		}
	}
	drop := []string{
		"", "//evil.com", "https://evil.com", "http://evil.com",
		"evil.com", "javascript:alert(1)", "/ok\r\nSet-Cookie: a=b",
	}
	for _, v := range drop {
		if got := safeReturnPath(v); got != "" {
			t.Fatalf("safeReturnPath(%q) = %q, phải bị loại", v, got)
		}
	}
	if orRoot(safeReturnPath("//evil.com")) != "/" {
		t.Fatal("giá trị bị loại phải rơi về /")
	}
}
