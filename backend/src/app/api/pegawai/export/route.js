import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS } from '@/lib/auth';
import { buatExport } from '@/lib/excel';

export async function GET(req) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const { searchParams } = new URL(req.url);
  const semua = searchParams.get('semua') === '1';
  const data = await prisma.pegawai.findMany({ where: semua ? {} : { aktif: true }, orderBy: { nama: 'asc' } });
  const buffer = buatExport(data);
  const tanggal = new Date().toISOString().slice(0, 10);
  return new Response(buffer, {
    headers: {
      'Content-Type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'Content-Disposition': `attachment; filename="DATA_PEGAWAI_AKTIF_${tanggal}.xlsx"`,
    },
  });
}
