package handlers

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// peta_jabatan_export.go -- permintaan pengguna: administrator & atasan/
// Kepala Sekolah bisa mencetak/mengunduh "Peta Jabatan Sekolah" dalam bentuk
// Excel (.xlsx) DAN PDF, meniru format ANJAB/ABK yang biasa dipakai instansi
// pemerintah: kotak organogram "KEPALA SEKOLAH" di atas, dua tabel
// berdampingan (KELOMPOK JABATAN FUNGSIONAL & KELOMPOK JABATAN PELAKSANA,
// masing-masing berkolom KJ/B/K/+-), tabel REKAPITULASI, legenda
// "Keterangan", dan tanda tangan QR Kepala Sekolah & Kepala Dinas. KHUSUS
// kertas ukuran Legal (8.5" x 14"). Dipakai tombol "Cetak Excel"/"Cetak PDF"
// pada halaman Peta Jabatan -- akses dibatasi sama seperti aksi kelola
// lainnya (lihat middleware "kelola" & resolveUnitKerjaSekolahForRequest/
// canManagePetaJabatanSekolah di peta_jabatan.go): HANYA administrator/admin
// (sekolah manapun, lewat parameter id_unit_kerja) & atasan/Kepala Sekolah
// (sekolahnya sendiri) -- bukan pegawai. Datanya SELALU dihitung ulang lewat
// hitungPetaJabatanSekolah (fungsi yang sama dipakai tabel menu & dokumen
// kenaikan pangkat di peta_jabatan_pdf.go) supaya angka yang dicetak selalu
// identik dengan yang tampil di layar.

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
	signers := resolvePetaJabatanSigners(db, idUnitKerja)

	f, err := buildPetaJabatanExcel(peta, signers)
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
	signers := resolvePetaJabatanSigners(db, idUnitKerja)

	pdfBytes, err := buildPetaJabatanSekolahExportPDF(peta, &uk, signers)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat PDF: "+err.Error())
		return
	}
	filename := fmt.Sprintf("peta_jabatan_%s.pdf", slugNamaFile(peta.UnitKerja.Unit))
	writePDFResponse(w, r, pdfBytes, filename)
}

// ============================================================
// penanda tangan (Kepala Sekolah & Kepala Dinas) -- dipakai Excel & PDF
// ============================================================

type petaJabatanSigners struct {
	KepsekNama    string
	KepsekNip     string
	KepsekJabatan string
	DinasNama     string
	DinasNip      string
	DinasJabatan  string
}

// resolvePetaJabatanSigners menentukan siapa "Kepala Sekolah" sekolah ini
// (lewat UnitKerja.IDAtasanLangsung -- field data master yang SATU pegawai
// eksplisit ditetapkan sebagai atasan langsung/Kepala Sekolah unit kerja
// itu, lihat komentar models.UnitKerja -- bukan dari akun yang login maupun
// dari pengajuan/approval manapun) dan "Kepala Dinas" (dari PengaturanSurat,
// sama persis seperti resolveSignerFullRekomendasi yang sudah dipakai
// dokumen cetak lain). Dipakai untuk blok tanda tangan QR pada dokumen
// Excel/PDF -- MURNI informasi cetak, tidak mengubah/menyimpan apa pun.
func resolvePetaJabatanSigners(db *gorm.DB, idUnitKerja uint) petaJabatanSigners {
	// KepsekJabatan SENGAJA selalu "Kepala Sekolah" -- ini jabatan STRUKTURAL
	// (penanda tangan dokumen) yang ditetapkan lewat UnitKerja.IDAtasanLangsung,
	// bukan jabatan fungsional/pelaksana pegawai yang bersangkutan (mis. "Guru
	// Ahli Pertama"). Jangan timpa dengan uk.AtasanLangsung.Jabatan.Jabatan.
	s := petaJabatanSigners{KepsekNama: "-", KepsekNip: "-", KepsekJabatan: "Kepala Sekolah"}
	var uk models.UnitKerja
	if db.Preload("AtasanLangsung").First(&uk, idUnitKerja).Error == nil && uk.AtasanLangsung != nil {
		s.KepsekNama = namaOrDash(uk.AtasanLangsung.Nama)
		s.KepsekNip = namaOrDash(uk.AtasanLangsung.NIP)
	}
	pengaturan := pengaturanSuratOrDefault(db)
	s.DinasNama, s.DinasNip, s.DinasJabatan, _, _ = resolveSignerFullRekomendasi(db, pengaturan)
	s.DinasNama, s.DinasNip = namaOrDash(s.DinasNama), namaOrDash(s.DinasNip)
	return s
}

// ============================================================
// pengelompokan Fungsional vs Pelaksana/Struktural -- sama dipakai Excel & PDF
// ============================================================

