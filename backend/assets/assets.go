// Package assets embeds small static binary assets used by the backend
// (currently just the instansi logo used on the printable leave forms) so
// they ship inside the compiled binary and need no separate file on disk at
// runtime/deploy time.
package assets

import _ "embed"

//go:embed logo.png
var LogoPNG []byte

// TutWuriPNG: logo Tut Wuri Handayani, ditampilkan OPSIONAL di sisi kanan
// kop surat Lampiran 3 (lihat UnitKerja.TampilkanLogoTutwuri &
// drawLetterheadUnitKerjaKustom di handlers/surat_rekomendasi.go) --
// BELUM ada berkas gambarnya (menunggu dikirim pengguna), jadi untuk
// sementara TIDAK dipakai //go:embed seperti LogoPNG di atas (supaya
// build tidak gagal karena file belum ada) -- nilainya kosong, kode
// pemanggil WAJIB mengecek len(TutWuriPNG) > 0 sebelum menggambarnya.
// Begitu file logo_tutwuri.png ditambahkan ke folder ini, ganti baris di
// bawah menjadi "//go:embed logo_tutwuri.png" + "var TutWuriPNG []byte".
var TutWuriPNG []byte
