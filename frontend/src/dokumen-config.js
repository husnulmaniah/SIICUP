// Sinkron dengan backend/src/lib/dokumen-config.js
export const JENIS_DOKUMEN = [
  { kode: 'SK_CPNS', label: 'SK CPNS' },
  { kode: 'SK_PNS', label: 'SK PNS' },
  { kode: 'SK_PANGKAT_TERAKHIR', label: 'SK Pangkat Terakhir' },
  { kode: 'KGB_TERAKHIR', label: 'KGB Terakhir' },
  { kode: 'SK_TERAKHIR', label: 'SK Terakhir' },
];
export const labelDokumen = (kode) => JENIS_DOKUMEN.find((d) => d.kode === kode)?.label || kode;
