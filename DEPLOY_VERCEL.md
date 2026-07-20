# Panduan Deploy SICUTI ke Vercel

Arsitektur di Vercel terdiri dari **dua project** dari satu repository:

| Project | Root Directory | Peran |
|---|---|---|
| `sicuti-api` | `backend/` | Next.js API + Prisma (PostgreSQL) + Vercel Blob |
| `sicuti-app` | `frontend/` | Vue 3 (Vite) — situs yang dibuka pengguna |

Berkas upload disimpan di **Vercel Blob**, database di **PostgreSQL** (Neon / Vercel Postgres / Supabase). Jika `BLOB_READ_WRITE_TOKEN` tidak diset (misal saat jalan di VPS sendiri), sistem otomatis kembali menyimpan berkas ke folder `./uploads` — jadi satu kode berjalan di kedua lingkungan.

---

## 1. Siapkan Database PostgreSQL

Paling mudah dengan **Neon** (gratis) atau **Vercel Postgres**:

1. Buat database baru bernama `sicuti`.
2. Salin connection string, contohnya:
   `postgresql://user:password@ep-xxx.aws.neon.tech/sicuti?sslmode=require`

## 2. Push Kode ke GitHub

```bash
cd sicuti
git init
git add .
git commit -m "SICUTI"
git remote add origin https://github.com/USERNAME/sicuti.git
git push -u origin main
```

> Pastikan `backend/.env` **tidak ikut ter-push** (tambahkan ke `.gitignore`). Gunakan `.env.example` sebagai acuan.

## 3. Deploy Backend (`sicuti-api`)

1. Di dashboard Vercel: **Add New → Project** → pilih repo → **Root Directory: `backend`**.
2. Framework terdeteksi otomatis sebagai Next.js. Ubah **Build Command** menjadi:
   ```
   npm run vercel-build
   ```
   (menjalankan `prisma generate && prisma migrate deploy && next build` — migrasi otomatis tiap deploy)
3. Isi **Environment Variables**:
   | Nama | Nilai |
   |---|---|
   | `DATABASE_URL` | connection string **pooler** Neon (host `-pooler`) |
   | `DIRECT_URL` | connection string **langsung** Neon (tanpa `-pooler`, untuk migrasi) |
   | `JWT_SECRET` | string acak panjang (mis. hasil `openssl rand -hex 32`) |
4. **Deploy**, lalu buka tab **Storage → Create → Blob** pada project ini. Vercel otomatis menambahkan `BLOB_READ_WRITE_TOKEN` ke environment — **redeploy sekali** agar token terbaca.
5. Catat URL backend, mis. `https://sicuti-api.vercel.app`.

### Membuat migrasi pertama & akun awal (dijalankan dari komputer Anda)

Karena repositori belum memiliki folder migrasi Postgres, buat sekali dari lokal:

```bash
cd backend
npm install
# arahkan ke database produksi
echo 'DATABASE_URL="postgresql://...connection-string..."' > .env
npx prisma migrate dev --name init   # membuat folder prisma/migrations + menerapkan ke DB
node prisma/seed.js                  # akun admin/operator + pegawai contoh
git add prisma/migrations && git commit -m "migrasi awal" && git push
```

Deploy berikutnya cukup mengandalkan `prisma migrate deploy` yang sudah ada di build command.

## 4. Deploy Frontend (`sicuti-app`)

1. **Add New → Project** → repo yang sama → **Root Directory: `frontend`** (framework: Vite).
2. Edit `frontend/vercel.json`: ganti `GANTI-DENGAN-URL-BACKEND.vercel.app` dengan URL backend dari langkah 3.5, lalu commit & push.
   ```json
   { "source": "/api/:path*", "destination": "https://sicuti-api.vercel.app/api/:path*" }
   ```
   Rewrite ini membuat frontend dan API tampak satu domain (tanpa masalah CORS). Alternatifnya, kosongkan rewrite dan isi env `VITE_API_URL=https://sicuti-api.vercel.app/api`.
3. **Deploy**. Situs siap dibuka, login `admin / admin123` — segera ganti password (sistem juga akan memaksa jika masih default).

## 5. Uji Cepat Setelah Deploy

- Login admin → menu Data Pegawai Aktif → **Unduh Template**, **Import Excel**, **Export Excel**.
- Ajukan cuti dengan upload berkas → buka detail → **Lihat/Unduh** berkas (harus tampil; tersimpan di Vercel Blob).
- Setujui → **Preview & Unduh Surat** (Word harus memuat logo kop).
- Menu Laporan → Export Excel.

## Catatan

- **Ukuran upload**: request body serverless Vercel dibatasi ±4,5 MB. Batas 5 MB/berkas cuti masih pas-pasan; jika sering gagal, kecilkan batas di kode atau unggah berkas satu per satu (form sudah mendukung per-jenis).
- **Region**: pilih region Vercel & database yang sama/berdekatan (mis. Singapore `sin1` + Neon Singapore) agar cepat dari Indonesia.
- **VPS/lokal**: tetap didukung — isi `DATABASE_URL` ke Postgres lokal, kosongkan `BLOB_READ_WRITE_TOKEN`, berkas tersimpan di `./uploads` seperti semula.
