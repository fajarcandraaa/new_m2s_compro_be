# Contract review: Backend (Gate 1)

Reviewer: Senior Backend Engineer (Go)
Inputs reviewed: `api/openapi.yaml` (cb3a436), `docs/db-schema.md`, `docs/architecture.md`, `docs/brief.md`, `docs/content/seed.json`, `docs/content/articles.json`
Date: 2026-10-15

## Summary

The contract is implementable today in Go 1.24 with chi + pgx/v5 + goose + bluemonday. Shapes are consistent, envelopes are uniform, enums match the DB `CHECK`s, and every operation lists its roles. I estimate about 62 operations, roughly 7 to 9 focused hours for the BE including the required kin-openapi integration tests, assuming generic helpers for list/pagination/reorder/search.

Checks I ran:
- **Seed vs. input schemas.** Every seed record (settings, 5 services + packages, 2 case studies, 3 articles) validates against `SettingsInput` / `ServiceInput` / `CaseStudyInput` / `ArticleInput` once mapped to camelCase, with `""` URLs mapped to `null`, `status=published` and `seo` all-null. No maxLength or pattern violations, so the seed is feasible as written.
- **kin-openapi.** Loaded the spec with Go 1.24.7 and ran `openapi3filter.ValidateResponse` against sample bodies, including `type: [string, "null"]` and `oneOf: [ServiceRef, {type: null}]`. Findings are in B1.

---

## Blocking (must change before freeze)

### B1. kin-openapi v0.150.0 does not build on Go 1.24
- **Location:** `docs/architecture.md` §2 (Dependencies) and §9 (Docker `golang:1.24-alpine`); brief §2 (Go 1.24).
- **Problem:** `github.com/getkin/kin-openapi@v0.150.0` declares `go 1.25`. Every release from v0.137.0 onward declares go 1.25, and v0.136.0 declares go 1.26. With `GOTOOLCHAIN=local` (the default in the official golang Docker images) `go test` fails with `requires go >= 1.25`. Adding it to `go.mod`, even as a test-only dependency, makes `go mod tidy` raise the module's `go` directive to 1.25, which then breaks `go build` in the `golang:1.24-alpine` stage. That violates the DoD (`go build/vet/test` green) and the PM-fixed stack.
- **Proposed change:** Pin **`github.com/getkin/kin-openapi v0.135.0`** (declares go 1.22.5). I verified on Go 1.24.7 that it loads this spec, routes with `routers/gorillamux`, and validates responses correctly, including nullable type arrays and the `oneOf … null` promo `service`. It accepts valid bodies and rejects `mapsUrl: 5` and unknown properties. One caveat: `doc.Validate(ctx)` fails on v0.135.0 with `unsupported 'type' value "null"`, so the tests must skip `doc.Validate` and call only `ValidateResponse`. Update architecture.md §2 accordingly. The alternative is for the PM to approve Go 1.25 for the BE (Dockerfile `golang:1.25-alpine`), but that changes brief §2.

### B2. Role matrix deviates from brief §3 and needs explicit PM sign-off
- **Location:** `GET /admin/settings`, `PATCH /admin/settings/toggles` (`x-roles: [admin, editor]`), `GET /admin/leads/export` (`[admin, editor]`); brief §3: "Hanya `admin` yang boleh ke `settings` dan `users`".
- **Problem:** The spec gives editors read access to settings and write access to the two homepage toggles. That is reasonable, because brief §6 puts the `show_testimonials` / `show_client_logos` switches on the Testimoni screen, which editors use. But brief §3 is a "do not change without approval" section. Separately, a bulk PII export (CSV of all leads) is a stretch of "baca leads".
- **Proposed change:** PM explicitly approves, or rejects, (a) editor `GET /admin/settings` + `PATCH /admin/settings/toggles`. I recommend approving it, since otherwise the Testimoni screen is half-broken for editors. PM also decides (b) whether `GET /admin/leads/export` is admin-only. I recommend admin-only, which costs one word in `x-roles` plus the description. Record the decision in brief §3 or in the spec's `info.description`.

