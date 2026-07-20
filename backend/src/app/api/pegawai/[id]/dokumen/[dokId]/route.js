import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { bacaFile, hapusFile } from '@/lib/storage';

const bolehAkses = (user, pegawaiId) => ADMINS.includes(user.role) || user.pegawaiId === pegawaiId;

// GET /api/pegawai/[id]/dokumen/[dokId]?token=... — lihat/unduh (token via query agar bisa dibuka tab baru)
export async function GET(req, { params }) {
  const url = new URL(req.url);
  const tokenQuery = url.searchParams.get('token');
  const reqWithToken = tokenQuery
    ? new Request(req.url, { headers: { authorization: `Bearer ${tokenQuery}` } })
    : req;
  const { user, response } = await requireAuth(reqWithToken);
  if (response) return response;

  const dok = await prisma.dokumenPegawai.findUnique({ where: { id: Number(params.dokId) } });
  if (!dok || dok.pegawaiId !== Number(params.id)) return errJson('Dokumen tidak ditemukan.', 404);
  if (!bolehAkses(user, dok.pegawaiId)) return errJson('Anda tidak memiliki akses.', 403);

  const buffer = await bacaFile(dok.path);
  if (!buffer) return errJson('File fisik tidak ditemukan di server.', 404);
  return new Response(buffer, {
    headers: {
      'Content-Type': dok.mimeType || 'application/octet-stream',
      'Content-Disposition': `inline; filename="${encodeURIComponent(dok.namaFile)}"`,
    },
  });
}

// DELETE /api/pegawai/[id]/dokumen/[dokId]
export async function DELETE(req, { params }) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const dok = await prisma.dokumenPegawai.findUnique({ where: { id: Number(params.dokId) } });
  if (!dok || dok.pegawaiId !== Number(params.id)) return errJson('Dokumen tidak ditemukan.', 404);
  if (!bolehAkses(user, dok.pegawaiId)) return errJson('Anda tidak memiliki akses.', 403);
  await hapusFile(dok.path);
  await prisma.dokumenPegawai.delete({ where: { id: dok.id } });
  return json({ message: 'Dokumen dihapus.' });
}
