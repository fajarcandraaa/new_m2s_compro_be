package api_test

import (
	"context"
	"testing"

	compro "github.com/fajarcandraaa/new_m2s_compro_be"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/seed"
)

// TestSeedServiceFaqs covers the FAQ backfill: seeded services carry 3-5
// FAQs, an empty list is filled once and admin edits are left alone.
func TestSeedServiceFaqs(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	pub := e.anon()
	ctx := context.Background()

	list, _ := decodeList[obj](t, pub.do("GET", "/api/v1/services", nil))
	byID := map[string]obj{}
	for _, s := range list {
		if n := len(faqList(t, s)); n < 3 || n > 5 {
			t.Fatalf("seeded service %v has %d faqs", s["slug"], n)
		}
	}
	admins, _ := decodeList[obj](t, a.do("GET", "/api/v1/admin/services", nil))
	for _, s := range admins {
		byID[s["slug"].(string)] = s
	}
	runSeed := func() {
		t.Helper()
		if err := seed.Run(ctx, testStore, compro.Content, seed.Admin{Email: testAdminEmail, Password: testAdminPassword}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seeded := faqList(t, byID["it-consultant"])

	// A database from before v1.1 (faqs still []) is backfilled once.
	if _, err := testPool.Exec(ctx, "UPDATE services SET faqs = '[]'::jsonb WHERE slug = 'it-consultant'"); err != nil {
		t.Fatal(err)
	}
	runSeed()
	got := faqList(t, decode[obj](t, pub.do("GET", "/api/v1/services/it-consultant", nil)))
	if len(got) != len(seeded) || got[0] != seeded[0] {
		t.Fatalf("backfill: %v", got)
	}

	// Admin edits are never overwritten.
	svc := byID["ui-ux-design"]
	id := svc["id"].(string)
	body := obj{}
	for _, k := range []string{"slug", "audience", "eyebrow", "title", "tagline", "summary", "points", "priceLabel",
		"priceValue", "audienceText", "deliverables", "systems", "seo", "status"} {
		body[k] = svc[k]
	}
	pk := []obj{}
	for _, p := range svc["packages"].([]any) {
		m := p.(map[string]any)
		delete(m, "sortOrder")
		pk = append(pk, m)
	}
	body["packages"] = pk
	original := svc["faqs"]
	body["faqs"] = []obj{{"question": "Diubah admin?", "answer": "Ya."}}
	expect(t, a.do("PUT", "/api/v1/admin/services/"+id, body), 200)
	runSeed()
	got = faqList(t, decode[obj](t, a.do("GET", "/api/v1/admin/services/"+id, nil)))
	if len(got) != 1 || got[0][0] != "Diubah admin?" {
		t.Fatalf("seed overwrote admin faqs: %v", got)
	}
	body["faqs"] = original
	expect(t, a.do("PUT", "/api/v1/admin/services/"+id, body), 200)
}
