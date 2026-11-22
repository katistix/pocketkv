// Package store holds all KV stores in memory and persists them to a single
// JSON file. Every mutation rewrites the file atomically (temp file + rename).
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

var (
	storeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	valueKeyRe  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._:-]{0,255}$`)
)

// Store is a named collection of JSON values plus the read-only API keys
// that can read it.
type Store struct {
	Name      string                     `json:"name"`
	CreatedAt time.Time                  `json:"created_at"`
	Values    map[string]json.RawMessage `json:"values"`
	APIKeys   []APIKey                   `json:"api_keys"`
}

type APIKey struct {
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

type Summary struct {
	Name      string
	CreatedAt time.Time
	Values    int
	APIKeys   int
}

type DB struct {
	mu       sync.RWMutex
	path     string
	stores   map[string]*Store
	keyIndex map[string]string // API key -> store name
}

type fileFormat struct {
	Stores map[string]*Store `json:"stores"`
}

// Open loads the data file at path, or starts empty if it does not exist.
func Open(path string) (*DB, error) {
	db := &DB{path: path, stores: map[string]*Store{}, keyIndex: map[string]string{}}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return db, nil
	}
	if err != nil {
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for name, s := range f.Stores {
		if s.Values == nil {
			s.Values = map[string]json.RawMessage{}
		}
		// The file is pretty-printed; serve values compact.
		for k, v := range s.Values {
			var buf bytes.Buffer
			if err := json.Compact(&buf, v); err != nil {
				return nil, fmt.Errorf("store %q key %q: %w", name, k, err)
			}
			s.Values[k] = buf.Bytes()
		}
		db.stores[name] = s
		for _, k := range s.APIKeys {
			db.keyIndex[k.Key] = name
		}
	}
	return db, nil
}

// List returns a summary of every store, sorted by name.
func (db *DB) List() []Summary {
	db.mu.RLock()
	defer db.mu.RUnlock()
	out := make([]Summary, 0, len(db.stores))
	for _, s := range db.stores {
		out = append(out, Summary{Name: s.Name, CreatedAt: s.CreatedAt, Values: len(s.Values), APIKeys: len(s.APIKeys)})
	}
	slices.SortFunc(out, func(a, b Summary) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Get returns a copy of the named store.
func (db *DB) Get(name string) (Store, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	s, ok := db.stores[name]
	if !ok {
		return Store{}, ErrNotFound
	}
	c := *s
	c.Values = maps.Clone(s.Values) // values are never mutated in place
	c.APIKeys = slices.Clone(s.APIKeys)
	return c, nil
}

func (db *DB) CreateStore(name string) error {
	if !storeNameRe.MatchString(name) {
		return errors.New("store name must be 1-63 characters: lowercase letters, digits and dashes")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, ok := db.stores[name]; ok {
		return fmt.Errorf("store %q %w", name, ErrExists)
	}
	db.stores[name] = &Store{Name: name, CreatedAt: time.Now().UTC(), Values: map[string]json.RawMessage{}}
	return db.save()
}

func (db *DB) DeleteStore(name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	s, ok := db.stores[name]
	if !ok {
		return ErrNotFound
	}
	for _, k := range s.APIKeys {
		delete(db.keyIndex, k.Key)
	}
	delete(db.stores, name)
	return db.save()
}

// SetValue stores value (which must be valid JSON) under key. If overwrite is
// false and the key already exists, it returns ErrExists.
func (db *DB) SetValue(name, key string, value []byte, overwrite bool) error {
	if !valueKeyRe.MatchString(key) {
		return errors.New("key must be 1-256 characters: letters, digits and . _ : - (starting with a letter, digit or _)")
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, value); err != nil {
		return fmt.Errorf("value is not valid JSON: %w", err)
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	s, ok := db.stores[name]
	if !ok {
		return ErrNotFound
	}
	if _, exists := s.Values[key]; exists && !overwrite {
		return fmt.Errorf("key %q %w", key, ErrExists)
	}
	s.Values[key] = json.RawMessage(buf.Bytes())
	return db.save()
}

func (db *DB) DeleteValue(name, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	s, ok := db.stores[name]
	if !ok {
		return ErrNotFound
	}
	if _, ok := s.Values[key]; !ok {
		return ErrNotFound
	}
	delete(s.Values, key)
	return db.save()
}

// Value returns the JSON value for key in the named store.
func (db *DB) Value(name, key string) (json.RawMessage, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	s, ok := db.stores[name]
	if !ok {
		return nil, ErrNotFound
	}
	v, ok := s.Values[key]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}

// CreateAPIKey generates a new read-only API key for the named store.
func (db *DB) CreateAPIKey(name, label string) (APIKey, error) {
	label = strings.TrimSpace(label)
	if len(label) > 64 {
		return APIKey{}, errors.New("label must be at most 64 characters")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	s, ok := db.stores[name]
	if !ok {
		return APIKey{}, ErrNotFound
	}
	k := APIKey{Key: "pkv_" + rand.Text(), Label: label, CreatedAt: time.Now().UTC()}
	s.APIKeys = append(s.APIKeys, k)
	db.keyIndex[k.Key] = name
	return k, db.save()
}

func (db *DB) DeleteAPIKey(name, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	s, ok := db.stores[name]
	if !ok || db.keyIndex[key] != name {
		return ErrNotFound
	}
	s.APIKeys = slices.DeleteFunc(s.APIKeys, func(k APIKey) bool { return k.Key == key })
	delete(db.keyIndex, key)
	return db.save()
}

// StoreForAPIKey returns the name of the store an API key grants access to.
func (db *DB) StoreForAPIKey(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	name, ok := db.keyIndex[key]
	return name, ok
}

// save writes all stores to disk atomically. The caller must hold db.mu.
func (db *DB) save() error {
	data, err := json.MarshalIndent(fileFormat{Stores: db.stores}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(db.path), ".pocketkv-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), db.path)
}
