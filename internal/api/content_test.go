package api_test

import (
	"strings"
	"testing"
	"time"
)

func serviceBody(title string) obj {
	return obj{
		"audience": "business", "eyebrow": "", "title": title, "tagline": "", "summary": "",
		"points": []string{"Satu"}, "priceLabel": "", "priceValue": "", "audienceText": "",
		"deliverables": []obj{{"title": "D", "body": ""}}, "systems": []string{},
		"packages": []obj{}, "seo": obj{"title": nil, "description": nil, "ogImageUrl": nil}, "status": "draft",
	}
}

func TestServicesAdmin(t *testing.T) {
	e := newEnv(t)
	a := e.admin()

	b := serviceBody("Layanan Uji")
	b["packages"] = []obj{
		{"name": "A", "priceLabel": "", "priceValue": "1", "description": "", "features": []string{}, "highlighted": true},
		{"name": "B", "priceLabel": "", "priceValue": "", "description": "", "features": []string{""}, "highlighted": true},
	}
	b["points"] = []string{"ok", ""}
	b["seo"] = obj{"title": strings.Repeat("x", 71), "description": nil, "ogImageUrl": "javascript:alert(1)"}
	er := expectErr(t, a.do("POST", "/api/v1/admin/services", b), 422, "validation_error",
		"packages", "packages.1.priceValue", "packages.1.features.0", "points.1", "seo.title", "seo.ogImageUrl")
	_ = er

	b = serviceBody("Layanan Uji")
	b["packages"] = []obj{
		{"name": "A", "priceLabel": "Mulai", "priceValue": "Rp 1", "description": "", "features": []string{"f1"}, "highlighted": false},
		{"name": "B", "priceLabel": "", "priceValue": "Hubungi Kami", "description": "", "features": []string{}, "highlighted": true},
	}
	res := a.do("POST", "/api/v1/admin/services", b)
	expect(t, res, 201)
	if got := e.reval.last(); strings.Join(got, ",") != "services,promos" {
		t.Fatalf("revalidate: %v", got)
	}
	svc := decode[obj](t, res)
	id := svc["id"].(string)
	if svc["slug"] != "layanan-uji" || svc["sortOrder"].(float64) != 6 {
		t.Fatalf("created service: slug=%v sortOrder=%v", svc["slug"], svc["sortOrder"])
	}
	pk := svc["packages"].([]any)
	if len(pk) != 2 || pk[1].(map[string]any)["sortOrder"].(float64) != 2 {
		t.Fatalf("packages: %v", pk)
	}
	// Draft hidden publicly.
	expectErr(t, e.anon().do("GET", "/api/v1/services/layanan-uji", nil), 404, "not_found")

	// PUT keeps slug; packages replaced.
	b["title"] = "Layanan Uji Diubah"
	b["packages"] = []obj{}
	b["status"] = "published"
	res = a.do("PUT", "/api/v1/admin/services/"+id, b)
	expect(t, res, 200)
	if got := decode[obj](t, res); got["slug"] != "layanan-uji" || len(got["packages"].([]any)) != 0 {
		t.Fatalf("PUT: %v", got)
	}
	expect(t, e.anon().do("GET", "/api/v1/services/layanan-uji", nil), 200)

	// Reorder: must contain every id.
	list, meta := decodeList[obj](t, a.do("GET", "/api/v1/admin/services", nil))
	if meta.Total != len(list) || meta.Page != 1 {
		t.Fatalf("admin services meta: %+v", meta)
	}
	ids := make([]string, len(list))
	for i, s := range list {
		ids[len(list)-1-i] = s["id"].(string)
	}
	expectErr(t, a.do("PUT", "/api/v1/admin/services/order", obj{"ids": ids[1:]}), 422, "validation_error", "ids")
	expectErr(t, a.do("PUT", "/api/v1/admin/services/order", obj{"ids": append([]string{ids[0]}, ids...)}), 422, "validation_error", "ids")
	expectErr(t, a.do("PUT", "/api/v1/admin/services/order", obj{"ids": []string{"x"}}), 422, "validation_error")
	res = a.do("PUT", "/api/v1/admin/services/order", obj{"ids": ids})
	expect(t, res, 200)
	reordered, _ := decodeList[obj](t, res)
	if reordered[0]["id"] != id || reordered[0]["sortOrder"].(float64) != 1 {
		t.Fatalf("reorder: first=%v", reordered[0]["id"])
	}
	// Restore order.
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	expect(t, a.do("PUT", "/api/v1/admin/services/order", obj{"ids": ids}), 200)
	expect(t, a.do("GET", "/api/v1/admin/services?status=published&audience=business", nil), 200)
	expect(t, a.do("GET", "/api/v1/admin/services/"+id, nil), 200)

	expect(t, a.do("DELETE", "/api/v1/admin/services/"+id, nil), 204)
	expectErr(t, a.do("GET", "/api/v1/admin/services/"+id, nil), 404, "not_found")
}

