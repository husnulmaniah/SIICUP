import * as XLSX from 'xlsx';

// Pemetaan kolom Excel <-> field database (format SIASN/BKN)
export const KOLOM_PEGAWAI = [
  { header: 'NIP BARU', field: 'nip' },
  { header: 'NIK', field: 'nik' },
  { header: 'NAMA', field: 'nama' },
  { header: 'TEMPAT LAHIR NAMA', field: 'tempatLahir' },
  { header: 'TANGGAL LAHIR', field: 'tanggalLahir' },
  { header: 'JENIS KELAMIN', field: 'jenisKelamin' },
  { header: 'STATUS CPNS PNS', field: 'statusCpnsPns' },
  { header: 'TANGGAL SK CPNS', field: 'tanggalSkCpns' },
  { header: 'TMT CPNS', field: 'tmtCpns' },
  { header: 'TANGGAL SK PNS', field: 'tanggalSkPns' },
  { header: 'TMT PNS', field: 'tmtPns' },
  { header: 'GOL AWAL NAMA', field: 'golAwal' },
  { header: 'GOL AKHIR NAMA', field: 'golAkhir' },
  { header: 'TMT GOLONGAN', field: 'tmtGolongan' },
  { header: 'JENIS JABATAN NAMA', field: 'jenisJabatan' },
  { header: 'JABATAN NAMA', field: 'jabatan' },
  { header: 'TMT JABATAN', field: 'tmtJabatan' },
  { header: 'TINGKAT PENDIDIKAN NAMA', field: 'tingkatPendidikan' },
  { header: 'PENDIDIKAN NAMA', field: 'pendidikan' },
  { header: 'KECAMATAN', field: 'kecamatan' },
  { header: 'UNOR NAMA', field: 'unor' },
  { header: 'TEMPAT TUGAS', field: 'tempatTugas' },
  { header: 'STATUS', field: 'status' },
  { header: 'TANGGAL KENAIKAN GAJI BERKALA TERAKHIR', field: 'tanggalKgbTerakhir' },
  { header: 'TANGGAL KENAIKAN PANGKAT TERAKHIR', field: 'tanggalKpTerakhir' },
  { header: 'TANGGAL PENSIUN', field: 'tanggalPensiun' },
  { header: 'TANGGAL KENAIKAN GAJI BERKALA', field: 'tanggalKgb' },
  { header: 'TANGGAL KENAIKAN PANGKAT', field: 'tanggalKp' },
  { header: 'TAHUN PENGANGKATAN', field: 'tahunPengangkatan' },
];

const norm = (v) =>
  String(v ?? '')
    .replace(/[\u200c\u200b\ufeff]/g, '')
    .replace(/\s+/g, ' ')
    .trim();

const clean = (v) => {
  if (v === null || v === undefined) return null;
  // Tanggal dari Excel bisa berupa objek Date
  if (v instanceof Date) return v.toISOString().slice(0, 10);
  const s = norm(v);
  return s === '' || s.toUpperCase() === 'NULL' || s === '#N/A' ? null : s;
};

export const cleanNip = (v) => {
  const s = clean(v);
  return s ? s.replace(/\s+/g, '') : null;
};

