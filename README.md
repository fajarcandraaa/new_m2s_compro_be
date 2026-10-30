# Mind2Screen Company Profile: Backend

Go API for the Mind2Screen company-profile site and its CMS (`/admin` in the FE). It implements the
contract `api/openapi.yaml` (v1.1.0, tag `contract-v1.1`; v1.0 was tag `contract-v1.0`), the data model in `docs/db-schema.md` and the
architecture in `docs/architecture.md`.

- Go 1.24, chi, pgx/v5, goose v3 (embedded SQL migrations), PostgreSQL 16
- JWT (HS256) in the httpOnly cookie `m2s_session`, bcrypt, session-version revocation
- bluemonday UGC sanitisation for article HTML, strict (plain text) for FAQ items; image uploads (JPEG/PNG/WebP, 2 MiB) served at `/uploads/*`
- Migrations and the idempotent seed run on start; the seed reads `docs/content/*.json`, which is embedded in the binary

## Layout

```
cmd/api            entrypoint: config → pool → migrate → seed → router → graceful shutdown
assets.go          //go:embed migrations/*.sql and docs/content/*.json
migrations/        00001_users.sql … 00009_faqs.sql (goose)
internal/config    env parsing and validation
internal/db        pgxpool + goose runner
internal/seed      idempotent seed (admin from ADMIN_*)
internal/store     SQL per resource
internal/domain    entities, slugify, promo status, E.164, read minutes, validation, sanitiser
internal/auth      bcrypt, JWT cookie, session/role/CSRF middleware
internal/httpx     envelopes, error mapping, JSON decoding, query parsing, client IP, logging
internal/api       router, handlers and DTOs (+ contract tests)
internal/media     upload sniffing/storage and the /uploads file server
internal/revalidate  async POST {FRONTEND_URL}/api/revalidate
internal/ratelimit in-memory token bucket (login 10/min, leads 5/min per client IP)
scripts/smoke.sh   curl smoke test of every endpoint
```

## Run locally (without Docker)

Requirements: Go 1.24 and a PostgreSQL 16 database.

```bash
cp .env.example .env            # edit DATABASE_URL and the secrets
set -a; . ./.env; set +a
GOTOOLCHAIN=local go run ./cmd/api
# → migrations applied, seed done, listening :8080
curl localhost:8080/healthz      # {"status":"ok","db":"ok"}
```

For plain http on localhost keep `COOKIE_SECURE=false`, otherwise the browser drops the session cookie.
The admin user is created from `ADMIN_EMAIL` / `ADMIN_PASSWORD` only while the `users` table is empty.

### Quality checks and tests

```bash
export GOTOOLCHAIN=local
go build ./... && go vet ./... && gofmt -l .
go test ./...                                   # unit tests; DB tests are skipped
TEST_DATABASE_URL='postgres://postgres@127.0.0.1:5433/m2s_test?sslmode=disable' go test ./...
```

With `TEST_DATABASE_URL` set, the integration tests in `internal/api`:

- drop and recreate the `public` schema of that database (**use a throwaway database**);
- run the migrations and the seed (twice, to prove the seed is idempotent);
- exercise the endpoints through `httptest`;
- validate every response against `api/openapi.yaml` with kin-openapi v0.135.0 (`openapi3filter.ValidateResponse`).

Set `TEST_LOG=1` to see request logs while the tests run.

Without a database, `TestSpecExamplesValid` still checks every request and response example in `api/openapi.yaml`
against its schema.

kin-openapi is pinned at v0.135.0: later releases require Go ≥ 1.25. The tests skip `doc.Validate()`, which
fails on 3.1 `type: "null"` in that version.

Smoke test against a running server with a freshly seeded database. It needs `curl`, `jq` and `python3`;
the results from 2026-10-15 are in `docs/qa/be-smoke.md`.

```bash
B=http://localhost:8080 ADMIN_EMAIL=admin@mindtoscreen.com ADMIN_PASSWORD=... scripts/smoke.sh
```

## Run with Docker Compose (local, BE + FE + Postgres)

Clone both repos next to each other:

```
parent/
  new_m2s_compro_be/   (this repo, contains docker-compose.yml)
  new_m2s_compro_fe/
```

```bash
cd new_m2s_compro_be
cp .env.example .env     # set POSTGRES_PASSWORD (URL-safe), JWT_SECRET, ADMIN_*, REVALIDATE_SECRET
docker compose up --build
```

| Service | Image / build | Port | Notes |
|---|---|---|---|
| `postgres` | `postgres:16-alpine` | not published | volume `pgdata`; healthcheck `pg_isready` |
| `be` | `build: .` (multi-stage → distroless static, nonroot) | `8080` | waits for healthy postgres; volume `uploads:/data/uploads`; `COOKIE_SECURE=false` |
| `fe` | `../new_m2s_compro_fe` with build args `BACKEND_URL=http://be:8080` and `NEXT_PUBLIC_SITE_URL=${SITE_URL}` | `3000` | open http://localhost:3000 and http://localhost:3000/admin |

Inside compose, `DATABASE_URL` is built from `POSTGRES_*`, and `FRONTEND_URL` is `http://fe:3000`. The BE image has
no shell, so `be` has no compose healthcheck. Use `GET /healthz` from outside instead.

Reset everything: `docker compose down -v` (this deletes the database and the uploads).

## Deploy on Dokploy (VPS)

Deploy the BE first; it runs migrations and the seed automatically. Then deploy the FE.

