package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/api"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/ratelimit"
)

func validLead() obj {
	return obj{"name": "Sari", "company": "PT Maju", "email": "sari@example.com", "whatsapp": "0812-3456-7890",
		"service": "custom-software-development", "startTimeframe": "1–3 bulan lagi",
		"message": "Kami butuh HRIS untuk 200 karyawan."}
}

func leadCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM leads").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLeadSubmission(t *testing.T) {
	e := newEnv(t)
	c := e.anon()

	res := c.do("POST", "/api/v1/leads", validLead(), noCSRF())
	expect(t, res, 201)
	created := decode[obj](t, res)
	a := e.admin()
	lead := decode[obj](t, a.do("GET", "/api/v1/admin/leads/"+created["id"].(string), nil))
	if lead["whatsapp"] != "+6281234567890" || lead["status"] != "new" || lead["serviceTitle"] != "Custom Software Development" {
		t.Fatalf("stored lead: %v", lead)
	}
	var ipHash, ua *string
	if err := testPool.QueryRow(context.Background(), "SELECT ip_hash, user_agent FROM leads WHERE id = $1", created["id"]).Scan(&ipHash, &ua); err != nil {
		t.Fatal(err)
	}
	if ipHash == nil || len(*ipHash) != 64 || strings.Contains(*ipHash, "198.51") {
		t.Fatalf("ip_hash: %v", ipHash)
	}
	if _, ok := lead["ipHash"]; ok {
		t.Fatal("ip hash must not be exposed")
	}

	// E.164 normalisation variants.
	for in, want := range map[string]string{
		"081234567890":      "+6281234567890",
		"6281234567890":     "+6281234567890",
		"+62 812 3456 7890": "+6281234567890",
		"0812 (3456) 7890":  "+6281234567890",
		"+1 415 555 2671":   "+14155552671",
	} {
		b := validLead()
		b["whatsapp"] = in
		b["company"] = ""
		b["service"] = nil
		res := c.do("POST", "/api/v1/leads", b)
		expect(t, res, 201)
		l := decode[obj](t, a.do("GET", "/api/v1/admin/leads/"+decode[obj](t, res)["id"].(string), nil))
		if l["whatsapp"] != want || l["company"] != nil || l["serviceSlug"] != nil {
			t.Fatalf("%q → %v (company %v)", in, l["whatsapp"], l["company"])
		}
	}

	// Validation.
	b := validLead()
	b["whatsapp"] = "12345"
	b["email"] = "x@"
	b["message"] = "pendek"
	b["service"] = "tidak-ada"
	b["name"] = "  "
	expectErr(t, c.do("POST", "/api/v1/leads", b), 422, "validation_error", "whatsapp", "email", "message", "name")
	b = validLead()
	b["service"] = "tidak-ada"
	expectErr(t, c.do("POST", "/api/v1/leads", b), 422, "validation_error", "service")
	b = validLead()
	b["phone"] = "x"
	expectErr(t, c.do("POST", "/api/v1/leads", b), 400, "bad_request")

	// Honeypot: fake 201, nothing stored (checked before validation).
	before := leadCount(t)
	hp := obj{"name": "", "email": "bot", "whatsapp": "1", "message": "", "website": "http://spam.example"}
	res = c.do("POST", "/api/v1/leads", hp)
	expect(t, res, 201)
	if decode[obj](t, res)["id"] == "" || leadCount(t) != before {
		t.Fatal("honeypot lead must not be stored")
	}
}

func TestLeadRateLimit(t *testing.T) {
	e := newEnv(t, func(d *api.Deps) { d.LeadLimiter = ratelimit.New(5, time.Minute) })
	c := e.anon()
	c.ip = "203.0.113.9"
	for i := 0; i < 4; i++ {
		expect(t, c.do("POST", "/api/v1/leads", validLead()), 201)
	}
	// The honeypot still counts toward the limit.
	b := validLead()
	b["website"] = "x"
	expect(t, c.do("POST", "/api/v1/leads", b), 201)
	res := c.do("POST", "/api/v1/leads", validLead())
	expectErr(t, res, 429, "rate_limited")
	if res.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	// Keyed per client IP (rightmost X-Forwarded-For with 1 trusted hop).
	other := e.anon()
	other.ip = "spoofed, 203.0.113.10"
	expect(t, other.do("POST", "/api/v1/leads", validLead()), 201)
	spoof := e.anon()
	spoof.ip = "1.2.3.4, 203.0.113.9"
	expectErr(t, spoof.do("POST", "/api/v1/leads", validLead()), 429, "rate_limited")
}

