package api

import (
	"strings"
	"testing"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

func TestLeadsCSV(t *testing.T) {
	title := "Web Development"
	slug := "web-development"
	notes := "@catatan"
	leads := []domain.Lead{
		{ID: "lead-1", CreatedAt: time.Date(2026, 10, 15, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600)),
			UpdatedAt: time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC), Name: "=cmd|' /C calc'!A0",
			Email: "a@b.co", Whatsapp: "+6281234567890", ServiceSlug: &slug, ServiceTitle: &title, Status: "new",
			Message: "Halo, \"kami\"\nbutuh web", Notes: &notes},
		{ID: "lead-2", CreatedAt: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
			Name:      "-1", Email: "c@d.co", Whatsapp: "+14155552671",
			ServiceSlug: &slug, Status: "done", Message: "\tTab"},
	}
	b, err := LeadsCSV(leads)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	want := "\xEF\xBB\xBFcreatedAt,name,company,email,whatsapp,service,startTimeframe,status,message,notes,id,updatedAt,serviceSlug\r\n" +
		"2026-10-15T03:00:00Z,'=cmd|' /C calc'!A0,,a@b.co,'+6281234567890,Web Development,,new,\"Halo, \"\"kami\"\"\r\nbutuh web\",'@catatan,lead-1,2026-10-16T03:00:00Z,web-development\r\n" +
		"2026-10-14T00:00:00Z,'-1,,c@d.co,'+14155552671,web-development,,done,'\tTab,,lead-2,2026-10-15T00:00:00Z,web-development\r\n"
	if s != want {
		t.Fatalf("csv mismatch:\n got %q\nwant %q", s, want)
	}
	if !strings.HasSuffix(s, "\r\n") {
		t.Fatal("CRLF")
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"": "", "abc": "abc", "=1": "'=1", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx"} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q", in, got)
		}
	}
}