// splitPetaJabatanByJenis mengelompokkan baris hasil hitungPetaJabatanSekolah
// jadi dua kelompok mengikuti models.Jabatan.JenisJabatan: "KELOMPOK JABATAN
// FUNGSIONAL" (jenis "fungsional") vs "KELOMPOK JABATAN PELAKSANA" (jenis
// "pelaksana" ATAU "struktural" -- digabung satu tabel karena sekolah hampir
// tidak pernah punya jabatan struktural selain Kepala Sekolah itu sendiri,
// yang sudah digambarkan tersendiri sebagai kotak organogram di atas, bukan
// sebagai baris tabel).
func splitPetaJabatanByJenis(rows []jabatanPetaOut) (fungsional, pelaksana []jabatanPetaOut) {
	for _, row := range rows {
		if row.JenisJabatan == models.JenisJabatanFungsional {
			fungsional = append(fungsional, row)
		} else {
			pelaksana = append(pelaksana, row)
		}
	}
	return
}

// sumBK menjumlahkan B & K seluruh baris Jabatan level-atas (SENGAJA tidak
// ikut menjumlahkan SubJabatan-nya lagi -- B/K Jabatan induk sudah berisi
// SUM seluruh pecahannya, lihat hitungPetaJabatanSekolah, jadi menjumlahkan
// sub-jabatan di sini akan menghitung dua kali).
func sumBK(rows []jabatanPetaOut) (b, k int) {
	for _, row := range rows {
		b += row.B
		k += row.K
	}
	return
}

// kjString menampilkan nilai KJ (Kelas Jabatan) atau "-" kalau belum diisi.
func kjString(kj *int) string {
	if kj == nil {
		return "-"
	}
	return strconv.Itoa(*kj)
}

// ============================================================
// Excel (.xlsx) -- kertas Legal, landscape (dua tabel berdampingan)
// ============================================================

// paperSizeLegal: kode ukuran kertas "Legal" (8.5" x 14") menurut daftar
// ECMA-376/ST_PaperSize yang dipakai excelize (lihat komentar
// (*excelize.File).SetPageLayout -- index 5 = "Legal paper (8.5 in. by 14 in.)").
const paperSizeLegal = 5

