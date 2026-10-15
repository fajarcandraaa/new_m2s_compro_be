package api_test

import (
	"strings"
	"testing"
	"time"
)

func articleBody(title string) obj {
	return obj{
		"title": title, "excerpt": "Ringkasan.", "contentHtml": "<p>Halo dunia.</p>",
		"status": "draft", "publishedAt": nil, "tags": []string{}, "coverImageUrl": nil,
		"author": "", "seo": obj{"title": nil, "description": nil, "ogImageUrl": nil},
	}
}

func TestArticleCRUDAndRules(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	pub := e.anon()

	// Create without slug → generated from title; collision → -2.
	b := articleBody("Café & Strategi Sistem Internal")
	b["tags"] = []string{"Strategi", "AI & Teknologi", "strategi", "Proses Bisnis"}
	b["contentHtml"] = `<h2 onclick="evil()">Judul</h2><p>` + strings.Repeat("kata ", 450) +
		`</p><script>alert(1)</script><a href="javascript:alert(1)">x</a><img src="/uploads/abc.png" alt="a">`
	res := a.do("POST", "/api/v1/admin/articles", b)
	expect(t, res, 201)
	if got := e.reval.last(); len(got) != 1 || got[0] != "articles" {
		t.Fatalf("revalidate tags: %v", got)
	}
	art := decode[obj](t, res)
	id := art["id"].(string)
	if art["slug"] != "cafe-dan-strategi-sistem-internal" {
		t.Fatalf("slug: %v", art["slug"])
	}
	if art["author"] != "Tim Mind2Screen" || art["readMinutes"].(float64) != 3 {
		t.Fatalf("author/readMinutes: %v %v", art["author"], art["readMinutes"])
	}
	html := art["contentHtml"].(string)
	for _, bad := range []string{"<script", "onclick", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Fatalf("sanitiser kept %q: %s", bad, html)
		}
	}
	if !strings.Contains(html, `<img src="/uploads/abc.png"`) {
		t.Fatalf("sanitiser dropped image: %s", html)
	}
	tags := art["tags"].([]any)
	if len(tags) != 3 || tags[0].(map[string]any)["slug"] != "strategi" ||
		tags[1].(map[string]any)["slug"] != "ai-dan-teknologi" || tags[1].(map[string]any)["name"] != "AI & Teknologi" ||
		tags[2].(map[string]any)["slug"] != "proses-bisnis" {
		t.Fatalf("tags order/dedupe: %v", tags)
	}

	res = a.do("POST", "/api/v1/admin/articles", articleBody("Café & Strategi Sistem Internal"))
	expect(t, res, 201)
	second := decode[obj](t, res)
	if second["slug"] != "cafe-dan-strategi-sistem-internal-2" {
		t.Fatalf("collision suffix: %v", second["slug"])
	}

	// Explicit slug taken → 409 slug_taken.
	dup := articleBody("Lain")
	dup["slug"] = "cafe-dan-strategi-sistem-internal"
	expectErr(t, a.do("POST", "/api/v1/admin/articles", dup), 409, "slug_taken", "slug")
	bad := articleBody("Lain")
	bad["slug"] = "Bad Slug"
	expectErr(t, a.do("POST", "/api/v1/admin/articles", bad), 422, "validation_error", "slug")
	expectErr(t, a.do("POST", "/api/v1/admin/articles", articleBody("!!!")), 422, "validation_error", "title")
	expectErr(t, a.do("POST", "/api/v1/admin/articles", articleBody("")), 422, "validation_error", "title")

	// PUT without slug keeps it even when the title changes.
	upd := articleBody("Judul Baru Sama Sekali")
	res = a.do("PUT", "/api/v1/admin/articles/"+id, upd)
	expect(t, res, 200)
	if got := decode[obj](t, res); got["slug"] != "cafe-dan-strategi-sistem-internal" || len(got["tags"].([]any)) != 0 {
		t.Fatalf("PUT should keep slug and replace tags: %v", got)
	}
	// PUT with the article's own slug is fine; another article's slug is not.
	upd["slug"] = "cafe-dan-strategi-sistem-internal"
	expect(t, a.do("PUT", "/api/v1/admin/articles/"+id, upd), 200)
	upd["slug"] = "cafe-dan-strategi-sistem-internal-2"
	expectErr(t, a.do("PUT", "/api/v1/admin/articles/"+id, upd), 409, "slug_taken", "slug")
	delete(upd, "slug")

	// Draft is hidden publicly.
	slug := "cafe-dan-strategi-sistem-internal"
	expectErr(t, pub.do("GET", "/api/v1/articles/"+slug, nil), 404, "not_found")

	// Scheduled (future publishedAt) is hidden publicly but flagged in admin.
	upd["status"] = "published"
	upd["publishedAt"] = time.Now().Add(48 * time.Hour).In(time.FixedZone("WIB", 7*3600)).Format(time.RFC3339)
	upd["tags"] = []string{"Keamanan Data"}
	res = a.do("PUT", "/api/v1/admin/articles/"+id, upd)
	expect(t, res, 200)
	if got := decode[obj](t, res); got["isScheduled"] != true || !strings.HasSuffix(got["publishedAt"].(string), "Z") {
		t.Fatalf("scheduled: %v %v", got["isScheduled"], got["publishedAt"])
	}
	expectErr(t, pub.do("GET", "/api/v1/articles/"+slug, nil), 404, "not_found")
	list, _ := decodeList[obj](t, pub.do("GET", "/api/v1/articles?tag=keamanan-data", nil))
	for _, it := range list {
		if it["slug"] == slug {
			t.Fatal("scheduled article listed publicly")
		}
	}
	sched, _ := decodeList[obj](t, a.do("GET", "/api/v1/admin/articles?status=scheduled", nil))
	if len(sched) != 1 || sched[0]["id"] != id {
		t.Fatalf("admin scheduled filter: %v", sched)
	}
	expect(t, a.do("GET", "/api/v1/admin/articles?q=cafe&tag=keamanan-data&status=draft", nil), 200)

	// Publish with publishedAt null → now → visible.
	upd["publishedAt"] = nil
	res = a.do("PUT", "/api/v1/admin/articles/"+id, upd)
	expect(t, res, 200)
	if got := decode[obj](t, res); got["publishedAt"] == nil || got["isScheduled"] != false {
		t.Fatalf("publish now: %v", got)
	}
	expect(t, pub.do("GET", "/api/v1/articles/"+slug, nil), 200)
	expect(t, a.do("GET", "/api/v1/admin/articles/"+id, nil), 200)

	expectErr(t, a.do("GET", "/api/v1/admin/articles/not-a-uuid", nil), 400, "bad_request")
	expectErr(t, a.do("GET", "/api/v1/admin/articles/00000000-0000-4000-8000-000000000000", nil), 404, "not_found")
	upd["publishedAt"] = "15/10/2026"
	expectErr(t, a.do("PUT", "/api/v1/admin/articles/"+id, upd), 422, "validation_error", "publishedAt")

	// Delete.
	n := e.reval.count()
	expect(t, a.do("DELETE", "/api/v1/admin/articles/"+id, nil), 204)
	if e.reval.count() != n+1 {
		t.Fatal("delete did not revalidate")
	}
	expectErr(t, a.do("DELETE", "/api/v1/admin/articles/"+id, nil), 404, "not_found")
	expect(t, a.do("DELETE", "/api/v1/admin/articles/"+second["id"].(string), nil), 204)
	expectErr(t, pub.do("GET", "/api/v1/articles/"+slug, nil), 404, "not_found")
}

func TestDecodeRules(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	expectErr(t, a.do("POST", "/api/v1/admin/articles", `{"title":`), 400, "bad_request")
	expectErr(t, a.do("POST", "/api/v1/admin/articles", `{"title":"x","bogus":1}`), 400, "bad_request")
	expectErr(t, a.do("POST", "/api/v1/admin/articles", `{"title":5}`), 400, "bad_request")
	big := `{"title":"x","contentHtml":"` + strings.Repeat("a", 1<<20) + `"}`
	expectErr(t, a.do("POST", "/api/v1/admin/articles", big), 413, "payload_too_large")
	expectErr(t, a.do("PUT", "/api/v1/admin/articles/123", articleBody("x")), 400, "bad_request")
}
