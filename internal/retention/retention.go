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
	// Months is the retention window. 0 disables the job.
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
	cutoff := now.UTC().AddDate(0, -opts.Months, 0)

	candidates, err := st.CountLeadsBeforeCutoff(ctx, cutoff, opts.ExemptStatuses)
	if err != nil {
		return Result{}, fmt.Errorf("retention: count: %w", err)
	}
	res := Result{Enabled: true, Cutoff: cutoff, Candidates: candidates}
	if opts.DryRun {
		return res, nil
	}
	deleted, err := st.DeleteLeadsBeforeCutoff(ctx, cutoff, opts.ExemptStatuses)
	if err != nil {
		return res, fmt.Errorf("retention: delete: %w", err)
	}
	res.Deleted = deleted
	return res, nil
}
