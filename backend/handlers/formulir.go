package handlers

import (
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"strings"
	"time"

	"cuti-app/assets"
	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// formulir.go generates the two printable leave forms ("Surat Rekomendasi
// Izin Cuti" and "Formulir Permintaan dan Pemberian Cuti") for a pengajuan
// cuti that has been disetujui (approved). Both mirror the office's existing
// paper forms, filled in with the actual submission's data.

var romanMonths = []string{"", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI", "XII"}

func romanMonth(m time.Month) string { return romanMonths[int(m)] }

var bulanIndo = []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func formatDateID(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), bulanIndo[int(t.Month())], t.Year())
}

// formatTanggalRentang renders a date range the way the office's paper forms
// do, e.g. "14 September-03 Oktober 2026" (or "10-12 September 2026" when
// both dates fall in the same month).
func formatTanggalRentang(start, end time.Time) string {
	if start.Month() == end.Month() && start.Year() == end.Year() {
		return fmt.Sprintf("%02d-%02d %s %d", start.Day(), end.Day(), bulanIndo[int(start.Month())], end.Year())
	}
	return fmt.Sprintf("%02d %s-%02d %s %d", start.Day(), bulanIndo[int(start.Month())], end.Day(), bulanIndo[int(end.Month())], end.Year())
}

// masaKerjaText computes "X Tahun Y Bulan" of service from a pegawai's TMT
// (tanggal mulai tugas) up to a reference date.
func masaKerjaText(tmt *time.Time, ref time.Time) string {
	if tmt == nil {
		return "-"
	}
	years := ref.Year() - tmt.Year()
	months := int(ref.Month()) - int(tmt.Month())
	if ref.Day() < tmt.Day() {
		months--
	}
	if months < 0 {
		years--
		months += 12
	}
	if years < 0 {
		years, months = 0, 0
	}
	return fmt.Sprintf("%d Tahun %d Bulan", years, months)
}

// jenisCutiCheckboxIndex maps our jenis_cuti name to the standard PP 11/2017
// leave-type numbering (1-6) used on the office's printed form. Types not in
// our master data by default (e.g. "Cuti di Luar Tanggungan Negara") simply
// never get ticked; "Cuti Tahunan Umroh" is treated as ordinary Cuti Tahunan.
func jenisCutiCheckboxIndex(jenisNama string) int {
	j := strings.ToLower(jenisNama)
	switch {
	case strings.Contains(j, "besar"):
		return 4
	case strings.Contains(j, "melahirkan"):
		return 5
	case strings.Contains(j, "luar tanggungan"):
		return 6
	case strings.Contains(j, "sakit"):
		return 2
	case strings.Contains(j, "alasan penting"):
		return 3
	case strings.Contains(j, "tahunan"):
		return 1
	default:
		return 0
	}
}

func namaOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// truncateToWidth shortens s (adding a trailing "...") so it renders within
// maxWidth at the given font size. Used as a safety net for free-text
// fields (nama, jabatan, unit kerja) on the printed forms: those columns are
// sized for typical values, but an unusually long one should never be
// allowed to visually collide with a neighboring column/label -- truncating
// is far less confusing on a printed government form than overlapping text.
// boldWidthSafety compensates for utils.TextWidth only having Helvetica
// Regular metrics (no separate Bold table): a bold glyph typically renders
// ~10% wider than the regular-width estimate. truncateToWidth callers that
// render the *result* in bold multiply their maxWidth by this factor first,
// so the estimate doesn't undershoot and let bold text collide with a
// neighboring column.
const boldWidthSafety = 0.90

func truncateToWidth(s string, maxWidth, size float64) string {
	if utils.TextWidth(s, size) <= maxWidth {
		return s
	}
	const ellipsis = "..."
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		trimmed := strings.TrimRight(string(r), " ")
		if utils.TextWidth(trimmed+ellipsis, size) <= maxWidth {
			return trimmed + ellipsis
		}
	}
	return ellipsis
}

// wrapCapped bungkus teks ke beberapa baris (utils.WrapText) tapi dibatasi
// maksimal maxLines baris -- dipakai untuk jabatan penandatangan (mis. "Plt.
// Kepala Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara")
// yang kadang terlalu panjang untuk satu baris tapi tetap harus terbaca utuh
// (bukan dipotong "...") selama masih muat dalam maxLines baris. Kalau
// setelah dibungkus tetap lebih dari maxLines baris, baris terakhir baru
// dipotong (truncateToWidth) sebagai jaring pengaman supaya tinggi blok
// tanda tangan tidak pernah membengkak tak terkendali.
func wrapCapped(s string, maxWidth, size float64, maxLines int) []string {
	lines := utils.WrapText(s, maxWidth, size)
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	head := append([]string{}, lines[:maxLines-1]...)
	rest := strings.Join(lines[maxLines-1:], " ")
	return append(head, truncateToWidth(rest, maxWidth, size))
}

// wrapJabatan bungkus teks jabatan ke maxLines baris, mencoba sizes dari yang
// terbesar ke terkecil sampai ketemu satu yang muat tanpa perlu dipotong "..."
// -- beberapa jabatan Plt./Kepala Dinas nama lengkapnya sangat panjang (mis.
// "Plt. Kepala Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali
// Utara") dan kalau dipaksa pakai satu ukuran huruf tetap, 2 baris saja
// kadang tidak cukup -- mengecilkan huruf sedikit lebih baik daripada
// memotong sebagian jabatannya. Kalau bahkan pada ukuran terkecil di sizes
// tetap tidak muat, baris terakhir dipotong (wrapCapped) sebagai jaring
// pengaman terakhir.
func wrapJabatan(s string, maxWidth float64, maxLines int, sizes []float64) (lines []string, size float64) {
	for _, sz := range sizes {
		l := utils.WrapText(s, maxWidth, sz)
		if len(l) <= maxLines {
			return l, sz
		}
	}
	smallest := sizes[len(sizes)-1]
	return wrapCapped(s, maxWidth, smallest, maxLines), smallest
}

// drawLetterhead draws the shared "PEMERINTAH KABUPATEN MOROWALI UTARA /
// DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH / ... / KOLONODALE" header with the
// instansi logo, followed by a horizontal rule. Returns the yTop just below
// the rule.
func drawLetterhead(p *utils.PDFPage, marginX, rightX float64) float64 {
	p.Image("logo", marginX, 24, 48, 72)
	centerX := marginX + 48 + (rightX-marginX-48)/2
	p.SetFont(true, 14)
	p.TextCentered(centerX, 42, "PEMERINTAH KABUPATEN MOROWALI UTARA")
	p.TextCentered(centerX, 60, "DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH")
	// Baris alamat tetap dibuat lebih kecil dari judul kop (bukan ikut naik ke
	// 14) -- kalau alamat ikut sebesar judul instansi, baris subjudul ber-
	// alamat ini akan terlihat janggal karena porsinya jadi terlalu besar.
	p.SetFont(false, 10)
	p.TextCentered(centerX, 75, "Alamat : Jln. Bumi Nangka Kompleks Perkantoran Kode Pos (94971)")
	p.SetFont(true, 14)
	p.TextCentered(centerX, 93, "KOLONODALE")
	p.SetLineWidth(1.4)
	p.Line(marginX, 102, rightX, 102)
	p.SetLineWidth(0.75)
	return 102
}

