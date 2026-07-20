import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';

const AKSI_VALID = { SETUJUI: 'DISETUJUI', KEMBALIKAN: 'DIKEMBALIKAN', TOLAK: 'DITOLAK' };

// POST /api/cuti/[id]/aksi  { aksi: SETUJUI|KEMBALIKAN|TOLAK, catatan }
export async function POST(req, { params }) {
  const { user, response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const id = Number(params.id);
  const { aksi, catatan } = await req.json();
  const statusBaru = AKSI_VALID[aksi];
  if (!statusBaru) return errJson('Aksi tidak dikenal. Gunakan SETUJUI, KEMBALIKAN, atau TOLAK.');

  const cuti = await prisma.pengajuanCuti.findUnique({ where: { id } });
  if (!cuti) return errJson('Pengajuan tidak ditemukan.', 404);
  if (cuti.status !== 'DIAJUKAN') return errJson(`Pengajuan berstatus ${cuti.status}, tidak dapat diproses lagi.`);
  if ((aksi === 'KEMBALIKAN' || aksi === 'TOLAK') && !catatan?.trim()) {
    return errJson('Catatan wajib diisi saat mengembalikan atau menolak pengajuan.');
  }

  const updated = await prisma.pengajuanCuti.update({
    where: { id },
    data: { status: statusBaru, catatan: catatan?.trim() || null, diprosesOleh: user.nama },
  });
  await prisma.riwayatCuti.create({ data: { cutiId: id, aksi: statusBaru, oleh: user.nama, catatan: catatan?.trim() || null } });
  return json(updated);
}
