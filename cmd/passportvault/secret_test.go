package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPasswordCharacterValidation(t *testing.T) {
	validCases := []string{
		"password",
		"P@ssw0rd!#$%",
		"correct horse battery staple",
		"1234567890",
		"~`!@#$%^&*()-_=+[{]}\\|;:'\",<.>/?",
	}
	for _, tc := range validCases {
		if err := validatePassword(tc); err != nil {
			t.Errorf("validatePassword(%q) unexpected error: %v", tc, err)
		}
	}

	invalidCases := []struct {
		input string
		desc  string
	}{
		{"密码123", "Chinese characters"},
		{"パスワード", "Japanese Katakana"},
		{"ひらがな", "Japanese Hiragana"},
		{"한국어", "Korean Hangul"},
		{"pass！word", "Full-width exclamation mark"},
		{"【secret】", "Full-width brackets"},
		{"pass\tword", "Tab control char"},
		{"pass\x00word", "NUL control char"},
		{"pass😀word", "Emoji"},
	}
	for _, tc := range invalidCases {
		if err := validatePassword(tc.input); err == nil {
			t.Errorf("validatePassword(%q [%s]) should have failed but succeeded", tc.input, tc.desc)
		}
	}
}

func TestReadMaskedLoopBasic(t *testing.T) {
	input := strings.NewReader("secret\n")
	output := bytes.NewBuffer(nil)

	got, err := readMaskedLoop("", input, output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Fatalf("got %q, want %q", got, "secret")
	}
	if output.String() != "******" {
		t.Fatalf("output got %q, want %q", output.String(), "******")
	}
}

func TestReadMaskedLoopBackspace(t *testing.T) {
	// Type 's', 'e', 'c', backspace, backspace, 'k', 'e', 'y', enter
	input := strings.NewReader("sec\x7f\x7fkey\n")
	output := bytes.NewBuffer(nil)

	got, err := readMaskedLoop("", input, output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "skey" {
		t.Fatalf("got %q, want %q", got, "skey")
	}
	// 's'(*), 'e'(*), 'c'(*), backspace(\b \b), backspace(\b \b), 'k'(*), 'e'(*), 'y'(*)
	wantOutput := "***\b \b\b \b***"
	if output.String() != wantOutput {
		t.Fatalf("output got %q, want %q", output.String(), wantOutput)
	}
}

func TestReadMaskedLoopBackspaceEmpty(t *testing.T) {
	// Backspacing when nothing entered should not underflow or print backspaces
	input := strings.NewReader("\x7f\x08abc\n")
	output := bytes.NewBuffer(nil)

	got, err := readMaskedLoop("", input, output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc" {
		t.Fatalf("got %q, want %q", got, "abc")
	}
	if output.String() != "***" {
		t.Fatalf("output got %q, want %q", output.String(), "***")
	}
}

func TestReadMaskedLoopDisallowsChineseAndJapanese(t *testing.T) {
	// Type 'p', Chinese '你', 'a', Japanese 'あ', 's', 's', enter
	input := strings.NewReader("p你aあss\n")
	output := bytes.NewBuffer(nil)

	got, err := readMaskedLoop("", input, output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "pass" {
		t.Fatalf("got %q, want %q", got, "pass")
	}
	if output.String() != "****" {
		t.Fatalf("output got %q, want %q", output.String(), "****")
	}
}

func TestReadMaskedLoopNoticeOnDisallowedOnly(t *testing.T) {
	// First input is only Chinese '密码\n' which gets disallowed -> prompt prints notice, then user types 'ok\n'
	input := strings.NewReader("密码\nok\n")
	output := bytes.NewBuffer(nil)

	got, err := readMaskedLoop("Password", input, output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("got %q, want %q", got, "ok")
	}
	outStr := output.String()
	if !strings.Contains(outStr, "Chinese, Japanese") {
		t.Fatalf("expected notice in output, got: %q", outStr)
	}
	if !strings.HasSuffix(outStr, "**") {
		t.Fatalf("expected suffix ** for ok, got: %q", outStr)
	}
}

func TestReadMaskedLoopCtrlU(t *testing.T) {
	// Type 'wrong', Ctrl+U (erase line), 'right', enter
	input := strings.NewReader("wrong\x15right\n")
	output := bytes.NewBuffer(nil)

	got, err := readMaskedLoop("", input, output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "right" {
		t.Fatalf("got %q, want %q", got, "right")
	}
	wantOutput := "*****\b \b\b \b\b \b\b \b\b \b*****"
	if output.String() != wantOutput {
		t.Fatalf("output got %q, want %q", output.String(), wantOutput)
	}
}