func buildPetaJabatanExcel(peta *petaJabatanSekolahOut, signers petaJabatanSigners) (*excelize.File, error) {
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
	kepsekStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"DC2626"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    excelThinBorder(),
	})
	groupStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"0EA5E9"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    excelThinBorder(),
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
	jumlahLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true},
		Fill:   excelize.Fill{Type: "pattern", Color: []string{"FEF3C7"}, Pattern: 1},
		Border: excelThinBorder(),
	})
	jumlahNumStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FEF3C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    excelThinBorder(),
	})
	emptyStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Italic: true, Color: "6B7280"},
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border:    excelThinBorder(),
	})
	legendTitleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 9}})
	legendStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true, Size: 9}})
	signPlainStyle, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "center"}})
	signBoldStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Alignment: &excelize.Alignment{Horizontal: "center"}})

	// ---- judul ----
	f.SetCellValue(sheet, "A1", "PETA JABATAN SEKOLAH")
	f.MergeCell(sheet, "A1", "K1")
	f.SetCellStyle(sheet, "A1", "K1", titleStyle)
	f.SetCellValue(sheet, "A2", peta.UnitKerja.Unit)
	f.MergeCell(sheet, "A2", "K2")
	f.SetCellStyle(sheet, "A2", "K2", subtitleStyle)
	f.SetCellValue(sheet, "A3", "Dicetak: "+formatDateID(absensiNow()))
	f.MergeCell(sheet, "A3", "K3")
	f.SetCellStyle(sheet, "A3", "K3", subtitleStyle)

	// ---- kotak "KEPALA SEKOLAH" (organogram, disederhanakan jadi baris
	// judul bersorot -- menggantikan kotak+garis organogram pada dokumen
	// PDF, yang tidak praktis digambar di lembar kerja Excel) ----
	f.SetCellValue(sheet, "A5", "KEPALA SEKOLAH: "+signers.KepsekNama)
	f.MergeCell(sheet, "A5", "K5")
	f.SetCellStyle(sheet, "A5", "K5", kepsekStyle)
	f.SetRowHeight(sheet, 5, 22)

	fungsional, pelaksana := splitPetaJabatanByJenis(peta.Rows)

	const tableStartRow = 7
	leftEnd := writePetaJabatanExcelTable(f, sheet, tableStartRow, "A", "B", "C", "D", "E",
		"KELOMPOK JABATAN FUNGSIONAL", fungsional, "Jumlah Kebutuhan JF",
		groupStyle, headerStyle, jabatanLabelStyle, subLabelStyle, jabatanNumStyle, subNumStyle, jumlahLabelStyle, jumlahNumStyle, emptyStyle)
	rightEnd := writePetaJabatanExcelTable(f, sheet, tableStartRow, "G", "H", "I", "J", "K",
		"KELOMPOK JABATAN PELAKSANA", pelaksana, "Jumlah Kebutuhan JP",
		groupStyle, headerStyle, jabatanLabelStyle, subLabelStyle, jabatanNumStyle, subNumStyle, jumlahLabelStyle, jumlahNumStyle, emptyStyle)

	row := leftEnd
	if rightEnd > row {
		row = rightEnd
	}
	row++ // jarak

	// ---- REKAPITULASI ----
	fB, fK := sumBK(fungsional)
	pB, pK := sumBK(pelaksana)
	totalB, totalK := fB+pB, fK+pK

	f.SetCellValue(sheet, cellRef("A", row), "REKAPITULASI")
	f.MergeCell(sheet, cellRef("A", row), cellRef("D", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("D", row), groupStyle)
	row++
	f.SetCellValue(sheet, cellRef("B", row), "B")
	f.SetCellValue(sheet, cellRef("C", row), "K")
	f.SetCellValue(sheet, cellRef("D", row), "+/-")
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("D", row), headerStyle)
	row++
	rekapRows := []struct {
		label string
		b, k  int
		bold  bool
	}{
		{"Jabatan Fungsional", fB, fK, false},
		{"Jabatan Pelaksana", pB, pK, false},
		{"JUMLAH", totalB, totalK, true},
	}
	for _, rr := range rekapRows {
		labelStyle, numStyle := jabatanLabelStyle, jabatanNumStyle
		if rr.bold {
			labelStyle, numStyle = jumlahLabelStyle, jumlahNumStyle
		}
		f.SetCellValue(sheet, cellRef("A", row), rr.label)
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), labelStyle)
		f.SetCellValue(sheet, cellRef("B", row), rr.b)
		f.SetCellValue(sheet, cellRef("C", row), rr.k)
		f.SetCellValue(sheet, cellRef("D", row), signedStr(rr.b-rr.k))
		f.SetCellStyle(sheet, cellRef("B", row), cellRef("D", row), numStyle)
		row++
	}
	row++ // jarak

	// ---- Keterangan ----
	f.SetCellValue(sheet, cellRef("A", row), "Keterangan:")
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), legendTitleStyle)
	row++
	for _, line := range []string{
		"KJ   = Kelas Jabatan",
		"B    = Bezetting (jumlah pegawai saat ini)",
		"K    = Kebutuhan pegawai",
		"+/-  = Kelebihan (+) / Kekurangan (-) pegawai",
	} {
		f.SetCellValue(sheet, cellRef("A", row), line)
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), legendStyle)
		row++
	}
	row += 2 // jarak sebelum blok tanda tangan

	// ---- tanda tangan QR: Kepala Sekolah (kiri) & Kepala Dinas (kanan) ----
	dicetakStr := formatDateID(absensiNow())
	f.SetCellValue(sheet, cellRef("A", row), "Mengetahui,")
	f.MergeCell(sheet, cellRef("A", row), cellRef("E", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("E", row), signPlainStyle)
	f.SetCellValue(sheet, cellRef("G", row), "Kolonodale, "+dicetakStr)
	f.MergeCell(sheet, cellRef("G", row), cellRef("K", row))
	f.SetCellStyle(sheet, cellRef("G", row), cellRef("K", row), signPlainStyle)
	row++
	f.SetCellValue(sheet, cellRef("A", row), signers.KepsekJabatan)
	f.MergeCell(sheet, cellRef("A", row), cellRef("E", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("E", row), signBoldStyle)
	f.SetCellValue(sheet, cellRef("G", row), "Kepala Dinas Pendidikan dan Kebudayaan")
	f.MergeCell(sheet, cellRef("G", row), cellRef("K", row))
	f.SetCellStyle(sheet, cellRef("G", row), cellRef("K", row), signBoldStyle)
	row++
	qrRow := row

	kepsekQuery := fmt.Sprintf("Kepala Sekolah %s NIP %s Jabatan %s -- Peta Jabatan Sekolah %s dicetak %s",
		signers.KepsekNama, signers.KepsekNip, signers.KepsekJabatan, peta.UnitKerja.Unit, dicetakStr)
	if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(kepsekQuery), qrcode.Medium, 80); err == nil {
		_ = f.AddPictureFromBytes(sheet, cellRef("C", qrRow), &excelize.Picture{
			Extension: ".png", File: qrPng, Format: &excelize.GraphicOptions{AltText: "QR Tanda Tangan Kepala Sekolah"},
		})
	}
	dinasQuery := fmt.Sprintf("Kepala Dinas %s NIP %s Jabatan %s -- Peta Jabatan Sekolah %s dicetak %s",
		signers.DinasNama, signers.DinasNip, signers.DinasJabatan, peta.UnitKerja.Unit, dicetakStr)
	if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(dinasQuery), qrcode.Medium, 80); err == nil {
		_ = f.AddPictureFromBytes(sheet, cellRef("I", qrRow), &excelize.Picture{
			Extension: ".png", File: qrPng, Format: &excelize.GraphicOptions{AltText: "QR Tanda Tangan Kepala Dinas"},
		})
	}
	for i := 0; i < 6; i++ {
		f.SetRowHeight(sheet, qrRow+i, 13)
	}
	row = qrRow + 6

	f.SetCellValue(sheet, cellRef("A", row), signers.KepsekNama)
	f.MergeCell(sheet, cellRef("A", row), cellRef("E", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("E", row), signBoldStyle)
	f.SetCellValue(sheet, cellRef("G", row), signers.DinasNama)
	f.MergeCell(sheet, cellRef("G", row), cellRef("K", row))
	f.SetCellStyle(sheet, cellRef("G", row), cellRef("K", row), signBoldStyle)
	row++
	f.SetCellValue(sheet, cellRef("A", row), "NIP. "+signers.KepsekNip)
	f.MergeCell(sheet, cellRef("A", row), cellRef("E", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("E", row), signPlainStyle)
	f.SetCellValue(sheet, cellRef("G", row), "NIP. "+signers.DinasNip)
	f.MergeCell(sheet, cellRef("G", row), cellRef("K", row))
	f.SetCellStyle(sheet, cellRef("G", row), cellRef("K", row), signPlainStyle)

	// ---- lebar kolom & ukuran kertas ----
	f.SetColWidth(sheet, "A", "A", 34)
	f.SetColWidth(sheet, "G", "G", 34)
	f.SetColWidth(sheet, "B", "E", 9)
	f.SetColWidth(sheet, "H", "K", 9)
	f.SetColWidth(sheet, "F", "F", 3)

	size := paperSizeLegal
	orientation := "landscape"
	fitToWidth := 1
	fitToHeight := 1
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{
		Size:        &size,
		Orientation: &orientation,
		FitToWidth:  &fitToWidth,
		FitToHeight: &fitToHeight,
	})
	// fitToWidth/fitToHeight di atas HANYA dipakai Excel/LibreOffice kalau
	// sheetPr/pageSetUpPr "fitToPage" juga diaktifkan -- tanpa ini, Excel diam-
	// diam memakai "scale" biasa dan dokumen bisa pecah ke beberapa halaman
	// cetak walau kontennya memang didesain pas satu halaman Legal.
	fitToPage := true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fitToPage})

	return f, nil
}

