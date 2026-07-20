# SICUTI — Sistem Informasi Cuti Guru & Pegawai

Aplikasi manajemen pengajuan cuti untuk Dinas Pendidikan & Kebudayaan Kab. Morowali Utara.

- **Backend**: Next.js 14 (API Routes) + Prisma + SQLite + JWT
- **Frontend**: Vue 3 + Vite + Pinia + DataTables.net + SweetAlert2
- **Tema**: Palet "Earthy & Nusantara" — Hijau Sage `#2F5233`, Terakota `#C05621`, Krim `#FDFBF7`, Cokelat Tua `#3B2314`

## Fitur

| Fitur | Admin Utama | Admin Pembantu | Pegawai |
|---|---|---|---|
| Data pegawai aktif (DataTables) | ✔ | ✔ | – |
| Unduh template, import & export Excel | ✔ | ✔ | – |
| Ajukan cuti (untuk pegawai mana pun) | ✔ | ✔ | ✔ (diri sendiri) |
| Setujui / Kembalikan / Tolak cuti | ✔ | ✔ | – |
| Perbaiki cuti yang dikembalikan | ✔ | ✔ | ✔ |
| Ajukan perubahan data pegawai | – | – | ✔ |
| Proses perubahan data | ✔ | ✔ | – |
| Kelola akun & reset password | ✔ | – | – |
| Preview & unduh surat Word (formulir + rekomendasi) | ✔ | ✔ | ✔ (miliknya) |
| Unggah berkas ber-TTD Kepala Dinas | ✔ | ✔ | – (hanya lihat/unduh) |
| Detail pegawai + dokumen kepegawaian (SK CPNS, SK PNS, SK Pangkat Terakhir, KGB Terakhir, SK Terakhir) | ✔ | ✔ | ✔ (miliknya, via Profil) |

**Jenis cuti & kelengkapan berkas** (unggah PDF/JPG/PNG, maks 5 MB/file):

1. **Cuti Tahunan** — rekomendasi kepsek, SK terakhir
2. **Cuti Melahirkan** — rekomendasi kepsek, SK terakhir, surat ket. HPL, buku KIA, hasil USG (opsional)
3. **Cuti Tahunan Umroh** — rekomendasi kepsek, SK terakhir, surat keterangan travel
4. **Cuti Sakit** — SK terakhir, surat rujukan, surat ket. rawat inap
5. **Cuti Alasan Penting** — SK terakhir, rekomendasi kepsek, dokumen pendukung (boleh banyak file)

**Aturan cuti tahunan (FIFO)**: jatah **12 hari kerja/tahun**. Pemakaian selalu memotong **sisa tahun terlama lebih dulu** — mis. sisa 2026 = 6, lalu cuti 2027 diambil 4 hari → terbaca dari 2026 (sisa 2), jatah 2027 tetap utuh. Saldo mulai dihitung **sejak tahun pengajuan pertama pegawai** (tahun sebelum itu tampil kosong pada formulir, bukan 12). Jika total sisa habis, pengajuan cuti tahunan baru **ditolak otomatis** (pengajuan berstatus DIAJUKAN ikut mengunci jatah). Form menampilkan sisa per tahun secara langsung.

**Rekomendasi kepala sekolah**: hanya wajib bagi pegawai yang bertempat tugas di **sekolah**; pegawai yang bertugas di **Dinas** otomatis dibebaskan dari lampiran ini (form menampilkan keterangannya, server juga memvalidasi). Pengajuan cuti kini juga memuat field **No. HP**.

**Perhitungan lama cuti**: saat mengajukan cuti tersedia pilihan pola **5 Hari Kerja** (Sabtu-Minggu tidak dihitung), **6 Hari Kerja** (Minggu tidak dihitung), atau **Hari Kalender**. Tanggal pada menu **Hari Libur** (admin) juga dikecualikan otomatis, sehingga akhir pekan dan libur nasional tidak mengurangi jatah cuti. Formulir Word menampilkan **Catatan Cuti** dengan sisa cuti tahunan otomatis: **N** (tahun berjalan), **N-1** (sisa 1 tahun sebelumnya), **N-2** (sisa 2 tahun sebelumnya) — dihitung dari jatah 12 hari dikurangi cuti tahunan yang disetujui per tahun.

