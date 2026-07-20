import bcrypt from 'bcryptjs';
import { prisma } from '@/lib/prisma';
import { signToken, json, errJson } from '@/lib/auth';
import { MAKS_GAGAL_LOGIN, DURASI_KUNCI_MENIT } from '@/lib/password';

export async function POST(req) {
  const { username, password } = await req.json();
  if (!username || !password) return errJson('Username dan password wajib diisi.');

  const uname = String(username).replace(/\s+/g, '');
  const user = await prisma.user.findUnique({ where: { username: uname }, include: { pegawai: true } });
  // Pesan error dibuat sama agar tidak membocorkan apakah username terdaftar
  if (!user) return errJson('Username atau password salah.', 401);

  // Akun terkunci sementara?
  if (user.terkunciSampai && new Date(user.terkunciSampai) > new Date()) {
    const menit = Math.ceil((new Date(user.terkunciSampai) - new Date()) / 60000);
    return errJson(`Akun terkunci sementara karena terlalu banyak percobaan gagal. Coba lagi dalam ${menit} menit.`, 429);
  }

  // Akun pegawai hanya bisa login jika pegawai masih berstatus aktif
  if (user.role === 'PEGAWAI' && user.pegawai && !user.pegawai.aktif) {
    return errJson('Akun Anda nonaktif karena pegawai tidak lagi berstatus aktif.', 403);
  }

  const cocok = bcrypt.compareSync(password, user.password);
  if (!cocok) {
    const gagal = user.gagalLogin + 1;
    const kunci = gagal >= MAKS_GAGAL_LOGIN;
    await prisma.user.update({
      where: { id: user.id },
      data: {
        gagalLogin: kunci ? 0 : gagal,
        terkunciSampai: kunci ? new Date(Date.now() + DURASI_KUNCI_MENIT * 60000) : null,
      },
    });
    if (kunci) {
      return errJson(`Terlalu banyak percobaan gagal. Akun dikunci ${DURASI_KUNCI_MENIT} menit.`, 429);
    }
    return errJson(`Username atau password salah. Sisa percobaan: ${MAKS_GAGAL_LOGIN - gagal}.`, 401);
  }

  // Sukses: reset counter, catat waktu login.
  // Deteksi password default (= NIP/username) -> wajib ganti password.
  const masihDefault = bcrypt.compareSync(user.username, user.password);
  const mustChangePassword = user.mustChangePassword || masihDefault;
  await prisma.user.update({
    where: { id: user.id },
    data: { gagalLogin: 0, terkunciSampai: null, loginTerakhir: new Date(), mustChangePassword },
  });

  return json({
    token: signToken(user),
    user: {
      id: user.id, username: user.username, nama: user.nama, role: user.role,
      pegawaiId: user.pegawaiId, mustChangePassword,
    },
  });
}
