// Hitung lama cuti berdasarkan pola kerja & daftar hari libur.
// pola: '5_HARI' (Sabtu-Minggu libur) | '6_HARI' (Minggu libur) | 'KALENDER' (semua hari dihitung)
// tanggalLibur: array 'YYYY-MM-DD'
// Sinkron dengan frontend/src/hari-kerja.js
export function hitungLamaCuti(mulai, selesai, pola = '5_HARI', tanggalLibur = []) {
  const libur = new Set(tanggalLibur);
  const awal = new Date(mulai);
  const akhir = new Date(selesai);
  if (isNaN(awal) || isNaN(akhir) || akhir < awal) return 0;
  let d = Date.UTC(awal.getUTCFullYear(), awal.getUTCMonth(), awal.getUTCDate());
  const end = Date.UTC(akhir.getUTCFullYear(), akhir.getUTCMonth(), akhir.getUTCDate());
  let jumlah = 0;
  while (d <= end) {
    const tgl = new Date(d);
    const hari = tgl.getUTCDay(); // 0=Minggu, 6=Sabtu
    const iso = tgl.toISOString().slice(0, 10);
    let dihitung = true;
    if (pola !== 'KALENDER') {
      if (hari === 0) dihitung = false;
      if (pola === '5_HARI' && hari === 6) dihitung = false;
      if (libur.has(iso)) dihitung = false;
    }
    if (dihitung) jumlah++;
    d += 86400000;
  }
  return jumlah;
}

export const POLA_KERJA = [
  { kode: '5_HARI', label: '5 Hari Kerja (Sabtu & Minggu tidak dihitung)' },
  { kode: '6_HARI', label: '6 Hari Kerja (Minggu tidak dihitung)' },
  { kode: 'KALENDER', label: 'Hari Kalender (semua hari dihitung)' },
];