func caseStudyBody(title string, featured bool, status string) obj {
	return obj{
		"systemName": "UJI", "category": "HRIS", "featured": featured, "title": title, "summary": "",
		"cardBody": "", "metrics": []obj{{"value": "1", "label": "a"}, {"value": "2", "label": "b"}},
		"scope": []string{}, "initialConditionsTitle": "", "initialConditions": []string{}, "risksTitle": "",
		"risks": []string{}, "approachTitle": "", "approach": []string{}, "solutions": []string{},
		"impactTitle": "", "impacts": []string{}, "lesson": "",
		"seo": obj{"title": nil, "description": nil, "ogImageUrl": nil}, "status": status,
	}
}

func TestFeaturedCaseStudiesMaxTwo(t *testing.T) {
	e := newEnv(t)
	a := e.admin()

	// Seed already has 2 featured published case studies.
	expectErr(t, a.do("POST", "/api/v1/admin/case-studies", caseStudyBody("Ketiga", true, "published")), 409, "conflict", "featured")

	// A featured draft is allowed; publishing it is checked.
	res := a.do("POST", "/api/v1/admin/case-studies", caseStudyBody("Ketiga", true, "draft"))
	expect(t, res, 201)
	third := decode[obj](t, res)
	id := third["id"].(string)
	expectErr(t, a.do("PUT", "/api/v1/admin/case-studies/"+id, caseStudyBody("Ketiga", true, "published")), 409, "conflict", "featured")

	// Un-feature a seeded one, then publishing works.
	list, _ := decodeList[obj](t, a.do("GET", "/api/v1/admin/case-studies?q=TUMBUH", nil))
	if len(list) != 1 {
		t.Fatalf("q search: %d", len(list))
	}
	seeded := list[0]
	seededID := seeded["id"].(string)
	for _, k := range []string{"id", "slug", "createdAt", "updatedAt", "sortOrder"} {
		delete(seeded, k)
	}
	seeded["featured"] = false
	res = a.do("PUT", "/api/v1/admin/case-studies/"+seededID, seeded)
	expect(t, res, 200)
	if got := decode[obj](t, res); got["slug"] != "tumbuh-sistem-pertumbuhan-sdm" || got["sortOrder"].(float64) != 1 {
		t.Fatalf("PUT without slug/sortOrder must keep them: %v %v", got["slug"], got["sortOrder"])
	}
	expect(t, a.do("PUT", "/api/v1/admin/case-studies/"+id, caseStudyBody("Ketiga", true, "published")), 200)
	feat, _ := decodeList[obj](t, e.anon().do("GET", "/api/v1/case-studies?featured=true", nil))
	if len(feat) != 2 {
		t.Fatalf("featured public: %d", len(feat))
	}

	// Metrics must be exactly 2.
	bad := caseStudyBody("Metrik", false, "draft")
	bad["metrics"] = []obj{{"value": "", "label": "a"}}
	expectErr(t, a.do("POST", "/api/v1/admin/case-studies", bad), 422, "validation_error", "metrics", "metrics.0.value")

	// Restore seed state.
	expect(t, a.do("DELETE", "/api/v1/admin/case-studies/"+id, nil), 204)
	seeded["featured"] = true
	expect(t, a.do("PUT", "/api/v1/admin/case-studies/"+seededID, seeded), 200)
	if got := e.reval.last(); len(got) != 1 || got[0] != "case-studies" {
		t.Fatalf("revalidate: %v", got)
	}
}

func promoBody(title string, start, end time.Time, serviceID any) obj {
	return obj{"title": title, "serviceId": serviceID, "description": "", "code": nil,
		"startsAt": start.Format(time.RFC3339), "endsAt": end.Format(time.RFC3339)}
}

