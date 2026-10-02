package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"cuti-app/assets"
	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// surat_rekomendasi.go menangani menu "Surat Rekomendasi" (khusus
// administrator/admin) & "Arsip Surat" (pegawai/atasan) -- surat
// rekomendasi perpanjangan kontrak untuk pegawai berstatus PPPK/PPPK Paruh
// Waktu, dikirim satu-satu ATAU sekaligus (kolektif, difilter dari Status
// Kepegawaian & tahun TMT) lewat POST /api/surat-rekomendasi. Begitu
// dikirim, surat langsung muncul di menu Arsip Surat akun pegawai
// penerimanya sebagai PDF yang bisa dilihat/diunduh kapan saja -- TIDAK
// ada proses approval terpisah, beda dari Pengajuan Cuti/Pensiun/dll.
//
// Nomor surat: kode klasifikasi TETAP "800.1.11" (models.
// KodeKlasifikasiSuratRekomendasi) -- nomor urutnya diisi manual oleh
// admin HANYA untuk surat pertama di suatu tahun (lihat
// nextNomorSuratRekomendasi), setelah itu otomatis melanjutkan (+1) baik
// dibuat satu-satu maupun sekaligus dalam satu kali kirim.
//
// QR tanda tangan otomatis & tata letak kop/blok tanda tangan meniru pola
// yang sudah ada di formulir.go (buildSuratRekomendasiPage untuk Surat
// Rekomendasi Izin Cuti) -- SENGAJA tidak dipakai bersama (bukan
// generalisasi dari fungsi itu) supaya perubahan pada fitur cuti yang
// sudah lama stabil tidak ikut berisiko saat fitur baru ini dikembangkan.

// ============================================================
// nomor surat & builder QR
// ============================================================

// nomorSuratRekomendasiLengkap merangkai nomor surat lengkap format
// "800.1.11/{urut}/Disdikbud /{bulan romawi}/ {tahun}" -- kode klasifikasi
// 800.1.11 TETAP untuk semua Surat Rekomendasi (beda dari Surat
// Rekomendasi Izin Cuti yang kodenya berbeda-beda per jenis cuti, lihat
// suratNomorKode di formulir.go). Bulan romawi diambil dari BULAN TANGGAL
// SURAT (bukan tanggal pembuatan baris di database).
func nomorSuratRekomendasiLengkap(nomorUrut, tahun int, tanggalSurat time.Time) string {
	return fmt.Sprintf("%s/%d/Disdikbud /%s/ %d", models.KodeKlasifikasiSuratRekomendasi, nomorUrut, romanMonth(tanggalSurat.Month()), tahun)
}

// nomorSuratRekomendasiSekolahLengkap merangkai nomor surat "Lampiran 3"
// (khusus pegawai bertempat tugas Sekolah/Puskesmas) -- BEDA dari
// nomorSuratRekomendasiLengkap di atas dalam dua hal: (1) bagian nomor urut
// SENGAJA dikosongkan ("......", bukan nomor urut gabungan Dinas) karena
// penomoran surat keluar sekolah itu urusan administrasi sekolah
// masing-masing, dicetak lalu diisi tangan; (2) "Disdikbud" diganti dengan
// singkatan Unit Kerja (sekolah/puskesmas) pegawai sendiri, lihat field baru
// models.UnitKerja.Singkatan (mis. "SDN-LMR") -- diisi manual admin di menu
// Unit Kerja, tampil "-" kalau belum diisi.
func nomorSuratRekomendasiSekolahLengkap(tahun int, tanggalSurat time.Time, singkatanUnitKerja string) string {
	return fmt.Sprintf("%s/....../%s/%s/ %d", models.KodeKlasifikasiSuratRekomendasi, singkatanUnitKerja, romanMonth(tanggalSurat.Month()), tahun)
}

// buildSignatureQRRekomendasi meniru gaya buildSignatureQR di formulir.go
// (query Google Search berlabel jelas) tapi untuk konteks Surat
// Rekomendasi perpanjangan kontrak PPPK, bukan izin cuti.
func buildSignatureQRRekomendasi(item models.SuratRekomendasi, pegawai models.Pegawai, signerNama, signerJabatan, nomorLengkap string) ([]byte, error) {
	query := fmt.Sprintf(
		"Nama Pejabat: %s Jabatan: %s Merekomendasikan perpanjangan kontrak atas nama %s NIP %s Nomor Surat %s Ditandatangani pada tanggal %s di Kolonodale",
		namaOrDash(signerNama), namaOrDash(signerJabatan), namaOrDash(pegawai.Nama), namaOrDash(pegawai.NIP), nomorLengkap, formatDateID(item.TanggalSurat),
	)
	googleURL := "https://www.google.com/search?q=" + neturl.QueryEscape(query)
	return qrcode.Encode(googleURL, qrcode.Medium, 240)
}

// resolveSignerFullRekomendasi mencari data LENGKAP pegawai penandatangan
// (Plt. Kepala Dinas, dikenali dari NIP di PengaturanSurat) -- BEDA dari
// resolveSignerInfo di formulir.go yang cuma mengembalikan jabatan, karena
// Surat Rekomendasi ini juga perlu menampilkan Pangkat/Gol. Ruang & Unit
// Kerja penandatangan pada blok "Yang bertanda tangan dibawah ini".
func resolveSignerFullRekomendasi(db *gorm.DB, pengaturan models.PengaturanSurat) (nama, nip, jabatan, pangkatGol, unitKerja string) {
	nama = pengaturan.NamaKepalaDinas
	nip = pengaturan.NipKepalaDinas
	jabatan = "Kepala Dinas"
	pangkatGol = "-"
	unitKerja = "-"
	if strings.TrimSpace(nip) == "" {
		return
	}
	var pegawai models.Pegawai
	if err := db.Preload("Jabatan").Preload("UnitKerja").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
		Where("nip = ?", nip).First(&pegawai).Error; err == nil {
		if pegawai.Jabatan != nil {
			jabatan = pegawai.Jabatan.Jabatan
		}
		if pegawai.UnitKerja != nil {
			unitKerja = pegawai.UnitKerja.Unit
		}
		if pegawai.PangkatGol != nil {
			pk, gol := "-", "-"
			if pegawai.PangkatGol.Pangkat != nil {
				pk = pegawai.PangkatGol.Pangkat.Pangkat
			}
			if pegawai.PangkatGol.Gol != nil {
				gol = pegawai.PangkatGol.Gol.Gol
			}
			pangkatGol = pk + " / " + gol
		}
	}
	return
}