func TestLeadAdminFlowAndExport(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	a := e.admin()
	b := validLead()
	b["name"] = "=HYPERLINK(\"x\")"
	b["message"] = "Baris satu, dengan koma\nbaris \"dua\"."
	id := decode[obj](t, c.do("POST", "/api/v1/leads", b))["id"].(string)

	list, meta := decodeList[obj](t, a.do("GET", "/api/v1/admin/leads?status=new&q=HYPERLINK", nil))
	if len(list) != 1 || meta.Total != 1 || list[0]["id"] != id {
		t.Fatalf("lead search: %v", list)
	}
	expectErr(t, a.do("PATCH", "/api/v1/admin/leads/"+id, obj{}), 422, "validation_error")
	expectErr(t, a.do("PATCH", "/api/v1/admin/leads/"+id, obj{"status": "closed"}), 422, "validation_error", "status")
	res := a.do("PATCH", "/api/v1/admin/leads/"+id, obj{"status": "contacted", "notes": "Sudah ditelepon."})
	expect(t, res, 200)
	if l := decode[obj](t, res); l["status"] != "contacted" || l["notes"] != "Sudah ditelepon." {
		t.Fatalf("patch: %v", l)
	}
	res = a.do("PATCH", "/api/v1/admin/leads/"+id, obj{"status": "done"})
	if l := decode[obj](t, res); l["notes"] != "Sudah ditelepon." || l["status"] != "done" {
		t.Fatalf("absent notes must be unchanged: %v", l)
	}
	res = a.do("PATCH", "/api/v1/admin/leads/"+id, obj{"notes": nil})
	if l := decode[obj](t, res); l["notes"] != nil {
		t.Fatalf("notes null must clear: %v", l)
	}
	expectErr(t, a.do("PATCH", "/api/v1/admin/leads/00000000-0000-4000-8000-000000000000", obj{"status": "done"}), 404, "not_found")

	res = a.do("GET", "/api/v1/admin/leads/export?q=HYPERLINK", nil)
	expect(t, res, 200)
	if ct := res.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content type: %s", ct)
	}
	if cd := res.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="leads-`) {
		t.Fatalf("content disposition: %s", cd)
	}
	body := res.Body.String()
	if !strings.HasPrefix(body, "\xEF\xBB\xBFcreatedAt,name,company,email,whatsapp,service,startTimeframe,status,message,notes\r\n") {
		t.Fatalf("csv header: %q", body[:120])
	}
	if !strings.Contains(body, `"'=HYPERLINK(""x"")"`) || !strings.Contains(body, "'+6281234567890") ||
		!strings.Contains(body, "Custom Software Development") || !strings.Contains(body, ",done,") {
		t.Fatalf("csv row: %q", body)
	}
}

