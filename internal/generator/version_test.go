package generator

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		GitCommit = origCommit
		BuildDate = origDate
	}()

	GitCommit = "commit12345"
	BuildDate = "2026-09-09T00:00:00Z"

	vStr := VersionString()
	expectedLines := []string{
		"Mihomo Subscription Publisher",
		"Generator Version: " + GeneratorVersion,
		"Mihomo Version:    " + MihomoVersion,
		"Git Commit:        commit12345",
		"Build Date:        2026-09-09T00:00:00Z",
	}
	for _, expected := range expectedLines {
		if !strings.Contains(vStr, expected) {
			t.Errorf("VersionString() missing %q; got:\n%s", expected, vStr)
		}
	}
}

func TestInitBuildInfo(t *testing.T) {
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		GitCommit = origCommit
		BuildDate = origDate
	}()

	// Reset to unknown and call initBuildInfo
	GitCommit = "unknown"
	BuildDate = "unknown"
	initBuildInfo()

	// In a git repository, GitCommit and BuildDate should either be populated from VCS info
	// or remain "unknown" if build info is not present in test binary.
	if GitCommit == "" {
		t.Error("GitCommit should not be empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}
}
