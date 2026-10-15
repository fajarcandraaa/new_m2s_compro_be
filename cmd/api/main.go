// Command api runs the Mind2Screen backend: config → db pool → migrate →
// seed → router → http.Server with graceful shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	compro "github.com/fajarcandraaa/new_m2s_compro_be"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/api"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/config"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/db"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/httpx"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/revalidate"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/seed"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("config", "err", err.Error())
		return err
	}
	slog.SetDefault(slog.New(httpx.LogHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.MigrateOnStart {
		if err := db.Migrate(ctx, pool, compro.Migrations); err != nil {
			return err
		}
		slog.Info("migrations applied")
	}
	st := store.New(pool)
	if cfg.SeedOnStart {
		if err := seed.Run(ctx, st, compro.Content, seed.Admin{
			Email: cfg.AdminEmail, Password: cfg.AdminPassword, Name: cfg.AdminName,
		}); err != nil {
			return err
		}
		slog.Info("seed done")
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		slog.Warn("upload dir not writable", "dir", cfg.UploadDir, "err", err.Error())
	}

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: api.NewRouter(api.Deps{
			Store:       st,
			Config:      cfg,
			Revalidator: revalidate.New(cfg.FrontendURL, cfg.RevalidateSecret),
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
