# Contract review: Frontend (Gate 1)

- Reviewer: Senior Frontend Engineer (Next.js 15 App Router, TypeScript, openapi-typescript)
- Reviewed: `api/openapi.yaml` @ cb3a436, `docs/architecture.md`, `docs/db-schema.md`, FE `docs/brief.md` §4–6, seed content (`seed.json`, `articles.json`)
- Figma checked (file `is20WupHCNceNrBx0pfS29`): Promo `51:16280`, Detail Studi Kasus `47:10875`, Detail Artikel `49:13040`, Diskusi `51:17390`, CMS page `59:1504` (all frames listed in metadata; screenshots of Dashboard, Studi Kasus / Editor, Testimoni)
- Method: I went through every public route in brief §5 and every CMS screen in §6, listed the fields each one renders or submits, and traced each field to an operation and schema. I also checked how the generated TS types will look under openapi-typescript 7 (3.1 `type: [x, "null"]` becomes `x | null`, and `oneOf [Ref, null]` becomes `Ref | null`).

## Overall verdict

The contract is in good shape, and I could build every page today with only small gaps. Every field shown in the Figma frames I checked has a source. The homepage needs 6 independent, parallel server-side calls and has no N+1. List filters and search match the CMS screens. Nullable fields are explicit and never omitted, and the error envelope works for inline form errors. The items below are mostly about **semantics that are not written down yet**. If those stay unwritten, BE and FE will each guess and the guesses will diverge.

**I agree to freeze after the Blocking items are resolved.** The Blocking items only change descriptions and examples. None adds an endpoint. I would like the Should-change items folded into the same edit because they are cheap, but none of them stops me from starting.

---

## Blocking (must change before freeze)

### B1. Key format of `error.fields` for nested and array inputs
- **Location:** `components/schemas/Error` → `error.fields` (and `responses/ValidationError`); architecture §10 "Validation".
- **Problem:** Keys are described only as "camelCase input field name". The two largest CMS forms are made of nested objects and arrays: `ServiceInput.packages[].{name,priceValue,features[]}`, `deliverables[].{title,body}`, `seo.{title,description,ogImageUrl}`, `CaseStudyInput.metrics[].{value,label}`, plus the one-item-per-line lists `initialConditions[]`, `risks[]` and so on. Without an agreed key format, BE might emit `packages[0].name`, `packages.0.name` or just `packages`, and FE cannot place errors inline (react-hook-form `setError(path)`).
- **Proposed change:** Document the format in the `Error.fields` description and add a nested example to `responses/ValidationError`:
  - The key is a **dot path with numeric indices**, matching react-hook-form paths exactly: `seo.title`, `packages.0.priceValue`, `packages.1.features.2`, `deliverables.0.title`, `metrics.1.label`, `initialConditions.3`.
  - An error about the array as a whole (count, duplicates, "at most one highlighted package") goes on the array key: `metrics`, `tags`, `packages`.
  - Errors that cannot be tied to a field go only in `message`.
  - Example:
    ```yaml
    fields:
      title: Wajib diisi.
      seo.description: Maksimal 160 karakter.
      packages.0.priceValue: Wajib diisi.
      metrics.1.label: Wajib diisi.
    ```

### B2. Slug semantics: auto-generation, collisions, and update without `slug`
- **Location:** `ServiceInput.slug`, `CaseStudyInput.slug`, `ArticleInput.slug` (and the descriptions of the `POST`/`PUT` operations that use them); `responses/Conflict`.
- **Problem:** The spec says only "`slug` optional (generated from title)". The article editor in brief §6 shows the slug as "auto dari judul, bisa diedit", so the FE previews the slug while the user types. Three things are undefined:
  1. The `PUT` input schema is also "full update", so it is unclear what happens when `slug` is omitted. If the BE regenerates it from the title, every title edit silently changes a public URL.
  2. A generated slug can collide with an existing one, and the spec does not say whether the BE returns 409 or adds a suffix.
  3. The slugify rule is not published, so the FE preview and the BE result can differ (for example, `AI & Teknologi` could become `ai-teknologi` or `ai--teknologi`).