// writePetaJabatanExcelTable menulis SATU tabel (Jabatan Fungsional ATAU
// Jabatan Pelaksana) mulai dari startRow, pada lima kolom berurutan
// (colLabel..colSel): baris label kelompok (judul biru), baris header kolom
// (Jabatan/Sub-Jabatan | KJ | B | K | +/-), baris data (Jabatan tebal +
// SubJabatan miring menjorok), lalu baris "Jumlah Kebutuhan .." tebal.
// Mengembalikan baris pertama yang masih KOSONG setelah tabel ini (dipakai
// pemanggil untuk menyamakan posisi lanjutan dengan tabel sebelahnya, yang
// panjangnya bisa berbeda).
func writePetaJabatanExcelTable(
	f *excelize.File, sheet string, startRow int,
	colLabel, colKJ, colB, colK, colSel string,
	groupTitle string, rows []jabatanPetaOut, jumlahLabel string,
	groupStyle, headerStyle, jabatanLabelStyle, subLabelStyle, jabatanNumStyle, subNumStyle, jumlahLabelStyle, jumlahNumStyle, emptyStyle int,
) int {
	row := startRow
	f.SetCellValue(sheet, colLabel+strconv.Itoa(row), groupTitle)
	f.MergeCell(sheet, colLabel+strconv.Itoa(row), colSel+strconv.Itoa(row))
	f.SetCellStyle(sheet, colLabel+strconv.Itoa(row), colSel+strconv.Itoa(row), groupStyle)
	row++

	f.SetCellValue(sheet, colLabel+strconv.Itoa(row), "Jabatan / Sub-Jabatan")
	f.SetCellValue(sheet, colKJ+strconv.Itoa(row), "KJ")
	f.SetCellValue(sheet, colB+strconv.Itoa(row), "B")
	f.SetCellValue(sheet, colK+strconv.Itoa(row), "K")
	f.SetCellValue(sheet, colSel+strconv.Itoa(row), "+/-")
	f.SetCellStyle(sheet, colLabel+strconv.Itoa(row), colSel+strconv.Itoa(row), headerStyle)
	row++

	if len(rows) == 0 {
		f.SetCellValue(sheet, colLabel+strconv.Itoa(row), "Belum ada data.")
		f.MergeCell(sheet, colLabel+strconv.Itoa(row), colSel+strconv.Itoa(row))
		f.SetCellStyle(sheet, colLabel+strconv.Itoa(row), colSel+strconv.Itoa(row), emptyStyle)
		row++
	}

	totalB, totalK := 0, 0
	for _, j := range rows {
		totalB += j.B
		totalK += j.K
		f.SetCellValue(sheet, colLabel+strconv.Itoa(row), j.Jabatan)
		f.SetCellValue(sheet, colKJ+strconv.Itoa(row), kjString(j.KJ))
		f.SetCellValue(sheet, colB+strconv.Itoa(row), j.B)
		f.SetCellValue(sheet, colK+strconv.Itoa(row), j.K)
		f.SetCellValue(sheet, colSel+strconv.Itoa(row), signedStr(j.Selisih))
		f.SetCellStyle(sheet, colLabel+strconv.Itoa(row), colLabel+strconv.Itoa(row), jabatanLabelStyle)
		f.SetCellStyle(sheet, colKJ+strconv.Itoa(row), colSel+strconv.Itoa(row), jabatanNumStyle)
		row++
		for _, s := range j.SubJabatan {
			f.SetCellValue(sheet, colLabel+strconv.Itoa(row), "    - "+s.Nama)
			f.SetCellValue(sheet, colKJ+strconv.Itoa(row), kjString(s.KJ))
			f.SetCellValue(sheet, colB+strconv.Itoa(row), s.B)
			f.SetCellValue(sheet, colK+strconv.Itoa(row), s.K)
			f.SetCellValue(sheet, colSel+strconv.Itoa(row), signedStr(s.Selisih))
			f.SetCellStyle(sheet, colLabel+strconv.Itoa(row), colLabel+strconv.Itoa(row), subLabelStyle)
			f.SetCellStyle(sheet, colKJ+strconv.Itoa(row), colSel+strconv.Itoa(row), subNumStyle)
			row++
		}
	}

	f.SetCellValue(sheet, colLabel+strconv.Itoa(row), jumlahLabel)
	f.MergeCell(sheet, colLabel+strconv.Itoa(row), colKJ+strconv.Itoa(row))
	f.SetCellStyle(sheet, colLabel+strconv.Itoa(row), colKJ+strconv.Itoa(row), jumlahLabelStyle)
	f.SetCellValue(sheet, colB+strconv.Itoa(row), totalB)
	f.SetCellValue(sheet, colK+strconv.Itoa(row), totalK)
	f.SetCellValue(sheet, colSel+strconv.Itoa(row), signedStr(totalB-totalK))
	f.SetCellStyle(sheet, colB+strconv.Itoa(row), colSel+strconv.Itoa(row), jumlahNumStyle)
	row++

	return row
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
// PDF -- kertas Legal (8.5" x 14"), potret: organogram + dua tabel
// berdampingan + rekapitulasi + keterangan + tanda tangan QR
// ============================================================

// buildPetaJabatanSekolahExportPDF membangun dokumen cetak "Peta Jabatan
// Sekolah" ala ANJAB/ABK (BUKAN dokumen kenaikan pangkat -- lihat
// buildKenaikanPangkatPDF di peta_jabatan_pdf.go untuk itu): kop sekolah,
// kotak organogram "KEPALA SEKOLAH", dua tabel berdampingan (Jabatan
// Fungsional & Jabatan Pelaksana, kolom KJ/B/K/+-), tabel REKAPITULASI,
// legenda "Keterangan", dan tanda tangan QR Kepala Sekolah & Kepala Dinas.
//
// SENGAJA dirancang untuk muat dalam SATU halaman Legal (budget tinggi
// dipakai dengan teliti di bawah) -- kalau satu kelompok (Fungsional/
// Pelaksana) sekolah tertentu punya jabatan/sub-jabatan TERLALU banyak untuk
// muat, daftarnya dipotong dengan catatan "+N lainnya, lihat Excel untuk
// daftar lengkap" alih-alih memaksakan paginasi dua-kolom yang rumit --
// REKAPITULASI tetap dihitung dari data LENGKAP (bukan yang terpotong),
// jadi totalnya tetap benar walau tampilan tabelnya terpotong. Versi Excel
// (lihat buildPetaJabatanExcel di atas) tidak punya batasan ini sama
// sekali, jadi selalu jadi sumber data yang lengkap.
func buildPetaJabatanSekolahExportPDF(peta *petaJabatanSekolahOut, uk *models.UnitKerja, signers petaJabatanSigners) ([]byte, error) {
	const (
		marginX = 36.0
	)
	pageW := utils.PageWidthLegal
	pageH := utils.PageHeightLegal
	rightX := pageW - marginX
	centerX := (marginX + rightX) / 2

	doc := utils.NewPDFDoc()
	p := utils.NewPDFPage(pageW, pageH)
	doc.AddPage(p)

	unitKerjaNama := peta.UnitKerja.Unit
	dicetakStr := formatDateID(absensiNow())

	// ---- kop sekolah + judul ----
	y := drawLetterheadUnitKerjaKustom(doc, p, marginX, rightX, unitKerjaNama, uk) + 10
	p.SetFont(true, 14)
	p.TextCentered(centerX, y, "PETA JABATAN SEKOLAH")
	y += 16
	p.SetFont(false, 9.5)
	p.TextCentered(centerX, y, unitKerjaNama)
	y += 13
	p.SetFont(false, 8)
	p.TextCentered(centerX, y, "Dicetak: "+dicetakStr+" WITA")
	y += 14

	// ---- kolom tabel (dua tabel berdampingan) ----
	const colGap = 16.0
	colW := (rightX - marginX - colGap) / 2
	leftColX := marginX
	rightColX := marginX + colW + colGap
	leftCenterX := leftColX + colW/2
	rightCenterX := rightColX + colW/2

	// ---- organogram: kotak "KEPALA SEKOLAH" + garis bercabang ke dua
	// label kelompok tepat di atas masing-masing tabel ----
	boxW, boxH := 190.0, 30.0
	boxX := centerX - boxW/2
	p.FilledRect(boxX, y, boxW, boxH, 0.80, 0.11, 0.11)
	p.SetTextColor(1, 1, 1)
	p.SetFont(true, 10)
	p.TextCentered(centerX, y+12, "KEPALA SEKOLAH")
	p.SetFont(false, 8)
	p.TextCentered(centerX, y+23, signers.KepsekNama)
	p.SetTextColor(0, 0, 0)
	chartTopY := y + boxH
	branchY := chartTopY + 12
	p.SetLineWidth(1.2)
	p.Line(centerX, chartTopY, centerX, branchY)
	p.Line(leftCenterX, branchY, rightCenterX, branchY)
	p.Line(leftCenterX, branchY, leftCenterX, branchY+8)
	p.Line(rightCenterX, branchY, rightCenterX, branchY+8)
	p.SetLineWidth(0.75)
	groupBoxY := branchY + 8
	const groupBoxH = 16.0
	p.FilledRect(leftColX, groupBoxY, colW, groupBoxH, 0.03, 0.58, 0.81)
	p.FilledRect(rightColX, groupBoxY, colW, groupBoxH, 0.03, 0.58, 0.81)
	p.SetTextColor(1, 1, 1)
	p.SetFont(true, 8)
	p.TextCentered(leftCenterX, groupBoxY+11, "KELOMPOK JABATAN FUNGSIONAL")
	p.TextCentered(rightCenterX, groupBoxY+11, "KELOMPOK JABATAN PELAKSANA")
	p.SetTextColor(0, 0, 0)

	yTableTop := groupBoxY + groupBoxH + 4

	// ---- budget vertikal: sisakan ruang untuk REKAPITULASI + Keterangan +
	// tanda tangan + footer, supaya tabel tidak sampai tumpang tindih
	// (lihat rincian estimasi pada komentar fungsi ini). ----
	const reserveBawah = 300.0
	yMaxTable := pageH - reserveBawah

	fungsional, pelaksana := splitPetaJabatanByJenis(peta.Rows)
	yLeftEnd := drawPetaJabatanPDFColumn(p, leftColX, yTableTop, colW, fungsional, "Jumlah Kebutuhan JF", yMaxTable)
	yRightEnd := drawPetaJabatanPDFColumn(p, rightColX, yTableTop, colW, pelaksana, "Jumlah Kebutuhan JP", yMaxTable)

	y = yLeftEnd
	if yRightEnd > y {
		y = yRightEnd
	}
	y += 14

	// ---- REKAPITULASI ----
	fB, fK := sumBK(fungsional)
	pB, pK := sumBK(pelaksana)
	totalB, totalK := fB+pB, fK+pK

	rekapW := 260.0
	rekapX := centerX - rekapW/2
	labelW := 130.0
	numW := (rekapW - labelW) / 3
	xRB := rekapX + labelW + numW/2
	xRK := rekapX + labelW + numW + numW/2
	xRSel := rekapX + labelW + numW*2 + numW/2

	p.FilledRect(rekapX, y, rekapW, 15, 0.03, 0.58, 0.81)
	p.SetTextColor(1, 1, 1)
	p.SetFont(true, 9)
	p.TextCentered(rekapX+rekapW/2, y+11, "REKAPITULASI")
	p.SetTextColor(0, 0, 0)
	y += 15
	p.SetLineWidth(0.75)
	p.Rect(rekapX, y, rekapW, 15)
	p.SetFont(true, 8)
	p.TextCentered(xRB, y+11, "B")
	p.TextCentered(xRK, y+11, "K")
	p.TextCentered(xRSel, y+11, "+/-")
	y += 15
	rekapRows := []struct {
		label string
		b, k  int
		bold  bool
	}{
		{"Jabatan Fungsional", fB, fK, false},
		{"Jabatan Pelaksana", pB, pK, false},
		{"JUMLAH", totalB, totalK, true},
	}
	for _, rr := range rekapRows {
		p.Rect(rekapX, y, rekapW, 14)
		p.SetFont(rr.bold, 8)
		p.Text(rekapX+4, y+10, rr.label)
		p.TextCentered(xRB, y+10, fmt.Sprintf("%d", rr.b))
		p.TextCentered(xRK, y+10, fmt.Sprintf("%d", rr.k))
		p.TextCentered(xRSel, y+10, signedStr(rr.b-rr.k))
		y += 14
	}
	y += 14

	// ---- Keterangan ----
	p.SetFont(true, 8)
	p.Text(marginX, y, "Keterangan:")
	y += 11
	p.SetFont(false, 7.5)
	for _, line := range []string{
		"KJ = Kelas Jabatan",
		"B = Bezetting (jumlah pegawai saat ini)",
		"K = Kebutuhan pegawai",
		"+/- = Kelebihan (+) / Kekurangan (-) pegawai",
	} {
		p.Text(marginX, y, line)
		y += 10
	}
	y += 12

	// ---- tanda tangan QR: Kepala Sekolah (kiri) & Kepala Dinas (kanan) ----
	signColW := (rightX - marginX - 20) / 2
	leftSignCenterX := marginX + signColW/2
	rightSignCenterX := rightX - signColW/2
	qrSide := 50.0

	p.SetFont(false, 9)
	p.TextCentered(leftSignCenterX, y, "Mengetahui,")
	p.TextCentered(rightSignCenterX, y, "Kolonodale, "+dicetakStr)
	y += 13
	p.SetFont(true, 9)
	p.TextCentered(leftSignCenterX, y, signers.KepsekJabatan)
	p.TextCentered(rightSignCenterX, y, "Kepala Dinas Pendidikan dan Kebudayaan")
	qrY := y + 6

	kepsekQuery := fmt.Sprintf("Kepala Sekolah %s NIP %s Jabatan %s -- Peta Jabatan Sekolah %s dicetak %s",
		signers.KepsekNama, signers.KepsekNip, signers.KepsekJabatan, unitKerjaNama, dicetakStr)
	if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(kepsekQuery), qrcode.Medium, 180); err == nil {
		if err := doc.RegisterImage("ttd_qr_peta_jabatan_export_kepsek", qrPng); err == nil {
			p.Image("ttd_qr_peta_jabatan_export_kepsek", leftSignCenterX-qrSide/2, qrY, qrSide, qrSide)
		}
	}
	dinasQuery := fmt.Sprintf("Kepala Dinas %s NIP %s Jabatan %s -- Peta Jabatan Sekolah %s dicetak %s",
		signers.DinasNama, signers.DinasNip, signers.DinasJabatan, unitKerjaNama, dicetakStr)
	if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(dinasQuery), qrcode.Medium, 180); err == nil {
		if err := doc.RegisterImage("ttd_qr_peta_jabatan_export_dinas", qrPng); err == nil {
			p.Image("ttd_qr_peta_jabatan_export_dinas", rightSignCenterX-qrSide/2, qrY, qrSide, qrSide)
		}
	}
	p.SetFont(true, 9)
	p.TextCentered(leftSignCenterX, qrY+qrSide+13, signers.KepsekNama)
	p.TextCentered(rightSignCenterX, qrY+qrSide+13, signers.DinasNama)
	p.SetFont(false, 8.5)
	p.TextCentered(leftSignCenterX, qrY+qrSide+25, "NIP. "+signers.KepsekNip)
	p.TextCentered(rightSignCenterX, qrY+qrSide+25, "NIP. "+signers.DinasNip)

	// ---- footer ----
	p.SetFont(false, 7.5)
	p.Text(marginX, pageH-24, "Dicetak dari aplikasi SIMADU pada "+absensiNow().Format("02-01-2006 15:04")+" WITA")
	p.TextRight(rightX, pageH-24, "Halaman 1 dari 1")

	return doc.Output()
}