func TestPromoStatusAndWindow(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	pub := e.anon()
	now := time.Now().UTC().Truncate(time.Second)
	day := 24 * time.Hour

	// A draft service for the "service: null" rule.
	res := a.do("POST", "/api/v1/admin/services", serviceBody("Layanan Draft Promo"))
	expect(t, res, 201)
	draftSvc := decode[obj](t, res)["id"].(string)

	ids := map[string]string{}
	for name, b := range map[string]obj{
		"active":      promoBody("Aktif", now.Add(-day), now.Add(5*day), draftSvc),
		"active-soon": promoBody("Aktif Segera Berakhir", now.Add(-day), now.Add(2*day), nil),
		"upcoming":    promoBody("Akan Datang", now.Add(3*day), now.Add(9*day), nil),
		"ended-3d":    promoBody("Berakhir 3 hari", now.Add(-20*day), now.Add(-3*day), nil),
		"ended-10d":   promoBody("Berakhir 10 hari", now.Add(-30*day), now.Add(-10*day), nil),
	} {
		res := a.do("POST", "/api/v1/admin/promos", b)
		expect(t, res, 201)
		ids[name] = decode[obj](t, res)["id"].(string)
	}
	if got := e.reval.last(); len(got) != 1 || got[0] != "promos" {
		t.Fatalf("revalidate: %v", got)
	}

	list, _ := decodeList[obj](t, pub.do("GET", "/api/v1/promos", nil))
	var order []string
	for _, p := range list {
		order = append(order, p["title"].(string)+":"+p["status"].(string))
		if p["id"] == ids["active"] && p["service"] != nil {
			t.Fatalf("draft service must be null publicly: %v", p["service"])
		}
		if p["id"] == ids["ended-10d"] {
			t.Fatal("promo ended 10 days ago must be hidden")
		}
	}
	want := "Aktif Segera Berakhir:active,Aktif:active,Akan Datang:upcoming,Berakhir 3 hari:ended"
	if strings.Join(order, ",") != want {
		t.Fatalf("public promo order:\n got %s\nwant %s", strings.Join(order, ","), want)
	}
	ended, _ := decodeList[obj](t, pub.do("GET", "/api/v1/promos?status=ended", nil))
	if len(ended) != 1 || ended[0]["id"] != ids["ended-3d"] {
		t.Fatalf("ended filter: %v", ended)
	}
	active, _ := decodeList[obj](t, pub.do("GET", "/api/v1/promos?status=active", nil))
	if len(active) != 2 {
		t.Fatalf("active filter: %d", len(active))
	}

	adminAll, meta := decodeList[obj](t, a.do("GET", "/api/v1/admin/promos?pageSize=100", nil))
	if meta.Total < 5 {
		t.Fatalf("admin promos must include all ended: %d", meta.Total)
	}
	for _, p := range adminAll {
		if p["id"] == ids["active"] && (p["service"] == nil || p["serviceId"] != draftSvc) {
			t.Fatalf("admin promo service must be shown regardless of status: %v", p)
		}
	}
	expect(t, a.do("GET", "/api/v1/admin/promos?status=ended&q=Berakhir", nil), 200)

	// endsAt must be after startsAt; offsets are accepted and stored as UTC.
	expectErr(t, a.do("POST", "/api/v1/admin/promos", promoBody("Salah", now, now, nil)), 422, "validation_error", "endsAt")
	b := promoBody("WIB", now, now.Add(day), nil)
	b["startsAt"] = "2026-11-01T00:00:00+07:00"
	b["endsAt"] = "2026-11-08T00:00:00+07:00"
	b["code"] = "HEMAT-10"
	res = a.do("PUT", "/api/v1/admin/promos/"+ids["upcoming"], b)
	expect(t, res, 200)
	if got := decode[obj](t, res); got["startsAt"] != "2026-10-31T17:00:00Z" || got["code"] != "HEMAT-10" {
		t.Fatalf("utc conversion: %v %v", got["startsAt"], got["code"])
	}
	b["code"] = "bad code!"
	b["serviceId"] = "not-a-uuid"
	expectErr(t, a.do("PUT", "/api/v1/admin/promos/"+ids["upcoming"], b), 422, "validation_error", "code", "serviceId")
	b["code"] = nil
	b["serviceId"] = "00000000-0000-4000-8000-000000000000"
	expectErr(t, a.do("PUT", "/api/v1/admin/promos/"+ids["upcoming"], b), 422, "validation_error", "serviceId")

	// Deleting a service sets promo.service_id null.
	expect(t, a.do("DELETE", "/api/v1/admin/services/"+draftSvc, nil), 204)
	got := decode[obj](t, a.do("GET", "/api/v1/admin/promos/"+ids["active"], nil))
	if got["serviceId"] != nil || got["service"] != nil {
		t.Fatalf("service delete should null the promo service: %v", got)
	}
	for _, id := range ids {
		expect(t, a.do("DELETE", "/api/v1/admin/promos/"+id, nil), 204)
	}
}