// suratNomorKode returns the "800.1.11.X" kode klasifikasi used in the nomor
// surat of the Surat Rekomendasi Izin Cuti, based on jenis cuti -- each jenis
// cuti has its own kode:
//
//	Cuti Tahunan (termasuk Cuti Tahunan Umroh)  -> 800.1.11.4
//	Cuti Sakit                                  -> 800.1.11.2
//	Cuti Melahirkan                             -> 800.1.11.3
//	Cuti Alasan Penting                         -> 800.1.11.5
//	Cuti Besar                                  -> 800.1.11.6
//	Cuti Luar Tanggungan Negara                 -> 800.1.11.7
//
// Matching is done by keyword (case-insensitive), the same way
// dokumenRequirementsForJenis (pengajuan_cuti.go) matches jenis cuti names --
// more specific keywords are checked before the generic "tahunan" fallback so
// e.g. "Cuti Tahunan Umroh" still resolves to 800.1.11.4. Jenis cuti not
// listed above (or not yet created in master data) fall back to 800.1.11.4,
// the original default before per-jenis kode existed.
func suratNomorKode(jenisNama string) string {
	j := strings.ToLower(jenisNama)
	switch {
	case strings.Contains(j, "sakit"):
		return "800.1.11.2"
	case strings.Contains(j, "melahirkan"):
		return "800.1.11.3"
	case strings.Contains(j, "alasan penting"):
		return "800.1.11.5"
	case strings.Contains(j, "besar"):
		return "800.1.11.6"
	case strings.Contains(j, "luar tanggungan"):
		return "800.1.11.7"
	default:
		return "800.1.11.4" // tahunan (termasuk tahunan umroh) & fallback
	}
}

// resolveSignerInfo looks up the Pegawai record matching pengaturan's
// configured NIP (if any) to determine their CURRENT jabatan, so the
// signature block shows the penandatangan's actual position (e.g. "Kepala
// Dinas Pendidikan dan Kebudayaan Daerah" or, if a Sekretaris is signing as
// pelaksana tugas, "Sekretaris Dinas Pendidikan dan Kebudayaan Daerah")
// instead of a hardcoded "Kepala Dinas" label. Falls back to the generic
// "Kepala Dinas" label when no matching pegawai/jabatan is found (e.g. the
// configured NIP doesn't exist in Data Pegawai, or hasn't been set at all).
//
// Called from two places: approvePengajuan (pengajuan_cuti.go) freezes the
// result onto the pengajuan's TtdNama/TtdNip/TtdJabatan at approval time, and
// downloadFormRekomendasi/downloadFormCuti below fall back to calling it
// live only for pengajuan approved before that snapshot existed (empty
// TtdNip/TtdNama).
func resolveSignerInfo(db *gorm.DB, pengaturan models.PengaturanSurat) (nama, nip, jabatan string) {
	nama = pengaturan.NamaKepalaDinas
	nip = pengaturan.NipKepalaDinas
	jabatan = "Kepala Dinas"
	if strings.TrimSpace(nip) == "" {
		return
	}
	var pegawai models.Pegawai
	if err := db.Preload("Jabatan").Where("nip = ?", nip).First(&pegawai).Error; err == nil && pegawai.Jabatan != nil {
		jabatan = pegawai.Jabatan.Jabatan
	}
	return
}

// buildSignatureQR encodes a Google Search URL for the automatic digital-
// signature stamp printed on both forms (Formulir Cuti & Surat Rekomendasi)
// -- scanning it with an ordinary phone camera opens straight to a Google
// search (not just raw text in a QR-reader app) showing: nama surat, NIP,
// nama pegawai, jenis cuti, lama cuti, siapa yang bertanda tangan beserta
// jabatannya, dan tanggal disetujuinya pengajuan cuti tersebut. A failure
// here (extremely unlikely) is meant to be treated by the caller as "skip
// the stamp", not a hard error: it's a supplementary trust marker, not the
// form's substance.
//
// namaSurat identifies which document this stamp belongs to ("Surat
// Rekomendasi Izin Cuti" / "Formulir Permintaan dan Pemberian Cuti") since
// the same function serves both forms.
func buildSignatureQR(item models.PengajuanCuti, pegawai models.Pegawai, namaSurat string) ([]byte, error) {
	jenisNama := "-"
	if item.JenisCuti != nil {
		jenisNama = item.JenisCuti.Jenis
	}
	tglDisetujui := "-"
	if item.TglApproval != nil {
		tglDisetujui = formatDateID(*item.TglApproval)
	}
	// Format kalimat SENGAJA disederhanakan jadi satu kalimat mengalir --
	// bukan lagi daftar field dipisah tanda "-" -- sesuai permintaan
	// eksplisit: "<nama surat> <jenis cuti> atas nama <nama pegawai> -
	// <NIP pegawai> - yang ditandatangani oleh <nama penandatangan> - <NIP
	// penandatangan> - pada tanggal <tanggal disetujui>". Jabatan
	// penandatangan & rincian lama cuti TIDAK lagi disertakan di sini (di
	// luar permintaan) supaya kalimatnya tetap pendek & tidak terpotong
	// Google saat ditampilkan di ringkasan hasil pencarian.
	query := fmt.Sprintf(
		"%s %s atas nama %s - %s - yang ditandatangani oleh %s - %s - pada tanggal %s",
		namaSurat,
		jenisNama,
		pegawai.Nama,
		namaOrDash(pegawai.NIP),
		namaOrDash(item.TtdNama),
		namaOrDash(item.TtdNip),
		tglDisetujui,
	)
	googleURL := "https://www.google.com/search?q=" + neturl.QueryEscape(query)
	return qrcode.Encode(googleURL, qrcode.Medium, 240)
}

// drawSignatureQR registers (under a page-unique name) and draws the
// automatic signature QR at (x, yTop) sized side x side pt square. Any error
// (encoding or registration) is swallowed on purpose -- see buildSignatureQR.
func drawSignatureQR(doc *utils.PDFDoc, p *utils.PDFPage, name string, item models.PengajuanCuti, pegawai models.Pegawai, namaSurat string, x, yTop, side float64) {
	png, err := buildSignatureQR(item, pegawai, namaSurat)
	if err != nil {
		return
	}
	if err := doc.RegisterImage(name, png); err != nil {
		return
	}
	p.Image(name, x, yTop, side, side)
}

