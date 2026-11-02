// Package retention deletes old leads. It runs from cmd/retention as a
// scheduled job, not from the API process, so a slow delete never touches
// request latency and can be retried independently.
package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// Options controls one retention run.
type Options struct {
	// Months is the retention window in calendar months. 0 disables the job.
	Months int
	// DryRun counts candidates without deleting.
	DryRun bool
	// ExemptStatuses are never deleted. Empty today; use it when a converted
	// status exists so finished leads can be kept.
	ExemptStatuses []string
	// Now overrides the clock in tests. Zero means time.Now().
	Now time.Time
}

// Result reports what a run did.
type Result struct {
	Enabled    bool
	Cutoff     time.Time
	Candidates int64
	Deleted    int64
}

// Run counts leads past the cutoff and, unless DryRun, deletes them. Deleting
// is idempotent: a second run finds nothing left and deletes 0.
func Run(ctx context.Context, st *store.Store, opts Options) (Result, error) {
	if opts.Months <= 0 {
		return Result{Enabled: false}, nil
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := SubtractMonths(now.UTC(), opts.Months)

	candidates, deleted, err := st.PruneLeadsBeforeCutoff(ctx, cutoff, opts.ExemptStatuses, opts.DryRun)
	if err != nil {
		return Result{}, fmt.Errorf("retention: prune: %w", err)
	}
	return Result{Enabled: true, Cutoff: cutoff, Candidates: candidates, Deleted: deleted}, nil
}

// SubtractMonths moves t back by whole calendar months, clamping the day to the
// last day of the target month (31 Mar - 1 month = 28 Feb, not 3 Mar). Using
// calendar months keeps the cutoff aligned with the retention policy; fixed
// 30-day or 730-day arithmetic would drift.
func SubtractMonths(t time.Time, months int) time.Time {
	y, m, d := t.Date()
	target := time.Date(y, m-time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	lastDay := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	if d > lastDay {
		d = lastDay
	}
	return time.Date(target.Year(), target.Month(), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}
