package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"mihomo-sub-publisher/internal/app"
	"mihomo-sub-publisher/internal/generator"
	"mihomo-sub-publisher/internal/logging"
)

func main() {
	var (
		configPath  = flag.String("config", "configs/config.yaml", "Path to config.yaml")
		tokensPath  = flag.String("tokens", "configs/tokens.yaml", "Path to tokens.yaml")
		showVersion = flag.Bool("version", false, "Show version information")
		logJSON     = flag.Bool("json-log", true, "Output logs in JSON format")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("Mihomo Subscription Publisher\n")
		fmt.Printf("Generator Version: %s\n", generator.GeneratorVersion)
		fmt.Printf("Mihomo Version:    %s\n", generator.MihomoVersion)
		os.Exit(0)
	}

	logger := logging.Init(os.Stderr, slog.LevelInfo, *logJSON)

	application := app.NewApp(*configPath, *tokensPath)
	if err := application.Run(); err != nil {
		logger.Error("application fatal error", "error", err)
		os.Exit(1)
	}
}
