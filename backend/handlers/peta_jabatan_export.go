package handlers

import (
	"fmt"
	"net/http"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// peta_jabatan_export.go -- permintaan pengguna: administrator & atasan/
// Kepala Sekolah bisa mencetak/mengunduh tabel "Peta Jabatan Sekolah"
// (Jabatan/Sub-Jabatan/B/K/+-) dalam bentuk Excel (.xlsx) DAN PDF, KHUSUS
// kertas ukuran Legal (8.5" x 14") supaya seluruh tabel tetap muat rapi.
// Dipakai tombol "Cetak Excel"/"Cetak PDF" pada halaman Peta Jabatan --
// akses dibatasi sama seperti aksi kelola lainnya (lihat middleware "kelola"
// & resolveUnitKerjaSekolahForRequest/canManagePetaJabatanSekolah di
// peta_jabatan.go): administrator/admin boleh sekolah manapun (lewat
// parameter id_unit_kerja), atasan HANYA sekolahnya sendiri. Datanya SELALU
// dihitung ulang lewat hitungPetaJabatanSekolah (fungsi yang sama dipakai
// tabel menu & dokumen kenaikan pangkat di peta_jabatan_pdf.go) supaya angka
// yang dicetak selalu identik dengan yang tampil di layar.

// exportPetaJabatanSekolahExcel menangani GET /api/peta-jabatan/sekolah/export-excel?id_unit_kerja=..
func exportPetaJabatanSekolahExcel(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	idUnitKerja, err := resolveUnitKerjaSekolahForRequest(claims, r, db)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	peta, err := hitungPetaJabatanSekolah(db, idUnitKerja)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	f, err := buildPetaJabatanExcel(peta)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat file excel: "+err.Error())
		return
	}
	filename := fmt.Sprintf("peta_jabatan_%s.xlsx", slugNamaFile(peta.UnitKerja.Unit))
	writeXlsxResponse(w, f, filename)
}

// cetakPetaJabatanSekolahPDF menangani GET /api/peta-jabatan/sekolah/cetak-pdf?id_unit_kerja=..
func cetakPetaJabatanSekolahPDF(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	idUnitKerja, err := resolveUnitKerjaSekolahForRequest(claims, r, db)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	peta, err := hitungPetaJabatanSekolah(db, idUnitKerja)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	var uk models.UnitKerja
	db.First(&uk, idUnitKerja)

	pdfBytes, err := buildPetaJabatanSekolahExportPDF(peta, &uk)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat PDF: "+err.Error())
		return
	}
	filename := fmt.Sprintf("peta_jabatan_%s.pdf", slugNamaFile(peta.UnitKerja.Unit))
	writePDFResponse(w, r, pdfBytes, filename)
}

// ============================================================
// Excel (.xlsx) -- satu sheet, diset kertas Legal untuk cetak
// ============================================================

// paperSizeLegal: kode ukuran kertas "Legal" (8.5" x 14") menurut daftar
// ECMA-376/ST_PaperSize yang dipakai excelize (lihat komentar
// (*excelize.File).SetPageLayout -- index 5 = "Legal paper (8.5 in. by 14 in.)").
const paperSizeLegal = 5

