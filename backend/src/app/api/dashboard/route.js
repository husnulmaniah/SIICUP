import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json } from '@/lib/auth';

export async function GET(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const isAdmin = ADMINS.includes(user.role);
  const whereCuti = isAdmin ? {} : { pegawaiId: user.pegawaiId ?? -1 };

  const [pegawaiAktif, totalCuti, diajukan, disetujui, dikembalikan, ditolak, perubahanMenunggu, terbaru] =
    await Promise.all([
      prisma.pegawai.count({ where: { aktif: true } }),
      prisma.pengajuanCuti.count({ where: whereCuti }),
      prisma.pengajuanCuti.count({ where: { ...whereCuti, status: 'DIAJUKAN' } }),
      prisma.pengajuanCuti.count({ where: { ...whereCuti, status: 'DISETUJUI' } }),
      prisma.pengajuanCuti.count({ where: { ...whereCuti, status: 'DIKEMBALIKAN' } }),
      prisma.pengajuanCuti.count({ where: { ...whereCuti, status: 'DITOLAK' } }),
      isAdmin ? prisma.perubahanData.count({ where: { status: 'DIAJUKAN' } }) : Promise.resolve(0),
      prisma.pengajuanCuti.findMany({
        where: whereCuti,
        include: { pegawai: { select: { nama: true, nip: true } } },
        orderBy: { createdAt: 'desc' },
        take: 8,
      }),
    ]);

  return json({ pegawaiAktif, totalCuti, diajukan, disetujui, dikembalikan, ditolak, perubahanMenunggu, terbaru });
}
