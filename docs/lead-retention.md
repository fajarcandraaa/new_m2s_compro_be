# Job Retensi Lead (`cmd/retention`)

Binary terpisah dari API. Dijalankan sebagai **Dokploy Scheduled Job**, bukan goroutine di dalam
proses API, supaya penghapusan tidak menambah latensi request dan bisa diulang tanpa me-restart API.

## Dasar cutoff

Baris `leads` dihapus bila `updated_at < now() - LEAD_RETENTION_MONTHS`.

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

Wajib dilakukan berurutan. Jangan melewati langkah backup.

1. **Backup data lead.** Ekspor seluruh lead dari admin API (admin-only, CSV):

   ```sh
   curl -fSs -b "m2s_session=<cookie admin>" \
     "https://<domain>/api/v1/admin/leads/export" -o leads-backup-$(date +%F).csv
   ```

   Simpan file di luar container dan pastikan ukuran serta jumlah barisnya masuk akal.
   Catatan: file CSV ini berisi data pribadi; simpan terbatas dan hapus setelah selesai.

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

Exit code `0` saat sukses atau saat job dinonaktifkan, non-zero saat config tidak valid atau query
database gagal. Semua log berbentuk JSON ke stdout agar terbaca di Dokploy.

> Catatan gambar Docker: binary ini perlu ada di image (mis. `COPY --from=build /out/retention /app/retention`).
> Perubahan `Dockerfile` produksi menunggu persetujuan tim infra.

## Pengecualian status di masa depan

`retention.Options.ExemptStatuses` sudah disediakan dan diteruskan ke query
(`status <> ALL($2::text[])`). Kosong hari ini. Saat status `converted` ditambahkan, isi daftar itu
dengan `["converted"]` (atau status bisnis lain yang harus disimpan) tanpa menulis ulang query.
Job tidak pernah mengasumsikan `status = 'done'` berarti konversi.
