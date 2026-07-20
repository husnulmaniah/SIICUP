import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { simpanFile } from '@/lib/storage';

const MAX_SIZE = 10 * 1024 * 1024; // 10 MB untuk hasil scan
const ALLOWED = ['application/pdf', 'image/jpeg', 'image/png', 'image/webp'];

// POST /api/cuti/[id]/berkas-ttd — admin unggah berkas yang telah ditandatangani Kepala Dinas
export async function POST(req, { params }) {
  const { user, response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const id = Number(params.id);
  const cuti = await prisma.pengajuanCuti.findUnique({ where: { id } });
  if (!cuti) return errJson('Pengajuan tidak ditemukan.', 404);
  if (cuti.status !== 'DISETUJUI') return errJson('Berkas bertanda tangan hanya untuk pengajuan yang telah DISETUJUI.');

  const form = await req.formData();
  const files = form.getAll('file').filter((f) => f && typeof f !== 'string' && f.size > 0);
  if (!files.length) return errJson('Pilih minimal satu file untuk diunggah.');

  const tersimpan = [];
  for (const f of files) {
    if (f.size > MAX_SIZE) return errJson(`Ukuran "${f.name}" melebihi 10 MB.`);
    if (!ALLOWED.includes(f.type)) return errJson(`Format "${f.name}" tidak didukung. Gunakan PDF/JPG/PNG.`);
    const pathTersimpan = await simpanFile(`cuti${id}_TTD`, f);
    const berkas = await prisma.berkasCuti.create({
      data: {
        cutiId: id,
        jenisBerkas: 'BERKAS_TTD',
        namaBerkas: 'Berkas Ditandatangani Kepala Dinas',
        namaFile: f.name,
        path: pathTersimpan,
        mimeType: f.type,
        ukuran: f.size,
      },
    });
    tersimpan.push(berkas);
  }

  await prisma.riwayatCuti.create({
    data: { cutiId: id, aksi: 'BERKAS_TTD', oleh: user.nama, catatan: `Unggah ${tersimpan.length} berkas bertanda tangan Kepala Dinas.` },
  });
  return json({ message: `${tersimpan.length} berkas bertanda tangan tersimpan.`, berkas: tersimpan }, 201);
}
