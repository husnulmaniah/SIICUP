import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { requireAuth, json, errJson } from '@/lib/auth';
import { cekKebijakanPassword } from '@/lib/password';

// PUT /api/users/[id] — reset password (admin utama). Akun pegawai tanpa input = kembali ke NIP.
// Akun yang direset selalu diwajibkan mengganti password saat login berikutnya.
export async function PUT(req, { params }) {
  const { response } = await requireAuth(req, ['ADMIN_UTAMA']);
  if (response) return response;
  const id = Number(params.id);
  const { passwordBaru } = await req.json();
  const target = await prisma.user.findUnique({ where: { id } });
  if (!target) return errJson('Akun tidak ditemukan.', 404);

  let pass = passwordBaru;
  if (!pass) {
    if (target.role !== 'PEGAWAI') return errJson('Isi password baru untuk akun admin.');
    pass = target.username; // default kembali ke NIP, tapi wajib diganti saat login
  } else {
    const salah = cekKebijakanPassword(pass);
    if (salah) return errJson(salah);
  }
  await prisma.user.update({
    where: { id },
    data: { password: bcrypt.hashSync(pass, 10), mustChangePassword: true, gagalLogin: 0, terkunciSampai: null },
  });
  return json({ message: `Password akun ${target.username} direset. Pengguna wajib mengganti password saat login.` });
}

// PATCH /api/users/[id] — ubah role (admin utama): PEGAWAI <-> ADMIN_PEMBANTU <-> ADMIN_UTAMA
export async function PATCH(req, { params }) {
  const { user, response } = await requireAuth(req, ['ADMIN_UTAMA']);
  if (response) return response;
  const id = Number(params.id);
  const { role } = await req.json();
  if (!['PEGAWAI', 'ADMIN_PEMBANTU', 'ADMIN_UTAMA'].includes(role)) return errJson('Role tidak dikenal.');
  if (id === user.id) return errJson('Anda tidak dapat mengubah role akun sendiri.');

  const target = await prisma.user.findUnique({ where: { id } });
  if (!target) return errJson('Akun tidak ditemukan.', 404);
  if (role === 'PEGAWAI' && !target.pegawaiId) {
    return errJson('Akun ini tidak tertaut ke data pegawai sehingga tidak bisa dijadikan role PEGAWAI.');
  }
  if (target.role === 'ADMIN_UTAMA' && role !== 'ADMIN_UTAMA') {
    const jumlahUtama = await prisma.user.count({ where: { role: 'ADMIN_UTAMA' } });
    if (jumlahUtama <= 1) return errJson('Tidak bisa menurunkan role: harus tersisa minimal satu Admin Utama.');
  }

  const updated = await prisma.user.update({
    where: { id },
    data: { role },
    select: { id: true, username: true, nama: true, role: true },
  });
  return json({ message: `Role ${updated.username} kini ${role.replace('_', ' ')}.`, user: updated });
}

// DELETE /api/users/[id]
export async function DELETE(req, { params }) {
  const { user, response } = await requireAuth(req, ['ADMIN_UTAMA']);
  if (response) return response;
  const id = Number(params.id);
  if (id === user.id) return errJson('Anda tidak dapat menghapus akun sendiri.');
  const target = await prisma.user.findUnique({ where: { id } });
  if (!target) return errJson('Akun tidak ditemukan.', 404);
  if (target.role === 'ADMIN_UTAMA') {
    const jumlahUtama = await prisma.user.count({ where: { role: 'ADMIN_UTAMA' } });
    if (jumlahUtama <= 1) return errJson('Tidak bisa menghapus Admin Utama terakhir.');
  }
  await prisma.user.delete({ where: { id } });
  return json({ message: 'Akun dihapus.' });
}