> **Integrasi SK Terakhir**: setiap pegawai punya penyimpanan dokumen kepegawaian (tombol **Detail** pada Data Pegawai Aktif, atau menu Profil bagi pegawai). Jika dokumen **SK Terakhir** sudah tersimpan di sana, form pengajuan cuti tidak lagi mewajibkan unggah SK — server otomatis menyalinnya sebagai lampiran pengajuan (snapshot, sehingga penggantian SK di kemudian hari tidak mengubah berkas cuti lama). Pengguna tetap bisa mengunggah SK berbeda bila diinginkan.

Alur status: `DIAJUKAN → DISETUJUI / DIKEMBALIKAN / DITOLAK`. Pengajuan yang dikembalikan dapat diperbaiki dan otomatis kembali berstatus `DIAJUKAN`, lengkap dengan riwayat proses (timeline).

**Setelah pengajuan DISETUJUI:**

1. Halaman detail menampilkan tombol **Preview & Unduh Surat** dengan dua dokumen:
   - **Formulir Permintaan dan Pemberian Cuti** — diunduh sebagai **PDF** (dirender persis dari pratinjau, termasuk logo kop); memuat catatan cuti N/N-1/N-2
   - **Surat Rekomendasi Izin Cuti** — diunduh sebagai **Word (.docx)**, penerusan oleh Kepala Dinas kepada Bupati Cq. Kepala BKPSDM
   Nomor surat, tanggal, serta nama & NIP Kepala Dinas dapat diedit di pratinjau sebelum diunduh sebagai **.docx** (dibuat server-side dengan library `docx`). Kop surat menampilkan **lambang Kabupaten Morowali Utara** (`backend/assets/logo-morut.png` untuk dokumen Word, `frontend/public/logo-morut.png` untuk pratinjau — ganti kedua file ini bila logo berubah).
2. Panel **Berkas Ditandatangani Kepala Dinas** — admin mengunggah hasil scan berkas yang telah di-TTD (PDF/JPG/PNG, maks 10 MB, boleh banyak file); pegawai dapat melihat dan mengunduhnya. Setiap unggahan tercatat di riwayat proses.

## Menjalankan

Butuh **Node.js 18+**.

### 1. Backend (port 3001)

```bash
cd backend
npm install
npm run db:setup      # migrasi database + seed akun awal
npm run dev
```

### 2. Frontend (port 5173)

```bash
cd frontend
npm install
npm run dev
```

Buka **http://localhost:5173** (permintaan `/api` diteruskan otomatis ke backend).

### Akun awal (hasil seed)

| Role | Username | Password |
|---|---|---|
| Admin Utama | `admin` | `admin123` |
| Admin Pembantu | `operator` | `operator123` |
| Pegawai | NIP pegawai | NIP pegawai |

Akun pegawai **dibuat otomatis** saat pegawai ditambahkan manual atau diimport dari Excel: username & password awal = NIP (tanpa spasi). Pegawai nonaktif tidak dapat login.

### Keamanan akun

- **Wajib ganti password**: akun dengan password bawaan (NIP) atau yang baru direset admin dipaksa mengganti password saat login — seluruh menu dikunci sampai password diganti.
- **Kebijakan password**: minimal 8 karakter, mengandung huruf dan angka, tidak boleh sama dengan NIP/username.
- **Kunci otomatis**: 5 kali gagal login beruntun mengunci akun 15 menit; pesan error tidak membocorkan apakah username terdaftar; waktu login terakhir dicatat.
- **Ubah role** (Admin Utama, menu Kelola Akun): akun pegawai dapat dijadikan **Admin Pembantu** dan dikembalikan menjadi Pegawai kapan saja; sistem menjaga minimal satu Admin Utama tersisa.

### Laporan untuk atasan

