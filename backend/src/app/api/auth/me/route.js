import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { requireAuth, json, errJson } from '@/lib/auth';
import { cekKebijakanPassword } from '@/lib/password';

export async function GET(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const pegawai = user.pegawaiId ? await prisma.pegawai.findUnique({ where: { id: user.pegawaiId } }) : null;
  return json({
    id: user.id, username: user.username, nama: user.nama, role: user.role,
    mustChangePassword: user.mustChangePassword, loginTerakhir: user.loginTerakhir, pegawai,
  });
}

// Ganti password sendiri
export async function PUT(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const { passwordLama, passwordBaru } = await req.json();
  const salahKebijakan = cekKebijakanPassword(passwordBaru);
  if (salahKebijakan) return errJson(salahKebijakan);
  if (passwordBaru === user.username) return errJson('Password baru tidak boleh sama dengan username/NIP.');
  if (!bcrypt.compareSync(passwordLama || '', user.password)) return errJson('Password lama salah.');
  await prisma.user.update({
    where: { id: user.id },
    data: { password: bcrypt.hashSync(passwordBaru, 10), mustChangePassword: false },
  });
  return json({ message: 'Password berhasil diubah.' });
}
