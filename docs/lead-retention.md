# Job Retensi Lead (`cmd/retention`)

Binary terpisah dari API. Dijalankan sebagai **Dokploy Scheduled Job**, bukan goroutine di dalam
proses API, supaya penghapusan tidak menambah latensi request dan bisa diulang tanpa me-restart API.

## Dasar cutoff

Baris `leads` dihapus bila `updated_at < now() - LEAD_RETENTION_MONTHS`.

- Pengurangan memakai **bulan kalender** dengan pembatasan akhir bulan: 31 Maret − 1 bulan = 28
  Februari (bukan 3 Maret seperti normalisasi tanggal Go), 31 Maret 2024 − 1 bulan = 29 Februari.
  Tidak ada asumsi 30 hari atau 730 hari, jadi cutoff tidak bergeser seiring tahun kabisat.
- `updated_at` adalah **waktu perubahan administratif terakhir** (admin mengubah status atau catatan),
  bukan waktu interaksi terakhir pelanggan. Lead yang sudah lama tidak di-follow up tapi baru
  di-update admin tetap dihitung baru.
- Lead yang belum pernah diubah admin tetap ikut terhitung, karena kolom `updated_at` punya
  `NOT NULL DEFAULT now()` sehingga nilainya sama dengan `created_at` sampai ada patch admin.
- Tabel lain tidak mereferensikan `leads` (tidak ada foreign key maupun tabel turunan), jadi
  menghapus satu baris `leads` tidak meninggalkan data yatim.

## Konfigurasi

| Name | Default | Keterangan |
|---|---|---|
| `DATABASE_URL` | — (wajib) | Koneksi PostgreSQL yang sama dengan API |
| `LEAD_RETENTION_MONTHS` | `24` | Jendela retensi. `0` = job dinonaktifkan (tidak menyentuh DB) |
| `DRY_RUN` | `true` | `true` hanya menghitung kandidat, `false` benar-benar menghapus |

`DRY_RUN` sengaja default `true`: job yang belum pernah dibersihkan untuk produksi tidak akan
menghapus apa pun. Ubah ke `false` hanya setelah langkah di bawah selesai.

## Prosedur sebelum penghapusan pertama di produksi

Wajib dilakukan berurutan. Jangan melewati langkah backup. `DRY_RUN=false` dan jadwal rutin **baru
boleh diaktifkan setelah tim infra menyetujui jumlah kandidat dari dry-run production**.

1. **Backup data lead.** Ekspor seluruh lead dari admin API (admin-only, CSV, tanpa pagination):

   ```sh
   curl -fSs -b "m2s_session=<cookie admin>" \
     "https://<domain>/api/v1/admin/leads/export" -o leads-backup-$(date +%F).csv
   ```

   Kolom CSV: `createdAt, name, company, email, whatsapp, service, startTimeframe, status, message,
   notes, id, updatedAt, serviceSlug`. `id`, `updatedAt`, dan `serviceSlug` disertakan agar setiap
   baris bisa dicocokkan dengan database, dengan cutoff job, dan bisa dimasukkan ulang tanpa menebak
   layanan. `ip_hash` dan `user_agent` sengaja tidak diekspor (dipakai internal untuk investigasi
   penyalahgunaan), jadi baris hasil restore akan mengosongkan dua kolom itu.

   Verifikasi file sebelum lanjut:
   - jumlah baris CSV (tanpa header) sama dengan `total` pada `GET /api/v1/admin/leads`;
   - buka dengan spreadsheet atau `python3 -c 'import csv,sys; rows=list(csv.reader(open(sys.argv[1], encoding="utf-8-sig"))); print(len(rows)-1, rows[0])' leads-backup-*.csv`;
   - pastikan kolom `updatedAt` terisi pada semua baris.

   Simpan file di luar container, akses terbatas. Catatan: file ini berisi data pribadi; hapus setelah
   persetujuan penghapusan selesai dan masa simpan backup berakhir.

   Restore: belum ada endpoint import lead. Jika suatu saat perlu memulihkan, masukkan ulang secara
   manual dari CSV (baris dengan `id` yang sama tidak akan bertabrakan karena PK uuid, tapi tanggal
   harus diisi eksplisit, bukan `now()`). Sel yang diawali `'` karena guard CSV injection harus
   dibuang satu kutipnya bila karakter setelahnya `= + - @ \t \r`. Prosedur ini diuji di database
   test: `TestLeadCSVExportRestoreRoundTrip` (export → hapus → restore → bandingkan semua kolom,
   termasuk ID, timestamp, Unicode, dan notes dengan newline/koma/kutip).

2. **Jalankan dry-run dan catat hasilnya.** Set `DRY_RUN=true`, lalu jalankan job. Log JSON berisi:

   ```json
   {"msg":"retention run","dry_run":true,"months":24,"cutoff":"2024-11-02T03:00:00Z","candidates":0,"deleted":0}
   ```

   Kirim `candidates` dan `cutoff` ke tim infra/legal untuk persetujuan.

3. **Minta persetujuan tertulis** untuk menghapus kandidat tersebut. Selama belum ada persetujuan,
   `DRY_RUN` tetap `true`.

4. **Aktifkan penghapusan.** Set `DRY_RUN=false`, jalankan job sekali secara manual, lalu cek log:
   `deleted` harus sama dengan `candidates` pada dry-run (kecuali ada lead baru di antara keduanya).

5. **Jadwalkan rutin** (mis. harian) dengan `DRY_RUN=false`. Job idempoten: run kedua pada hari yang
   sama menemukan 0 kandidat dan menghapus 0 baris.

## Menjalankan

```sh
LEAD_RETENTION_MONTHS=24 DRY_RUN=true /app/retention
```

Exit code `0` hanya bila job selesai tanpa error, atau bila job dinonaktifkan (`LEAD_RETENTION_MONTHS=0`).
Exit code `1` untuk config tidak valid, kegagalan koneksi, atau kegagalan query/transaksi — scheduler
harus membacanya sebagai job gagal, bukan sukses. Semua log berbentuk JSON ke stdout.

Penghitungan kandidat dan penghapusan berjalan dalam **satu transaksi**: angka `candidates` dan
`deleted` berasal dari snapshot yang sama, dan bila `DELETE` gagal seluruh transaksi rollback.
Beberapa instance job yang berjalan bersamaan tetap aman karena `DELETE` mengunci baris yang sama,
tapi jadwalkan satu run per periode.

Binary `/app/retention` sudah ikut dibangun di `Dockerfile` yang sama dengan API
(`go build ./cmd/retention` lalu `COPY --from=build /out/retention /app/retention`); tidak ada image
atau port tambahan.

## Pengecualian status di masa depan

`retention.Options.ExemptStatuses` sudah disediakan dan diteruskan ke query
(`status <> ALL($2::text[])`). Kosong hari ini. Saat status `converted` ditambahkan, isi daftar itu
dengan `["converted"]` (atau status bisnis lain yang harus disimpan) tanpa menulis ulang query.
Job tidak pernah mengasumsikan `status = 'done'` berarti konversi.
