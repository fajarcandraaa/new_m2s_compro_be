package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

type users map[string]*domain.User

func (u users) GetUser(_ context.Context, id string) (*domain.User, error) {
	if v, ok := u[id]; ok {
		return v, nil
	}
	return nil, pgx.ErrNoRows
}

func TestPasswordRules(t *testing.T) {
	BcryptCost = bcrypt.MinCost
	for pw, bad := range map[string]bool{"": true, "short": true, "0123456789": false, strings.Repeat("a", 64): false,
		strings.Repeat("a", 65): true, strings.Repeat("é", 37): true, strings.Repeat("é", 36): false} {
		if (PasswordProblem(pw) != "") != bad {
			t.Errorf("PasswordProblem(len %d) bad=%v", len(pw), !bad)
		}
	}
	h, err := HashPassword("0123456789")
	if err != nil || !CheckPassword(&h, "0123456789") || CheckPassword(&h, "x") || CheckPassword(nil, "0123456789") {
		t.Fatal("hash/check")
	}
}

func TestSessionMiddleware(t *testing.T) {
	u := &domain.User{ID: "5b8d5e9a-4a2b-4c1e-9a7b-1d2c3e4f5a6b", Role: domain.RoleEditor, IsActive: true, SessionVersion: 3}
	m := &Manager{Secret: []byte(strings.Repeat("k", 32)), TTL: time.Hour, Users: users{u.ID: u}}
	tok, err := m.Issue(u)
	if err != nil {
		t.Fatal(err)
	}
	handler := m.RequireSession(RequireRole(domain.RoleAdmin, domain.RoleEditor)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFrom(r.Context()).ID != u.ID {
			t.Error("user not in context")
		}
		w.WriteHeader(204)
	})))
	call := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/", nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: CookieName, Value: token})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}
	if rec := call(tok); rec.Code != 204 {
		t.Fatalf("valid session: %d", rec.Code)
	}
	if rec := call(""); rec.Code != 401 {
		t.Fatalf("no cookie: %d", rec.Code)
	}
	u.SessionVersion = 4 // revoked
	rec := call(tok)
	if rec.Code != 401 || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("stale session: %d %s", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	u.SessionVersion = 3
	u.IsActive = false
	if rec := call(tok); rec.Code != 401 {
		t.Fatalf("inactive: %d", rec.Code)
	}
	u.IsActive = true
	other := &Manager{Secret: []byte(strings.Repeat("x", 32)), TTL: time.Hour}
	forged, _ := other.Issue(u)
	if rec := call(forged); rec.Code != 401 {
		t.Fatalf("forged: %d", rec.Code)
	}
	expired := &Manager{Secret: m.Secret, TTL: -time.Minute}
	old, _ := expired.Issue(u)
	if rec := call(old); rec.Code != 401 {
		t.Fatalf("expired: %d", rec.Code)
	}
	adminOnly := m.RequireSession(RequireRole(domain.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: tok})
	rec = httptest.NewRecorder()
	adminOnly.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("editor on admin route: %d", rec.Code)
	}
}

func TestCSRF(t *testing.T) {
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for method, want := range map[string]int{"GET": 204, "POST": 403, "PUT": 403, "PATCH": 403, "DELETE": 403} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/", nil))
		if rec.Code != want {
			t.Errorf("%s without header: %d", method, rec.Code)
		}
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set(CSRFHeader, CSRFValue)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 204 {
		t.Errorf("with header: %d", rec.Code)
	}
}
