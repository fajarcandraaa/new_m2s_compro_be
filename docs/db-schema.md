# Database schema (PostgreSQL 16)

Companion to `api/openapi.yaml`. Column names are snake_case; the API maps them to camelCase.

## Conventions

- Primary keys: `id uuid PRIMARY KEY DEFAULT gen_random_uuid()` (built into PG 13+, no extension needed). Exception: `settings` (singleton, `id smallint = 1`) and `article_tags` (composite PK).
- Timestamps: `timestamptz NOT NULL DEFAULT now()`. `updated_at` is set by the application in every UPDATE (no triggers, keeps migrations simple).
- Enums: `text` + `CHECK (... IN (...))` instead of `CREATE TYPE`, so adding a value later is a one-line migration.
- Slugs: `text NOT NULL UNIQUE` with `CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$')`.
- Text length limits are enforced in Go validation (they mirror `maxLength` in the OpenAPI spec). The DB only has `CHECK`s that protect integrity: enums, slug format, non-empty required names, date order.
- Nullable in the DB = nullable in the API. Optional strings that the seed has as `""` (maps/linkedin/instagram URL) are stored as `NULL`.
- Visibility rules live in SQL `WHERE` clauses, not views:
  - public content: `status = 'published'`
  - public articles additionally need `published_at <= now()`
  - public promos: `ends_at > now() - interval '7 days'`

## Lists: `jsonb` vs child tables

| Data | Storage | Why |
|---|---|---|
| `services.points`, `services.systems` (string lists) | `jsonb` array of strings | Always read and written as a whole with the parent, never queried on their own, and order matters. A child table would only add joins and diff logic. |
| `services.deliverables` (`[{title, body}]`) | `jsonb` array of objects | Same reasons. |
| `case_studies.metrics` (exactly 2 `{value,label}`), `scope`, `initial_conditions`, `risks`, `approach`, `solutions`, `impacts` | `jsonb` arrays | The CMS edits each list as one textarea (one point per line), so it is replaced as a whole on every save. |
| `service_packages` | **child table** | Packages are named entities with their own price, highlight flag, sort order and feature list. They are likely to be referenced later (promo per package, per-package CTA). On a service PUT they are replaced as a whole in the same transaction (`DELETE ... WHERE service_id = $1` then INSERT). `features` inside a package is `jsonb`. |
| Article tags | **join table** `article_tags` | We filter by tag (`?tag=`), count per tag and find related articles. All of that needs relational queries. |

Every `jsonb` column has `NOT NULL DEFAULT '[]'::jsonb` and `CHECK (jsonb_typeof(col) = 'array')`. Element shape is validated in Go before writing.

## Tables

### `users`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK, default `gen_random_uuid()` |
| name | text | NOT NULL, `CHECK (name <> '')` |
| email | text | NOT NULL; unique index on `lower(email)` |
| password_hash | text | NOT NULL (bcrypt, cost 12) |
| role | text | NOT NULL, `CHECK (role IN ('admin','editor'))` |
| is_active | boolean | NOT NULL DEFAULT true |
| session_version | integer | NOT NULL DEFAULT 1. Copied into the JWT as `sv`. Incremented on password reset, role change and deactivation, so existing tokens stop working. |
| last_login_at | timestamptz | NULL |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Indexes: `users_email_lower_uq UNIQUE (lower(email))`.

### `settings` (singleton)
| Column | Type | Constraints |
|---|---|---|
| id | smallint | PK, `CHECK (id = 1)` |
| company_name | text | NOT NULL |
| legal_name | text | NOT NULL |
| tagline | text | NOT NULL DEFAULT '' |
| email | text | NOT NULL |
| whatsapp_number | text | NOT NULL (E.164) |
| whatsapp_display | text | NOT NULL DEFAULT '' |
| whatsapp_greeting | text | NOT NULL DEFAULT '' |
| address | text | NOT NULL DEFAULT '' |
| maps_url, linkedin_url, instagram_url | text | NULL |
| show_testimonials | boolean | NOT NULL DEFAULT true |
| show_client_logos | boolean | NOT NULL DEFAULT false |
| updated_at | timestamptz | NOT NULL DEFAULT now() |

The row is created by the seed. Updates use `UPDATE settings ... WHERE id = 1`.

