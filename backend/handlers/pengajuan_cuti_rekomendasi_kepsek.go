package handlers

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"cuti-app/models"
	"cuti-app/utils"

	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// pengajuan_cuti_rekomendasi_kepsek.go generates "Surat Rekomendasi Kepala
// Sekolah" -- the recommendation letter a Kepala Sekolah writes to endorse a
// pegawai's leave request, normally attached when the pegawai forwards their
// cuti paperwork to Dinas/BKPSDM. Permintaan pengguna: sebelumnya pegawai
// harus mengupload sendiri hasil scan surat ini (dicetak manual, ditanda
// tangani basah Kepala Sekolah) sebagai salah satu syarat wajib mengajukan
// cuti -- sekarang surat ini dibuat OTOMATIS oleh sistem begitu Kepala
// Sekolah (atasan langsung pegawai) menyetujui pengajuan cuti lewat menu
// Pengajuan Cuti seperti biasa, lengkap dengan nomor surat OTOMATIS (lihat
// nextNomorUrutSuratKepsek) & tanda tangan QR (meniru pola buildSignatureQR
// di formulir.go) -- pegawai TIDAK perlu lagi mengupload apa pun (lihat
// dokumenRequirementsForJenis di pengajuan_cuti.go, yang sudah tidak lagi
// mewajibkan berkas "rekomendasi_kepsek").
//
// Kop surat memakai Kop Surat Sekolah milik sekolah pegawai sendiri kalau
// sudah diatur (drawLetterheadUnitKerjaKustom di surat_rekomendasi.go,
// dipakai ulang di sini), sama seperti "Lampiran 3". Data Kepala Sekolah
// (nama/NIP/pangkat-golongan/jabatan) & nomor surat dibekukan SEKALI oleh
// approvePengajuan (pengajuan_cuti.go) ke field KepsekNama dkk./
// NomorUrutKepsek/TahunSuratKepsek pada models.PengajuanCuti -- lihat
// catatan lengkap di sana.

// nomorSuratKepsekLengkap merangkai nomor surat lengkap Surat Rekomendasi
// Kepala Sekolah, format "800/{urut}/{singkatan sekolah}/{bulan romawi}/
// {tahun}" (mis. "800/125/SMAN.1/XI/2026") -- BEDA dari
// nomorSuratRekomendasiSekolahLengkap (surat_rekomendasi.go) yang sengaja
// mengosongkan nomor urutnya ("......") karena surat ITU memang tidak
// diotomatisasi; surat INI permintaan pengguna eksplisit "nomor suratnya
// juga akan otomatis".
func nomorSuratKepsekLengkap(nomorUrut, tahun int, tanggalSurat time.Time, singkatanUnitKerja string) string {
	return fmt.Sprintf("800/%d/%s/%s/%d", nomorUrut, singkatanUnitKerja, romanMonth(tanggalSurat.Month()), tahun)
}

// nextNomorUrutSuratKepsek returns the next sequential nomor urut for
// Surat Rekomendasi Kepala Sekolah letters issued by a given school
// (idUnitKerja) in a given year -- restarts from 1 each year, scoped PER
// SCHOOL so every school's numbering is independent of every other school's
// (mengikuti pola models.SuratRekomendasi.NomorUrut/Tahun).
func nextNomorUrutSuratKepsek(db *gorm.DB, idUnitKerja uint, tahun int) int {
	var maxUrut int
	db.Model(&models.PengajuanCuti{}).
		Joins("JOIN pegawai pgcek ON pgcek.id = pengajuan_cuti.id_pegawai").
		Where("pgcek.id_unit_kerja = ? AND pengajuan_cuti.tahun_surat_kepsek = ?", idUnitKerja, tahun).
		Select("COALESCE(MAX(pengajuan_cuti.nomor_urut_kepsek), 0)").Scan(&maxUrut)
	return maxUrut + 1
}

// buildSignatureQRKepsek meniru gaya buildSignatureQR di formulir.go (query
// Google Search berlabel jelas) tapi untuk konteks Surat Rekomendasi Kepala
// Sekolah -- ditandatangani Kepala Sekolah (snapshot item.KepsekNama dkk.),
// bukan Kepala Dinas.
func buildSignatureQRKepsek(item models.PengajuanCuti, pegawai models.Pegawai, nomorLengkap string) ([]byte, error) {
	jenisNama := "-"
	if item.JenisCuti != nil {
		jenisNama = item.JenisCuti.Jenis
	}
	tglDisetujui := "-"
	if item.TglApproval != nil {
		tglDisetujui = formatDateID(*item.TglApproval)
	}
	query := fmt.Sprintf(
		"Nama Pejabat: %s Jabatan: %s Merekomendasikan %s atas nama %s Nomor Surat %s Mulai %s sampai dengan %s Ditandatangani pada tanggal %s",
		namaOrDash(item.KepsekNama), namaOrDash(item.KepsekJabatan), jenisNama, pegawai.Nama, nomorLengkap,
		formatDateID(item.TglMulai), formatDateID(item.TglSelesai), tglDisetujui,
	)
	googleURL := "https://www.google.com/search?q=" + neturl.QueryEscape(query)
	return qrcode.Encode(googleURL, qrcode.Medium, 240)
}

