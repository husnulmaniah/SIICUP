import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { cleanNip } from '@/lib/excel';
import { sanitize } from '@/lib/pegawai-util';

// GET /api/pegawai?aktif=1 — daftar pegawai (semua role login boleh melihat daftar aktif; kelola hanya admin)
export async function GET(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const { searchParams } = new URL(req.url);
  const semua = searchParams.get('semua') === '1' && ADMINS.includes(user.role);
  const data = await prisma.pegawai.findMany({
    where: semua ? {} : { aktif: true },
    orderBy: { nama: 'asc' },
  });
  return json(data);
}

// POST /api/pegawai — tambah pegawai (admin), otomatis buat akun username/password = NIP
export async function POST(req) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const body = await req.json();
  const nip = cleanNip(body.nip);
  if (!nip || !body.nama) return errJson('NIP dan nama wajib diisi.');

  const ada = await prisma.pegawai.findUnique({ where: { nip } });
  if (ada) return errJson(`NIP ${nip} sudah terdaftar.`);

  const pegawai = await prisma.pegawai.create({ data: { ...sanitize(body), nip } });
  await prisma.user.create({
    data: { username: nip, password: bcrypt.hashSync(nip, 10), role: 'PEGAWAI', nama: pegawai.nama, pegawaiId: pegawai.id, mustChangePassword: true },
  });
  return json(pegawai, 201);
}

