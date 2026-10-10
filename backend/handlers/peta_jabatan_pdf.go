package handlers

import (
	"fmt"
	neturl "net/url"

	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// peta_jabatan_pdf.go membangun dokumen cetak "Peta Jabatan Sekolah" untuk
// satu PengajuanKenaikanPangkat -- kop sekolah, garis komando sederhana
// (Kepala Sekolah paling atas), tabel B (Bezetting)/K (Kebutuhan)/+- SELURUH
// Jabatan di sekolah itu (sama angkanya dengan tabel menu, lihat
// hitungPetaJabatanSekolah), dan blok tanda tangan QR Kepala Sekolah &
// Kepala Dinas -- QR masing-masing HANYA tampil begitu tahap persetujuannya
// sendiri sudah dilewati (atasan untuk QR kiri, administrator untuk QR
// kanan), sama pola dengan dokumen Berita Acara/Surat Rekomendasi Sekolah
// yang sudah ada (tampilkanQR mengikuti status, bukan selalu tampil).

// buildKenaikanPangkatPDF menyusun ulang dokumen PETA JABATAN SEKOLAH untuk
// satu pengajuan kenaikan pangkat -- dipanggil ulang setiap kali statusnya
// berubah (setujuiKenaikanPangkatAtasan & setujuiKenaikanPangkatAdmin)
// supaya dokumen yang tersimpan selalu mencerminkan tahap persetujuan
// terakhir.
func buildKenaikanPangkatPDF(db *gorm.DB, item models.PengajuanKenaikanPangkat) ([]byte, error) {
	var pegawai models.Pegawai
	if err := db.Preload("Jabatan").Preload("UnitKerja").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
		First(&pegawai, item.IDPegawai).Error; err != nil {
		return nil, fmt.Errorf("data pegawai tidak ditemukan: %w", err)
	}
	var jabatanAsal, jabatanTujuan models.Jabatan
	db.First(&jabatanAsal, item.IDJabatanAsal)
	db.First(&jabatanTujuan, item.IDJabatanTujuan)

	var subAsal, subTujuan *models.SubJabatan
	if item.IDSubJabatanAsal != nil {
		var s models.SubJabatan
		if db.First(&s, *item.IDSubJabatanAsal).Error == nil {
			subAsal = &s
		}
	}
	if item.IDSubJabatanTujuan != nil {
		var s models.SubJabatan
		if db.First(&s, *item.IDSubJabatanTujuan).Error == nil {
			subTujuan = &s
		}
	}

	var atasan *models.Pegawai
	if item.IDAtasanApprove != nil {
		var a models.Pegawai
		if db.Preload("Jabatan").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").First(&a, *item.IDAtasanApprove).Error == nil {
			atasan = &a
		}
	}

	peta, err := hitungPetaJabatanSekolah(db, item.IDUnitKerja)
	if err != nil {
		return nil, err
	}

	pengaturan := pengaturanSuratOrDefault(db)
	dinasNama, dinasNip, dinasJabatan, _, _ := resolveSignerFullRekomendasi(db, pengaturan)

	doc := utils.NewPDFDoc()
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)

	marginX := 42.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX
	centerX := marginX + (rightX-marginX)/2

	unitKerjaNama := "-"
	var ukForKop *models.UnitKerja
	if pegawai.UnitKerja != nil {
		unitKerjaNama = pegawai.UnitKerja.Unit
		ukForKop = pegawai.UnitKerja
	}
	y := drawLetterheadUnitKerjaKustom(doc, p, marginX, rightX, unitKerjaNama, ukForKop) + 14

	p.SetFont(true, 13)
	const judul = "PETA JABATAN SEKOLAH"
	p.TextCentered(centerX, y, judul)
	judulW := utils.TextWidth(judul, 13)
	p.Line(centerX-judulW/2, y+3, centerX+judulW/2, y+3)
	y += 18

	nomor := "-"
	if item.NomorSurat != nil {
		nomor = *item.NomorSurat
	}
	p.SetFont(false, 9)
	p.TextCentered(centerX, y, "Nomor: "+nomor)
	y += 20

	jabatanAsalLabel := jabatanAsal.Jabatan
	if subAsal != nil {
		jabatanAsalLabel += " (" + subAsal.Nama + ")"
	}
	jabatanTujuanLabel := jabatanTujuan.Jabatan
	if subTujuan != nil {
		jabatanTujuanLabel += " (" + subTujuan.Nama + ")"
	}
	ringkasan := fmt.Sprintf(
		"Diterbitkan sehubungan dengan pengajuan Kenaikan Pangkat a.n. %s (NIP. %s), dari jabatan %s ke jabatan %s, pada %s.",
		pegawai.Nama, pegawai.NIP, jabatanAsalLabel, jabatanTujuanLabel, unitKerjaNama,
	)
	p.SetFont(false, 9)
	y += p.JustifiedText(marginX, y, rightX-marginX, 13, ringkasan)
	y += 14

	// --- garis komando sederhana: Kepala Sekolah paling atas ---
	kepsekNama := "-"
	if atasan != nil {
		kepsekNama = atasan.Nama
	}
	boxW, boxH := 220.0, 28.0
	boxX := centerX - boxW/2
	p.SetLineWidth(1)
	p.Rect(boxX, y, boxW, boxH)
	p.SetFont(true, 9)
	p.TextCentered(centerX, y+10, "KEPALA SEKOLAH")
	p.SetFont(false, 8.5)
	p.TextCentered(centerX, y+21, kepsekNama)
	chartTopY := y
	y += boxH

	jumlahJabatan := len(peta.Rows)
	if jumlahJabatan > 0 {
		lineMidY := y + 10
		p.Line(centerX, chartTopY+boxH, centerX, lineMidY)
		cols := jumlahJabatan
		if cols > 6 {
			cols = 6
		}
		spacing := (rightX - marginX) / float64(cols)
		childBoxW, childBoxH := spacing-10, 24.0
		p.Line(marginX+spacing/2, lineMidY, rightX-spacing/2, lineMidY)
		for i := 0; i < cols; i++ {
			cx := marginX + spacing/2 + float64(i)*spacing
			p.Line(cx, lineMidY, cx, lineMidY+10)
			cbx := cx - childBoxW/2
			p.Rect(cbx, lineMidY+10, childBoxW, childBoxH)
			label := peta.Rows[i].Jabatan
			p.SetFont(false, 7)
			for _, ln := range utils.WrapText(label, childBoxW-6, 7) {
				p.TextCentered(cx, lineMidY+20, ln)
				break // satu baris saja supaya kotak tetap ringkas, label lengkap tetap ada di tabel di bawah
			}
		}
		y = lineMidY + 10 + childBoxH + 16
	} else {
		y += 16
	}

	// --- tabel B/K/+- ---
	p.SetFont(true, 9)
	p.Text(marginX, y, "Jabatan")
	p.TextCentered(centerX+120, y, "B")
	p.TextCentered(centerX+160, y, "K")
	p.TextCentered(centerX+200, y, "+/-")
	y += 4
	p.Line(marginX, y, rightX, y)
	y += 12
	p.SetFont(false, 8.5)
	for _, row := range peta.Rows {
		p.SetFont(true, 8.5)
		p.Text(marginX, y, row.Jabatan)
		p.TextCentered(centerX+120, y, fmt.Sprintf("%d", row.B))
		p.TextCentered(centerX+160, y, fmt.Sprintf("%d", row.K))
		p.TextCentered(centerX+200, y, signedStr(row.Selisih))
		y += 13
		p.SetFont(false, 8)
		for _, sub := range row.SubJabatan {
			p.Text(marginX+14, y, "- "+sub.Nama)
			p.TextCentered(centerX+120, y, fmt.Sprintf("%d", sub.B))
			p.TextCentered(centerX+160, y, fmt.Sprintf("%d", sub.K))
			p.TextCentered(centerX+200, y, signedStr(sub.Selisih))
			y += 12
		}
		if y > utils.PageHeightA4-220 {
			break // batas aman supaya tidak tumpang tindih blok tanda tangan di bawah (lihat komentar di atas fungsi)
		}
	}
	y += 10

	// --- blok tanda tangan QR ---
	signY := utils.PageHeightA4 - 150
	if y > signY-20 {
		signY = y + 20
	}
	colW := (rightX - marginX - 20) / 2
	leftCenterX := marginX + colW/2
	rightCenterX := rightX - colW/2
	qrSide := 54.0

	p.SetFont(false, 9)
	p.TextCentered(leftCenterX, signY, "Mengetahui,")
	p.TextCentered(leftCenterX, signY+13, "Kepala Sekolah "+unitKerjaNama)
	p.TextCentered(rightCenterX, signY, "Kolonodale, "+formatDateID(absensiNow()))
	p.TextCentered(rightCenterX, signY+13, "Kepala Dinas Pendidikan dan Kebudayaan")

	qrY := signY + 20
	if item.TglAtasanApprove != nil && atasan != nil {
		atasanJabatan := "Kepala Sekolah"
		if atasan.Jabatan != nil {
			atasanJabatan = atasan.Jabatan.Jabatan
		}
		query := fmt.Sprintf("Kepala Sekolah %s NIP %s Jabatan %s menyetujui Peta Jabatan Nomor %s tanggal %s",
			namaOrDash(atasan.Nama), namaOrDash(atasan.NIP), namaOrDash(atasanJabatan), nomor, formatDateID(*item.TglAtasanApprove))
		if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(query), qrcode.Medium, 180); err == nil {
			if err := doc.RegisterImage("ttd_qr_peta_jabatan_atasan", qrPng); err == nil {
				p.Image("ttd_qr_peta_jabatan_atasan", leftCenterX-qrSide/2, qrY, qrSide, qrSide)
			}
		}
		p.SetFont(true, 9)
		p.TextCentered(leftCenterX, qrY+qrSide+14, atasan.Nama)
		p.SetFont(false, 8.5)
		p.TextCentered(leftCenterX, qrY+qrSide+26, "NIP. "+namaOrDash(atasan.NIP))
	} else {
		p.SetFont(false, 8.5)
		p.TextCentered(leftCenterX, qrY+qrSide/2, "(belum ditandatangani)")
	}

	if item.TglAdminApprove != nil {
		query := fmt.Sprintf("Kepala Dinas %s NIP %s Jabatan %s menyetujui Peta Jabatan Nomor %s tanggal %s",
			namaOrDash(dinasNama), namaOrDash(dinasNip), namaOrDash(dinasJabatan), nomor, formatDateID(*item.TglAdminApprove))
		if qrPng, err := qrcode.Encode("https://www.google.com/search?q="+neturl.QueryEscape(query), qrcode.Medium, 180); err == nil {
			if err := doc.RegisterImage("ttd_qr_peta_jabatan_admin", qrPng); err == nil {
				p.Image("ttd_qr_peta_jabatan_admin", rightCenterX-qrSide/2, qrY, qrSide, qrSide)
			}
		}
		p.SetFont(true, 9)
		p.TextCentered(rightCenterX, qrY+qrSide+14, namaOrDash(dinasNama))
		p.SetFont(false, 8.5)
		p.TextCentered(rightCenterX, qrY+qrSide+26, "NIP. "+namaOrDash(dinasNip))
	} else {
		p.SetFont(false, 8.5)
		p.TextCentered(rightCenterX, qrY+qrSide/2, "(belum ditandatangani)")
	}

	return doc.Output()
}

// signedStr memformat selisih B-K dengan tanda "+" eksplisit untuk nilai
// positif (mis. "+4", "0", "-1") -- sama konvensi dengan kolom "+/-" pada
// tabel menu (lihat frontend PetaJabatanView.vue).
func signedStr(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf("%d", n)
}
