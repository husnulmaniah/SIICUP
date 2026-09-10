import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { parseImport } from '@/lib/excel';

// POST /api/pegawai/import  (multipart: file) — upsert berdasarkan NIP + buat akun otomatis
export async function POST(req) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;

  const form = await req.formData();
  const file = form.get('file');
  if (!file || typeof file === 'string') return errJson('File Excel (.xlsx) wajib dilampirkan.');

  let rows;
  try {
    rows = parseImport(await file.arrayBuffer());
  } catch {
    return errJson('File tidak dapat dibaca. Pastikan format .xlsx sesuai template.');
  }
  if (!rows.length) return errJson('Tidak ada baris data valid (NIP & NAMA wajib terisi).');

  let dibuat = 0, diperbarui = 0;
  for (const row of rows) {
    const existing = await prisma.pegawai.findUnique({ where: { nip: row.nip } });
    if (existing) {
      await prisma.pegawai.update({ where: { id: existing.id }, data: { ...row, aktif: true } });
      diperbarui++;
    } else {
      const pegawai = await prisma.pegawai.create({ data: { ...row, aktif: true } });
      const adaAkun = await prisma.user.findUnique({ where: { username: row.nip } });
      if (!adaAkun) {
        await prisma.user.create({
          data: { username: row.nip, password: bcrypt.hashSync(row.nip, 10), role: 'PEGAWAI', nama: row.nama, pegawaiId: pegawai.id, mustChangePassword: true },
        });
      }
      dibuat++;
    }
  }
  return json({ message: `Import selesai: ${dibuat} pegawai baru, ${diperbarui} diperbarui.`, dibuat, diperbarui });
}
