// keteranganSurat.js -- daftar pilihan tetap untuk kolom "Keterangan" pada
// surat kolektif (dipakai bersama oleh AbsensiView.vue -- pengajuan mandiri
// pegawai sekolah lewat "Ajukan Surat Kolektif" & dialog edit-nya -- dan
// RekapAbsensiView.vue -- input langsung administrator/admin/admin absen
// lewat "Input Surat Kolektif") supaya isi kolom ini konsisten & mudah
// dibaca (dulu kotak teks bebas, jadi banyak variasi tulisan untuk maksud
// yang sama).
//
// Kolom "keterangan" pada database TETAP teks bebas (tidak diubah jadi
// enum) -- pilihan "Lainnya" di bawah memungkinkan tetap mengisi teks bebas
// kalau tidak ada yang cocok, dan data lama (sebelum dropdown ini ada)
// tetap terbaca lewat pisahkanKeterangan (lihat di bawah).
export const KETERANGAN_SURAT_OPTIONS = [
  'Surat Tugas',
  'Absensi Manual',
  'Surat Keterangan Sakit',
  'Surat Opname',
  'Surat Cuti',
  'Surat Izin',
  'Surat Dispensasi',
]

export const KETERANGAN_LAINNYA = 'Lainnya'

// Daftar lengkap untuk opsi dropdown (termasuk "Lainnya" di baris terakhir).
export const KETERANGAN_SURAT_DROPDOWN = [...KETERANGAN_SURAT_OPTIONS, KETERANGAN_LAINNYA]

// pisahkanKeterangan menentukan pilihan dropdown mana yang harus terpilih
// dari teks keterangan yang sudah tersimpan (dipakai saat membuka dialog
// edit pengajuan) -- kalau teksnya cocok persis dengan salah satu opsi
// tetap, pilih itu; kalau tidak (termasuk kosong atau teks bebas dari
// sebelum dropdown ini ada), otomatis dianggap "Lainnya" dengan teks aslinya
// tetap ditampilkan di kotak isian tambahan supaya tidak ada data yang
// hilang/tertukar.
export function pisahkanKeterangan(teks) {
  const t = (teks || '').trim()
  if (!t) return { pilihan: null, lainnya: '' }
  if (KETERANGAN_SURAT_OPTIONS.includes(t)) return { pilihan: t, lainnya: '' }
  return { pilihan: KETERANGAN_LAINNYA, lainnya: t }
}

// gabungkanKeterangan mengubah balik (pilihan dropdown, teks "Lainnya")
// jadi satu string keterangan yang dikirim ke server.
export function gabungkanKeterangan(pilihan, lainnya) {
  if (!pilihan) return ''
  if (pilihan === KETERANGAN_LAINNYA) return (lainnya || '').trim()
  return pilihan
}