// buildSuratRekomendasiPppk menggambar PDF Surat Rekomendasi perpanjangan
// kontrak PPPK/PPPK Paruh Waktu -- item.Pegawai WAJIB sudah dipreload
// (Jabatan, UnitKerja, PangkatGol.Pangkat, PangkatGol.Gol, Status) oleh
// pemanggil.
func buildSuratRekomendasiPppk(item models.SuratRekomendasi, signerNama, signerNip, signerJabatan, signerPangkatGol, signerUnitKerja string) ([]byte, error) {
	doc := utils.NewPDFDoc()
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		return nil, err
	}
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)

	marginX := 42.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX
	centerX := marginX + (rightX-marginX)/2

	drawLetterhead(p, marginX, rightX)

	pegawai := models.Pegawai{}
	if item.Pegawai != nil {
		pegawai = *item.Pegawai
	}

	const lineH = 15.0
	y := 118.0

	p.SetFont(true, 13)
	const judulSurat = "SURAT REKOMENDASI"
	p.TextCentered(centerX, y, judulSurat)
	titleW := utils.TextWidth(judulSurat, 13)
	p.Line(centerX-titleW/2, y+3, centerX+titleW/2, y+3)
	y += lineH

	nomorLengkap := nomorSuratRekomendasiLengkap(item.NomorUrut, item.Tahun, item.TanggalSurat)
	p.SetFont(false, 12)
	p.TextCentered(centerX, y, "Nomor : "+nomorLengkap)
	y += lineH * 1.8

	const labelW = 132.0
	drawField := func(label, val string) {
		p.Text(marginX, y, label)
		p.Text(marginX+labelW, y, ": "+namaOrDash(val))
		y += lineH
	}

	p.Text(marginX, y, "Yang bertanda tangan dibawah ini :")
	y += lineH
	drawField("Nama", strings.ToUpper(namaOrDash(signerNama)))
	drawField("NIP", signerNip)
	drawField("Pangkat/Gol. Ruang", signerPangkatGol)
	drawField("Jabatan", signerJabatan)
	drawField("Unit Kerja", signerUnitKerja)
	y += lineH * 0.6

	statusNama := "-"
	if pegawai.Status != nil {
		statusNama = pegawai.Status.Status
	}
	jabatanNama := "-"
	if pegawai.Jabatan != nil {
		jabatanNama = pegawai.Jabatan.Jabatan
	}
	unitKerjaNama := "-"
	if pegawai.UnitKerja != nil {
		unitKerjaNama = pegawai.UnitKerja.Unit
	}
	golNama := "-"
	if pegawai.PangkatGol != nil && pegawai.PangkatGol.Gol != nil {
		golNama = pegawai.PangkatGol.Gol.Gol
	}

	p.Text(marginX, y, "Dengan ini menyatakan bahwa saudara :")
	y += lineH
	drawField("Nama", strings.ToUpper(namaOrDash(pegawai.Nama)))
	drawField("NI PPPK", pegawai.NIP)
	drawField("Golongan", golNama)
	drawField("Jabatan", jabatanNama)
	drawField("Status Kepegawaian", "ASN "+statusNama)
	drawField("Unit Kerja", unitKerjaNama)
	y += lineH

	// Kalimat poin 1 menyesuaikan otomatis untuk PPPK Paruh Waktu (beda
	// istilah resmi dari PPPK biasa) -- lihat models.database seed.go
	// untuk daftar Status yang tersedia ("PNS"/"PPPK"/"PPPK Paruh Waktu").
	jenisPppk := "Pegawai Pemerintah dengan Perjanjian Kerja (PPPK)"
	if strings.Contains(strings.ToLower(statusNama), "paruh waktu") {
		jenisPppk = "Pegawai Pemerintah dengan Perjanjian Kerja Paruh Waktu (PPPK Paruh Waktu)"
	}
	tmtText := "-"
	if pegawai.TMT != nil {
		tmtText = formatDateID(*pegawai.TMT)
	}

	poin1 := fmt.Sprintf(
		"1. Benar merupakan %s pada Unit Kerja %s terhitung mulai tanggal %s sampai dengan saat ini melaksanakan tugas secara nyata dan sah secara terus-menerus;",
		jenisPppk, unitKerjaNama, tmtText,
	)
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, poin1) + lineH*0.4

	poin2 := "2. Berdasarkan point 1 (satu) diatas, maka kami merekomendasikan yang bersangkutan dapat dipertimbangkan untuk proses perpanjangan perjanjian kerja (kontrak)."
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, poin2) + lineH*0.4

	poin3 := "3. Apabila dikemudian hari, terdapat hal-hal yang tidak sesuai, maka kami siap mempertanggungjawabkan secara hukum tanpa melibatkan siapapun."
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, poin3) + lineH*1.6

	penutup := "Demikian rekomendasi ini kami buat untuk dipergunakan sebagaimana mestinya."
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, penutup) + lineH*1.6

	// Blok tanda tangan -- tata letak & lebar kolom meniru
	// buildSuratRekomendasiPage di formulir.go (lihat komentar sigColW di
	// sana perihal riwayat nama panjang "BERNOULLI TANARI, S.Pd.,M.Pd" yang
	// pernah terpotong).
	sigX := pageW - 270
	sigColW := rightX - sigX

	signerNamaDisp := truncateToWidth(strings.ToUpper(namaOrDash(signerNama)), sigColW*boldWidthSafety, 12)
	nameW := utils.TextWidth(signerNamaDisp, 12)
	jabatanLines, jabatanSize := wrapJabatan(namaOrDash(signerJabatan), sigColW, 2, []float64{12, 11, 10.5, 10, 9.5, 9, 8.5, 8})
	jabatanLineH := jabatanSize + 2

	p.SetFont(false, 12)
	p.Text(sigX, y, "Kolonodale, "+formatDateID(item.TanggalSurat)+".")
	y += lineH
	p.SetFont(false, jabatanSize)
	for _, jl := range jabatanLines {
		p.Text(sigX, y, jl)
		y += jabatanLineH
	}
	p.SetFont(false, 12)

	const qrSide = 150.0
	qrCenterX := sigX + nameW/2
	if qrCenterX-qrSide/2 < sigX {
		qrCenterX = sigX + qrSide/2
	}
	if qrCenterX+qrSide/2 > rightX {
		qrCenterX = rightX - qrSide/2
	}
	if qrPng, err := buildSignatureQRRekomendasi(item, pegawai, signerNama, signerJabatan, nomorLengkap); err == nil {
		if err := doc.RegisterImage("ttd_qr_surat_rekomendasi", qrPng); err == nil {
			p.Image("ttd_qr_surat_rekomendasi", qrCenterX-qrSide/2, y+4, qrSide, qrSide)
		}
	}
	y += qrSide + 18

	p.SetFont(true, 12)
	p.Text(sigX, y, signerNamaDisp)
	p.Line(sigX, y+3, sigX+nameW, y+3)
	y += 16
	p.SetFont(false, 12)
	p.Text(sigX, y, "NIP: "+namaOrDash(signerNip)+".")

	return doc.Output()
}

// ============================================================
// Lampiran 3: Surat Rekomendasi khusus pegawai bertempat tugas Sekolah/
// Puskesmas (kop surat unit kerja sendiri, ditandatangani atasan langsung)
// ============================================================

// drawLetterheadUnitKerjaKustom menggambar kop surat "Lampiran 3" --
// dipakai KHUSUS untuk Lampiran 3 (lihat buildSuratRekomendasiSekolah),
// berbeda dari drawLetterhead di formulir.go yang kopnya tetap/fixed Dinas
// (dipakai Lampiran 2 untuk pegawai Dinas/Kantor). Logo Kabupaten
// (assets.LogoPNG) SELALU digambar di kiri.
//
// Kalau sekolah (atasan langsungnya, lewat menu "Kop Surat Sekolah" --
// lihat handlers/kop_surat.go) SUDAH mengisi UnitKerja.NamaSekolahKop,
// dipakai kop KUSTOM sekolah itu: judul (boleh multi-baris) + opsional
// alamat kop kiri/kanan (dua kolom kalau keduanya diisi) + opsional logo
// Tut Wuri Handayani di kanan (UnitKerja.TampilkanLogoTutwuri, lihat
// assets.TutWuriPNG) + perataan sesuai UnitKerja.PerataanKop. Kalau BELUM
// pernah diisi (uk nil atau NamaSekolahKop kosong), jatuh kembali ke kop
// teks polos bawaan (nama pemerintah + nama unit kerja) seperti
// sebelumnya, supaya sekolah yang belum mengatur kopnya sendiri tetap
// mendapat surat yang wajar.
func drawLetterheadUnitKerjaKustom(doc *utils.PDFDoc, p *utils.PDFPage, marginX, rightX float64, unitKerjaNama string, uk *models.UnitKerja) float64 {
	const logoW, logoH = 44.0, 58.0

	hasLogoKiri := false
	if err := doc.RegisterImage("logo_kop_sekolah_kiri", assets.LogoPNG); err == nil {
		p.Image("logo_kop_sekolah_kiri", marginX, 20, logoW, logoH)
		hasLogoKiri = true
	}
	// Logo Tut Wuri Handayani OPSIONAL di kanan -- lihat komentar
	// assets.TutWuriPNG: berkasnya belum ditambahkan, jadi untuk sementara
	// len(...) selalu 0 & logo ini belum pernah benar-benar tergambar
	// sampai berkasnya ditambahkan ke package assets.
	hasLogoKanan := false
	if uk != nil && uk.TampilkanLogoTutwuri && len(assets.TutWuriPNG) > 0 {
		if err := doc.RegisterImage("logo_kop_sekolah_kanan", assets.TutWuriPNG); err == nil {
			p.Image("logo_kop_sekolah_kanan", rightX-logoW, 20, logoW, logoH)
			hasLogoKanan = true
		}
	}

	textLeft, textRight := marginX, rightX
	if hasLogoKiri {
		textLeft = marginX + logoW + 10
	}
	if hasLogoKanan {
		textRight = rightX - logoW - 10
	}
	textCenterX := textLeft + (textRight-textLeft)/2

	perataan := models.PerataanKopTengah
	if uk != nil && uk.PerataanKop != nil && strings.TrimSpace(*uk.PerataanKop) != "" {
		perataan = strings.ToLower(strings.TrimSpace(*uk.PerataanKop))
	}
	drawAligned := func(y float64, bold bool, size float64, s string) {
		p.SetFont(bold, size)
		switch perataan {
		case models.PerataanKopKiri:
			p.Text(textLeft, y, s)
		case models.PerataanKopKanan:
			p.TextRight(textRight, y, s)
		default:
			p.TextCentered(textCenterX, y, s)
		}
	}

	customKop := uk != nil && uk.NamaSekolahKop != nil && strings.TrimSpace(*uk.NamaSekolahKop) != ""

	y := 26.0
	const titleLineH = 16.0
	const addrLineH = 14.0

	var titleLines []string
	if customKop {
		titleLines = strings.Split(strings.ReplaceAll(*uk.NamaSekolahKop, "\r\n", "\n"), "\n")
	} else {
		titleLines = []string{"PEMERINTAH KABUPATEN MOROWALI UTARA", strings.ToUpper(namaOrDash(unitKerjaNama)), "Kabupaten Morowali Utara"}
	}
	for i, line := range titleLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sz, bold := 13.0, true
		if !customKop && i == len(titleLines)-1 {
			// baris terakhir mode bawaan ("Kabupaten Morowali Utara") tetap
			// kecil & reguler seperti sebelumnya.
			sz, bold = 10, false
		}
		drawAligned(y, bold, sz, line)
		y += titleLineH
	}

	// Alamat kop kiri/kanan -- opsional, hanya terisi kalau sekolah sudah
	// mengaturnya lewat menu Kop Surat Sekolah.
	var kiriLines, kananLines []string
	if uk != nil && uk.AlamatKopKiri != nil && strings.TrimSpace(*uk.AlamatKopKiri) != "" {
		kiriLines = strings.Split(strings.ReplaceAll(*uk.AlamatKopKiri, "\r\n", "\n"), "\n")
	}
	if uk != nil && uk.AlamatKopKanan != nil && strings.TrimSpace(*uk.AlamatKopKanan) != "" {
		kananLines = strings.Split(strings.ReplaceAll(*uk.AlamatKopKanan, "\r\n", "\n"), "\n")
	}
	if len(kiriLines) > 0 && len(kananLines) > 0 {
		// Dua kolom (kiri rata kiri, kanan rata kanan) -- SENGAJA
		// mengabaikan PerataanKop di sini supaya dua kolomnya tetap jelas.
		maxLines := len(kiriLines)
		if len(kananLines) > maxLines {
			maxLines = len(kananLines)
		}
		p.SetFont(false, 10)
		for i := 0; i < maxLines; i++ {
			if i < len(kiriLines) {
				if l := strings.TrimSpace(kiriLines[i]); l != "" {
					p.Text(textLeft, y, l)
				}
			}
			if i < len(kananLines) {
				if l := strings.TrimSpace(kananLines[i]); l != "" {
					p.TextRight(textRight, y, l)
				}
			}
			y += addrLineH
		}
	} else if len(kiriLines) > 0 {
		for _, line := range kiriLines {
			if line = strings.TrimSpace(line); line != "" {
				drawAligned(y, false, 10, line)
				y += addrLineH
			}
		}
	}

	y += 8
	p.SetLineWidth(1.4)
	p.Line(marginX, y, rightX, y)
	p.SetLineWidth(0.75)
	return y
}

