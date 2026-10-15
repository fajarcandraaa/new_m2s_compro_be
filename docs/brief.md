# Project Brief: Mind2Screen Company Profile v2

Owner: Senior Project Manager. Dokumen ini adalah sumber konteks utama untuk Tech Lead, Backend Engineer, dan Frontend Engineer. Baca seluruhnya sebelum mulai.

## 1. Produk
Situs company profile baru untuk **Mind2Screen** (legal: Mindtoscreen Technology Indonesia, Malang), partner pembuatan sistem internal B2B (HRIS, DMS, portal internal, sistem custom), plus layanan Web Development untuk usaha & personal (B2C, sekunder).

Terdiri dari:
1. **Situs publik** (Bahasa Indonesia saja; struktur siap untuk EN nanti, tapi jangan bangun i18n sekarang).
2. **CMS admin** sederhana di `/admin` untuk mengelola konten.

Target: **selesai hari ini**. Prioritaskan fitur inti yang berfungsi benar di atas polesan.

## 2. Repo, stack, dan lingkungan
| | Repo | Path lokal | Stack |
|---|---|---|---|
| Backend | `fajarcandraaa/new_m2s_compro_be` | `/home/user/new_m2s_compro_be` | Go 1.24, chi, pgx/v5, goose (embedded SQL migrations, auto-run on start), PostgreSQL 16 |
| Frontend | `fajarcandraaa/new_m2s_compro_fe` | `/home/user/new_m2s_compro_fe` | Next.js 15 App Router, TypeScript, Tailwind CSS v4, `motion` (Framer Motion), TanStack Query, react-hook-form + zod, Tiptap |

- **Branch kerja di kedua repo:** `feat/initial-build`. Jangan push ke `main`. Jangan buat PR. Commit kecil dan bermakna. Akhiri pesan commit dengan baris attribution yang diberikan PM.
- **Lingkungan sesi:** Go 1.24, Node 22, PostgreSQL 16 tersedia (binary di `/usr/lib/postgresql/16/bin`; jalankan cluster lokal sendiri untuk tes, misalnya `initdb` di folder sementara + `pg_ctl` di port 5433). Chromium untuk Playwright ada di `/opt/pw-browsers/chromium-1194/chrome-linux/chrome` (jangan `playwright install`). **Tidak ada Docker daemon**: tulis Dockerfile/compose, tapi tidak bisa di-`docker build` di sini.
- **Deploy target:** Docker lokal, lalu VPS dengan Dokploy.
  - Repo BE berisi `docker-compose.yml` (postgres + be + fe dengan `fe.build.context: ../new_m2s_compro_fe`) untuk jalan lokal dengan kedua repo berdampingan.
  - Di Dokploy: Compose (postgres + be) dari repo BE dan aplikasi Dockerfile (fe) dari repo FE.