### B3. "required" vs. Go JSON decoding: define what the BE actually enforces
- **Location:** All `*Input` schemas, especially required-nullable fields (`ArticleInput.publishedAt`, `coverImageUrl`, `Seo.*`, `PromoInput.serviceId/code`, `TestimonialInput.company`, `SettingsInput.*Url`), required booleans (`CaseStudyInput.featured`, `TestimonialInput.isActive`, `UserUpdateInput.isActive`), required strings without `minLength` (`tagline`, `eyebrow`, …), and the PATCH bodies `LeadPatchInput` and `SettingsTogglesInput`.
- **Problem:** `encoding/json` with `DisallowUnknownFields` catches unknown keys, but it cannot tell a missing key from `null`, `false` or `""`. A PUT that omits `isActive` silently deactivates a testimonial, and a missing `featured` becomes `false`. Nothing in the stack validates requests against the schema (kin-openapi is test-only). PATCH needs real presence semantics: `notes: null` (clear) is not the same as `notes` absent (keep). The contract currently implies strictness the BE will not deliver.
- **Proposed change:** Add to `info.description` → Conventions:
  1. "For PUT/POST bodies the BE treats a missing property the same as its zero value (`null` / `false` / `""` / `[]`). Clients must always send the full object, which the generated types enforce. Required-ness is enforced by FE types, not by a BE 422."
  2. "For PATCH bodies (`LeadPatchInput`, `SettingsTogglesInput`) property presence is significant: an absent key means unchanged, and `notes: null` or `notes: ""` clears notes. An empty object `{}` returns 422 `validation_error`."

  BE side: PATCH DTOs decode into a small `Optional[T]` type that tracks `Set`/`Null`, about 20 lines. I do not recommend adding runtime kin-openapi request validation today: its messages are English, its error-to-field mapping takes extra work, and its 3.1 support is partial.

### B4. Lead rate limiting: the client-IP source is under-specified and probably misconfigured
- **Location:** `POST /leads` description (5/min per client IP); `docs/architecture.md` §3 (`RealIP` + `TRUSTED_PROXY_HOPS`) and §8 (`Dokploy = 2`); brief §3 (rate limit per IP).
- **Problem:**
  1. If the FE submits the `/diskusi` form through a Server Action or a Route Handler, every lead reaches the BE from the FE container's IP. That gives one global bucket of 5 leads/min for all visitors, and the 6th real customer in a minute gets 429. Nothing in the contract forbids this.
  2. As far as I know, Next.js does not *append* to `X-Forwarded-For` when proxying a rewrite. It sets the header only if it is absent (`??=`). If that is right, then in Dokploy the chain browser → Traefik (appends client) → Next (leaves header unchanged) → BE yields an XFF whose rightmost entry is the real client, so the correct value is `TRUSTED_PROXY_HOPS=1`, not 2. With 2 the BE picks an attacker-supplied entry (spoofable) or falls back to the Next container IP (global bucket). I could not verify the Next version here (FE `node_modules` are absent), so this needs a quick empirical check.
- **Proposed change:**
  - Spec (`POST /leads` description): "Must be called from the browser via the same-origin `/api` rewrite. Do not proxy through a Server Action or Route Handler, because rate limiting keys on the client IP."
  - Architecture §3/§8 needs three changes. Define the algorithm: take the entry `TRUSTED_PROXY_HOPS` positions from the right of XFF; if XFF has fewer entries, use `RemoteAddr`; strip the port; use the result only for rate limiting and `ip_hash`. Change the Dokploy guidance to "verify with a debug log of XFF/RemoteAddr on first deploy; expected 1". Add a BE debug log line of the derived IP at `LOG_LEVEL=debug`.
  - The same keying applies to the `POST /auth/login` limit (10/min).

---

## Should change

### S1. The featured-case-study check in db-schema is invalid SQL and does not prevent races
- **Location:** `docs/db-schema.md` → `case_studies` ("`SELECT count(*) ... FOR UPDATE`"); `CaseStudyInput` 409.
- **Problem:** PostgreSQL rejects `FOR UPDATE` with aggregates (`FOR UPDATE is not allowed with aggregate functions`). Row locks also do not stop two concurrent transactions from each featuring a third row. The rule's semantics are also unclear: does it count drafts, and does publishing an already-featured draft trigger it?
- **Proposed change:** Inside the write transaction, run `SELECT pg_advisory_xact_lock(hashtext('case_studies_featured'))`, then `SELECT count(*) FROM case_studies WHERE featured AND status='published' AND id <> $1`. Return 409 `conflict` with `fields.featured` when the resulting row would be `featured && published` and the count is ≥ 2. A featured draft is allowed. Publishing a featured draft is checked. Write this sentence into `CaseStudyInput.description`.

