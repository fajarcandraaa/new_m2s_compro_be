# Final code review (Phase 3): BE + FE

- **Reviewer:** Senior Tech Lead (author of `contract-v1.0`)
- **Date:** 2026-10-15
- **Scope:** `new_m2s_compro_be` and `new_m2s_compro_fe`, branch `feat/initial-build`
- **References:** `api/openapi.yaml` (tag `contract-v1.0`), `docs/brief.md`, `docs/reviews/gate1-decisions.md`

## Verdict

**Ready for hand-off.** I found no Critical or High defects. Six small defects are fixed in this review: one deploy fix, two robustness fixes in the BE and three in the FE. The open items are hardening or deploy-time checks, and none of them blocks go-live.

| Severity | Found | Fixed | Open |
|---|---|---|---|
| Critical | 0 | 0 | 0 |
| High | 0 | 0 | 0 |
| Medium | 4 | 3 | 1 |
| Low | 10 | 3 | 7 |

## Commits from this review

| Repo | Commit | Change |
|---|---|---|
| BE | `7aa1792` | compose: `NEXT_PUBLIC_SITE_URL` build arg for `fe`, plus a README update |
| BE | `13afcd5` | AccessLog wraps Recoverer; startup warning when revalidation is disabled |
| FE | `a4d3101` | absolute image URLs no longer crash `next/image`; JSON-LD image URL built with `new URL` |
| FE | `76368bc` | branded root `error.tsx` |
| BE | (this file) | review report |

## Findings

### Medium

| ID | Finding | Location | Status |
|---|---|---|---|
| M1 | **`NEXT_PUBLIC_SITE_URL` was missing from the local compose FE build.** The FE inlines it at build time, and `robots.txt`, `sitemap.xml`, canonical/OG URLs and pages pre-rendered at build all read it. Before the fix, the local stack always baked in the Dockerfile default. The FE Dockerfile already declares `ARG NEXT_PUBLIC_SITE_URL`. I used `${SITE_URL:-http://localhost:3000}`, which matches the runtime `SITE_URL` default and avoids an "unset variable" warning. `docker-compose.dokploy.yml` does not build the FE, so it needs no change there. The README Dokploy section now lists the build arg. | BE `docker-compose.yml:57`, `README.md:95,119`; FE `Dockerfile:20` | **Fixed** (`7aa1792`) |
| M2 | **Absolute image URLs crashed public pages.** The contract and BE validation (`mediaURLRe`) accept `https://…` for `coverImageUrl`, `seo.ogImageUrl` and `client-logos.imageUrl`. `next.config.ts` only configures `localPatterns` for `/uploads` and `/brand`, so `next/image` threw "hostname is not configured" during render. One logo created through the API with an external URL would break the homepage hero, and an article with such a cover would break `/artikel`. Absolute URLs now render with `unoptimized`. | FE `src/components/site/ArticleCover.tsx:11`, `src/components/site/ClientLogo.tsx:13` | **Fixed** (`a4d3101`) |
| M3 | **The FE had no error boundary.** At runtime a failed BE call throws (`settle`), which is correct because ISR then keeps serving the last good page. Pages that were never cached, such as the dynamic `/artikel?tag=…`, fell back to Next's unbranded "Application error". I added a branded root `error.tsx` with "Coba Lagi" and home actions. I smoke-tested it with the BE unreachable: cached `/` returned 200 and the dynamic page returned 500 with the branded boundary. | FE `src/app/error.tsx` | **Fixed** (`76368bc`) |
| M4 | **Client-IP trust depends on the proxy chain.** The rate-limit key is the `TRUSTED_PROXY_HOPS`-th XFF entry from the right. Next's rewrite proxy does not append to XFF: it only sets XFF from the socket when the header is absent. So in Dokploy (Traefik → FE → BE), `hops=1` is correct only if Traefik writes the real client IP last, which is its default for untrusted sources. In the local compose stack nothing sits in front of the FE, so a client-supplied XFF passes through untouched and can be used to evade the 5/min lead limit. That is acceptable locally. **Action:** follow the README check "Verify `TRUSTED_PROXY_HOPS` on the first deploy" (`LOG_LEVEL=debug`, compare `client_ip` with your real IP). This is the same deploy check gate1 BE-B4 asked for. | BE `internal/httpx/middleware.go:22`, `README.md` § Verify TRUSTED_PROXY_HOPS | **Open** (deploy check) |

### Low

