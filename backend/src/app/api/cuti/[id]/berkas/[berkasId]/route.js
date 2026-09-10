import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, errJson } from '@/lib/auth';
import { bacaFile } from '@/lib/storage';

// GET /api/cuti/[id]/berkas/[berkasId]?token=...  (token via query agar bisa dibuka di tab baru)
export async function GET(req, { params }) {
  const url = new URL(req.url);
  const tokenQuery = url.searchParams.get('token');
  const reqWithToken = tokenQuery
    ? new Request(req.url, { headers: { authorization: `Bearer ${tokenQuery}` } })
    : req;

  const { user, response } = await requireAuth(reqWithToken);
  if (response) return response;

  const berkas = await prisma.berkasCuti.findUnique({
    where: { id: Number(params.berkasId) },
    include: { cuti: true },
  });
  if (!berkas || berkas.cutiId !== Number(params.id)) return errJson('Berkas tidak ditemukan.', 404);
  if (!ADMINS.includes(user.role) && berkas.cuti.pegawaiId !== user.pegawaiId) {
    return errJson('Anda tidak memiliki akses ke berkas ini.', 403);
  }

  const buffer = await bacaFile(berkas.path);
  if (!buffer) return errJson('File fisik tidak ditemukan di server.', 404);
  return new Response(buffer, {
    headers: {
      'Content-Type': berkas.mimeType || 'application/octet-stream',
      'Content-Disposition': `inline; filename="${encodeURIComponent(berkas.namaFile)}"`,
    },
  });
}