### S2. Slug handling on create/update is ambiguous
- **Location:** `ServiceInput.slug`, `CaseStudyInput.slug`, `ArticleInput.slug` ("optional, generated from title").
- **Problem:** The contract does not say what happens on PUT when `slug` is omitted (keep it or regenerate it), how a generated slug collision is handled (409 or a suffix), how long a generated slug from a 200-char title may be (slug max is 120), or what happens with a title that slugifies to empty (e.g. "!!!").
- **Proposed change:** Add to each Input description: "POST without `slug` → `slugify(title)`, truncated to 120 chars at a hyphen; on collision append `-2`, `-3`, …. An explicit `slug` that collides → 409 `slug_taken` with `fields.slug`. PUT without `slug` keeps the current slug. A title that slugifies to empty → 422 on `title`."

### S3. Public promos leak draft services
- **Location:** `Promo.service` (public `GET /promos`).
- **Problem:** The `ServiceRef` join ignores the service status, so a promo linked to a draft service exposes the draft's title and links to a page that returns 404.
- **Proposed change:** Add to `Promo.service.description`: "null when the related service is missing or not `published`." The admin `PromoAdmin.service` keeps showing it regardless of status.

### S4. URL fields accept `javascript:` and other schemes
- **Location:** `Site/SettingsInput.mapsUrl|linkedinUrl|instagramUrl` (`format: uri`); `Seo.ogImageUrl`, `ArticleInput.coverImageUrl`, `ClientLogoInput.imageUrl` (`format: uri-reference`).
- **Problem:** `format: uri` and `uri-reference` both accept `javascript:alert(1)`; I confirmed kin-openapi accepts it for `Seo.ogImageUrl`. The footer renders the settings URLs as `href`, which makes them a stored XSS vector. Editors can set image URLs.
- **Proposed change:** Add `pattern: ^https?://` to the three settings URLs. Add `pattern: ^(/uploads/[A-Za-z0-9._-]+|https://.+)$` to the image/OG URL fields. The BE mirrors these in `Validate()`.

### S5. Password `maxLength: 72` counts characters, but bcrypt's limit is 72 bytes
- **Location:** `UserCreateInput.password`, `PasswordResetInput.password`, `LoginInput.password`; `ADMIN_PASSWORD`.
- **Problem:** `golang.org/x/crypto/bcrypt.GenerateFromPassword` returns `ErrPasswordTooLong` above 72 **bytes**. A 40-character password with multibyte characters passes the schema and then becomes a 500.
- **Proposed change:** Description: "8–72 characters and at most 72 bytes UTF-8; over 72 bytes → 422." The BE checks `len([]byte(p)) <= 72`. The seed refuses to create the admin with a warning when `ADMIN_PASSWORD` is under 8 characters or over 72 bytes.

### S6. Lead `whatsapp` normalisation is unspecified, and the pattern accepts junk
- **Location:** `LeadCreateInput.whatsapp` ("BE normalises"), `Lead.whatsapp`.
- **Problem:** `^\+?[0-9 ()-]{8,20}$` accepts `((((((((` and `--------`. "Normalises" does not say to what, but the CMS "Hubungi via WhatsApp" button needs `wa.me/<digits>`.
- **Proposed change:** "Strip spaces, `(`, `)` and `-`. Leading `0` → `+62`, leading `62` → `+62`. Result must match `^\+[1-9][0-9]{7,14}$`, else 422. Stored and returned in E.164." `Lead.whatsapp` then gets the same pattern as `Site.whatsappNumber`.

### S7. CSV export columns and format are unspecified
- **Location:** `GET /admin/leads/export`.
- **Problem:** The column set, order, header labels, timestamp format and timezone are undefined, and the injection guard misses `\t` and `\r` (OWASP list).
- **Proposed change:** Columns: `createdAt, name, company, email, whatsapp, serviceSlug, serviceTitle, startTimeframe, status, notes, message`, with header labels in English camelCase as listed. Times use RFC3339 UTC. `Content-Type: text/csv; charset=utf-8`, BOM, CRLF line endings. Prefix `'` for cells starting with `= + - @ \t \r`. Order is `createdAt desc`. Also see B2 on the role.

### S8. Ordering is missing for the admin leads list
- **Location:** `GET /admin/leads`.
- **Problem:** No order is stated. Every other list has one.
- **Proposed change:** "Ordered by createdAt desc." The `leads_created_idx` index already supports it.

