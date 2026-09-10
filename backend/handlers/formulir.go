package handlers

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cuti-app/assets"
	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

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

// drawLetterhead draws the shared "PEMERINTAH KABUPATEN MOROWALI UTARA /
// DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH / ... / KOLONODALE" header with the
// instansi logo, followed by a horizontal rule. Returns the yTop just below
// the rule.
func drawLetterhead(p *utils.PDFPage, marginX, rightX float64) float64 {
	p.Image("logo", marginX, 28, 44, 64)
	centerX := marginX + 44 + (rightX-marginX-44)/2
	p.SetFont(true, 12)
	p.TextCentered(centerX, 40, "PEMERINTAH KABUPATEN MOROWALI UTARA")
	p.TextCentered(centerX, 55, "DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH")
	p.SetFont(false, 9)
	p.TextCentered(centerX, 68, "Alamat : Jln. Bumi Nangka Kompleks Perkantoran Kode Pos (94971)")
	p.SetFont(true, 12)
	p.TextCentered(centerX, 83, "KOLONODALE")
	p.SetLineWidth(1.4)
	p.Line(marginX, 92, rightX, 92)
	p.SetLineWidth(0.75)
	return 92
}

// buildSuratRekomendasi generates the "Surat Rekomendasi Izin Cuti" -- the
// cover letter the Dinas sends to the Bupati/BKPSDM forwarding an approved
// leave request.
func buildSuratRekomendasi(item models.PengajuanCuti, pegawai models.Pegawai, pengaturan models.PengaturanSurat) ([]byte, error) {
	doc := utils.NewPDFDoc()
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		return nil, err
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

	y := 112.0
	nomor := fmt.Sprintf("800.1.11.4/     /Disdikbud /%s/ %d", romanMonth(tglApproval.Month()), tglApproval.Year())
	p.SetFont(false, 10)
	p.Text(marginX, y, "Nomor")
	p.Text(marginX+68, y, ": "+nomor)
	y += 15
	p.Text(marginX, y, "Lampiran")
	p.Text(marginX+68, y, ": Satu Berkas")
	y += 15
	p.Text(marginX, y, "Perihal")
	p.Text(marginX+68, y, ": Rekomendasi Izin Cuti")
	y += 28

	p.SetFont(true, 10)
	p.Text(marginX, y, "Yth. Bupati Morowali Utara")
	y += 14
	p.Text(marginX, y, "Cq Kepala Badan Kepegawaian dan")
	y += 14
	p.Text(marginX, y, "Pengembangan SDM")
	y += 14
	p.SetFont(false, 10)
	p.Text(marginX, y, "Di -")
	y += 14
	p.Text(marginX+30, y, "Tempat")
	y += 26

	jenisNama := ""
	if item.JenisCuti != nil {
		jenisNama = item.JenisCuti.Jenis
	}
	tglRange := formatTanggalRentang(item.TglMulai, item.TglSelesai)
	para1 := fmt.Sprintf(
		"Menindak lanjuti surat permohonan %s atas nama %s; Tanggal %s dengan ini kami tidak keberatan dan menyetujui permohonan tersebut kami teruskan kepada Bapak untuk ditindaklanjuti (Permohonan Terlampir).",
		jenisNama, strings.ToUpper(pegawai.Nama), tglRange,
	)
	y = p.MultilineText(marginX, y, rightX-marginX, 14, para1) + 12

	para2 := "Demikian Surat Permohonan Cuti ini kami teruskan kepada Bapak, atas pertimbangan Bapak kami ucapkan terima kasih."
	y = p.MultilineText(marginX, y, rightX-marginX, 14, para2) + 34

	sigX := pageW - 230
	p.Text(sigX, y, "Kolonodale, "+formatDateID(tglApproval)+".")
	y += 15
	p.Text(sigX, y, "Kepala Dinas")
	y += 58

	kepalaDinasNama := namaOrDash(pengaturan.NamaKepalaDinas)
	p.SetFont(true, 10)
	p.Text(sigX, y, kepalaDinasNama)
	p.Line(sigX, y+3, sigX+utils.TextWidth(kepalaDinasNama, 10), y+3)
	y += 14
	p.SetFont(false, 10)
	p.Text(sigX, y, "NIP: "+namaOrDash(pengaturan.NipKepalaDinas)+".")

	return doc.Output()
}

