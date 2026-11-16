package galaxyai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// SPA-8542: the mint call keeps X-Service-Secret and adds X-Service-Name.
func TestMintSendsServiceName(t *testing.T) {
	for _, tc := range []struct{ env, want string }{
		{"", "paca-api"},
		{" paca-api-tmo ", "paca-api-tmo"},
		{"bad name\nx", "bad-name-x"},
	} {
		t.Setenv("GALAXY_SERVICE_NAME", tc.env)
		var name, secret, path string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name, secret, path = r.Header.Get("X-Service-Name"), r.Header.Get("X-Service-Secret"), r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		}))
		c := New(srv.URL, "s3cret", srv.URL+"/ai/v1", "")
		tok, err := c.mintActAsToken(context.Background(), "u-1")
		srv.Close()
		if err != nil || tok != "tok" {
			t.Fatalf("mint: tok=%q err=%v", tok, err)
		}
		if path != "/internal/mint-service-token" || secret != "s3cret" || name != tc.want {
			t.Fatalf("env %q: path=%q secret=%q name=%q, want name %q", tc.env, path, secret, name, tc.want)
		}
	}
}
