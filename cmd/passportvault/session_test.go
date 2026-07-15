package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"passportvault/internal/vault"
)

func TestParseArgsDefaultsToSession(t *testing.T) {
	_, command, rest, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if command != "session" {
		t.Fatalf("command = %q, want session", command)
	}
	if len(rest) != 0 {
		t.Fatalf("rest = %#v, want empty", rest)
	}
}

func TestInteractiveSessionSearchRevealAndQuit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.dat")
	v := vault.New()
	if err := v.Add(vault.Entry{Name: "github", Username: "alice", Password: "secret", Notes: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := v.Add(vault.Entry{Name: "gmail", Username: "me@example.com", Password: "mailpass"}); err != nil {
		t.Fatal(err)
	}
	if err := vault.SaveFile(path, v, "master", ""); err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader("2\ngit\n1\nr\n\n8\n")
	output := bytes.NewBuffer(nil)
	cfg := appConfig{vaultPath: path}
	if err := RunInteractiveSession(cfg, "master", input, output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"PassportVault",
		"github\talice",
		"Name: github",
		"Password: ********",
		"Password: secret",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestInteractiveSessionAddPersistsEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.dat")
	if err := vault.SaveFile(path, vault.New(), "master", ""); err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader("3\nbank\nuser1\nbankpass\nprimary\n8\n")
	output := bytes.NewBuffer(nil)
	cfg := appConfig{vaultPath: path}
	if err := RunInteractiveSession(cfg, "master", input, output); err != nil {
		t.Fatal(err)
	}

	loaded, err := vault.LoadFile(path, "master", "")
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := loaded.Find("bank")
	if !ok {
		t.Fatal("bank entry was not saved")
	}
	if entry.Username != "user1" || entry.Password != "bankpass" || entry.Notes != "primary" {
		t.Fatalf("saved entry mismatch: %#v", entry)
	}
}