// buildSuratRekomendasi generates the "Surat Rekomendasi Izin Cuti" -- the
// cover letter the Dinas sends to the Bupati/BKPSDM forwarding an approved
// leave request. signerNama/signerNip/signerJabatan identify the
// penandatangan (see resolveSignerInfo / the TtdNama et al. fields).
func buildSuratRekomendasi(item models.PengajuanCuti, pegawai models.Pegawai, signerNama, signerNip, signerJabatan string) ([]byte, error) {
	doc := utils.NewPDFDoc()
	if err := buildSuratRekomendasiPage(doc, item, pegawai, signerNama, signerNip, signerJabatan); err != nil {
		return nil, err
	}
	return doc.Output()
}

// buildSuratRekomendasiPage draws the Surat Rekomendasi page onto an
// EXISTING doc (registering the "logo" & signature-QR images on it) instead
// of always creating its own single-page PDFDoc -- this is what lets
// buildGabungan (lihat di bawah) put this page and buildFormulirCutiPage's
// page together into ONE multi-page PDF (rekomendasi di halaman 1, formulir
// cuti di halaman 2) for the "download sekaligus" endpoint. buildSuratRekomendasi
// above is now just a thin wrapper for the existing single-form endpoint.
func buildSuratRekomendasiPage(doc *utils.PDFDoc, item models.PengajuanCuti, pegawai models.Pegawai, signerNama, signerNip, signerJabatan string) error {
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		return err
	}
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)

	marginX := 42.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX

	tglApproval := time.Now()
	if item.TglApproval != nil {
		tglApproval = *item.TglApproval
	}

	drawLetterhead(p, marginX, rightX)

	// Isi surat (badan/body letter, di bawah kop) dipakai ukuran 12, dan
	// jarak antar baris (line height) distandarkan ke 15 di semua bagian --
	// baik baris tunggal (Nomor/Lampiran/Perihal, blok Yth, tanda tangan)
	// maupun paragraf (MultilineText) -- supaya konsisten dan suratnya
	// penuh mengisi halaman A4 selayaknya surat resmi cetak.
	const lineH = 15.0
	y := 120.0

	jenisNama := ""
	if item.JenisCuti != nil {
		jenisNama = item.JenisCuti.Jenis
	}

	// Bagian nomor urut (di antara kode klasifikasi dan "/Disdikbud") diisi
	// manual oleh administrator/admin lewat updateNomorSurat di
	// pengajuan_cuti.go (item.NomorSurat) -- kalau belum diisi, tetap
	// dikosongkan seperti sebelum field ini ada supaya layout suratnya tidak
	// berubah. Kode klasifikasi ("800.1.11.X") sendiri mengikuti jenis
	// cuti-nya -- lihat suratNomorKode.
	noBagian := strings.TrimSpace(item.NomorSurat)
	if noBagian == "" {
		noBagian = "    "
	}
	nomor := fmt.Sprintf("%s/%s/Disdikbud /%s/ %d", suratNomorKode(jenisNama), noBagian, romanMonth(tglApproval.Month()), tglApproval.Year())
	p.SetFont(false, 12)
	p.Text(marginX, y, "Nomor")
	p.Text(marginX+68, y, ": "+nomor)
	y += lineH
	p.Text(marginX, y, "Lampiran")
	p.Text(marginX+68, y, ": Satu Berkas")
	y += lineH
	p.Text(marginX, y, "Perihal")
	p.Text(marginX+68, y, ": Rekomendasi Izin Cuti")
	y += lineH * 2

	p.SetFont(true, 12)
	p.Text(marginX, y, "Yth. Bupati Morowali Utara")
	y += lineH
	p.Text(marginX, y, "Cq Kepala Badan Kepegawaian dan")
	y += lineH
	p.Text(marginX, y, "Pengembangan SDM")
	y += lineH
	p.SetFont(false, 12)
	p.Text(marginX, y, "Di -")
	y += lineH
	p.Text(marginX+30, y, "Tempat")
	y += lineH * 2

	tglRange := formatTanggalRentang(item.TglMulai, item.TglSelesai)
	para1 := fmt.Sprintf(
		"Menindak lanjuti surat permohonan %s atas nama %s; Tanggal %s dengan ini kami tidak keberatan dan menyetujui permohonan tersebut kami teruskan kepada Bapak untuk ditindaklanjuti (Permohonan Terlampir).",
		jenisNama, strings.ToUpper(pegawai.Nama), tglRange,
	)
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, para1) + lineH

	para2 := "Demikian Surat Permohonan Cuti ini kami teruskan, atas Perkenaanya kami ucapkan terima kasih."
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, para2) + lineH*2

	// Digeser lebih ke kiri (dari pageW-230) supaya kolom tanda tangan lebih
	// lebar -- nama penandatangan yang cukup panjang (mis. "BERNOULLI
	// TANARI, S.Pd.,M.Pd") sebelumnya terpotong karena kolomnya terlalu
	// sempit padahal ruang kosong di sisi kiri masih banyak.
	sigX := pageW - 270
	sigColW := rightX - sigX

	// Nama penandatangan dipotong (truncateToWidth) bila tidak biasa
	// panjangnya. Jabatan (kadang cukup panjang, mis. "Plt. Kepala Dinas
	// Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara") dibungkus
	// ke maks. 2 baris, mengecilkan huruf lebih dulu (wrapJabatan) sebelum
	// akhirnya dipotong, supaya tetap terbaca utuh -- semuanya supaya tidak
	// pernah meluber melewati tepi kanan halaman.
	signerNamaDisp := truncateToWidth(namaOrDash(signerNama), sigColW*boldWidthSafety, 12)
	nameW := utils.TextWidth(signerNamaDisp, 12)
	jabatanLines, jabatanSize := wrapJabatan(namaOrDash(signerJabatan), sigColW, 2, []float64{12, 11, 10.5, 10, 9.5, 9, 8.5, 8})
	jabatanLineH := jabatanSize + 2

	p.Text(sigX, y, "Kolonodale, "+formatDateID(tglApproval)+".")
	y += lineH
	p.SetFont(false, jabatanSize)
	for _, jl := range jabatanLines {
		p.Text(sigX, y, jl)
		y += jabatanLineH
	}
	p.SetFont(false, 12)
	// Barcode/QR tanda tangan otomatis -- SENGAJA diposisikan di tengah lebar
	// NAMA (bukan di tengah kolom tanda tangan) sesuai permintaan eksplisit,
	// di ruang kosong yang dulunya disediakan untuk tanda tangan basah
	// (lihat drawSignatureQR/buildSignatureQR). qrCenterX di-clamp supaya QR
	// tidak pernah keluar dari kolom tanda tangan kalau nama kebetulan sangat
	// pendek/panjang. Ukuran 150x150 (dari semula 40 lalu 80) supaya benar-
	// benar mudah dipindai kamera HP, sepadan dengan ukuran QR tanda tangan
	// pada formulir resmi lain (mis. Surat Izin Cuti BKPSDM).
	const qrSide = 150.0
	qrCenterX := sigX + nameW/2
	if qrCenterX-qrSide/2 < sigX {
		qrCenterX = sigX + qrSide/2
	}
	if qrCenterX+qrSide/2 > rightX {
		qrCenterX = rightX - qrSide/2
	}
	drawSignatureQR(doc, p, "ttd_qr_rekomendasi", item, pegawai, "Surat Rekomendasi Izin Cuti", qrCenterX-qrSide/2, y+4, qrSide)
	y += qrSide + 18

	p.SetFont(true, 12)
	p.Text(sigX, y, signerNamaDisp)
	p.Line(sigX, y+3, sigX+nameW, y+3)
	y += 16
	p.SetFont(false, 12)
	p.Text(sigX, y, "NIP: "+namaOrDash(signerNip)+".")

	return nil
}