/** Buat workbook template import (header + 1 baris contoh) sebagai Buffer */
export function buatTemplate() {
  const headers = KOLOM_PEGAWAI.map((k) => k.header);
  const contoh = [
    '197401111998031004',      // NIP BARU
    '7206091101740001',        // NIK
    'NAMA PEGAWAI, S.Pd',      // NAMA
    'KOLONODALE',              // TEMPAT LAHIR NAMA
    '1974-01-11',              // TANGGAL LAHIR
    'L',                       // JENIS KELAMIN
    'PNS',                     // STATUS CPNS PNS
    '1998-02-10',              // TANGGAL SK CPNS
    '1998-03-01',              // TMT CPNS
    '1999-03-15',              // TANGGAL SK PNS
    '1999-04-01',              // TMT PNS
    'PENGATUR MUDA, II/A',     // GOL AWAL NAMA
    'PEMBINA, IV/A',           // GOL AKHIR NAMA
    '2020-04-01',              // TMT GOLONGAN
    'FUNGSIONAL TERTENTU',     // JENIS JABATAN NAMA
    'GURU AHLI MADYA',         // JABATAN NAMA
    '2020-04-01',              // TMT JABATAN
    'S1',                      // TINGKAT PENDIDIKAN NAMA
    'PENDIDIKAN MATEMATIKA',   // PENDIDIKAN NAMA
    'BUNGKU UTARA',            // KECAMATAN
    'DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH', // UNOR NAMA
    'SMP NEGERI 1 BUNGKU UTARA', // TEMPAT TUGAS
    'AKTIF',                   // STATUS
    '2024-03-01',              // TGL KGB TERAKHIR
    '2020-04-01',              // TGL KP TERAKHIR
    '2034-02-01',              // TANGGAL PENSIUN
    '2026-03-01',              // TGL KGB (berikutnya)
    '2024-04-01',              // TGL KP (berikutnya)
    '1998',                    // TAHUN PENGANGKATAN
  ];
  const ws = XLSX.utils.aoa_to_sheet([headers, contoh]);
  ws['!cols'] = headers.map((h) => ({ wch: Math.max(h.length + 3, 16) }));
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, 'DATA PEGAWAI');
  return XLSX.write(wb, { type: 'buffer', bookType: 'xlsx' });
}

/** Parse file import (ArrayBuffer) -> array objek pegawai */
export function parseImport(arrayBuffer) {
  const wb = XLSX.read(arrayBuffer, { type: 'array', cellDates: true });
  const ws = wb.Sheets[wb.SheetNames[0]];
  const rows = XLSX.utils.sheet_to_json(ws, { header: 1, defval: null });
  if (!rows.length) return [];

  // Cari baris header: baris yang mengandung "NIP BARU" (atau "NIP") dan "NAMA"
  let headerIdx = rows.findIndex((r) => {
    if (!r) return false;
    const h = r.map((c) => norm(c).toUpperCase());
    return (h.includes('NIP BARU') || h.includes('NIP')) && h.includes('NAMA');
  });
  if (headerIdx < 0) headerIdx = 0;
  const headerRow = rows[headerIdx].map((c) => norm(c).toUpperCase());

  // Pemetaan index kolom: cocokkan persis (header template baku)
  const idx = {};
  for (const k of KOLOM_PEGAWAI) {
    let i = headerRow.indexOf(k.header);
    if (i < 0 && k.header === 'NIP BARU') i = headerRow.indexOf('NIP'); // toleransi header lama
    if (i >= 0) idx[k.field] = i;
  }

  const hasil = [];
  for (let r = headerIdx + 1; r < rows.length; r++) {
    const row = rows[r];
    if (!row) continue;
    const item = {};
    for (const k of KOLOM_PEGAWAI) {
      item[k.field] = idx[k.field] !== undefined ? clean(row[idx[k.field]]) : null;
    }
    item.nip = cleanNip(item.nip);
    if (!item.nip || !item.nama) continue; // NIP BARU & NAMA wajib
    hasil.push(item);
  }
  return hasil;
}

/** Bangun workbook export dari daftar pegawai -> Buffer */
export function buatExport(pegawaiList) {
  const headers = ['NO', ...KOLOM_PEGAWAI.map((k) => k.header), 'STATUS AKTIF'];
  const data = pegawaiList.map((p, i) => [
    i + 1,
    ...KOLOM_PEGAWAI.map((k) => p[k.field] ?? ''),
    p.aktif ? 'AKTIF' : 'NONAKTIF',
  ]);
  const ws = XLSX.utils.aoa_to_sheet([headers, ...data]);
  ws['!cols'] = headers.map((h) => ({ wch: Math.max(h.length + 3, 14) }));
  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, 'DATA PEGAWAI AKTIF');
  return XLSX.write(wb, { type: 'buffer', bookType: 'xlsx' });
}