// buildSuratRekomendasiSekolah menggambar PDF "Lampiran 3" -- varian Surat
// Rekomendasi perpanjangan kontrak KHUSUS pegawai dengan tempat tugas
// Sekolah/Puskesmas (lihat isSekolahPegawai di pengajuan_cuti.go). Isi &
// susunan paragraf SAMA seperti buildSuratRekomendasiPppk ("Lampiran 2",
// khusus Dinas/Kantor), tapi beda pada 3 hal sesuai permintaan pengguna:
//  1. Kop surat memakai nama Unit Kerja (sekolah/puskesmas) milik pegawai
//     sendiri (drawLetterheadUnitKerja), BUKAN kop Dinas tetap.
//  2. Penandatangan UTAMA ("Yang bertanda tangan dibawah ini", blok tanda
//     tangan kanan + QR) diambil dari data ATASAN LANGSUNG pegawai
//     (item.Pegawai.Atasan == Kepala Sekolah/Kepala Puskesmas) -- BUKAN
//     Kepala Dinas. Jika data Atasan pegawai belum diisi admin, kolom ini
//     tampil "-" (surat tetap bisa dibuat, tapi sebaiknya admin segera
//     melengkapi data Atasan pegawai tersebut).
//  3. Ditambahkan blok tanda tangan KEDUA di kiri "Mengetahui : Kepala
//     Dinas" (tetap pakai resolveSignerFullRekomendasi/PengaturanSurat,
//     SAMA seperti penandatangan Lampiran 2) sesuai contoh Lampiran 3.
//
// Semua paragraf memakai JustifiedText (rata kiri-kanan) sesuai permintaan.
func buildSuratRekomendasiSekolah(item models.SuratRekomendasi, dinasNama, dinasNip, dinasJabatan string) ([]byte, error) {
	doc := utils.NewPDFDoc()
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)

	marginX := 42.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX
	centerX := marginX + (rightX-marginX)/2

	pegawai := models.Pegawai{}
	if item.Pegawai != nil {
		pegawai = *item.Pegawai
	}
	unitKerjaNama := "-"
	if pegawai.UnitKerja != nil {
		unitKerjaNama = pegawai.UnitKerja.Unit
	}

	var unitKerjaForKop *models.UnitKerja
	if pegawai.UnitKerja != nil {
		unitKerjaForKop = pegawai.UnitKerja
	}
	kopBawahY := drawLetterheadUnitKerjaKustom(doc, p, marginX, rightX, unitKerjaNama, unitKerjaForKop)

	// signerNama..signerUnitKerja: identitas penandatangan utama (Kepala
	// Sekolah/Kepala Puskesmas), diambil dari atasan langsung pegawai.
	signerNama, signerNip, signerJabatan, signerPangkatGol, signerUnitKerja := "-", "-", "Kepala Sekolah", "-", unitKerjaNama
	var atasan *models.Pegawai
	if pegawai.Atasan != nil {
		atasan = pegawai.Atasan
		signerNama = atasan.Nama
		signerNip = atasan.NIP
		if atasan.Jabatan != nil {
			signerJabatan = atasan.Jabatan.Jabatan
		}
		if atasan.UnitKerja != nil {
			signerUnitKerja = atasan.UnitKerja.Unit
		}
		if atasan.PangkatGol != nil {
			pk, gol := "-", "-"
			if atasan.PangkatGol.Pangkat != nil {
				pk = atasan.PangkatGol.Pangkat.Pangkat
			}
			if atasan.PangkatGol.Gol != nil {
				gol = atasan.PangkatGol.Gol.Gol
			}
			signerPangkatGol = pk + " / " + gol
		}
	}

	const lineH = 15.0
	y := kopBawahY + 16

	p.SetFont(true, 13)
	const judulSurat = "SURAT REKOMENDASI"
	p.TextCentered(centerX, y, judulSurat)
	titleW := utils.TextWidth(judulSurat, 13)
	p.Line(centerX-titleW/2, y+3, centerX+titleW/2, y+3)
	y += lineH

	// nomorLengkap: nomor urut SENGAJA dikosongkan ("......"), diganti
	// singkatan Unit Kerja (sekolah/puskesmas) pegawai sendiri -- lihat
	// nomorSuratRekomendasiSekolahLengkap & models.UnitKerja.Singkatan
	// (diisi manual admin di menu Unit Kerja, "-" kalau belum diisi).
	singkatanUnitKerja := "-"
	if pegawai.UnitKerja != nil && pegawai.UnitKerja.Singkatan != nil && strings.TrimSpace(*pegawai.UnitKerja.Singkatan) != "" {
		singkatanUnitKerja = *pegawai.UnitKerja.Singkatan
	}
	nomorLengkap := nomorSuratRekomendasiSekolahLengkap(item.Tahun, item.TanggalSurat, singkatanUnitKerja)
	p.SetFont(false, 12)
	p.TextCentered(centerX, y, "Nomor : "+nomorLengkap)
	y += lineH * 1.8

	const labelW = 132.0
	drawField := func(label, val string) {
		p.Text(marginX, y, label)
		p.Text(marginX+labelW, y, ": "+namaOrDash(val))
		y += lineH
	}

	p.Text(marginX, y, "Yang bertanda tangan dibawah ini :")
	y += lineH
	drawField("Nama", strings.ToUpper(namaOrDash(signerNama)))
	drawField("NIP", signerNip)
	drawField("Pangkat/Gol. Ruang", signerPangkatGol)
	drawField("Jabatan", signerJabatan)
	drawField("Unit Kerja", signerUnitKerja)
	y += lineH * 0.6

	statusNama := "-"
	if pegawai.Status != nil {
		statusNama = pegawai.Status.Status
	}
	jabatanNama := "-"
	if pegawai.Jabatan != nil {
		jabatanNama = pegawai.Jabatan.Jabatan
	}
	golNama := "-"
	if pegawai.PangkatGol != nil && pegawai.PangkatGol.Gol != nil {
		golNama = pegawai.PangkatGol.Gol.Gol
	}

	p.Text(marginX, y, "Dengan ini menyatakan bahwa saudara :")
	y += lineH
	drawField("Nama", strings.ToUpper(namaOrDash(pegawai.Nama)))
	drawField("NI PPPK", pegawai.NIP)
	drawField("Golongan", golNama)
	drawField("Jabatan", jabatanNama)
	drawField("Status Kepegawaian", "ASN "+statusNama)
	drawField("Unit Kerja", unitKerjaNama)
	y += lineH

	jenisPppk := "Pegawai Pemerintah dengan Perjanjian Kerja (PPPK)"
	if strings.Contains(strings.ToLower(statusNama), "paruh waktu") {
		jenisPppk = "Pegawai Pemerintah dengan Perjanjian Kerja Paruh Waktu (PPPK Paruh Waktu)"
	}
	tmtText := "-"
	if pegawai.TMT != nil {
		tmtText = formatDateID(*pegawai.TMT)
	}

	poin1 := fmt.Sprintf(
		"1. Benar merupakan %s pada Unit Kerja %s terhitung mulai tanggal %s sampai dengan saat ini melaksanakan tugas secara nyata dan sah secara terus-menerus;",
		jenisPppk, unitKerjaNama, tmtText,
	)
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, poin1) + lineH*0.4

	poin2 := "2. Berdasarkan point 1 (satu) diatas, maka kami merekomendasikan yang bersangkutan dapat dipertimbangkan untuk proses perpanjangan perjanjian kerja (kontrak)."
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, poin2) + lineH*0.4

	poin3 := "3. Apabila dikemudian hari, terdapat hal-hal yang tidak sesuai, maka kami siap mempertanggungjawabkan secara hukum tanpa melibatkan siapapun."
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, poin3) + lineH*1.4

	penutup := "Demikian rekomendasi ini kami buat untuk dipergunakan sebagaimana mestinya."
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, penutup) + lineH*1.8

	// Blok tanda tangan: KIRI "Mengetahui" Kepala Dinas (saksi, tanpa QR),
	// KANAN penandatangan utama (Kepala Sekolah/Kepala Puskesmas = atasan
	// langsung pegawai, dengan QR tanda tangan otomatis) -- sesuai tata
	// letak dua kolom pada contoh Lampiran 3.
	sigYStart := y
	halfW := (rightX - marginX) / 2

	leftX := marginX
	yL := sigYStart
	p.SetFont(false, 12)
	p.Text(leftX, yL, "Mengetahui :")
	yL += lineH
	dinasJabatanLines, dinasJabatanSize := wrapJabatan(namaOrDash(dinasJabatan), halfW-20, 2, []float64{12, 11, 10.5, 10, 9.5, 9, 8.5, 8})
	for _, jl := range dinasJabatanLines {
		p.SetFont(false, dinasJabatanSize)
		p.Text(leftX, yL, jl)
		yL += dinasJabatanSize + 2
	}
	yL += lineH * 3.4
	dinasNamaDisp := truncateToWidth(strings.ToUpper(namaOrDash(dinasNama)), (halfW-20)*boldWidthSafety, 12)
	p.SetFont(true, 12)
	p.Text(leftX, yL, dinasNamaDisp)
	p.Line(leftX, yL+3, leftX+utils.TextWidth(dinasNamaDisp, 12), yL+3)
	yL += 16
	p.SetFont(false, 12)
	p.Text(leftX, yL, "NIP: "+namaOrDash(dinasNip)+".")

	sigX := centerX + 10
	sigColW := rightX - sigX
	yR := sigYStart
	p.SetFont(false, 12)
	p.Text(sigX, yR, "Kolonodale, "+formatDateID(item.TanggalSurat)+".")
	yR += lineH
	signerJabatanLines, signerJabatanSize := wrapJabatan(namaOrDash(signerJabatan), sigColW, 2, []float64{12, 11, 10.5, 10, 9.5, 9, 8.5, 8})
	jabatanLineH := signerJabatanSize + 2
	for _, jl := range signerJabatanLines {
		p.SetFont(false, signerJabatanSize)
		p.Text(sigX, yR, jl)
		yR += jabatanLineH
	}
	p.SetFont(false, 12)

	signerNamaDisp := truncateToWidth(strings.ToUpper(namaOrDash(signerNama)), sigColW*boldWidthSafety, 12)
	nameW := utils.TextWidth(signerNamaDisp, 12)

	const qrSide = 110.0
	qrCenterX := sigX + nameW/2
	if qrCenterX-qrSide/2 < sigX {
		qrCenterX = sigX + qrSide/2
	}
	if qrCenterX+qrSide/2 > rightX {
		qrCenterX = rightX - qrSide/2
	}
	// QR tanda tangan HANYA ditampilkan kalau atasan langsung (Kepala
	// Sekolah/Kepala Puskesmas) SUDAH menyetujui lewat menu Arsip Surat di
	// akunnya (lihat approveSuratRekomendasi) -- sebelum disetujui, surat
	// ini TETAP bisa dilihat/diunduh, hanya saja ruang QR-nya dikosongkan
	// (Nama & NIP atasan tetap tercetak seperti biasa di bawahnya).
	if atasan != nil && strings.TrimSpace(signerNip) != "" && item.StatusApproval == models.StatusDisetuju {
		if qrPng, err := buildSignatureQRRekomendasi(item, pegawai, signerNama, signerJabatan, nomorLengkap); err == nil {
			if err := doc.RegisterImage("ttd_qr_surat_rekomendasi_sekolah", qrPng); err == nil {
				p.Image("ttd_qr_surat_rekomendasi_sekolah", qrCenterX-qrSide/2, yR+4, qrSide, qrSide)
			}
		}
	}
	yR += qrSide + 14

	p.SetFont(true, 12)
	p.Text(sigX, yR, signerNamaDisp)
	p.Line(sigX, yR+3, sigX+nameW, yR+3)
	yR += 16
	p.SetFont(false, 12)
	p.Text(sigX, yR, "NIP: "+namaOrDash(signerNip)+".")

	return doc.Output()
}