// buildFormulirCuti generates the official "Formulir Permintaan dan
// Pemberian Cuti" (leave request/grant form).
func buildFormulirCuti(item models.PengajuanCuti, pegawai models.Pegawai, signerNama, signerNip, signerJabatan string, jatah map[int]models.JatahCuti) ([]byte, error) {
	doc := utils.NewPDFDoc()
	if err := buildFormulirCutiPage(doc, item, pegawai, signerNama, signerNip, signerJabatan, jatah); err != nil {
		return nil, err
	}
	return doc.Output()
}

// buildGabungan menggabungkan Surat Rekomendasi & Formulir Cuti (masing-
// masing sudah otomatis memuat stempel QR tanda tangan) jadi SATU berkas PDF
// multi-halaman -- Surat Rekomendasi di halaman 1, Formulir Cuti di halaman
// 2 -- untuk tombol "Download Sekaligus" di akun pegawai. utils.PDFDoc sudah
// mendukung banyak halaman dengan ukuran kertas berbeda-beda per halaman
// (A4 untuk rekomendasi, Legal untuk formulir cuti) dalam SATU dokumen, jadi
// ini bukan menggabungkan dua berkas PDF terpisah secara biner (yang butuh
// pustaka PDF pihak ketiga) melainkan menggambar kedua halaman itu ke dalam
// doc yang sama sebelum di-Output() sekali saja.
func buildGabungan(item models.PengajuanCuti, pegawai models.Pegawai, signerNama, signerNip, signerJabatan string, jatah map[int]models.JatahCuti) ([]byte, error) {
	doc := utils.NewPDFDoc()
	if err := buildSuratRekomendasiPage(doc, item, pegawai, signerNama, signerNip, signerJabatan); err != nil {
		return nil, err
	}
	if err := buildFormulirCutiPage(doc, item, pegawai, signerNama, signerNip, signerJabatan, jatah); err != nil {
		return nil, err
	}
	return doc.Output()
}

