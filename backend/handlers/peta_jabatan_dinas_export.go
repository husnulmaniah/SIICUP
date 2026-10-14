package handlers

import (
	"fmt"
	"net/http"
	neturl "net/url"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// peta_jabatan_dinas_export.go -- cetak/unduh pohon struktur "Peta Jabatan
// Dinas" (lihat handlers/peta_jabatan_dinas.go) dalam bentuk PDF (organogram
// berjenjang + tabel B/K per unit, kertas Legal) & Excel (satu lembar kerja
// berisi SELURUH unit secara mendatar/flat). BERBEDA dari
// handlers/peta_jabatan_export.go (Peta Jabatan Sekolah, SATU unit per
// dokumen, organogram HANYA satu tingkat) -- di sini organogramnya SUNGGUHAN
// berjenjang (akar -> Sekretariat/Bidang -> Sub Bagian/Seksi, lihat
// drawDinasChartNode yang REKURSIF mengikuti bentuk pohon apa adanya, bukan
// jumlah level yang digariskan tetap) supaya tetap benar meski administrator
// menambah/mengubah cabang baru di kemudian hari.
//
// Hak akses EKSPOR/CETAK SENGAJA sama dengan hak MELIHAT (administrator/
// admin/atasan/pegawai, lihat closure "lihat" di peta_jabatan_dinas.go) --
// BUKAN "kelola" -- karena mengunduh dokumen dari data yang SUDAH bisa
// dilihat siapa pun yang berhak di layar bukan aksi mengelola, cukup
// menyajikan ulang data yang sama dalam bentuk cetak.

func RegisterPetaJabatanDinasExportRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	lihat := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan", "pegawai") }

	mux.Handle("GET /api/peta-jabatan/dinas/export-excel", lihat(func(w http.ResponseWriter, r *http.Request) { exportPetaJabatanDinasExcel(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/dinas/cetak-pdf", lihat(func(w http.ResponseWriter, r *http.Request) { cetakPetaJabatanDinasPDF(w, r, db) }))
}

// muatRootPetaJabatanDinas: ambil root + seluruh pohon (reuse buildNodeDinasOut
// dari peta_jabatan_dinas.go) -- dipakai bersama oleh kedua handler export
// di bawah supaya PDF & Excel selalu dicetak dari data pohon yang identik
// dengan yang tampil di menu.
func muatRootPetaJabatanDinas(db *gorm.DB) (*nodeDinasOut, error) {
	var root models.UnitKerja
	if err := db.Where("tempat_kerja = ? AND tipe_struktur_dinas = ?", models.TempatKerjaDinas, models.StrukturDinasRoot).
		First(&root).Error; err != nil {
		return nil, fmt.Errorf("struktur Peta Jabatan Dinas belum dibuat")
	}
	return buildNodeDinasOut(db, root)
}

func exportPetaJabatanDinasExcel(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	root, err := muatRootPetaJabatanDinas(db)
	if err != nil {
		utils.Error(w, http.StatusNotFound, err.Error())
		return
	}
	f, err := buildPetaJabatanDinasExcel(db, *root)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat file excel: "+err.Error())
		return
	}
	writeXlsxResponse(w, f, "peta_jabatan_dinas.xlsx")
}

func cetakPetaJabatanDinasPDF(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	root, err := muatRootPetaJabatanDinas(db)
	if err != nil {
		utils.Error(w, http.StatusNotFound, err.Error())
		return
	}
	pdfBytes, err := buildPetaJabatanDinasPDF(db, *root)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat PDF: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, "peta_jabatan_dinas.pdf")
}

// ============================================================
// PDF -- organogram berjenjang (kertas Legal, landscape) + tabel B/K per
// unit (halaman berikutnya, portrait)
// ============================================================

// dinasLayoutWeight: lebar relatif (dalam satuan "unit daun") yang
// dibutuhkan satu node & seluruh keturunannya pada organogram -- node tanpa
// anak selalu bernilai 1, node dengan anak = SUM bobot seluruh anaknya.
// Dipakai drawDinasChartNode untuk membagi lebar horizontal secara
// proporsional, supaya cabang dengan lebih banyak anak (mis. Bidang dengan
// 2 Seksi) mendapat jatah ruang lebih lebar daripada cabang tanpa anak
// (mis. Bidang Kebudayaan yang tidak punya Seksi pada data ANJAB).
func dinasLayoutWeight(n nodeDinasOut) int {
	if len(n.Anak) == 0 {
		return 1
	}
	total := 0
	for _, c := range n.Anak {
		total += dinasLayoutWeight(c)
	}
	return total
}

// dinasBoxColor: warna kotak organogram per tipe node -- merah (akar),
// biru (Sekretariat/Bidang, langsung di bawah akar), hijau (Seksi/Sub
// Bagian, tingkat paling bawah) -- murni supaya tingkatan garis komando
// langsung terlihat sekilas tanpa perlu membaca label tipe.
func dinasBoxColor(tipe string) (r, g, b float64) {
	switch tipe {
	case models.StrukturDinasRoot:
		return 0.80, 0.11, 0.11
	case models.StrukturDinasSekretariat, models.StrukturDinasBidang:
		return 0.03, 0.58, 0.81
	default: // seksi, sub_bagian
		return 0.16, 0.55, 0.27
	}
}

// drawDinasChartNode menggambar SATU node (kotak berisi nama unit + nama
// kepala) di tengah rentang [xLeft,xRight] pada ketinggian y, lalu REKURSIF
// menggambar seluruh anaknya satu tingkat di bawah -- garis vertikal dari
// bawah kotak ke garis cabang mendatar, lalu garis vertikal pendek dari
// garis cabang ke tiap kotak anak (gaya organogram standar). Rekursi ini
// TIDAK mengasumsikan jumlah anak atau kedalaman tertentu, jadi tetap benar
// berapa pun cabang yang administrator tambahkan di kemudian hari.
func drawDinasChartNode(p *utils.PDFPage, n nodeDinasOut, xLeft, xRight, y, boxW, boxH, levelGap float64) {
	cx := (xLeft + xRight) / 2
	bx := cx - boxW/2
	r, g, b := dinasBoxColor(n.Tipe)
	p.FilledRect(bx, y, boxW, boxH, r, g, b)
	p.SetTextColor(1, 1, 1)
	p.SetFont(true, 7.2)
	// utils.WrapText/TextWidth mengukur lebar berdasarkan tabel Helvetica
	// REGULAR -- teks TEBAL (bold) dirender sedikit LEBIH LEBAR dari hasil
	// ukur itu, jadi lebar yang dipakai untuk membungkus baris judul (tebal)
	// SENGAJA diperkecil (faktor 0.82) supaya tidak melebihi kotak begitu
	// benar-benar dirender tebal, lalu tiap barisnya tetap dipotong ulang
	// (truncateLabel) sebagai jaminan terakhir kalau ukuran katanya sendiri
	// masih kepanjangan.
	titleBudget := (boxW - 8) * 0.82
	ly := y + 9
	wrapped := utils.WrapText(n.Unit, titleBudget, 7.2)
	for i, ln := range wrapped {
		if i >= 2 {
			break
		}
		p.TextCentered(cx, ly, truncateLabel(ln, boxW-8, 7.2))
		ly += 9
	}
	p.SetFont(false, 6.8)
	kepalaNama := "(belum ditetapkan)"
	if n.Kepala != nil && n.Kepala.Nama != "" {
		kepalaNama = n.Kepala.Nama
	}
	p.TextCentered(cx, y+boxH-6, truncateLabel(kepalaNama, boxW-8, 6.8))
	p.SetTextColor(0, 0, 0)

	if len(n.Anak) == 0 {
		return
	}
	totalWeight := dinasLayoutWeight(n)
	branchY := y + boxH + levelGap/2
	childY := y + boxH + levelGap
	p.SetLineWidth(1)
	p.Line(cx, y+boxH, cx, branchY)

	curX := xLeft
	childCenters := make([]float64, len(n.Anak))
	childRanges := make([][2]float64, len(n.Anak))
	for i, c := range n.Anak {
		wgt := dinasLayoutWeight(c)
		childXRight := curX + (xRight-xLeft)*float64(wgt)/float64(totalWeight)
		childRanges[i] = [2]float64{curX, childXRight}
		childCenters[i] = (curX + childXRight) / 2
		curX = childXRight
	}
	if len(childCenters) > 1 {
		p.Line(childCenters[0], branchY, childCenters[len(childCenters)-1], branchY)
	}
	for i, c := range n.Anak {
		ccx := childCenters[i]
		p.Line(ccx, branchY, ccx, childY)
		drawDinasChartNode(p, c, childRanges[i][0], childRanges[i][1], childY, boxW, boxH, levelGap)
	}
}

// dinasChartDepth: kedalaman pohon (akar = 1) -- dipakai menghitung apakah
// organogram masih muat tinggi halaman landscape yang tersedia.
func dinasChartDepth(n nodeDinasOut) int {
	maxAnak := 0
	for _, c := range n.Anak {
		if d := dinasChartDepth(c); d > maxAnak {
			maxAnak = d
		}
	}
	return 1 + maxAnak
}

// drawDinasUnitTable: tabel B/K/+- sederhana (Jabatan | B | K | +/-) untuk
// SATU unit, dimulai pada y -- mengembalikan y setelah tabel (dipakai
// pemanggil untuk tahu posisi berikutnya). Sama gaya kolomnya dengan tabel
// pada peta_jabatan_pdf.go (dokumen Kenaikan Pangkat) supaya pengguna yang
// sudah familiar dengan dokumen itu tidak perlu membiasakan diri dengan
// tata letak baru.
func drawDinasUnitTable(p *utils.PDFPage, marginX, rightX, centerX, y float64, rows []jabatanPetaOut) float64 {
	if len(rows) == 0 {
		p.SetFont(false, 8)
		p.Text(marginX+10, y, "(belum ada data jabatan pada unit ini)")
		return y + 14
	}
	p.SetFont(true, 8.5)
	p.Text(marginX+10, y, "Jabatan")
	p.TextCentered(centerX+110, y, "KJ")
	p.TextCentered(centerX+150, y, "B")
	p.TextCentered(centerX+190, y, "K")
	p.TextCentered(centerX+230, y, "+/-")
	y += 4
	p.SetLineWidth(0.6)
	p.Line(marginX+10, y, rightX, y)
	y += 12
	for _, row := range rows {
		p.SetFont(true, 8)
		p.Text(marginX+10, y, row.Jabatan)
		p.SetFont(false, 8)
		p.TextCentered(centerX+110, y, kjString(row.KJ))
		p.TextCentered(centerX+150, y, fmt.Sprintf("%d", row.B))
		p.TextCentered(centerX+190, y, fmt.Sprintf("%d", row.K))
		p.TextCentered(centerX+230, y, signedStr(row.Selisih))
		y += 12
	}
	return y + 6
}

// flattenDinasNonRoot: urutan depth-first SELURUH node SELAIN akar (akar
// tidak punya tabel B/K sendiri, lihat komentar buildNodeDinasOut) --
// dipakai baik oleh halaman tabel PDF maupun lembar kerja Excel supaya
// urutannya konsisten antara kedua dokumen.
func flattenDinasNonRoot(n nodeDinasOut, depth int, out *[]struct {
	node  nodeDinasOut
	depth int
}) {
	for _, c := range n.Anak {
		*out = append(*out, struct {
			node  nodeDinasOut
			depth int
		}{c, depth})
		flattenDinasNonRoot(c, depth+1, out)
	}
}

func buildPetaJabatanDinasPDF(db *gorm.DB, root nodeDinasOut) ([]byte, error) {
	doc := utils.NewPDFDoc()
	dicetakStr := formatDateID(absensiNow())

	// ---- halaman 1: organogram (landscape Legal) ----
	chartW, chartH := utils.PageHeightLegal, utils.PageWidthLegal // ditukar -> landscape
	chartPage := utils.NewPDFPage(chartW, chartH)
	doc.AddPage(chartPage)
	chartMarginX := 24.0
	chartRightX := chartW - chartMarginX
	chartCenterX := (chartMarginX + chartRightX) / 2

	chartPage.SetFont(true, 13)
	chartPage.TextCentered(chartCenterX, 24, "PETA JABATAN DINAS")
	chartPage.SetFont(false, 8.5)
	chartPage.TextCentered(chartCenterX, 38, root.Unit)
	chartPage.SetFont(false, 7.5)
	chartPage.TextCentered(chartCenterX, 50, "Dicetak: "+dicetakStr+" WITA")

	boxW, boxH := 128.0, 36.0
	levelGap := 40.0
	chartTopY := 66.0
	depth := dinasChartDepth(root)
	neededH := chartTopY + float64(depth)*(boxH+levelGap)
	if neededH > chartH-20 {
		// jarak antar level diperkecil otomatis kalau pohonnya bertambah dalam
		// di masa depan, supaya tetap muat SATU halaman tanpa kode perlu
		// diubah manual.
		levelGap = (chartH - 20 - chartTopY - float64(depth)*boxH) / float64(depth)
		if levelGap < 18 {
			levelGap = 18
		}
	}
	drawDinasChartNode(chartPage, root, chartMarginX, chartRightX, chartTopY, boxW, boxH, levelGap)

	// ---- halaman 2+: tabel B/K per unit (portrait Legal) ----
	var flat []struct {
		node  nodeDinasOut
		depth int
	}
	flattenDinasNonRoot(root, 1, &flat)

	tableMarginX := 36.0
	tableW, tableH := utils.PageWidthLegal, utils.PageHeightLegal
	tableRightX := tableW - tableMarginX
	tableCenterX := (tableMarginX + tableRightX) / 2
	var tp *utils.PDFPage
	var y float64
	newTablePage := func() {
		tp = utils.NewPDFPage(tableW, tableH)
		doc.AddPage(tp)
		tp.SetFont(true, 12)
		tp.TextCentered(tableCenterX, 30, "PETA JABATAN DINAS -- TABEL B/K PER UNIT")
		tp.SetFont(false, 8)
		tp.TextCentered(tableCenterX, 44, "Dicetak: "+dicetakStr+" WITA")
		y = 64
	}
	newTablePage()
	for _, fl := range flat {
		estTinggi := 30.0 + float64(len(fl.node.Rows))*12
		if y+estTinggi > tableH-50 {
			newTablePage()
		}
		indent := float64(fl.depth-1) * 14
		tp.SetFont(true, 9.5)
		judulUnit := labelTipeStrukturDinas(fl.node.Tipe) + ": " + fl.node.Unit
		tp.Text(tableMarginX+indent, y, judulUnit)
		y += 11
		tp.SetFont(false, 8)
		kepalaNama := "(belum ditetapkan)"
		if fl.node.Kepala != nil && fl.node.Kepala.Nama != "" {
			kepalaNama = fl.node.Kepala.Nama + " (NIP. " + namaOrDash(fl.node.Kepala.NIP) + ")"
		}
		tp.Text(tableMarginX+indent+6, y, "Kepala: "+kepalaNama)
		y += 12
		y = drawDinasUnitTable(tp, tableMarginX+indent, tableRightX, tableCenterX, y, fl.node.Rows)
		y += 8
	}

	// ---- tanda tangan Kepala Dinas (QR), di halaman tabel terakhir ----
	pengaturan := pengaturanSuratOrDefault(db)
	dinasNama, dinasNip, dinasJabatan, _, _ := resolveSignerFullRekomendasi(db, pengaturan)
	dinasNama, dinasNip = namaOrDash(dinasNama), namaOrDash(dinasNip)
	if y > tableH-150 {
		newTablePage()
		y = tableH - 140
	} else {
		y = tableH - 140
	}
	signCenterX := tableRightX - 100
	tp.SetFont(false, 9)
	tp.TextCentered(signCenterX, y, "Kolonodale, "+dicetakStr)
	tp.SetFont(true, 9)
	tp.TextCentered(signCenterX, y+13, namaOrDash(dinasJabatan))
	qrY := y + 20
	qrSide := 65.0
	query := fmt.Sprintf("Kepala Dinas %s NIP %s Jabatan %s -- Peta Jabatan Dinas dicetak %s", dinasNama, dinasNip, namaOrDash(dinasJabatan), dicetakStr)
	if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(query), qrcode.Medium, 220); err == nil {
		if err := doc.RegisterImage("ttd_qr_peta_jabatan_dinas_full", qrPng); err == nil {
			tp.Image("ttd_qr_peta_jabatan_dinas_full", signCenterX-qrSide/2, qrY, qrSide, qrSide)
		}
	}
	tp.SetFont(true, 9)
	tp.TextCentered(signCenterX, qrY+qrSide+14, dinasNama)
	tp.SetFont(false, 8.5)
	tp.TextCentered(signCenterX, qrY+qrSide+26, "NIP. "+dinasNip)

	return doc.Output()
}

// ============================================================
// Excel -- satu lembar kerja, seluruh unit ditulis mendatar (flat)
// ============================================================

func buildPetaJabatanDinasExcel(db *gorm.DB, root nodeDinasOut) (*excelize.File, error) {
	f := excelize.NewFile()
	const sheet = "Peta Jabatan Dinas"
	f.SetSheetName("Sheet1", sheet)

	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}, Alignment: &excelize.Alignment{Horizontal: "center"}})
	subtitleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Size: 11}, Alignment: &excelize.Alignment{Horizontal: "center"}})
	unitHeaderStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"0EA5E9"}, Pattern: 1},
		Border: excelThinBorder(),
	})
	colHeaderStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"2563EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"}, Border: excelThinBorder(),
	})
	labelStyle, _ := f.NewStyle(&excelize.Style{Border: excelThinBorder()})
	numStyle, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "center"}, Border: excelThinBorder()})
	emptyStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true, Color: "6B7280"}, Border: excelThinBorder()})
	signPlainStyle, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "center"}})
	signBoldStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Alignment: &excelize.Alignment{Horizontal: "center"}})

	f.SetCellValue(sheet, "A1", "PETA JABATAN DINAS")
	f.MergeCell(sheet, "A1", "H1")
	f.SetCellStyle(sheet, "A1", "H1", titleStyle)
	f.SetCellValue(sheet, "A2", root.Unit)
	f.MergeCell(sheet, "A2", "H2")
	f.SetCellStyle(sheet, "A2", "H2", subtitleStyle)
	dicetakStr := formatDateID(absensiNow())
	f.SetCellValue(sheet, "A3", "Dicetak: "+dicetakStr)
	f.MergeCell(sheet, "A3", "H3")
	f.SetCellStyle(sheet, "A3", "H3", subtitleStyle)

	row := 5
	cols := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	headers := []string{"Jabatan", "Jenis Jabatan", "KJ", "B", "K", "+/-"}

	var flat []struct {
		node  nodeDinasOut
		depth int
	}
	flattenDinasNonRoot(root, 1, &flat)

	for _, fl := range flat {
		kepalaNama := "(belum ditetapkan)"
		if fl.node.Kepala != nil && fl.node.Kepala.Nama != "" {
			kepalaNama = fl.node.Kepala.Nama
		}
		judul := fmt.Sprintf("%s: %s -- Kepala: %s", labelTipeStrukturDinas(fl.node.Tipe), fl.node.Unit, kepalaNama)
		f.SetCellValue(sheet, cellRef("A", row), judul)
		f.MergeCell(sheet, cellRef("A", row), cellRef("H", row))
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("H", row), unitHeaderStyle)
		row++

		if len(fl.node.Rows) == 0 {
			f.SetCellValue(sheet, cellRef("A", row), "(belum ada data jabatan pada unit ini)")
			f.MergeCell(sheet, cellRef("A", row), cellRef("H", row))
			f.SetCellStyle(sheet, cellRef("A", row), cellRef("H", row), emptyStyle)
			row++
			continue
		}

		for i, h := range headers {
			f.SetCellValue(sheet, cellRef(cols[i], row), h)
		}
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("F", row), colHeaderStyle)
		row++
		for _, jr := range fl.node.Rows {
			f.SetCellValue(sheet, cellRef("A", row), jr.Jabatan)
			f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), labelStyle)
			f.SetCellValue(sheet, cellRef("B", row), jr.JenisJabatan)
			f.SetCellValue(sheet, cellRef("C", row), kjString(jr.KJ))
			f.SetCellValue(sheet, cellRef("D", row), jr.B)
			f.SetCellValue(sheet, cellRef("E", row), jr.K)
			f.SetCellValue(sheet, cellRef("F", row), signedStr(jr.Selisih))
			f.SetCellStyle(sheet, cellRef("B", row), cellRef("F", row), numStyle)
			row++
		}
		row++ // jarak antar unit
	}

	row++
	pengaturan := pengaturanSuratOrDefault(db)
	dinasNama, dinasNip, dinasJabatan, _, _ := resolveSignerFullRekomendasi(db, pengaturan)
	dinasNama, dinasNip = namaOrDash(dinasNama), namaOrDash(dinasNip)

	f.SetCellValue(sheet, cellRef("A", row), "Kolonodale, "+dicetakStr)
	f.MergeCell(sheet, cellRef("A", row), cellRef("H", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("H", row), signPlainStyle)
	row++
	f.SetCellValue(sheet, cellRef("A", row), namaOrDash(dinasJabatan))
	f.MergeCell(sheet, cellRef("A", row), cellRef("H", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("H", row), signBoldStyle)
	row += 6 // ruang tanda tangan basah/QR cetak manual
	f.SetCellValue(sheet, cellRef("A", row), dinasNama)
	f.MergeCell(sheet, cellRef("A", row), cellRef("H", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("H", row), signBoldStyle)
	row++
	f.SetCellValue(sheet, cellRef("A", row), "NIP. "+dinasNip)
	f.MergeCell(sheet, cellRef("A", row), cellRef("H", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("H", row), signPlainStyle)

	f.SetColWidth(sheet, "A", "A", 40)
	f.SetColWidth(sheet, "B", "B", 16)
	f.SetColWidth(sheet, "C", "F", 9)

	size := paperSizeLegal
	orientation := "portrait"
	fitToWidth := 1
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Size: &size, Orientation: &orientation, FitToWidth: &fitToWidth})
	fitToPage := true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fitToPage})

	return f, nil
}
