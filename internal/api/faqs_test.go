package api_test

import (
	"strings"
	"testing"
)

// faqList extracts faqs from a decoded response object as [question, answer]
// pairs; it fails when the key is missing.
func faqList(t *testing.T, v obj) [][2]string {
	t.Helper()
	raw, ok := v["faqs"].([]any)
	if !ok {
		t.Fatalf("faqs missing or not an array: %v", v["faqs"])
	}
	out := make([][2]string, len(raw))
	for i, it := range raw {
		m := it.(map[string]any)
		out[i] = [2]string{m["question"].(string), m["answer"].(string)}
	}
	return out
}

func faqItems(n int) []obj {
	out := make([]obj, n)
	for i := range out {
		out[i] = obj{"question": "Pertanyaan?", "answer": "Jawaban."}
	}
	return out
}

// checkFaqValidation runs the shared 422 cases against a create endpoint.
func checkFaqValidation(t *testing.T, c *client, path string, body obj) {
	t.Helper()
	body["faqs"] = faqItems(11)
	expectErr(t, c.do("POST", path, body), 422, "validation_error", "faqs")
	body["faqs"] = []obj{
		{"question": "Valid?", "answer": "Ya."},
		{"question": "  <b> </b>  ", "answer": strings.Repeat("a", 1001)},
		{"question": strings.Repeat("q", 201), "answer": "<script>x</script>"},
		{"question": "", "answer": ""},
	}
	e := expectErr(t, c.do("POST", path, body), 422, "validation_error",
		"faqs.1.question", "faqs.1.answer", "faqs.2.question", "faqs.2.answer", "faqs.3.question", "faqs.3.answer")
	if _, bad := e.Error.Fields["faqs.0.question"]; bad {
		t.Fatalf("valid item flagged: %v", e.Error.Fields)
	}
	// Limits apply after stripping: 200 characters of text inside markup is fine.
	body["faqs"] = []obj{{"question": "<p>" + strings.Repeat("q", 200) + "</p>", "answer": strings.Repeat("a", 1000)}}
	res := c.do("POST", path, body)
	expect(t, res, 201)
	created := decode[obj](t, res)
	if got := faqList(t, created); got[0][0] != strings.Repeat("q", 200) {
		t.Fatalf("stripped question: %q", got[0][0])
	}
	expect(t, c.do("DELETE", path+"/"+created["id"].(string), nil), 204)
	delete(body, "faqs")
}

func TestServiceFaqs(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	pub := e.anon()

	// Omitted faqs → [].
	b := serviceBody("Layanan FAQ")
	res := a.do("POST", "/api/v1/admin/services", b)
	expect(t, res, 201)
	svc := decode[obj](t, res)
	id := svc["id"].(string)
	if got := faqList(t, svc); len(got) != 0 {
		t.Fatalf("omitted faqs: %v", got)
	}

	// PUT with faqs: trimmed, HTML stripped, order kept; visible publicly.
	b["status"] = "published"
	b["faqs"] = []obj{
		{"question": "  Berapa <b>lama</b>?  ", "answer": "<p>Tergantung scope &amp; modul.</p><script>alert(1)</script>"},
		{"question": "Apakah bisa integrasi?", "answer": "Bisa, dengan API & database lama."},
		{"question": "Data milik siapa?", "answer": "Milik Anda."},
	}
	res = a.do("PUT", "/api/v1/admin/services/"+id, b)
	expect(t, res, 200)
	want := [][2]string{
		{"Berapa lama?", "Tergantung scope & modul."},
		{"Apakah bisa integrasi?", "Bisa, dengan API & database lama."},
		{"Data milik siapa?", "Milik Anda."},
	}
	check := func(where string, v obj) {
		t.Helper()
		got := faqList(t, v)
		if len(got) != len(want) {
			t.Fatalf("%s: faqs %v", where, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: faqs[%d] = %q, want %q", where, i, got[i], want[i])
			}
		}
	}
	check("PUT", decode[obj](t, res))
	check("admin GET", decode[obj](t, a.do("GET", "/api/v1/admin/services/"+id, nil)))
	check("public GET", decode[obj](t, pub.do("GET", "/api/v1/services/layanan-faq", nil)))
	list, _ := decodeList[obj](t, pub.do("GET", "/api/v1/services", nil))
	found := false
	for _, s := range list {
		faqList(t, s) // present on every list item
		if s["slug"] == "layanan-faq" {
			check("public list", s)
			found = true
		}
	}
	if !found {
		t.Fatal("service missing from public list")
	}

	// Full-object PUT without faqs clears them.
	delete(b, "faqs")
	res = a.do("PUT", "/api/v1/admin/services/"+id, b)
	expect(t, res, 200)
	if got := faqList(t, decode[obj](t, res)); len(got) != 0 {
		t.Fatalf("PUT without faqs: %v", got)
	}

	checkFaqValidation(t, a, "/api/v1/admin/services", serviceBody("Layanan FAQ Validasi"))
	expect(t, a.do("DELETE", "/api/v1/admin/services/"+id, nil), 204)
}

func TestArticleFaqs(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	pub := e.anon()

	b := articleBody("Artikel FAQ")
	res := a.do("POST", "/api/v1/admin/articles", b)
	expect(t, res, 201)
	art := decode[obj](t, res)
	id := art["id"].(string)
	if got := faqList(t, art); len(got) != 0 {
		t.Fatalf("omitted faqs: %v", got)
	}

	b["status"] = "published"
	b["faqs"] = []obj{
		{"question": "Kedua dulu?", "answer": "Ya."},
		{"question": " <i>Pertama</i> ", "answer": " a < b dan c > d "},
	}
	res = a.do("PUT", "/api/v1/admin/articles/"+id, b)
	expect(t, res, 200)
	want := [][2]string{{"Kedua dulu?", "Ya."}, {"Pertama", "a < b dan c > d"}}
	for where, v := range map[string]obj{
		"PUT":        decode[obj](t, res),
		"admin GET":  decode[obj](t, a.do("GET", "/api/v1/admin/articles/"+id, nil)),
		"public GET": decode[obj](t, pub.do("GET", "/api/v1/articles/artikel-faq", nil)),
	} {
		got := faqList(t, v)
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("%s: faqs %v", where, got)
		}
	}
	// Summaries do not carry faqs (the contract check rejects extra keys too).
	list, _ := decodeList[obj](t, pub.do("GET", "/api/v1/articles?pageSize=100", nil))
	for _, it := range list {
		if _, ok := it["faqs"]; ok {
			t.Fatalf("ArticleSummary has faqs: %v", it["slug"])
		}
	}

	delete(b, "faqs")
	res = a.do("PUT", "/api/v1/admin/articles/"+id, b)
	expect(t, res, 200)
	if got := faqList(t, decode[obj](t, res)); len(got) != 0 {
		t.Fatalf("PUT without faqs: %v", got)
	}

	checkFaqValidation(t, a, "/api/v1/admin/articles", articleBody("Artikel FAQ Validasi"))
	expect(t, a.do("DELETE", "/api/v1/admin/articles/"+id, nil), 204)
}