- **Proposed change:** Write these rules into the descriptions of the three input schemas:
  - **Create, `slug` omitted:** the BE uses `slugify(title)`. On a collision it appends `-2`, `-3` and so on, and never returns 409 for a slug it generated itself.
  - **Create or update, `slug` sent:** the BE validates the pattern. If another row already uses it, the BE returns 409 `slug_taken` with `fields.slug`, which already matches the Conflict example.
  - **Update, `slug` omitted:** the BE **keeps the current slug**. It never regenerates.
  - **Slugify rule (one sentence, shared by `domain.Slugify` and the FE helper):**
    1. Lowercase the text.
    2. Apply NFKD and drop combining marks.
    3. Replace every run of characters outside `[a-z0-9]` with `-`.
    4. Trim `-` from both ends.
    5. Cut to 120 characters, then trim a trailing `-`.

### B3. Time semantics for promo periods and date-only CMS inputs
- **Location:** `PromoInput.startsAt` / `endsAt`, `Promo.endsAt`, `PromoStatus` description; info "Conventions" (times).
- **Problem:** The Promo frame shows "1–31 Oktober 2026", and the CMS promo panel has two date inputs (Mulai / Berakhir). The contract defines `active` as `startsAt <= now < endsAt`, which makes `endsAt` **exclusive**, but it does not say so. It also does not say:
  - which timezone a date-only value refers to;
  - whether input may carry an offset, given that output is "UTC".

  Without this, the FE will either show "1 Okt – 1 Nov" or end promos a day early, and the 7-day dimming window will be off by the same amount.
- **Proposed change:** Document the following:
  - `endsAt` is an **exclusive** instant.
  - Inputs accept any RFC3339 offset. Go's `time.RFC3339` parse already does; the BE stores and returns UTC.
  - Convention for date-only CMS inputs, which the BE does not need to know about but which the spec should state so tests and seed data follow it:
    - Mulai `D` → `D 00:00 +07:00`
    - Berakhir `D` → `(D + 1 day) 00:00 +07:00`
    - The public site shows `endsAt − 1 s` in `Asia/Jakarta`.
  - Add one sentence to info "Conventions": "Business dates are interpreted in Asia/Jakarta (WIB); the FE formats all dates in WIB." This also fixes article dates near midnight: seed `published_at` is `08:00+07:00`, which is `01:00Z`, so the date is safe there, but a CMS value like `00:30` WIB would otherwise render as the previous day if it were formatted in UTC.

---

## Should change

### S1. Model the CSRF header as a security scheme, not as a required parameter
- **Location:** `components/parameters/XRequestedWith`, referenced by every mutating `/admin/*` and `/auth/*` operation.
- **Problem:** openapi-typescript turns a required header parameter into `parameters.header: { "X-Requested-With": "m2s-cms" }` on about 35 operations. With openapi-fetch, every call site must then pass `params: { header: {...} }`, even though the header is set once in client middleware. The type demands it whether or not middleware already sends it.
- **Proposed change:**
  - Add `components/securitySchemes/csrfHeader: { type: apiKey, in: header, name: X-Requested-With, description: "Must equal m2s-cms" }`.
  - Change mutating admin operations to `security: [{ sessionCookie: [], csrfHeader: [] }]`, and login/logout to `security: [{ csrfHeader: [] }]`.
  - Drop the parameter refs. kin-openapi request validation then needs an `AuthenticationFunc` that checks the value, which is a few lines in the BE test helper.
  - Behaviour does not change (403 `csrf_failed`).

### S2. Specify the normalised format of `Lead.whatsapp`
- **Location:** `LeadCreateInput.whatsapp` ("BE normalises") and `Lead.whatsapp`.
- **Problem:** The Leads detail panel has a "Hubungi via WhatsApp" button, which needs `https://wa.me/<digits>`. The Diskusi form placeholder is `08xx xxxx xxxx`. The FE cannot build the link reliably without knowing what the BE stores.
- **Proposed change:** In the `Lead.whatsapp` description and pattern, state that it is stored as **E.164**:
  - Strip spaces, `(`, `)` and `-`.
  - Replace a leading `0` with `+62`, and a leading `62` with `+62`.
  - Add `pattern: ^\+[1-9][0-9]{7,14}$` on the response field.
  - If a number cannot be normalised to that form, return 422 on `whatsapp`.

  The CSV export uses the same value.

### S3. 409 `conflict` responses must carry `fields` when a field is responsible
- **Location:** `responses/Conflict`, `Error.fields` description ("validation_error / slug_taken only"), `CaseStudyInput` (featured limit), `POST/PUT /admin/users` (email taken, self role change or deactivation).
- **Problem:** The Studi Kasus editor shows the "Tampilkan di Beranda" toggle with the help text "Maks. 2 studi kasus unggulan". The Pengguna form has email, role and active fields. A bare toast is acceptable, but an inline error on the toggle or field is what the design implies. Right now the FE would have to match on `message` text to place it.
- **Proposed change:** Widen the `fields` description to "validation_error, slug_taken, conflict". Then require:
  - featured limit → `fields.featured`
  - email taken → `fields.email`
  - self role change → `fields.role`
  - self deactivation → `fields.isActive`
  - self delete → no field (toast)