// buildSuratRekomendasiKepsek draws the "Surat Rekomendasi Kepala Sekolah"
// PDF. pegawai MUST already be preloaded with Jabatan, UnitKerja and
// PangkatGol.Pangkat/PangkatGol.Gol (lihat downloadFormRekomendasiKepsek di
// bawah) -- item.KepsekNama dkk./NomorUrutKepsek/TahunSuratKepsek are the
// frozen snapshot from approvePengajuan; kosong (letter belum pernah
// disetujui sebagai pegawai sekolah) ditampilkan sebagai "-"/nomor kosong,
// sama seperti pola surat lain di aplikasi ini saat datanya belum lengkap.
func buildSuratRekomendasiKepsek(item models.PengajuanCuti, pegawai models.Pegawai) ([]byte, error) {
	doc := utils.NewPDFDoc()
	p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p)

	marginX := 42.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX
	centerX := marginX + (rightX-marginX)/2

	unitKerjaNama := "-"
	var uk *models.UnitKerja
	if pegawai.UnitKerja != nil {
		unitKerjaNama = pegawai.UnitKerja.Unit
		uk = pegawai.UnitKerja
	}
	kopBawahY := drawLetterheadUnitKerjaKustom(doc, p, marginX, rightX, unitKerjaNama, uk)

	const lineH = 15.0
	y := kopBawahY + 16

	p.SetFont(true, 13)
	const judul = "SURAT REKOMENDASI KEPALA SEKOLAH"
	p.TextCentered(centerX, y, judul)
	titleW := utils.TextWidth(judul, 13)
	p.Line(centerX-titleW/2, y+3, centerX+titleW/2, y+3)
	y += lineH

	singkatan := "-"
	if uk != nil && uk.Singkatan != nil && strings.TrimSpace(*uk.Singkatan) != "" {
		singkatan = *uk.Singkatan
	}
	tglSurat := time.Now()
	if item.TglApproval != nil {
		tglSurat = *item.TglApproval
	}
	nomorKosong := item.NomorUrutKepsek == 0
	var nomorLengkap string
	if nomorKosong {
		nomorLengkap = "......"
	} else {
		nomorLengkap = nomorSuratKepsekLengkap(item.NomorUrutKepsek, item.TahunSuratKepsek, tglSurat, singkatan)
	}
	p.SetFont(false, 12)
	p.TextCentered(centerX, y, "Nomor : "+nomorLengkap)
	y += lineH * 1.8

	const labelW = 150.0
	drawField := func(label, val string) {
		p.Text(marginX, y, label)
		p.Text(marginX+labelW, y, ": "+namaOrDash(val))
		y += lineH
	}

	p.Text(marginX, y, "Yang bertanda tangan di bawah ini:")
	y += lineH
	drawField("Nama", strings.ToUpper(namaOrDash(item.KepsekNama)))
	drawField("NIP", item.KepsekNip)
	drawField("Pangkat/Golongan", item.KepsekPangkatGol)
	jabatanKepsek := item.KepsekJabatan
	if strings.TrimSpace(jabatanKepsek) == "" {
		jabatanKepsek = "Kepala " + unitKerjaNama
	}
	drawField("Jabatan", jabatanKepsek)
	y += lineH * 0.6

	pembuka := "Dengan ini memberikan rekomendasi dan persetujuan prinsip kepada pegawai di bawah ini untuk mengajukan permohonan cuti:"
	y = p.MultilineText(marginX, y, rightX-marginX, lineH, pembuka) + lineH*0.4

	golNama := "-"
	if pegawai.PangkatGol != nil {
		pk, gol := "-", "-"
		if pegawai.PangkatGol.Pangkat != nil {
			pk = pegawai.PangkatGol.Pangkat.Pangkat
		}
		if pegawai.PangkatGol.Gol != nil {
			gol = pegawai.PangkatGol.Gol.Gol
		}
		golNama = pk + " / " + gol
	}
	jabatanNama := "-"
	if pegawai.Jabatan != nil {
		jabatanNama = pegawai.Jabatan.Jabatan
	}
	drawField("Nama", strings.ToUpper(namaOrDash(pegawai.Nama)))
	drawField("NIP", pegawai.NIP)
	drawField("Pangkat/Golongan", golNama)
	drawField("Jabatan", jabatanNama)
	drawField("Unit Kerja", unitKerjaNama)
	y += lineH * 0.6

	jenisNama := "-"
	if item.JenisCuti != nil {
		jenisNama = item.JenisCuti.Jenis
	}
	p.Text(marginX, y, "Rincian Pengajuan Cuti:")
	y += lineH
	drawField("Jenis Cuti", jenisNama)
	drawField("Alasan Cuti", item.AlasanCuti)
	drawField("Lama Cuti", fmt.Sprintf("%d Hari Kerja", item.JumlahHari))
	drawField("Terhitung Mulai Tanggal", formatTanggalRentang(item.TglMulai, item.TglSelesai))
	drawField("Alamat Selama Cuti", item.AlamatSelamaCuti)
	y += lineH * 0.6

	poin1 := "Pada prinsipnya, pihak sekolah tidak keberatan yang bersangkutan mengambil cuti tersebut karena tugas dan tanggung jawab pembelajaran/administrasi selama yang bersangkutan tidak hadir telah diatur dan dialihkan sementara kepada guru/pegawai pengganti sehingga kegiatan belajar mengajar tetap berjalan sebagaimana mestinya."
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, poin1) + lineH*0.4

	penutup := "Demikian surat rekomendasi ini dibuat dengan sebenarnya untuk digunakan sebagai kelengkapan administrasi pengajuan cuti pada Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara."
	y = p.JustifiedText(marginX, y, rightX-marginX, lineH, penutup) + lineH*1.8

	sigX := pageW - 270
	sigColW := rightX - sigX
	signerNamaDisp := truncateToWidth(strings.ToUpper(namaOrDash(item.KepsekNama)), sigColW*boldWidthSafety, 12)
	nameW := utils.TextWidth(signerNamaDisp, 12)

	p.SetFont(false, 12)
	p.Text(sigX, y, "Kolonodale, "+formatDateID(tglSurat)+".")
	y += lineH
	p.Text(sigX, y, "Kepala Sekolah,")
	y += lineH

	const qrSide = 110.0
	qrCenterX := sigX + nameW/2
	if qrCenterX-qrSide/2 < sigX {
		qrCenterX = sigX + qrSide/2
	}
	if qrCenterX+qrSide/2 > rightX {
		qrCenterX = rightX - qrSide/2
	}
	// QR tanda tangan HANYA ditampilkan kalau pengajuan ini sudah pernah
	// disetujui sebagai pegawai sekolah (NomorUrutKepsek != 0) -- sebelum
	// itu (pengajuan masih pending/belum pernah disetujui sama sekali),
	// ruang QR dikosongkan saja, sama seperti pola surat lain di aplikasi
	// ini saat datanya belum lengkap.
	if !nomorKosong && strings.TrimSpace(item.KepsekNip) != "" {
		if qrPng, err := buildSignatureQRKepsek(item, pegawai, nomorLengkap); err == nil {
			if err := doc.RegisterImage("ttd_qr_rekomendasi_kepsek", qrPng); err == nil {
				p.Image("ttd_qr_rekomendasi_kepsek", qrCenterX-qrSide/2, y+4, qrSide, qrSide)
			}
		}
	}
	y += qrSide + 18

	p.SetFont(true, 12)
	p.Text(sigX, y, signerNamaDisp)
	p.Line(sigX, y+3, sigX+nameW, y+3)
	y += 16
	p.SetFont(false, 12)
	p.Text(sigX, y, "NIP: "+namaOrDash(item.KepsekNip)+".")

	return doc.Output()
}

