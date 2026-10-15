# BE smoke test (curl)

Date: 2026-10-15. Branch `feat/initial-build`. Contract: `contract-v1.0`.

## Setup

Empty database, local PostgreSQL 16, no Docker. A tiny fake FE (Python `http.server`) listened on
`127.0.0.1:13000` and recorded every `POST /api/revalidate` call and whether `X-Revalidate-Secret` matched.

```bash
# empty DB
psql -h 127.0.0.1 -p 5433 -U postgres -d m2s -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'

# build + run (migrations and seed run on start)
GOTOOLCHAIN=local go build -o ./bin/api ./cmd/api
DATABASE_URL='postgres://postgres@127.0.0.1:5433/m2s?sslmode=disable' \
JWT_SECRET=<32+ chars> ADMIN_EMAIL=admin@mindtoscreen.com ADMIN_PASSWORD=<10-64 chars> \
COOKIE_SECURE=false UPLOAD_DIR=/tmp/m2s-uploads PORT=18080 \
FRONTEND_URL=http://127.0.0.1:13000 REVALIDATE_SECRET=smoke-secret ./bin/api

# smoke test (every endpoint; script prints the table below)
B=http://localhost:18080 ADMIN_PASSWORD=<same> scripts/smoke.sh
```

`scripts/smoke.sh` holds every curl command (method, path, headers and body); each row below is one call.

## Startup on the empty database

```
{"level":"INFO","msg":"migrations applied"}
{"level":"INFO","msg":"seed: admin user created"}
{"level":"INFO","msg":"seed: settings created"}
{"level":"INFO","msg":"seed: services created","count":5}
{"level":"INFO","msg":"seed: case studies created","count":2}
{"level":"INFO","msg":"seed: articles created","count":3}
{"level":"INFO","msg":"seed done"}
{"level":"INFO","msg":"listening","addr":":18080"}
```

Row counts after the seed: users 1, settings 1, services 5, service_packages 4, case_studies 2, tags 3,
articles 3, article_tags 3. `goose_db_version` holds versions 0–8.

A restart against the same database logs `migrations applied` and `seed done` with no `created`
lines, and the row counts stay the same, so the seed is idempotent. SIGTERM logs `shutting down` and the
process exits cleanly.

## Results

