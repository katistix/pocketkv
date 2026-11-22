package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/katistix/pocketkv/internal/store"
)

func TestAPI(t *testing.T) {
	db, _ := store.Open(filepath.Join(t.TempDir(), "db.json"))
	db.CreateStore("s")
	db.SetValue("s", "hero", []byte(`{"a":1}`), false)
	k, _ := db.CreateAPIKey("s", "")
	h := New(db)
	auth := http.Header{"Authorization": {"Bearer " + k.Key}}

	tests := []struct {
		name, path string
		header     http.Header
		status     int
		body       string
	}{
		{"get", "/api/v1/values/hero", auth, 200, `{"a":1}`},
		{"list", "/api/v1/values", auth, 200, `{"hero":{"a":1}}`},
		{"missing key", "/api/v1/values/nope", auth, 404, ""},
		{"no auth", "/api/v1/values/hero", nil, 401, ""},
		{"bad token", "/api/v1/values/hero", http.Header{"Authorization": {"Bearer pkv_wrong"}}, 401, ""},
		{"query param not accepted", "/api/v1/values/hero?key=" + k.Key, nil, 401, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			for k, v := range tt.header {
				req.Header[k] = v
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.status, rec.Body)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Fatalf("body = %s, want %s", rec.Body, tt.body)
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Fatal("missing CORS header")
			}
		})
	}
}
