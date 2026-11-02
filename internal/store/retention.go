package store

import (
	"context"
	"fmt"
	"time"
)

// cutoffWhere matches leads that have not been touched administratively since
// cutoff. updated_at is NOT NULL DEFAULT now() and equals created_at until an
// admin patches the lead, so a lead that was never patched is compared by its
// creation time. Exempted statuses are excluded so converted leads can be kept
// once that status exists.
const cutoffWhere = ` WHERE updated_at < $1 AND status <> ALL($2::text[])`

// CountLeadsBeforeCutoff returns how many leads the retention job would delete.
func (s *Store) CountLeadsBeforeCutoff(ctx context.Context, cutoff time.Time, exemptStatuses []string) (int64, error) {
	if exemptStatuses == nil {
		exemptStatuses = []string{}
	}
	var n int64
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM leads`+cutoffWhere, cutoff, exemptStatuses).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count leads before cutoff: %w", err)
	}
	return n, nil
}

// DeleteLeadsBeforeCutoff permanently deletes leads older than cutoff and
// returns the number of deleted rows.
func (s *Store) DeleteLeadsBeforeCutoff(ctx context.Context, cutoff time.Time, exemptStatuses []string) (int64, error) {
	if exemptStatuses == nil {
		exemptStatuses = []string{}
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM leads`+cutoffWhere, cutoff, exemptStatuses)
	if err != nil {
		return 0, fmt.Errorf("store: delete leads before cutoff: %w", err)
	}
	return tag.RowsAffected(), nil
}
