# Architecture

This document covers how the backend is built and deployed. The API contract is in `api/openapi.yaml` and the data model is in `docs/db-schema.md`. Product decisions come from `docs/brief.md` §3.

## 1. Topology

```
browser ──► FE (Next.js, :3000) ──rewrite /api/*, /uploads/*──► BE (Go, :8080) ──► PostgreSQL 16
              │  Server Components ──── BACKEND_URL ───────────►│
              ◄──────── POST /api/revalidate {tags} ─────────────┘ (after admin writes)
```

- The browser only ever talks to the FE origin, so there is no CORS and the cookie is first-party.
- The BE serves `/api/v1/*`, `/uploads/*` (static files from `UPLOAD_DIR`) and `/healthz`.

## 2. BE package layout

```
cmd/api/main.go            wiring: config → db pool → migrate → seed → router → http.Server (graceful shutdown on SIGTERM)
assets.go                  package compro (module root): //go:embed migrations/*.sql docs/content/*.json
migrations/                00001_users.sql … 00008_leads.sql (goose, see db-schema.md)
internal/config            env parsing + validation (fails fast on missing JWT_SECRET etc.)
internal/db                pgxpool setup, goose runner (goose.SetBaseFS + "postgres" dialect via pgx stdlib)
internal/seed              idempotent seed from embedded JSON + ADMIN_* env
internal/store             SQL per resource (pgx, hand-written queries, no ORM); returns domain structs
internal/domain            entity structs, enums, pure logic (promo status, slugify, read minutes, validation)
internal/auth              bcrypt, JWT (HS256, claims: sub, role, sv, exp), session middleware, role guard
internal/httpx             JSON decode (DisallowUnknownFields, 1 MiB cap), respond/envelopes, error type → status mapping
internal/api               chi router + handlers grouped by resource (public_*.go, admin_*.go), DTO ↔ domain mapping
internal/media             multipart upload, sniffing, random names, image dimensions
internal/revalidate        async FE revalidation client
internal/ratelimit         in-memory token bucket per key (IP), used for POST /leads and POST /auth/login
```