// buildFormulirCutiPage is buildFormulirCuti's counterpart to
// buildSuratRekomendasiPage above -- draws onto an existing doc instead of
// always creating its own, so buildGabungan can combine both forms into one
// multi-page PDF.
func buildFormulirCutiPage(doc *utils.PDFDoc, item models.PengajuanCuti, pegawai models.Pegawai, signerNama, signerNip, signerJabatan string, jatah map[int]models.JatahCuti) error {
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		return err
	}
	// Formulir dicetak di atas kertas Legal (8.5" x 14") agar seluruh tabel
	// (I-VII) selalu muat dalam 1 lembar, sesuai kebiasaan cetak formulir ini
	// di kantor.
	p := utils.NewPDFPage(utils.PageWidthLegal, utils.PageHeightLegal)
	doc.AddPage(p)

	marginX := 40.0
	pageW := utils.PageWidthLegal
	rightX := pageW - marginX

	tglApproval := time.Now()
	if item.TglApproval != nil {
		tglApproval = *item.TglApproval
	}

	drawLetterhead(p, marginX, rightX)

	// Blok "Kepada" rata kanan, di bawah kop surat. Ukuran huruf & jarak baris
	// distandarkan sama seperti isi tabel di bawahnya (12 / lineH 15).
	blockX := pageW/2 + 30
	y := 122.0
	p.SetFont(false, 12)
	p.Text(blockX, y, "Kolonodale, "+formatDateID(tglApproval)+".")
	y += 15
	p.Text(blockX, y, "Kepada")
	y += 15
	p.Text(blockX, y, "Yth. Bupati Morowali Utara")
	y += 15
	// Dipecah jadi 2 baris (seperti pada Surat Rekomendasi) -- pada ukuran
	// 12pt, kalimat "Cq. Kepala Badan Kepegawaian dan Pengembangan SDM" tidak
	// lagi muat dalam satu baris di kolom kanan tanpa meluber ke luar tepi
	// halaman.
	p.Text(blockX, y, "Cq. Kepala Badan Kepegawaian dan")
	y += 15
	p.Text(blockX, y, "Pengembangan SDM")
	y += 15
	p.Text(blockX, y, "Di")
	y += 15
	p.Text(blockX+20, y, "Kolonodale")

	// Jarak ke judul & ke tabel dipadatkan (dari 249/+24 semula) supaya ada
	// ruang ekstra di tabel Baris VII untuk QR tanda tangan otomatis yang
	// diperbesar tanpa membuat formulir meluber ke halaman ke-2.
	y = 225
	p.SetFont(true, 13)
	p.TextCentered(pageW/2, y, "FORMULIR PERMINTAAN DAN PEMBERIAN CUTI")
	y += 18

	// ---- tabel utama ----
	// Ukuran huruf isi tabel (I-VII) dinaikkan (mengikuti perlakuan yang
	// sama pada Surat Rekomendasi) dan tinggi tiap baris ikut diperbesar
	// proporsional, supaya keseluruhan tabel memenuhi kertas Legal dengan
	// baik alih-alih menyisakan banyak ruang kosong di bagian bawah.
	tableX := marginX
	tableTop := y
	labelColW := 20.0
	contentX := tableX + labelColW
	contentW := rightX - contentX
	pad := 4.0

	rowBottom := func(rowTop, rowH float64, romawi string) float64 {
		bottom := rowTop + rowH
		p.SetFont(true, 10)
		p.Text(tableX+3, rowTop+15, romawi)
		p.Line(tableX, bottom, rightX, bottom)
		p.Line(tableX+labelColW, rowTop, tableX+labelColW, bottom)
		return bottom
	}

	// Row I: Data Pegawai
	rowTop := tableTop
	// Dipadatkan dari 80 -- baris ini hanya berisi 3 baris teks tetap
	// (Nama/NIP, Jabatan/Masa Kerja, Unit Kerja) yang berakhir di rowTop+63,
	// jadi 72 masih menyisakan margin bawah yang wajar. Ruang yang dihemat
	// dialihkan ke Baris VII supaya QR tanda tangan bisa diperbesar lagi
	// (150->170) tanpa membuat formulir meluber ke halaman ke-2.
	rowH := 72.0
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Data Pegawai")
	p.SetFont(false, 12)
	rightHalfX := contentX + contentW*0.62
	jabatanNama := "-"
	if pegawai.Jabatan != nil {
		jabatanNama = pegawai.Jabatan.Jabatan
	}
	unitNama := "-"
	if pegawai.UnitKerja != nil {
		unitNama = pegawai.UnitKerja.Unit
	}
	// Baris "Nama" & "Jabatan" berbagi baris yang sama dengan "NIP"/"Masa
	// Kerja" di kolom kanan -- nilai yang tidak biasa panjangnya dipotong
	// (truncateToWidth) supaya tidak pernah bertumpukan dengan label kolom
	// kanan tersebut. Baris "Unit Kerja" berdiri sendiri (tanpa pasangan di
	// kolom kanan) sehingga diberi jatah lebar penuh sampai tepi tabel.
	valueX := contentX + pad + 62
	pairedMaxW := rightHalfX - valueX - 6
	fullMaxW := rightX - valueX - pad
	namaValue := truncateToWidth(strings.ToUpper(pegawai.Nama), pairedMaxW*boldWidthSafety, 12)
	jabatanValue := truncateToWidth(jabatanNama, pairedMaxW, 12)
	unitValue := truncateToWidth(unitNama, fullMaxW, 12)
	p.Text(contentX+pad, rowTop+33, "Nama")
	p.SetFont(true, 12)
	p.Text(valueX, rowTop+33, ": "+namaValue)
	p.SetFont(false, 12)
	p.Text(rightHalfX, rowTop+33, "NIP")
	p.Text(rightHalfX+46, rowTop+33, ": "+namaOrDash(pegawai.NIP))
	p.Text(contentX+pad, rowTop+48, "Jabatan")
	p.Text(valueX, rowTop+48, ": "+jabatanValue)
	p.Text(rightHalfX, rowTop+48, "Masa Kerja")
	p.Text(rightHalfX+64, rowTop+48, ": "+masaKerjaText(pegawai.TMT, tglApproval))
	p.Text(contentX+pad, rowTop+63, "Unit Kerja")
	p.Text(valueX, rowTop+63, ": "+unitValue)
	y = rowBottom(rowTop, rowH, "I")

	// Row II: Jenis Cuti yang di ambil -- dirender sebagai tabel bergrid
	// penuh (kolom kosong tipis | nomor | label | kotak centang, diulang
	// untuk kelompok kiri 1-3 dan kanan 4-6, dengan garis horizontal di
	// antara tiap baris) supaya semirip mungkin dengan formulir cetak asli.
	rowTop = y
	const itemHRowII = 15.0
	rowH = 26.0 + itemHRowII*3.0
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Jenis Cuti yang di ambil")
	p.SetFont(false, 11)
	selected := jenisCutiCheckboxIndex(func() string {
		if item.JenisCuti != nil {
			return item.JenisCuti.Jenis
		}
		return ""
	}())
	leftLabels := []struct {
		num   int
		label string
	}{{1, "Cuti Tahunan"}, {2, "Cuti Sakit"}, {3, "Cuti Karena Alasan Penting"}}
	rightLabels := []struct {
		num   int
		label string
	}{{4, "Cuti Besar"}, {5, "Cuti Melahirkan"}, {6, "Cuti di Luar Tanggungan Negara"}}
	midX := contentX + contentW*0.52
	checkSize := 11.0
	blankColW := 6.0
	checkColW := 26.0
	trailColW := 6.0
	leftNumX := contentX + blankColW
	leftLabelX := leftNumX + 16.0
	leftCheckX := midX - checkColW
	rightNumX := midX + blankColW
	rightLabelX := rightNumX + 16.0
	rightCheckX := rightX - trailColW - checkColW

	gridTop := rowTop + 26.0
	itemH := itemHRowII
	for i := 0; i < 3; i++ {
		cellTop := gridTop + itemH*float64(i)
		ly := cellTop + itemH*0.75
		l := leftLabels[i]
		p.Text(leftNumX+2, ly, fmt.Sprintf("%d", l.num))
		p.Text(leftLabelX+3, ly, l.label)
		// Kotak centang di formulir asli bukan kotak kecil terpisah -- sel
		// tabelnya sendiri berfungsi sebagai "kotak", jadi tanda centang
		// digambar langsung di tengah sel tanpa kotak tambahan.
		if selected == l.num {
			p.Checkmark(leftCheckX+(checkColW-checkSize)/2, cellTop+(itemH-checkSize)/2, checkSize)
		}
		r := rightLabels[i]
		p.Text(rightNumX+2, ly, fmt.Sprintf("%d", r.num))
		p.Text(rightLabelX+3, ly, r.label)
		if selected == r.num {
			p.Checkmark(rightCheckX+(checkColW-checkSize)/2, cellTop+(itemH-checkSize)/2, checkSize)
		}
		if i > 0 {
			p.Line(contentX, cellTop, rightX, cellTop)
		}
	}
	// garis horizontal di bawah judul "Jenis Cuti yang di ambil", memisahkan
	// dari grid nomor/label/centang di bawahnya.
	p.Line(contentX, gridTop, rightX, gridTop)
	// garis-garis vertikal grid (kolom kosong | nomor | label | centang),
	// diulang untuk kelompok kiri dan kanan, plus kolom kosong di ujung kanan.
	for _, vx := range []float64{leftNumX, leftLabelX, leftCheckX, midX, rightNumX, rightLabelX, rightCheckX, rightX - trailColW} {
		p.Line(vx, gridTop, vx, rowTop+rowH)
	}
	y = rowBottom(rowTop, rowH, "II")

	// Row III: Alasan Cuti -- tinggi baris menyesuaikan panjang teks alasan
	// (bebas diisi pegawai) supaya tidak pernah meluber ke baris berikutnya.
	rowTop = y
	alasanFontSize := 12.0
	alasanLineHeight := 15.0
	// Dibatasi maks. 2 baris (baris terakhir dipotong "..." kalau lebih
	// panjang, lihat wrapCapped) -- alasan cuti bebas diisi pegawai dan bisa
	// sangat panjang; tanpa batas ini baris III bisa tumbuh tak terbatas dan
	// mendorong Baris VII (yang kini bertumpuk 1 kolom: jabatan - QR - nama -
	// NIP, jadi butuh ruang vertikal ekstra) meluber ke halaman ke-2.
	const alasanMaxLines = 2
	alasanLines := wrapCapped(namaOrDash(item.AlasanCuti), contentW-2*pad, alasanFontSize, alasanMaxLines)
	rowH = 45.0 + float64(len(alasanLines))*alasanLineHeight
	if rowH < 60 {
		rowH = 60
	}
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Alasan Cuti")
	p.SetFont(false, alasanFontSize)
	alasanTy := rowTop + 33
	for _, al := range alasanLines {
		p.Text(contentX+pad, alasanTy, al)
		alasanTy += alasanLineHeight
	}
	y = rowBottom(rowTop, rowH, "III")

	// Row IV: Lama Cuti
	rowTop = y
	// Dipadatkan dari 48 -- isinya cuma 1 baris teks tetap (rowTop+33) yang
	// tidak pernah wrap, jadi 40 masih aman. Ruang yang dihemat dialihkan ke
	// Baris VII (lihat komentar di Baris I).
	rowH = 40.0
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Lama Cuti")
	p.SetFont(false, 12)
	p.Text(contentX+pad, rowTop+33, fmt.Sprintf("Selama %d Hari   Mulai Tanggal %s", item.JumlahHari, formatTanggalRentang(item.TglMulai, item.TglSelesai)))
	y = rowBottom(rowTop, rowH, "IV")

	// Row V: Catatan Cuti (riwayat kuota cuti tahunan N-2..N + legenda jenis
	// cuti) -- juga dirender bergrid penuh: tabel kuota di kiri (kolom
	// kosong | label | Tahun | Sisa | Keterangan, dengan garis antar baris)
	// dan daftar legenda 1-6 di kanan (kolom kosong | nomor | label, dengan
	// garis antar baris), meniru formulir cetak asli.
	rowTop = y
	// Tinggi tiap baris di dalam grid (tabel kuota kiri 5 baris & legenda
	// kanan 6 baris) distandarkan 15pt (sama seperti lineH di tempat lain),
	// bukan proporsional membagi rowH tetap -- supaya baris tidak lebih
	// lebar dari yang dibutuhkan dan seluruh formulir tetap muat dalam 1
	// lembar kertas Legal. rowH mengikuti sisi grid yang lebih tinggi
	// (legenda kanan, 6 baris).
	const itemHRowV = 15.0
	rowH = 26.0 + itemHRowV*6.0
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Catatan Cuti")
	quotaW := contentW * 0.5
	blankColW2 := 6.0
	quotaEnd := contentX + quotaW
	numColX := contentX + blankColW2
	colTahunX := numColX + 16.0
	remaining := quotaEnd - colTahunX
	// Kolom "Keterangan" diberi jatah lebar paling besar (nilainya selalu
	// "Cuti Tahunan", teks terpanjang di baris ini pada ukuran 12/10.5) --
	// kolom Tahun & Sisa cukup sempit karena isinya cuma "N-1"/angka.
	colSisaX := colTahunX + remaining*0.30
	colKetX := colSisaX + remaining*0.22
	p.SetFont(false, 10.5)

	// legenda jenis cuti 1-6 -- dipakai baik untuk judul tabel kuota kiri
	// (harus mengikuti jenis cuti yang SEBENARNYA dipilih pegawai, bukan
	// selalu "1 Cuti Tahunan") maupun daftar bernomor di kanan (selalu
	// menampilkan keenam jenis, lihat bawah).
	legend := []string{"Cuti Tahunan", "Cuti Sakit", "Cuti Karena Alasan Penting", "Cuti Besar", "Cuti Melahirkan", "Cuti di Luar Tanggungan Negara"}

	// Tabel kuota kiri: baris judul mengikuti nomor & label jenis cuti yang
	// dipilih pegawai (sama seperti tanda centang pada Baris II) -- SEBELUM
	// perbaikan ini judul selalu tertulis "1 Cuti Tahunan" walaupun jenis
	// cuti yang diambil bukan Cuti Tahunan (mis. Cuti Melahirkan harusnya
	// "5 Cuti Melahirkan"). Fallback ke "1 Cuti Tahunan" hanya kalau jenis
	// cutinya tidak dikenali/kosong (selected == 0, lihat jenisCutiCheckboxIndex).
	catatanNum := selected
	if catatanNum < 1 || catatanNum > len(legend) {
		catatanNum = 1
	}
	catatanLabel := legend[catatanNum-1]

	// lalu baris header Tahun/Sisa/Keterangan, lalu 3 baris data (N-1/N-2/N)
	// -- persis urutan & struktur pada formulir cetak asli.
	gridTop3 := rowTop + 26.0
	rowHLeft := (rowH - 26.0) / 5.0
	titleTop := gridTop3
	titleLy := titleTop + rowHLeft*0.68
	p.Text(numColX+2, titleLy, fmt.Sprintf("%d", catatanNum))
	p.Text(colTahunX+3, titleLy, catatanLabel)

	headerTop := gridTop3 + rowHLeft
	headerLy := headerTop + rowHLeft*0.68
	p.Text(colTahunX+3, headerLy, "Tahun")
	p.Text(colSisaX+3, headerLy, "Sisa")
	p.Text(colKetX+3, headerLy, "Keterangan")

	years := []struct {
		label string
		year  int
	}{{"N-1", item.TglMulai.Year() - 1}, {"N-2", item.TglMulai.Year() - 2}, {"N", item.TglMulai.Year()}}
	for i, yr := range years {
		cellTop := gridTop3 + rowHLeft*float64(2+i)
		ly := cellTop + rowHLeft*0.68
		p.Text(colTahunX+3, ly, yr.label)
		if jc, ok := jatah[yr.year]; ok {
			p.Text(colSisaX+3, ly, fmt.Sprintf("%d", jc.Sisa()))
			p.Text(colKetX+3, ly, catatanLabel)
		} else {
			p.Text(colSisaX+3, ly, "-")
		}
	}
	// garis horizontal antar baris tabel kuota (setelah judul, header, dan
	// tiap baris data).
	for i := 1; i <= 4; i++ {
		hy := gridTop3 + rowHLeft*float64(i)
		p.Line(contentX, hy, quotaEnd, hy)
	}

	legendNumX := quotaEnd + blankColW2
	legendLabelX := legendNumX + 16.0
	legendTrailW := 6.0
	itemH2 := itemHRowV
	for i, l := range legend {
		cellTop := gridTop3 + itemH2*float64(i)
		ly := cellTop + itemH2*0.68
		p.Text(legendNumX+2, ly, fmt.Sprintf("%d", i+1))
		p.Text(legendLabelX+3, ly, l)
		if i > 0 {
			p.Line(quotaEnd, cellTop, rightX, cellTop)
		}
	}

	// garis horizontal di bawah judul "Catatan Cuti", membentang penuh.
	p.Line(contentX, gridTop3, rightX, gridTop3)
	// garis-garis vertikal grid.
	for _, vx := range []float64{numColX, colTahunX, colSisaX, colKetX} {
		p.Line(vx, gridTop3, vx, rowTop+rowH)
	}
	p.Line(quotaEnd, rowTop, quotaEnd, rowTop+rowH)
	for _, vx := range []float64{legendNumX, legendLabelX, rightX - legendTrailW} {
		p.Line(vx, gridTop3, vx, rowTop+rowH)
	}
	y = rowBottom(rowTop, rowH, "V")

	// Row VI: Alamat Selama Menjalankan Cuti + tanda tangan pegawai. Jarak
	// dipadatkan (dari 140/68/117/121/136 semula) untuk memberi ruang ekstra
	// ke Baris VII (blok tanda tangan otomatis) tanpa meluber ke
	// halaman ke-2 -- alamat juga dibatasi maks. 4 baris (jaring pengaman
	// yang sama seperti Alasan Cuti di Baris III).
	rowTop = y
	rowH = 124.0
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Alamat Selama Menjalankan Cuti")
	p.SetFont(false, 12)
	alamatLines := wrapCapped(namaOrDash(item.AlamatSelamaCuti), rightHalfX-contentX-pad-8, 12, 4)
	alamatTy := rowTop + 33.0
	for _, al := range alamatLines {
		p.Text(contentX+pad, alamatTy, al)
		alamatTy += 15
	}
	p.Text(rightHalfX, rowTop+33, "Telp: "+namaOrDash(pegawai.NoHP))
	// Blok tanda tangan pegawai diposisikan di tengah kolom kanan (bukan
	// titik tetap) dan nama yang tidak biasa panjangnya dipotong supaya
	// tidak pernah meluber melewati batas kanan tabel.
	sigColCenterVI := rightHalfX + (rightX-rightHalfX)/2
	sigMaxWVI := (rightX - rightHalfX) - 16
	p.SetFont(false, 11)
	p.TextCentered(sigColCenterVI, rowTop+55, "Hormat Saya")
	p.SetFont(true, 12)
	namaPegawaiUpper := truncateToWidth(strings.ToUpper(pegawai.Nama), sigMaxWVI*boldWidthSafety, 12)
	p.TextCentered(sigColCenterVI, rowTop+100, namaPegawaiUpper)
	w := utils.TextWidth(namaPegawaiUpper, 12)
	p.Line(sigColCenterVI-w/2, rowTop+104, sigColCenterVI+w/2, rowTop+104)
	p.SetFont(false, 11)
	p.TextCentered(sigColCenterVI, rowTop+119, "NIP: "+namaOrDash(pegawai.NIP)+".")
	y = rowBottom(rowTop, rowH, "VI")

	// Row VII: Pertimbangan Atasan Langsung -- 4 opsi dibingkai pada satu
	// baris penuh di bawah judul (seperti formulir cetak asli), diikuti
	// stempel tanda tangan otomatis di bawahnya. Blok tanda tangan BERTUMPUK
	// 1 kolom rata tengah -- jabatan di atas, QR persis di tengah (di antara
	// jabatan & nama), lalu nama dan NIP di bawah QR -- meniru gaya pada
	// Surat Rekomendasi & contoh surat cetak asli (BKPSDM), atas permintaan
	// eksplisit supaya QR "di tengah-tengah" alih-alih sejajar di kiri dengan
	// teks di kanan. Karena layout bertumpuk butuh ruang vertikal jauh lebih
	// besar dibanding versi sejajar sebelumnya, QR diperkecil dari 170x170
	// ke 120x120 (baris III alasan cuti juga dipangkas ke maks. 2 baris,
	// lihat komentar di atas) supaya formulir tetap muat 1 halaman Legal.
	rowTop = y
	p.SetFont(true, 12)
	p.Text(contentX+pad, rowTop+18, "Pertimbangan Atasan Langsung")
	opts := []string{"Disetujui", "Perubahan", "Ditangguhkan", "Tidak Disetujui"}
	optW := contentW / float64(len(opts))
	gridTopVII := rowTop + 24.0
	optHeaderH := 22.0
	checkSize2 := 10.5
	p.SetFont(false, 10.5)
	for i, opt := range opts {
		colStart := contentX + optW*float64(i)
		colCenter := colStart + optW/2
		tw := utils.TextWidth(opt, 10.5)
		// pengajuan hanya dicetak setelah status "disetujui", jadi opsi
		// pertama otomatis ditandai centang -- langsung di dalam sel,
		// tanpa kotak tambahan (meniru gaya centang pada Baris II).
		if i == 0 {
			startX := colCenter - (checkSize2+4+tw)/2
			p.Checkmark(startX, gridTopVII+(optHeaderH-checkSize2)/2, checkSize2)
			p.Text(startX+checkSize2+4, gridTopVII+optHeaderH*0.72, opt)
		} else {
			p.TextCentered(colCenter, gridTopVII+optHeaderH*0.72, opt)
		}
		if i > 0 {
			p.Line(colStart, gridTopVII, colStart, gridTopVII+optHeaderH)
		}
	}
	p.Line(contentX, gridTopVII, rightX, gridTopVII)
	p.Line(contentX, gridTopVII+optHeaderH, rightX, gridTopVII+optHeaderH)

	centerVII := contentX + contentW/2
	const capWidthVII = 420.0
	jabatanWVII := contentW - 2*pad
	if jabatanWVII > capWidthVII {
		jabatanWVII = capWidthVII
	}
	jabatanLinesVII, jabatanSizeVII := wrapJabatan(namaOrDash(signerJabatan), jabatanWVII, 2, []float64{11, 10, 9.5, 9, 8.5, 8, 7.5})
	jabatanLineHVII := jabatanSizeVII + 3
	p.SetFont(false, jabatanSizeVII)
	jabatanTopVII := gridTopVII + optHeaderH + 12.0
	for i, jl := range jabatanLinesVII {
		p.TextCentered(centerVII, jabatanTopVII+float64(i)*jabatanLineHVII, jl)
	}
	// jabatanBottomVII = baseline baris jabatan terakhir.
	jabatanBottomVII := jabatanTopVII + float64(len(jabatanLinesVII)-1)*jabatanLineHVII

	const qrSideVII = 120.0
	qrTopVII := jabatanBottomVII + 8.0
	qrX := centerVII - qrSideVII/2
	drawSignatureQR(doc, p, "ttd_qr_formulir", item, pegawai, "Formulir Permintaan dan Pemberian Cuti", qrX, qrTopVII, qrSideVII)

	nameTopVII := qrTopVII + qrSideVII + 14.0
	kepalaDinasNama := truncateToWidth(namaOrDash(signerNama), capWidthVII*boldWidthSafety, 12)
	p.SetFont(true, 12)
	p.TextCentered(centerVII, nameTopVII, kepalaDinasNama)
	w2 := utils.TextWidth(kepalaDinasNama, 12)
	p.Line(centerVII-w2/2, nameTopVII+4, centerVII+w2/2, nameTopVII+4)
	p.SetFont(false, 11)
	nipBaselineVII := nameTopVII + 18.0
	p.TextCentered(centerVII, nipBaselineVII, "NIP: "+namaOrDash(signerNip)+".")

	// rowH menampung seluruh blok bertumpuk (header opsi + jabatan + QR +
	// nama + NIP) + padding bawah.
	rowH = (nipBaselineVII - rowTop) + 8.0
	y = rowBottom(rowTop, rowH, "VII")

	// bingkai luar tabel
	p.Line(tableX, tableTop, tableX, y)
	p.Line(rightX, tableTop, rightX, y)
	p.Line(tableX, tableTop, rightX, tableTop)

	return nil
}