// ============================================================
// Lampiran 1: surat permohonan perpanjangan kontrak (diajukan PEGAWAI
// SENDIRI ke Bupati)
// ============================================================

// buildPermohonanPerpanjanganKontrak menggambar PDF "Lampiran 1" -- surat
// permohonan perpanjangan kontrak yang ditulis & ditandatangani PEGAWAI
// SENDIRI (ditujukan ke Bupati Morowali Utara), BUKAN surat resmi dari
// Kepala Dinas. SENGAJA beda total dari buildSuratRekomendasiPppk di
// atas: TIDAK memakai kop/logo surat, TIDAK ada nomor surat (kode
// klasifikasi 800.1.11 TIDAK berlaku untuk surat ini), dan TIDAK ada QR
// tanda tangan otomatis -- karena memang harus ditandatangani tangan oleh
// pegawai sendiri sebagai pemohon.
//
// Dibuat dari baris models.SuratRekomendasi yang SAMA (dipakai lagi
// IDPegawai & Tahun-nya) supaya pegawai bisa mengunduh KEDUA berkas
// sekaligus dari satu kartu Arsip Surat yang sama: (1) Surat Rekomendasi
// dari Kepala Dinas (pdfSuratRekomendasi), (2) surat permohonan ini
// (pdfPermohonanSuratRekomendasi) yang tinggal dicetak, ditandatangani
// sendiri, lalu dilampirkan bersama SK/SKP/Rekomendasi sesuai isi surat
// ini sendiri. Periode SKP & tanggal akhir kontrak pada isi surat
// diturunkan dari Tahun yang sama dengan surat rekomendasinya (mis. Tahun
// 2026 -> "kontrak berakhir 30 September 2026" & "SKP Oktober-Desember
// 2025 dan Januari-Juni 2026").
func buildPermohonanPerpanjanganKontrak(item models.SuratRekomendasi) ([]byte, error) {
	doc := utils.NewPDFDoc()
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)

	marginX := 56.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX

	pegawai := models.Pegawai{}
	if item.Pegawai != nil {
		pegawai = *item.Pegawai
	}
	jabatanNama := "-"
	if pegawai.Jabatan != nil {
		jabatanNama = pegawai.Jabatan.Jabatan
	}
	unitKerjaNama := "-"
	if pegawai.UnitKerja != nil {
		unitKerjaNama = pegawai.UnitKerja.Unit
	}
	statusNama := ""
	if pegawai.Status != nil {
		statusNama = pegawai.Status.Status
	}
	// jenisPppk: menyesuaikan istilah PPPK/PPPK Paruh Waktu persis seperti
	// buildSuratRekomendasiPppk, supaya isi surat permohonan ini konsisten
	// dengan status kepegawaian pegawai yang sebenarnya.
	jenisPppk := "PPPK"
	if strings.Contains(strings.ToLower(statusNama), "paruh waktu") {
		jenisPppk = "PPPK Paruh Waktu"
	}

	const lineH = 16.0
	y := 56.0

	p.SetFont(false, 11)
	p.TextRight(rightX, y, "Lampiran 1")
	y += lineH
	// tanggal surat permohonan ini SENGAJA memakai tanggal SAAT DIUNDUH
	// (bukan tanggal_surat milik Surat Rekomendasi), karena surat ini
	// pegawai sendiri yang menulis & menandatangani -- wajar kalau
	// tanggalnya mengikuti kapan pegawai benar-benar mencetak/mengajukan.
	p.TextRight(rightX, y, "Kolonodale, "+formatDateID(time.Now())+".")
	y += lineH * 2

	p.Text(marginX, y, "Hal : Permohonan Perpanjangan Kontrak "+jenisPppk)
	y += lineH * 1.8

	p.Text(marginX, y, "Yth, Bapak BUPATI MOROWALI UTARA")
	y += lineH
	p.Text(marginX, y, "di-")
	y += lineH
	p.Text(marginX+28, y, "T e m p a t.")
	y += lineH * 1.8

	p.Text(marginX, y, "Dengan hormat,")
	y += lineH
	p.Text(marginX, y, "Saya yang bertanda tangan di bawah ini :")
	y += lineH

	const labelW = 94.0
	drawField := func(label, val string) {
		p.Text(marginX, y, label)
		p.Text(marginX+labelW, y, ": "+namaOrDash(val))
		y += lineH
	}
	drawField("Nama", pegawai.Nama)
	drawField("NI PPPK", pegawai.NIP)
	drawField("Jabatan", jabatanNama)
	drawField("Unit Kerja", unitKerjaNama)
	y += lineH * 0.6

	tahunLalu := item.Tahun - 1
	paragraf1 := fmt.Sprintf(
		"Dengan ini mengajukan permohonan perpanjangan perjanjian kerja (Kontrak) sebagai %s untuk jangka waktu selanjutnya, karena kontrak kerja saya akan berakhir pada 30 September %d.",
		jenisPppk, item.Tahun,
	)
	// JustifiedText (rata kiri-kanan) dipakai untuk semua paragraf isi surat
	// permohonan ini (sesuai permintaan), BUKAN MultilineText biasa yang
	// rata kiri saja -- lihat JustifiedText di utils/pdfwriter.go.
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, paragraf1) + lineH*0.6

	p.Text(marginX, y, "Sebagai bahan pertimbangan, saya lampirkan :")
	y += lineH

	poin := []string{
		fmt.Sprintf("1. Fotokopi SK %s Terakhir;", jenisPppk),
		fmt.Sprintf("2. ASLI Sasaran Kinerja Pegawai (SKP) Oktober s/d Desember %d dan Januari s/d Juni %d;", tahunLalu, item.Tahun),
		"3. ASLI Rekomendasi Perpanjangan Kontrak dari atasan langsung.",
	}
	for _, butir := range poin {
		y = p.JustifiedText(marginX+14, y, rightX-marginX-14, lineH, butir) + lineH*0.3
	}
	y += lineH * 0.4

	penutup := "Demikian permohonan ini saya sampaikan, kiranya dapat dipertimbangkan untuk terus mengabdi bagi daerah dan masyarakat Kabupaten Morowali Utara. Atas perhatian bapak Bupati, saya ucapkan terima kasih."
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, penutup) + lineH*2.6

	// blok tanda tangan -- ruang kosong di atas nama (SENGAJA TIDAK ada
	// garis tanda tangan maupun QR) untuk pegawai tanda tangan sendiri
	// secara fisik di kertas cetak, baru di bawahnya tercetak Nama & NI
	// PPPK sebagai identitas penanda tangan -- sebelumnya hanya NIP saja,
	// sekarang nama pegawai ikut ditampilkan sesuai permintaan.
	sigX := pageW - 230
	p.Text(sigX, y, "Yang bermohon,")
	y += lineH * 3.4
	p.SetFont(true, 11)
	p.Text(sigX, y, strings.ToUpper(namaOrDash(pegawai.Nama)))
	y += lineH
	p.SetFont(false, 11)
	p.Text(sigX, y, "NI PPPK : "+namaOrDash(pegawai.NIP))

	return doc.Output()
}