func buildPetaJabatanExcel(peta *petaJabatanSekolahOut) (*excelize.File, error) {
	f := excelize.NewFile()
	const sheet = "Peta Jabatan"
	f.SetSheetName("Sheet1", sheet)

	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	subtitleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"2563EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    excelThinBorder(),
	})
	jabatanLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true},
		Border: excelThinBorder(),
	})
	subLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Italic: true},
		Alignment: &excelize.Alignment{Indent: 1},
		Border:    excelThinBorder(),
	})
	jabatanNumStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    excelThinBorder(),
	})
	subNumStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    excelThinBorder(),
	})
	emptyStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Italic: true, Color: "6B7280"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	f.SetCellValue(sheet, "A1", "PETA JABATAN SEKOLAH")
	f.MergeCell(sheet, "A1", "D1")
	f.SetCellStyle(sheet, "A1", "D1", titleStyle)

	f.SetCellValue(sheet, "A2", peta.UnitKerja.Unit)
	f.MergeCell(sheet, "A2", "D2")
	f.SetCellStyle(sheet, "A2", "D2", subtitleStyle)

	f.SetCellValue(sheet, "A3", "Dicetak: "+formatDateID(absensiNow()))
	f.MergeCell(sheet, "A3", "D3")
	f.SetCellStyle(sheet, "A3", "D3", subtitleStyle)

	const headerRow = 5
	f.SetCellValue(sheet, cellRef("A", headerRow), "Jabatan / Sub-Jabatan")
	f.SetCellValue(sheet, cellRef("B", headerRow), "B")
	f.SetCellValue(sheet, cellRef("C", headerRow), "K")
	f.SetCellValue(sheet, cellRef("D", headerRow), "+/-")
	f.SetCellStyle(sheet, cellRef("A", headerRow), cellRef("D", headerRow), headerStyle)

	row := headerRow + 1
	for _, j := range peta.Rows {
		f.SetCellValue(sheet, cellRef("A", row), j.Jabatan)
		f.SetCellValue(sheet, cellRef("B", row), j.B)
		f.SetCellValue(sheet, cellRef("C", row), j.K)
		f.SetCellValue(sheet, cellRef("D", row), signedStr(j.Selisih))
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), jabatanLabelStyle)
		f.SetCellStyle(sheet, cellRef("B", row), cellRef("D", row), jabatanNumStyle)
		row++
		for _, s := range j.SubJabatan {
			f.SetCellValue(sheet, cellRef("A", row), "    - "+s.Nama)
			f.SetCellValue(sheet, cellRef("B", row), s.B)
			f.SetCellValue(sheet, cellRef("C", row), s.K)
			f.SetCellValue(sheet, cellRef("D", row), signedStr(s.Selisih))
			f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), subLabelStyle)
			f.SetCellStyle(sheet, cellRef("B", row), cellRef("D", row), subNumStyle)
			row++
		}
	}
	if len(peta.Rows) == 0 {
		f.SetCellValue(sheet, cellRef("A", row), "Belum ada data jabatan/pegawai pada sekolah ini.")
		f.MergeCell(sheet, cellRef("A", row), cellRef("D", row))
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("D", row), emptyStyle)
	}

	f.SetColWidth(sheet, "A", "A", 42)
	f.SetColWidth(sheet, "B", "D", 12)

	size := paperSizeLegal
	orientation := "portrait"
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{
		Size:        &size,
		Orientation: &orientation,
	})

	return f, nil
}

func excelThinBorder() []excelize.Border {
	return []excelize.Border{
		{Type: "top", Color: "D1D5DB", Style: 1},
		{Type: "bottom", Color: "D1D5DB", Style: 1},
		{Type: "left", Color: "D1D5DB", Style: 1},
		{Type: "right", Color: "D1D5DB", Style: 1},
	}
}

func cellRef(col string, row int) string {
	return fmt.Sprintf("%s%d", col, row)
}

// ============================================================
// PDF -- kertas Legal (8.5" x 14"), tabel dipaginasi otomatis
// ============================================================

// flatPetaRow adalah satu baris tabel PDF setelah Jabatan & Sub-Jabatan-nya
// diratakan jadi satu daftar berurutan (Jabatan induk, lalu seluruh
// Sub-Jabatannya menjorok), supaya logika paginasi tidak perlu peduli lagi
// soal struktur dua level.
type flatPetaRow struct {
	Label   string
	B, K    int
	Selisih int
	Bold    bool
}

