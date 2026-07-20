import jwt from 'jsonwebtoken';
import { prisma } from './prisma';

const SECRET = process.env.JWT_SECRET || 'sicuti-secret-ganti-di-produksi';

export function signToken(user) {
  return jwt.sign(
    { id: user.id, username: user.username, role: user.role, nama: user.nama, pegawaiId: user.pegawaiId },
    SECRET,
    { expiresIn: '12h' }
  );
}

export function json(data, status = 200) {
  return Response.json(data, { status });
}

export function errJson(message, status = 400) {
  return Response.json({ error: message }, { status });
}

/**
 * Ambil user dari header Authorization. roles: array role yang diizinkan (kosong = semua yang login).
 * Return { user } atau { response } (error siap dikirim).
 */
export async function requireAuth(req, roles = []) {
  const header = req.headers.get('authorization') || '';
  const token = header.startsWith('Bearer ') ? header.slice(7) : null;
  if (!token) return { response: errJson('Tidak terautentikasi. Silakan login.', 401) };
  let payload;
  try {
    payload = jwt.verify(token, SECRET);
  } catch {
    return { response: errJson('Sesi berakhir. Silakan login kembali.', 401) };
  }
  const user = await prisma.user.findUnique({ where: { id: payload.id } });
  if (!user) return { response: errJson('Akun tidak ditemukan.', 401) };
  if (roles.length && !roles.includes(user.role)) {
    return { response: errJson('Anda tidak memiliki akses untuk tindakan ini.', 403) };
  }
  return { user };
}

export const ADMINS = ['ADMIN_UTAMA', 'ADMIN_PEMBANTU'];