// ============================================================
// DTO
// ============================================================

type suratRekomendasiOut struct {
	ID                uint   `json:"id"`
	Judul             string `json:"judul"`
	NomorSurat        string `json:"nomor_surat"`
	NomorUrut         int    `json:"nomor_urut"`
	Tahun             int    `json:"tahun"`
	TanggalSurat      string `json:"tanggal_surat"`
	IDPegawai         uint   `json:"id_pegawai"`
	NamaPegawai       string `json:"nama_pegawai"`
	NipPegawai        string `json:"nip_pegawai"`
	Jabatan           string `json:"jabatan"`
	UnitKerja         string `json:"unit_kerja"`
	StatusKepegawaian string `json:"status_kepegawaian"`
	DibuatOlehNama    string `json:"dibuat_oleh_nama"`
	CreatedAt         string `json:"created_at"`
	// IsSekolah: true kalau pegawai penerima surat ini bertempat tugas
	// Sekolah/Puskesmas (lihat isSekolahPegawai) -- dipakai frontend untuk
	// tahu surat ini pakai "Lampiran 3" (butuh persetujuan atasan) atau
	// "Lampiran 2" biasa (Dinas/Kantor, otomatis "disetujui", tanpa approval).
	IsSekolah bool `json:"is_sekolah"`
	// StatusApproval & AtasanApproveNama/TglApproval: lihat komentar field
	// senama di models.SuratRekomendasi -- HANYA relevan kalau IsSekolah
	// true (surat Dinas/Kantor selalu "disetujui" sejak dibuat).
	StatusApproval    string `json:"status_approval"`
	AtasanApproveNama string `json:"atasan_approve_nama,omitempty"`
	TglApproval       string `json:"tgl_approval,omitempty"`
}

func toSuratRekomendasiOut(item models.SuratRekomendasi) suratRekomendasiOut {
	out := suratRekomendasiOut{
		ID:             item.ID,
		Judul:          item.Judul,
		NomorUrut:      item.NomorUrut,
		Tahun:          item.Tahun,
		TanggalSurat:   item.TanggalSurat.Format("2006-01-02"),
		IDPegawai:      item.IDPegawai,
		DibuatOlehNama: item.DibuatOlehNama,
		CreatedAt:      item.CreatedAt.Format(time.RFC3339),
		StatusApproval: item.StatusApproval,
	}
	if item.AtasanApprove != nil {
		out.AtasanApproveNama = item.AtasanApprove.Nama
	}
	if item.TglApproval != nil {
		out.TglApproval = item.TglApproval.Format(time.RFC3339)
	}
	if item.Pegawai != nil {
		out.NamaPegawai = item.Pegawai.Nama
		out.NipPegawai = item.Pegawai.NIP
		if item.Pegawai.Jabatan != nil {
			out.Jabatan = item.Pegawai.Jabatan.Jabatan
		}
		if item.Pegawai.UnitKerja != nil {
			out.UnitKerja = item.Pegawai.UnitKerja.Unit
		}
		if item.Pegawai.Status != nil {
			out.StatusKepegawaian = item.Pegawai.Status.Status
		}
		out.IsSekolah = isSekolahPegawai(*item.Pegawai)
		if out.IsSekolah {
			singkatan := "-"
			if item.Pegawai.UnitKerja != nil && item.Pegawai.UnitKerja.Singkatan != nil && strings.TrimSpace(*item.Pegawai.UnitKerja.Singkatan) != "" {
				singkatan = *item.Pegawai.UnitKerja.Singkatan
			}
			out.NomorSurat = nomorSuratRekomendasiSekolahLengkap(item.Tahun, item.TanggalSurat, singkatan)
		} else {
			out.NomorSurat = nomorSuratRekomendasiLengkap(item.NomorUrut, item.Tahun, item.TanggalSurat)
		}
	} else {
		out.NomorSurat = nomorSuratRekomendasiLengkap(item.NomorUrut, item.Tahun, item.TanggalSurat)
	}
	return out
}

// suratRekomendasiPreload juga memuat relasi Pegawai.Atasan (dengan
// Jabatan/UnitKerja/PangkatGol-nya) -- dibutuhkan untuk membangun "Lampiran
// 3" (lihat buildSuratRekomendasiSekolah) yang mengambil Nama/NIP/Jabatan
// penandatangan utama (Kepala Sekolah/Kepala Puskesmas) dari data ATASAN
// LANGSUNG pegawai, bukan dari Kepala Dinas.
func suratRekomendasiPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Pegawai", func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }).
		Preload("Pegawai.Jabatan").Preload("Pegawai.UnitKerja").Preload("Pegawai.Status").
		Preload("Pegawai.PangkatGol.Pangkat").Preload("Pegawai.PangkatGol.Gol").
		Preload("Pegawai.Atasan").Preload("Pegawai.Atasan.Jabatan").Preload("Pegawai.Atasan.UnitKerja").
		Preload("Pegawai.Atasan.PangkatGol.Pangkat").Preload("Pegawai.Atasan.PangkatGol.Gol").
		Preload("AtasanApprove")
}

func canManageSuratRekomendasi(claims *utils.Claims) bool {
	return claims != nil && (claims.RoleName == "administrator" || claims.RoleName == "admin")
}

// canAccessSuratRekomendasi: administrator/admin bebas, pegawai hanya
// suratnya sendiri, ATASAN (Kepala Sekolah/Kepala Puskesmas dkk.) juga boleh
// melihat/mengunduh surat BAWAHAN LANGSUNGNYA -- dibutuhkan supaya atasan
// bisa meninjau isi surat sebelum menyetujuinya (lihat approveSuratRekomendasi
// & isAtasanLangsungSuratRekomendasi).
func canAccessSuratRekomendasi(claims *utils.Claims, item models.SuratRekomendasi) bool {
	if claims == nil {
		return false
	}
	if canManageSuratRekomendasi(claims) {
		return true
	}
	if claims.IDPegawai == nil {
		return false
	}
	if *claims.IDPegawai == item.IDPegawai {
		return true
	}
	return isAtasanLangsungSuratRekomendasi(claims, item)
}

// isAtasanLangsungSuratRekomendasi mengecek apakah akun yang login (role
// "atasan") adalah atasan langsung (Pegawai.IDAtasan) dari pegawai penerima
// surat ini -- item.Pegawai WAJIB sudah dipreload oleh pemanggil.
func isAtasanLangsungSuratRekomendasi(claims *utils.Claims, item models.SuratRekomendasi) bool {
	return claims != nil && claims.RoleName == "atasan" && claims.IDPegawai != nil &&
		item.Pegawai != nil && item.Pegawai.IDAtasan != nil && *item.Pegawai.IDAtasan == *claims.IDPegawai
}

// ============================================================
// HTTP handlers
// ============================================================

// listSuratRekomendasi menangani GET /api/surat-rekomendasi -- administrator/
// admin melihat SEMUA surat (boleh difilter ?tahun=, ?q=nama/nip, ?id_status=),
// pegawai/atasan HANYA melihat surat milik pegawai dirinya sendiri (menu
// "Arsip Surat").
func listSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims == nil {
		utils.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := r.URL.Query()
	base := db.Model(&models.SuratRekomendasi{})

	if canManageSuratRekomendasi(claims) {
		search := strings.TrimSpace(q.Get("q"))
		idStatus := strings.TrimSpace(q.Get("id_status"))
		if search != "" || idStatus != "" {
			base = base.Joins("JOIN pegawai ON pegawai.id = surat_rekomendasi.id_pegawai")
		}
		if tahunStr := strings.TrimSpace(q.Get("tahun")); tahunStr != "" {
			if tahun, err := strconv.Atoi(tahunStr); err == nil {
				base = base.Where("surat_rekomendasi.tahun = ?", tahun)
			}
		}
		if search != "" {
			base = base.Where("pegawai.nama ILIKE ? OR pegawai.nip ILIKE ?", "%"+search+"%", "%"+search+"%")
		}
		if idStatus != "" {
			base = base.Where("pegawai.id_status = ?", idStatus)
		}
	} else if claims.RoleName == "atasan" {
		// atasan (mis. Kepala Sekolah/Kepala Puskesmas) melihat surat
		// BAWAHAN LANGSUNGNYA di sini (bukan suratnya sendiri) -- supaya bisa
		// meninjau & menyetujui surat Lampiran 3 yang masih "pending" (lihat
		// approveSuratRekomendasi), pola sama seperti listPengajuanCuti role
		// "atasan" di pengajuan_cuti.go.
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []suratRekomendasiOut{})
			return
		}
		base = base.Where("surat_rekomendasi.id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)", *claims.IDPegawai)
	} else {
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []suratRekomendasiOut{})
			return
		}
		base = base.Where("surat_rekomendasi.id_pegawai = ?", *claims.IDPegawai)
	}

	var items []models.SuratRekomendasi
	if err := suratRekomendasiPreload(base).
		Order("surat_rekomendasi.tanggal_surat desc, surat_rekomendasi.nomor_urut desc").
		Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data surat rekomendasi")
		return
	}
	out := make([]suratRekomendasiOut, 0, len(items))
	for _, it := range items {
		out = append(out, toSuratRekomendasiOut(it))
	}
	utils.Success(w, "ok", out)
}

