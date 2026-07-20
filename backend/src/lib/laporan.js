import * as XLSX from 'xlsx';
import { JENIS_CUTI } from './cuti-config';

const fmt = (t) => new Date(t).toLocaleDateString('id-ID', { day: '2-digit', month: 'long', year: 'numeric' });

/** Bangun workbook laporan rekap cuti -> Buffer */
export function buatLaporan(data, { dari, sampai, status, jenis }) {
  const periode =
    dari && sampai ? `Periode ${fmt(dari)} s.d. ${fmt(sampai)}`
    : dari ? `Mulai ${fmt(dari)}` : sampai ? `Sampai ${fmt(sampai)}` : 'Seluruh Periode';
  const filterInfo = [
    status && status !== 'SEMUA' ? `Status: ${status}` : null,
    jenis && jenis !== 'SEMUA' ? `Jenis: ${JENIS_CUTI[jenis]?.label || jenis}` : null,
  ].filter(Boolean).join(' | ');

  const judul = [
    ['REKAPAN CUTI GURU DAN PEGAWAI'],
    ['DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH KABUPATEN MOROWALI UTARA'],
    [periode + (filterInfo ? ` (${filterInfo})` : '')],
    [],
  ];
  const header = ['NO', 'NIP', 'NAMA', 'JABATAN', 'UNIT KERJA', 'GOL. AKHIR', 'JENIS CUTI', 'TANGGAL MULAI', 'TANGGAL SELESAI', 'LAMA (HARI)', 'STATUS', 'DIPROSES OLEH', 'CATATAN'];
  const rows = data.map((c, i) => [
    i + 1, c.pegawai.nip, c.pegawai.nama, c.pegawai.jabatan || '-', c.pegawai.tempatTugas || c.pegawai.unor || '-',
    c.pegawai.golAkhir || '-', JENIS_CUTI[c.jenisCuti]?.label || c.jenisCuti,
    fmt(c.tanggalMulai), fmt(c.tanggalSelesai), c.lamaCuti, c.status, c.diprosesOleh || '-', c.catatan || '-',
  ]);

  // Ringkasan di bawah tabel
  const perStatus = {};
  const perJenis = {};
  for (const c of data) {
    perStatus[c.status] = (perStatus[c.status] || 0) + 1;
    perJenis[c.jenisCuti] = (perJenis[c.jenisCuti] || 0) + 1;
  }
  const ringkasan = [
    [],
    ['RINGKASAN'],
    ['Total Pengajuan', data.length],
    ['Total Hari Cuti', data.reduce((a, c) => a + c.lamaCuti, 0)],
    ...Object.entries(perStatus).map(([k, v]) => [`Status ${k}`, v]),
    ...Object.entries(perJenis).map(([k, v]) => [JENIS_CUTI[k]?.label || k, v]),
  ];

  const ws = XLSX.utils.aoa_to_sheet([...judul, header, ...rows, ...ringkasan]);
  ws['!cols'] = header.map((h, i) => ({ wch: i === 2 || i === 4 ? 34 : Math.max(String(h).length + 4, 14) }));
  ws['!merges'] = [
    { s: { r: 0, c: 0 }, e: { r: 0, c: header.length - 1 } },
    { s: { r: 1, c: 0 }, e: { r: 1, c: header.length - 1 } },
    { s: { r: 2, c: 0 }, e: { r: 2, c: header.length - 1 } },
  ];
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, 'REKAPAN CUTI');
  return XLSX.write(wb, { type: 'buffer', bookType: 'xlsx' });
}