## 3. Arsitektur yang sudah diputuskan PM (jangan diubah tanpa persetujuan)
- **Satu origin untuk browser.** Next.js me-rewrite `/api/*` dan `/uploads/*` ke BE (`BACKEND_URL` internal, mis. `http://be:8080`). Server Components memanggil BE langsung via `BACKEND_URL`.
- **Auth admin:** email + password (bcrypt), JWT disimpan di **cookie httpOnly** `m2s_session` (Secure di production, SameSite=Lax), diset oleh BE. Peran: `admin` (semua) dan `editor` (konten saja: artikel, layanan, studi kasus, promo, testimoni, logo klien, dashboard, baca leads). Hanya `admin` yang boleh ke `settings` dan `users`. Proteksi CSRF untuk endpoint admin yang mengubah data: wajibkan header `X-Requested-With: m2s-cms` (cukup untuk same-origin + SameSite=Lax).
- **Auto-migration:** file SQL versioned di `migrations/` di-embed (`embed.FS`) dan dijalankan goose saat start bila `MIGRATE_ON_START=true` (default true). **Seed** idempoten: jalan hanya jika tabel kosong, dari `docs/content/seed.json` + `docs/content/articles.json` (embed). Admin awal dari env `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME`.
- **Media:** upload multipart ke `UPLOAD_DIR` (default `/data/uploads`, volume Docker), disajikan BE di `/uploads/*`. Validasi mime by content sniffing: jpg, png, webp. Tolak SVG demi keamanan. Maksimal 2 MB. Nama file acak.
- **Konten rich text** (artikel) disimpan sebagai HTML, disanitasi di BE dengan `bluemonday` (UGC policy) saat simpan.
- **Revalidasi FE:** setelah konten admin berubah, BE memanggil `POST {FRONTEND_URL}/api/revalidate` dengan header `X-Revalidate-Secret` (env `REVALIDATE_SECRET`), body `{ "tags": ["articles"] }` dst. Fire-and-forget, gagal tidak memblokir. FE memakai `fetch(..., { next: { tags: [...], revalidate: 300 } })`.
- **Status promo** dihitung dari `starts_at`/`ends_at` vs sekarang (`active`, `upcoming`, `ended`). Promo berakhir tetap tampil di publik 7 hari setelah `ends_at` (redup), lalu disembunyikan.
- **Toggle settings:** `show_testimonials` mengontrol section testimoni beranda (false → section tidak dirender sama sekali; juga jika tidak ada testimoni aktif). `show_client_logos` mengontrol strip logo di hero. Beranda menampilkan maksimal 3 testimoni aktif sesuai `sort_order`.
- **Leads:** dari form `/diskusi`. Field: name, company, email, whatsapp, service (slug opsional), start_timeframe (opsional), message. Status: `new`, `contacted`, `done`; plus catatan internal. Rate limit `POST /leads` (5/menit per IP) dan honeypot field `website` (jika terisi → 201 palsu, tidak disimpan).
- **Respons API:** JSON camelCase. Error: `{ "error": { "code": "validation_error", "message": "...", "fields": { "email": "..." } } }`. List: `{ "data": [...], "meta": { "page", "pageSize", "total" } }`. Single: `{ "data": {...} }`. Waktu dalam RFC3339 UTC.
- **Observability minimal:** `GET /healthz` (cek DB), log terstruktur `slog` JSON.

## 4. Design system (wajib dipatuhi FE)
**Figma:** file key `is20WupHCNceNrBx0pfS29`. Sub-agent bisa melihat desain lewat tool Figma MCP `get_screenshot` (pakai `enableBase64Response: true`) atau `get_metadata` dengan node ID di bawah.

**Token warna:**

| Token | Terang | Gelap |
|---|---|---|
| ground | #F1F8FE | #080808 |
| surface | #FFFFFF | #181818 |
| line | #D5DEE6 | #333333 |
| ink | #1A1A1A | #FFFFFF |
| ink-muted | #666666 | #A3A3A3 |
| logo-gray | #808080 | #808080 |
| cyan | #30C2DB | #30C2DB |
| cyan-glow | #43F4FA | #43F4FA |
| cyan-ink | #0B7285 | #30C2DB |
| on-cyan | #080808 | #080808 |

- Section gelap (hero, CTA penutup, footer) memakai tema gelap lewat class/attribute scope (mis. `data-theme="dark"` pada section), bukan mode global.
- **Tipografi:** Poppins (`next/font/google`).
  - display 48/52 600 (hero beranda 56/64 di desktop, 36/42 di mobile)
  - heading 28/36 600, title 20/28 600, body 16/26 400, small 14/22 400
  - tagline 14/20 500 UPPERCASE letter-spacing 12% (eyebrow)
- **Spasi:** 4, 8, 16, 24, 32 (space-1/2/4/6/8). Section desktop padding 96–112px vertikal, gutter desktop 96px, mobile 16px. Container 1248px.
- **Radius:** sm 4 (tag, input), md 8 (button), lg 16 (card, panel).