// listCalonPegawaiSuratRekomendasi menangani GET /api/surat-rekomendasi/
// calon-pegawai?q=..&id_status=..&tahun_tmt=..&pageSize=.. -- daftar pegawai
// dipakai mengisi pilihan pada dialog "Kirim Surat Rekomendasi" (baik
// dicari satu-satu lewat nama/NIP, maupun difilter sekaligus lewat Status
// Kepegawaian & tahun TMT).
func listCalonPegawaiSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	q := r.URL.Query()
	idStatus := strings.TrimSpace(q.Get("id_status"))
	// idStatusList: id_status boleh berisi BEBERAPA id status dipisah koma
	// (mis. "4,5" = PPPK + PPPK Paruh Waktu sekaligus) supaya admin bisa
	// mengirim surat rekomendasi kolektif untuk gabungan kedua status itu
	// dalam satu kali "Kirim", TANPA menghilangkan kemampuan memfilter satu
	// status saja (kirim satu-satu/per status tetap jalan seperti biasa).
	var idStatusList []string
	if idStatus != "" {
		for _, part := range strings.Split(idStatus, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				idStatusList = append(idStatusList, part)
			}
		}
	}
	tahunTmt := strings.TrimSpace(q.Get("tahun_tmt"))
	search := strings.TrimSpace(q.Get("q"))

	// defaultPageSize/maxPageSize: pencarian BEBAS tanpa filter (hanya
	// mengetik nama/NIP) dibatasi wajar (200) supaya ringan. TAPI begitu
	// admin memfilter sekaligus lewat Status Kepegawaian dan/atau Tahun
	// TMT, maksudnya jelas "ambil SEMUA pegawai yang cocok" untuk kirim
	// kolektif/"pilih semua" -- jadi limitnya dinaikkan jauh lebih besar
	// (5000) supaya TIDAK ada pegawai yang tercecer dari daftar hanya
	// karena jumlahnya di atas 200 (lihat laporan bug: filter PPPK + tahun
	// TMT 2025 menghasilkan lebih dari 200 pegawai, tapi daftar/tombol
	// "Kirim" hanya menghitung 200 karena limit lama selalu 200 flat).
	defaultPageSize, maxPageSize := 200, 500
	if idStatus != "" || tahunTmt != "" {
		defaultPageSize, maxPageSize = 5000, 5000
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}

	query := db.Model(&models.Pegawai{}).Omit(dokumenFileFields...).
		Preload("Jabatan").Preload("UnitKerja").Preload("Status").
		Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol")

	if len(idStatusList) > 0 {
		query = query.Where("id_status IN ?", idStatusList)
	}
	if tahunTmt != "" {
		if tahun, err := strconv.Atoi(tahunTmt); err == nil {
			query = query.Where("tmt IS NOT NULL AND EXTRACT(YEAR FROM tmt) = ?", tahun)
		}
	}
	if search != "" {
		query = query.Where("nama ILIKE ? OR nip ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var items []models.Pegawai
	if err := query.Order("nama asc").Limit(pageSize).Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}
	utils.Success(w, "ok", items)
}

// nextNomorSuratRekomendasi menangani GET /api/surat-rekomendasi/next-nomor?
// tahun=YYYY -- kalau BELUM ADA surat rekomendasi tahun itu ("editable":
// true), frontend WAJIB menampilkan input nomor urut awal yang bisa diubah
// admin; kalau SUDAH ADA ("editable": false), nomor_urut yang dikembalikan
// di sini harus dipakai apa adanya (kelanjutan otomatis).
func nextNomorSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	tahun := time.Now().Year()
	if tahunStr := strings.TrimSpace(r.URL.Query().Get("tahun")); tahunStr != "" {
		if t, err := strconv.Atoi(tahunStr); err == nil {
			tahun = t
		}
	}
	var maxNomor int
	db.Model(&models.SuratRekomendasi{}).Where("tahun = ?", tahun).
		Select("COALESCE(MAX(nomor_urut), 0)").Scan(&maxNomor)
	utils.Success(w, "ok", map[string]interface{}{
		"tahun":      tahun,
		"nomor_urut": maxNomor + 1,
		"editable":   maxNomor == 0,
	})
}

type buatSuratRekomendasiPayload struct {
	Judul         string `json:"judul"`
	TanggalSurat  string `json:"tanggal_surat"`
	Tahun         int    `json:"tahun"`
	NomorUrutAwal *int   `json:"nomor_urut_awal"`
	IDPegawai     []uint `json:"id_pegawai"`
}

