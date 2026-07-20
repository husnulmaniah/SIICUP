import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';
import { JENIS_CUTI } from '@/lib/cuti-config';
import { hitungLamaCuti } from '@/lib/hari-kerja';
import { simpanFile, salinFile } from '@/lib/storage';
import { hitungSaldoTahunan, JENIS_TAHUNAN } from '@/lib/cuti-tahunan';

const MAX_SIZE = 5 * 1024 * 1024; // 5 MB per berkas
const ALLOWED = ['application/pdf', 'image/jpeg', 'image/png', 'image/webp'];

// GET /api/cuti — admin melihat semua, pegawai hanya miliknya
export async function GET(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;
  const where = ADMINS.includes(user.role) ? {} : { pegawaiId: user.pegawaiId ?? -1 };
  const data = await prisma.pengajuanCuti.findMany({
    where,
    include: { pegawai: { select: { nama: true, nip: true, jabatan: true, tempatTugas: true } }, berkas: { select: { id: true } } },
    orderBy: { createdAt: 'desc' },
  });
  return json(data);
}

// POST /api/cuti — multipart form: data pengajuan + berkas per jenis kebutuhan
// Admin dapat mengajukan atas nama pegawai (field pegawaiId); pegawai untuk dirinya sendiri.
export async function POST(req) {
  const { user, response } = await requireAuth(req);
  if (response) return response;

  const form = await req.formData();
  const jenisCuti = form.get('jenisCuti');
  const config = JENIS_CUTI[jenisCuti];
  if (!config) return errJson('Jenis cuti tidak dikenal.');

  let pegawaiId;
  if (ADMINS.includes(user.role)) {
    pegawaiId = Number(form.get('pegawaiId'));
    if (!pegawaiId) return errJson('Pilih pegawai yang diajukan cutinya.');
  } else {
    pegawaiId = user.pegawaiId;
    if (!pegawaiId) return errJson('Akun Anda tidak tertaut ke data pegawai.');
  }
  const pegawai = await prisma.pegawai.findUnique({ where: { id: pegawaiId } });
  if (!pegawai || !pegawai.aktif) return errJson('Pegawai tidak ditemukan atau nonaktif.');

  const tanggalMulai = new Date(form.get('tanggalMulai'));
  const tanggalSelesai = new Date(form.get('tanggalSelesai'));
  const alasan = String(form.get('alasan') || '').trim();
  if (isNaN(tanggalMulai) || isNaN(tanggalSelesai)) return errJson('Tanggal mulai dan selesai wajib diisi.');
  if (tanggalSelesai < tanggalMulai) return errJson('Tanggal selesai tidak boleh sebelum tanggal mulai.');
  if (!alasan) return errJson('Alasan cuti wajib diisi.');

  // Lama cuti = hari kerja sesuai pola; akhir pekan & hari libur nasional tidak dihitung
  const polaKerja = ['5_HARI', '6_HARI', 'KALENDER'].includes(form.get('polaKerja'))
    ? form.get('polaKerja') : '5_HARI';
  const daftarLibur = (await prisma.hariLibur.findMany()).map((h) => h.tanggal);
  const lamaCuti = hitungLamaCuti(tanggalMulai, tanggalSelesai, polaKerja, daftarLibur);
  if (lamaCuti < 1) return errJson('Rentang tanggal tidak mengandung hari kerja (semua akhir pekan/libur). Periksa kembali tanggalnya.');

  // Batas cuti tahunan: jatah 12 hari/tahun, FIFO dari sisa tahun terlama.
  // Pengajuan yang masih DIAJUKAN ikut dihitung agar jatah tidak terpakai ganda.
  if (JENIS_TAHUNAN.includes(jenisCuti)) {
    const tahunAjuan = tanggalMulai.getUTCFullYear();
    const saldo = await hitungSaldoTahunan(prisma, pegawaiId, tahunAjuan, ['DIAJUKAN', 'DISETUJUI']);
    if (saldo.totalSisa <= 0) {
      return errJson('Jatah cuti tahunan telah habis (12 hari sudah terpakai, termasuk pengajuan yang sedang diproses). Pengajuan cuti tahunan baru tidak dapat dibuat.');
    }
    if (lamaCuti > saldo.totalSisa) {
      return errJson(`Sisa cuti tahunan Anda ${saldo.totalSisa} hari; pengajuan ${lamaCuti} hari melebihi sisa jatah.`);
    }
  }

  // Rekomendasi kepala sekolah hanya wajib bagi pegawai yang bertugas di sekolah.
  // Pegawai yang bertempat tugas di Dinas tidak perlu melampirkannya.
  const diDinas = ((pegawai.tempatTugas || pegawai.unor || '').toUpperCase()).includes('DINAS');

  // Validasi kelengkapan berkas wajib + kumpulkan file.
  // Khusus SK_TERAKHIR: jika tidak diunggah tetapi sudah ada di dokumen pegawai,
  // otomatis dilampirkan dari data pegawai (disalin sebagai snapshot).
  const fileEntries = []; // { kode, label, file }
  let skDariDokumen = null;
  for (const b of config.berkas) {
    const files = form.getAll(`berkas_${b.kode}`).filter((f) => f && typeof f !== 'string' && f.size > 0);
    if (b.wajib && files.length === 0) {
      if (b.kode === 'REKOMENDASI_KEPSEK' && diDinas) continue;
      if (b.kode === 'SK_TERAKHIR') {
        const dok = await prisma.dokumenPegawai.findUnique({
          where: { pegawaiId_jenis: { pegawaiId, jenis: 'SK_TERAKHIR' } },
        });
        if (dok) {
          skDariDokumen = dok;
          continue;
        }
      }
      return errJson(`Berkas wajib belum diunggah: ${b.label}`);
    }
    for (const f of files) {
      if (f.size > MAX_SIZE) return errJson(`Ukuran "${f.name}" melebihi 5 MB.`);
      if (!ALLOWED.includes(f.type)) return errJson(`Format "${f.name}" tidak didukung. Gunakan PDF/JPG/PNG.`);
      fileEntries.push({ kode: b.kode, label: b.label, file: f });
    }
  }

  // Simpan pengajuan
  const cuti = await prisma.pengajuanCuti.create({
    data: {
      pegawaiId,
      jenisCuti,
      tanggalMulai,
      tanggalSelesai,
      lamaCuti,
      polaKerja,
      alasan,
      alamatSelamaCuti: String(form.get('alamatSelamaCuti') || '') || null,
      noHp: String(form.get('noHp') || '') || null,
      status: 'DIAJUKAN',
      diajukanOlehId: user.id,
    },
  });

  // Simpan berkas (Vercel Blob di produksi, ./uploads di lokal) + metadata
  for (const e of fileEntries) {
    const tersimpan = await simpanFile(`cuti${cuti.id}_${e.kode}`, e.file);
    await prisma.berkasCuti.create({
      data: { cutiId: cuti.id, jenisBerkas: e.kode, namaBerkas: e.label, namaFile: e.file.name, path: tersimpan, mimeType: e.file.type, ukuran: e.file.size },
    });
  }

  // Lampirkan SK Terakhir dari dokumen pegawai (disalin sebagai snapshot pengajuan)
  if (skDariDokumen) {
    const salinan = await salinFile(skDariDokumen.path, `cuti${cuti.id}_SK_TERAKHIR`);
    if (salinan) {
      await prisma.berkasCuti.create({
        data: {
          cutiId: cuti.id,
          jenisBerkas: 'SK_TERAKHIR',
          namaBerkas: 'SK Terakhir (otomatis dari data pegawai)',
          namaFile: skDariDokumen.namaFile,
          path: salinan,
          mimeType: skDariDokumen.mimeType,
          ukuran: skDariDokumen.ukuran,
        },
      });
    }
  }

  await prisma.riwayatCuti.create({ data: { cutiId: cuti.id, aksi: 'DIAJUKAN', oleh: user.nama } });
  return json(cuti, 201);
}
