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

// PruneLeadsBeforeCutoff counts, and unless dryRun deletes, leads matching the
// cutoff. Both statements run in one transaction so the reported candidate and
// deleted counts refer to the same snapshot and a failure leaves nothing
// half-applied.
func (s *Store) PruneLeadsBeforeCutoff(ctx context.Context, cutoff time.Time, exemptStatuses []string, dryRun bool) (candidates, deleted int64, err error) {
	if exemptStatuses == nil {
		exemptStatuses = []string{}
	}
	err = s.Tx(ctx, func(tx *Store) error {
		if err := tx.db.QueryRow(ctx, `SELECT count(*) FROM leads`+cutoffWhere, cutoff, exemptStatuses).Scan(&candidates); err != nil {
			return fmt.Errorf("store: count leads before cutoff: %w", err)
		}
		if dryRun {
			return nil
		}
		tag, err := tx.db.Exec(ctx, `DELETE FROM leads`+cutoffWhere, cutoff, exemptStatuses)
		if err != nil {
			return fmt.Errorf("store: delete leads before cutoff: %w", err)
		}
		deleted = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return candidates, deleted, nil
}
