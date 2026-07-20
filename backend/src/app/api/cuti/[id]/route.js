import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { JENIS_CUTI } from '@/lib/cuti-config';
import { hitungLamaCuti } from '@/lib/hari-kerja';
import { simpanFile, hapusFile } from '@/lib/storage';

export async function GET(req, { params }) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const cuti = await prisma.pengajuanCuti.findUnique({
    where: { id: Number(params.id) },
    include: {
      pegawai: true,
      berkas: true,
      riwayat: { orderBy: { createdAt: 'asc' } },
    },
  });
  if (!cuti) return errJson('Pengajuan tidak ditemukan.', 404);
  if (!ADMINS.includes(user.role) && cuti.pegawaiId !== user.pegawaiId) {
    return errJson('Anda tidak memiliki akses ke pengajuan ini.', 403);
  }
  return json(cuti);
}

// PUT — perbaiki pengajuan yang DIKEMBALIKAN (pemilik atau admin), lalu status kembali DIAJUKAN.
// multipart: field data + berkas tambahan opsional (berkas_KODE)
export async function PUT(req, { params }) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const id = Number(params.id);
  const cuti = await prisma.pengajuanCuti.findUnique({ where: { id } });
  if (!cuti) return errJson('Pengajuan tidak ditemukan.', 404);
  const pemilik = cuti.pegawaiId === user.pegawaiId;
  if (!ADMINS.includes(user.role) && !pemilik) return errJson('Anda tidak memiliki akses.', 403);
  if (cuti.status !== 'DIKEMBALIKAN') return errJson('Hanya pengajuan berstatus DIKEMBALIKAN yang dapat diperbaiki.');

  const form = await req.formData();
  const data = {};
  if (form.get('tanggalMulai')) data.tanggalMulai = new Date(form.get('tanggalMulai'));
  if (form.get('tanggalSelesai')) data.tanggalSelesai = new Date(form.get('tanggalSelesai'));
  if (form.get('alasan')) data.alasan = String(form.get('alasan'));
  if (form.get('alamatSelamaCuti') !== null) data.alamatSelamaCuti = String(form.get('alamatSelamaCuti') || '') || null;
  if (form.get('noHp') !== null) data.noHp = String(form.get('noHp') || '') || null;
  if (form.get('polaKerja') && ['5_HARI', '6_HARI', 'KALENDER'].includes(form.get('polaKerja'))) {
    data.polaKerja = form.get('polaKerja');
  }
  const mulai = data.tanggalMulai || cuti.tanggalMulai;
  const selesai = data.tanggalSelesai || cuti.tanggalSelesai;
  if (selesai < mulai) return errJson('Tanggal selesai tidak boleh sebelum tanggal mulai.');
  const daftarLibur = (await prisma.hariLibur.findMany()).map((h) => h.tanggal);
  data.lamaCuti = hitungLamaCuti(mulai, selesai, data.polaKerja || cuti.polaKerja, daftarLibur);
  if (data.lamaCuti < 1) return errJson('Rentang tanggal tidak mengandung hari kerja. Periksa kembali tanggalnya.');
  data.status = 'DIAJUKAN';
  data.catatan = null;

  // Hapus berkas yang diminta dihapus
  const hapusIds = String(form.get('hapusBerkas') || '').split(',').map(Number).filter(Boolean);
  if (hapusIds.length) {
    const lama = await prisma.berkasCuti.findMany({ where: { id: { in: hapusIds }, cutiId: id } });
    for (const b of lama) await hapusFile(b.path);
    await prisma.berkasCuti.deleteMany({ where: { id: { in: hapusIds }, cutiId: id } });
  }

  // Berkas baru
  const config = JENIS_CUTI[cuti.jenisCuti];
  for (const b of config.berkas) {
    const files = form.getAll(`berkas_${b.kode}`).filter((f) => f && typeof f !== 'string' && f.size > 0);
    for (const f of files) {
      if (f.size > 5 * 1024 * 1024) return errJson(`Ukuran "${f.name}" melebihi 5 MB.`);
      const tersimpan = await simpanFile(`cuti${id}_${b.kode}`, f);
      await prisma.berkasCuti.create({
        data: { cutiId: id, jenisBerkas: b.kode, namaBerkas: b.label, namaFile: f.name, path: tersimpan, mimeType: f.type, ukuran: f.size },
      });
    }
  }

  const updated = await prisma.pengajuanCuti.update({ where: { id }, data });
  await prisma.riwayatCuti.create({ data: { cutiId: id, aksi: 'DIPERBAIKI', oleh: user.nama, catatan: 'Pengajuan diperbaiki dan diajukan ulang.' } });
  return json(updated);
}