// drawPetaJabatanPDFColumn menggambar SATU tabel (Jabatan Fungsional ATAU
// Jabatan Pelaksana) di dalam satu kolom lebar colW mulai dari (colX, yTop):
// header kolom (Jabatan/Sub-Jabatan | KJ | B | K | +/-), baris data (Jabatan
// tebal + SubJabatan miring menjorok), lalu baris "Jumlah Kebutuhan ..".
// Berhenti & menulis catatan "+N lainnya.." kalau akan melewati yMax,
// supaya tidak tumpang tindih dengan REKAPITULASI/tanda tangan di bawahnya
// (lihat komentar buildPetaJabatanSekolahExportPDF). Mengembalikan posisi Y
// setelah baris terakhir yang digambar (dipakai pemanggil untuk menyamakan
// posisi lanjutan dengan kolom sebelahnya, yang jumlah barisnya bisa beda).
func drawPetaJabatanPDFColumn(p *utils.PDFPage, colX, yTop, colW float64, rows []jabatanPetaOut, jumlahLabel string, yMax float64) float64 {
	const (
		labelW  = 158.0
		kjW     = 22.0
		bW      = 22.0
		kW      = 22.0
		selW    = 30.0
		rowH    = 13.0
		headerH = 15.0
	)
	xKJ := colX + labelW + kjW/2
	xB := colX + labelW + kjW + bW/2
	xK := colX + labelW + kjW + bW + kW/2
	xSel := colX + labelW + kjW + bW + kW + selW/2

	y := yTop
	p.SetFont(true, 7.5)
	p.Text(colX, y, "Jabatan / Sub-Jabatan")
	p.TextCentered(xKJ, y, "KJ")
	p.TextCentered(xB, y, "B")
	p.TextCentered(xK, y, "K")
	p.TextCentered(xSel, y, "+/-")
	y += 3
	p.Line(colX, y, colX+colW, y)
	y += headerH - 3

	// totalB/totalK dihitung dari SELURUH rows (sama seperti sumBK yang
	// dipakai REKAPITULASI) supaya baris "Jumlah Kebutuhan .." di bawah tabel
	// ini tetap konsisten dengan REKAPITULASI walau tabelnya sendiri terpotong
	// (truncated) -- sebelumnya baris ini salah hitung ulang dari baris yang
	// sempat digambar saja, jadi bisa berbeda angka dengan REKAPITULASI.
	totalB, totalK := sumBK(rows)
	sisa := 0
	truncated := false
	for i, j := range rows {
		// +1 baris cadangan untuk baris "Jumlah" setelahnya, +1 lagi kalau
		// masih ada baris berikutnya yang BELUM digambar (perlu baris
		// catatan "+N lainnya..").
		needed := rowH
		if i < len(rows)-1 {
			needed += rowH
		}
		if y+needed > yMax {
			sisa = len(rows) - i
			truncated = true
			break
		}
		p.SetFont(true, 7.5)
		p.Text(colX, y, truncateLabel(j.Jabatan, labelW, 7.5))
		p.TextCentered(xKJ, y, kjString(j.KJ))
		p.TextCentered(xB, y, fmt.Sprintf("%d", j.B))
		p.TextCentered(xK, y, fmt.Sprintf("%d", j.K))
		p.TextCentered(xSel, y, signedStr(j.Selisih))
		y += rowH
		for _, s := range j.SubJabatan {
			if y+rowH*2 > yMax {
				truncated = true
				break
			}
			p.SetFont(false, 7)
			p.Text(colX+10, y, truncateLabel("- "+s.Nama, labelW-10, 7))
			p.TextCentered(xKJ, y, kjString(s.KJ))
			p.TextCentered(xB, y, fmt.Sprintf("%d", s.B))
			p.TextCentered(xK, y, fmt.Sprintf("%d", s.K))
			p.TextCentered(xSel, y, signedStr(s.Selisih))
			y += rowH
		}
		if truncated {
			sisa = len(rows) - i - 1
			break
		}
	}
	if truncated && sisa > 0 {
		p.SetFont(false, 7)
		p.Text(colX, y, fmt.Sprintf("+%d jabatan lainnya, lihat Excel untuk daftar lengkap", sisa))
		y += rowH
		// REKAPITULASI di bawah tetap dihitung dari SELURUH baris (lihat
		// sumBK pada pemanggil), bukan hanya yang sempat digambar di sini --
		// jadi total B/K yang dicetak tetap benar walau tampilan tabel ini
		// terpotong.
	}

	p.SetFont(true, 7.5)
	p.Rect(colX, y, colW, rowH+2)
	p.Text(colX+4, y+10, jumlahLabel)
	p.TextCentered(xB, y+10, fmt.Sprintf("%d", totalB))
	p.TextCentered(xK, y+10, fmt.Sprintf("%d", totalK))
	p.TextCentered(xSel, y+10, signedStr(totalB-totalK))
	y += rowH + 2

	return y
}

// truncateLabel memotong s dengan "..." kalau lebih lebar dari maxWidth pada
// ukuran font size -- dipakai supaya nama jabatan/sub-jabatan yang panjang
// tidak menabrak kolom KJ/B/K/+- di sebelahnya (kolom tabel PDF ini cukup
// sempit karena dua tabel berdampingan, lihat drawPetaJabatanPDFColumn).
func truncateLabel(s string, maxWidth, size float64) string {
	if utils.TextWidth(s, size) <= maxWidth {
		return s
	}
	ellipsis := "..."
	ellipsisW := utils.TextWidth(ellipsis, size)
	for len([]rune(s)) > 0 {
		s = string([]rune(s)[:len([]rune(s))-1])
		if utils.TextWidth(s, size)+ellipsisW <= maxWidth {
			return s + ellipsis
		}
	}
	return ellipsis
}
