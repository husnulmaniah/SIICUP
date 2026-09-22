package handlers

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"cuti-app/assets"
	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// kp4_pdf.go cetak formulir KP4 ("Surat Keterangan Untuk Mendapatkan
// Pembayaran Tunjangan Keluarga") jadi PDF 2 halaman: halaman 1 data
// pegawai (meniru layout label:value pada Form. KP4 kosong.docx), halaman 2
// data keluarga (tabel istri/suami & anak-anak). Kepala Dinas/PLT
// ditandatangani otomatis memakai data yang SAMA dengan formulir cuti
// (models.PengaturanSurat, resolveSignerInfo di formulir.go) termasuk QR
// tanda tangan otomatis (lihat buildKp4SignatureQR).

func jenisKelaminLabel(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case models.Kp4JenisKelaminL:
		return "Laki-laki"
	case models.Kp4JenisKelaminP:
		return "Perempuan"
	default:
		return "-"
	}
}

func statusAnakLabel(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case models.Kp4StatusAnakKandung:
		return "Anak Kandung"
	case models.Kp4StatusAnakTiri:
		return "Anak Tiri"
	case models.Kp4StatusAnakAngkat:
		return "Anak Angkat"
	default:
		return "-"
	}
}

func sudahBelumLabel(v bool) string {
	if v {
		return "Sudah"
	}
	return "Belum"
}

func dapatTidakLabel(v bool) string {
	if v {
		return "Dapat"
	}
	return "Tidak"
}

func masihTidakLabel(v bool) string {
	if v {
		return "Masih"
	}
	return "Tidak"
}

// formatRupiahKp4 renders a rupiah amount with "." as pemisah ribuan, gaya
// Indonesia, mis. 4500000 -> "Rp 4.500.000".
func formatRupiahKp4(v float64) string {
	n := int64(v + 0.5)
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	res := "Rp " + string(out)
	if neg {
		res = "-" + res
	}
	return res
}

func formatTglOrDash(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return formatDateID(*t)
}

// buildKp4SignatureQR sama pola dengan buildSignatureQR (formulir.go) --
// encode URL pencarian Google berisi kalimat berlabel "Nama Pejabat: ...
// Jabatan: ... Mengesahkan ..." supaya scan QR dari HP membuka ringkasan
// hasil pencarian, bukan cuma teks mentah.
func buildKp4SignatureQR(signerNama, signerJabatan string, pegawai models.Pegawai, tglCetak time.Time) ([]byte, error) {
	query := fmt.Sprintf(
		"Nama Pejabat: %s Jabatan: %s Mengesahkan Surat Keterangan KP4 (Tunjangan Keluarga) atas nama %s Ditandatangani pada tanggal %s di Kolonodale",
		namaOrDash(signerNama),
		namaOrDash(signerJabatan),
		pegawai.Nama,
		formatDateID(tglCetak),
	)
	googleURL := "https://www.google.com/search?q=" + neturl.QueryEscape(query)
	return qrcode.Encode(googleURL, qrcode.Medium, 240)
}

func drawKp4SignatureQR(doc *utils.PDFDoc, p *utils.PDFPage, name, signerNama, signerJabatan string, pegawai models.Pegawai, tglCetak time.Time, x, yTop, side float64) {
	png, err := buildKp4SignatureQR(signerNama, signerJabatan, pegawai, tglCetak)
	if err != nil {
		return
	}
	if err := doc.RegisterImage(name, png); err != nil {
		return
	}
	p.Image(name, x, yTop, side, side)
}

// ---------------------------------------------------------------------------
// Tabel generik sederhana (border kotak per sel, header boleh melipat baris,
// isi baris SATU baris teks per sel) -- dipakai untuk tabel istri/suami &
// anak-anak. Tidak ada helper tabel generik di utils/pdfwriter.go (lihat
// catatan di sana), jadi ditulis khusus di sini.
// ---------------------------------------------------------------------------

type kp4Kolom struct {
	Judul string
	Lebar float64
}

