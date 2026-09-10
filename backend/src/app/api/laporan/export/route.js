import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS } from '@/lib/auth';
import { buatLaporan } from '@/lib/laporan';

export async function GET(req) {
  const url = new URL(req.url);
  const tokenQuery = url.searchParams.get('token');
  const reqWithToken = tokenQuery
    ? new Request(req.url, { headers: { authorization: `Bearer ${tokenQuery}` } })
    : req;
  const { response } = await requireAuth(reqWithToken, ADMINS);
  if (response) return response;

  const sp = url.searchParams;
  const where = {};
  if (sp.get('dari') || sp.get('sampai')) {
    where.tanggalMulai = {};
    if (sp.get('dari')) where.tanggalMulai.gte = new Date(sp.get('dari'));
    if (sp.get('sampai')) where.tanggalMulai.lte = new Date(sp.get('sampai') + 'T23:59:59');
  }
  if (sp.get('status') && sp.get('status') !== 'SEMUA') where.status = sp.get('status');
  if (sp.get('jenis') && sp.get('jenis') !== 'SEMUA') where.jenisCuti = sp.get('jenis');

  const data = await prisma.pengajuanCuti.findMany({
    where,
    include: { pegawai: { select: { nama: true, nip: true, jabatan: true, tempatTugas: true, unor: true, golAkhir: true } } },
    orderBy: { tanggalMulai: 'asc' },
  });

  const buffer = buatLaporan(data, {
    dari: sp.get('dari'), sampai: sp.get('sampai'), status: sp.get('status'), jenis: sp.get('jenis'),
  });
  const tanggal = new Date().toISOString().slice(0, 10);
  return new Response(buffer, {
    headers: {
      'Content-Type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'Content-Disposition': `attachment; filename="LAPORAN_REKAP_CUTI_${tanggal}.xlsx"`,
    },
  });
}
