package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katistix/pocketkv/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	db, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(db, "admin", "secret")
}

func do(h http.Handler, method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginFlow(t *testing.T) {
	h := newTestServer(t)

	// Signed out: pages redirect to the login page, keeping the target.
	rec := do(h, "GET", "/stores/x", nil, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?next=%2Fstores%2Fx" {
		t.Fatalf("signed out GET: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(h, "POST", "/stores", url.Values{"name": {"s"}}, nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("signed out POST: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(h, "GET", "/login", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("login page: %d", rec.Code)
	}

	// Wrong password.
	rec = do(h, "POST", "/login", url.Values{"user": {"admin"}, "password": {"nope"}}, nil)
	if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("wrong password: %d", rec.Code)
	}

	// Right password: cookie set, redirected to next.
	rec = do(h, "POST", "/login", url.Values{"user": {"admin"}, "password": {"secret"}, "next": {"/stores/x"}}, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/stores/x" {
		t.Fatalf("login: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	cookie := rec.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags: %+v", cookie)
	}
	if rec := do(h, "GET", "/", nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("signed in GET: %d", rec.Code)
	}
	if rec := do(h, "POST", "/stores", url.Values{"name": {"s"}}, cookie); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/stores/s" {
		t.Fatalf("signed in POST: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	// Logout invalidates the session server-side.
	do(h, "POST", "/logout", nil, cookie)
	if rec := do(h, "GET", "/", nil, cookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("after logout: %d", rec.Code)
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "/",
		"/stores/x":        "/stores/x",
		"//evil.com":       "/",
		"/\\evil.com":      "/",
		"https://evil.com": "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
