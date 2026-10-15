package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	compro "github.com/fajarcandraaa/new_m2s_compro_be"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/api"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/auth"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/config"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/db"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/ratelimit"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/seed"
	"github.com/fajarcandraaa/new_m2s_compro_be/internal/store"
)

// Integration tests run against TEST_DATABASE_URL (e.g.
// postgres://postgres@127.0.0.1:5433/m2s_test?sslmode=disable). The public
// schema is dropped and recreated, migrated and seeded once per package run.
// Without TEST_DATABASE_URL they are skipped and only unit tests run.

const (
	testAdminEmail    = "admin@test.local"
	testAdminPassword = "admin-password-123"
)

var (
	testPool   *pgxpool.Pool
	testStore  *store.Store
	testCfg    *config.Config
	specRouter routers.Router
	uploadDir  string
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	auth.BcryptCost = bcrypt.MinCost
	if os.Getenv("TEST_LOG") == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		return m.Run()
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		return 1
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		fmt.Fprintln(os.Stderr, "reset schema:", err)
		return 1
	}
	if err := db.Migrate(ctx, pool, compro.Migrations); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}
	testStore = store.New(pool)
	if err := seed.Run(ctx, testStore, compro.Content, seed.Admin{Email: testAdminEmail, Password: testAdminPassword, Name: "Test Admin"}); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		return 1
	}
	// Seeding twice must be a no-op.
	if err := seed.Run(ctx, testStore, compro.Content, seed.Admin{Email: testAdminEmail, Password: testAdminPassword}); err != nil {
		fmt.Fprintln(os.Stderr, "seed (2nd run):", err)
		return 1
	}
	testPool = pool

	dir, err := os.MkdirTemp("", "m2s-uploads-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempdir:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	uploadDir = dir
	testCfg = &config.Config{
		JWTSecret: strings.Repeat("s", 40), JWTTTL: time.Hour, CookieSecure: false,
		UploadDir: dir, UploadMaxBytes: 2 << 20, TrustedProxyHops: 1,
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load spec:", err)
		return 1
	}
	// kin-openapi v0.135.0 gorillamux leaks a path-level `servers` override
	// into every following path, so drop it and route /healthz under the
	// base path (see validate).
	doc.Paths.Value("/healthz").Servers = nil
	// doc.Validate is intentionally skipped (v0.135.0 rejects type "null").
	if specRouter, err = gorillamux.NewRouter(doc); err != nil {
		fmt.Fprintln(os.Stderr, "spec router:", err)
		return 1
	}
	return m.Run()
}

func needDB(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
}

// fakeReval records revalidation calls.
type fakeReval struct {
	mu   sync.Mutex
	tags [][]string
}

func (f *fakeReval) Fire(tags ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tags = append(f.tags, tags)
}

func (f *fakeReval) last() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.tags) == 0 {
		return nil
	}
	return f.tags[len(f.tags)-1]
}

func (f *fakeReval) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tags)
}

type env struct {
	t     *testing.T
	h     http.Handler
	reval *fakeReval
}

// newEnv builds a router with generous rate limits unless overridden.
func newEnv(t *testing.T, mod ...func(*api.Deps)) *env {
	t.Helper()
	needDB(t)
	rec := &fakeReval{}
	d := api.Deps{Store: testStore, Config: testCfg, Revalidator: rec,
		LeadLimiter: ratelimit.New(1000, time.Minute), LoginLimiter: ratelimit.New(1000, time.Minute)}
	for _, f := range mod {
		f(&d)
	}
	return &env{t: t, h: api.NewRouter(d), reval: rec}
}

// client carries a session cookie.
type client struct {
	e      *env
	cookie *http.Cookie
	ip     string
}

func (e *env) anon() *client { return &client{e: e, ip: "198.51.100.7"} }

