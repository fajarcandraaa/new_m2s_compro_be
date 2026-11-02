package api_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

// csvRestore undoes the CSV injection guard so the cell can be re-inserted.
// Hanya sel yang diawali "'" + karakter guard yang diubah.
func csvRestore(v string) string {
	if len(v) >= 2 && v[0] == '\'' && strings.ContainsRune("=+-@\t\r", rune(v[1])) {
		return v[1:]
	}
	return v
}

// TestLeadCSVExportRestoreRoundTrip menjalankan prosedur backup manual:
// export CSV -> hapus baris -> restore dari CSV -> bandingkan semua kolom.
func TestLeadCSVExportRestoreRoundTrip(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	ctx := context.Background()

	id := "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	email := fmt.Sprintf("roundtrip-%d@example.com", time.Now().UnixNano())
	name := "Budi Ünïcode 日本語 🚀"
	company := "PT Contoh, Tbk"
	whatsapp := "+6281234567890"
	slug := "custom-software-development"
	timeframe := "1–3 bulan lagi"
	message := "Butuh sistem cuti & lembur.\nBaris kedua, dengan koma \"dan kutip\"."
	notes := "Follow up, senin\ncatatan \"internal\""
	created := time.Date(2026, 10, 15, 4, 12, 9, 0, time.UTC)
	updated := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)

	_, err := testPool.Exec(ctx, `INSERT INTO leads (id, name, company, email, whatsapp, service_slug,
		start_timeframe, message, status, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'contacted', $9, $10, $11)`,
		id, name, company, email, whatsapp, slug, timeframe, message, notes, created, updated)
	if err != nil {
		t.Fatalf("insert lead: %v", err)
	}
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), "DELETE FROM leads WHERE id = $1", id); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	res := a.do("GET", "/api/v1/admin/leads/export?q="+url.QueryEscape(email), nil)
	expect(t, res, 200)

	const bom = "\xEF\xBB\xBF"
	body := res.Body.Bytes()
	if !bytes.HasPrefix(body, []byte(bom)) {
		t.Fatal("csv must start with UTF-8 BOM")
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(body, []byte(bom)))).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	wantHeader := []string{"createdAt", "name", "company", "email", "whatsapp", "service",
		"startTimeframe", "status", "message", "notes", "id", "updatedAt", "serviceSlug"}
	if len(rows) != 2 {
		t.Fatalf("csv rows = %d, want header + 1", len(rows))
	}
	for i, want := range wantHeader {
		if rows[0][i] != want {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], want)
		}
	}
	rec := map[string]string{}
	for i, k := range wantHeader {
		rec[k] = csvRestore(rows[1][i])
	}
	for field, want := range map[string]string{
		"createdAt": created.Format(time.RFC3339), "name": name, "company": company, "email": email,
		"whatsapp": whatsapp, "startTimeframe": timeframe, "status": "contacted", "message": message,
		"notes": notes, "id": id, "updatedAt": updated.Format(time.RFC3339), "serviceSlug": slug,
	} {
		if rec[field] != want {
			t.Fatalf("csv %s = %q, want %q", field, rec[field], want)
		}
	}
	if rec["service"] == "" {
		t.Fatal("csv service title must be filled from the slug")
	}

	// Hapus, lalu restore manual persis seperti prosedur di docs/lead-retention.md.
	if _, err := testPool.Exec(ctx, "DELETE FROM leads WHERE id = $1", id); err != nil {
		t.Fatalf("delete lead: %v", err)
	}
	var n int
	if err := testPool.QueryRow(ctx, "SELECT count(*) FROM leads WHERE id = $1", id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("lead must be gone after delete (n=%d, err=%v)", n, err)
	}

	createdAt, err := time.Parse(time.RFC3339, rec["createdAt"])
	if err != nil {
		t.Fatalf("parse createdAt: %v", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, rec["updatedAt"])
	if err != nil {
		t.Fatalf("parse updatedAt: %v", err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO leads (id, name, company, email, whatsapp, service_slug,
		start_timeframe, message, status, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		rec["id"], rec["name"], rec["company"], rec["email"], rec["whatsapp"], rec["serviceSlug"],
		rec["startTimeframe"], rec["message"], rec["status"], rec["notes"], createdAt, updatedAt)
	if err != nil {
		t.Fatalf("restore from csv: %v", err)
	}

	var (
		gotName, gotCompany, gotEmail, gotWhatsapp, gotSlug, gotTimeframe, gotStatus, gotMessage, gotNotes string
		gotCreated, gotUpdated                                                                             time.Time
	)
	if err := testPool.QueryRow(ctx, `SELECT name, company, email, whatsapp, service_slug, start_timeframe,
		status, message, notes, created_at, updated_at FROM leads WHERE id = $1`, id).Scan(
		&gotName, &gotCompany, &gotEmail, &gotWhatsapp, &gotSlug, &gotTimeframe,
		&gotStatus, &gotMessage, &gotNotes, &gotCreated, &gotUpdated); err != nil {
		t.Fatalf("read restored lead: %v", err)
	}
	if gotName != name || gotCompany != company || gotEmail != email || gotWhatsapp != whatsapp ||
		gotSlug != slug || gotTimeframe != timeframe || gotStatus != "contacted" ||
		gotMessage != message || gotNotes != notes {
		t.Fatalf("restored fields differ: %+v", []string{gotName, gotCompany, gotEmail, gotWhatsapp,
			gotSlug, gotTimeframe, gotStatus, gotMessage, gotNotes})
	}
	if !gotCreated.Equal(created) || !gotUpdated.Equal(updated) {
		t.Fatalf("restored timestamps: %s / %s", gotCreated, gotUpdated)
	}

	// API harus menyajikan baris hasil restore apa adanya.
	lead := decode[obj](t, a.do("GET", "/api/v1/admin/leads/"+id, nil))
	if lead["message"] != message || lead["notes"] != notes || lead["serviceSlug"] != slug {
		t.Fatalf("api after restore: %v", lead)
	}
}
