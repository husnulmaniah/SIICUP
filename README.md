# Sistem Informasi Cuti Pegawai

Aplikasi web pengelolaan cuti pegawai dengan backend **Go** (net/http standar + GORM + PostgreSQL) dan frontend **Vue 3 + PrimeVue**. Semua tabel master memiliki CRUD lengkap, tampilan tabel data (DataTable) yang responsif, serta fitur **import & export Excel** lengkap dengan template siap pakai.

> **Mau deploy ke Railway (backend) + Vercel (frontend)?** Lihat [`DEPLOY.md`](./DEPLOY.md) untuk panduan lengkapnya. Bagian di bawah ini untuk menjalankan di komputer lokal.

## Fitur Utama

- **4 Role pengguna**: `administrator`, `admin`, `pegawai`, `atasan` — masing-masing dengan hak akses berbeda.
- **CRUD lengkap** untuk 14 tabel: Role, Jabatan, Unit Kerja, Status, Pangkat, Golongan, Pangkat-Golongan, Jenis Cuti, Pola Hari Kerja, Tanggal Merah, Pegawai, User, Jatah Cuti Tahunan, dan Pengajuan Cuti.
- **Import & Export Excel** di setiap tabel (tombol *Template*, *Import*, *Export*), termasuk validasi baris per baris saat import dengan laporan error yang jelas.
- **Alur persetujuan cuti**: pegawai mengajukan → atasan menyetujui/menolak → kuota cuti tahunan otomatis terpotong saat disetujui (khusus jenis cuti "Tahunan").
- **Perhitungan otomatis jumlah hari cuti** berdasarkan pola hari kerja (5 hari / 6 hari kerja) dan tanggal merah/hari libur yang terdaftar.
- **Dashboard** khusus per role (statistik untuk admin, daftar bawahan untuk atasan, sisa kuota cuti untuk pegawai).
- **Desain responsif** — sidebar otomatis menjadi menu geser (drawer) di layar HP/tablet.

## Struktur Folder

```
sistem-cuti-pegawai/
├── backend/          Go API (net/http + GORM + PostgreSQL)
│   ├── cmd/server/   entry point (main.go)
│   ├── config/       koneksi database & konfigurasi env
│   ├── database/     migrasi & seeding data awal
│   ├── handlers/     semua logic endpoint API
│   ├── middleware/   autentikasi JWT & pembatasan role
│   ├── models/       struct GORM untuk semua tabel
│   ├── routes/       pendaftaran seluruh route
│   ├── utils/        helper JWT, password, response, excel
│   └── vendor/       dependency Go sudah di-vendor (tidak perlu internet untuk build)
└── frontend/         Vue 3 + PrimeVue (Vite)
    └── src/
        ├── api/          instance axios
        ├── components/   CrudManager.vue (komponen tabel CRUD generik)
        ├── config/       tables.js (konfigurasi kolom & form setiap tabel)
        ├── layouts/      AppLayout.vue (sidebar + topbar responsif)
        ├── router/       routing & penjagaan akses per role
        ├── stores/       Pinia store autentikasi
        └── views/        halaman Login, Dashboard, Pengajuan Cuti, Master Data
```

## Prasyarat

- **Go** 1.24 atau lebih baru — https://go.dev/dl/
- **Node.js** 20 atau lebih baru (disertai npm) — https://nodejs.org
- **PostgreSQL** 13 atau lebih baru — https://www.postgresql.org/download/ (di Windows bisa pakai installer resmi, atau Laragon/XAMPP yang menyertakan PostgreSQL)

## 1. Menyiapkan Database

Buat database baru di PostgreSQL, misalnya lewat `psql` atau pgAdmin:

```sql
CREATE DATABASE cuti_app;
```

## 2. Menjalankan Backend (Go)

```bash
cd backend
copy .env.example .env      # Windows (gunakan "cp .env.example .env" di Mac/Linux)
```

Edit file `.env` sesuai kredensial PostgreSQL anda:

```
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres_anda
DB_NAME=cuti_app
DB_SSLMODE=disable
JWT_SECRET=ganti-dengan-teks-acak-yang-panjang
APP_PORT=8080
```

Jalankan servernya:

```bash
go run ./cmd/server
```

Dependency Go sudah disertakan di folder `vendor/`, jadi perintah di atas akan langsung berjalan **tanpa perlu koneksi internet**. Saat pertama kali dijalankan, aplikasi akan otomatis:

1. Membuat semua tabel (migrasi otomatis).
2. Mengisi data awal (seed): 4 role, beberapa data master contoh, 2 data pegawai contoh, dan **4 akun login siap pakai**.

Server berjalan di `http://localhost:8080`.

Build ke file `.exe` (opsional, untuk deploy):

```bash
go build -o cuti-server.exe ./cmd/server
```

### Akun demo (password semua akun: `admin123`)