### `services`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| slug | text | NOT NULL UNIQUE, slug CHECK |
| audience | text | NOT NULL, `CHECK (audience IN ('business','personal'))` |
| status | text | NOT NULL DEFAULT 'draft', `CHECK (status IN ('draft','published'))` |
| sort_order | integer | NOT NULL DEFAULT 0 |
| eyebrow | text | NOT NULL DEFAULT '' |
| title | text | NOT NULL, `CHECK (title <> '')` |
| tagline, summary, price_label, price_value, audience_text | text | NOT NULL DEFAULT '' |
| points | jsonb | NOT NULL DEFAULT '[]', array CHECK |
| deliverables | jsonb | NOT NULL DEFAULT '[]', array CHECK |
| systems | jsonb | NOT NULL DEFAULT '[]', array CHECK |
| seo_title, seo_description, og_image_url | text | NULL |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Indexes: `services_status_sort_idx (status, sort_order)`.
Reorder (`PUT /admin/services/order`) runs one transaction: `UPDATE services SET sort_order = x.ord FROM unnest($1::uuid[]) WITH ORDINALITY AS x(id, ord) WHERE services.id = x.id`. The handler first checks that the id set matches the table exactly; otherwise it returns 422. `sort_order` is deliberately not UNIQUE, so reordering needs no deferred constraints.

### `service_packages`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| service_id | uuid | NOT NULL, FK → `services(id)` **ON DELETE CASCADE** |
| name | text | NOT NULL, `CHECK (name <> '')` |
| price_label | text | NOT NULL DEFAULT '' |
| price_value | text | NOT NULL |
| description | text | NOT NULL DEFAULT '' |
| features | jsonb | NOT NULL DEFAULT '[]', array CHECK |
| highlighted | boolean | NOT NULL DEFAULT false |
| sort_order | integer | NOT NULL DEFAULT 0 |

Indexes: `service_packages_service_idx (service_id, sort_order)`.

### `case_studies`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| slug | text | NOT NULL UNIQUE, slug CHECK |
| status | text | NOT NULL DEFAULT 'draft', CHECK draft/published |
| system_name | text | NOT NULL |
| category | text | NOT NULL |
| featured | boolean | NOT NULL DEFAULT false |
| sort_order | integer | NOT NULL DEFAULT 0 |
| title | text | NOT NULL, `CHECK (title <> '')` |
| summary, card_body | text | NOT NULL DEFAULT '' |
| metrics | jsonb | NOT NULL DEFAULT '[]', `CHECK (jsonb_typeof(metrics)='array')` (exactly 2 items is enforced in Go) |
| scope, initial_conditions, risks, approach, solutions, impacts | jsonb | NOT NULL DEFAULT '[]', array CHECK |
| initial_conditions_title, risks_title, approach_title, impact_title, lesson | text | NOT NULL DEFAULT '' |
| seo_title, seo_description, og_image_url | text | NULL |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Indexes: `case_studies_status_sort_idx (status, sort_order)`.
"At most 2 published case studies are featured" is checked in the service layer inside the write transaction (`SELECT count(*) ... FOR UPDATE`) and returns 409. It is not a DB constraint, because an exact-count rule cannot be expressed as one.

### `tags`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| slug | text | NOT NULL UNIQUE, slug CHECK (derived from name: lowercase, `&` dropped, non-alphanumerics → `-`) |
| name | text | NOT NULL; unique index on `lower(name)` |
| created_at | timestamptz | NOT NULL DEFAULT now() |

### `articles`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| slug | text | NOT NULL UNIQUE, slug CHECK |
| status | text | NOT NULL DEFAULT 'draft', CHECK draft/published |
| title | text | NOT NULL, `CHECK (title <> '')` |
| excerpt | text | NOT NULL DEFAULT '' |
| content_html | text | NOT NULL DEFAULT '' (sanitised with bluemonday UGC before INSERT/UPDATE) |
| cover_image_url | text | NULL |
| author | text | NOT NULL DEFAULT 'Tim Mind2Screen' |
| read_minutes | integer | NOT NULL DEFAULT 1, `CHECK (read_minutes >= 1)`. Computed on save from the word count of the stripped HTML, 200 wpm, rounded up. Seed values are kept as given. |
| published_at | timestamptz | NULL. `CHECK (status = 'draft' OR published_at IS NOT NULL)`. On publish the app sets `now()` if it is null. |
| seo_title, seo_description, og_image_url | text | NULL |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Indexes:
- `articles_public_idx (published_at DESC) WHERE status = 'published'` for the public list
- `articles_updated_idx (updated_at DESC)` for the CMS table

`is_scheduled` is computed in SQL as `status = 'published' AND published_at > now()`.

### `article_tags`
| Column | Type | Constraints |
|---|---|---|
| article_id | uuid | FK → `articles(id)` **ON DELETE CASCADE** |
| tag_id | uuid | FK → `tags(id)` **ON DELETE CASCADE** |

PK `(article_id, tag_id)`. Index `article_tags_tag_idx (tag_id, article_id)`. Orphan tags are kept on purpose: the tag picker keeps suggesting them, and they show `articleCount = 0`.