// ---- HTTP handlers ----

// pengaturanSuratOrDefault loads the singleton pengaturan surat row, falling
// back to an empty value if it somehow doesn't exist yet (EnsurePengaturanSurat
// creates it on every server start).
func pengaturanSuratOrDefault(db *gorm.DB) models.PengaturanSurat {
	var p models.PengaturanSurat
	db.First(&p, 1)
	return p
}

func loadJatahHistory(db *gorm.DB, idPegawai uint, years []int) map[int]models.JatahCuti {
	result := map[int]models.JatahCuti{}
	var rows []models.JatahCuti
	db.Where("id_pegawai = ? AND tahun IN ?", idPegawai, years).Find(&rows)
	for _, r := range rows {
		result[r.Tahun] = r
	}
	return result
}

func writePDFResponse(w http.ResponseWriter, r *http.Request, data []byte, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	w.Write(data)
}

// loadApprovedPengajuanForForm fetches a pengajuan and checks that it is
// disetujui (approved) and that the caller may access it -- both forms are
// only generatable once a leave request has actually been granted.
func loadApprovedPengajuanForForm(w http.ResponseWriter, r *http.Request, db *gorm.DB) (models.PengajuanCuti, models.Pegawai, bool) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.Preload("JenisCuti").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return item, models.Pegawai{}, false
	}
	var pegawai models.Pegawai
	if err := db.Preload("Jabatan").Preload("UnitKerja").First(&pegawai, item.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return item, models.Pegawai{}, false
	}
	item.Pegawai = &pegawai
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return item, pegawai, false
	}
	if item.Status != models.StatusDisetuju {
		utils.Error(w, http.StatusBadRequest, "formulir hanya bisa dicetak setelah pengajuan disetujui")
		return item, pegawai, false
	}
	return item, pegawai, true
}

