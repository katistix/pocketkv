// Package web serves the dashboard: server-rendered html/template pages
// behind a login page with a session cookie.
package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/katistix/pocketkv/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

const maxFormBytes = 5 << 20 // 5 MiB per request

type server struct {
	db    *store.DB
	auth  *auth
	pages map[string]*template.Template
}

// New returns the dashboard handler. Every page except /login needs a
// signed-in session, and every non-GET request gets cross-origin protection.
func New(db *store.DB, user, password string) http.Handler {
	s := &server{db: db, auth: newAuth(user, password), pages: map[string]*template.Template{}}
	for _, p := range []string{"login", "index", "store", "value"} {
		s.pages[p] = template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/"+p+".html"))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("POST /stores", s.createStore)
	mux.HandleFunc("GET /stores/{store}", s.showStore)
	mux.HandleFunc("POST /stores/{store}/delete", s.deleteStore)
	mux.HandleFunc("GET /stores/{store}/value", s.editValue)
	mux.HandleFunc("POST /stores/{store}/value", s.saveValue)
	mux.HandleFunc("POST /stores/{store}/value/delete", s.deleteValue)
	mux.HandleFunc("POST /stores/{store}/keys", s.createAPIKey)
	mux.HandleFunc("POST /stores/{store}/keys/delete", s.deleteAPIKey)
	mux.HandleFunc("POST /logout", s.logout)

	root := http.NewServeMux()
	root.HandleFunc("GET /login", s.loginPage)
	root.HandleFunc("POST /login", s.login)
	root.Handle("/", s.auth.require(mux))

	var h http.Handler = root
	h = http.NewCrossOriginProtection().Handler(h)
	return http.MaxBytesHandler(h, maxFormBytes)
}

// --- pages ---

type indexData struct {
	Stores []store.Summary
	Error  string
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "index", indexData{Stores: s.db.List()})
}

func (s *server) createStore(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if err := s.db.CreateStore(name); err != nil {
		s.render(w, http.StatusBadRequest, "index", indexData{Stores: s.db.List(), Error: err.Error()})
		return
	}
	redirect(w, r, "/stores/"+name)
}

func (s *server) deleteStore(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteStore(r.PathValue("store")); err != nil {
		s.fail(w, err)
		return
	}
	redirect(w, r, "/")
}

type valueRow struct {
	Key     string
	Preview string
	Size    int
}

type storeData struct {
	Store     store.Store
	Values    []valueRow
	BaseURL   string
	SampleKey string // a real key from this store for the usage examples
	Error     string
}

func (s *server) showStore(w http.ResponseWriter, r *http.Request) {
	s.renderStore(w, r, http.StatusOK, "")
}

func (s *server) renderStore(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	st, err := s.db.Get(r.PathValue("store"))
	if err != nil {
		s.fail(w, err)
		return
	}
	rows := make([]valueRow, 0, len(st.Values))
	for k, v := range st.Values {
		rows = append(rows, valueRow{Key: k, Preview: truncate(string(v), 80), Size: len(v)})
	}
	slices.SortFunc(rows, func(a, b valueRow) int { return strings.Compare(a.Key, b.Key) })
	sample := "my-key"
	if len(rows) > 0 {
		sample = rows[0].Key
	}
	s.render(w, status, "store", storeData{Store: st, Values: rows, BaseURL: baseURL(r), SampleKey: sample, Error: errMsg})
}

type valueData struct {
	Store string
	Key   string
	Value string
	IsNew bool
	Error string
}

func (s *server) editValue(w http.ResponseWriter, r *http.Request) {
	name, key := r.PathValue("store"), r.URL.Query().Get("key")
	if _, err := s.db.Get(name); err != nil {
		s.fail(w, err)
		return
	}
	d := valueData{Store: name, Key: key, IsNew: key == "", Value: "{\n  \n}"}
	if key != "" {
		v, err := s.db.Value(name, key)
		if err != nil {
			s.fail(w, err)
			return
		}
		var buf bytes.Buffer
		json.Indent(&buf, v, "", "  ")
		d.Value = buf.String()
	}
	s.render(w, http.StatusOK, "value", d)
}

func (s *server) saveValue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("store")
	d := valueData{
		Store: name,
		Key:   strings.TrimSpace(r.FormValue("key")),
		Value: r.FormValue("value"),
		IsNew: r.FormValue("new") == "1",
	}
	if err := s.db.SetValue(name, d.Key, []byte(d.Value), !d.IsNew); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, err)
			return
		}
		d.Error = err.Error()
		s.render(w, http.StatusBadRequest, "value", d)
		return
	}
	redirect(w, r, "/stores/"+name)
}

func (s *server) deleteValue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("store")
	if err := s.db.DeleteValue(name, r.FormValue("key")); err != nil {
		s.fail(w, err)
		return
	}
	redirect(w, r, "/stores/"+name)
}

func (s *server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("store")
	if _, err := s.db.CreateAPIKey(name, r.FormValue("label")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, err)
			return
		}
		s.renderStore(w, r, http.StatusBadRequest, err.Error())
		return
	}
	redirect(w, r, "/stores/"+name+"#keys")
}

func (s *server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("store")
	if err := s.db.DeleteAPIKey(name, r.FormValue("key")); err != nil {
		s.fail(w, err)
		return
	}
	redirect(w, r, "/stores/"+name+"#keys")
}

// --- helpers ---

func (s *server) render(w http.ResponseWriter, status int, page string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	log.Printf("dashboard: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