func drawKp4Table(p *utils.PDFPage, x, yTop float64, kolom []kp4Kolom, rows [][]string) float64 {
	const headerFont = 7.5
	const bodyFont = 7.5
	const pad = 3.0

	p.SetFont(true, headerFont)
	headerLineH := headerFont + 3
	maxHeaderLines := 1
	headerLines := make([][]string, len(kolom))
	for i, k := range kolom {
		// *boldWidthSafety: utils.WrapText/TextWidth cuma punya metrik lebar
		// Helvetica REGULER (lihat catatan boldWidthSafety di formulir.go) --
		// header tabel ini dicetak BOLD, yang glyph-nya lebih lebar dari
		// perkiraan itu, jadi tanpa faktor pengaman ini teks header bisa
		// meluber sedikit melewati garis kolom (pernah terjadi pada kolom
		// "No. Putusan Pengadilan (Khusus Aa)" sebelum faktor ini ditambahkan).
		lines := utils.WrapText(k.Judul, (k.Lebar-2*pad)*boldWidthSafety, headerFont)
		if len(lines) == 0 {
			lines = []string{""}
		}
		headerLines[i] = lines
		if len(lines) > maxHeaderLines {
			maxHeaderLines = len(lines)
		}
	}
	headerH := float64(maxHeaderLines)*headerLineH + 2*pad

	cx := x
	for i, k := range kolom {
		p.Rect(cx, yTop, k.Lebar, headerH)
		ty := yTop + pad + headerFont
		for _, ln := range headerLines[i] {
			p.TextCentered(cx+k.Lebar/2, ty, ln)
			ty += headerLineH
		}
		cx += k.Lebar
	}
	y := yTop + headerH

	p.SetFont(false, bodyFont)
	rowH := bodyFont + 2*pad + 2
	for _, row := range rows {
		cx = x
		for i, k := range kolom {
			p.Rect(cx, y, k.Lebar, rowH)
			val := "-"
			if i < len(row) && row[i] != "" {
				val = row[i]
			}
			// potong ke satu baris supaya tinggi baris tetap seragam --
			// nilai KP4 di tabel ini pendek/diskrit (nama, tanggal, status),
			// bukan paragraf, jadi pemotongan ini jarang benar-benar kena.
			lines := utils.WrapText(val, k.Lebar-2*pad, bodyFont)
			display := val
			if len(lines) > 0 {
				display = lines[0]
			}
			p.TextCentered(cx+k.Lebar/2, y+pad+bodyFont-1, display)
			cx += k.Lebar
		}
		y += rowH
	}
	return y
}

// ---------------------------------------------------------------------------
// Halaman 1: Data Pegawai
// ---------------------------------------------------------------------------

