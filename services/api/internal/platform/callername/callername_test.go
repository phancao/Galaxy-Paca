package callername

import (
	"net/http"
	"strings"
	"testing"
)

func TestServiceNameDefault(t *testing.T) {
	t.Setenv("GALAXY_SERVICE_NAME", "  ")
	if got := ServiceName(); got != "paca-api" {
		t.Fatalf("ServiceName() = %q, want paca-api", got)
	}
}

func TestServiceNameEnvOverride(t *testing.T) {
	t.Setenv("GALAXY_SERVICE_NAME", "  paca-api-tmo  ")
	if got := ServiceName(); got != "paca-api-tmo" {
		t.Fatalf("ServiceName() = %q, want paca-api-tmo", got)
	}
}

func TestServiceNameSanitizesAndTruncates(t *testing.T) {
	t.Setenv("GALAXY_SERVICE_NAME", "evil\r\nX-Injected: 1 name")
	if got := ServiceName(); got != "evil--X-Injected:-1-name" {
		t.Fatalf("ServiceName() = %q", got)
	}
	t.Setenv("GALAXY_SERVICE_NAME", strings.Repeat("a", 100))
	if got := ServiceName(); len(got) != 64 {
		t.Fatalf("len(ServiceName()) = %d, want 64", len(got))
	}
}

func TestSetKeepsOtherHeaders(t *testing.T) {
	t.Setenv("GALAXY_SERVICE_NAME", "")
	h := http.Header{}
	h.Set("X-Service-Secret", "s")
	Set(h)
	if h.Get(Header) != "paca-api" || h.Get("X-Service-Secret") != "s" {
		t.Fatalf("headers = %v", h)
	}
}