| ID | Finding | Location | Status |
|---|---|---|---|
| L1 | **A panic produced no access-log line.** Recoverer was outside AccessLog, so the panic unwound past AccessLog's post-processing. The order is now swapped. | BE `internal/api/router.go:73-75` | **Fixed** (`13afcd5`) |
| L2 | **An empty `REVALIDATE_SECRET` or `FRONTEND_URL` silently disabled revalidation.** It was logged only at debug level, per request. Startup now logs a warning. | BE `cmd/api/main.go:65` | **Fixed** (`13afcd5`) |
| L3 | **Article JSON-LD image** was built as `${SITE_URL}${coverImageUrl}`, which is wrong for absolute URLs. It now uses `new URL(…, SITE_URL)`. | FE `src/app/(site)/artikel/[slug]/page.tsx:52` | **Fixed** (`a4d3101`, together with M2) |
| L4 | **Logout does not revoke the JWT server-side.** It only clears the cookie, which is documented in architecture.md §Sessions. A copied token stays valid until the TTL ends (12 h). Revocation happens on password reset, role change and deactivation. Recommendation: accept for v1. If needed later, add a `jti` deny-list or bump `session_version` on logout (that also logs out other devices). | BE `internal/api/auth.go:55` | Open (accepted) |
| L5 | **Generated-slug race.** Two concurrent creates with the same title can pick the same candidate. The unique index then turns the second into 409 `slug_taken`, although the contract says generated slugs never return 409. This is very unlikely with a few CMS users. Fix later by retrying on 23505 or taking an advisory lock per table. | BE `internal/store/store.go:121` | Open |
| L6 | **`JWT_SECRET` is also the HMAC key for `leads.ip_hash`.** This is key reuse: rotating the JWT secret changes every IP hash. Recommendation: a separate `IP_HASH_SECRET`, or derive a subkey with HKDF. | BE `internal/api/public.go:212` | Open |
| L7 | **The pool size is hard-coded** (`MaxConns = 10`) and overrides `pool_max_conns` in `DATABASE_URL`. That is fine for this traffic. Make it configurable if the deployment grows. | BE `internal/db/db.go:22` | Open |
| L8 | **The 15 s `middleware.Timeout` returns a bare 504** without the error envelope, and 504 is not in the contract. It can only happen on a hung DB. Recommendation: wrap it with a JSON 504, or accept it. | BE `internal/api/router.go:76` | Open |
| L9 | **The sitemap lists only the newest 100 articles** (`getArticles(1, 100)`), and `generateStaticParams` does the same. Older articles still render on demand and stay reachable through links. Recommendation: paginate once there are more than 100 articles. | FE `src/app/sitemap.ts:8` | Open |
| L10 | **No security headers on the FE** (CSP, `X-Frame-Options`/`frame-ancestors`, `Referrer-Policy`). `poweredByHeader` is already off. Recommendation: add `headers()` in `next.config.ts`, at least `frame-ancestors 'none'` for `/admin`. I did not add a CSP here because the inline JSON-LD and the Tiptap editor need testing first. | FE `next.config.ts` | Open |

Informational, no action needed: scheduled articles appear publicly within the 300 s ISR window after `publishedAt`, because nothing fires a revalidation at publish time. This matches the brief.

## What I checked and found correct

### 1. Contract conformance
- Every BE integration-test request is validated against `api/openapi.yaml` with kin-openapi `ValidateResponse` (`IncludeResponseStatus`, so undocumented status codes fail). Unknown routes must return the error envelope (`internal/api/main_test.go:251`).
- FE `src/lib/api/schema.d.ts` regenerated from `docs/openapi.yaml` is byte-identical. `docs/openapi.yaml` is identical to BE `api/openapi.yaml`, and tag `contract-v1.0` exists.
- `src/lib/api/types.ts` only re-exports `components["schemas"]`. All calls go through typed `openapi-fetch` clients. The one raw `fetch` is the multipart upload in `components/admin/media.ts`, which sends the CSRF header.

### 2. Security
- **Session cookie and JWT:**
  - Cookie `m2s_session` is HttpOnly and SameSite=Lax. `Secure` defaults to true, local compose sets false and Dokploy sets true.
  - JWT is HS256 only (`WithValidMethods`), with expiry required and a secret of at least 32 characters enforced at boot.
  - Every admin request loads the user and checks `is_active` and `session_version` (`sv`). A stale session gets 401 and the cookie is cleared.
  - Login uses a dummy-hash comparison for unknown emails, returns a uniform 401 and is rate limited (10/min per IP).
- **CSRF:** `X-Requested-With: m2s-cms` is required on POST/PUT/PATCH/DELETE under `/auth/*` and `/admin/*`. The check runs before the session check, as specified. The browser client always sends it.
- **Role gating, server-side:**
  - The `/admin` group requires admin or editor.
  - `PUT /settings`, `GET /leads/export`, `PATCH /leads/{id}` and every `/users*` route are `adminOnly`.
  - Editors may `GET /settings` and `PATCH /settings/toggles`, per gate1 BE-B2.
  - Admins cannot change their own role, deactivate themselves or delete themselves.