func TestTestimonialsLogosAndToggles(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	pub := e.anon()

	var tIDs []string
	for i, active := range []bool{true, true, false, true} {
		res := a.do("POST", "/api/v1/admin/testimonials", obj{"clientName": "Klien " + string(rune('A'+i)),
			"clientRole": "Direktur", "company": "", "quote": "Kutipan.", "isActive": active})
		expect(t, res, 201)
		tt := decode[obj](t, res)
		if tt["company"] != nil {
			t.Fatalf(`"" must become null: %v`, tt["company"])
		}
		tIDs = append(tIDs, tt["id"].(string))
	}
	if got := e.reval.last(); len(got) != 1 || got[0] != "testimonials" {
		t.Fatalf("revalidate: %v", got)
	}
	list, meta := decodeList[obj](t, pub.do("GET", "/api/v1/testimonials", nil))
	if len(list) != 3 || meta.Total != 3 {
		t.Fatalf("public testimonials must exclude inactive: %d", len(list))
	}
	top, _ := decodeList[obj](t, pub.do("GET", "/api/v1/testimonials?limit=2", nil))
	if len(top) != 2 || top[0]["id"] != tIDs[0] {
		t.Fatalf("limit/sort: %v", top)
	}
	inactive, _ := decodeList[obj](t, a.do("GET", "/api/v1/admin/testimonials?isActive=false", nil))
	if len(inactive) != 1 {
		t.Fatalf("admin isActive filter: %d", len(inactive))
	}
	rev := []string{tIDs[3], tIDs[2], tIDs[1], tIDs[0]}
	res := a.do("PUT", "/api/v1/admin/testimonials/order", obj{"ids": rev})
	expect(t, res, 200)
	if l, _ := decodeList[obj](t, res); l[0]["id"] != tIDs[3] {
		t.Fatal("testimonial reorder failed")
	}
	expect(t, a.do("PUT", "/api/v1/admin/testimonials/"+tIDs[2], obj{"clientName": "Klien C", "clientRole": "",
		"company": "PT Uji", "quote": "Baru.", "isActive": true}), 200)
	expectErr(t, a.do("PUT", "/api/v1/admin/testimonials/"+tIDs[2], obj{"clientName": "", "clientRole": "",
		"company": nil, "quote": "", "isActive": true}), 422, "validation_error", "clientName", "quote")
	expect(t, a.do("GET", "/api/v1/admin/testimonials/"+tIDs[2], nil), 200)

	// Client logos.
	expectErr(t, a.do("POST", "/api/v1/admin/client-logos", obj{"name": "X", "imageUrl": "javascript:alert(1)"}), 422, "validation_error", "imageUrl")
	var lIDs []string
	for _, u := range []string{"/uploads/aaa.png", "https://cdn.example.com/logo.webp"} {
		res := a.do("POST", "/api/v1/admin/client-logos", obj{"name": "Logo", "imageUrl": u})
		expect(t, res, 201)
		lIDs = append(lIDs, decode[obj](t, res)["id"].(string))
	}
	if got := e.reval.last(); len(got) != 1 || got[0] != "client-logos" {
		t.Fatalf("revalidate: %v", got)
	}
	logos, _ := decodeList[obj](t, pub.do("GET", "/api/v1/client-logos", nil))
	if len(logos) != 2 {
		t.Fatalf("logos: %d", len(logos))
	}
	expect(t, a.do("PUT", "/api/v1/admin/client-logos/order", obj{"ids": []string{lIDs[1], lIDs[0]}}), 200)
	expect(t, a.do("PUT", "/api/v1/admin/client-logos/"+lIDs[0], obj{"name": "Logo A", "imageUrl": "/uploads/bbb.jpg"}), 200)
	expect(t, a.do("GET", "/api/v1/admin/client-logos", nil), 200)
	expect(t, a.do("GET", "/api/v1/admin/client-logos/"+lIDs[0], nil), 200)

	// Toggles (PATCH semantics).
	expectErr(t, a.do("PATCH", "/api/v1/admin/settings/toggles", obj{}), 422, "validation_error")
	expectErr(t, a.do("PATCH", "/api/v1/admin/settings/toggles", obj{"showTestimonials": nil}), 422, "validation_error", "showTestimonials")
	res = a.do("PATCH", "/api/v1/admin/settings/toggles", obj{"showClientLogos": true})
	expect(t, res, 200)
	st := decode[obj](t, res)
	if st["showClientLogos"] != true || st["showTestimonials"] != true {
		t.Fatalf("toggles partial update: %v", st)
	}
	if got := e.reval.last(); len(got) != 1 || got[0] != "site" {
		t.Fatalf("revalidate: %v", got)
	}
	site := decode[obj](t, pub.do("GET", "/api/v1/site", nil))
	if site["showClientLogos"] != true {
		t.Fatal("public site does not reflect toggle")
	}
	expect(t, a.do("PATCH", "/api/v1/admin/settings/toggles", obj{"showClientLogos": false, "showTestimonials": true}), 200)

	for _, id := range tIDs {
		expect(t, a.do("DELETE", "/api/v1/admin/testimonials/"+id, nil), 204)
	}
	for _, id := range lIDs {
		expect(t, a.do("DELETE", "/api/v1/admin/client-logos/"+id, nil), 204)
	}
}