func buildKp4Halaman1(doc *utils.PDFDoc, ring *kp4Ringkasan, pengaturan models.PengaturanKp4, signerNama, signerNip, signerJabatan string, tglCetak time.Time) error {
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		return err
	}

	marginX := 40.0
	rightX := utils.PageWidthA4 - marginX
	y := drawLetterhead(p, marginX, rightX)
	y += 14

	p.SetFont(true, 12)
	p.TextCentered((marginX+rightX)/2, y, "KP4")
	y += 15
	p.SetFont(true, 11)
	p.TextCentered((marginX+rightX)/2, y, "SURAT KETERANGAN")
	y += 13
	p.TextCentered((marginX+rightX)/2, y, "UNTUK MENDAPATKAN PEMBAYARAN TUNJANGAN KELUARGA")
	y += 18
	p.SetLineWidth(1.0)
	p.Line(marginX, y, rightX, y)
	p.SetLineWidth(0.75)
	y += 14

	labelW := 180.0
	valX := marginX + labelW
	lineH := 15.0
	baris := func(label, value string) {
		p.SetFont(false, 10)
		p.Text(marginX, y, label)
		p.Text(marginX+labelW-10, y, ":")
		lines := utils.WrapText(value, rightX-valX, 10)
		if len(lines) == 0 {
			lines = []string{"-"}
		}
		for i, ln := range lines {
			p.Text(valX, y+float64(i)*lineH, ln)
		}
		y += lineH * float64(len(lines))
	}

	baris("Nama Instansi", namaOrDash(pengaturan.NamaInstansi))
	baris("Alamat Lengkap Instansi", namaOrDash(pengaturan.AlamatInstansi))
	baris("Instansi Induk", namaOrDash(pengaturan.InstansiInduk))
	baris("Bendaharawan Gaji", namaOrDash(pengaturan.BendaharawanGaji))
	y += 6

	p.SetFont(true, 10.5)
	p.Text(marginX, y, "DATA PEGAWAI :")
	y += lineH

	pg := ring.Pegawai
	kp4 := ring.Kp4Data
	if kp4 == nil {
		kp4 = &models.Kp4Data{}
	}

	pangkatGol := "-"
	if pg.PangkatGol != nil {
		namaPangkat := "-"
		namaGol := "-"
		if pg.PangkatGol.Pangkat != nil {
			namaPangkat = pg.PangkatGol.Pangkat.Pangkat
		}
		if pg.PangkatGol.Gol != nil {
			namaGol = pg.PangkatGol.Gol.Gol
		}
		pangkatGol = namaPangkat + " / " + namaGol
	}
	statusKepegawaian := "-"
	if pg.Status != nil {
		statusKepegawaian = pg.Status.Status
	}
	jenisJabatan := "-"
	if pg.Jabatan != nil {
		switch pg.Jabatan.JenisJabatan {
		case models.JenisJabatanStruktural:
			jenisJabatan = "Struktural"
		case models.JenisJabatanFungsional:
			jenisJabatan = "Fungsional"
		default:
			jenisJabatan = "Pelaksana"
		}
	}

	baris("Nama Lengkap", namaOrDash(pg.Nama))
	baris("N. I. P", namaOrDash(pg.NIP))
	baris("Pangkat / Golongan (ruang)", pangkatGol)
	baris("T.M.T Golongan (ruang)", formatTglOrDash(pg.TglKenaikanPangkatTerakhir))
	baris("Tempat / Tanggal Lahir", namaOrDash(kp4.TempatLahir)+" / "+formatTglOrDash(pg.TglLahir))
	baris("Jenis Kelamin", jenisKelaminLabel(kp4.JenisKelamin))
	baris("Agama / Kebangsaan", namaOrDash(kp4.Agama)+" / Indonesia")
	baris("Alamat Lengkap",
		fmt.Sprintf("%s, Desa/Kel. %s, Kec. %s, %s, %s",
			namaOrDash(kp4.AlamatJalan), namaOrDash(kp4.Desa), namaOrDash(kp4.Kecamatan), namaOrDash(kp4.Kabupaten), namaOrDash(kp4.Provinsi)))
	baris("T.M.T CPNS", formatTglOrDash(pg.TMT))
	baris("Status Kepegawaian", statusKepegawaian)
	baris("Digaji Menurut (PP/SK)", namaOrDash(kp4.DigajiMenurut))
	baris("Besarnya Penghasilan", formatRupiahKp4(kp4.BesarnyaPenghasilan))
	baris("Jabatan Struktural / Fungsional", jenisJabatan)
	baris("Jumlah Keluarga Tertanggung", fmt.Sprintf("%d Orang", ring.JumlahKeluargaTertanggung))
	baris("SK Terakhir yang Dimiliki", namaOrDash(kp4.SkTerakhir))
	baris("Masa Kerja Golongan", ring.MasaKerjaGolongan)
	baris("Masa Kerja Keseluruhan", ring.MasaKerjaKeseluruhan)
	y += 8

	p.SetFont(false, 9)
	pernyataan := "Keterangan ini saya buat dengan sesungguhnya dan apabila keterangan ini tidak benar (palsu), saya bersedia dituntut di muka Pengadilan berdasarkan Undang-undang yang berlaku, dan bersedia mengembalikan semua uang tunjangan yang telah saya terima yang seharusnya bukan menjadi hak saya."
	y = p.MultilineText(marginX, y, rightX-marginX, 13, pernyataan)
	y += 20

	drawKp4TandaTangan(doc, p, "1", ring.Pegawai, signerNama, signerNip, signerJabatan, tglCetak, marginX, rightX, y)
	return nil
}

