package main

import "testing"

func TestRun(t *testing.T) {
	t.Run("disabled exits 0", func(t *testing.T) {
		code := run(func(k string) string {
			if k == "DATABASE_URL" {
				return "postgres://user@localhost:5432/db"
			}
			return map[string]string{"LEAD_RETENTION_MONTHS": "0"}[k]
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	})

	t.Run("invalid config exits 1", func(t *testing.T) {
		if code := run(func(string) string { return "" }); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
	})

	t.Run("invalid database url exits 1", func(t *testing.T) {
		code := run(func(k string) string {
			if k == "DATABASE_URL" {
				return "not-a-url"
			}
			return ""
		})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
	})
}
