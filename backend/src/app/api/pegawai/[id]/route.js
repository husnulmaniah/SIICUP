import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { cleanNip } from '@/lib/excel';
import { sanitize } from '@/lib/pegawai-util';

export async function GET(req, { params }) {
  const { response } = await requireAuth(req);
  if (response) return response;
  const pegawai = await prisma.pegawai.findUnique({ where: { id: Number(params.id) } });
  if (!pegawai) return errJson('Pegawai tidak ditemukan.', 404);
  return json(pegawai);
}

export async function PUT(req, { params }) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const id = Number(params.id);
  const body = await req.json();
  const data = sanitize(body);

  if (body.nip) {
    const nip = cleanNip(body.nip);
    const lain = await prisma.pegawai.findFirst({ where: { nip, NOT: { id } } });
    if (lain) return errJson(`NIP ${nip} sudah dipakai pegawai lain.`);
    data.nip = nip;
  }

  const pegawai = await prisma.pegawai.update({ where: { id }, data });

  // Sinkron akun: username mengikuti NIP, nama mengikuti nama pegawai
  const akun = await prisma.user.findUnique({ where: { pegawaiId: id } });
  if (akun) {
    await prisma.user.update({ where: { id: akun.id }, data: { username: pegawai.nip, nama: pegawai.nama } });
  } else if (pegawai.aktif) {
    await prisma.user.create({
      data: { username: pegawai.nip, password: bcrypt.hashSync(pegawai.nip, 10), role: 'PEGAWAI', nama: pegawai.nama, pegawaiId: id, mustChangePassword: true },
    });
  }
  return json(pegawai);
}

export async function DELETE(req, { params }) {
  const { response } = await requireAuth(req, ['ADMIN_UTAMA']);
  if (response) return response;
  const id = Number(params.id);
  await prisma.user.deleteMany({ where: { pegawaiId: id } });
  await prisma.pegawai.delete({ where: { id } });
  return json({ message: 'Pegawai dihapus.' });
}
