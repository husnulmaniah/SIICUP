import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json } from '@/lib/auth';

function buildWhere(searchParams) {
  const where = {};
  const dari = searchParams.get('dari');
  const sampai = searchParams.get('sampai');
  const status = searchParams.get('status');
  const jenis = searchParams.get('jenis');
  if (dari || sampai) {
    where.tanggalMulai = {};
    if (dari) where.tanggalMulai.gte = new Date(dari);
    if (sampai) where.tanggalMulai.lte = new Date(sampai + 'T23:59:59');
  }
  if (status && status !== 'SEMUA') where.status = status;
  if (jenis && jenis !== 'SEMUA') where.jenisCuti = jenis;
  return where;
}

// GET /api/laporan?dari=YYYY-MM-DD&sampai=&status=&jenis= — data rekap + ringkasan (admin)
export async function GET(req) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const { searchParams } = new URL(req.url);
  const where = buildWhere(searchParams);

  const data = await prisma.pengajuanCuti.findMany({
    where,
    include: { pegawai: { select: { nama: true, nip: true, jabatan: true, tempatTugas: true, unor: true, golAkhir: true } } },
    orderBy: { tanggalMulai: 'asc' },
  });

  const ringkasan = {
    total: data.length,
    totalHari: data.reduce((a, c) => a + c.lamaCuti, 0),
    perStatus: {},
    perJenis: {},
  };
  for (const c of data) {
    ringkasan.perStatus[c.status] = (ringkasan.perStatus[c.status] || 0) + 1;
    ringkasan.perJenis[c.jenisCuti] = (ringkasan.perJenis[c.jenisCuti] || 0) + 1;
  }
  return json({ data, ringkasan });
}