- **Editor restrictions in the UI:**
  - The sidebar hides admin-only groups.
  - The Pengguna and Info Perusahaan pages are wrapped in `AdminOnly`.
  - CSV export and the lead status/notes form are hidden for editors.
- **SQL:**
  - All values are bound parameters (`$n`) through the `where` builder.
  - LIKE input is escaped (`likePattern`).
  - Table and column names come only from constants or whitelists (`slugTables`, `orderTables`).
- **HTML:**
  - Article HTML is sanitised with bluemonday `UGCPolicy` on every write. The seed goes through the same `ArticleInput.Validate`.
  - The FE uses `dangerouslySetInnerHTML` only for that sanitised `contentHtml` and for JSON-LD, where `<` is escaped.
- **Uploads:**
  - The body is capped, then the content type is sniffed and the image must decode (JPEG/PNG/WebP, with the format matching the sniffed MIME).
  - Files get random hex names and are written atomically with mode 0644.
  - `/uploads/*` rejects names that are empty, start with a dot or contain `/` or `\`, and serves only regular files with `nosniff`. I probed `..%2f`, `../` and `%2e%2e`: all return 404.
- **Revalidate route:** a `timingSafeEqual` compare (length check first), 401 when the secret is unset, and validated tag input.
- **Secrets:** none are committed. `.env` is git- and docker-ignored, and the examples use placeholders.
- **CORS:** no CORS middleware. Everything is same-origin through the Next rewrite, and cross-origin `OPTIONS` returns 405.
- **Error responses:** 500 responses use a generic message and log the details server-side. Decode errors are generic.
- **CSV export:** guards against formula injection (`csvSafe`).
- **Login redirect:** the `next` parameter is restricted to `/admin…` paths.

### 3. Business rules
- **Promos:**
  - Status is computed in SQL at query time, with `endsAt` exclusive.
  - The public list shows active, upcoming and promos ended within 7 days.
  - A promo whose service is a draft returns `service: null`.
  - The CMS follows the D+1 00:00 +07:00 convention.
- **Featured case studies:** at most 2 published, enforced with `pg_advisory_xact_lock` and a count inside the save transaction. Featured drafts are allowed.
- **Scheduled articles:** hidden from the public list, detail, related, tag counts and therefore the sitemap (`visibleArticle`). The FE tag filter also hides tags with zero articles.
- **Toggles:** both are honoured on the homepage, and a section is also hidden when it has no items. At most 3 testimonials are shown.
- **Leads:**
  - Rate limited to 5/min per IP.
  - Honeypot `website` returns a fake 201 and stores nothing.
  - Phone numbers are normalised to E.164 with 422 on invalid input.
  - `service` must be a published slug.
  - `company` is optional.

### 4. Robustness
- **Transactions:** article and tag writes, case-study saves with the featured check, and reorders (with `FOR UPDATE`) all run in transactions.
- **N+1:** article tags load in a single `ANY($1)` query per list.
- **Context timeouts:** the request context has a 15 s timeout that cancels pgx queries. The HTTP server sets read-header, read, write and idle timeouts.
- **Graceful shutdown:** SIGINT/SIGTERM triggers `Shutdown` with a 20 s timeout.
- **DB pool:** max 10 connections, 5 min idle timeout, and a ping at boot.
- **FE build without a BE:** a build-time failure renders fallbacks, and at runtime a failure keeps the stale ISR page. `getSite` falls back to `FALLBACK_SITE`.

### 5. Deploy readiness
- **BE Dockerfile:** multi-stage, distroless, nonroot, and the uploads volume is writable by uid 65532.
- **FE Dockerfile:** standalone, non-root, with a healthcheck. It declares both `BACKEND_URL` and `NEXT_PUBLIC_SITE_URL` as ARGs.
- **`docker compose config -q`:** passes for both files with `--env-file .env.example`. Without an env file it fails, which is intended: the required secrets use the `:?` guard, and the README says to `cp .env.example .env` first.
- **`.env.example`:** covers every variable `config.Load` reads, plus the compose-only `POSTGRES_*` and `SITE_URL`.

## Verification (after fixes)

| Check | Result |
|---|---|
| BE `GOTOOLCHAIN=local go build ./...` | OK |
| BE `go vet ./...` | OK |
| BE `gofmt -l .` | no files |
| BE `go test -count=1 ./...` (with `TEST_DATABASE_URL` set to `m2s_test`) | all packages `ok` |
| `docker compose --env-file .env.example config -q` (local and dokploy) | OK |
| FE `npm run lint` | 0 problems |
| FE `npm run typecheck` | OK |
| FE `BACKEND_URL=http://127.0.0.1:8080 npm run build` | OK. Built in a scratch copy so the running `next start` on :3000, which serves `.next`, was not affected. |
| Running services :8080 / :3000 | untouched, both 200 |