// buildFormulirCuti generates the official "Formulir Permintaan dan
// Pemberian Cuti" (leave request/grant form).
func buildFormulirCuti(item models.PengajuanCuti, pegawai models.Pegawai, pengaturan models.PengaturanSurat, jatah map[int]models.JatahCuti) ([]byte, error) {
	doc := utils.NewPDFDoc()
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		return nil, err
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

	// Blok "Kepada" rata kanan, di bawah kop surat.
	blockX := pageW/2 + 30
	y := 112.0
	p.SetFont(false, 10)
	p.Text(blockX, y, "Kolonodale, "+formatDateID(tglApproval)+".")
	y += 14
	p.Text(blockX, y, "Kepada")
	y += 14
	p.Text(blockX, y, "Yth. Bupati Morowali Utara")
	y += 14
	p.Text(blockX, y, "Cq. Kepala Badan Kepegawaian dan Pengembangan SDM")
	y += 14
	p.Text(blockX, y, "Di")
	y += 14
	p.Text(blockX+20, y, "Kolonodale")

	y = 218
	p.SetFont(true, 12)
	p.TextCentered(pageW/2, y, "FORMULIR PERMINTAAN DAN PEMBERIAN CUTI")
	y += 20

	// ---- tabel utama ----
	tableX := marginX
	tableTop := y
	labelColW := 20.0
	contentX := tableX + labelColW
	contentW := rightX - contentX
	pad := 4.0

	rowBottom := func(rowTop, rowH float64, romawi string) float64 {
		bottom := rowTop + rowH
		p.SetFont(true, 9)
		p.Text(tableX+3, rowTop+13, romawi)
		p.Line(tableX, bottom, rightX, bottom)
		p.Line(tableX+labelColW, rowTop, tableX+labelColW, bottom)
		return bottom
	}

	// Row I: Data Pegawai
	rowTop := tableTop
	rowH := 60.0
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Data Pegawai")
	p.SetFont(false, 9)
	rightHalfX := contentX + contentW*0.62
	jabatanNama := "-"
	if pegawai.Jabatan != nil {
		jabatanNama = pegawai.Jabatan.Jabatan
	}
	unitNama := "-"
	if pegawai.UnitKerja != nil {
		unitNama = pegawai.UnitKerja.Unit
	}
	p.Text(contentX+pad, rowTop+29, "Nama")
	p.SetFont(true, 9)
	p.Text(contentX+pad+55, rowTop+29, ": "+strings.ToUpper(pegawai.Nama))
	p.SetFont(false, 9)
	p.Text(rightHalfX, rowTop+29, "NIP")
	p.Text(rightHalfX+40, rowTop+29, ": "+namaOrDash(pegawai.NIP))
	p.Text(contentX+pad, rowTop+43, "Jabatan")
	p.Text(contentX+pad+55, rowTop+43, ": "+jabatanNama)
	p.Text(rightHalfX, rowTop+43, "Masa Kerja")
	p.Text(rightHalfX+55, rowTop+43, ": "+masaKerjaText(pegawai.TMT, tglApproval))
	p.Text(contentX+pad, rowTop+57, "Unit Kerja")
	p.Text(contentX+pad+55, rowTop+57, ": "+unitNama)
	y = rowBottom(rowTop, rowH, "I")

	// Row II: Jenis Cuti yang di ambil -- dirender sebagai tabel bergrid
	// penuh (kolom kosong tipis | nomor | label | kotak centang, diulang
	// untuk kelompok kiri 1-3 dan kanan 4-6, dengan garis horizontal di
	// antara tiap baris) supaya semirip mungkin dengan formulir cetak asli.
	rowTop = y
	rowH = 60.0
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Jenis Cuti yang di ambil")
	p.SetFont(false, 8.5)
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
	checkSize := 9.0
	blankColW := 6.0
	checkColW := 22.0
	trailColW := 6.0
	leftNumX := contentX + blankColW
	leftLabelX := leftNumX + 14.0
	leftCheckX := midX - checkColW
	rightNumX := midX + blankColW
	rightLabelX := rightNumX + 14.0
	rightCheckX := rightX - trailColW - checkColW

	gridTop := rowTop + 20.0
	itemH := (rowH - 20.0) / 3.0
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
	alasanLines := utils.WrapText(namaOrDash(item.AlasanCuti), contentW-2*pad, 9)
	rowH = 22.0 + float64(len(alasanLines))*12
	if rowH < 34 {
		rowH = 34
	}
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Alasan Cuti")
	p.SetFont(false, 9)
	p.MultilineText(contentX+pad, rowTop+29, contentW-2*pad, 12, namaOrDash(item.AlasanCuti))
	y = rowBottom(rowTop, rowH, "III")

	// Row IV: Lama Cuti
	rowTop = y
	rowH = 34.0
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Lama Cuti")
	p.SetFont(false, 9)
	p.Text(contentX+pad, rowTop+29, fmt.Sprintf("Selama %d Hari   Mulai Tanggal %s", item.JumlahHari, formatTanggalRentang(item.TglMulai, item.TglSelesai)))
	y = rowBottom(rowTop, rowH, "IV")

	// Row V: Catatan Cuti (riwayat kuota cuti tahunan N-2..N + legenda jenis
	// cuti) -- juga dirender bergrid penuh: tabel kuota di kiri (kolom
	// kosong | label | Tahun | Sisa | Keterangan, dengan garis antar baris)
	// dan daftar legenda 1-6 di kanan (kolom kosong | nomor | label, dengan
	// garis antar baris), meniru formulir cetak asli.
	rowTop = y
	rowH = 98.0
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Catatan Cuti")
	quotaW := contentW * 0.5
	blankColW2 := 6.0
	quotaEnd := contentX + quotaW
	numColX := contentX + blankColW2
	colTahunX := numColX + 14.0
	remaining := quotaEnd - colTahunX
	colSisaX := colTahunX + remaining*0.40
	colKetX := colSisaX + remaining*0.32
	p.SetFont(false, 8)

	// Tabel kuota kiri: baris "1 Cuti Tahunan" (judul, meniru penomoran
	// jenis cuti #1), lalu baris header Tahun/Sisa/Keterangan, lalu 3 baris
	// data (N-1/N-2/N) -- persis urutan & struktur pada formulir cetak asli.
	gridTop3 := rowTop + 18.0
	rowHLeft := (rowH - 18.0) / 5.0
	titleTop := gridTop3
	titleLy := titleTop + rowHLeft*0.72
	p.Text(numColX+2, titleLy, "1")
	p.Text(colTahunX+3, titleLy, "Cuti Tahunan")

	headerTop := gridTop3 + rowHLeft
	headerLy := headerTop + rowHLeft*0.72
	p.Text(colTahunX+3, headerLy, "Tahun")
	p.Text(colSisaX+3, headerLy, "Sisa")
	p.Text(colKetX+3, headerLy, "Keterangan")

	years := []struct {
		label string
		year  int
	}{{"N-1", item.TglMulai.Year() - 1}, {"N-2", item.TglMulai.Year() - 2}, {"N", item.TglMulai.Year()}}
	for i, yr := range years {
		cellTop := gridTop3 + rowHLeft*float64(2+i)
		ly := cellTop + rowHLeft*0.72
		p.Text(colTahunX+3, ly, yr.label)
		if jc, ok := jatah[yr.year]; ok {
			p.Text(colSisaX+3, ly, fmt.Sprintf("%d", jc.Sisa()))
			p.Text(colKetX+3, ly, "Cuti Tahunan")
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
	legendLabelX := legendNumX + 14.0
	legendTrailW := 6.0
	itemH2 := (rowH - 18.0) / 6.0
	legend := []string{"Cuti Tahunan", "Cuti Sakit", "Cuti Karena Alasan Penting", "Cuti Besar", "Cuti Melahirkan", "Cuti di Luar Tanggungan Negara"}
	for i, l := range legend {
		cellTop := gridTop3 + itemH2*float64(i)
		ly := cellTop + itemH2*0.72
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

	// Row VI: Alamat Selama Menjalankan Cuti + tanda tangan pegawai
	rowTop = y
	rowH = 98.0
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Alamat Selama Menjalankan Cuti")
	p.SetFont(false, 9)
	p.MultilineText(contentX+pad, rowTop+29, rightHalfX-contentX-pad-8, 12, namaOrDash(item.AlamatSelamaCuti))
	p.Text(rightHalfX, rowTop+29, "Telp: "+namaOrDash(pegawai.NoHP))
	p.SetFont(false, 8.5)
	p.TextCentered(rightHalfX+70, rowTop+45, "Hormat Saya")
	p.SetFont(true, 9)
	namaPegawaiUpper := strings.ToUpper(pegawai.Nama)
	p.TextCentered(rightHalfX+70, rowTop+82, namaPegawaiUpper)
	w := utils.TextWidth(namaPegawaiUpper, 9)
	p.Line(rightHalfX+70-w/2, rowTop+85, rightHalfX+70+w/2, rowTop+85)
	p.SetFont(false, 8.5)
	p.TextCentered(rightHalfX+70, rowTop+96, "NIP: "+namaOrDash(pegawai.NIP)+".")
	y = rowBottom(rowTop, rowH, "VI")

	// Row VII: Pertimbangan Atasan Langsung -- 4 opsi hanya dibingkai pada
	// satu baris tipis di bawah judul (seperti formulir cetak asli, yang
	// menyisakan area kosong besar di bawahnya untuk catatan/tanda tangan
	// atasan langsung), diikuti kolom tanda tangan Kepala Dinas di kanan.
	rowTop = y
	rowH = 92.0
	p.SetFont(true, 9)
	p.Text(contentX+pad, rowTop+13, "Pertimbangan Atasan Langsung")
	opts := []string{"Disetujui", "Perubahan", "Ditangguhkan", "Tidak Disetujui"}
	optRegionEnd := rightHalfX
	optW := (optRegionEnd - contentX) / float64(len(opts))
	gridTopVII := rowTop + 18.0
	optHeaderH := 16.0
	checkSize2 := 8.0
	p.SetFont(false, 8)
	for i, opt := range opts {
		colStart := contentX + optW*float64(i)
		colCenter := colStart + optW/2
		tw := utils.TextWidth(opt, 8)
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
	p.Line(contentX, gridTopVII, optRegionEnd, gridTopVII)
	p.Line(contentX, gridTopVII+optHeaderH, optRegionEnd, gridTopVII+optHeaderH)
	dividerX := optRegionEnd
	p.Line(dividerX, rowTop, dividerX, rowTop+rowH)
	sigColCenter := dividerX + (rightX-dividerX)/2
	p.SetFont(false, 8.5)
	p.TextCentered(sigColCenter, rowTop+27, "Kepala Dinas")
	kepalaDinasNama := namaOrDash(pengaturan.NamaKepalaDinas)
	p.SetFont(true, 9)
	p.TextCentered(sigColCenter, rowTop+70, kepalaDinasNama)
	w2 := utils.TextWidth(kepalaDinasNama, 9)
	p.Line(sigColCenter-w2/2, rowTop+73, sigColCenter+w2/2, rowTop+73)
	p.SetFont(false, 8.5)
	p.TextCentered(sigColCenter, rowTop+84, "NIP: "+namaOrDash(pengaturan.NipKepalaDinas)+".")
	y = rowBottom(rowTop, rowH, "VII")

	// bingkai luar tabel
	p.Line(tableX, tableTop, tableX, y)
	p.Line(rightX, tableTop, rightX, y)
	p.Line(tableX, tableTop, rightX, tableTop)

	return doc.Output()
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

func downloadFormRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, pegawai, ok := loadApprovedPengajuanForForm(w, r, db)
	if !ok {
		return
	}
	pengaturan := pengaturanSuratOrDefault(db)
	pdfBytes, err := buildSuratRekomendasi(item, pegawai, pengaturan)
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
	pengaturan := pengaturanSuratOrDefault(db)
	year := item.TglMulai.Year()
	jatah := loadJatahHistory(db, pegawai.ID, []int{year, year - 1, year - 2})
	pdfBytes, err := buildFormulirCuti(item, pegawai, pengaturan, jatah)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat formulir: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("formulir_cuti_%s.pdf", pegawai.NIP))
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