| Username        | Role          | Keterangan                              |
|------------------|---------------|------------------------------------------|
| `administrator` | administrator | akses penuh + kelola akun & role         |
| `admin`          | admin         | kelola master data & data pegawai        |
| `andi`           | atasan        | atasan dari pegawai "Siti Rahma"         |
| `siti`           | pegawai       | staff, bisa mengajukan cuti              |

## 3. Menjalankan Frontend (Vue + PrimeVue)

```bash
cd frontend
npm install
copy .env.example .env      # Windows (gunakan "cp .env.example .env" di Mac/Linux)
```

Pastikan isi `.env` menunjuk ke alamat backend:

```
VITE_API_BASE_URL=http://localhost:8080/api
```

Jalankan mode development:

```bash
npm run dev
```

Buka `http://localhost:5173` di browser.

Build untuk produksi:

```bash
npm run build
```

Hasil build ada di folder `frontend/dist` — folder ini bisa di-hosting di web server statis mana saja (Nginx, Apache, IIS, dsb).

## Ringkasan Hak Akses per Role

| Menu                          | administrator | admin | atasan | pegawai |
|-------------------------------|:---:|:---:|:---:|:---:|
| Master data (Jabatan, Unit Kerja, dst) | CRUD | CRUD | - | - |
| Data Pegawai                  | CRUD | CRUD | lihat bawahan | lihat data sendiri |
| Jatah Cuti Tahunan             | CRUD | CRUD | lihat bawahan | lihat milik sendiri |
| Role & Akun Pengguna           | CRUD | - | - | - |
| Pengajuan Cuti — ajukan        | ✔ (atas nama siapa saja) | ✔ | ✔ (untuk diri sendiri) | ✔ |
| Pengajuan Cuti — setujui/tolak | - | - | ✔ (bawahan langsung) | - |
| Import / Export Excel          | ✔ | ✔ (kecuali menu Role & User) | - | - |

Keterhubungan atasan-bawahan diatur lewat kolom **Atasan Langsung** pada form Data Pegawai (menentukan siapa yang berhak menyetujui cuti pegawai tersebut), dan akun login dihubungkan ke data pegawai lewat kolom **Hubungkan ke Pegawai** pada form Akun Pengguna.

## Cara Kerja Import / Export Excel

Setiap tabel punya 3 tombol:

1. **Template** — mengunduh file `.xlsx` kosong berisi header kolom yang benar + satu baris contoh.
2. **Export** — mengunduh seluruh data yang ada saat ini ke `.xlsx`.
3. **Import** — mengunggah file `.xlsx` (isi sesuai format template) untuk **menambahkan** data baru secara massal. Setiap baris divalidasi; jika ada baris yang gagal (misal kolom wajib kosong, atau relasi seperti nama Jabatan belum terdaftar), sistem akan menampilkan nomor baris dan pesan errornya tanpa membatalkan baris lain yang valid.

Untuk kolom yang berupa relasi (misalnya Jabatan, Unit Kerja, Pangkat/Golongan pada data Pegawai), isi menggunakan **nama/teksnya** (bukan ID) — sistem otomatis mencocokkan ke data master yang sudah ada. Pastikan data master tersebut sudah dibuat lebih dulu sebelum import data yang mereferensikannya.

## Aturan Bisnis Penting

- **Jumlah hari cuti** dihitung otomatis dari tanggal mulai–selesai, mengurangi hari Minggu (selalu), hari Sabtu (jika Pola Hari Kerja yang dipilih adalah "5 hari kerja"), dan semua tanggal yang terdaftar di menu **Tanggal Merah**.
- **Kuota cuti tahunan** (menu Jatah Cuti Tahunan) hanya terpotong untuk pengajuan dengan Jenis Cuti yang namanya mengandung kata **"Tahunan"** (mengikuti aturan cuti PNS/pegawai di Indonesia bahwa cuti sakit/melahirkan/besar tidak memotong jatah cuti tahunan). Anda bisa menambah jenis cuti baru lewat menu Jenis Cuti.
- Pengajuan yang statusnya masih **pending** dapat diedit/dihapus oleh pegawai yang mengajukan; setelah **disetujui/ditolak**, hanya admin/administrator yang dapat mengubahnya (mengedit pengajuan yang sudah diproses akan mengembalikan statusnya ke pending untuk diproses ulang).

## Troubleshooting

- **Backend gagal konek database** → pastikan PostgreSQL sudah berjalan dan kredensial di `.env` backend sudah benar.
- **Frontend tidak bisa memuat data (network error)** → pastikan backend sudah berjalan di port yang sama dengan `VITE_API_BASE_URL` pada `.env` frontend, dan tidak ada firewall yang memblokir port 8080.
- **Login gagal** → gunakan salah satu akun demo di atas, atau reset database (`DROP DATABASE cuti_app; CREATE DATABASE cuti_app;`) lalu jalankan ulang backend agar data seed dibuat ulang.