**Aturan yang tidak boleh dilanggar:**
- Cyan satu-satunya warna brand: dipakai untuk aksi utama dan satu aksen per tampilan. Teks di atas cyan memakai on-cyan, teks cyan di latar terang memakai cyan-ink.
- **Tanpa bayangan.** Kedalaman datang dari ground vs surface + border line.
- Gradien hanya `cyan → cyan-glow` ke kanan bawah, dan hanya pada mark/bentuk layar brand.
- Tanpa emoji, tanpa tanda seru. Belum ada set ikon: pakai label teks (mis. "Pilih", "Ya/Tidak", "Menu").
- Logo: pakai file master di `/public/brand` apa adanya, jangan digambar ulang atau diubah warnanya. Versi `-white` di latar gelap, `-gray` di latar terang. Header memakai lockup horizontal.
- **Copy:** Bahasa Indonesia, "Kami" kepada "Anda" (kapital). Tombol menyebut langkah berikutnya: "Diskusi Kebutuhan Sistem", "Lihat Studi Kasus", "Chat via WhatsApp".
- **Elemen grafis brand:**
  - **Layar miring:** bidang tegak dengan sisi atas dan bawah miring paralel, path relatif 200×520: `M0 170 L200 0 L200 350 L0 520 Z`, sudut radius 8, gradien brand. Dipakai besar dan terpotong tepi.
  - **Bintang 4 titik:** path 100×100: `M50 0 C53 36 60 46 100 50 C60 54 53 64 50 100 C47 64 40 54 0 50 C40 46 47 36 50 0 Z`, warna cyan.
  - **Dot field:** titik 8px warna surface (gelap), jarak 32px, offset 12px, di atas ground gelap.

**Komponen (nama Figma):** Button (Primary/Secondary × Default/Hover/Disabled; Disabled opacity 0.4; Hover primary → cyan-glow, secondary → border cyan), Tag (Eyebrow / Chip), Input, Textarea, Select, Card, Service Card, Case Study Card (2 angka dampak), Article Card (sampul 16:9; default tanpa gambar = latar gelap + 2 layar miring di kanan), Promo Card + Status Badge, Testimonial, Client Logo, Step (garis cyan 2px di atas), Header, Footer, Page Hero (gelap, dot field, 2 layar miring kanan), CMS/Sidebar, CMS/Nav Item, CMS/Toggle.

**Node ID Figma halaman** (halaman "Website – Desktop" `29:1504`, "Website – Mobile" `35:1504`, "CMS" `59:1504`, "Components" `5:4`, "Foundations" `5:2`):
- Beranda desktop `29:1505`, prototipe hero `34:4416`, state gelombang kursor `41:6221`, spesifikasi gelombang `41:9548`, anotasi toggle testimoni `42:6523`
- Layanan `45:6523`, Detail Layanan `45:8793`, Studi Kasus `47:9771`, Detail Studi Kasus `47:10875`, Artikel `49:11934`, Detail Artikel `49:13040`, Cara Kerja `50:14100`, Tentang `50:15245`, Promo `51:16280`, Diskusi Kebutuhan `51:17390`, State brief terkirim `51:18518`
- Mobile: Beranda `35:1505`, Layanan `55:1865`, Diskusi `55:2045`
- CMS (frame pada page `59:1504`, nama "CMS – ..."): Login, Dashboard, Artikel, Artikel / Editor, Layanan, Studi Kasus / Editor, Promo, Testimoni, Leads, Info Perusahaan, Pengguna

