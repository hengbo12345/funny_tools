package main

import (
	"bytes"
	"strings"
	"testing"

	"mihomo-sub-publisher/internal/generator"
)

func TestVersionFlagV(t *testing.T) {
	origCommit := generator.GitCommit
	origDate := generator.BuildDate
	defer func() {
		generator.GitCommit = origCommit
		generator.BuildDate = origDate
	}()

	generator.GitCommit = "abc12345"
	generator.BuildDate = "2026-09-09T00:00:00Z"

	var stdout, stderr bytes.Buffer
	code := run([]string{"-v"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	out := stdout.String()
	for _, expected := range []string{
		"Mihomo Subscription Publisher",
		"Generator Version: " + generator.GeneratorVersion,
		"Mihomo Version:    " + generator.MihomoVersion,
		"Git Commit:        abc12345",
		"Build Date:        2026-09-09T00:00:00Z",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("output missing %q; got:\n%s", expected, out)
		}
	}
}

func TestVersionFlagVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "Git Commit:") {
		t.Errorf("expected 'Git Commit:' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Build Date:") {
		t.Errorf("expected 'Build Date:' in output, got:\n%s", out)
	}
}

func TestVersionFlagDoubleDashVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "Mihomo Subscription Publisher") {
		t.Errorf("expected 'Mihomo Subscription Publisher' in output, got:\n%s", out)
	}
}

func TestVersionLdflagsInMain(t *testing.T) {
	origCommit := GitCommit
	origDate := BuildDate
	origGenCommit := generator.GitCommit
	origGenDate := generator.BuildDate
	defer func() {
		GitCommit = origCommit
		BuildDate = origDate
		generator.GitCommit = origGenCommit
		generator.BuildDate = origGenDate
	}()

	GitCommit = "custom-commit-sha"
	BuildDate = "2026-09-09T12:34:56Z"

	var stdout, stderr bytes.Buffer
	code := run([]string{"-v"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "Git Commit:        custom-commit-sha") {
		t.Errorf("expected injected GitCommit, got:\n%s", out)
	}
	if !strings.Contains(out, "Build Date:        2026-09-09T12:34:56Z") {
		t.Errorf("expected injected BuildDate, got:\n%s", out)
	}
}

func TestInvalidFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-non-existent-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2 for invalid flag, got %d", code)
	}
}