| Method | Path | Expected | Got | Result | Note |
|---|---|---|---|---|---|
| GET | `/healthz` | 200 | 200 | ok | db ok |
| GET | `/api/v1/site` | 200 | 200 | ok | seeded settings |
| GET | `/api/v1/services` | 200 | 200 | ok | 5 seeded |
| GET | `/api/v1/services?audience=personal` | 200 | 200 | ok | audience filter |
| GET | `/api/v1/services?audience=x` | 400 | 400 | ok | bad enum |
| GET | `/api/v1/services/web-development` | 200 | 200 | ok | 4 packages |
| GET | `/api/v1/services/tidak-ada` | 404 | 404 | ok | unknown slug |
| GET | `/api/v1/case-studies` | 200 | 200 | ok |  |
| GET | `/api/v1/case-studies?featured=true` | 200 | 200 | ok | max 2 |
| GET | `/api/v1/case-studies/tumbuh-sistem-pertumbuhan-sdm` | 200 | 200 | ok |  |
| GET | `/api/v1/articles?page=1&pageSize=9` | 200 | 200 | ok | 3 seeded |
| GET | `/api/v1/articles?tag=keamanan-data` | 200 | 200 | ok | 2 items |
| GET | `/api/v1/articles/data-ktp-bocor-itu-sebenarnya-dampaknya-apa` | 200 | 200 | ok |  |
| GET | `/api/v1/articles/data-ktp-bocor-itu-sebenarnya-dampaknya-apa/related?limit=3` | 200 | 200 | ok |  |
| GET | `/api/v1/tags` | 200 | 200 | ok | with articleCount |
| GET | `/api/v1/promos` | 200 | 200 | ok | empty seed |
| GET | `/api/v1/testimonials?limit=3` | 200 | 200 | ok |  |
| GET | `/api/v1/client-logos` | 200 | 200 | ok |  |
| GET | `/api/v1/nope` | 404 | 404 | ok | JSON 404 |
| DELETE | `/api/v1/site` | 405 | 405 | ok | JSON 405 |
| POST | `/api/v1/leads` | 201 | 201 | ok | valid, no CSRF needed |
| POST | `/api/v1/leads` | 201 | 201 | ok | honeypot → fake 201 |
| POST | `/api/v1/leads` | 422 | 422 | ok | bad whatsapp |
| POST | `/api/v1/leads` | 400 | 400 | ok | unknown field |
| POST | `/api/v1/leads` | 201 | 201 | ok | 5th request |
| POST | `/api/v1/leads` | 429 | 429 | ok | 6th/min → 429 + Retry-After |
| POST | `/api/v1/auth/login` | 403 | 403 | ok | missing CSRF header |
| POST | `/api/v1/auth/login` | 401 | 401 | ok | wrong password |
| GET | `/api/v1/auth/me` | 401 | 401 | ok | no cookie |
| POST | `/api/v1/auth/login` | 200 | 200 | ok | sets m2s_session |
| GET | `/api/v1/auth/me` | 200 | 200 | ok |  |
| POST | `/api/v1/admin/articles` | 403 | 403 | ok | CSRF checked before session |
| GET | `/api/v1/admin/dashboard` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/settings` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/settings` | 200 | 200 | ok | E.164 normalised; revalidate site |
| PUT | `/api/v1/admin/settings` | 422 | 422 | ok | javascript: URL |
| PATCH | `/api/v1/admin/settings/toggles` | 200 | 200 | ok | partial |
| PATCH | `/api/v1/admin/settings/toggles` | 422 | 422 | ok | {} → 422 |
| PATCH | `/api/v1/admin/settings/toggles` | 200 | 200 | ok | restore |
| GET | `/api/v1/admin/services` | 200 | 200 | ok | unpaginated |
| POST | `/api/v1/admin/services` | 201 | 201 | ok | slug generated, appended |
| GET | `/api/v1/admin/services/a8555733-effe-4f87-9982-cf83fe0c82a4` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/services/a8555733-effe-4f87-9982-cf83fe0c82a4` | 200 | 200 | ok | slug kept |
| POST | `/api/v1/admin/services` | 409 | 409 | ok | explicit slug taken |
| PUT | `/api/v1/admin/services/order` | 200 | 200 | ok | all ids |
| PUT | `/api/v1/admin/services/order` | 422 | 422 | ok | missing id |
| GET | `/api/v1/admin/services/not-a-uuid` | 400 | 400 | ok | bad id |
| GET | `/api/v1/admin/case-studies?q=TUMBUH` | 200 | 200 | ok |  |
| POST | `/api/v1/admin/case-studies` | 409 | 409 | ok | 3rd featured published |
| POST | `/api/v1/admin/case-studies` | 201 | 201 | ok | featured draft allowed |
| GET | `/api/v1/admin/case-studies/80740f9a-8e5a-4b2a-b676-5a647f01b7e1` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/case-studies/80740f9a-8e5a-4b2a-b676-5a647f01b7e1` | 409 | 409 | ok | publishing it → 409 featured |
| PUT | `/api/v1/admin/case-studies/80740f9a-8e5a-4b2a-b676-5a647f01b7e1` | 200 | 200 | ok | unfeatured publish |
| POST | `/api/v1/admin/articles` | 201 | 201 | ok | scheduled; sanitised |
| GET | `/api/v1/articles/artikel-smoke-dan-uji` | 404 | 404 | ok | scheduled hidden publicly |
| GET | `/api/v1/admin/articles?status=scheduled` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/articles/0e4e1506-8a9a-4de7-a258-d0c49d72bdfe` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/articles/0e4e1506-8a9a-4de7-a258-d0c49d72bdfe` | 200 | 200 | ok | publish now (publishedAt null) |
| GET | `/api/v1/articles/artikel-smoke-dan-uji` | 200 | 200 | ok | now visible |
| POST | `/api/v1/admin/articles` | 409 | 409 | ok | explicit slug taken |
| POST | `/api/v1/admin/articles` | 413 | 413 | ok | body > 1 MiB |
| POST | `/api/v1/admin/promos` | 201 | 201 | ok | active, draft service |
| POST | `/api/v1/admin/promos` | 201 | 201 | ok | ended 3 days ago |
| POST | `/api/v1/admin/promos` | 201 | 201 | ok | ended 10 days ago |
| GET | `/api/v1/promos` | 200 | 200 | ok | active+ended≤7d; service null (draft) |
| GET | `/api/v1/promos?status=ended` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/promos?pageSize=50` | 200 | 200 | ok | admin sees all |
| GET | `/api/v1/admin/promos/74e99e19-9852-4e7c-a34f-fd9b29ea2cec` | 200 | 200 | ok | service shown in admin |
| PUT | `/api/v1/admin/promos/74e99e19-9852-4e7c-a34f-fd9b29ea2cec` | 200 | 200 | ok |  |
| POST | `/api/v1/admin/promos` | 422 | 422 | ok | endsAt <= startsAt |
| POST | `/api/v1/admin/testimonials` | 201 | 201 | ok |  |
| POST | `/api/v1/admin/testimonials` | 201 | 201 | ok | inactive |
| GET | `/api/v1/admin/testimonials` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/testimonials/adec9ba3-d67d-45b6-8f9a-c93daa5d0943` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/testimonials/0d43c75d-f901-4cf1-b2c0-4e924e84c257` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/testimonials/order` | 200 | 200 | ok |  |
| GET | `/api/v1/testimonials` | 200 | 200 | ok | only active |
| POST | `/api/v1/admin/media` | 201 | 201 | ok | png 120x40 |
| GET | `/uploads/2d0bf1c654bb4265c7aa7bde0583cda4.png` | 200 | 200 | ok | served, nosniff |
| POST | `/api/v1/admin/media` | 415 | 415 | ok | svg rejected |
| POST | `/api/v1/admin/media` | 413 | 413 | ok | > 2 MiB |
| POST | `/api/v1/admin/client-logos` | 201 | 201 | ok |  |
| POST | `/api/v1/admin/client-logos` | 201 | 201 | ok |  |
| POST | `/api/v1/admin/client-logos` | 422 | 422 | ok | javascript: URL |
| GET | `/api/v1/admin/client-logos` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/client-logos/61d4c724-e752-45f0-8817-de83afcf011a` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/client-logos/61d4c724-e752-45f0-8817-de83afcf011a` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/client-logos/order` | 200 | 200 | ok |  |
| GET | `/api/v1/client-logos` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/leads?status=new&q=Sari` | 200 | 200 | ok |  |
| GET | `/api/v1/admin/leads/49065adf-cb76-4dd8-a232-b380ad66eec2` | 200 | 200 | ok | whatsapp +6281234567890 |
| PATCH | `/api/v1/admin/leads/49065adf-cb76-4dd8-a232-b380ad66eec2` | 200 | 200 | ok | status + notes |
| PATCH | `/api/v1/admin/leads/49065adf-cb76-4dd8-a232-b380ad66eec2` | 422 | 422 | ok | {} → 422 |
| GET | `/api/v1/admin/leads/export` | 200 | 200 | ok | CSV with BOM |
| GET | `/api/v1/admin/users` | 200 | 200 | ok |  |
| POST | `/api/v1/admin/users` | 201 | 201 | ok | editor |
| POST | `/api/v1/admin/users` | 409 | 409 | ok | email taken (case-insensitive) |
| GET | `/api/v1/admin/users/509ffbd1-7b2f-47ff-9ba9-077f802279e7` | 200 | 200 | ok |  |
| PUT | `/api/v1/admin/users/509ffbd1-7b2f-47ff-9ba9-077f802279e7` | 200 | 200 | ok |  |
| POST | `/api/v1/admin/users/509ffbd1-7b2f-47ff-9ba9-077f802279e7/password` | 204 | 204 | ok |  |
| PUT | `/api/v1/admin/users/bb551749-354c-4ab1-ae32-fa043db55a3c` | 409 | 409 | ok | self role change |
| DELETE | `/api/v1/admin/users/bb551749-354c-4ab1-ae32-fa043db55a3c` | 409 | 409 | ok | self delete |
| POST | `/api/v1/auth/login` | 200 | 200 | ok | editor login |
| GET | `/api/v1/admin/settings` | 200 | 200 | ok | editor may read |
| PUT | `/api/v1/admin/settings` | 403 | 403 | ok | editor forbidden |
| PATCH | `/api/v1/admin/settings/toggles` | 200 | 200 | ok | editor allowed |
| GET | `/api/v1/admin/users` | 403 | 403 | ok | editor forbidden |
| GET | `/api/v1/admin/leads` | 200 | 200 | ok | editor may read |
| PATCH | `/api/v1/admin/leads/49065adf-cb76-4dd8-a232-b380ad66eec2` | 403 | 403 | ok | editor forbidden |
| GET | `/api/v1/admin/leads/export` | 403 | 403 | ok | editor forbidden |
| GET | `/api/v1/admin/dashboard` | 200 | 200 | ok | editor allowed |
| POST | `/api/v1/auth/logout` | 204 | 204 | ok | editor logout |
| DELETE | `/api/v1/admin/users/509ffbd1-7b2f-47ff-9ba9-077f802279e7` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/client-logos/61d4c724-e752-45f0-8817-de83afcf011a` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/client-logos/0b42c3d1-e50d-47fa-a1f4-b3e37e243c87` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/testimonials/adec9ba3-d67d-45b6-8f9a-c93daa5d0943` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/testimonials/0d43c75d-f901-4cf1-b2c0-4e924e84c257` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/promos/74e99e19-9852-4e7c-a34f-fd9b29ea2cec` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/promos/26ad78ce-40d5-4b52-9d04-5bd29c71e32f` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/promos/0ee9e510-dea2-4d93-872c-f89af12a6347` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/articles/0e4e1506-8a9a-4de7-a258-d0c49d72bdfe` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/case-studies/80740f9a-8e5a-4b2a-b676-5a647f01b7e1` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/services/a8555733-effe-4f87-9982-cf83fe0c82a4` | 204 | 204 | ok |  |
| DELETE | `/api/v1/admin/services/a8555733-effe-4f87-9982-cf83fe0c82a4` | 404 | 404 | ok | already gone |
| POST | `/api/v1/auth/logout` | 204 | 204 | ok | admin logout |
| GET | `/api/v1/auth/me` | 401 | 401 | ok | after logout |