// drawKp4TandaTangan menggambar blok tanda tangan Kepala Dinas (kiri,
// otomatis dengan QR) & pegawai yang bersangkutan (kanan, kolom tanda
// tangan basah -- sesuai formulir asli, tidak diisi otomatis) berdampingan,
// dipakai di akhir halaman 1 & halaman 2. qrNameSuffix membedakan nama
// gambar QR yang diregister per halaman (RegisterImage butuh nama unik per
// dokumen).
func drawKp4TandaTangan(doc *utils.PDFDoc, p *utils.PDFPage, qrNameSuffix string, pegawai models.Pegawai, signerNama, signerNip, signerJabatan string, tglCetak time.Time, marginX, rightX, yTop float64) {
	colW := (rightX - marginX - 30) / 2
	leftX := marginX
	rightColX := marginX + colW + 30
	// Titik tengah masing-masing kolom -- dipakai supaya nama+garis+NIP
	// digambar rata tengah kolom (bukan rata kiri) dan garis bawah tanda
	// tangan mengikuti panjang nama yang sebenarnya, bukan lebar kolom penuh
	// (lihat truncateToWidth/boldWidthSafety di formulir.go untuk pola yang
	// sama dipakai pada blok TTE kepala dinas di formulir cuti).
	leftColCenter := leftX + colW/2
	rightColCenter := rightColX + colW/2

	p.SetFont(false, 10)
	p.Text(leftX, yTop, "Mengetahui / Mengesahkan :")
	y1 := yTop + 13
	jabatanLines := utils.WrapText(namaOrDash(signerJabatan), colW, 10)
	for _, jl := range jabatanLines {
		p.Text(leftX, y1, jl)
		y1 += 13
	}
	qrSide := 70.0
	drawKp4SignatureQR(doc, p, "kp4_qr_"+qrNameSuffix, signerNama, signerJabatan, pegawai, tglCetak, leftX, y1+4, qrSide)
	y1 += qrSide + 16
	p.SetFont(true, 10)
	signerNamaDisp := truncateToWidth(namaOrDash(signerNama), (colW-10)*boldWidthSafety, 10)
	p.TextCentered(leftColCenter, y1, signerNamaDisp)
	wLeft := utils.TextWidth(signerNamaDisp, 10)
	p.Line(leftColCenter-wLeft/2, y1+3, leftColCenter+wLeft/2, y1+3)
	y1 += 13
	p.SetFont(false, 10)
	p.TextCentered(leftColCenter, y1, "NIP. "+namaOrDash(signerNip))

	p.SetFont(false, 10)
	p.Text(rightColX, yTop, "Kolonodale, "+formatDateID(tglCetak)+".")
	y2 := yTop + 13
	p.Text(rightColX, y2, "Pegawai yang bersangkutan,")
	y2 += 13 + 70 + 16 // ruang kosong setinggi blok QR di kiri, utk tanda tangan basah
	p.SetFont(true, 10)
	pegawaiNamaDisp := truncateToWidth(namaOrDash(pegawai.Nama), (colW-10)*boldWidthSafety, 10)
	p.TextCentered(rightColCenter, y2, pegawaiNamaDisp)
	wRight := utils.TextWidth(pegawaiNamaDisp, 10)
	p.Line(rightColCenter-wRight/2, y2+3, rightColCenter+wRight/2, y2+3)
	y2 += 13
	p.SetFont(false, 10)
	p.TextCentered(rightColCenter, y2, "NIP. "+namaOrDash(pegawai.NIP))
}

// ---------------------------------------------------------------------------
// Halaman 2: Data Keluarga
// ---------------------------------------------------------------------------

