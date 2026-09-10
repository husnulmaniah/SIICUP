import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { hitungSaldoTahunan } from '@/lib/cuti-tahunan';

// GET /api/cuti/sisa?pegawaiId= — sisa cuti tahunan (FIFO) untuk info form pengajuan.
// Menghitung pengajuan DIAJUKAN + DISETUJUI agar jatah tidak terpakai ganda.
export async function GET(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const { searchParams } = new URL(req.url);
  let pegawaiId = Number(searchParams.get('pegawaiId')) || null;
  if (!ADMINS.includes(user.role)) pegawaiId = user.pegawaiId;
  if (!pegawaiId) return errJson('Pegawai tidak ditentukan.');

  const tahun = new Date().getUTCFullYear();
  const hasil = await hitungSaldoTahunan(prisma, pegawaiId, tahun, ['DIAJUKAN', 'DISETUJUI']);
  const rincian = Object.entries(hasil.saldo)
    .map(([t, sisa]) => ({ tahun: Number(t), sisa }))
    .sort((a, b) => a.tahun - b.tahun);
  return json({ firstYear: hasil.firstYear, totalSisa: hasil.totalSisa, rincian });
}
