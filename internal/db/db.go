// Package db sets up the pgx connection pool and runs the embedded goose
// migrations.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// goose.WithSessionLocker uses PostgreSQL advisory locks so concurrent
// instances serialize migrations safely (multi-instance Swarm replicas).

const (
	connectMaxAttempts = 5
	connectBackoff     = 5 * time.Second
)

// Connect opens a pgx pool and verifies the connection, retrying up to 5
// times with a 5 s backoff. This handles the window where the external
// database is briefly unreachable at container startup.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute

	var (
		pool    *pgxpool.Pool
		lastErr error
	)
	for attempt := 1; attempt <= connectMaxAttempts; attempt++ {
		pool, lastErr = pgxpool.NewWithConfig(ctx, cfg)
		if lastErr != nil {
			slog.Warn("db: open pool failed, retrying", "attempt", attempt, "err", lastErr)
		} else {
			pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			lastErr = pool.Ping(pingCtx)
			cancel()
			if lastErr == nil {
				return pool, nil
			}
			pool.Close()
			slog.Warn("db: ping failed, retrying", "attempt", attempt, "err", lastErr)
		}
		if attempt < connectMaxAttempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("db: connect cancelled: %w", ctx.Err())
			case <-time.After(connectBackoff):
			}
		}
	}
	return nil, fmt.Errorf("db: connect failed after %d attempts: %w", connectMaxAttempts, lastErr)
}

// Migrate runs every pending goose migration found in fsys (directory
// "migrations") against the pool.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	sub, err := fs.Sub(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("db: migrations dir: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("db: session locker: %w", err)
	}
	provider, err := goose.NewProvider(
		goose.DialectPostgres, sqlDB, sub,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return fmt.Errorf("db: goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	return nil
}
