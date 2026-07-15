package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.dat")
	v := New()
	entry := Entry{Name: "github", Username: "alice", Password: "secret", Notes: "2fa"}
	if err := v.Add(entry); err != nil {
		t.Fatal(err)
	}

	if err := SaveFile(path, v, "master", ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path, "master", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(loaded.Entries))
	}
	if loaded.Entries[0].Name != "github" || loaded.Entries[0].Password != "secret" {
		t.Fatalf("loaded entry mismatch: %#v", loaded.Entries[0])
	}
}

func TestLoadRejectsWrongPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.dat")
	if err := SaveFile(path, New(), "right", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadFile(path, "wrong", ""); err == nil {
		t.Fatal("LoadFile succeeded with wrong password")
	}
}

func TestLoadRejectsTamperedCiphertext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.dat")
	if err := SaveFile(path, New(), "master", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-8] ^= 1
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadFile(path, "master", ""); err == nil {
		t.Fatal("LoadFile succeeded with tampered ciphertext")
	}
}

func TestKeyFileRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.dat")
	keyPath := filepath.Join(dir, "vault.key")
	wrongKeyPath := filepath.Join(dir, "wrong.key")
	if err := os.WriteFile(keyPath, []byte("key material"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrongKeyPath, []byte("wrong material"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(path, New(), "master", keyPath); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadFile(path, "master", ""); err == nil {
		t.Fatal("LoadFile succeeded without required key file")
	}
	if _, err := LoadFile(path, "master", wrongKeyPath); err == nil {
		t.Fatal("LoadFile succeeded with wrong key file")
	}
	if _, err := LoadFile(path, "master", keyPath); err != nil {
		t.Fatal(err)
	}
}

func TestEntryOperations(t *testing.T) {
	v := New()
	if err := v.Add(Entry{Name: "github", Username: "alice", Password: "old", Notes: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Add(Entry{Name: "mail", Username: "bob", Password: "mailpass"}); err != nil {
		t.Fatal(err)
	}

	results := v.Search("git")
	if len(results) != 1 || results[0].Name != "github" {
		t.Fatalf("Search returned %#v", results)
	}
	if err := v.Update("github", Entry{Name: "github", Username: "alice2", Password: "new", Notes: "two"}); err != nil {
		t.Fatal(err)
	}
	entry, ok := v.Find("github")
	if !ok || entry.Username != "alice2" || entry.Password != "new" {
		t.Fatalf("Find after update returned %#v, %v", entry, ok)
	}
	if err := v.Remove("mail"); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.Find("mail"); ok {
		t.Fatal("removed entry still found")
	}
}
