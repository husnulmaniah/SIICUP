// Package assets embeds small static binary assets used by the backend
// (logo instansi/Kabupaten & logo Tut Wuri Handayani untuk kop surat) supaya
// ikut terbungkus di dalam binary hasil compile & tidak perlu berkas
// terpisah di disk saat runtime/deploy.
package assets

import _ "embed"

// LogoPNG: logo Kabupaten/instansi -- dipakai di SEMUA kop surat cetak
// (Formulir Cuti, Lampiran 2/surat rekomendasi Dinas lewat drawLetterhead di
// handlers/formulir.go, MAUPUN slot kiri Lampiran 3/surat rekomendasi
// Sekolah lewat drawLetterheadUnitKerjaKustom di
// handlers/surat_rekomendasi.go) -- SATU berkas yang sama, supaya logo
// Kabupaten konsisten di semua jenis surat kecuali sekolah mengganti slot
// kiri Lampiran 3-nya sendiri dengan logo kustom (lihat
// UnitKerja.LogoKiriFile & handlers/kop_surat.go).
//
//go:embed logo.png
var LogoPNG []byte

// TutWuriPNG: logo Tut Wuri Handayani, ditampilkan OPSIONAL di sisi kanan
// kop surat Lampiran 3 (lihat UnitKerja.TampilkanLogoTutwuri &
// drawLetterheadUnitKerjaKustom di handlers/surat_rekomendasi.go) --
// digantikan logo kustom sekolah sendiri (UnitKerja.LogoKananFile) kalau
// ada, lihat komentar pada field itu.
//
//go:embed logo_tutwuri.png
var TutWuriPNG []byte
