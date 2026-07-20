// Konfigurasi jenis cuti dan kelengkapan berkas yang wajib/opsional diunggah.
// Sinkron dengan frontend/src/cuti-config.js
export const JENIS_CUTI = {
  TAHUNAN: {
    label: 'Cuti Tahunan',
    berkas: [
      { kode: 'REKOMENDASI_KEPSEK', label: 'Surat Rekomendasi Kepala Sekolah', wajib: true },
      { kode: 'SK_TERAKHIR', label: 'SK Terakhir', wajib: true },
    ],
  },
  MELAHIRKAN: {
    label: 'Cuti Melahirkan',
    berkas: [
      { kode: 'REKOMENDASI_KEPSEK', label: 'Surat Rekomendasi Kepala Sekolah', wajib: true },
      { kode: 'SK_TERAKHIR', label: 'SK Terakhir', wajib: true },
      { kode: 'SURAT_HPL', label: 'Surat Keterangan HPL (Rumah Sakit/Puskesmas)', wajib: true },
      { kode: 'BUKU_KIA', label: 'Buku KIA', wajib: true },
      { kode: 'HASIL_USG', label: 'Hasil USG', wajib: false },
    ],
  },
  TAHUNAN_UMROH: {
    label: 'Cuti Tahunan (Umroh)',
    berkas: [
      { kode: 'REKOMENDASI_KEPSEK', label: 'Surat Rekomendasi Kepala Sekolah', wajib: true },
      { kode: 'SK_TERAKHIR', label: 'SK Terakhir', wajib: true },
      { kode: 'SURAT_TRAVEL', label: 'Surat Keterangan dari Travel Pemberangkatan', wajib: true },
    ],
  },
  SAKIT: {
    label: 'Cuti Sakit',
    berkas: [
      { kode: 'SK_TERAKHIR', label: 'SK Terakhir', wajib: true },
      { kode: 'SURAT_RUJUKAN', label: 'Surat Rujukan', wajib: true },
      { kode: 'SURAT_RAWAT_INAP', label: 'Surat Keterangan Rawat Inap', wajib: true },
    ],
  },
  ALASAN_PENTING: {
    label: 'Cuti Alasan Penting',
    berkas: [
      { kode: 'SK_TERAKHIR', label: 'SK Terakhir', wajib: true },
      { kode: 'REKOMENDASI_KEPSEK', label: 'Surat Rekomendasi Kepala Sekolah', wajib: true },
      {
        kode: 'DOKUMEN_PENDUKUNG',
        label:
          'Dokumen Pendukung (surat ket. rawat inap keluarga / surat keterangan kematian / surat keterangan KUA / dokumen istri melahirkan)',
        wajib: true,
        multiple: true,
      },
    ],
  },
};

export const STATUS_CUTI = ['DIAJUKAN', 'DISETUJUI', 'DIKEMBALIKAN', 'DITOLAK'];
