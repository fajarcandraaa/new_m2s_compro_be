package config

import "testing"

func TestLoadRetention(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c, err := LoadRetention(env(map[string]string{"DATABASE_URL": "postgres://x"}))
		if err != nil {
			t.Fatal(err)
		}
		if c.Months != DefaultLeadRetentionMonths || !c.DryRun {
			t.Fatalf("got months=%d dryRun=%v, want %d/true", c.Months, c.DryRun, DefaultLeadRetentionMonths)
		}
	})

	t.Run("disabled dan live delete", func(t *testing.T) {
		c, err := LoadRetention(env(map[string]string{
			"DATABASE_URL":          "postgres://x",
			"LEAD_RETENTION_MONTHS": "0",
			"DRY_RUN":               "false",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if c.Months != 0 || c.DryRun {
			t.Fatalf("got months=%d dryRun=%v, want 0/false", c.Months, c.DryRun)
		}
	})

	invalid := map[string]map[string]string{
		"missing DATABASE_URL":  {},
		"negative months":       {"DATABASE_URL": "postgres://x", "LEAD_RETENTION_MONTHS": "-1"},
		"non-numeric months":    {"DATABASE_URL": "postgres://x", "LEAD_RETENTION_MONTHS": "dua"},
		"invalid DRY_RUN value": {"DATABASE_URL": "postgres://x", "DRY_RUN": "yes"},
	}
	for name, e := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadRetention(env(e)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