### S9. Reorderable admin lists are paginated, but reorder needs every id
- **Location:** `GET /admin/services`, `/admin/testimonials`, `/admin/client-logos` (Page/PageSize/Q); `ReorderInput` ("every id … missing → 422").
- **Problem:** With the default `pageSize=20` and a `q` filter, the CMS can display a subset and then send a partial reorder, which returns 422. Pagination on tables with 5–20 rows is extra work for both sides.
- **Proposed change:** Make these three admin lists unpaginated: drop `Page`/`PageSize`/`Q` and keep `meta` with page=1, as on the public lists. Keep the `status`/`audience`/`isActive` filters for display only, and note that reorder must send all ids regardless of filter. That is less code and removes a whole class of bugs.

### S10. Invalid query/path parameters on public endpoints: 400 vs. 404
- **Location:** `/services/{slug}`, `/case-studies/{slug}`, `/articles/{slug}`, `/articles/{slug}/related` (only 404 documented); `Page`/`PageSize`/`limit`/boolean query params.
- **Problem:** A slug that fails the pattern (e.g. `/articles/Foo_Bar`) has no defined status. Out-of-range `pageSize`, `page` past the end, and `featured=1` are also undefined.
- **Proposed change:** Add to Conventions: "A path slug that does not match the pattern → 404 `not_found`, so the FE renders a real 404. An admin `{id}` that is not a UUID → 400. Query values outside the schema (`pageSize>100`, `page<1`, a boolean other than `true`/`false`, an unknown enum value) → 400 `bad_request`. A page past the end → 200 with `data: []` and the real `total`. Unknown query params are ignored."

### S11. CSRF scope: architecture contradicts the spec
- **Location:** `docs/architecture.md` §3 (CSRF only in the `/api/v1/admin` group, after `RequireSession`); spec (`/auth/login` and `/auth/logout` require `X-Requested-With`).
- **Problem:** An engineer following architecture.md will not guard `/auth/*`, and login CSRF stays open.
- **Proposed change:** Architecture §3: "`csrf` middleware applies to mutating methods on `/api/v1/admin/*` and `/api/v1/auth/*`. It runs before `RequireSession`, so a request missing the header returns 403 `csrf_failed` even when unauthenticated." State the precedence explicitly, because the integration test asserts it.

### S12. Tag creation: slug collisions and races
- **Location:** `ArticleInput.tags` ("unknown names are created"); db-schema `tags` (unique `slug` and unique `lower(name)`).
- **Problem:** "AI Teknologi" and "AI & Teknologi" both slugify to `ai-teknologi`, and the unique violation would map to a misleading 409 `slug_taken`. Two editors creating the same tag concurrently also collide.
- **Proposed change:** "Tags are resolved by derived slug: an existing tag with the same slug is reused (its existing name wins). Duplicate names within the payload are deduped." Implementation: `INSERT … ON CONFLICT (slug) DO NOTHING`, then `SELECT id … WHERE slug = ANY($1)`, inside the article transaction.

### S13. `ServicePackage.highlighted` "at most one" is not enforced
- **Location:** `ServicePackage.highlighted`.
- **Problem:** The description reads like a rule, but nothing enforces it.
- **Proposed change:** Either "more than one highlighted → 422 on `packages`" (two lines of Go) or reword it to "FE convention, not validated". I prefer the 422.

### S14. Unspecified statuses: JSON body cap, unknown routes, method not allowed
- **Location:** architecture §3 error table; `Error.code` enum.
- **Problem:** The table has no entry for a body over the 1 MiB `MaxBytesReader` cap (JSON endpoints), for an unknown route under `/api/v1`, or for a wrong method. chi returns plain-text 404/405 by default, so the FE's error parser breaks on them.
- **Proposed change:** A JSON body over 1 MiB → 413 `payload_too_large`. Set chi `NotFound` → JSON 404 `not_found` and `MethodNotAllowed` → JSON 405 with code `bad_request` (or add `method_not_allowed` to the enum). Add 413/415/429 rows to the architecture table.

### S15. Public `GET /promos?status=ended` window
- **Location:** `GET /promos` `status` param.
- **Problem:** It is unclear whether `status=ended` on the public endpoint returns all ended promos or only those within the 7-day window.
- **Proposed change:** "Public `ended` is limited to the 7-day window. The admin list returns all ended promos."

---