Related articles query, used by `GET /articles/{slug}/related`:
`SELECT a.*, count(*) shared FROM article_tags t1 JOIN article_tags t2 ON t2.tag_id = t1.tag_id AND t2.article_id <> t1.article_id JOIN articles a ON a.id = t2.article_id WHERE t1.article_id = $1 AND <visible> GROUP BY a.id ORDER BY shared DESC, a.published_at DESC LIMIT $2`.

### `promos`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| service_id | uuid | NULL, FK → `services(id)` **ON DELETE SET NULL** |
| title | text | NOT NULL, `CHECK (title <> '')` |
| description | text | NOT NULL DEFAULT '' |
| code | text | NULL |
| starts_at | timestamptz | NOT NULL |
| ends_at | timestamptz | NOT NULL, `CHECK (ends_at > starts_at)` |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Indexes: `promos_ends_idx (ends_at)`, `promos_starts_idx (starts_at DESC)`.
There is no status column. Status is computed on every read (`CASE WHEN now() < starts_at THEN 'upcoming' WHEN now() < ends_at THEN 'active' ELSE 'ended' END`), so it can never go stale.

### `testimonials`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| client_name | text | NOT NULL, `CHECK (client_name <> '')` |
| client_role | text | NOT NULL DEFAULT '' |
| company | text | NULL |
| quote | text | NOT NULL, `CHECK (quote <> '')` |
| is_active | boolean | NOT NULL DEFAULT true |
| sort_order | integer | NOT NULL DEFAULT 0 |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Index: `testimonials_sort_idx (sort_order)`.

### `client_logos`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| name | text | NOT NULL, `CHECK (name <> '')` |
| image_url | text | NOT NULL |
| sort_order | integer | NOT NULL DEFAULT 0 |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Index: `client_logos_sort_idx (sort_order)`. Deleting a logo row does not delete the file. Orphaned uploads are acceptable for v1.

### `leads`
| Column | Type | Constraints |
|---|---|---|
| id | uuid | PK |
| name | text | NOT NULL |
| company | text | NOT NULL |
| email | text | NOT NULL |
| whatsapp | text | NOT NULL |
| service_slug | text | NULL. A snapshot of the slug at submit time, with **no FK**, so a lead survives service deletion or renaming. The API joins `services` on slug for `serviceTitle` (null if gone). |
| start_timeframe | text | NULL |
| message | text | NOT NULL |
| status | text | NOT NULL DEFAULT 'new', `CHECK (status IN ('new','contacted','done'))` |
| notes | text | NULL |
| ip_hash | text | NULL. sha256(IP + JWT_SECRET), for abuse investigation only. Never exposed in the API or the CSV. |
| user_agent | text | NULL (truncated to 300 characters, not exposed) |
| created_at, updated_at | timestamptz | NOT NULL DEFAULT now() |

Indexes: `leads_created_idx (created_at DESC)`, `leads_status_created_idx (status, created_at DESC)`.

### Not stored
- **Sessions**: the JWT is stateless. Revocation uses `users.session_version` and `is_active`, which the auth middleware checks with one PK lookup per admin request.
- **Media**: there is no media table. Files live in `UPLOAD_DIR`, and entities store the URL.
- **Rate-limit counters**: kept in memory per process, which is enough for a single BE instance.

## Search (`q`)
`ILIKE '%' || $q || '%'` on the columns listed per endpoint in the OpenAPI spec. Escape `%`, `_` and `\` in `q`. Data volumes are tiny, so there are no trigram indexes.

## Goose migration plan

`migrations/` is embedded with `//go:embed migrations/*.sql` and run by `goose.Up` on start when `MIGRATE_ON_START=true`. Each file has `-- +goose Up` / `-- +goose Down`. Files are numbered sequentially (`goose -s`), not timestamped, so branch ordering stays predictable.

| # | File | Contents |
|---|---|---|
| 1 | `00001_users.sql` | `users` + unique `lower(email)` index |
| 2 | `00002_settings.sql` | `settings` singleton |
| 3 | `00003_services.sql` | `services`, `service_packages` + indexes |
| 4 | `00004_case_studies.sql` | `case_studies` + index |
| 5 | `00005_articles.sql` | `tags`, `articles`, `article_tags` + indexes |
| 6 | `00006_promos.sql` | `promos` (FK to services) + indexes |
| 7 | `00007_testimonials_client_logos.sql` | `testimonials`, `client_logos` + indexes |
| 8 | `00008_leads.sql` | `leads` + indexes |

Down migrations drop the tables in reverse order. Seed data is **not** a migration. It is Go code (`internal/seed`) that runs after migrations, in one transaction:
- admin user: only if `users` is empty
- settings: only if no row exists
- services + packages, case studies, tags + articles: only if the respective table is empty

All seeded content gets status `published`.
