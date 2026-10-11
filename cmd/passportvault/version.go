package main

import (
	"fmt"
	"runtime/debug"
)

var (
	// Injected at build time via -ldflags "-X main.commitID=... -X main.buildDate=..."
	commitID  = ""
	buildDate = ""
)

func getCommitID() string {
	if commitID != "" {
		return commitID
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				rev := setting.Value
				if len(rev) > 7 {
					return rev[:7]
				}
				return rev
			}
		}
	}
	return "unknown"
}

func getBuildDate() string {
	if buildDate != "" {
		return buildDate
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				return setting.Value
			}
		}
	}
	return "unknown"
}

func versionString() string {
	return fmt.Sprintf("commit: %s, built: %s", getCommitID(), getBuildDate())
}

func printVersion() {
	fmt.Printf("PassportVault (commit: %s, built: %s)\n", getCommitID(), getBuildDate())
}
