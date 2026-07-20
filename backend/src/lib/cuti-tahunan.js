// Saldo cuti tahunan dengan alur FIFO:
// - Jatah 12 hari kerja per tahun.
// - Perhitungan DIMULAI sejak tahun pengajuan cuti pertama pegawai tercatat;
//   tahun-tahun sebelum itu dianggap kosong (tidak bernilai 12).
// - Cuti yang diambil memotong sisa TAHUN TERLAMA lebih dulu (carry-over):
//   contoh: sisa 2026 = 6, lalu cuti 2027 diambil 4 hari -> terbaca dari 2026 (sisa 2), 2027 tetap 12.
export const JATAH_TAHUNAN = 12;
export const JENIS_TAHUNAN = ['TAHUNAN', 'TAHUNAN_UMROH'];

export async function hitungSaldoTahunan(prisma, pegawaiId, tahunAcuan, statuses = ['DISETUJUI']) {
  // Kapan pegawai pertama kali masuk data pengajuan cuti (jenis apa pun)?
  const pertama = await prisma.pengajuanCuti.findFirst({
    where: { pegawaiId },
    orderBy: { tanggalMulai: 'asc' },
    select: { tanggalMulai: true },
  });
  const firstYear = pertama ? new Date(pertama.tanggalMulai).getUTCFullYear() : null;
  const mulaiTahun = firstYear === null ? tahunAcuan : Math.min(firstYear, tahunAcuan);

  const saldo = {};
  for (let y = mulaiTahun; y <= tahunAcuan; y++) saldo[y] = JATAH_TAHUNAN;

  const daftar = await prisma.pengajuanCuti.findMany({
    where: {
      pegawaiId,
      jenisCuti: { in: JENIS_TAHUNAN },
      status: { in: statuses },
      tanggalMulai: { lt: new Date(Date.UTC(tahunAcuan + 1, 0, 1)) },
    },
    orderBy: { tanggalMulai: 'asc' },
    select: { lamaCuti: true, tanggalMulai: true },
  });

  for (const c of daftar) {
    const tahunCuti = new Date(c.tanggalMulai).getUTCFullYear();
    let sisaAmbil = c.lamaCuti;
    // FIFO: habiskan saldo tahun terlama dulu, hanya sampai tahun cuti itu sendiri
    for (let y = mulaiTahun; y <= tahunCuti && sisaAmbil > 0; y++) {
      const ambil = Math.min(saldo[y] ?? 0, sisaAmbil);
      if (ambil > 0) {
        saldo[y] -= ambil;
        sisaAmbil -= ambil;
      }
    }
  }

  const totalSisa = Object.values(saldo).reduce((a, v) => a + v, 0);
  return { firstYear, mulaiTahun, saldo, totalSisa };
}

/** Sisa per label N/N-1/N-2 untuk formulir; sisa null = kosong (tahun sebelum data pertama) */
export function catatanNSisa(hasil, tahunAcuan) {
  const nilai = (tahun) => {
    if (hasil.firstYear === null || tahun < hasil.mulaiTahun) return null;
    return hasil.saldo[tahun] ?? null;
  };
  return [
    { label: 'N-1', tahun: tahunAcuan - 1, sisa: nilai(tahunAcuan - 1) },
    { label: 'N-2', tahun: tahunAcuan - 2, sisa: nilai(tahunAcuan - 2) },
    { label: 'N', tahun: tahunAcuan, sisa: nilai(tahunAcuan) },
  ].map((c) => ({
    ...c,
    keterangan: c.sisa === null ? '' : c.label === 'N' ? `Cuti tahunan berjalan ${c.tahun}` : `Sisa cuti tahun ${c.tahun}`,
  }));
}