Menu **📊 Laporan** (admin): saring pengajuan per rentang tanggal, status, dan jenis cuti — tampil ringkasan (total pengajuan, total hari, per status, per jenis) dan tabel rinci, lalu **Export Excel** menghasilkan file rekap berjudul resmi lengkap dengan periode dan ringkasan.

### Import data pegawai (format SIASN/BKN)

Struktur data pegawai mengikuti kolom: `NIP BARU | NIK | NAMA | TEMPAT LAHIR NAMA | TANGGAL LAHIR | JENIS KELAMIN | STATUS CPNS PNS | TANGGAL SK CPNS | TMT CPNS | TANGGAL SK PNS | TMT PNS | GOL AWAL NAMA | GOL AKHIR NAMA | TMT GOLONGAN | JENIS JABATAN NAMA | JABATAN NAMA | TMT JABATAN | TINGKAT PENDIDIKAN NAMA | PENDIDIKAN NAMA | KECAMATAN | UNOR NAMA | TEMPAT TUGAS | STATUS | TANGGAL KENAIKAN GAJI BERKALA TERAKHIR | TANGGAL KENAIKAN PANGKAT TERAKHIR | TANGGAL PENSIUN | TANGGAL KENAIKAN GAJI BERKALA | TANGGAL KENAIKAN PANGKAT | TAHUN PENGANGKATAN`.

1. Menu **Data Pegawai Aktif → Unduh Template** (header persis kolom di atas + 1 baris contoh).
2. Isi data — kolom `NIP BARU` dan `NAMA` wajib. Baris header dicari otomatis (tidak harus di baris pertama), sel tanggal Excel dikonversi ke `YYYY-MM-DD`, dan header `NIP` lama tetap dikenali sebagai `NIP BARU`.
3. **Import Excel** — NIP yang sudah ada diperbarui (upsert), NIP baru otomatis dibuatkan akun (username & password = NIP).
4. **Export Excel** menghasilkan file dengan susunan kolom yang sama plus kolom `NO` dan `STATUS AKTIF`.

> **Catatan migrasi**: jika sebelumnya sudah menjalankan versi lama, struktur tabel pegawai berubah — jalankan ulang `npx prisma migrate reset` lalu `npm run db:setup` di folder backend (data lama pegawai akan dihapus).

## Struktur

```
sicuti/
├── backend/
│   ├── prisma/schema.prisma      # model: User, Pegawai, PengajuanCuti, BerkasCuti, RiwayatCuti, PerubahanData
│   ├── src/lib/                  # auth (JWT), excel (template/import/export), cuti-config
│   ├── src/app/api/              # endpoint REST
│   └── uploads/                  # berkas cuti tersimpan di sini
└── frontend/
    └── src/
        ├── views/                # Login, Dashboard, Pegawai, Cuti (list/form/detail), Perubahan Data, Akun, Profil
        ├── components/           # AppLayout, StatusBadge
        ├── stores/auth.js        # Pinia
        └── cuti-config.js        # sinkron dgn backend
```

## Menjalankan secara lokal

Database kini **PostgreSQL** (siap Vercel). Untuk lokal, gunakan Postgres lokal/Docker atau database gratis Neon, lalu isi `DATABASE_URL` di `backend/.env` (contoh ada di `.env.example`). Tanpa `BLOB_READ_WRITE_TOKEN`, berkas otomatis tersimpan di `backend/uploads/` seperti biasa.

## Deploy ke Vercel

Proyek sudah **Vercel-ready**: PostgreSQL + Vercel Blob (dengan fallback disk lokal), `vercel-build` yang menjalankan migrasi otomatis, CORS middleware, rewrite `/api` di frontend, dan logo kop yang ikut terbundel di serverless function. Ikuti panduan lengkap di **`DEPLOY_VERCEL.md`** (dua project: `backend/` dan `frontend/`).

## Catatan produksi

- Wajib set `JWT_SECRET` acak dan `DATABASE_URL` di environment (jangan pakai nilai contoh).
- Batas upload di Vercel ±4,5 MB per request; berkas cuti dibatasi 5 MB per file.
- Untuk VPS sendiri: `npm run build && npm start` di kedua folder, reverse proxy meneruskan `/api` ke port 3001.
