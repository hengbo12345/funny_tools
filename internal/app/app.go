package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/extension"
	"mihomo-sub-publisher/internal/generator"
	"mihomo-sub-publisher/internal/logging"
	"mihomo-sub-publisher/internal/ruleproviders"
	"mihomo-sub-publisher/internal/server"
	"mihomo-sub-publisher/internal/source"
	"mihomo-sub-publisher/internal/storage"
	"mihomo-sub-publisher/internal/token"
)

// App orchestrates the complete lifecycle of the Mihomo Subscription Publisher service.
type App struct {
	configPath string
	tokensPath string

	cfg         *config.Config
	tokenStore  *token.Store
	snapshotMgr *generator.SnapshotManager
	pipeline    *generator.Pipeline
	httpServer  *server.Server
	scheduler   *source.Scheduler
	watcher     *token.Watcher
}

// NewApp creates a new App instance with given configuration paths.
func NewApp(configPath, tokensPath string) *App {
	return &App{
		configPath: configPath,
		tokensPath: tokensPath,
	}
}

// Run executes the application according to §56 lifecycle specifications.
func (a *App) Run() error {
	logger := logging.Logger()
	logger.Info("starting mihomo subscription publisher",
		"generator_version", generator.GeneratorVersion,
		"mihomo_version", generator.MihomoVersion,
	)

	// 1. Load and validate main configuration
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	a.cfg = cfg

	// Ensure directories exist with secure permissions (0700)
	if err := storage.EnsureDir(cfg.Storage.CacheDir, storage.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}
	if err := storage.EnsureDir(cfg.Storage.DataDir, storage.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// 2. Load token state
	statePath := filepath.Join(cfg.Storage.DataDir, "token-state.json")
	a.tokenStore = token.NewStore(a.tokensPath, statePath)
	if err := a.tokenStore.Load(); err != nil {
		return fmt.Errorf("failed to load tokens: %w", err)
	}

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	// Start periodic token state flusher (every 1 min)
	token.StartPeriodicPersistence(appCtx, a.tokenStore, 1*time.Minute)

	// 3. Load on-disk generated snapshot if available
	a.snapshotMgr = generator.NewSnapshotManager()
	if _, err := a.snapshotMgr.Restore(cfg.Storage.CacheDir); err != nil {
		logger.Warn("no previous valid snapshot restored from disk", "error", err)
	}

	// 4. Initialize components for generation pipeline
	fetcher := source.NewFetcher(cfg.Source)
	registry := ruleproviders.NewDefaultRegistry()
	extEngine := extension.NewEngine()
	a.pipeline = generator.NewPipeline(cfg, fetcher, registry, extEngine, a.snapshotMgr)

	// 5. Start HTTP Server
	a.httpServer = server.NewServer(cfg, a.tokenStore, a.snapshotMgr)
	if err := a.httpServer.Start(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// 6. Start Token Reset Watcher
	triggerPath := filepath.Join(cfg.Storage.DataDir, "token-reset-requests.json")
	a.watcher = token.NewWatcher(triggerPath, a.tokenStore, cfg.HotReload.Debounce)
	a.watcher.Start(appCtx)

	// 7. Perform initial source update
	logger.Info("executing initial source subscription update")
	if _, err := a.pipeline.Run(appCtx, nil); err != nil {
		logger.Warn("initial source update failed, relying on restored snapshot if available", "error", err)
	}

	// 8. Start periodic scheduler
	a.scheduler = source.NewScheduler(cfg.Source, func(ctx context.Context) error {
		_, err := a.pipeline.Run(ctx, nil)
		return err
	})
	a.scheduler.Start(appCtx)

	// 9. Wait for termination signals (SIGINT, SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("received termination signal, initiating graceful shutdown", "signal", sig.String())

	// 10. Shutdown sequence (§56)
	// Stop background workers
	cancelApp()

	// Shutdown HTTP Server with timeout
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancelShutdown()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during HTTP server shutdown", "error", err)
	} else {
		logger.Info("HTTP server shut down gracefully")
	}

	// Flush token state to disk
	if err := a.tokenStore.SaveState(); err != nil {
		logger.Error("error flushing token state on shutdown", "error", err)
	} else {
		logger.Info("token state flushed to disk on shutdown")
	}

	logger.Info("mihomo subscription publisher stopped cleanly")
	return nil
}
