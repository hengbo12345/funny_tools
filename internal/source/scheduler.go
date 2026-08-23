package source

import (
	"context"
	"time"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/logging"
)

// Scheduler coordinates periodic source subscription updates.
type Scheduler struct {
	cfg      config.SourceConfig
	onUpdate func(ctx context.Context) error
}

// NewScheduler creates a new source subscription scheduler.
func NewScheduler(cfg config.SourceConfig, onUpdate func(ctx context.Context) error) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		onUpdate: onUpdate,
	}
}

// Start begins the scheduling loop.
func (s *Scheduler) Start(ctx context.Context) {
	interval := s.cfg.Interval
	if interval <= 0 {
		interval = 1 * time.Hour
	}

	go func() {
		logger := logging.Logger()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Info("scheduler triggered source update")
				if err := s.onUpdate(ctx); err != nil {
					logger.Error("scheduled source update failed", "error", err)
				}
			}
		}
	}()
}
