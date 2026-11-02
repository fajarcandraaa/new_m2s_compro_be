package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Retention is the configuration for cmd/retention, the lead-retention job
// that runs as a scheduled task outside the API process. It is separate from
// Config so the job does not need JWT_SECRET or the other API settings.
type Retention struct {
	DatabaseURL string
	// Months is the retention window applied to leads.updated_at.
	// 0 disables the job.
	Months int
	// DryRun only counts candidates; it never deletes.
	DryRun bool
}

// DefaultLeadRetentionMonths is used when LEAD_RETENTION_MONTHS is unset.
const DefaultLeadRetentionMonths = 24

// LoadRetention reads the job configuration through getenv (os.Getenv in
// production) and fails fast on invalid values.
func LoadRetention(getenv func(string) string) (*Retention, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	c := &Retention{DatabaseURL: get("DATABASE_URL", "")}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	months, err := strconv.Atoi(get("LEAD_RETENTION_MONTHS", strconv.Itoa(DefaultLeadRetentionMonths)))
	if err != nil || months < 0 {
		errs = append(errs, fmt.Errorf("LEAD_RETENTION_MONTHS: invalid value %q (use 0 to disable)", getenv("LEAD_RETENTION_MONTHS")))
	}
	c.Months = months

	// Default true: a job that was never explicitly cleared for deletion must
	// not delete anything. Set DRY_RUN=false only after the dry-run counts and
	// the backup have been reviewed.
	raw := get("DRY_RUN", "true")
	dryRun, err := strconv.ParseBool(raw)
	if err != nil {
		errs = append(errs, fmt.Errorf("DRY_RUN: invalid boolean %q", raw))
	}
	c.DryRun = dryRun

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return c, nil
}
