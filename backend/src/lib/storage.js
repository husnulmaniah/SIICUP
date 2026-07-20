import fs from 'fs';
import path from 'path';
import crypto from 'crypto';

// Lapisan penyimpanan berkas:
// - Jika BLOB_READ_WRITE_TOKEN diset (Vercel) -> Vercel Blob, kolom `path` berisi URL https.
// - Jika tidak (lokal/VPS) -> file disimpan di ./uploads, kolom `path` berisi nama file.
// Fungsi baca/hapus mengenali kedua bentuk, sehingga data lama tetap terbaca.

const pakaiBlob = () => !!process.env.BLOB_READ_WRITE_TOKEN;
const UPLOAD_DIR = path.join(process.cwd(), 'uploads');

const namaAcak = (prefix, ext) => `${prefix}_${crypto.randomBytes(6).toString('hex')}${ext || '.bin'}`;

/** Simpan File (dari formData) -> return path/url untuk disimpan di DB */
export async function simpanFile(prefix, file) {
  const ext = path.extname(file.name) || '.bin';
  const nama = namaAcak(prefix, ext);
  if (pakaiBlob()) {
    const { put } = await import('@vercel/blob');
    const blob = await put(`sicuti/${nama}`, file, { access: 'public', addRandomSuffix: true });
    return blob.url;
  }
  fs.mkdirSync(UPLOAD_DIR, { recursive: true });
  fs.writeFileSync(path.join(UPLOAD_DIR, nama), Buffer.from(await file.arrayBuffer()));
  return nama;
}

/** Salin berkas yang sudah tersimpan (snapshot) -> return path/url baru, atau null jika sumber hilang */
export async function salinFile(sumber, prefix) {
  try {
    const ext = path.extname((sumber || '').split('?')[0]) || '.bin';
    const nama = namaAcak(prefix, ext);
    if (sumber.startsWith('http')) {
      const { copy } = await import('@vercel/blob');
      const blob = await copy(sumber, `sicuti/${nama}`, { access: 'public', addRandomSuffix: true });
      return blob.url;
    }
    const asal = path.join(UPLOAD_DIR, sumber);
    if (!fs.existsSync(asal)) return null;
    fs.mkdirSync(UPLOAD_DIR, { recursive: true });
    fs.copyFileSync(asal, path.join(UPLOAD_DIR, nama));
    return nama;
  } catch {
    return null;
  }
}

/** Baca berkas -> Buffer, atau null bila tidak ditemukan */
export async function bacaFile(p) {
  try {
    if (p.startsWith('http')) {
      const res = await fetch(p);
      if (!res.ok) return null;
      return Buffer.from(await res.arrayBuffer());
    }
    const f = path.join(UPLOAD_DIR, p);
    if (!fs.existsSync(f)) return null;
    return fs.readFileSync(f);
  } catch {
    return null;
  }
}

/** Hapus berkas (abaikan error agar penghapusan data tidak gagal karena file hilang) */
export async function hapusFile(p) {
  try {
    if (p.startsWith('http')) {
      const { del } = await import('@vercel/blob');
      await del(p);
    } else {
      fs.unlinkSync(path.join(UPLOAD_DIR, p));
    }
  } catch {}
}