1. **Compose application from this repo** (postgres + be)
   - Compose path: `docker-compose.dokploy.yml`. Both services run on the default network. `be` also joins the
     external `dokploy-network` with the alias `m2s-be`.
   - The named volumes `pgdata` and `uploads` keep the data across redeploys.
   - Do not give the BE a public domain. The FE reaches it over `dokploy-network`.
   - Environment (Dokploy UI):
     - `POSTGRES_PASSWORD`, `JWT_SECRET`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `REVALIDATE_SECRET`
     - `FRONTEND_URL=http://<fe-app-service-name>:3000` (the FE app's internal name on `dokploy-network`)
     - `TRUSTED_PROXY_HOPS=1`, `LOG_LEVEL=info`
     - `COOKIE_SECURE` is `true` in this file.
2. **Application (Dockerfile) from the FE repo**
   - Set a domain with HTTPS (Traefik).
   - Build arg **and** runtime env `BACKEND_URL=http://m2s-be:8080`.
   - Build arg `NEXT_PUBLIC_SITE_URL=https://<domain>` (inlined at build: canonical URLs, robots.txt, sitemap, OG).
   - Runtime env `SITE_URL=https://<domain>` and the same `REVALIDATE_SECRET` as the BE.
3. Backups: a scheduled `pg_dump` of the database, plus a copy of the `uploads` volume.

### Verify `TRUSTED_PROXY_HOPS` on the first deploy

Rate limiting (`POST /leads` 5/min, `POST /auth/login` 10/min) and `leads.ip_hash` use the client IP, taken
`TRUSTED_PROXY_HOPS` entries from the right of `X-Forwarded-For` (or `RemoteAddr` if XFF is shorter).

- Traefik appends the visitor's IP to XFF.
- Next.js only sets XFF when it is absent; it does not append.
- So `1` is expected, both locally and on Dokploy.

To verify it:

1. Temporarily set `LOG_LEVEL=debug` on the BE.
2. Submit the `/diskusi` form from a phone on mobile data.
3. Find the `"msg":"client ip"` line in the BE logs. It shows `xff`, `remote_addr` and `client_ip`.
4. Check that `client_ip` is the phone's public IP, not the FE container IP or a value the browser could forge.
   If it is wrong, adjust `TRUSTED_PROXY_HOPS`.
5. Set `LOG_LEVEL` back to `info`.

The form must post from the browser through the FE's `/api/v1/*` rewrite, never through a Server Action.

## Environment variables

| Name | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | required | `postgres://user:pass@host:5432/db?sslmode=disable` |
| `MIGRATE_ON_START` | `true` | Run embedded goose migrations on boot (failure = exit 1) |
| `SEED_ON_START` | `true` | Run the idempotent seed on boot |
| `JWT_SECRET` | required, ≥ 32 chars | HS256 key; also the HMAC key for `leads.ip_hash` |
| `JWT_TTL` | `12h` | Session lifetime and cookie Max-Age (Go duration) |
| `COOKIE_SECURE` | `true` | `Secure` flag on `m2s_session`; `false` for plain-http local runs |
| `ADMIN_EMAIL` | none | Initial admin (only when `users` is empty) |
| `ADMIN_PASSWORD` | none | Initial admin password, 10–64 chars and ≤ 72 bytes; otherwise skipped with a warning |
| `ADMIN_NAME` | `Admin` | Initial admin display name |
| `UPLOAD_DIR` | `/data/uploads` | Upload storage (Docker volume) |
| `UPLOAD_MAX_BYTES` | `2097152` | Max upload size |
| `FRONTEND_URL` | `http://fe:3000` | Base URL for `POST /api/revalidate` |
| `REVALIDATE_SECRET` | none | Shared with the FE; empty disables revalidation |
| `TRUSTED_PROXY_HOPS` | `1` | Client IP position from the right of `X-Forwarded-For` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | `m2s` / `m2s` / required | Compose files only (postgres service + `DATABASE_URL`) |
| `SITE_URL` | `http://localhost:3000` | Compose only, passed to the FE |

## API notes

### Conventions

- Base path `/api/v1`. `GET /healthz` lives at the root, and uploads are served at `/uploads/*`.
- Envelopes:
  - single item: `{data}`
  - list: `{data, meta:{page,pageSize,total}}`
  - error: `{error:{code,message,fields?}}`, where `fields` keys are dot paths such as `packages.0.priceValue`
- JSON request bodies:
  - unknown fields → 400
  - bodies over 1 MiB → 413
  - POST/PUT are full-object writes; PATCH is partial, and `{}` → 422

### Roles and CSRF

- **CSRF:** every mutating `/admin/*` request, plus `/auth/login` and `/auth/logout`, needs
  `X-Requested-With: m2s-cms`. The header is checked before the session, so a missing header → 403 `csrf_failed`.
- **Admin only:** `PUT /admin/settings`, `/admin/users/*`, `PATCH /admin/leads/{id}`, `GET /admin/leads/export`.
- **Admin and editor:** everything else under `/admin`, including `GET /admin/settings` and `PATCH /admin/settings/toggles`.

### Revalidation tags

The BE sends these after each successful mutation; failures are only logged.

| Mutation | Tags |
|---|---|
| settings, toggles | `site` |
| services | `services`, `promos` |
| case studies | `case-studies` |
| articles | `articles` |
| promos | `promos` |
| testimonials | `testimonials` |
| client logos | `client-logos` |