Dependencies: `chi/v5`, `pgx/v5`, `pressly/goose/v3`, `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, `microcosm-cc/bluemonday`, `golang.org/x/image/webp` (for decoding WebP dimensions), `getkin/kin-openapi` (tests only, v0.150.0; verified to load this 3.1 spec and validate its `type: [x, "null"]` schemas).

## 3. Request flow

`chi` middleware stack, outermost first:

1. `RequestID`
2. `RealIP`, using our own helper that honours `TRUSTED_PROXY_HOPS`
3. `Recoverer`, which logs the panic and returns 500 `internal_error`
4. slog access log (method, path, status, duration, request_id; never bodies or cookies)
5. `Timeout(15s)`

Route groups:
- `/api/v1` public: handler → store → `httpx.JSON(w, 200, envelope)`.
- `/api/v1/admin`: `auth.RequireSession` (cookie → JWT verify → load user by id → check `is_active` and `session_version`) → `auth.RequireRole(...)` per route → `csrf` (mutating methods require `X-Requested-With: m2s-cms`) → handler.
- Successful admin writes call `revalidate.Fire(tags...)` after the DB commit.

Handlers return `error`. A single adapter maps typed errors to the envelope:

| Error | Status | Code |
|---|---|---|
| `httpx.ErrBadRequest` | 400 | `bad_request` |
| `ValidationError{fields}` | 422 | `validation_error` |
| `ErrUnauthorized` | 401 | `unauthorized` |
| `ErrForbidden` | 403 | `forbidden` |
| CSRF failure | 403 | `csrf_failed` |
| `ErrNotFound` (incl. `pgx.ErrNoRows`) | 404 | `not_found` |
| `ConflictError` | 409 | `conflict` / `slug_taken` |
| unique violation `23505` on slug | 409 | `slug_taken` |
| unique violation `23505` on email | 409 | `conflict` |
| anything else | 500 | `internal_error`; logged with request_id, generic message to the client |

## 4. Auth & roles

- Login: look up by `lower(email)`, `bcrypt.CompareHashAndPassword` (cost 12). On an unknown email, still compare against a dummy hash to keep timing flat. Inactive users fail with the same 401.
- Success sets the cookie `m2s_session=<jwt>`: `Path=/; HttpOnly; SameSite=Lax; Secure=<COOKIE_SECURE>; Max-Age=JWT_TTL`. It also updates `last_login_at`.
- Logout clears the cookie. There is no server-side session store. Revocation increments `session_version`, which happens on password reset, role change and deactivation.
- Role matrix:

  | Area | Access |
  |---|---|
  | `settings` PUT, `users/*`, `PATCH leads` | admin only |
  | `GET settings`, `PATCH settings/toggles`, dashboard, all content CRUD, media, `GET leads*` | admin + editor |

- Self-protection on `users/{id}`: an admin cannot change their own role, deactivate themselves or delete themselves (409). This also prevents removing the last admin through self-demotion.

## 5. Migrations & seed

- On start, if `MIGRATE_ON_START=true` (the default), `goose.Up` runs over the embedded FS. A failure is fatal and the process exits non-zero, so a bad migration never serves traffic.
- Then, if `SEED_ON_START=true` (the default), the seed runs (see db-schema.md). It is idempotent per table and wrapped in one transaction.
  - Article HTML from `articles.json` goes through the same bluemonday policy as CMS saves.
  - Times are converted to UTC.
  - Tag slugs are derived with `domain.Slugify`.
- The admin user is created from `ADMIN_EMAIL`, `ADMIN_PASSWORD` and `ADMIN_NAME`, and only when `users` is empty. If either email or password is unset, startup logs a warning and skips creating it.

## 6. Media

- `POST /api/v1/admin/media` reads the request through `http.MaxBytesReader` (2 MiB + multipart overhead). Over the limit → 413.
- The type is sniffed from the first 512 bytes with `http.DetectContentType`. Only `image/jpeg`, `image/png` and `image/webp` are allowed; everything else (including SVG) → 415. The image must also decode via `image.DecodeConfig` to get width and height, which rejects polyglots.
- Files are written to `UPLOAD_DIR/<32 hex>.<ext>`: first to a temp file in the same dir, then `os.Rename`. Mode is 0644.
- Files are served by `http.FileServer` at `/uploads/` with:
  - `X-Content-Type-Options: nosniff`
  - `Cache-Control: public, max-age=31536000, immutable` (names are random, so caching forever is safe)
  - directory listing disabled
- Responses return site-relative URLs (`/uploads/…`). Next.js rewrites `/uploads/*` to the BE. `next/image` uses those URLs through `images.remotePatterns` or `unoptimized` for the local origin; the FE decides which.

## 7. Revalidation

`revalidate.Fire(tags ...string)` runs in a goroutine with its own `context.WithTimeout(3s)` (detached from the request) and sends:
- `POST {FRONTEND_URL}/api/revalidate`
- headers `X-Revalidate-Secret: {REVALIDATE_SECRET}` and `Content-Type: application/json`
- body `{"tags":[...]}`

Any error or non-2xx is logged at `warn` and never blocks the response. If `FRONTEND_URL` or `REVALIDATE_SECRET` is empty, the call is a no-op with a debug log. The mapping from resource to tags is listed in `info.description` of the OpenAPI spec. The FE route handler compares the secret in constant time and calls `revalidateTag` for each tag.

## 8. Configuration

### Backend
| Name | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | — (required) | `postgres://user:pass@host:5432/db?sslmode=disable` |
| `MIGRATE_ON_START` | `true` | Run embedded goose migrations on boot |
| `SEED_ON_START` | `true` | Run the idempotent seed on boot |
| `JWT_SECRET` | — (required, ≥32 chars) | HS256 signing key; also used to salt `leads.ip_hash` |
| `JWT_TTL` | `12h` | Session lifetime (Go duration); also the cookie Max-Age |
| `COOKIE_SECURE` | `true` | Secure flag on `m2s_session`; set `false` for plain-http local runs |
| `ADMIN_EMAIL` | — | Initial admin, created only when `users` is empty |
| `ADMIN_PASSWORD` | — | Initial admin password (≥8 chars) |
| `ADMIN_NAME` | `Admin` | Initial admin display name |
| `UPLOAD_DIR` | `/data/uploads` | Upload storage (Docker volume) |
| `UPLOAD_MAX_BYTES` | `2097152` | Max upload size |
| `FRONTEND_URL` | `http://fe:3000` | Base URL the BE uses to call FE revalidation (internal network) |
| `REVALIDATE_SECRET` | — | Shared secret for `/api/revalidate`; empty disables revalidation |
| `TRUSTED_PROXY_HOPS` | `1` | How many trusted proxies append to `X-Forwarded-For`. The client IP is the entry this many positions from the right. Local compose (Next only) = 1; Dokploy (Traefik + Next) = 2. Used for rate limiting. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

### Frontend
| Name | Default | Purpose |
|---|---|---|
| `BACKEND_URL` | `http://be:8080` | BE base URL for Server Components and for the `/api/*` and `/uploads/*` rewrites. **Rewrites are resolved at build time in standalone output**, so pass it as a Docker build arg *and* as a runtime env var. |
| `SITE_URL` | `http://localhost:3000` | Canonical origin for metadata, OG URLs, `sitemap.xml` and `robots.txt` (server-only, read at runtime) |
| `REVALIDATE_SECRET` | — | Must equal the BE value |
| `PORT` | `3000` | Next standalone server port |

## 9. Docker & deployment

**Local** (`docker-compose.yml` in the BE repo, with both repos side by side):
- `postgres`: `postgres:16-alpine`, volume `pgdata`, healthcheck `pg_isready`.
- `be`: `build: .` (multi-stage: `golang:1.24-alpine` build → `gcr.io/distroless/static-debian12:nonroot`, `CGO_ENABLED=0`). It needs:
  - `depends_on: postgres: condition: service_healthy`
  - volume `uploads:/data/uploads`
  - env `COOKIE_SECURE=false`
  - exposed on `8080` for debugging
- `fe`: `build: { context: ../new_m2s_compro_fe, args: { BACKEND_URL: http://be:8080 } }`, published on `3000`, `depends_on: be`.
- The uploads volume must be writable by the distroless `nonroot` uid 65532. Create the dir in the build stage and `COPY --chown=65532:65532`.
- The BE image has no shell or curl, so the compose healthcheck for `be` uses a tiny `/app/healthcheck` binary built from `cmd/healthcheck` (GET `/healthz`). Alternatively, drop the healthcheck and rely on `depends_on: service_started`.

**Dokploy** (VPS):
- **Compose app from the BE repo** using `docker-compose.dokploy.yml`. It contains only `postgres` and `be`, both on the external `dokploy-network`, with named volumes `pgdata` and `uploads` so data survives redeploys.
  - The BE is not given a public domain; the FE reaches it over `dokploy-network`.
  - Secrets (`JWT_SECRET`, `ADMIN_PASSWORD`, `REVALIDATE_SECRET`, DB password) are set in Dokploy's env UI, never in the repo.
  - Set `TRUSTED_PROXY_HOPS=2` and `FRONTEND_URL=http://<fe-app-service-name>:3000`.
- **Application from the FE repo** (Dockerfile, Next standalone output).
  - Its domain plus HTTPS is set in Dokploy (Traefik).
  - Build arg and env `BACKEND_URL=http://<be-service-name>:8080` (the compose service's name or alias on `dokploy-network`).
  - Set `SITE_URL=https://<domain>` and the same `REVALIDATE_SECRET`.
- Deploy order: BE compose first (migrations and seed run automatically), then FE.
- Backups: a Dokploy scheduled `pg_dump` for `pgdata`, plus a copy of the `uploads` volume.

## 10. Coding conventions (BE)

- **Errors**
  - Wrap with `fmt.Errorf("store: get article %s: %w", id, err)`.
  - Never leak internal errors to clients. The adapter in §3 is the only place that writes error responses.
  - No `panic` outside `main` wiring.
- **Validation**
  - Each input DTO has `Validate() error` that returns `ValidationError{Fields: map[string]string}`.
  - Keys are camelCase API field names; messages are in Bahasa Indonesia (e.g. `"Wajib diisi."`, `"Maksimal 160 karakter."`).
  - Limits must match the `maxLength`, `pattern` and `enum` values in `openapi.yaml`. When changing one, change both.
  - Decode with `DisallowUnknownFields`; an unknown field → 400.
  - Trim surrounding whitespace on strings before validating.
- **DTOs**
  - Explicit request and response structs in `internal/api` with `json:"camelCase"` tags.
  - Nullable fields are pointers and serialize as `null`, never omitted. That means **no `omitempty`** on fields the spec marks required.
  - Lists are never `nil`; initialise them to `[]` so they serialize as `[]`.
  - Times are `time.Time` in UTC, formatted with `time.RFC3339`.
- **SQL**
  - Parameterised queries only.
  - Multi-statement writes (service + packages, article + tags, reorder) use `pgx.BeginFunc`.
  - Allowed sort columns are whitelisted.
- **Logging**: `slog` JSON handler on stdout. Include `request_id` in every log line. Never log passwords, cookies, JWTs, lead message bodies or full emails.
- **Testing**
  - `go test ./...` must pass without a DB. Unit tests cover domain logic: slugify, promo status, read minutes, validation, CSV escaping, sniffing.
  - Integration tests (`//go:build integration`, or skip when `TEST_DATABASE_URL` is unset) run migrations and seed against a throwaway database, then exercise the main endpoints through `httptest`. Each response is validated with kin-openapi `openapi3filter.ValidateResponse` against `api/openapi.yaml`.
  - Required coverage: `GET /site`, `/services`, `/services/{slug}`, `/case-studies`, `/articles` (incl. `?tag=`), `/articles/{slug}`, `/articles/{slug}/related`, `/promos`, `POST /leads` (valid, honeypot, 422, 429), login, me, CSRF rejection, editor → 403 on users/settings, and one full admin CRUD cycle (articles).
  - Run locally with a throwaway cluster (`initdb` + `pg_ctl -o "-p 5433"`).
- **FE contract usage**: the FE copies `api/openapi.yaml` to its own `docs/openapi.yaml` and generates types with `openapi-typescript` (`npm run gen:api`). The BE file is the master copy. Any contract change lands in the BE repo first and is then copied.
