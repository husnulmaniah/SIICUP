import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { JENIS_DOKUMEN } from '@/lib/dokumen-config';
import { simpanFile, hapusFile } from '@/lib/storage';

const MAX_SIZE = 10 * 1024 * 1024;
const ALLOWED = ['application/pdf', 'image/jpeg', 'image/png', 'image/webp'];

const bolehAkses = (user, pegawaiId) => ADMINS.includes(user.role) || user.pegawaiId === pegawaiId;

// GET /api/pegawai/[id]/dokumen — daftar dokumen kepegawaian
export async function GET(req, { params }) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const pegawaiId = Number(params.id);
  if (!bolehAkses(user, pegawaiId)) return errJson('Anda tidak memiliki akses.', 403);
  const dokumen = await prisma.dokumenPegawai.findMany({ where: { pegawaiId }, orderBy: { jenis: 'asc' } });
  return json(dokumen);
}

// POST /api/pegawai/[id]/dokumen — multipart { jenis, file }; upload/replace (upsert per jenis)
export async function POST(req, { params }) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const pegawaiId = Number(params.id);
  if (!bolehAkses(user, pegawaiId)) return errJson('Anda tidak memiliki akses.', 403);

  const pegawai = await prisma.pegawai.findUnique({ where: { id: pegawaiId } });
  if (!pegawai) return errJson('Pegawai tidak ditemukan.', 404);

  const form = await req.formData();
  const jenis = String(form.get('jenis') || '');
  if (!JENIS_DOKUMEN.some((d) => d.kode === jenis)) return errJson('Jenis dokumen tidak dikenal.');
  const file = form.get('file');
  if (!file || typeof file === 'string' || !file.size) return errJson('File wajib dilampirkan.');
  if (file.size > MAX_SIZE) return errJson(`Ukuran "${file.name}" melebihi 10 MB.`);
  if (!ALLOWED.includes(file.type)) return errJson(`Format "${file.name}" tidak didukung. Gunakan PDF/JPG/PNG.`);

  const namaSimpan = await simpanFile(`peg${pegawaiId}_${jenis}`, file);

  // Hapus file lama jika mengganti
  const lama = await prisma.dokumenPegawai.findUnique({ where: { pegawaiId_jenis: { pegawaiId, jenis } } });
  if (lama) await hapusFile(lama.path);

  const dok = await prisma.dokumenPegawai.upsert({
    where: { pegawaiId_jenis: { pegawaiId, jenis } },
    update: { namaFile: file.name, path: namaSimpan, mimeType: file.type, ukuran: file.size },
    create: { pegawaiId, jenis, namaFile: file.name, path: namaSimpan, mimeType: file.type, ukuran: file.size },
  });
  return json(dok, 201);
}
