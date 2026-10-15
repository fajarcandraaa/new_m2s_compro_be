# Gate 1: Keputusan PM atas review kontrak

Sumber: `contract-review-frontend.md` (FE) dan `contract-review-backend.md` (BE). Semua butir di bawah **wajib** diterapkan Tech Lead sebelum kontrak dibekukan sebagai v1.0.

## Blocking (semua diterima)
| # | Keputusan |
|---|---|
| FE-B1 | `error.fields` memakai **dot path** dengan indeks numerik: `packages.0.priceValue`, `seo.title`, `metrics.1.label`, `initialConditions.2`. Didokumentasikan di schema Error. |
| FE-B2 / BE-S2 | **Slug:** (a) Saat create, jika `slug` kosong atau tidak dikirim, BE meng-generate dari judul. (b) Saat PUT, jika `slug` kosong atau tidak dikirim, slug lama **dipertahankan**. (c) Slug hasil generate yang bentrok diberi akhiran `-2`, `-3`, dan seterusnya, tidak pernah 409. (d) Slug yang dikirim eksplisit dan bentrok → 409 dengan `fields.slug`. (e) **Aturan slugify:** lowercase → normalisasi NFKD dan buang diakritik → `&` jadi `dan` → karakter selain `[a-z0-9]` jadi `-` → strip berulang disatukan → trim `-` → maksimal 120 karakter. Rumus ini ditulis di spec supaya FE bisa meniru pratinjaunya. |
| FE-B3 | **Tanggal promo:** `endsAt` **eksklusif**. Input menerima RFC3339 dengan offset apa pun, output UTC. Konvensi CMS: tanggal "Berakhir D" dikirim sebagai D+1 00:00 `+07:00`, lalu ditampilkan sebagai `endsAt − 1 detik` di zona Asia/Jakarta. |
| BE-B1 | Tetap Go 1.24. Pin `kin-openapi v0.135.0` untuk tes; lewati `doc.Validate()` dan validasi respons per operasi saja. Perbarui architecture.md. |
| BE-B2 | **Disetujui:** editor boleh `GET /admin/settings` dan `PATCH /admin/settings/toggles`. **Diubah:** `GET /admin/leads/export` menjadi **admin-only** karena berisi data pribadi massal. `PATCH /admin/leads/{id}` tetap admin-only, sesuai brief. |
| BE-B3 | **Semantik penulisan:** POST/PUT berarti objek penuh (field yang tidak dikirim dianggap nilai kosong, dan FE selalu mengirim objek lengkap). PATCH berarti parsial (field yang tidak dikirim = tidak berubah, `null` = kosongkan untuk field nullable, `{}` → 422). Ditulis di `info.description` dan deskripsi operasi terkait. |
| BE-B4 | Form `/diskusi` **wajib** dikirim dari browser lewat rewrite `/api` (bukan Server Action atau Route Handler), dan ditulis di deskripsi `POST /leads`. Default `TRUSTED_PROXY_HOPS=1`. Nilai untuk Dokploy diverifikasi saat deploy pertama (log debug IP), dan ini dicatat di architecture.md. |

## Should change (diterima)
- **FE-S1:** header CSRF `X-Requested-With: m2s-cms` dijadikan security scheme `apiKey` (in: header), bukan parameter. Berlaku di semua operasi admin yang mengubah data **dan** `/auth/login`, `/auth/logout` (BE-S11).
- **FE-S2 / BE-S6:** WhatsApp dinormalisasi ke **E.164** (`08…` → `+628…`, `628…` → `+628…`). Input yang tidak valid → 422. Berlaku untuk leads dan settings.
- **FE-S3:** respons 409 menyertakan `fields` (mis. `featured`, `email`, `role`, `isActive`, `slug`).
- **FE-S4:** tag artikel punya urutan tetap (sesuai urutan input, disimpan dengan kolom `position`). Tag pertama dipakai sebagai label kategori.
- **FE-S5:** `sortOrder` tidak wajib di input jika diatur lewat endpoint reorder. Item baru ditaruh di urutan terakhir.
- **FE-S6:** string kosong pada field nullable disimpan sebagai `null`.
- **FE-S7:** batas maksimal `PageMeta.pageSize` di **respons** dihapus (batas 100 hanya berlaku untuk query param).
- **BE-S1:** batas "maks. 2 studi kasus featured" memakai `pg_advisory_xact_lock`, lalu count. Draft featured boleh; yang dicek saat publish atau saat set featured pada yang sudah terbit. Perbarui db-schema.md.
- **BE-S3:** promo publik yang layanannya draft → `service: null`.
- **BE-S4:** semua field URL hanya menerima skema `http`/`https` (pattern `^https?://`), kecuali URL media yang site-relative `/uploads/...`.
- **BE-S5:** password `minLength 10`, `maxLength 64`, dan BE juga menolak jika lebih dari 72 byte (422).
- **BE-S7:** kolom CSV leads (urut): `createdAt, name, company, email, whatsapp, service, startTimeframe, status, message, notes`. UTF-8 dengan BOM supaya terbaca benar di Excel.
- **BE-S8:** daftar leads admin diurutkan `createdAt desc`. Daftar konten admin diurutkan `updatedAt desc`, kecuali resource yang bisa diurutkan ulang.
- **BE-S9:** list admin untuk services, testimonials, dan client-logos **tidak dipaginasi** (mengembalikan semua, urut `sortOrder`).
- **Lainnya (PM):** field `company` pada lead menjadi **opsional (nullable)** karena audiens B2C mungkin tidak punya perusahaan. Ini mengubah keputusan Tech Lead #4.

## Nice to have
Diterima sebagaimana tertulis di masing-masing review. Tidak memblokir freeze. Tech Lead boleh menerapkan yang hanya berupa perubahan deskripsi satu baris; sisanya diabaikan untuk v1.0.

## Setelah revisi
Tech Lead: lint ulang (0 error), validasi ulang semua contoh dengan kin-openapi v0.135.0, commit, buat tag `contract-v1.0`, lalu salin `api/openapi.yaml` ke `/home/user/new_m2s_compro_fe/docs/openapi.yaml` dan commit di repo FE. Perubahan kontrak setelah ini harus lewat PM.