// buatSuratRekomendasi menangani POST /api/surat-rekomendasi -- mengirim
// surat rekomendasi untuk SATU atau BEBERAPA pegawai sekaligus (kolektif)
// dalam satu kali panggilan.
//
// PENTING -- anti nomor surat dobel: kalau pegawai yang dipilih SUDAH
// PERNAH dikirimi surat rekomendasi di TAHUN YANG SAMA, kirim ulang untuk
// pegawai itu TIDAK membuat baris baru/nomor baru -- baris yang sudah ada
// hanya diperbarui (Judul & catatan pengirim), sementara Nomor Urut &
// Tanggal Surat ASLINYA dipertahankan apa adanya. Pegawai yang BELUM punya
// surat tahun ini tetap mendapat baris baru dengan nomor urut berikutnya
// seperti biasa (lihat komentar models.SuratRekomendasi perihal aturan
// nomor urut per tahun). Ini mencegah satu pegawai punya dua nomor surat
// berbeda di tahun yang sama hanya karena admin tidak sadar sudah pernah
// mengirim sebelumnya.
//
// MODE NOMOR URUT -- p.NomorUrutAwal boleh diisi KAPAN SAJA (bukan cuma
// untuk surat pertama tahun ini): kalau diisi, dipakai sebagai nomor awal
// untuk pegawai-pegawai BARU di batch ini (override "mulai nomor baru");
// kalau dikosongkan DAN sudah pernah ada surat tahun ini, otomatis
// melanjutkan dari nomor_urut terbesar + 1 ("lanjutkan otomatis") --
// pilihan ini yang ditampilkan sebagai radio button di frontend
// (SuratRekomendasiView.vue, modeNomor). Kalau belum ada surat SAMA
// SEKALI untuk tahun itu, p.NomorUrutAwal WAJIB diisi (tidak ada "nomor
// sebelumnya" untuk dilanjutkan). Override nomor awal divalidasi supaya
// TIDAK bentrok dengan nomor urut surat lain yang sudah terbit tahun yang
// sama (lihat pengecekan "bentrok" di bawah).
func buatSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p buatSuratRekomendasiPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	judul := strings.TrimSpace(p.Judul)
	if judul == "" {
		utils.Error(w, http.StatusBadRequest, "judul surat wajib diisi")
		return
	}
	tanggalSurat, err := time.Parse("2006-01-02", strings.TrimSpace(p.TanggalSurat))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal surat wajib diisi & valid")
		return
	}
	tahun := p.Tahun
	if tahun == 0 {
		tahun = tanggalSurat.Year()
	}
	if tahun < 2000 || tahun > time.Now().Year()+1 {
		utils.Error(w, http.StatusBadRequest, "tahun surat tidak valid")
		return
	}

	// dedupe id_pegawai TANPA mengacak urutan pemilihan admin -- nomor urut
	// yang dibagikan mengikuti urutan pemilihan.
	seen := map[uint]bool{}
	idList := make([]uint, 0, len(p.IDPegawai))
	for _, id := range p.IDPegawai {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		idList = append(idList, id)
	}
	if len(idList) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu pegawai")
		return
	}

	var pegawaiList []models.Pegawai
	// Preload UnitKerja supaya isSekolahPegawai di bawah memeriksa field
	// TempatKerja yang otoritatif (bukan jatuh ke tebakan teks TempatTgs) --
	// dipakai menentukan StatusApproval baris baru (lihat komentar di bawah).
	if err := db.Preload("UnitKerja").Where("id IN ?", idList).Find(&pegawaiList).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}
	pegawaiByID := map[uint]models.Pegawai{}
	for _, pg := range pegawaiList {
		pegawaiByID[pg.ID] = pg
	}

	// surat yang SUDAH ADA tahun ini untuk pegawai-pegawai yang dipilih --
	// dikunci dari IDPegawai (satu pegawai maksimal satu baris per tahun).
	// Kirim ulang untuk pegawai di peta ini akan MEMPERBARUI baris itu, BUKAN
	// membuat baris baru.
	var existingRows []models.SuratRekomendasi
	if err := db.Where("tahun = ? AND id_pegawai IN ?", tahun, idList).Find(&existingRows).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memeriksa surat rekomendasi yang sudah ada")
		return
	}
	existingByPegawai := map[uint]models.SuratRekomendasi{}
	for _, er := range existingRows {
		existingByPegawai[er.IDPegawai] = er
	}

	// nomor urut awal HANYA perlu ditanyakan kalau memang ada pegawai BARU
	// (belum punya surat tahun ini) yang bakal butuh nomor baru -- kalau
	// SEMUA yang dipilih ternyata sudah punya surat tahun ini (murni
	// kirim ulang/perbarui), tidak ada nomor baru yang dipakai sama sekali.
	adaYangBaru := false
	jumlahBaru := 0
	for _, id := range idList {
		if _, ada := existingByPegawai[id]; !ada {
			adaYangBaru = true
			jumlahBaru++
		}
	}

	var maxNomor int
	db.Model(&models.SuratRekomendasi{}).Where("tahun = ?", tahun).
		Select("COALESCE(MAX(nomor_urut), 0)").Scan(&maxNomor)

	// nomorMulai: default "lanjutkan otomatis" (maxNomor+1). Kalau admin
	// mengisi p.NomorUrutAwal -- baik karena ini surat pertama tahun ini
	// (wajib) ATAU karena admin SENGAJA memilih mode "mulai dari nomor
	// baru" di tengah tahun -- nomor itu dipakai sebagai override.
	nomorMulai := maxNomor + 1
	if adaYangBaru {
		if p.NomorUrutAwal != nil {
			if *p.NomorUrutAwal < 1 {
				utils.Error(w, http.StatusBadRequest, "nomor urut awal tidak valid")
				return
			}
			nomorMulai = *p.NomorUrutAwal
		} else if maxNomor == 0 {
			utils.Error(w, http.StatusBadRequest, fmt.Sprintf("ini surat rekomendasi pertama untuk tahun %d -- isi nomor urut awal terlebih dahulu", tahun))
			return
		}
	}

	// bentrok nomor urut: kalau admin meng-override nomor awal (bukan
	// kelanjutan otomatis), pastikan rentang nomor yang akan dipakai untuk
	// pegawai BARU di batch ini (nomorMulai..nomorMulai+jumlahBaru-1) belum
	// dipakai surat lain milik pegawai manapun di tahun yang sama --
	// mencegah dua surat berbeda terbit dengan nomor urut sama persis.
	if p.NomorUrutAwal != nil && jumlahBaru > 0 {
		var nomorDipakai []int
		db.Model(&models.SuratRekomendasi{}).Where("tahun = ?", tahun).Pluck("nomor_urut", &nomorDipakai)
		dipakaiSet := make(map[int]bool, len(nomorDipakai))
		for _, n := range nomorDipakai {
			dipakaiSet[n] = true
		}
		var bentrok []string
		for i := 0; i < jumlahBaru; i++ {
			if n := nomorMulai + i; dipakaiSet[n] {
				bentrok = append(bentrok, strconv.Itoa(n))
			}
		}
		if len(bentrok) > 0 {
			utils.Error(w, http.StatusBadRequest, fmt.Sprintf("nomor urut %s di tahun %d sudah dipakai surat lain -- pilih nomor urut awal yang lain", strings.Join(bentrok, ", "), tahun))
			return
		}
	}

	claims, _ := middleware.GetClaims(r)
	dibuatOleh := ""
	if claims != nil {
		dibuatOleh = claims.Username
	}

	var dibuat, diperbarui int
	var dilewati []string
	nomorBerjalan := nomorMulai
	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, id := range idList {
			pg, ok := pegawaiByID[id]
			if !ok {
				dilewati = append(dilewati, fmt.Sprintf("ID %d (tidak ditemukan)", id))
				continue
			}
			if existing, ada := existingByPegawai[id]; ada {
				// sudah pernah dikirimi tahun ini -- perbarui judul & catatan
				// pengirim SAJA, nomor urut & tanggal surat aslinya tetap.
				existing.Judul = judul
				existing.DibuatOlehNama = dibuatOleh
				if err := tx.Save(&existing).Error; err != nil {
					return fmt.Errorf("gagal memperbarui surat untuk %s: %w", pg.Nama, err)
				}
				diperbarui++
				continue
			}
			// StatusApproval baris baru: pegawai Sekolah/Puskesmas ("Lampiran
			// 3") mulai "pending" -- butuh persetujuan atasan langsung dulu
			// sebelum QR tanda tangannya tampil (lihat buildSuratRekomendasiSekolah
			// & approveSuratRekomendasi). Pegawai Dinas/Kantor ("Lampiran 2")
			// tetap langsung "disetujui" seperti sebelumnya, tidak berubah.
			statusApproval := models.StatusDisetuju
			if isSekolahPegawai(pg) {
				statusApproval = models.StatusPending
			}
			row := models.SuratRekomendasi{
				IDPegawai:      id,
				Judul:          judul,
				NomorUrut:      nomorBerjalan,
				Tahun:          tahun,
				TanggalSurat:   tanggalSurat,
				DibuatOlehNama: dibuatOleh,
				StatusApproval: statusApproval,
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("gagal menyimpan surat untuk %s: %w", pg.Nama, err)
			}
			dibuat++
			nomorBerjalan++
		}
		return nil
	})
	if txErr != nil {
		utils.Error(w, http.StatusInternalServerError, txErr.Error())
		return
	}
	if dibuat == 0 && diperbarui == 0 {
		utils.Error(w, http.StatusBadRequest, "tidak ada surat yang berhasil diproses -- pegawai yang dipilih tidak ditemukan")
		return
	}

	var bagianPesan []string
	if dibuat > 0 {
		bagianPesan = append(bagianPesan, fmt.Sprintf("%d surat baru dikirim (nomor urut %d-%d/%d)", dibuat, nomorMulai, nomorBerjalan-1, tahun))
	}
	if diperbarui > 0 {
		bagianPesan = append(bagianPesan, fmt.Sprintf("%d surat diperbarui (sudah pernah dikirim tahun %d -- nomor & tanggal surat lama dipertahankan)", diperbarui, tahun))
	}
	msg := strings.Join(bagianPesan, "; ")
	if len(dilewati) > 0 {
		msg += fmt.Sprintf(" -- %d dilewati: %s", len(dilewati), strings.Join(dilewati, ", "))
	}
	utils.Created(w, msg, map[string]interface{}{"dibuat": dibuat, "diperbarui": diperbarui, "dilewati": len(dilewati)})
}

// hapusSuratRekomendasi menangani DELETE /api/surat-rekomendasi/{id} --
// menarik kembali surat yang salah kirim (mis. salah nomor/pegawai).
// Menghapus TIDAK menggeser nomor urut surat lain yang sudah terbit
// (meninggalkan "lubang" nomor apa adanya, sama seperti membatalkan blanko
// surat fisik).
func hapusSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	if err := db.Delete(&models.SuratRekomendasi{}, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus surat rekomendasi: "+err.Error())
		return
	}
	utils.Success(w, "surat rekomendasi berhasil dihapus", nil)
}

// exportSuratRekomendasi menangani GET /api/surat-rekomendasi/export?
// tanggal_mulai=YYYY-MM-DD&tanggal_selesai=YYYY-MM-DD -- mengunduh daftar
// Nomor Surat -> Nama Pegawai (beserta beberapa kolom pendukung) sebagai
// Excel, difilter dari TANGGAL SURAT (tanggal dikirimnya surat rekomendasi
// itu, models.SuratRekomendasi.TanggalSurat) -- BUKAN tanggal baris
// dibuat/CreatedAt. Kedua parameter opsional & bisa dipakai sendiri-sendiri
// (mis. hanya tanggal_mulai = "sejak tanggal itu", hanya tanggal_selesai =
// "sampai tanggal itu"); kalau keduanya kosong, seluruh surat diexport.
func exportSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	q := r.URL.Query()
	query := db.Model(&models.SuratRekomendasi{})

	var dariLabel, sampaiLabel string
	if s := strings.TrimSpace(q.Get("tanggal_mulai")); s != "" {
		if tgl, err := time.Parse("2006-01-02", s); err == nil {
			query = query.Where("tanggal_surat >= ?", tgl)
			dariLabel = tgl.Format("2006-01-02")
		} else {
			utils.Error(w, http.StatusBadRequest, "tanggal_mulai tidak valid (format YYYY-MM-DD)")
			return
		}
	}
	if s := strings.TrimSpace(q.Get("tanggal_selesai")); s != "" {
		if tgl, err := time.Parse("2006-01-02", s); err == nil {
			query = query.Where("tanggal_surat <= ?", tgl)
			sampaiLabel = tgl.Format("2006-01-02")
		} else {
			utils.Error(w, http.StatusBadRequest, "tanggal_selesai tidak valid (format YYYY-MM-DD)")
			return
		}
	}

	var rows []models.SuratRekomendasi
	if err := suratRekomendasiPreload(query).
		Order("tanggal_surat asc, nomor_urut asc").
		Find(&rows).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data surat rekomendasi")
		return
	}

	items := make([]suratRekomendasiOut, 0, len(rows))
	for _, it := range rows {
		items = append(items, toSuratRekomendasiOut(it))
	}

	// noCounter: kolom "No" (nomor urut BARIS di Excel, BUKAN nomor_urut
	// surat) -- ExcelColumn.Get tidak diberi indeks barisnya sendiri, jadi
	// dihitung manual lewat closure ini. Aman karena kolom "No" SENGAJA
	// ditaruh PALING PERTAMA di slice columns di bawah -- ExportData
	// memanggil Get tiap kolom berurutan per baris, jadi noCounter
	// bertambah tepat satu kali per baris, sesuai urutan baris.
	noCounter := 0
	columns := []utils.ExcelColumn{
		{Header: "No", Get: func(i interface{}) string { noCounter++; return strconv.Itoa(noCounter) }},
		{Header: "Nomor Surat", Get: func(i interface{}) string { return i.(suratRekomendasiOut).NomorSurat }},
		{Header: "Nama Pegawai", Get: func(i interface{}) string { return i.(suratRekomendasiOut).NamaPegawai }},
		{Header: "NIP", Get: func(i interface{}) string { return i.(suratRekomendasiOut).NipPegawai }},
		{Header: "Jabatan", Get: func(i interface{}) string { return i.(suratRekomendasiOut).Jabatan }},
		{Header: "Unit Kerja", Get: func(i interface{}) string { return i.(suratRekomendasiOut).UnitKerja }},
		{Header: "Status Kepegawaian", Get: func(i interface{}) string { return i.(suratRekomendasiOut).StatusKepegawaian }},
		{Header: "Judul Surat", Get: func(i interface{}) string { return i.(suratRekomendasiOut).Judul }},
		{Header: "Tanggal Surat", Get: func(i interface{}) string { return i.(suratRekomendasiOut).TanggalSurat }},
	}
	f, err := utils.ExportData(items, columns)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := "surat_rekomendasi"
	if dariLabel != "" || sampaiLabel != "" {
		filename += "_" + dariLabel + "_" + sampaiLabel
	}
	filename += ".xlsx"
	writeXlsxResponse(w, f, filename)
}