func TestUsersAdmin(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	me := decode[obj](t, a.do("GET", "/api/v1/auth/me", nil))
	meID := me["id"].(string)

	expectErr(t, a.do("POST", "/api/v1/admin/users", obj{"name": "", "email": "x", "role": "root", "password": "short"}),
		422, "validation_error", "name", "email", "role", "password")
	expectErr(t, a.do("POST", "/api/v1/admin/users", obj{"name": "X", "email": "x2@test.local", "role": "editor",
		"password": strings.Repeat("é", 40)}), 422, "validation_error", "password")
	res := a.do("POST", "/api/v1/admin/users", obj{"name": "Uji Editor", "email": "uji.editor@test.local", "role": "editor", "password": "editor-pass-123"})
	expect(t, res, 201)
	uid := decode[obj](t, res)["id"].(string)
	expectErr(t, a.do("POST", "/api/v1/admin/users", obj{"name": "Dup", "email": "UJI.EDITOR@test.local", "role": "editor", "password": "editor-pass-123"}),
		409, "conflict", "email")

	list, _ := decodeList[obj](t, a.do("GET", "/api/v1/admin/users?q=uji", nil))
	if len(list) != 1 {
		t.Fatalf("user search: %d", len(list))
	}
	expect(t, a.do("GET", "/api/v1/admin/users/"+uid, nil), 200)

	// Self-protection.
	expectErr(t, a.do("PUT", "/api/v1/admin/users/"+meID, obj{"name": "Admin", "email": testAdminEmail, "role": "editor", "isActive": true}), 409, "conflict", "role")
	expectErr(t, a.do("PUT", "/api/v1/admin/users/"+meID, obj{"name": "Admin", "email": testAdminEmail, "role": "admin", "isActive": false}), 409, "conflict", "isActive")
	expectErr(t, a.do("DELETE", "/api/v1/admin/users/"+meID, nil), 409, "conflict")
	expectErr(t, a.do("PUT", "/api/v1/admin/users/"+meID, obj{"name": "Admin", "email": "uji.editor@test.local", "role": "admin", "isActive": true}), 409, "conflict", "email")
	expect(t, a.do("PUT", "/api/v1/admin/users/"+meID, obj{"name": "Test Admin", "email": testAdminEmail, "role": "admin", "isActive": true}), 200)

	// Password reset revokes the user's sessions.
	ed := e.login("uji.editor@test.local", "editor-pass-123")
	expectErr(t, a.do("POST", "/api/v1/admin/users/"+uid+"/password", obj{"password": "short"}), 422, "validation_error", "password")
	expect(t, a.do("POST", "/api/v1/admin/users/"+uid+"/password", obj{"password": "new-editor-pass-1"}), 204)
	expectErr(t, ed.do("GET", "/api/v1/auth/me", nil), 401, "unauthorized")
	ed = e.login("uji.editor@test.local", "new-editor-pass-1")

	// Role change revokes; deactivation revokes and blocks login.
	expect(t, a.do("PUT", "/api/v1/admin/users/"+uid, obj{"name": "Uji", "email": "uji.editor@test.local", "role": "admin", "isActive": true}), 200)
	expectErr(t, ed.do("GET", "/api/v1/auth/me", nil), 401, "unauthorized")
	ed = e.login("uji.editor@test.local", "new-editor-pass-1")
	expect(t, a.do("PUT", "/api/v1/admin/users/"+uid, obj{"name": "Uji", "email": "uji.editor@test.local", "role": "admin", "isActive": false}), 200)
	expectErr(t, ed.do("GET", "/api/v1/auth/me", nil), 401, "unauthorized")
	expectErr(t, e.anon().do("POST", "/api/v1/auth/login", obj{"email": "uji.editor@test.local", "password": "new-editor-pass-1"}), 401, "unauthorized")

	// Resetting your own password re-issues the cookie.
	res = a.do("POST", "/api/v1/admin/users/"+meID+"/password", obj{"password": "another-admin-pass"})
	expect(t, res, 204)
	if !strings.Contains(res.Header().Get("Set-Cookie"), "m2s_session=") {
		t.Fatal("own password reset must re-issue the cookie")
	}
	expect(t, a.do("GET", "/api/v1/auth/me", nil), 200)
	expect(t, a.do("POST", "/api/v1/admin/users/"+meID+"/password", obj{"password": testAdminPassword}), 204)

	expect(t, a.do("DELETE", "/api/v1/admin/users/"+uid, nil), 204)
	expectErr(t, a.do("GET", "/api/v1/admin/users/"+uid, nil), 404, "not_found")
}

func multipartBody(t *testing.T, field, filename string, data []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{48, 194, 219, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMediaUpload(t *testing.T) {
	e := newEnv(t)
	a := e.admin()
	upload := func(field, name string, data []byte) *httptest.ResponseRecorder {
		body, ct := multipartBody(t, field, name, data)
		return a.do("POST", "/api/v1/admin/media", body, withHeader("Content-Type", ct))
	}

	res := upload("file", "logo.svg.png", pngBytes(t, 32, 18))
	expect(t, res, 201)
	m := decode[obj](t, res)
	url := m["url"].(string)
	if m["mimeType"] != "image/png" || m["width"].(float64) != 32 || m["height"].(float64) != 18 ||
		!strings.HasPrefix(url, "/uploads/") || len(m["filename"].(string)) != 36 {
		t.Fatalf("upload: %v", m)
	}
	get := a.do("GET", url, nil)
	expect(t, get, 200)
	if get.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(get.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("upload headers: %v", get.Header())
	}
	expectErr(t, a.do("GET", "/uploads/missing.png", nil), 404, "not_found")
	expectErr(t, a.do("GET", "/uploads/", nil), 404, "not_found")

	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	expectErr(t, upload("file", "x.png", svg), 415, "unsupported_media_type")
	expectErr(t, upload("file", "x.png", []byte("just text, not an image at all")), 415, "unsupported_media_type")
	// PNG signature but not decodable (polyglot / truncated).
	expectErr(t, upload("file", "x.png", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)), 415, "unsupported_media_type")

	big := make([]byte, 2<<20+1)
	_, _ = rand.Read(big)
	copy(big, pngBytes(t, 2, 2))
	expectErr(t, upload("file", "big.png", big), 413, "payload_too_large")
	expectErr(t, upload("other", "x.png", pngBytes(t, 2, 2)), 400, "bad_request")

	// Media needs a session and CSRF.
	body, ct := multipartBody(t, "file", "x.png", pngBytes(t, 2, 2))
	expectErr(t, e.anon().do("POST", "/api/v1/admin/media", body, withHeader("Content-Type", ct)), 401, "unauthorized")
	expectErr(t, a.do("POST", "/api/v1/admin/media", body, withHeader("Content-Type", ct), noCSRF()), 403, "csrf_failed")
	_ = auth.CookieName
	_ = http.StatusOK
}
