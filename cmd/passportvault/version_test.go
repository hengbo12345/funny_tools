package main

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	v := versionString()
	if !strings.Contains(v, "commit:") {
		t.Fatalf("versionString missing commit: %s", v)
	}
	if !strings.Contains(v, "built:") {
		t.Fatalf("versionString missing built: %s", v)
	}
}

func TestParseArgsVersion(t *testing.T) {
	cases := [][]string{
		{"version"},
		{"-version"},
		{"--version"},
		{"-v"},
	}
	for _, args := range cases {
		_, cmd, _, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%v) error: %v", args, err)
		}
		if cmd != "version" {
			t.Fatalf("parseArgs(%v) command = %q, want %q", args, cmd, "version")
		}
	}
}

func TestRunVersionCommand(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Fatalf("run(version) error: %v", err)
	}
	if err := run([]string{"-version"}); err != nil {
		t.Fatalf("run(-version) error: %v", err)
	}
}