// pdfSuratRekomendasi menangani GET /api/surat-rekomendasi/{id}/pdf?inline=1
// -- administrator/admin boleh membuka surat siapa saja, pegawai/atasan
// hanya boleh membuka surat miliknya sendiri (lihat canAccessSuratRekomendasi).
func pdfSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.SuratRekomendasi
	if err := suratRekomendasiPreload(db).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "surat rekomendasi tidak ditemukan")
		return
	}
	if !canAccessSuratRekomendasi(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak berhak mengakses surat ini")
		return
	}
	if item.Pegawai == nil {
		utils.Error(w, http.StatusInternalServerError, "data pegawai penerima surat tidak ditemukan")
		return
	}

	pengaturan := pengaturanSuratOrDefault(db)
	signerNama, signerNip, signerJabatan, signerPangkatGol, signerUnitKerja := resolveSignerFullRekomendasi(db, pengaturan)

	// Alur surat bercabang menurut tempat tugas pegawai (lihat
	// isSekolahPegawai di pengajuan_cuti.go): pegawai di Dinas/Kantor tetap
	// memakai "Lampiran 2" (buildSuratRekomendasiPppk, kop Dinas tetap,
	// ditandatangani Kepala Dinas) -- TIDAK berubah. Pegawai bertempat
	// tugas Sekolah/Puskesmas memakai "Lampiran 3" (buildSuratRekomendasiSekolah,
	// kop unit kerja sendiri, ditandatangani atasan langsung pegawai,
	// Kepala Dinas tampil sebagai "Mengetahui").
	var pdfBytes []byte
	var err error
	if isSekolahPegawai(*item.Pegawai) {
		pdfBytes, err = buildSuratRekomendasiSekolah(item, signerNama, signerNip, signerJabatan)
	} else {
		pdfBytes, err = buildSuratRekomendasiPppk(item, signerNama, signerNip, signerJabatan, signerPangkatGol, signerUnitKerja)
	}
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat surat: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("surat_rekomendasi_%s.pdf", item.Pegawai.NIP))
}

// pdfPermohonanSuratRekomendasi menangani GET /api/surat-rekomendasi/{id}/
// pdf-permohonan?inline=1 -- mengunduh/melihat Lampiran 1 (surat
// permohonan perpanjangan kontrak yang DIAJUKAN PEGAWAI SENDIRI ke
// Bupati), dibuat dari baris SuratRekomendasi yang SAMA dengan
// pdfSuratRekomendasi di atas -- sehingga dari satu kartu Arsip Surat,
// pegawai bisa mengunduh KEDUA berkas sekaligus: surat rekomendasi resmi
// dari Kepala Dinas, dan surat permohonan ini. Hak akses SAMA PERSIS
// dengan pdfSuratRekomendasi (lihat canAccessSuratRekomendasi).
func pdfPermohonanSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.SuratRekomendasi
	if err := suratRekomendasiPreload(db).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "surat rekomendasi tidak ditemukan")
		return
	}
	if !canAccessSuratRekomendasi(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak berhak mengakses surat ini")
		return
	}
	if item.Pegawai == nil {
		utils.Error(w, http.StatusInternalServerError, "data pegawai penerima surat tidak ditemukan")
		return
	}
	pdfBytes, err := buildPermohonanPerpanjanganKontrak(item)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat surat: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("permohonan_perpanjangan_kontrak_%s.pdf", item.Pegawai.NIP))
}

// approveSuratRekomendasi menangani PUT /api/surat-rekomendasi/{id}/approve
// -- KHUSUS surat "Lampiran 3" (pegawai bertempat tugas Sekolah/Puskesmas,
// lihat isSekolahPegawai) yang masih berstatus "pending": atasan langsung
// pegawai (Kepala Sekolah/Kepala Puskesmas, role "atasan", dikenali dari
// Pegawai.IDAtasan) menyetujuinya di sini -- begitu disetujui, QR tanda
// tangan otomatis langsung muncul di PDF surat ini (lihat gating
// item.StatusApproval pada buildSuratRekomendasiSekolah). administrator/admin
// juga boleh memakai endpoint ini (jaga-jaga, mis. atasan belum punya akun).
// Surat "Lampiran 2" (Dinas/Kantor) TIDAK memerlukan ini -- selalu sudah
// "disetujui" sejak dibuat (lihat buatSuratRekomendasi).
func approveSuratRekomendasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.SuratRekomendasi
	if err := suratRekomendasiPreload(db).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "surat rekomendasi tidak ditemukan")
		return
	}
	if item.Pegawai == nil {
		utils.Error(w, http.StatusInternalServerError, "data pegawai penerima surat tidak ditemukan")
		return
	}
	if !canManageSuratRekomendasi(claims) && !isAtasanLangsungSuratRekomendasi(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda hanya dapat menyetujui surat rekomendasi bawahan langsung anda")
		return
	}
	if !isSekolahPegawai(*item.Pegawai) {
		utils.Error(w, http.StatusBadRequest, "surat rekomendasi pegawai Dinas/Kantor tidak memerlukan persetujuan atasan")
		return
	}
	if item.StatusApproval == models.StatusDisetuju {
		utils.Error(w, http.StatusBadRequest, "surat ini sudah disetujui sebelumnya")
		return
	}
	now := time.Now()
	item.StatusApproval = models.StatusDisetuju
	item.TglApproval = &now
	if claims.IDPegawai != nil {
		item.IDAtasanApprove = claims.IDPegawai
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyetujui surat rekomendasi: "+err.Error())
		return
	}
	suratRekomendasiPreload(db).First(&item, item.ID)
	utils.Success(w, "surat rekomendasi berhasil disetujui -- tanda tangan QR otomatis sudah muncul di surat ini", toSuratRekomendasiOut(item))
}

// RegisterSuratRekomendasiRoutes mendaftarkan semua endpoint
// /api/surat-rekomendasi*.
func RegisterSuratRekomendasiRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "pegawai", "atasan") }
	// approverRoles: administrator/admin (jaga-jaga/override) + "atasan" --
	// dipakai KHUSUS endpoint approve, pola sama seperti approverRoles di
	// pengajuan_cuti.go.
	approverRoles := func(h http.HandlerFunc) http.Handler { return authed(h, "atasan", "administrator", "admin") }

	mux.Handle("GET /api/surat-rekomendasi", anyRole(func(w http.ResponseWriter, r *http.Request) { listSuratRekomendasi(w, r, db) }))
	mux.Handle("GET /api/surat-rekomendasi/calon-pegawai", manage(func(w http.ResponseWriter, r *http.Request) { listCalonPegawaiSuratRekomendasi(w, r, db) }))
	mux.Handle("GET /api/surat-rekomendasi/next-nomor", manage(func(w http.ResponseWriter, r *http.Request) { nextNomorSuratRekomendasi(w, r, db) }))
	mux.Handle("GET /api/surat-rekomendasi/export", manage(func(w http.ResponseWriter, r *http.Request) { exportSuratRekomendasi(w, r, db) }))
	mux.Handle("POST /api/surat-rekomendasi", manage(func(w http.ResponseWriter, r *http.Request) { buatSuratRekomendasi(w, r, db) }))
	mux.Handle("DELETE /api/surat-rekomendasi/{id}", manage(func(w http.ResponseWriter, r *http.Request) { hapusSuratRekomendasi(w, r, db) }))
	mux.Handle("GET /api/surat-rekomendasi/{id}/pdf", anyRole(func(w http.ResponseWriter, r *http.Request) { pdfSuratRekomendasi(w, r, db) }))
	mux.Handle("GET /api/surat-rekomendasi/{id}/pdf-permohonan", anyRole(func(w http.ResponseWriter, r *http.Request) { pdfPermohonanSuratRekomendasi(w, r, db) }))
	mux.Handle("PUT /api/surat-rekomendasi/{id}/approve", approverRoles(func(w http.ResponseWriter, r *http.Request) { approveSuratRekomendasi(w, r, db) }))
}
