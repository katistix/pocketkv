package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "pocketkv_session"
	sessionTTL    = 30 * 24 * time.Hour
	failedDelay   = time.Second // slows down password guessing
)

// auth checks the dashboard credentials and keeps signed-in sessions in
// memory. Restarting the server signs everyone out.
type auth struct {
	userHash, passHash [32]byte

	mu       sync.Mutex
	sessions map[string]time.Time // session token -> expiry

	loginMu sync.Mutex // serializes login attempts so failures can't run in parallel
}

func newAuth(user, password string) *auth {
	return &auth{
		userHash: sha256.Sum256([]byte(user)),
		passHash: sha256.Sum256([]byte(password)),
		sessions: map[string]time.Time{},
	}
}

// check compares credentials in constant time. A wrong guess holds the login
// lock for failedDelay, which limits guessing to about one try per second.
func (a *auth) check(user, password string) bool {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	u, p := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(password))
	ok := subtle.ConstantTimeCompare(u[:], a.userHash[:])&subtle.ConstantTimeCompare(p[:], a.passHash[:]) == 1
	if !ok {
		time.Sleep(failedDelay)
	}
	return ok
}

func (a *auth) newSession() string {
	token := rand.Text()
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for t, exp := range a.sessions {
		if now.After(exp) {
			delete(a.sessions, t)
		}
	}
	a.sessions[token] = now.Add(sessionTTL)
	return token
}

func (a *auth) valid(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[c.Value]
	return ok && time.Now().Before(exp)
}

func (a *auth) endSession(r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
}

// require sends visitors without a valid session to the login page.
func (a *auth) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.valid(r) {
			to := "/login"
			if r.Method == http.MethodGet && r.URL.RequestURI() != "/" {
				to += "?next=" + url.QueryEscape(r.URL.RequestURI())
			}
			http.Redirect(w, r, to, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- handlers ---

type loginData struct {
	User  string
	Next  string
	Error string
}

func (s *server) loginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))
	if s.auth.valid(r) {
		redirect(w, r, next)
		return
	}
	s.render(w, http.StatusOK, "login", loginData{Next: next})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	user, next := r.FormValue("user"), safeNext(r.FormValue("next"))
	if !s.auth.check(user, r.FormValue("password")) {
		s.render(w, http.StatusUnauthorized, "login", loginData{User: user, Next: next, Error: "Wrong username or password."})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.auth.newSession(),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	redirect(w, r, next)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.endSession(r)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
	redirect(w, r, "/login")
}

// safeNext only allows redirects to local paths, never to another site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
