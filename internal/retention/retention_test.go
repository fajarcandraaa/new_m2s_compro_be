package retention_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	compro "github.com/fajarcandraaa/new_m2s_compro_be"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/db"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/retention"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// Test DB uses its own schema so `go test ./...` can run this package next to
// internal/api, which resets the public schema of the same database.
const schema = "retention_test"

var (
	pool    *pgxpool.Pool
	testSt  *store.Store
	nowBase = time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		return m.Run()
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse config:", err)
		return 1
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open admin pool:", err)
		return 1
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE; CREATE SCHEMA "+schema); err != nil {
		fmt.Fprintln(os.Stderr, "reset schema:", err)
		return 1
	}
	defer admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck

	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open pool:", err)
		return 1
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, compro.Migrations); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}
	testSt = store.New(pool)
	return m.Run()
}

func needDB(t *testing.T) {
	t.Helper()
	if pool == nil {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
}

// insertLead stores a lead with explicit timestamps and removes it afterwards.
func insertLead(t *testing.T, createdAt, updatedAt time.Time, status string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO leads (name, email, whatsapp, message, status, created_at, updated_at)
		VALUES ('Retention Test', $1, '+628000000000', 'test', $2, $3, $4) RETURNING id::text`,
		fmt.Sprintf("retention-%d@example.com", time.Now().UnixNano()), status, createdAt, updatedAt).Scan(&id)
	if err != nil {
		t.Fatalf("insert lead: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM leads WHERE id = $1", id); err != nil {
			t.Errorf("cleanup lead: %v", err)
		}
	})
	return id
}

func leadExists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM leads WHERE id = $1", id).Scan(&n); err != nil {
		t.Fatalf("count lead: %v", err)
	}
	return n == 1
}

func mustRun(t *testing.T, opts retention.Options) retention.Result {
	t.Helper()
	if opts.Now.IsZero() {
		opts.Now = nowBase
	}
	res, err := retention.Run(context.Background(), testSt, opts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res
}

func TestRetentionDeletesLeadPastCutoff(t *testing.T) {
	needDB(t)
	old := insertLead(t, nowBase.AddDate(0, -25, 0), nowBase.AddDate(0, -25, 0), "new")
	recent := insertLead(t, nowBase.AddDate(0, -1, 0), nowBase.AddDate(0, -1, 0), "new")

	res := mustRun(t, retention.Options{Months: 24})

	if res.Cutoff != nowBase.AddDate(0, -24, 0) {
		t.Fatalf("cutoff = %s", res.Cutoff)
	}
	if res.Deleted != 1 || res.Candidates != 1 {
		t.Fatalf("candidates=%d deleted=%d, want 1/1", res.Candidates, res.Deleted)
	}
	if leadExists(t, old) {
		t.Fatal("lead past cutoff must be deleted")
	}
	if !leadExists(t, recent) {
		t.Fatal("lead inside the window must be kept")
	}
}

func TestRetentionCountsLeadWithoutAdministrativeChange(t *testing.T) {
	needDB(t)
	// updated_at sama dengan created_at: belum pernah di-patch admin.
	id := insertLead(t, nowBase.AddDate(0, -30, 0), nowBase.AddDate(0, -30, 0), "new")

	res := mustRun(t, retention.Options{Months: 24, DryRun: true})

	if res.Candidates != 1 {
		t.Fatalf("candidates = %d, want 1", res.Candidates)
	}
	if leadExists(t, id) == false {
		t.Fatal("dry run must not delete")
	}
}

func TestRetentionUsesLastAdministrativeUpdate(t *testing.T) {
	needDB(t)
	// Lead lama, tapi baru di-patch admin: tidak boleh dihapus.
	id := insertLead(t, nowBase.AddDate(0, -30, 0), nowBase.AddDate(0, -2, 0), "contacted")

	res := mustRun(t, retention.Options{Months: 24})

	if res.Candidates != 0 || res.Deleted != 0 {
		t.Fatalf("candidates=%d deleted=%d, want 0/0", res.Candidates, res.Deleted)
	}
	if !leadExists(t, id) {
		t.Fatal("recently updated lead must be kept")
	}
}

func TestRetentionDisabled(t *testing.T) {
	needDB(t)
	id := insertLead(t, nowBase.AddDate(0, -30, 0), nowBase.AddDate(0, -30, 0), "new")

	res := mustRun(t, retention.Options{Months: 0})

	if res.Enabled || res.Deleted != 0 {
		t.Fatalf("enabled=%v deleted=%d, want false/0", res.Enabled, res.Deleted)
	}
	if !leadExists(t, id) {
		t.Fatal("disabled retention must not delete")
	}
}

func TestRetentionDryRunDeletesNothing(t *testing.T) {
	needDB(t)
	id := insertLead(t, nowBase.AddDate(0, -30, 0), nowBase.AddDate(0, -30, 0), "new")

	res := mustRun(t, retention.Options{Months: 24, DryRun: true})

	if res.Candidates != 1 || res.Deleted != 0 {
		t.Fatalf("candidates=%d deleted=%d, want 1/0", res.Candidates, res.Deleted)
	}
	if !leadExists(t, id) {
		t.Fatal("dry run must not delete")
	}
}

func TestRetentionRerunIsIdempotent(t *testing.T) {
	needDB(t)
	id := insertLead(t, nowBase.AddDate(0, -30, 0), nowBase.AddDate(0, -30, 0), "new")

	first := mustRun(t, retention.Options{Months: 24})
	second := mustRun(t, retention.Options{Months: 24})

	if first.Deleted != 1 {
		t.Fatalf("first run deleted = %d, want 1", first.Deleted)
	}
	if second.Candidates != 0 || second.Deleted != 0 {
		t.Fatalf("second run candidates=%d deleted=%d, want 0/0", second.Candidates, second.Deleted)
	}
	if leadExists(t, id) {
		t.Fatal("lead must stay deleted")
	}
}

func TestRetentionExemptsStatuses(t *testing.T) {
	needDB(t)
	id := insertLead(t, nowBase.AddDate(0, -30, 0), nowBase.AddDate(0, -30, 0), "done")

	res := mustRun(t, retention.Options{Months: 24, ExemptStatuses: []string{"done"}})

	if res.Deleted != 0 {
		t.Fatalf("deleted = %d, want 0", res.Deleted)
	}
	if !leadExists(t, id) {
		t.Fatal("exempted status must be kept")
	}
}

func TestRetentionCutoffClampsMonthEnd(t *testing.T) {
	needDB(t)
	now := time.Date(2026, 3, 31, 3, 0, 0, 0, time.UTC)
	// 31 Maret - 1 bulan = 28 Februari. updated_at tepat di cutoff tidak dihapus
	// (perbandingan strictly before), yang sehari lebih lama dihapus.
	createdAt := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	onCutoff := insertLead(t, createdAt, time.Date(2026, 2, 28, 3, 0, 0, 0, time.UTC), "new")
	before := insertLead(t, createdAt, time.Date(2026, 2, 27, 3, 0, 0, 0, time.UTC), "new")

	res := mustRun(t, retention.Options{Months: 1, Now: now})

	if !res.Cutoff.Equal(time.Date(2026, 2, 28, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("cutoff = %s, want 2026-02-28T03:00:00Z", res.Cutoff)
	}
	if res.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", res.Deleted)
	}
	if !leadExists(t, onCutoff) {
		t.Fatal("lead at the cutoff must be kept")
	}
	if leadExists(t, before) {
		t.Fatal("lead before the cutoff must be deleted")
	}
}

func TestRetentionReturnsErrorWhenDatabaseFails(t *testing.T) {
	needDB(t)
	p, err := pgxpool.New(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	p.Close() // query on a closed pool must surface as an error, not a silent no-op

	if _, err := retention.Run(context.Background(), store.New(p), retention.Options{Months: 24}); err == nil {
		t.Fatal("expected error from closed database")
	}
}
