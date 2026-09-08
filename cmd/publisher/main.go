package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"mihomo-sub-publisher/internal/app"
	"mihomo-sub-publisher/internal/generator"
	"mihomo-sub-publisher/internal/logging"
)

var (
	// GitCommit can be set during build with -ldflags "-X main.GitCommit=..."
	GitCommit string
	// BuildDate can be set during build with -ldflags "-X main.BuildDate=..."
	BuildDate string
)

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("publisher", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var (
		configPath  = flags.String("config", "configs/config.yaml", "Path to config.yaml")
		tokensPath  = flags.String("tokens", "configs/tokens.yaml", "Path to tokens.yaml")
		logJSON     = flags.Bool("json-log", true, "Output logs in JSON format")
		showVersion bool
	)
	flags.BoolVar(&showVersion, "v", false, "Show version information")
	flags.BoolVar(&showVersion, "version", false, "Show version information")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if GitCommit != "" {
		generator.GitCommit = GitCommit
	}
	if BuildDate != "" {
		generator.BuildDate = BuildDate
	}

	if showVersion {
		fmt.Fprint(stdout, generator.VersionString())
		return 0
	}

	logger := logging.Init(stderr, slog.LevelInfo, *logJSON)

	application := app.NewApp(*configPath, *tokensPath)
	if err := application.Run(); err != nil {
		logger.Error("application fatal error", "error", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
