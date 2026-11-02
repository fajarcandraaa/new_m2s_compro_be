// Command retention deletes leads that have not been updated for
// LEAD_RETENTION_MONTHS months. Intended to run as a Dokploy Scheduled Job,
// separately from the API process.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/config"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/db"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/retention"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	os.Exit(run(os.Getenv))
}

// run returns the process exit code. Any config or database failure exits 1 so
// the scheduler records the job as failed instead of success.
func run(getenv func(string) string) int {
	if err := execute(getenv); err != nil {
		slog.Error("retention failed", "err", err.Error())
		return 1
	}
	return 0
}

func execute(getenv func(string) string) error {
	cfg, err := config.LoadRetention(getenv)
	if err != nil {
		return err
	}
	if cfg.Months == 0 {
		slog.Info("retention disabled", "LEAD_RETENTION_MONTHS", 0)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := retention.Run(ctx, store.New(pool), retention.Options{
		Months: cfg.Months,
		DryRun: cfg.DryRun,
	})
	if err != nil {
		return err
	}
	slog.Info("retention run",
		"dry_run", cfg.DryRun,
		"months", cfg.Months,
		"cutoff", res.Cutoff.Format(time.RFC3339),
		"candidates", res.Candidates,
		"deleted", res.Deleted,
	)
	return nil
}