## 5. Halaman publik & routing (FE)
| Route | Sumber data | Catatan |
|---|---|---|
| `/` | site, services, case-studies?featured, testimonials, client-logos, promos?status=active | 10 section (lihat 5.1) |
| `/layanan` | services, promos aktif | Grid 4 layanan bisnis, paket Web Development (B2C), tabel BOS vs BOR, CTA |
| `/layanan/[slug]` | service | 2 kolom: konten + panel diskusi sticky |
| `/studi-kasus` | case-studies | Filter kategori (client-side), grid kartu |
| `/studi-kasus/[slug]` | case-study | Ringkasan angka, Kondisi awal / Risiko / Pendekatan / Dampak / Pelajaran, panel diskusi |
| `/artikel` | articles, tags | Artikel unggulan (terbaru) + grid + filter tag (`?tag=`) + pagination |
| `/artikel/[slug]` | article | Kolom baca 720px, tag, bagikan (WhatsApp, LinkedIn, salin tautan), CTA, artikel terkait (tag sama) |
| `/cara-kerja` | statis | Copy dari Figma |
| `/tentang` | statis + site | Copy dari Figma |
| `/promo` | promos | Filter Semua/Aktif/Akan datang; empty state |
| `/diskusi` | services (untuk select), site | Form brief → `POST /leads`; state sukses menggantikan form |
| `sitemap.xml`, `robots.txt` | | Semua route publik + detail; 404 harus status 404 asli |

Header nav: Layanan, Cara Kerja, Studi Kasus, Artikel, Tentang + tombol Primary "Diskusi Kebutuhan Sistem" (→ `/diskusi`). Mobile: tombol teks "Menu" membuka drawer. Footer gelap: email, WhatsApp, alamat dari `site`. Tombol WhatsApp → `https://wa.me/{nomor tanpa +}?text={whatsapp_greeting}`.

### 5.1 Section Beranda (urutan)
1. **Hero gelap**
   - Dot field interaktif (gelombang kursor) + header. Eyebrow "Partner Sistem Internal B2B".
   - H1 "Sistem internal yang tumbuh bersama bisnis Anda".
   - Lead: "Kami membangun HRIS, DMS, dan portal internal yang modular dan terdokumentasi. Kami mulai dari memahami cara kerja tim Anda, bukan langsung menulis kode."
   - CTA "Diskusi Kebutuhan Sistem" + "Lihat Studi Kasus". Visual animasi di kanan.
   - Strip logo klien ("Dipercaya tim di") jika `show_client_logos` true dan ada logo.
2. **Masalah:** 3 kartu (01 Kebutuhan tidak jelas / 02 Perubahan kecil merusak sistem / 03 Sistem tidak ikut tumbuh).
3. **Sistem yang Kami bangun:** 4 kartu (HRIS, DMS, PORTAL, CUSTOM).
4. **Proses:** 4 Step + panel BOS/BOR.
5. **Studi kasus:** 2 kartu unggulan + tombol "Lihat Semua Studi Kasus".
6. **Testimoni:** maks. 3, kondisional.
7. **Strip B2C** "Butuh website untuk usaha Anda?" (harga mulai dari layanan web-development) + "Lihat promo aktif" (hanya jika ada promo aktif) + "Lihat Paket Website".
8. **Cocok / kurang cocok:** 2 panel.
9. **CTA penutup gelap** "Ceritakan proses yang ingin Anda rapikan" + 2 tombol.
10. **Footer.**

Copy statis lengkap ada di frame Figma. Ambil persis dari sana.

### 5.2 Animasi (FE, prioritas 5)
- **Hero "Ide menjadi layar"** (`motion`):
  - t=0 bintang muncul (scale 0.6→1, opacity), 0.6s layar kiri naik (y +160 → 0, opacity), 1.2s layar kanan naik (y +200 → 0), 1.8s dot terang di sekitar visual menyala dari tengah.
  - Idle loop: layar kiri y ±8, layar kanan y ∓8, 2.4s ease-in-out, yoyo.
