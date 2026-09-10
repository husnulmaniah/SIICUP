# Panduan Deploy: Railway (Backend) + Vercel (Frontend)

Repo ini adalah **monorepo** dengan dua bagian yang di-deploy terpisah:

| Bagian | Root Directory | Di-deploy ke |
|---|---|---|
| `backend/` | `backend` | **Railway** (Go + PostgreSQL) |
| `frontend/` | `frontend` | **Vercel** (Vue 3 + Vite, situs yang dibuka pengguna) |

Deploy backend dulu supaya Anda punya URL API-nya, baru deploy frontend dengan URL itu.

---

## 1. Deploy Backend ke Railway

1. Buka [railway.app](https://railway.app), login dengan akun GitHub Anda.
2. **New Project → Deploy from GitHub repo** → pilih repo `husnulmaniah/SIICUP`.
3. Setelah project dibuat, buka tab **Settings** pada service yang terbuat, isi **Root Directory** = `backend`.
4. Railway akan otomatis mendeteksi ini project Go dan membaca `backend/railway.toml` (build: `go build -o bin/server ./cmd/server`, start: `./bin/server`).
5. Tambahkan database: klik **+ New → Database → PostgreSQL** di project yang sama. Railway otomatis membuat variabel `DATABASE_URL` yang bisa dipakai service backend.
6. Buka tab **Variables** pada service backend, tambahkan:
   | Nama | Nilai |
   |---|---|
   | `DATABASE_URL` | klik **Add Reference** → pilih `DATABASE_URL` dari service Postgres (biar otomatis terisi & ter-update) |
   | `JWT_SECRET` | teks acak panjang, contoh hasil `openssl rand -hex 32` |
   | `FRONTEND_ORIGINS` | isi setelah frontend di Vercel jadi, contoh `https://siicup.vercel.app` (boleh dikosongkan dulu di awal) |
7. **Deploy**. Setelah selesai, buka tab **Settings → Networking → Generate Domain** untuk mendapatkan URL publik, misalnya `https://siicup-backend.up.railway.app`.
8. Cek berhasil dengan membuka `https://siicup-backend.up.railway.app/api/health` di browser — harus muncul `{"status":"ok"}`. Saat pertama kali deploy, tabel & data awal (akun demo) otomatis dibuat.

## 2. Deploy Frontend ke Vercel

1. Buka [vercel.com](https://vercel.com) → project yang sudah ada (`siicup-frontend`) atau **Add New → Project** jika mau bikin baru → pilih repo `husnulmaniah/SIICUP`.
2. Di **Project Settings → General → Root Directory**, isi `frontend`. Framework Preset otomatis terdeteksi **Vite**.
3. Di **Project Settings → Environment Variables**, tambahkan:
   | Nama | Nilai |
   |---|---|
   | `VITE_API_BASE_URL` | URL backend Railway + `/api`, contoh `https://siicup-backend.up.railway.app/api` |
4. **Deploy** (atau **Redeploy** jika project sudah ada sebelumnya — pastikan klik redeploy setelah mengubah Root Directory/env var, perubahan tidak berlaku otomatis ke deployment yang sudah jalan).
5. Setelah selesai, buka URL Vercel-nya (misalnya `https://siicup-frontend.vercel.app`) — halaman login harus muncul.

## 3. Hubungkan Balik CORS

Setelah tahu URL final Vercel, kembali ke **Railway → service backend → Variables**, isi `FRONTEND_ORIGINS` dengan URL Vercel tersebut (boleh lebih dari satu, pisahkan koma jika ada preview URL juga), lalu klik **Deploy** lagi supaya perubahan variabel diterapkan.

## 4. Selesai — Coba Login

Buka URL Vercel Anda, login dengan salah satu akun demo (password semua `admin123`): `administrator`, `admin`, `andi` (atasan), `siti` (pegawai).

> **Keamanan**: akun demo di atas otomatis dibuat saat database masih kosong. Setelah aplikasi live, segera login sebagai `administrator` → menu **Akun Pengguna** → ubah password setiap akun demo (atau hapus dan buat akun asli instansi Anda).

## Update Berikutnya (setelah setup awal)

Setiap kali Anda `git push` ke branch `main`, **Railway dan Vercel otomatis build & deploy ulang** masing-masing bagiannya — tidak perlu mengulang langkah-langkah di atas, cukup langkah 1–4 sekali di awal.

## Troubleshooting

- **Frontend tidak bisa memuat data / error CORS di console browser** → pastikan `VITE_API_BASE_URL` di Vercel sudah benar dan sudah di-redeploy, dan `FRONTEND_ORIGINS` di Railway sudah termasuk URL Vercel Anda (lalu redeploy backend juga).
- **Railway build gagal** → cek tab **Deployments → View Logs**, pastikan Root Directory service memang `backend`.
- **Vercel build gagal / halaman putih** → cek Root Directory memang `frontend`, dan environment variable `VITE_API_BASE_URL` sudah diisi sebelum build (isi env var lalu **Redeploy**, bukan hanya save).
