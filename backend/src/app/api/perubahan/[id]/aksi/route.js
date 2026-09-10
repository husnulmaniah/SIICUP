import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';

// POST /api/perubahan/[id]/aksi  { aksi: SETUJUI|TOLAK, catatan }
export async function POST(req, { params }) {
  const { user, response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const id = Number(params.id);
  const { aksi, catatan } = await req.json();
  if (!['SETUJUI', 'TOLAK'].includes(aksi)) return errJson('Aksi tidak dikenal.');

  const item = await prisma.perubahanData.findUnique({ where: { id } });
  if (!item) return errJson('Usulan tidak ditemukan.', 404);
  if (item.status !== 'DIAJUKAN') return errJson('Usulan sudah diproses.');
  if (aksi === 'TOLAK' && !catatan?.trim()) return errJson('Catatan wajib diisi saat menolak.');

  if (aksi === 'SETUJUI') {
    const dataBaru = JSON.parse(item.dataBaru);
    await prisma.pegawai.update({ where: { id: item.pegawaiId }, data: dataBaru });
    if (dataBaru.nama) {
      await prisma.user.updateMany({ where: { pegawaiId: item.pegawaiId }, data: { nama: dataBaru.nama } });
    }
  }

  const updated = await prisma.perubahanData.update({
    where: { id },
    data: { status: aksi === 'SETUJUI' ? 'DISETUJUI' : 'DITOLAK', catatan: catatan?.trim() || null, diprosesOleh: user.nama },
  });
  return json(updated);
}