func buildKp4Halaman2(doc *utils.PDFDoc, ring *kp4Ringkasan, signerNama, signerNip, signerJabatan string, tglCetak time.Time) error {
	// Landscape Legal (1008 x 612) -- lebih lega untuk tabel anak-anak yang
	// punya 12 kolom.
	p := utils.NewPDFPage(utils.PageHeightLegal, utils.PageWidthLegal)
	doc.AddPage(p)

	marginX := 30.0
	rightX := p.W - marginX
	y := 30.0

	p.SetFont(true, 12)
	p.TextCentered((marginX+rightX)/2, y, "DATA KELUARGA KP4 - "+strings.ToUpper(ring.Pegawai.Nama)+" (NIP. "+namaOrDash(ring.Pegawai.NIP)+")")
	y += 22

	p.SetFont(true, 10.5)
	p.Text(marginX, y, "I. DATA KELUARGA (YANG MENJADI TANGGUNGAN PEGAWAI)")
	y += 14
	p.SetFont(false, 9.5)
	if ring.Pasangan != nil {
		p.Text(marginX, y, "Kawin syah dengan Isteri/Suami sebagaimana data berikut:")
	} else {
		p.Text(marginX, y, "Belum/tidak memiliki Isteri/Suami yang menjadi tanggungan.")
	}
	y += 16

	kolomPasangan := []kp4Kolom{
		{"No.", 30},
		{"Nama Isteri/Suami", 160},
		{"Tempat Lahir", 110},
		{"Tanggal Lahir", 80},
		{"N.I.K", 130},
		{"Pekerjaan", 110},
		{"Tanggal Perkawinan", 90},
		{"Isteri/Suami Ke-", 80},
		{"Penghasilan Perbulan", 100},
	}
	var barisPasangan [][]string
	if ring.Pasangan != nil {
		ps := ring.Pasangan
		barisPasangan = [][]string{{
			"1.", namaOrDash(ps.Nama), namaOrDash(ps.TempatLahir), formatTglOrDash(ps.TglLahir),
			namaOrDash(ps.NIK), namaOrDash(ps.Pekerjaan), formatTglOrDash(ps.TglPerkawinan),
			fmt.Sprintf("Ke-%d", ps.PasanganKe), formatRupiahKp4(ps.Penghasilan),
		}}
	} else {
		barisPasangan = [][]string{{"1.", "-", "-", "-", "-", "-", "-", "-", "-"}}
	}
	y = drawKp4Table(p, marginX, y, kolomPasangan, barisPasangan)
	y += 22

	p.SetFont(true, 10.5)
	p.Text(marginX, y, "II. ANAK-ANAK YANG MENJADI TANGGUNGAN")
	y += 14
	p.SetFont(false, 9)
	y = p.MultilineText(marginX, y, rightX-marginX, 12,
		"Mempunyai anak-anak seperti dalam daftar di bawah ini, yaitu Anak Kandung (Ak)/Anak Tiri (At)/Anak Angkat (Aa) yang masih menjadi tanggungan, belum mempunyai pekerjaan sendiri, dan masuk dalam Daftar Gaji.")
	y += 8

	kolomAnak := []kp4Kolom{
		{"No.", 26},
		{"Nama Anak", 130},
		{"Tempat Lahir", 85},
		{"Tanggal Lahir", 70},
		{"Status Anak", 65},
		{"Dari Isteri/Suami Ke-", 70},
		{"Jenis Kelamin", 65},
		{"Dapat/Tidak Tunjangan", 70},
		{"Sudah/Belum Kawin", 65},
		{"Sudah/Belum Bekerja", 65},
		{"Masih/Tidak Sekolah/Kuliah", 90},
		{"No. Putusan Pengadilan (Khusus Aa)", 137},
	}
	var barisAnak [][]string
	if len(ring.Anak) == 0 {
		barisAnak = [][]string{{"1.", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-"}}
	} else {
		for i, a := range ring.Anak {
			noPutusan := "-"
			if a.StatusAnak == models.Kp4StatusAnakAngkat {
				noPutusan = namaOrDash(a.NoPutusanPengadilan)
			}
			barisAnak = append(barisAnak, []string{
				fmt.Sprintf("%d.", i+1),
				namaOrDash(a.Nama),
				namaOrDash(a.TempatLahir),
				formatTglOrDash(a.TglLahir),
				statusAnakLabel(a.StatusAnak),
				fmt.Sprintf("Ke-%d", a.DariPasanganKe),
				jenisKelaminLabel(a.JenisKelamin),
				dapatTidakLabel(a.DapatTunjangan),
				sudahBelumLabel(a.SudahKawin),
				sudahBelumLabel(a.SudahBekerja),
				masihTidakLabel(a.MasihSekolah),
				noPutusan,
			})
		}
	}
	y = drawKp4Table(p, marginX, y, kolomAnak, barisAnak)
	y += 26

	drawKp4TandaTangan(doc, p, "2", ring.Pegawai, signerNama, signerNip, signerJabatan, tglCetak, marginX, rightX, y)
	return nil
}

func buildKp4PDF(db *gorm.DB, idPegawai uint) ([]byte, error) {
	ring, err := muatKp4Ringkasan(db, idPegawai)
	if err != nil {
		return nil, err
	}
	pengaturan := pengaturanKp4OrDefault(db)
	pengaturanSurat := pengaturanSuratOrDefault(db)
	signerNama, signerNip, signerJabatan := resolveSignerInfo(db, pengaturanSurat)
	tglCetak := time.Now()

	doc := utils.NewPDFDoc()
	if err := buildKp4Halaman1(doc, ring, pengaturan, signerNama, signerNip, signerJabatan, tglCetak); err != nil {
		return nil, err
	}
	if err := buildKp4Halaman2(doc, ring, signerNama, signerNip, signerJabatan, tglCetak); err != nil {
		return nil, err
	}
	return doc.Output()
}

func kp4CetakUntukPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB, idPegawai uint) {
	pdfBytes, err := buildKp4PDF(db, idPegawai)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat formulir KP4: "+err.Error())
		return
	}
	var pegawai models.Pegawai
	db.Select("nip").First(&pegawai, idPegawai)
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("kp4_%s.pdf", namaOrDash(pegawai.NIP)))
}
