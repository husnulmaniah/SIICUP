import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, errJson, json } from '@/lib/auth';
import { buatRekomendasi, defaultOpsi } from '@/lib/surat';
import { hitungSaldoTahunan, catatanNSisa } from '@/lib/cuti-tahunan';

// GET /api/cuti/[id]/surat — kembalikan data + opsi default utk preview di frontend
export async function GET(req, { params }) {
  // Pembuatan surat hanya untuk Admin Utama & Admin Pembantu
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const cuti = await prisma.pengajuanCuti.findUnique({
    where: { id: Number(params.id) },
    include: { pegawai: true },
  });
  if (!cuti) return errJson('Pengajuan tidak ditemukan.', 404);
  if (cuti.status !== 'DISETUJUI') return errJson('Surat hanya tersedia untuk pengajuan yang telah DISETUJUI.');
  const tahunAcuan = new Date(cuti.tanggalMulai).getUTCFullYear();
  const saldo = await hitungSaldoTahunan(prisma, cuti.pegawaiId, tahunAcuan, ['DISETUJUI']);
  const catatanCuti = catatanNSisa(saldo, tahunAcuan);
  return json({ cuti, opsi: defaultOpsi(cuti), catatanCuti });
}

// POST /api/cuti/[id]/surat  { jenis: 'formulir'|'rekomendasi', opsi:{nomorSurat,kota,tanggalSurat,kepalaDinas,nipKepalaDinas} }
// -> file .docx
export async function POST(req, { params }) {
  // Pembuatan surat hanya untuk Admin Utama & Admin Pembantu
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const cuti = await prisma.pengajuanCuti.findUnique({
    where: { id: Number(params.id) },
    include: { pegawai: true },
  });
  if (!cuti) return errJson('Pengajuan tidak ditemukan.', 404);
  if (cuti.status !== 'DISETUJUI') return errJson('Surat hanya dapat dibuat untuk pengajuan yang telah DISETUJUI.');

  const body = await req.json();
  const opsi = { ...defaultOpsi(cuti), ...(body.opsi || {}) };
  // Hanya Surat Rekomendasi yang dibuat sebagai Word di server.
  // Formulir cuti diunduh sebagai PDF langsung dari pratinjau di aplikasi.
  const buffer = await buatRekomendasi(cuti, opsi);
  const namaAman = cuti.pegawai.nama
    .normalize('NFKD')
    .replace(/[^\w\s.-]/g, '')
    .trim()
    .replace(/\s+/g, '_');
  const namaFile = `REKOMENDASI_CUTI_${cuti.pegawai.nip}_${namaAman}.docx`;

  return new Response(buffer, {
    headers: {
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      'Content-Disposition': `attachment; filename="${namaFile}"; filename*=UTF-8''${encodeURIComponent(namaFile)}`,
    },
  });
}
