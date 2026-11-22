// Package api serves the public, read-only HTTP API described in openapi.yaml.
package api

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/katistix/pocketkv/internal/store"
)

//go:embed openapi.yaml
var spec []byte

// New returns the handler for everything under /api/.
func New(db *store.DB) http.Handler {
	h := &handler{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/values", h.list)
	mux.HandleFunc("GET /api/v1/values/{key}", h.get)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return cors(mux)
}

// Spec serves the OpenAPI document.
func Spec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(spec)
}

type handler struct{ db *store.DB }

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	name, ok := h.auth(w, r)
	if !ok {
		return
	}
	s, err := h.db.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "store not found")
		return
	}
	writeJSON(w, s.Values)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	name, ok := h.auth(w, r)
	if !ok {
		return
	}
	v, err := h.db.Value(name, r.PathValue("key"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	writeJSON(w, v)
}

// auth resolves the API key, sent as "Authorization: Bearer <key>", to a
// store name.
func (h *handler) auth(w http.ResponseWriter, r *http.Request) (string, bool) {
	key, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	name, ok := h.db.StoreForAPIKey(key)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "missing or invalid API key")
	}
	return name, ok
}

// cors allows any origin: keys are meant to be embedded in static sites.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