// buildPetaJabatanSekolahExportPDF membangun dokumen cetak tabel Peta
// Jabatan Sekolah (BUKAN dokumen kenaikan pangkat -- lihat
// buildKenaikanPangkatPDF di peta_jabatan_pdf.go untuk itu). Kop surat
// sekolah dipakai di halaman pertama (sama fungsi drawLetterheadUnitKerjaKustom
// yang dipakai dokumen cetak sekolah lain), tabel dipaginasi otomatis kalau
// jumlah jabatan/sub-jabatan tidak muat dalam satu halaman Legal.
func buildPetaJabatanSekolahExportPDF(peta *petaJabatanSekolahOut, uk *models.UnitKerja) ([]byte, error) {
	var flat []flatPetaRow
	for _, j := range peta.Rows {
		flat = append(flat, flatPetaRow{Label: j.Jabatan, B: j.B, K: j.K, Selisih: j.Selisih, Bold: true})
		for _, s := range j.SubJabatan {
			flat = append(flat, flatPetaRow{Label: "     - " + s.Nama, B: s.B, K: s.K, Selisih: s.Selisih, Bold: false})
		}
	}

	const (
		marginX    = 42.0
		rowH       = 15.0
		headerH    = 16.0
		yTabelHal1 = 230.0 // perkiraan aman setelah kop sekolah + judul + subjudul (lihat drawLetterheadUnitKerjaKustom, bisa sedikit lebih pendek kalau kop belum diisi)
		yTabelLain = 66.0  // halaman lanjutan hanya pakai judul kecil, bukan kop penuh
	)
	pageW := utils.PageWidthLegal
	pageH := utils.PageHeightLegal
	rightX := pageW - marginX
	bottomY := pageH - 56.0

	xB := rightX - 140
	xK := rightX - 90
	xSel := rightX - 40

	muatHal1 := int((bottomY - (yTabelHal1 + headerH)) / rowH)
	muatLain := int((bottomY - (yTabelLain + headerH)) / rowH)
	if muatHal1 < 1 {
		muatHal1 = 1
	}
	if muatLain < 1 {
		muatLain = 1
	}

	totalHalaman := 1
	if sisa := len(flat) - muatHal1; sisa > 0 {
		totalHalaman += (sisa + muatLain - 1) / muatLain
	}

	unitKerjaNama := peta.UnitKerja.Unit
	dicetak := absensiNow().Format("02-01-2006 15:04")

	doc := utils.NewPDFDoc()

	gambarHeaderTabel := func(p *utils.PDFPage, yTop float64) float64 {
		p.SetFont(true, 9)
		p.Text(marginX, yTop, "Jabatan / Sub-Jabatan")
		p.TextCentered(xB, yTop, "B")
		p.TextCentered(xK, yTop, "K")
		p.TextCentered(xSel, yTop, "+/-")
		y := yTop + 4
		p.Line(marginX, y, rightX, y)
		return y + 12
	}

	gambarFooter := func(p *utils.PDFPage, halaman int) {
		p.SetFont(false, 7.5)
		p.Text(marginX, pageH-32, "Dicetak dari aplikasi SIMADU pada "+dicetak+" WITA")
		p.TextRight(rightX, pageH-32, fmt.Sprintf("Halaman %d dari %d", halaman, totalHalaman))
	}

	idx := 0
	for halaman := 1; halaman <= totalHalaman; halaman++ {
		p := utils.NewPDFPage(pageW, pageH)
		doc.AddPage(p)

		var y float64
		if halaman == 1 {
			y = drawLetterheadUnitKerjaKustom(doc, p, marginX, rightX, unitKerjaNama, uk) + 12
			p.SetFont(true, 14)
			p.TextCentered(pageW/2, y, "PETA JABATAN SEKOLAH")
			y += 18
			p.SetFont(false, 10)
			p.TextCentered(pageW/2, y, unitKerjaNama)
			y += 16
			p.SetFont(false, 8.5)
			p.TextCentered(pageW/2, y, "Dicetak: "+formatDateID(absensiNow())+" WITA")
			y += 14
			// kalau kop sekolah ternyata lebih pendek dari perkiraan
			// yTabelHal1, tabel tetap dimulai dari yTabelHal1 (konsisten
			// dengan perhitungan muatHal1 di atas); kalau LEBIH panjang
			// (kop kustom banyak baris), tabel digeser turun mengikuti y
			// sebenarnya supaya tidak tumpang tindih dengan kop.
			if y < yTabelHal1 {
				y = yTabelHal1
			}
		} else {
			p.SetFont(true, 11)
			p.Text(marginX, 40, "PETA JABATAN SEKOLAH -- "+unitKerjaNama+" (lanjutan)")
			p.SetLineWidth(1)
			p.Line(marginX, 48, rightX, 48)
			y = yTabelLain
		}

		y = gambarHeaderTabel(p, y)

		muat := muatHal1
		if halaman > 1 {
			muat = muatLain
		}
		count := 0
		for count < muat && idx < len(flat) {
			row := flat[idx]
			if row.Bold {
				p.SetFont(true, 8.5)
			} else {
				p.SetFont(false, 8)
			}
			p.Text(marginX, y, row.Label)
			p.TextCentered(xB, y, fmt.Sprintf("%d", row.B))
			p.TextCentered(xK, y, fmt.Sprintf("%d", row.K))
			p.TextCentered(xSel, y, signedStr(row.Selisih))
			y += rowH
			idx++
			count++
		}

		if halaman == 1 && len(flat) == 0 {
			p.SetFont(false, 9)
			p.TextCentered(pageW/2, y+10, "Belum ada data jabatan/pegawai pada sekolah ini.")
		}

		gambarFooter(p, halaman)
	}

	return doc.Output()
}
