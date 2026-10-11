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

func TestClearViewWritesWhenEnabled(t *testing.T) {
	output := bytes.NewBuffer(nil)
	s := &session{out: output, clearScreen: true}

	s.clearView()
	s.clearView()

	want := "\x1b[3J\x1b[H\x1b[2J\x1b[3J\x1b[H\x1b[2J"
	if got := output.String(); got != want {
		t.Fatalf("clear output = %q", got)
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

func TestInteractiveSessionWrongPasswordRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.dat")
	if err := vault.SaveFile(path, vault.New(), "correctpass", ""); err != nil {
		t.Fatal(err)
	}

	// 1. Initially wrong password provided
	// 2. Output shows "Error: Incorrect master password" and prompts choice
	// 3. Input "1" (re-enter)
	// 4. Input "correctpass"
	// 5. Input "8" (quit session)
	input := strings.NewReader("1\ncorrectpass\n8\n")
	output := bytes.NewBuffer(nil)
	cfg := appConfig{vaultPath: path}

	if err := RunInteractiveSession(cfg, "wrongpass", input, output); err != nil {
		t.Fatalf("RunInteractiveSession failed: %v", err)
	}

	outText := output.String()
	if !strings.Contains(outText, "Error: Incorrect master password.") {
		t.Fatalf("output missing error message: %s", outText)
	}
	if !strings.Contains(outText, "1. Re-enter password") || !strings.Contains(outText, "2. Exit") {
		t.Fatalf("output missing retry options: %s", outText)
	}
	if !strings.Contains(outText, "PassportVault") {
		t.Fatalf("output missing session menu after unlocking: %s", outText)
	}
}

func TestInteractiveSessionWrongPasswordExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.dat")
	if err := vault.SaveFile(path, vault.New(), "correctpass", ""); err != nil {
		t.Fatal(err)
	}

	// 1. Initially wrong password provided
	// 2. Output shows "Error: Incorrect master password" and prompts choice
	// 3. Input "2" (exit)
	input := strings.NewReader("2\n")
	output := bytes.NewBuffer(nil)
	cfg := appConfig{vaultPath: path}

	// Choosing exit should return nil (clean exit) without entering session
	if err := RunInteractiveSession(cfg, "wrongpass", input, output); err != nil {
		t.Fatalf("RunInteractiveSession should return nil on exit, got: %v", err)
	}

	outText := output.String()
	if !strings.Contains(outText, "Error: Incorrect master password.") {
		t.Fatalf("output missing error message: %s", outText)
	}
	if strings.Contains(outText, "PassportVault") {
		t.Fatalf("session should not have started on exit: %s", outText)
	}
}

