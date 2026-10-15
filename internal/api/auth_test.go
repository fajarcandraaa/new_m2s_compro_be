package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/api"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/ratelimit"
)

func TestLoginMeLogout(t *testing.T) {
	e := newEnv(t)
	c := e.anon()

	expectErr(t, c.do("POST", "/api/v1/auth/login", obj{"email": testAdminEmail, "password": testAdminPassword}, noCSRF()), 403, "csrf_failed")
	expectErr(t, c.do("POST", "/api/v1/auth/login", obj{"email": testAdminEmail, "password": "wrong-password"}), 401, "unauthorized")
	expectErr(t, c.do("POST", "/api/v1/auth/login", obj{"email": "not-an-email", "password": "x"}), 401, "unauthorized")
	expectErr(t, c.do("POST", "/api/v1/auth/login", obj{"email": "nobody@test.local", "password": "whatever-123"}), 401, "unauthorized")
	expectErr(t, c.do("POST", "/api/v1/auth/login", obj{"email": testAdminEmail, "password": "x", "remember": true}), 400, "bad_request")
	expectErr(t, c.do("GET", "/api/v1/auth/me", nil), 401, "unauthorized")

	res := c.do("POST", "/api/v1/auth/login", obj{"email": strings.ToUpper(testAdminEmail), "password": testAdminPassword})
	expect(t, res, 200)
	setCookie := res.Header().Get("Set-Cookie")
	for _, want := range []string{"m2s_session=", "HttpOnly", "SameSite=Lax", "Path=/", "Max-Age=3600"} {
		if !strings.Contains(setCookie, want) {
			t.Fatalf("Set-Cookie %q missing %q", setCookie, want)
		}
	}
	me := decode[obj](t, c.do("GET", "/api/v1/auth/me", nil))
	if me["email"] != testAdminEmail || me["role"] != "admin" || me["lastLoginAt"] == nil {
		t.Fatalf("me: %v", me)
	}

	expectErr(t, c.do("POST", "/api/v1/auth/logout", nil, noCSRF()), 403, "csrf_failed")
	res = c.do("POST", "/api/v1/auth/logout", nil)
	expect(t, res, 204)
	if !strings.Contains(res.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout should clear cookie: %s", res.Header().Get("Set-Cookie"))
	}
	expectErr(t, c.do("GET", "/api/v1/auth/me", nil), 401, "unauthorized")
	// Idempotent without a session.
	expect(t, c.do("POST", "/api/v1/auth/logout", nil), 204)

	// A forged token is rejected.
	c.cookie = &http.Cookie{Name: "m2s_session", Value: "not.a.jwt"}
	expectErr(t, c.do("GET", "/api/v1/auth/me", nil), 401, "unauthorized")
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t, func(d *api.Deps) { d.LoginLimiter = ratelimit.New(10, time.Minute) })
	c := e.anon()
	c.ip = "203.0.113.50"
	for i := 0; i < 10; i++ {
		expect(t, c.do("POST", "/api/v1/auth/login", obj{"email": testAdminEmail, "password": "bad-password"}), 401)
	}
	res := c.do("POST", "/api/v1/auth/login", obj{"email": testAdminEmail, "password": testAdminPassword})
	expectErr(t, res, 429, "rate_limited")
	if res.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestCSRFCheckedBeforeSession(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	// No cookie and no header: CSRF wins (403), not 401.
	expectErr(t, c.do("POST", "/api/v1/admin/articles", obj{"title": "x"}, noCSRF()), 403, "csrf_failed")
	expectErr(t, c.do("PUT", "/api/v1/admin/settings", obj{}, withHeader("X-Requested-With", "XMLHttpRequest")), 403, "csrf_failed")
	// With the header but no cookie: 401.
	expectErr(t, c.do("POST", "/api/v1/admin/articles", obj{"title": "x"}), 401, "unauthorized")
	// GETs do not need the header.
	expectErr(t, c.do("GET", "/api/v1/admin/dashboard", nil, noCSRF()), 401, "unauthorized")
	// Logged in but missing header on a mutation: 403 csrf_failed.
	a := e.admin()
	expectErr(t, a.do("DELETE", "/api/v1/admin/articles/00000000-0000-4000-8000-000000000000", nil, noCSRF()), 403, "csrf_failed")
	// POST /leads is exempt.
	expect(t, a.do("POST", "/api/v1/leads", validLead(), noCSRF()), 201)
}

func createEditor(t *testing.T, e *env, email string) *client {
	t.Helper()
	a := e.admin()
	expect(t, a.do("POST", "/api/v1/admin/users", obj{"name": "Editor", "email": email, "role": "editor", "password": "editor-pass-123"}), 201)
	return e.login(email, "editor-pass-123")
}

func TestRoleGating(t *testing.T) {
	e := newEnv(t)
	ed := createEditor(t, e, "editor.roles@test.local")

	// Admin-only: settings PUT, users, lead PATCH, lead export.
	settings := decode[obj](t, ed.do("GET", "/api/v1/admin/settings", nil))
	delete(settings, "updatedAt")
	delete(settings, "showTestimonials")
	delete(settings, "showClientLogos")
	expectErr(t, ed.do("PUT", "/api/v1/admin/settings", settings), 403, "forbidden")
	expectErr(t, ed.do("GET", "/api/v1/admin/users", nil), 403, "forbidden")
	expectErr(t, ed.do("POST", "/api/v1/admin/users", obj{"name": "X", "email": "x@test.local", "role": "admin", "password": "0123456789"}), 403, "forbidden")
	expectErr(t, ed.do("GET", "/api/v1/admin/leads/export", nil), 403, "forbidden")

	lead := decode[obj](t, ed.do("POST", "/api/v1/leads", validLead()))
	id := lead["id"].(string)
	expect(t, ed.do("GET", "/api/v1/admin/leads", nil), 200)
	expect(t, ed.do("GET", "/api/v1/admin/leads/"+id, nil), 200)
	expectErr(t, ed.do("PATCH", "/api/v1/admin/leads/"+id, obj{"status": "contacted"}), 403, "forbidden")

	// Editor allowed: dashboard, settings read, toggles, content, media.
	expect(t, ed.do("GET", "/api/v1/admin/dashboard", nil), 200)
	expect(t, ed.do("PATCH", "/api/v1/admin/settings/toggles", obj{"showTestimonials": true}), 200)
	expect(t, ed.do("GET", "/api/v1/admin/services", nil), 200)
	res := ed.do("POST", "/api/v1/admin/testimonials", obj{"clientName": "Budi", "clientRole": "HR", "company": nil, "quote": "Bagus.", "isActive": false})
	expect(t, res, 201)
	expect(t, ed.do("DELETE", "/api/v1/admin/testimonials/"+decode[obj](t, res)["id"].(string), nil), 204)

	// Admin can do the admin-only operations.
	a := e.admin()
	expect(t, a.do("PATCH", "/api/v1/admin/leads/"+id, obj{"status": "contacted"}), 200)
	expect(t, a.do("GET", "/api/v1/admin/users", nil), 200)
}