### S4. Define the order of `tags` on articles
- **Location:** `ArticleSummary.tags`, `Article.tags`, `ArticleAdmin*.tags`; `ArticleInput.tags`; db-schema `article_tags` (no position column).
- **Problem:** The Detail Artikel hero eyebrow ("ARTIKEL · AI & TEKNOLOGI") and the related-article cards ("KEAMANAN DATA") show **one** tag. The FE will use `tags[0]`, but there is no defined order: `article_tags` has no position, so order depends on the query plan.
- **Proposed change:** "`tags` are returned in the order given in `ArticleInput.tags`; the first is the primary tag shown as the eyebrow." Add `position smallint` to `article_tags`. If that is too much for today, use the fallback "ordered by tag name", which is stable even though it is not editor-controlled.

### S5. Input schemas should not require server-managed `sortOrder`
- **Location:** `ServicePackage.sortOrder` (the schema is reused inside `ServiceInput.packages`), `CaseStudyInput.sortOrder`.
- **Problem:**
  - `ServicePackage.sortOrder` is required with `minimum: 1`, yet its own description says "array order on input is authoritative and overwrites this". The FE has to send a value the BE ignores.
  - `CaseStudyInput.sortOrder` is required, but the Studi Kasus screen has no ordering UI (brief §6 lists only "tabel"), so the FE would invent a number on create.
- **Proposed change:**
  - Split out `ServicePackageInput` without `sortOrder`, or make it optional in input.
  - Make `CaseStudyInput.sortOrder` optional: on create default to `max+1`; on update keep the current value when omitted.

### S6. Normalise `""` to `null` for nullable string inputs
- **Location:**
  - `SettingsInput.mapsUrl/linkedinUrl/instagramUrl`
  - `Seo.*`
  - `TestimonialInput.company`
  - `PromoInput.code`
  - `LeadCreateInput.service/startTimeframe`
  - `LeadPatchInput.notes`
  - `ArticleInput.coverImageUrl`
- **Problem:** HTML inputs and react-hook-form produce `""` for an empty field. Under the current schemas, `""` fails `format: uri` and `pattern` checks, so the FE must hand-map about 15 fields to `null` and every one it misses becomes a confusing 422.
- **Proposed change:** "For fields typed `string | null`, the BE trims and treats `""` as `null` before validation." The BE already trims (architecture §10), so this is one more line in each `Validate()`. The FE will still send `null` where it can. This rule is the safety net.

### S7. Remove `maximum: 100` from the response `PageMeta.pageSize`, or define the unpaginated rule differently
- **Location:** `components/schemas/PageMeta.pageSize` ("Unpaginated lists return page=1 and pageSize = max(total,1)").
- **Problem:** An unpaginated list with more than 100 items, for example tags over time, makes the response violate its own schema, and the kin-openapi response tests would fail. The rule also hides the fact that reorder screens need the **whole** list.
- **Proposed change:**
  - Drop `maximum` from `PageMeta.pageSize`; keep it on the `PageSize` query parameter.
  - Add to the admin list descriptions of services, testimonials and client logos: "Reorder screens request `pageSize=100`; `ReorderInput` must contain every id." That is what the FE will do.

---

## Nice to have / accept as-is

### N1. Homepage aggregate endpoint: accept as-is
- **Location:** `/site`, `/services`, `/case-studies?featured=true`, `/testimonials?limit=3`, `/client-logos`, `/promos?status=active`.
- The homepage needs these 6 calls. They run in parallel in one Server Component (`Promise.all`), each with its own cache tag. That is better for granular revalidation than a `/home` aggregate would be, and there is no N+1. Accept.

### N2. Public `updatedAt` for SEO
- **Location:** `Article`, `ArticleSummary`, `CaseStudy`, `CaseStudySummary`, `Service`.
- `sitemap.xml` `<lastmod>`, `article:modified_time` and JSON-LD `dateModified` would use it. Add `updatedAt` (RFC3339) to the public schemas. Without it the FE uses `publishedAt` for articles and leaves `lastmod` empty for the rest.