- **Gelombang kursor** (`<canvas>`): radius 260px, panjang gelombang 88px; di puncak titik membesar sampai 15px dan berubah ke cyan (opacity 0.2–0.9), terdorong keluar maks. 9px.
  - Posisi kursor di-lerp 0.12 per frame, riak meluruh 600ms setelah kursor berhenti atau keluar hero.
  - Hanya render saat terlihat (IntersectionObserver); di perangkat sentuh satu riak pelan dari pusat mark setiap 4 detik.
- **Reveal section:** fade + translateY 16px, sekali saja.
- **`prefers-reduced-motion`:** semua animasi mati dan langsung tampil state akhir (dot field statis).

## 6. CMS (`/admin`, sederhana)
Layout: sidebar 240px (grup Konten: Dashboard, Artikel, Layanan, Studi Kasus, Promo, Testimoni; Bisnis: Leads; Pengaturan: Info Perusahaan, Pengguna, hanya admin) + area utama (judul, aksi kanan atas, tabel/form). Tema terang.

| Layar | Fungsi |
|---|---|
| Login | email, kata sandi |
| Dashboard | angka: leads `new`, konten draft, promo aktif; tabel 5 leads terbaru; draft terakhir |
| Artikel | tabel (cari, filter status) → editor: judul, slug (auto dari judul, bisa diedit), ringkasan, isi (Tiptap: heading, bold, italic, list, quote, link, image upload), status draft/published, jadwal terbit (`published_at` di masa depan = terjadwal), tag (multi), sampul (upload), SEO (judul, meta description, og image) + pratinjau Google |
| Layanan | tabel urut `sort_order` (ubah urutan lewat tombol naik/turun) → editor: semua field di seed (points, deliverables, systems, packages untuk web-development), status, SEO |
| Studi Kasus | tabel → editor: identitas, cerita (textarea satu poin per baris untuk tiap daftar), 2 angka dampak, featured (maks. 2), status, SEO |
| Promo | tabel + panel edit: judul, layanan terkait, deskripsi, mulai, berakhir, kode opsional |
| Testimoni | saklar `show_testimonials` di atas; tabel (klien, jabatan, perusahaan, kutipan, aktif on/off, urutan); bagian logo klien: saklar `show_client_logos`, upload, hapus, urutan |
| Leads | tabel (cari, filter status) + panel detail: semua field, ubah status, catatan internal, tombol "Hubungi via WhatsApp"; ekspor CSV |
| Info Perusahaan (admin) | semua field `settings` kecuali toggle |
| Pengguna (admin) | daftar, tambah (nama, email, peran, kata sandi awal), ubah peran, nonaktifkan, reset kata sandi; tidak bisa menghapus/menurunkan diri sendiri |

Feedback: toast teks "Tersimpan" / pesan error dari API; konfirmasi sebelum hapus.

## 7. Konten seed
`docs/content/seed.json` (settings, 5 layanan + paket, 2 studi kasus, tag) dan `docs/content/articles.json` (3 artikel asli, HTML). Testimoni, promo, dan logo klien kosong. Sampul artikel seed: tidak ada (FE memakai sampul default brand).

## 8. Definition of Done
- **BE:**
  - `go build ./...`, `go vet ./...`, `go test ./...` hijau.
  - Migrasi + seed jalan otomatis di DB kosong.
  - Semua endpoint sesuai `api/openapi.yaml` (tes validasi respons dengan kin-openapi untuk endpoint utama).
  - Dockerfile, `.env.example`, README.
- **FE:**
  - `npm run lint`, `npm run typecheck`, `npm run build` hijau.
  - Tipe API di-generate dari `docs/openapi.yaml` (`openapi-typescript`, script `npm run gen:api`), tidak boleh tipe manual yang menyimpang.
  - Semua route di bagian 5 dan semua layar CMS di bagian 6 berfungsi terhadap BE asli.
  - Lighthouse-friendly (gambar `next/image`, metadata).
  - Dockerfile standalone, `.env.example`, README.
- Tidak ada secret di repo. Semua konfigurasi via env.