// downloadFormRekomendasiKepsek menangani GET
// /api/pengajuan-cuti/{id}/form/rekomendasi-kepsek -- BEDA dari
// downloadFormRekomendasi/downloadFormCuti (formulir.go): surat ini sudah
// bisa diunduh/dilihat begitu TAHAP 1 (Kepala Sekolah) selesai, status
// StatusDisetujuiKepsek ATAU StatusDisetuju (lihat
// loadKepsekReadyPengajuanForForm di formulir.go) -- TIDAK perlu menunggu
// TAHAP 2/Dinas selesai juga, karena surat ini murni rekomendasi Kepala
// Sekolah sendiri. Hanya untuk pegawai bertempat tugas Sekolah (yang punya
// Kepala Sekolah).
func downloadFormRekomendasiKepsek(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, pegawai, ok := loadKepsekReadyPengajuanForForm(w, r, db)
	if !ok {
		return
	}
	if !isSekolahPegawai(pegawai) {
		utils.Error(w, http.StatusBadRequest, "surat rekomendasi Kepala Sekolah hanya berlaku untuk pegawai bertempat tugas sekolah")
		return
	}
	var pegawaiFull models.Pegawai
	if err := db.Preload("Jabatan").Preload("UnitKerja").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
		First(&pegawaiFull, pegawai.ID).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memuat data pegawai")
		return
	}
	pdfBytes, err := buildSuratRekomendasiKepsek(item, pegawaiFull)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat surat: "+err.Error())
		return
	}
	writePDFResponse(w, r, pdfBytes, fmt.Sprintf("surat_rekomendasi_kepsek_%s.pdf", pegawaiFull.NIP))
}