// resolveItemSignerTrio returns the signer nama/nip/jabatan snapshot frozen
// onto the pengajuan at approval time (TtdNama/TtdNip/TtdJabatan). For
// pengajuan approved before this feature existed (snapshot empty), it falls
// back to resolving the CURRENT signer from pengaturan surat live, matching
// the old (pre-snapshot) behaviour for those legacy documents.
func resolveItemSignerTrio(db *gorm.DB, item models.PengajuanCuti) (nama, nip, jabatan string) {
	if strings.TrimSpace(item.TtdNama) != "" || strings.TrimSpace(item.TtdNip) != "" {
		return item.TtdNama, item.TtdNip, item.TtdJabatan
	}
	return resolveSignerInfo(db, pengaturanSuratOrDefault(db))
}

func downloadFormRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, pegawai, ok := loadApprovedPengajuanForForm(w, r, db)
	if !ok {
		return
	}
	signerNama, signerNip, signerJabatan := resolveItemSignerTrio(db, item)
	pdfBytes, err := buildSuratRekomendasi(item, pegawai, signerNama, signerNip, signerJabatan)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat formulir: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("surat_rekomendasi_cuti_%s.pdf", pegawai.NIP))
}

func downloadFormCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, pegawai, ok := loadApprovedPengajuanForForm(w, r, db)
	if !ok {
		return
	}
	signerNama, signerNip, signerJabatan := resolveItemSignerTrio(db, item)
	year := item.TglMulai.Year()
	jatah := loadJatahHistory(db, pegawai.ID, []int{year, year - 1, year - 2})
	pdfBytes, err := buildFormulirCuti(item, pegawai, signerNama, signerNip, signerJabatan, jatah)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat formulir: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("formulir_cuti_%s.pdf", pegawai.NIP))
}