### N3. Media upload request body typing
- **Location:** `POST /admin/media` requestBody `file` (`type: string, contentMediaType: application/octet-stream`).
- openapi-typescript renders this as `string`, so the FE will hand-write the `FormData` call and use only the generated **response** type (`MediaUpload`). That is fine. Optionally add `format: binary` for documentation tools.
- Also add one line confirming the Next.js rewrite passes the multipart body through unchanged. It does, as long as the upload stays under 2 MiB.

### N4. Quick toggle for testimonial `isActive`: accept as-is
- **Location:** `PUT /admin/testimonials/{id}`.
- The "Tampil" switch in each table row is a full `PUT` with the row data the FE already holds (`TestimonialAdmin` contains every input field). Accept. A `PATCH { isActive }` would save a little code but is not needed.

### N5. Client logo name on upload: accept as-is
- **Location:** `ClientLogoInput.name` (required).
- The design has only an "Unggah Logo" button. The FE will upload, then ask for the company name (pre-filled from the file name) before `POST /admin/client-logos`. The name is also the image `alt` text, so keeping it required is right.
- Optional: add `width`/`height` to `ClientLogo`, copied from `MediaUpload`, so `next/image` can skip `fill`. Not needed; the FE uses a fixed logo box.

### N6. Case-study category: accept free text
- **Location:** `CaseStudyInput.category`.
- Figma shows "Kategori … Pilih", a select. The contract has free text, which is better for a filter driven by data. The FE will render a combobox that suggests the distinct categories of existing case studies. Accept, and no enum.

### N7. Featured article and pagination: accept as-is
- **Location:** `GET /articles` ("item 0 of page 1 is featured").
- Page 1 shows the featured article plus `pageSize−1` grid cards, and later pages show `pageSize` cards. This is consistent and needs no offset tricks. Accept.

### N8. Response envelopes are inline: accept as-is
- Every `{ data, meta }` wrapper is an inline object, so the generated types are reached through `paths[...]["get"]["responses"][200]["content"]["application/json"]`. The FE will add one helper type (`ApiData<Path, Method>`) and otherwise use `components["schemas"][...]`. Accept, and no need to name around 60 wrapper schemas.

### N9. `Promo.service` as `oneOf [ServiceRef, null]`: accept
- openapi-typescript renders it as `ServiceRef | null`, which is exactly what is wanted. The only nit is consistency with the `type: [x, "null"]` style used elsewhere. Accept.

### N10. `LeadCreateInput.company` is required
- The form also serves B2C web-development visitors, who may not have a company. Brief §3 lists `company` without "opsional", so I accept it as specified. Consider making it optional later if leads complain. The FE placeholder stays "Contoh: PT Maju Bersama".

### N11. Sanitiser policy and Tiptap output
- **Location:** `ArticleInput.contentHtml`.
- Please confirm the bluemonday UGC policy keeps everything the Tiptap toolbar produces:
  - relative `img src="/uploads/…"` (UGC `AllowStandardURLs` allows relative URLs; a BE test would lock it in)
  - `h2`/`h3`, `blockquote`, `ul`/`ol`, `a href`
- The FE styles article HTML by element (`.prose`), not by class, so stripping classes is fine.

### N12. Role gating: accept as-is
- `GET /auth/me` returns `User` (`id`, `name`, `email`, `role`, `isActive`). That covers the sidebar footer ("Nama Admin / Admin · Keluar"), hiding the Pengaturan group for editors, hiding lead status/notes editing for editors (PATCH is admin-only), and disabling self-demote/delete in Pengguna (compare `me.id`). `x-roles` on each operation matches brief §3. Accept.

### N13. Dashboard: accept as-is
- `DashboardSummary` covers all three stat tiles and the "Leads terbaru" table: Nama, Perusahaan, Layanan = `serviceTitle`, Masuk = `createdAt` shown relative, Status.
- It also covers "Draft terakhir": Judul, Jenis = `type`, Diubah = `updatedAt`, Status, and an Edit link built from `type` + `id`. Accept.

### N14. ISR cache tags: accept as-is
- The tag map in `info.description` matches how the FE will tag fetches. `services` also firing `promos` is correct. Time-based changes (scheduled articles, promo `upcoming → active → ended`, the 7-day dimming cut-off) rely on the 300 s fallback, which is acceptable for this site.
- The FE will tag the footer and header `GET /site` with `site`, so a settings change refreshes every page.

---

## Counts
- Blocking: 3 (B1–B3)
- Should change: 7 (S1–S7)
- Nice to have / accept as-is: 14 (N1–N14)
