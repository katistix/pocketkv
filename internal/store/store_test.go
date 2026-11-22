package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.json")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateStore("site"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetValue("site", "hero", []byte(`{ "title": "hi" }`), false); err != nil {
		t.Fatal(err)
	}
	k, err := db.CreateAPIKey("site", "blog")
	if err != nil {
		t.Fatal(err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := db2.StoreForAPIKey(k.Key)
	if !ok || name != "site" {
		t.Fatalf("StoreForAPIKey = %q, %v", name, ok)
	}
	v, err := db2.Value("site", "hero")
	if err != nil || string(v) != `{"title":"hi"}` {
		t.Fatalf("Value = %s, %v", v, err)
	}
}

func TestValidation(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "db.json"))
	if db.CreateStore("Bad Name") == nil {
		t.Error("accepted invalid store name")
	}
	db.CreateStore("s")
	if db.SetValue("s", "k", []byte(`{nope`), true) == nil {
		t.Error("accepted invalid JSON")
	}
	if db.SetValue("s", "a/b", []byte(`1`), true) == nil {
		t.Error("accepted invalid key")
	}
	db.SetValue("s", "k", []byte(`1`), false)
	if err := db.SetValue("s", "k", []byte(`2`), false); !errors.Is(err, ErrExists) {
		t.Errorf("create over existing key: %v", err)
	}
}

func TestDeleteStoreRevokesKeys(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "db.json"))
	db.CreateStore("s")
	k, _ := db.CreateAPIKey("s", "")
	db.DeleteStore("s")
	if _, ok := db.StoreForAPIKey(k.Key); ok {
		t.Error("key still valid after store deletion")
	}
}
