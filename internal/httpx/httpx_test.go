package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		xff    []string
		remote string
		hops   int
		want   string
	}{
		{nil, "10.0.0.5:1234", 1, "10.0.0.5"},
		{[]string{"203.0.113.9"}, "10.0.0.5:1234", 1, "203.0.113.9"},
		{[]string{"1.1.1.1, 203.0.113.9"}, "10.0.0.5:1234", 1, "203.0.113.9"},
		{[]string{"1.1.1.1, 203.0.113.9"}, "10.0.0.5:1234", 2, "1.1.1.1"},
		{[]string{"1.1.1.1", "203.0.113.9"}, "10.0.0.5:1234", 1, "203.0.113.9"},
		{[]string{"203.0.113.9"}, "10.0.0.5:1234", 2, "10.0.0.5"},
		{[]string{"203.0.113.9"}, "10.0.0.5:1234", 0, "10.0.0.5"},
		{[]string{"[2001:db8::1]:443"}, "[::1]:80", 1, "2001:db8::1"},
		{nil, "[::1]:80", 1, "::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		for _, x := range c.xff {
			r.Header.Add("X-Forwarded-For", x)
		}
		if got := ClientIP(r, c.hops); got != c.want {
			t.Errorf("ClientIP(%v, %s, %d) = %s, want %s", c.xff, c.remote, c.hops, got, c.want)
		}
	}
}

func TestDecode(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	run := func(body string) error {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var v in
		return Decode(httptest.NewRecorder(), r, &v)
	}
	if err := run(`{"name":"x"}`); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for body, status := range map[string]int{
		`{"name":"x","extra":1}`: 400,
		`{"name":`:               400,
		``:                       400,
		`{"name":"x"} {}`:        400,
		`[1]`:                    400,
		`{"name":"` + strings.Repeat("a", MaxJSONBody) + `"}`: 413,
	} {
		var he *Error
		if err := run(body); !errors.As(err, &he) || he.Status != status {
			t.Errorf("body %.30q: got %v, want %d", body, err, status)
		}
	}
}

func TestOptional(t *testing.T) {
	var v struct {
		A Optional[string] `json:"a"`
		B Optional[string] `json:"b"`
		C Optional[bool]   `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":null,"c":true}`), &v); err != nil {
		t.Fatal(err)
	}
	if !v.A.Set || !v.A.Null || v.B.Set || !v.C.Set || v.C.Null || !v.C.Value {
		t.Fatalf("optional: %+v", v)
	}
}

func TestWriteErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&domain.ValidationError{Fields: map[string]string{"title": "x"}}, 422, "validation_error"},
		{domain.SlugTaken(), 409, "slug_taken"},
		{&domain.ConflictError{Code: "conflict", Message: "x"}, 409, "conflict"},
		{pgx.ErrNoRows, 404, "not_found"},
		{&pgconn.PgError{Code: "23505", ConstraintName: "articles_slug_key"}, 409, "slug_taken"},
		{&pgconn.PgError{Code: "23505", ConstraintName: "users_email_lower_uq"}, 409, "conflict"},
		{RateLimited(7), 429, "rate_limited"},
		{errors.New("boom"), 500, "internal_error"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		WriteError(rec, httptest.NewRequest("GET", "/", nil), c.err)
		var body struct {
			Error struct {
				Code    string            `json:"code"`
				Message string            `json:"message"`
				Fields  map[string]string `json:"fields"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != c.status || body.Error.Code != c.code || body.Error.Message == "" {
			t.Errorf("%v → %d %s, want %d %s", c.err, rec.Code, body.Error.Code, c.status, c.code)
		}
		if c.code == "internal_error" && strings.Contains(rec.Body.String(), "boom") {
			t.Error("internal error leaked")
		}
	}
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest("GET", "/", nil), RateLimited(7))
	if rec.Header().Get("Retry-After") != "7" {
		t.Error("Retry-After missing")
	}
}

func TestTimeAndMeta(t *testing.T) {
	b, _ := json.Marshal(Time(time.Date(2026, 5, 20, 8, 0, 0, 123, time.FixedZone("WIB", 7*3600))))
	if string(b) != `"2026-05-20T01:00:00Z"` {
		t.Errorf("time = %s", b)
	}
	if m := Unpaginated(0); m.PageSize != 1 || m.Total != 0 || m.Page != 1 {
		t.Errorf("meta = %+v", m)
	}
}

func TestQueryParsing(t *testing.T) {
	r := httptest.NewRequest("GET", "/?page=2&pageSize=9&b=true&e=active", nil)
	p, s, err := Page(r)
	if err != nil || p != 2 || s != 9 {
		t.Fatalf("page: %d %d %v", p, s, err)
	}
	if b, err := QueryBool(r, "b"); err != nil || b == nil || !*b {
		t.Fatalf("bool: %v %v", b, err)
	}
	if _, err := QueryBool(httptest.NewRequest("GET", "/?b=1", nil), "b"); err == nil {
		t.Fatal("bool 1 must be rejected")
	}
	if _, err := QueryEnum(r, "e", "active", "ended"); err != nil {
		t.Fatal(err)
	}
	if _, err := QueryEnum(httptest.NewRequest("GET", "/?e=x", nil), "e", "active"); err == nil {
		t.Fatal("bad enum accepted")
	}
	if !IsUUID("5b8d5e9a-4a2b-4c1e-9a7b-1d2c3e4f5a6b") || IsUUID("123") {
		t.Fatal("IsUUID")
	}
	_ = http.StatusOK
}