## Nice to have / accept as-is

- **N1. `meta` on limited lists** (`/testimonials?limit`, `/articles/{slug}/related`): define `total` as the number returned and `pageSize = max(returned, 1)`, matching the "unpaginated" rule. Also note that the rule `pageSize = max(total,1)` breaks `PageMeta.pageSize.maximum: 100` above 100 rows. That is acceptable for v1 volumes, but say so.
- **N2. `maxLength` counts code points** (JSON Schema semantics). The BE uses `utf8.RuneCountInString`, and zod counts UTF-16 units, which differ only for astral characters such as emoji. Accept.
- **N3. `LeadCreateInput.service` vs. `Lead.serviceSlug` naming:** rename the input to `serviceSlug` for symmetry, or accept as-is.
- **N4. Honeypot ordering:** the BE checks the honeypot **before** validation, so a bot sending `website` longer than 200 chars still gets the fake 201 and the honeypot does not leak through a 422. Honeypot hits still count toward the rate limit. This is a description tweak only.
- **N5. Login validation:** a malformed email on login returns 401 `unauthorized` instead of 422 (uniform, no oracle). Optional.
- **N6. On 401 from a stale session** (inactive user or `sv` mismatch), the BE also clears `m2s_session`. Cheap; avoids a redirect loop in FE middleware.
- **N7. Admin resets their own password or changes their own email:** `session_version` increments and the admin gets logged out. Either re-issue the cookie in that response or document it. I will re-issue.
- **N8. `ip_hash`:** use `HMAC-SHA256(key=JWT_SECRET, ip)` instead of `sha256(ip + secret)`. Same cost, correct construction. Coupling it to JWT rotation is acceptable.
- **N9. In-memory rate limiter:** add a periodic sweep of idle buckets so the map does not grow without bound. Implementation detail.
- **N10. bluemonday UGC:** strips `target="_blank"` from Tiptap links and adds `rel="nofollow"`. Acceptable. If wanted, the BE can call `AddTargetBlankToFullyQualifiedLinks(true)`. Relative `/uploads/...` image `src` survives UGC (`AllowRelativeURLs`).
- **N11. EXIF metadata** in uploaded JPEGs is kept (possible GPS data in photos). Re-encoding is out of scope today. Accept and note it in the README.
- **N12. `MediaUpload.size.maximum: 2097152`** is hard-coded while `UPLOAD_MAX_BYTES` is configurable. Raising the env would break response validation. Accept, or drop the `maximum`.
- **N13. Healthcheck binary** (`cmd/healthcheck`): skip it and use `depends_on: service_started` for `fe`, since we cannot `docker build` here anyway. That saves time.
- **N14. Rewrite source:** recommend that the FE rewrite `/api/v1/:path*` rather than `/api/:path*`. That way the FE's own `/api/revalidate` route can never be shadowed (it is safe today only because default rewrites run after filesystem routes).
- **N15. Seed:** the seed runs every record through the same `Validate()` as the CMS before insert. Verified feasible today: all records pass. Seed `published_at` values (`+07:00`) are converted to UTC. Seed services and case studies get `status=published` and `seo` all-null. The Dockerfile must `COPY docs/content` (embedded from the module root via `assets.go`).
- **N16. Orphan tags** from draft-only articles appear in public `GET /tags` with `articleCount = 0`. The FE hides zero-count tags on `/artikel`. Accept.
- **N17. Route static vs. param** (`/admin/services/order` vs. `/admin/services/{id}`, `/admin/leads/export` vs. `/admin/leads/{id}`): chi prefers static segments, so this works. Accept.
- **N18. Over-engineering check:** `session_version` revocation, image dimension sniffing, the login rate limit, related articles and CSV export are each cheap (under 30 minutes) and are in the brief, so keep them. The only real descoping I propose is S9 (no pagination on reorderable admin lists) and N13.

---

## Freeze position

**I agree to freeze after the Blocking items are resolved.** B1 and B4 are architecture.md edits plus one line in the `POST /leads` description. B2 is a PM decision. B3 is a Conventions paragraph in `info.description`. None of them changes a response shape, so the FE's generated types are unaffected. I would like the Should-change items folded into the same edit where they are one-liners (S2, S3, S5, S7, S8, S10, S15 are descriptions only; S4 adds patterns; S9 removes params). I will not block on them, and I will implement S1, S11, S12 and S14 to the behaviour written above either way.
