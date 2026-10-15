package api_test

import (
	"net/http"
	"testing"
)

func TestPublicEndpointsMatchContract(t *testing.T) {
	e := newEnv(t)
	c := e.anon()

	expect(t, c.do("GET", "/healthz", nil), http.StatusOK)

	site := decode[obj](t, c.do("GET", "/api/v1/site", nil))
	if site["whatsappNumber"] != "+6282230109923" || site["mapsUrl"] != nil {
		t.Fatalf("unexpected site: %v", site)
	}

	services, meta := decodeList[obj](t, c.do("GET", "/api/v1/services", nil))
	if len(services) != 5 || meta.Total != 5 || meta.Page != 1 || meta.PageSize != 5 {
		t.Fatalf("services: %d %+v", len(services), meta)
	}
	if services[0]["slug"] != "custom-software-development" || services[4]["slug"] != "web-development" {
		t.Fatalf("services not ordered by sortOrder")
	}
	personal, _ := decodeList[obj](t, c.do("GET", "/api/v1/services?audience=personal", nil))
	if len(personal) != 1 {
		t.Fatalf("audience filter: %d", len(personal))
	}
	expectErr(t, c.do("GET", "/api/v1/services?audience=robots", nil), 400, "bad_request")

	web := decode[obj](t, c.do("GET", "/api/v1/services/web-development", nil))
	if pk := web["packages"].([]any); len(pk) != 4 {
		t.Fatalf("packages: %d", len(pk))
	}
	expectErr(t, c.do("GET", "/api/v1/services/does-not-exist", nil), 404, "not_found")
	expectErr(t, c.do("GET", "/api/v1/services/Bad_Slug", nil), 404, "not_found")

	cs, _ := decodeList[obj](t, c.do("GET", "/api/v1/case-studies", nil))
	if len(cs) < 2 {
		t.Fatalf("case studies: %d", len(cs))
	}
	feat, _ := decodeList[obj](t, c.do("GET", "/api/v1/case-studies?featured=true", nil))
	if len(feat) > 2 {
		t.Fatalf("featured > 2")
	}
	expectErr(t, c.do("GET", "/api/v1/case-studies?featured=yes", nil), 400, "bad_request")
	expect(t, c.do("GET", "/api/v1/case-studies?category=HRIS", nil), 200)
	expect(t, c.do("GET", "/api/v1/case-studies/tumbuh-sistem-pertumbuhan-sdm", nil), 200)
	expectErr(t, c.do("GET", "/api/v1/case-studies/nope", nil), 404, "not_found")

	arts, am := decodeList[obj](t, c.do("GET", "/api/v1/articles?pageSize=9", nil))
	if am.PageSize != 9 || am.Total < 3 || len(arts) < 3 {
		t.Fatalf("articles: %d %+v", len(arts), am)
	}
	tagged, tm := decodeList[obj](t, c.do("GET", "/api/v1/articles?tag=keamanan-data", nil))
	if tm.Total != 2 || len(tagged) != 2 {
		t.Fatalf("tag filter: %+v", tm)
	}
	past, pm := decodeList[obj](t, c.do("GET", "/api/v1/articles?page=99", nil))
	if len(past) != 0 || pm.Total < 3 {
		t.Fatalf("page past end: %d %+v", len(past), pm)
	}
	expectErr(t, c.do("GET", "/api/v1/articles?pageSize=101", nil), 400, "bad_request")
	expectErr(t, c.do("GET", "/api/v1/articles?page=0", nil), 400, "bad_request")

	art := decode[obj](t, c.do("GET", "/api/v1/articles/data-ktp-bocor-itu-sebenarnya-dampaknya-apa", nil))
	if art["readMinutes"].(float64) != 7 || art["publishedAt"] != "2026-05-02T01:00:00Z" {
		t.Fatalf("seed article: readMinutes=%v publishedAt=%v", art["readMinutes"], art["publishedAt"])
	}
	related, rm := decodeList[obj](t, c.do("GET", "/api/v1/articles/data-ktp-bocor-itu-sebenarnya-dampaknya-apa/related", nil))
	if len(related) != 1 || related[0]["slug"] != "kenapa-jangan-klik-link-sembarangan-di-whatsapp" || rm.Total != 1 {
		t.Fatalf("related: %v", related)
	}
	expectErr(t, c.do("GET", "/api/v1/articles/x/related?limit=13", nil), 400, "bad_request")
	expectErr(t, c.do("GET", "/api/v1/articles/nope/related", nil), 404, "not_found")

	tags, _ := decodeList[obj](t, c.do("GET", "/api/v1/tags", nil))
	if len(tags) < 3 || tags[0]["slug"] != "ai-dan-teknologi" {
		t.Fatalf("tags: %v", tags)
	}

	expect(t, c.do("GET", "/api/v1/promos", nil), 200)
	expectErr(t, c.do("GET", "/api/v1/promos?status=expired", nil), 400, "bad_request")
	expect(t, c.do("GET", "/api/v1/testimonials?limit=3", nil), 200)
	expectErr(t, c.do("GET", "/api/v1/testimonials?limit=0", nil), 400, "bad_request")
	expect(t, c.do("GET", "/api/v1/client-logos", nil), 200)

	expectErr(t, c.do("GET", "/api/v1/nope", nil), 404, "not_found")
	expectErr(t, c.do("DELETE", "/api/v1/site", nil), 405, "method_not_allowed")
}
