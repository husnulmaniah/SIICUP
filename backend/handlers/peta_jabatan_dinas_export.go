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
// Dinas" (lihat handlers/peta_jabatan_dinas.go) dalam bentuk PDF (SATU
// halaman organogram berjenjang -- lihat drawDinasChartNode -- yang
// langsung melampirkan KJ/B/K/+- setiap jabatan di dalam kotak unitnya
// masing-masing, TANPA halaman tabel terpisah lagi) & Excel (satu lembar
// kerja berisi SELURUH unit secara mendatar/flat, lihat
// buildPetaJabatanDinasExcel, TIDAK berubah). BERBEDA dari
// handlers/peta_jabatan_export.go (Peta Jabatan Sekolah, SATU unit per
// dokumen, organogram HANYA satu tingkat) -- di sini organogramnya SUNGGUHAN
// berjenjang (akar -> Sekretariat/Bidang -> Sub Bagian/Seksi, lihat
// drawDinasChartNode yang REKURSIF mengikuti bentuk pohon apa adanya, bukan
// jumlah level yang digariskan tetap) supaya tetap benar meski administrator
// menambah/mengubah cabang baru di kemudian hari.
//
// Hak akses EKSPOR/CETAK (permintaan pengguna): administrator/admin/atasan
// BOLEH mengunduh PDF & Excel, TAPI pegawai Dinas TIDAK BOLEH -- SENGAJA
// LEBIH KETAT daripada hak MELIHAT pohon di layar (closure "lihat" di
// peta_jabatan_dinas.go, yang tetap mengizinkan pegawai melihat), karena
// itu closure "lihatCetak" di bawah ini BERDIRI SENDIRI (bukan reuse
// "lihat") & sengaja tidak menyertakan "pegawai".
func RegisterPetaJabatanDinasExportRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	lihatCetak := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan") }

	mux.Handle("GET /api/peta-jabatan/dinas/export-excel", lihatCetak(func(w http.ResponseWriter, r *http.Request) { exportPetaJabatanDinasExcel(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/dinas/cetak-pdf", lihatCetak(func(w http.ResponseWriter, r *http.Request) { cetakPetaJabatanDinasPDF(w, r, db) }))
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
// PDF -- SATU halaman organogram berjenjang, dengan KJ/B/K/+- SETIAP
// jabatan dilampirkan LANGSUNG di dalam kotak unitnya masing-masing
// (permintaan pengguna: "cukup cetak halaman kerangka ini saja, dengan
// melampirkan KJ, B, K, +/- setiap jabatan") -- TIDAK ADA LAGI halaman
// tabel terpisah seperti sebelumnya; tanda tangan Kepala Dinas (QR) juga
// dipindah ke bagian bawah halaman yang sama.
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

// dinasBoxColor: warna PITA JUDUL kotak organogram per tipe node -- merah
// (akar), biru (Sekretariat/Bidang, langsung di bawah akar), hijau (Seksi/
// Sub Bagian, tingkat paling bawah) -- murni supaya tingkatan garis komando
// langsung terlihat sekilas tanpa perlu membaca label tipe. Badan (body)
// tabel KJ/B/K/+- di bawah pita judul SELALU putih/teks hitam (lihat
// drawDinasChartNode) supaya banyak baris jabatan tetap mudah dibaca.
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

// Konstanta tata letak kotak organogram -- dinasChartHeaderH TETAP (pita
// judul unit + nama kepala, persis gaya sebelum perubahan ini), sisanya
// mengatur tabel KJ/B/K/+- di badan kotak. dinasChartGroupBarH: tinggi
// pita label kelompok "JABATAN PELAKSANA"/"JABATAN FUNGSIONAL" (permintaan
// pengguna -- lihat drawDinasChartGroup).
const (
	dinasChartHeaderH       = 36.0
	dinasChartRowLineH      = 7.6
	dinasChartBodyPadTop    = 4.0
	dinasChartBodyPadBottom = 3.0
	dinasChartGroupBarH     = 9.0
)

// dinasGroupBarColor: warna pita label kelompok "JABATAN PELAKSANA" (biru
// tua) vs "JABATAN FUNGSIONAL" (abu-abu) -- PERSIS gaya gambar referensi
// pengguna (beda warna dari pita judul unit/dinasBoxColor, supaya kedua
// kelompok ini tetap jelas beda dari pita judul di atasnya).
func dinasGroupBarColor(fungsional bool) (r, g, b float64) {
	if fungsional {
		return 0.42, 0.45, 0.50
	}
	return 0.15, 0.39, 0.64
}

// dinasGroupBodyHeight: tinggi SATU kelompok (Pelaksana ATAU Fungsional)
// di badan kotak organogram -- pita label + SETIAP baris jabatan + SATU
// baris "Jumlah Kebutuhan .." tebal di akhir. Kelompok yang TIDAK punya
// baris (count==0) TIDAK digambar sama sekali, jadi tingginya 0 (tidak
// menyisakan spasi kosong pada kotak unit yang hanya punya satu dari dua
// kelompok).
func dinasGroupBodyHeight(count int) float64 {
	if count == 0 {
		return 0
	}
	return dinasChartGroupBarH + float64(count)*dinasChartRowLineH + dinasChartRowLineH
}

// dinasChartColumns: lebar kolom "Jabatan" + KEEMPAT kolom angka (KJ/B/K/
// +-) di badan putih kotak organogram, dihitung PROPORSIONAL terhadap boxW
// (bukan angka tetap) supaya tetap pas berapa pun lebar kotak yang dipakai.
func dinasChartColumns(boxW float64) (pad, jabatanW, kjW, bW, kW, pmW float64) {
	pad = 5
	innerW := boxW - pad*2
	kjW = innerW * 0.12
	bW = innerW * 0.11
	kW = innerW * 0.11
	pmW = innerW * 0.15
	jabatanW = innerW - kjW - bW - kW - pmW
	return
}

// dinasNodeBoxHeight: tinggi TOTAL kotak satu node -- pita judul (tetap,
// dinasChartHeaderH) + badan putih berisi tabel KJ/B/K/+- SETIAP jabatan
// pada unit ini (kalau ada; akar & Sekretariat biasanya TIDAK punya jabatan
// langsung -- lihat komentar buildNodeDinasOut -- jadi kotaknya hanya pita
// judul saja, persis seperti sebelum perubahan ini), DIPISAH dua kelompok
// Jabatan Pelaksana & Jabatan Fungsional (permintaan pengguna, lihat
// drawDinasChartGroup/splitPetaJabatanByJenis) -- kelompok yang tidak
// punya baris TIDAK menambah tinggi sama sekali (dinasGroupBodyHeight).
func dinasNodeBoxHeight(n nodeDinasOut) float64 {
	if len(n.Rows) == 0 {
		return dinasChartHeaderH
	}
	fungsional, pelaksana := splitPetaJabatanByJenis(n.Rows)
	bodyH := dinasChartBodyPadTop + dinasChartRowLineH /* baris judul kolom (sekali, dibagi kedua kelompok) */ +
		dinasGroupBodyHeight(len(pelaksana)) + dinasGroupBodyHeight(len(fungsional)) +
		dinasChartBodyPadBottom
	return dinasChartHeaderH + bodyH
}

// dinasComputeDepthMaxHeight: tinggi kotak TERBESAR pada tiap kedalaman
// pohon (akar = kedalaman 1) -- dipakai drawDinasChartNode supaya SELURUH
// node pada kedalaman yang sama tetap sejajar horizontal (satu garis cabang
// mendatar yang sama), MESKIPUN tinggi kotak masing-masing node berbeda
// (jumlah jabatan berbeda-beda per unit, lihat dinasNodeBoxHeight).
func dinasComputeDepthMaxHeight(n nodeDinasOut, depth int, out map[int]float64) {
	if h := dinasNodeBoxHeight(n); h > out[depth] {
		out[depth] = h
	}
	for _, c := range n.Anak {
		dinasComputeDepthMaxHeight(c, depth+1, out)
	}
}

// drawDinasChartNode menggambar SATU node -- pita judul berwarna (unit +
// kepala, persis gaya sebelumnya) DIIKUTI badan putih berisi tabel Jabatan/
// KJ/B/K/+- untuk setiap jabatan pada unit ini (kalau ada) -- di tengah
// rentang [xLeft,xRight] pada ketinggian y, lalu REKURSIF menggambar
// seluruh anaknya satu kedalaman di bawah: garis vertikal dari bawah kotak
// ke garis cabang mendatar, lalu garis vertikal pendek dari garis cabang ke
// tiap kotak anak (gaya organogram standar). depthMaxHeight (lihat
// dinasComputeDepthMaxHeight) menjamin garis cabang & kedalaman anak tetap
// SEJAJAR lintas cabang meskipun tinggi kotak tiap node berbeda. Rekursi
// ini TIDAK mengasumsikan jumlah anak/kedalaman/jumlah jabatan tertentu,
// jadi tetap benar berapa pun cabang atau jabatan yang administrator tambah
// di kemudian hari.
// gap: jarak vertikal dari node ini turun ke barisan anaknya SENDIRI --
// nextGap: jarak vertikal yang dipakai SETERUSNYA pada seluruh kedalaman di
// BAWAH barisan anak itu (diteruskan apa adanya pada setiap panggilan
// rekursif). Dipisah (bukan satu nilai gap untuk semua kedalaman) supaya
// buildPetaJabatanDinasPDF bisa MEMPERLEBAR KHUSUS jarak akar->Bidang
// (lihat cabang Sekretariat di bawahnya) TANPA ikut melebarkan jarak
// Bidang->Seksi dst yang tidak perlu diperlebar.
func drawDinasChartNode(p *utils.PDFPage, n nodeDinasOut, xLeft, xRight, y, boxW, gap, nextGap float64, depth int, depthMaxHeight map[int]float64) {
	cx := (xLeft + xRight) / 2
	bx := cx - boxW/2
	thisH := dinasNodeBoxHeight(n)
	r, g, b := dinasBoxColor(n.Tipe)

	// ---- pita judul (unit + kepala) ----
	p.FilledRect(bx, y, boxW, dinasChartHeaderH, r, g, b)
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
	p.TextCentered(cx, y+dinasChartHeaderH-6, truncateLabel(kepalaNama, boxW-8, 6.8))
	p.SetTextColor(0, 0, 0)

	// ---- badan putih: tabel Jabatan/KJ/B/K/+- (kalau unit ini punya
	// jabatan langsung) ----
	if len(n.Rows) > 0 {
		bodyY := y + dinasChartHeaderH
		bodyH := thisH - dinasChartHeaderH
		p.SetLineWidth(0.6)
		p.Rect(bx, bodyY, boxW, bodyH)
		pad, jabatanW, kjW, bW, kW, pmW := dinasChartColumns(boxW)
		jx := bx + pad
		kjCx := jx + jabatanW + kjW/2
		bCx := jx + jabatanW + kjW + bW/2
		kCx := jx + jabatanW + kjW + bW + kW/2
		pmCx := jx + jabatanW + kjW + bW + kW + pmW/2
		rowY := bodyY + dinasChartBodyPadTop + dinasChartRowLineH - 2
		p.SetFont(true, 6)
		p.Text(jx, rowY, "Jabatan")
		p.TextCentered(kjCx, rowY, "KJ")
		p.TextCentered(bCx, rowY, "B")
		p.TextCentered(kCx, rowY, "K")
		p.TextCentered(pmCx, rowY, "+/-")
		p.SetLineWidth(0.4)
		p.Line(bx+2, rowY+2, bx+boxW-2, rowY+2)

		// ---- permintaan pengguna: pisahkan Jabatan Pelaksana & Jabatan
		// Fungsional dengan pita label berwarna + baris "Jumlah Kebutuhan
		// .." masing-masing, PERSIS gaya "JABATAN PELAKSANA"/"JABATAN
		// FUNGSIONAL" pada gambar referensi -- Pelaksana digambar LEBIH
		// DULU (urutan pada gambar); kelompok yang kosong (len==0) SENGAJA
		// tidak digambar sama sekali (lihat drawDinasChartGroup).
		fungsional, pelaksana := splitPetaJabatanByJenis(n.Rows)
		rowY = drawDinasChartGroup(p, bx, cx, rowY, jx, jabatanW, kjCx, bCx, kCx, pmCx,
			"JABATAN PELAKSANA", "Jumlah Kebutuhan JP", pelaksana, false)
		rowY = drawDinasChartGroup(p, bx, cx, rowY, jx, jabatanW, kjCx, bCx, kCx, pmCx,
			"JABATAN FUNGSIONAL", "Jumlah Kebutuhan JF", fungsional, true)
	}

	if len(n.Anak) == 0 {
		return
	}
	totalWeight := dinasLayoutWeight(n)
	levelH := depthMaxHeight[depth]
	branchY := y + levelH + gap/2
	childY := y + levelH + gap
	p.SetLineWidth(1)
	p.Line(cx, y+thisH, cx, branchY)

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
		drawDinasChartNode(p, c, childRanges[i][0], childRanges[i][1], childY, boxW, nextGap, nextGap, depth+1, depthMaxHeight)
	}
}

// drawDinasChartGroup menggambar SATU kelompok (Pelaksana ATAU Fungsional)
// di badan kotak organogram, mulai dari rowY (posisi baris SEBELUM
// kelompok ini, konsisten dengan posisi yang dipakai drawDinasChartNode
// untuk baris judul kolom) -- pita label berwarna (dinasGroupBarColor),
// SETIAP baris jabatan kelompok ini, lalu baris "Jumlah Kebutuhan .."
// TEBAL berisi total B/K/+- KELOMPOK INI SAJA (bukan keseluruhan unit).
// Kelompok yang TIDAK punya baris (rows kosong) SENGAJA tidak menggambar
// apa pun & mengembalikan rowY semula -- unit yang hanya punya satu dari
// dua kelompok tidak menampilkan kelompok lain yang kosong. Mengembalikan
// posisi rowY SETELAH baris terakhir kelompok ini, dipakai pemanggil
// melanjutkan ke kelompok berikutnya.
func drawDinasChartGroup(p *utils.PDFPage, bx, cx, rowY, jx, jabatanW, kjCx, bCx, kCx, pmCx float64, label, subtotalLabel string, rows []jabatanPetaOut, fungsional bool) float64 {
	if len(rows) == 0 {
		return rowY
	}
	r, g, b := dinasGroupBarColor(fungsional)
	p.FilledRect(bx+1, rowY, (cx-bx)*2-2, dinasChartGroupBarH, r, g, b)
	p.SetTextColor(1, 1, 1)
	p.SetFont(true, 6)
	p.TextCentered(cx, rowY+dinasChartGroupBarH-2.5, label)
	p.SetTextColor(0, 0, 0)
	rowY += dinasChartGroupBarH

	for _, row := range rows {
		rowY += dinasChartRowLineH
		p.SetFont(false, 6)
		p.Text(jx, rowY, truncateLabel(row.Jabatan, jabatanW-2, 6))
		p.TextCentered(kjCx, rowY, kjString(row.KJ))
		p.TextCentered(bCx, rowY, fmt.Sprintf("%d", row.B))
		p.TextCentered(kCx, rowY, fmt.Sprintf("%d", row.K))
		p.TextCentered(pmCx, rowY, signedStr(row.Selisih))
	}

	sumB, sumK := sumBK(rows)
	rowY += dinasChartRowLineH
	p.SetFont(true, 6)
	p.Text(jx, rowY, truncateLabel(subtotalLabel, jabatanW-2, 6))
	p.TextCentered(bCx, rowY, fmt.Sprintf("%d", sumB))
	p.TextCentered(kCx, rowY, fmt.Sprintf("%d", sumK))
	p.TextCentered(pmCx, rowY, signedStr(sumB-sumK))
	return rowY
}

// dinasChartDepth: kedalaman pohon (akar = 1) -- dipakai menghitung apakah
// organogram masih muat tinggi halaman yang tersedia.
func dinasChartDepth(n nodeDinasOut) int {
	maxAnak := 0
	for _, c := range n.Anak {
		if d := dinasChartDepth(c); d > maxAnak {
			maxAnak = d
		}
	}
	return 1 + maxAnak
}

// flattenDinasNonRoot: urutan depth-first SELURUH node SELAIN akar (akar
// tidak punya tabel B/K sendiri, lihat komentar buildNodeDinasOut) --
// dipakai oleh lembar kerja Excel (buildPetaJabatanDinasExcel) supaya
// urutannya tetap konsisten dengan pohon yang tampil di layar/organogram.
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

	// ---- SATU halaman custom lebar (landscape), memuat SELURUH organogram
	// + tabel KJ/B/K/+- setiap jabatan di dalam kotaknya masing-masing,
	// serta tanda tangan Kepala Dinas (QR) di bagian bawah -- permintaan
	// pengguna: "cukup cetak halaman kerangka ini saja". Ukuran kertas
	// SENGAJA custom (bukan Legal/A4 baku), lebar & tingginya DIHITUNG
	// otomatis (lihat di bawah) dari jumlah cabang/kedalaman pohon saat ini,
	// supaya tetap muat SATU halaman tanpa kode perlu diubah manual kalau
	// administrator menambah Bidang/Seksi baru di kemudian hari -- PDF
	// viewer/printer tetap bisa menskalakan ke ukuran kertas fisik apa pun
	// saat benar-benar dicetak.
	boxW := 185.0
	levelGap := 40.0
	chartTopY := 72.0
	chartMarginX := 30.0

	// ---- permintaan pengguna: Sekretariat digambar sebagai CABANG
	// TERSENDIRI (elbow, digeser ke kanan) yang terhubung LANGSUNG ke
	// Kepala Dinas, PERSIS gaya bagan "Peta Kelas Jabatan" yang sudah
	// berjalan di instansi ini -- trunk utama (garis lurus dari Kepala
	// Dinas) tetap turun LANGSUNG ke baris Bidang, tidak melalui/terganggu
	// cabang Sekretariat ini. Dipisah berdasarkan TIPE (bukan diasumsikan
	// selalu PERSIS satu) supaya kode ini tetap aman walau suatu saat ada
	// lebih dari satu/tidak ada node bertipe Sekretariat.
	var sekretariatNodes []nodeDinasOut
	var bidangNodes []nodeDinasOut
	for _, c := range root.Anak {
		if c.Tipe == models.StrukturDinasSekretariat {
			sekretariatNodes = append(sekretariatNodes, c)
		} else {
			bidangNodes = append(bidangNodes, c)
		}
	}
	rootUntukBidang := root
	rootUntukBidang.Anak = bidangNodes

	// lebar halaman: proporsional terhadap jumlah "unit daun" pada baris
	// Bidang (dinasLayoutWeight), supaya kotak tidak pernah berdesakan
	// berapa pun cabang yang ditambahkan nanti.
	totalWeight := dinasLayoutWeight(rootUntukBidang)
	chartW := float64(totalWeight)*(boxW+45) + chartMarginX*2
	if chartW < 900 {
		chartW = 900
	}
	chartRightX := chartW - chartMarginX
	chartCenterX := (chartMarginX + chartRightX) / 2

	kepalaCx := chartCenterX
	kepalaH := dinasNodeBoxHeight(root)
	kepalaBottomY := chartTopY + kepalaH

	// ---- hitung (TANPA menggambar -- chartPage belum ada) tinggi cabang
	// Sekretariat, supaya baris Bidang di bawahnya bisa digeser turun
	// cukup jauh untuk tidak bertumpukan dengannya. Sekretariat SELALU
	// dihitung & SELALU akan digambar (lihat loop menggambar di bawah) WALAU
	// kepalanya (Sekretaris/Plt.) belum ditetapkan -- persis permintaan
	// pengguna: "walaupun jabatan sekretaris belum ada tetapi tetap
	// munculkan garis komandonya" (drawDinasChartNode SELALU menggambar
	// kotak + garis komando apa adanya, kepala kosong cuma menampilkan
	// "(belum ditetapkan)", lihat komentar fungsi itu).
	const sekretariatElbowGap = 22.0
	sekretariatBottomY := kepalaBottomY
	type sekretariatLayout struct {
		node           nodeDinasOut
		cx             float64
		depthMaxHeight map[int]float64
		depth          int
	}
	var sekretariatLayouts []sekretariatLayout
	for i, sek := range sekretariatNodes {
		dmh := map[int]float64{}
		dinasComputeDepthMaxHeight(sek, 1, dmh)
		sd := dinasChartDepth(sek)
		sum := 0.0
		for d := 1; d <= sd; d++ {
			sum += dmh[d]
		}
		sekCx := chartRightX - boxW/2 - float64(i)*(boxW+20)
		sekY := kepalaBottomY + sekretariatElbowGap
		if bottom := sekY + sum + float64(sd-1)*levelGap; bottom > sekretariatBottomY {
			sekretariatBottomY = bottom
		}
		sekretariatLayouts = append(sekretariatLayouts, sekretariatLayout{node: sek, cx: sekCx, depthMaxHeight: dmh, depth: sd})
	}

	// ---- baris Bidang: trunk LURUS dari Kepala Dinas, jarak (gap) ke
	// baris ini DIPERLEBAR OTOMATIS kalau perlu supaya tidak bertumpukan
	// dengan cabang Sekretariat di atas -- kedalaman SETELAH baris Bidang
	// (mis. Bidang->Seksi) TETAP pakai levelGap normal (lihat gap/nextGap
	// pada drawDinasChartNode).
	bidangDepthMaxHeight := map[int]float64{}
	dinasComputeDepthMaxHeight(rootUntukBidang, 1, bidangDepthMaxHeight)
	bidangDepth := dinasChartDepth(rootUntukBidang)
	bidangGap := levelGap
	if need := sekretariatBottomY - kepalaBottomY + levelGap; need > bidangGap {
		bidangGap = need
	}
	chartBottomY := chartTopY + bidangDepthMaxHeight[1]
	if bidangDepth > 1 {
		chartBottomY += bidangGap + bidangDepthMaxHeight[2]
		for d := 3; d <= bidangDepth; d++ {
			chartBottomY += levelGap + bidangDepthMaxHeight[d]
		}
	}

	// tinggi halaman: pas sampai dasar organogram + ruang blok tanda tangan
	// Kepala Dinas (QR) di bawahnya.
	const signatureReserve = 180.0
	chartH := chartBottomY + signatureReserve
	if chartH < 500 {
		chartH = 500
	}

	chartPage := utils.NewPDFPage(chartW, chartH)
	doc.AddPage(chartPage)

	chartPage.SetFont(true, 14)
	chartPage.TextCentered(chartCenterX, 26, "PETA JABATAN DINAS")
	chartPage.SetFont(false, 9)
	chartPage.TextCentered(chartCenterX, 41, root.Unit)
	chartPage.SetFont(false, 8)
	chartPage.TextCentered(chartCenterX, 54, "Dicetak: "+dicetakStr+" WITA")

	// ---- gambar cabang Sekretariat (elbow dari Kepala Dinas) ----
	for _, sl := range sekretariatLayouts {
		elbowY := kepalaBottomY + sekretariatElbowGap/2
		sekY := kepalaBottomY + sekretariatElbowGap
		chartPage.SetLineWidth(1)
		chartPage.Line(kepalaCx, kepalaBottomY, kepalaCx, elbowY)
		chartPage.Line(kepalaCx, elbowY, sl.cx, elbowY)
		chartPage.Line(sl.cx, elbowY, sl.cx, sekY)
		drawDinasChartNode(chartPage, sl.node, sl.cx-boxW/2, sl.cx+boxW/2, sekY, boxW, levelGap, levelGap, 1, sl.depthMaxHeight)
	}

	// ---- gambar Kepala Dinas + trunk utama + baris Bidang (dst) ----
	drawDinasChartNode(chartPage, rootUntukBidang, chartMarginX, chartRightX, chartTopY, boxW, bidangGap, levelGap, 1, bidangDepthMaxHeight)

	// ---- tanda tangan Kepala Dinas (QR), di bawah organogram pada halaman
	// yang sama (TIDAK lagi di halaman terpisah) ----
	pengaturan := pengaturanSuratOrDefault(db)
	dinasNama, dinasNip, dinasJabatan, _, _ := resolveSignerFullRekomendasi(db, pengaturan)
	dinasNama, dinasNip = namaOrDash(dinasNama), namaOrDash(dinasNip)
	signY := chartBottomY + 34
	signCenterX := chartRightX - 110
	chartPage.SetFont(false, 9.5)
	chartPage.TextCentered(signCenterX, signY, "Kolonodale, "+dicetakStr)
	chartPage.SetFont(true, 9.5)
	chartPage.TextCentered(signCenterX, signY+14, namaOrDash(dinasJabatan))
	qrY := signY + 22
	qrSide := 60.0
	query := fmt.Sprintf("Kepala Dinas %s NIP %s Jabatan %s -- Peta Jabatan Dinas dicetak %s", dinasNama, dinasNip, namaOrDash(dinasJabatan), dicetakStr)
	if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(query), qrcode.Medium, 220); err == nil {
		if err := doc.RegisterImage("ttd_qr_peta_jabatan_dinas_full", qrPng); err == nil {
			chartPage.Image("ttd_qr_peta_jabatan_dinas_full", signCenterX-qrSide/2, qrY, qrSide, qrSide)
		}
	}
	chartPage.SetFont(true, 9.5)
	chartPage.TextCentered(signCenterX, qrY+qrSide+14, dinasNama)
	chartPage.SetFont(false, 8.5)
	chartPage.TextCentered(signCenterX, qrY+qrSide+26, "NIP. "+dinasNip)

	return doc.Output()
}

// ============================================================
// Excel -- satu lembar kerja, seluruh unit ditulis mendatar (flat)
// ============================================================

// writeDinasJabatanGroupExcel menulis SATU kelompok (Pelaksana ATAU
// Fungsional) jabatan pada satu unit struktur Dinas di lembar kerja Excel,
// mulai dari row -- pita label berwarna (barStyle, beda warna utk
// Pelaksana vs Fungsional, PERSIS gaya tampilan layar/PDF, lihat
// drawDinasChartGroup), SETIAP baris jabatan kelompok ini, lalu baris
// "Jumlah Kebutuhan .." TEBAL berisi total B/K/+- KELOMPOK INI SAJA.
// Kelompok yang TIDAK punya baris (rows kosong) SENGAJA tidak menulis apa
// pun & mengembalikan row semula. Mengembalikan baris SETELAH kelompok
// ini, dipakai pemanggil melanjutkan ke kelompok berikutnya/unit berikutnya.
func writeDinasJabatanGroupExcel(f *excelize.File, sheet string, row int, label, subtotalLabel string, rows []jabatanPetaOut,
	barStyle, labelStyle, numStyle, subtotalLabelStyle, subtotalNumStyle int) int {
	if len(rows) == 0 {
		return row
	}
	f.SetCellValue(sheet, cellRef("A", row), label)
	f.MergeCell(sheet, cellRef("A", row), cellRef("E", row))
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("E", row), barStyle)
	row++

	for _, jr := range rows {
		f.SetCellValue(sheet, cellRef("A", row), jr.Jabatan)
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), labelStyle)
		f.SetCellValue(sheet, cellRef("B", row), kjString(jr.KJ))
		f.SetCellValue(sheet, cellRef("C", row), jr.B)
		f.SetCellValue(sheet, cellRef("D", row), jr.K)
		f.SetCellValue(sheet, cellRef("E", row), signedStr(jr.Selisih))
		f.SetCellStyle(sheet, cellRef("B", row), cellRef("E", row), numStyle)
		row++
	}

	sumB, sumK := sumBK(rows)
	f.SetCellValue(sheet, cellRef("A", row), subtotalLabel)
	f.SetCellStyle(sheet, cellRef("A", row), cellRef("A", row), subtotalLabelStyle)
	f.SetCellValue(sheet, cellRef("C", row), sumB)
	f.SetCellValue(sheet, cellRef("D", row), sumK)
	f.SetCellValue(sheet, cellRef("E", row), signedStr(sumB-sumK))
	f.SetCellStyle(sheet, cellRef("B", row), cellRef("E", row), subtotalNumStyle)
	row++
	return row
}

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
	// pelaksanaBarStyle/fungsionalBarStyle: pita label "JABATAN PELAKSANA"/
	// "JABATAN FUNGSIONAL" (permintaan pengguna) -- warna PERSIS sama
	// dengan dinasGroupBarColor pada PDF (biru tua utk Pelaksana, abu-abu
	// utk Fungsional) supaya ketiga tampilan (layar/PDF/Excel) konsisten.
	pelaksanaBarStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"266399"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"}, Border: excelThinBorder(),
	})
	fungsionalBarStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"6B7280"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"}, Border: excelThinBorder(),
	})
	subtotalLabelStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Border: excelThinBorder()})
	subtotalNumStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Alignment: &excelize.Alignment{Horizontal: "center"}, Border: excelThinBorder()})

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
	cols := []string{"A", "B", "C", "D", "E"}
	headers := []string{"Jabatan", "KJ", "B", "K", "+/-"}

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
		f.SetCellStyle(sheet, cellRef("A", row), cellRef("E", row), colHeaderStyle)
		row++

		// ---- permintaan pengguna: pisahkan Jabatan Pelaksana & Jabatan
		// Fungsional dengan pita label berwarna + baris "Jumlah Kebutuhan
		// .." masing-masing, konsisten dengan tampilan layar & PDF (lihat
		// drawDinasChartGroup) -- Pelaksana lebih dulu, kelompok kosong
		// tidak ditulis sama sekali.
		fungsional, pelaksana := splitPetaJabatanByJenis(fl.node.Rows)
		row = writeDinasJabatanGroupExcel(f, sheet, row, "JABATAN PELAKSANA", "Jumlah Kebutuhan JP", pelaksana,
			pelaksanaBarStyle, labelStyle, numStyle, subtotalLabelStyle, subtotalNumStyle)
		row = writeDinasJabatanGroupExcel(f, sheet, row, "JABATAN FUNGSIONAL", "Jumlah Kebutuhan JF", fungsional,
			fungsionalBarStyle, labelStyle, numStyle, subtotalLabelStyle, subtotalNumStyle)
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
	f.SetColWidth(sheet, "B", "E", 9)

	size := paperSizeLegal
	orientation := "portrait"
	fitToWidth := 1
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Size: &size, Orientation: &orientation, FitToWidth: &fitToWidth})
	fitToPage := true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fitToPage})

	return f, nil
}
