import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { requireAuth, json, errJson } from '@/lib/auth';
import { cekKebijakanPassword } from '@/lib/password';

// GET /api/users — daftar akun (admin utama)
export async function GET(req) {
  const { response } = await requireAuth(req, ['ADMIN_UTAMA']);
  if (response) return response;
  const users = await prisma.user.findMany({
    select: { id: true, username: true, nama: true, role: true, pegawaiId: true, createdAt: true },
    orderBy: [{ role: 'asc' }, { nama: 'asc' }],
  });
  return json(users);
}

// POST /api/users — buat akun admin pembantu / admin utama baru
export async function POST(req) {
  const { response } = await requireAuth(req, ['ADMIN_UTAMA']);
  if (response) return response;
  const { username, password, nama, role } = await req.json();
  if (!username || !password || !nama) return errJson('Username, password, dan nama wajib diisi.');
  if (!['ADMIN_UTAMA', 'ADMIN_PEMBANTU'].includes(role)) return errJson('Role harus ADMIN_UTAMA atau ADMIN_PEMBANTU.');
  const salahKebijakan = cekKebijakanPassword(password);
  if (salahKebijakan) return errJson(salahKebijakan);
  const ada = await prisma.user.findUnique({ where: { username } });
  if (ada) return errJson('Username sudah dipakai.');
  const user = await prisma.user.create({
    data: { username, password: bcrypt.hashSync(password, 10), nama, role },
    select: { id: true, username: true, nama: true, role: true },
  });
  return json(user, 201);
}