func (e *env) login(email, password string) *client {
	e.t.Helper()
	c := e.anon()
	res := c.do("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": password})
	if res.Code != http.StatusOK {
		e.t.Fatalf("login %s: %d %s", email, res.Code, res.Body.String())
	}
	return c
}

func (e *env) admin() *client { return e.login(testAdminEmail, testAdminPassword) }

type reqOpt func(*http.Request)

func noCSRF() reqOpt { return func(r *http.Request) { r.Header.Del(auth.CSRFHeader) } }

func withHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

func (c *client) do(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	c.e.t.Helper()
	var rd io.Reader
	var raw []byte
	switch b := body.(type) {
	case nil:
	case []byte:
		raw = b
	case string:
		raw = []byte(b)
	default:
		var err error
		if raw, err = json.Marshal(b); err != nil {
			c.e.t.Fatal(err)
		}
	}
	if raw != nil {
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(auth.CSRFHeader, auth.CSRFValue)
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == auth.CookieName {
			if ck.MaxAge < 0 || ck.Value == "" {
				c.cookie = nil
			} else {
				c.cookie = &http.Cookie{Name: ck.Name, Value: ck.Value}
			}
		}
	}
	validate(c.e.t, method, path, req.Header, raw, rec)
	return rec
}

// validate checks the response against the OpenAPI contract
// (kin-openapi v0.135.0 openapi3filter.ValidateResponse).
func validate(t *testing.T, method, path string, hdr http.Header, body []byte, rec *httptest.ResponseRecorder) {
	t.Helper()
	routePath := path
	if strings.HasPrefix(path, "/healthz") {
		routePath = "/api/v1" + path
	}
	if strings.HasPrefix(path, "/uploads/") {
		return // static files are outside the JSON contract
	}
	vreq := httptest.NewRequest(method, routePath, bytes.NewReader(body))
	vreq.Header = hdr.Clone()
	route, params, err := specRouter.FindRoute(vreq)
	if err != nil {
		// Unknown routes / wrong methods are not in the spec; their bodies
		// must still be the error envelope.
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: route not in spec (%v) but status %d", method, path, err, rec.Code)
		}
		assertErrorEnvelope(t, rec)
		return
	}
	in := &openapi3filter.RequestValidationInput{Request: vreq, PathParams: params, Route: route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}}
	rvi := &openapi3filter.ResponseValidationInput{RequestValidationInput: in, Status: rec.Code,
		Header: rec.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true}}
	rvi.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(context.Background(), rvi); err != nil {
		t.Errorf("%s %s → %d: response violates contract: %v\nbody: %s", method, path, rec.Code, err, truncate(rec.Body.String()))
	}
}

func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error.Code == "" || e.Error.Message == "" {
		t.Errorf("expected error envelope, got %d %s", rec.Code, truncate(rec.Body.String()))
	}
}

func truncate(s string) string {
	if len(s) > 800 {
		return s[:800] + "…"
	}
	return s
}

// ---- assertion helpers ----

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, rec.Code, truncate(rec.Body.String()))
	}
}

type errBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func expectErr(t *testing.T, rec *httptest.ResponseRecorder, status int, code string, fields ...string) errBody {
	t.Helper()
	expect(t, rec, status)
	var e errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if e.Error.Code != code {
		t.Fatalf("expected code %s, got %s (%s)", code, e.Error.Code, rec.Body.String())
	}
	for _, f := range fields {
		if _, ok := e.Error.Fields[f]; !ok {
			t.Fatalf("expected field error %q, got %v", f, e.Error.Fields)
		}
	}
	return e
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v: %s", err, truncate(rec.Body.String()))
	}
	return env.Data
}

type listMeta struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

func decodeList[T any](t *testing.T, rec *httptest.ResponseRecorder) ([]T, listMeta) {
	t.Helper()
	var env struct {
		Data []T      `json:"data"`
		Meta listMeta `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode list: %v: %s", err, truncate(rec.Body.String()))
	}
	return env.Data, env.Meta
}

type obj = map[string]any