**Total: 126 passed, 0 failed.**

## Behaviour checks

- **E.164.** The lead sent with `0812-3456-7890` is stored and returned as `+6281234567890`. Settings `0822-3010-9923` becomes `+6282230109923`.
- **Promos.** `GET /promos` returned `[{"title":"Aktif","status":"active","service":null},{"title":"Lalu","status":"ended","service":null}]`:
  - The promo that ended 10 days ago is hidden.
  - The promo that ended 3 days ago is shown as `ended`.
  - The active promo linked to a **draft** service has `service: null` publicly. The admin read shows the service.
- **Rate limit.** The honeypot request counts toward the limit, so the 6th lead from the same client IP within a minute gets 429 with `Retry-After`.
- **CSV export.** The file starts with the UTF-8 BOM (`357 273 277`), then the header `createdAt,name,company,email,whatsapp,service,startTimeframe,status,message,notes`, with CRLF line endings.
- **Uploads.** `GET /uploads/<name>` returns `Content-Type: image/png`, `X-Content-Type-Options: nosniff` and `Cache-Control: public, max-age=31536000, immutable`.
- **Revalidation.** The fake FE received 33 calls, all with the correct secret. Tags per mutation:

  | Mutation | Calls | Tags |
  |---|---|---|
  | settings PUT / toggles PATCH | 4 | `site` |
  | services create / update / reorder / delete | 4 | `services`, `promos` |
  | case studies | 3 | `case-studies` |
  | articles | 3 | `articles` |
  | promos | 7 | `promos` |
  | testimonials | 6 | `testimonials` |
  | client logos | 6 | `client-logos` |

  Failed mutations (4xx) fire nothing. Lead, user and media mutations fire nothing, as the contract specifies.
- **Client IP.** With `LOG_LEVEL=debug` the IP line reads `"xff":["203.0.113.1"],"remote_addr":"127.0.0.1:…","client_ip":"203.0.113.1"` (`TRUSTED_PROXY_HOPS=1`).

## Re-run for contract v1.1 (FAQ items)

Date: 2026-10-15. Branch `feat/seo-geo`. Fresh database, same setup without the fake FE. `scripts/smoke.sh` now
also asserts response bodies with `jq` (rows with method `jq`): the seeded services carry FAQs, `faqs` defaults to
`[]` on services and articles when omitted, FAQ text is trimmed and HTML-stripped, and more than 10 items or an
HTML-only question returns 422 on `faqs` / `faqs.1.question`. Result: `PASS=136 FAIL=0`.

Upgrade path: a database migrated and seeded by the v1.0 binary, then started with the v1.1 binary, logs
`seed: service faqs backfilled count=5`; the next restart logs no backfill.