func TestSettingsUpdate(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	cur := decode[obj](t, a.do("GET", "/api/v1/admin/settings", nil))
	for _, k := range []string{"updatedAt", "showTestimonials", "showClientLogos"} {
		delete(cur, k)
	}
	orig := obj{}
	for k, v := range cur {
		orig[k] = v
	}
	cur["whatsappNumber"] = "0822-3010-9923"
	cur["mapsUrl"] = "https://maps.example.com/x"
	cur["linkedinUrl"] = ""
	res := a.do("PUT", "/api/v1/admin/settings", cur)
	expect(t, res, 200)
	got := decode[obj](t, res)
	if got["whatsappNumber"] != "+6282230109923" || got["linkedinUrl"] != nil || got["mapsUrl"] != "https://maps.example.com/x" {
		t.Fatalf("settings normalisation: %v", got)
	}
	if r := e.reval.last(); len(r) != 1 || r[0] != "site" {
		t.Fatalf("revalidate: %v", r)
	}
	cur["mapsUrl"] = "javascript:alert(1)"
	cur["whatsappNumber"] = "12345"
	cur["email"] = "bukan-email"
	cur["companyName"] = ""
	expectErr(t, a.do("PUT", "/api/v1/admin/settings", cur), 422, "validation_error", "mapsUrl", "whatsappNumber", "email", "companyName")
	cur["showTestimonials"] = false
	expectErr(t, a.do("PUT", "/api/v1/admin/settings", cur), 400, "bad_request")
	expect(t, a.do("PUT", "/api/v1/admin/settings", orig), 200)
}

func TestDashboard(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	expect(t, a.do("POST", "/api/v1/admin/services", serviceBody("Draft Dasbor")), 201)
	d := decode[obj](t, a.do("GET", "/api/v1/admin/dashboard", nil))
	counts := d["counts"].(map[string]any)
	if counts["draftContent"].(float64) < 1 {
		t.Fatalf("dashboard counts: %v", counts)
	}
	drafts := d["recentDrafts"].([]any)
	if len(drafts) == 0 || drafts[0].(map[string]any)["title"] != "Draft Dasbor" || drafts[0].(map[string]any)["type"] != "service" {
		t.Fatalf("recent drafts: %v", drafts)
	}
	expect(t, a.do("DELETE", "/api/v1/admin/services/"+drafts[0].(map[string]any)["id"].(string), nil), 204)
}
