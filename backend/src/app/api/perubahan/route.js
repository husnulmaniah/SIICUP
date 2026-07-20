import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { FIELD_PEGAWAI } from '@/lib/pegawai-util';

const FIELD_BOLEH = FIELD_PEGAWAI;

// GET /api/perubahan — admin: semua; pegawai: miliknya
export async function GET(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const where = ADMINS.includes(user.role) ? {} : { pegawaiId: user.pegawaiId ?? -1 };
  const data = await prisma.perubahanData.findMany({
    where,
    include: { pegawai: { select: { nama: true, nip: true } } },
    orderBy: { createdAt: 'desc' },
  });
  return json(data);
}

// POST /api/perubahan — pegawai mengajukan perubahan data dirinya
export async function POST(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  if (!user.pegawaiId) return errJson('Akun Anda tidak tertaut ke data pegawai.');

  const body = await req.json();
  const dataBaru = {};
  for (const f of FIELD_BOLEH) {
    if (body.dataBaru?.[f] !== undefined && body.dataBaru[f] !== '' && body.dataBaru[f] !== null) {
      dataBaru[f] = String(body.dataBaru[f]);
    }
  }
  if (!Object.keys(dataBaru).length) return errJson('Isi minimal satu field yang ingin diubah.');

  const item = await prisma.perubahanData.create({
    data: { pegawaiId: user.pegawaiId, dataBaru: JSON.stringify(dataBaru), alasan: body.alasan || null },
  });
  return json(item, 201);
}