// downloadFormGabungan serves BOTH forms as one 2-halaman PDF (Surat
// Rekomendasi di halaman 1, Formulir Cuti di halaman 2) untuk tombol
// "Download Sekaligus" -- supaya pegawai tidak perlu mengunduh dua berkas
// terpisah. Sama seperti downloadFormRekomendasi/downloadFormCuti, hanya
// bisa diakses setelah pengajuan disetujui (lihat loadApprovedPengajuanForForm)
// dan otomatis tersedia tanpa admin perlu upload apa pun -- keduanya sudah
// memuat stempel QR tanda tangan otomatis.
func downloadFormGabungan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, pegawai, ok := loadApprovedPengajuanForForm(w, r, db)
	if !ok {
		return
	}
	signerNama, signerNip, signerJabatan := resolveItemSignerTrio(db, item)
	year := item.TglMulai.Year()
	jatah := loadJatahHistory(db, pegawai.ID, []int{year, year - 1, year - 2})
	pdfBytes, err := buildGabungan(item, pegawai, signerNama, signerNip, signerJabatan, jatah)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat formulir: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("cuti_%s.pdf", pegawai.NIP))
}

// uploadFormSigned lets admin/administrator upload the scan of a form that
// has actually been physically signed by Kepala Dinas ("ttd" = tanda
// tangan). It is stored as an ordinary PengajuanDokumen row (jenis
// "ttd_rekomendasi" / "ttd_cuti"), which means the existing
// GET /api/pengajuan-cuti/{id}/dokumen/{jenis} endpoint (with its ?inline=1
// support and canAccessPengajuan access control) already knows how to serve
// it back -- including to the pegawai who owns the pengajuan. Uploading
// again simply replaces the previous file, so admin can correct a mistake by
// re-uploading.
func uploadFormSigned(w http.ResponseWriter, r *http.Request, db *gorm.DB, jenis, label string) {
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.StatusDisetuju {
		utils.Error(w, http.StatusBadRequest, "berkas bertanda tangan hanya bisa diupload setelah pengajuan disetujui")
		return
	}
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas (maksimal 15MB)")
		return
	}
	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas wajib diupload")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, JPG, atau PNG")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}

	var existing models.PengajuanDokumen
	if err := db.Where("id_pengajuan = ? AND jenis = ?", item.ID, jenis).First(&existing).Error; err == nil {
		existing.NamaFile = fh.Filename
		existing.File = data
		existing.Label = label
		existing.PerluPerbaikan = false
		if err := db.Save(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan berkas: "+err.Error())
			return
		}
	} else {
		doc := models.PengajuanDokumen{IDPengajuan: item.ID, Jenis: jenis, Label: label, NamaFile: fh.Filename, File: data}
		if err := db.Create(&doc).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan berkas: "+err.Error())
			return
		}
	}
	utils.Success(w, "berkas bertanda tangan berhasil diupload", map[string]string{"jenis": jenis, "nama_file": fh.Filename})
}

// deleteFormSigned removes a previously uploaded signed-form scan (e.g. to
// let admin correct a wrong upload before re-uploading the right one).
func deleteFormSigned(w http.ResponseWriter, r *http.Request, db *gorm.DB, jenis string) {
	id := r.PathValue("id")
	if err := db.Where("id_pengajuan = ? AND jenis = ?", id, jenis).Delete(&models.PengajuanDokumen{}).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus berkas: "+err.Error())
		return
	}
	utils.Success(w, "berkas berhasil dihapus", nil)
}
