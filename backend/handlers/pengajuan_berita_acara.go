package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// pengajuan_berita_acara.go: pengajuan Berita Acara MANDIRI oleh pegawai
// bertugas di SEKOLAH (pegawai Dinas/Kantor TIDAK mengajukan lewat sini --
// Berita Acara mereka dibuat admin langsung lewat menu Berita Acara dengan
// penandatangan Kepala Dinas/Plt otomatis, lihat berita_acara.go) untuk SATU
// tanggal absensi yang kosong (tidak ada absen masuk & belum ada surat lain),
// dengan "Yang Mengetahui" = atasan langsung pegawai (Pegawai.IDAtasan) sesuai
// unit kerjanya -- BUKAN Kepala Dinas.
//
// Alur DUA TAHAP persetujuan (lihat models.PengajuanBeritaAcara &
// konstanta status di sana):
//
//  1. Pegawai mengajukan -> status "menunggu_atasan". PDF Berita Acara
//     langsung di-generate (reuse buildBeritaAcaraPDF, individu, signer =
//     atasan pegawai) TANPA QR -- bisa dilihat pegawai & atasan sebagai
//     draft/preview sebelum disetujui.
//  2. Atasan langsung menyetujui -> status "menunggu_admin" (PDF digenerate
//     ulang DENGAN QR tanda tangan atasan/Kepala Sekolah LANGSUNG tampil --
//     QR ini milik tanda tangan atasan sendiri, jadi TIDAK perlu menunggu
//     tahap akhir admin, SAMA pola dengan Surat Rekomendasi Sekolah -- DAN
//     nomor surat OTOMATIS dibuat & diisi, lihat
//     nextNomorUrutPengajuanBeritaAcara) ATAU mengembalikan ->
//     "dikembalikan_atasan" (pegawai edit & ajukan ulang, balik ke
//     "menunggu_atasan", QR & nomor surat hilang lagi sampai disetujui ulang).
//  3. Administrator/admin/akun ber-flag IsAdminAbsensi ATAU IsAdminVerifikasi
//     menyetujui -> status "disetujui": PDF digenerate ULANG, QR tanda
//     tangan atasan tetap disertakan, DAN baris AbsensiDokumen (jenis "berita_acara") otomatis
//     dibuat/diperbarui untuk tanggal kejadian ini -- inilah yang membuat
//     tanggal itu otomatis tercatat DD (Dinas Dalam) - Berita Acara di Rekap
//     Absen pegawai tsb, PERSIS seperti Berita Acara yang dibuat admin
//     langsung. ATAU menolak -> "ditolak" (status AKHIR/permanen, TIDAK bisa
//     diedit/diajukan ulang pegawai, lihat tolakPengajuanBeritaAcaraAdmin).
//     ATAU mengembalikan -> "dikembalikan_admin" (pegawai edit & ajukan ulang
//     LANGSUNG diteruskan lagi ke "menunggu_admin", TIDAK perlu persetujuan
//     atasan lagi, karena atasan sudah menyetujui & nomor surat otomatis
//     sudah ada sebelumnya).

// pengajuanBeritaAcaraOut: DTO respons API.
type pengajuanBeritaAcaraOut struct {
	ID              uint            `json:"id"`
	IDPegawai       uint            `json:"id_pegawai"`
	Pegawai         *models.Pegawai `json:"pegawai,omitempty"`
	TanggalKejadian string          `json:"tanggal_kejadian"`
	Alasan          string          `json:"alasan"`
	NomorSurat      string          `json:"nomor_surat"`
	Status          string          `json:"status"`
	NamaFile        string          `json:"nama_file"`
	// AdaBuktiDukung: true kalau pengajuan ini punya lampiran bukti dukung --
	// SELALU true untuk pengajuan baru (wajib diupload, lihat
	// buatPengajuanBeritaAcara), hanya pengajuan lama (sebelum fitur ini ada)
	// yang bisa bernilai false.
	AdaBuktiDukung    bool      `json:"ada_bukti_dukung"`
	AtasanApproveNama string    `json:"atasan_approve_nama,omitempty"`
	TglAtasanApprove  string    `json:"tgl_atasan_approve,omitempty"`
	CatatanAtasan     string    `json:"catatan_atasan"`
	AdminApproveNama  string    `json:"admin_approve_nama,omitempty"`
	TglAdminApprove   string    `json:"tgl_admin_approve,omitempty"`
	CatatanAdmin      string    `json:"catatan_admin"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func toPengajuanBeritaAcaraOut(item models.PengajuanBeritaAcara) pengajuanBeritaAcaraOut {
	out := pengajuanBeritaAcaraOut{
		ID:              item.ID,
		IDPegawai:       item.IDPegawai,
		Pegawai:         item.Pegawai,
		TanggalKejadian: item.TanggalKejadian.Format("2006-01-02"),
		Alasan:          item.Alasan,
		Status:          item.Status,
		NamaFile:        item.NamaFile,
		AdaBuktiDukung:  strings.TrimSpace(item.BuktiDukungNamaFile) != "",
		CatatanAtasan:   item.CatatanAtasan,
		CatatanAdmin:    item.CatatanAdmin,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
	if item.NomorSurat != nil {
		out.NomorSurat = *item.NomorSurat
	}
	if item.AtasanApprove != nil {
		out.AtasanApproveNama = item.AtasanApprove.Nama
	}
	if item.TglAtasanApprove != nil {
		out.TglAtasanApprove = item.TglAtasanApprove.Format(time.RFC3339)
	}
	if item.AdminApprove != nil {
		out.AdminApproveNama = item.AdminApprove.Nama
	}
	if item.TglAdminApprove != nil {
		out.TglAdminApprove = item.TglAdminApprove.Format(time.RFC3339)
	}
	return out
}

func pengajuanBeritaAcaraPreload(db *gorm.DB) *gorm.DB {
	return db.Omit("file", "bukti_dukung_file").
		Preload("Pegawai.Jabatan").Preload("Pegawai.UnitKerja").Preload("Pegawai.PangkatGol.Pangkat").Preload("Pegawai.PangkatGol.Gol").
		Preload("Pegawai.Atasan").Preload("Pegawai.Atasan.Jabatan").Preload("Pegawai.Atasan.UnitKerja").
		Preload("Pegawai.Atasan.PangkatGol.Pangkat").Preload("Pegawai.Atasan.PangkatGol.Gol").
		Preload("AtasanApprove").Preload("AdminApprove")
}

// canApproveFinalPengajuanBeritaAcara: tahap akhir boleh disetujui/
// dikembalikan oleh administrator, admin, ATAU akun mana pun ber-flag
// IsAdminAbsensi ATAU IsAdminVerifikasi -- sesuai permintaan pengguna
// (berbeda dari verifikasi Pengajuan Surat Kolektif yang HANYA administrator/
// IsAdminVerifikasi, TIDAK termasuk role "admin" polos maupun IsAdminAbsensi).
func canApproveFinalPengajuanBeritaAcara(claims *utils.Claims) bool {
	return claims != nil && (claims.RoleName == "administrator" || claims.RoleName == "admin" || claims.IsAdminAbsensi || claims.IsAdminVerifikasi)
}

// buildPengajuanBeritaAcaraPDF menggambar ulang PDF Berita Acara individu
// untuk SATU pengajuan, dengan signer = atasan langsung pegawai (kop surat
// memakai Unit Kerja/sekolah pegawai sendiri, sama seperti pola Lampiran 3
// Surat Rekomendasi -- lihat buildSuratRekomendasiSekolah) -- tampilkanQR
// true begitu atasan/Kepala Sekolah menyetujui tahap pertama (lihat
// setujuiPengajuanBeritaAcaraAtasan), TIDAK perlu menunggu tahap akhir admin
// -- tanda tangan QR di baris "Mengetahui, Kepala Sekolah" ini MILIK
// atasan/Kepala Sekolah sendiri, jadi ditampilkan begitu merekalah yang
// menyetujui, SAMA pola dengan QR pada Surat Rekomendasi Sekolah (lihat
// doApprove di ArsipSuratView.vue, approveSuratRekomendasi).
func buildPengajuanBeritaAcaraPDF(pegawai models.Pegawai, tglKejadian time.Time, alasan, nomorSurat string, tampilkanQR bool, buktiDukungData []byte, buktiDukungContentType, buktiDukungNamaFile string) ([]byte, error) {
	// signerJabatan SENGAJA selalu "Kepala Sekolah" -- TIDAK memakai jabatan
	// FORMAL atasan (models.Pegawai.Jabatan, mis. "Guru Ahli Madya" kalau
	// Kepala Sekolah yang bersangkutan jabatan struktural aslinya guru) --
	// baris "Yang Mengetahui" di Berita Acara ini harus selalu menyebut
	// "Kepala Sekolah" sesuai permintaan pengguna, karena itulah PERAN atasan
	// langsung pegawai sekolah di alur ini, bukan jabatan formalnya.
	signerNama, signerNip, signerJabatan, signerPangkatGol := "-", "-", "Kepala Sekolah", "-"
	if pegawai.Atasan != nil {
		a := pegawai.Atasan
		signerNama = a.Nama
		signerNip = a.NIP
		signerPangkatGol = pegawaiPangkatGolText(*a)
	}
	unitKerjaNama := "-"
	var unitKerjaKop *models.UnitKerja
	if pegawai.UnitKerja != nil {
		unitKerjaNama = pegawai.UnitKerja.Unit
		unitKerjaKop = pegawai.UnitKerja
	}
	signer := beritaAcaraSigner{
		Nama: signerNama, NIP: signerNip, Jabatan: signerJabatan, PangkatGol: signerPangkatGol,
		UnitKerja: unitKerjaNama, UnitKerjaKop: unitKerjaKop, TampilkanQR: tampilkanQR,
	}
	return buildBeritaAcaraPDF("individu", []models.Pegawai{pegawai}, signer, tglKejadian, tglKejadian, nomorSurat, alasan, buktiDukungData, buktiDukungContentType, buktiDukungNamaFile)
}

// tanggalPengajuanBeritaAcaraValid memvalidasi tanggal kejadian yang
// diajukan: hari kerja pegawai tsb, bukan tanggal merah, belum ada absen
// masuk sungguhan, belum ada AbsensiDokumen, dan belum ada pengajuan Berita
// Acara LAIN yang masih berjalan (status "disetujui" ATAU "ditolak" dianggap
// SUDAH SELESAI/tidak berjalan lagi -- "ditolak" bersifat akhir & permanen,
// jadi TIDAK boleh menghalangi pegawai mengajukan Berita Acara baru untuk
// tanggal yang sama) untuk tanggal yang sama -- memakai basis pengecekan
// yang SAMA dengan tanggalTerlewatValid (pengajuan_surat_kolektif.go) supaya
// definisinya konsisten di seluruh aplikasi, ditambah pengecekan KHUSUS
// terhadap tabel PengajuanBeritaAcara sendiri.
func tanggalPengajuanBeritaAcaraValid(db *gorm.DB, pegawai models.Pegawai, tanggal time.Time, excludeID uint) (bool, string) {
	if ok, reason := tanggalTerlewatValid(db, pegawai, tanggal, 0); !ok {
		return false, reason
	}
	var rows []models.PengajuanBeritaAcara
	db.Where("id_pegawai = ? AND tanggal_kejadian = ? AND status NOT IN ?", pegawai.ID, tanggal,
		[]string{models.PengajuanBeritaAcaraDisetujui, models.PengajuanBeritaAcaraDitolak}).Find(&rows)
	for _, row := range rows {
		if row.ID == excludeID {
			continue
		}
		return false, "sudah ada pengajuan Berita Acara lain yang masih berjalan untuk tanggal ini"
	}
	return true, ""
}

// buatPengajuanBeritaAcara menangani POST /api/pengajuan-berita-acara --
// pegawai bertugas di SEKOLAH mengajukan Berita Acara mandiri untuk SATU
// tanggal kejadian. Menerima multipart/form-data {tanggal_kejadian, alasan,
// file} -- field "file" (bukti dukung foto/scan pendukung alasan terpilih)
// WAJIB diupload, sesuai permintaan pengguna (lihat parseBuktiDukungUpload
// di berita_acara.go).
func buatPengajuanBeritaAcara(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}
	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").Preload("Jabatan").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
		Preload("Atasan").Preload("Atasan.Jabatan").Preload("Atasan.UnitKerja").
		Preload("Atasan.PangkatGol.Pangkat").Preload("Atasan.PangkatGol.Gol").
		First(&pegawai, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}
	if !isSekolahPegawai(pegawai) {
		utils.Error(w, http.StatusForbidden,
			"pengajuan Berita Acara mandiri hanya untuk pegawai bertugas di sekolah -- pegawai dengan tempat tugas dinas/kantor tidak bisa mengajukan sendiri, Berita Acara untuk anda hanya bisa dibuat oleh administrator/admin lewat menu Berita Acara.")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	alasan := strings.TrimSpace(r.FormValue("alasan"))
	if !isAlasanBeritaAcaraValid(alasan) {
		utils.Error(w, http.StatusBadRequest, "alasan tidak valid -- pilih salah satu dari daftar yang tersedia")
		return
	}
	tglKejadian, err := utils.ParseDateCell(strings.TrimSpace(r.FormValue("tanggal_kejadian")))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal kejadian tidak valid")
		return
	}
	if ok, reason := tanggalPengajuanBeritaAcaraValid(db, pegawai, tglKejadian, 0); !ok {
		utils.Error(w, http.StatusBadRequest, "tanggal kejadian tidak bisa diajukan: "+reason)
		return
	}

	buktiNamaFile, buktiContentType, buktiFileData, errMsg := parseBuktiDukungUpload(r)
	if errMsg != "" {
		utils.Error(w, http.StatusBadRequest, errMsg)
		return
	}

	pdfBytes, err := buildPengajuanBeritaAcaraPDF(pegawai, tglKejadian, alasan, "", false, buktiFileData, buktiContentType, buktiNamaFile)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat berkas PDF: "+err.Error())
		return
	}

	item := models.PengajuanBeritaAcara{
		IDPegawai:              pegawai.ID,
		TanggalKejadian:        tglKejadian,
		Alasan:                 alasan,
		Status:                 models.PengajuanBeritaAcaraMenungguAtasan,
		NamaFile:               generateBeritaAcaraNamaFile(tglKejadian),
		File:                   pdfBytes,
		BuktiDukungNamaFile:    buktiNamaFile,
		BuktiDukungFile:        buktiFileData,
		BuktiDukungContentType: buktiContentType,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	utils.Created(w, "pengajuan Berita Acara berhasil dikirim, menunggu persetujuan atasan", nil)
}

// updatePengajuanBeritaAcara menangani PUT /api/pengajuan-berita-acara/{id}
// -- pegawai pemilik mengedit & mengajukan ulang pengajuannya sendiri yang
// berstatus "dikembalikan_atasan" ATAU "dikembalikan_admin" (keduanya balik
// ke "menunggu_atasan" lagi).
func updatePengajuanBeritaAcara(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.IDPegawai != *claims.IDPegawai {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke pengajuan ini")
		return
	}
	if item.Status != models.PengajuanBeritaAcaraDikembalikanAtasan && item.Status != models.PengajuanBeritaAcaraDikembalikanAdmin {
		utils.Error(w, http.StatusBadRequest, "hanya pengajuan yang dikembalikan yang boleh diedit & diajukan ulang")
		return
	}
	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").Preload("Jabatan").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
		Preload("Atasan").Preload("Atasan.Jabatan").Preload("Atasan.UnitKerja").
		Preload("Atasan.PangkatGol.Pangkat").Preload("Atasan.PangkatGol.Gol").
		First(&pegawai, item.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	alasan := strings.TrimSpace(r.FormValue("alasan"))
	if !isAlasanBeritaAcaraValid(alasan) {
		utils.Error(w, http.StatusBadRequest, "alasan tidak valid -- pilih salah satu dari daftar yang tersedia")
		return
	}
	tglKejadian, err := utils.ParseDateCell(strings.TrimSpace(r.FormValue("tanggal_kejadian")))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal kejadian tidak valid")
		return
	}
	if ok, reason := tanggalPengajuanBeritaAcaraValid(db, pegawai, tglKejadian, item.ID); !ok {
		utils.Error(w, http.StatusBadRequest, "tanggal kejadian tidak bisa diajukan: "+reason)
		return
	}

	// Bukti dukung boleh TIDAK dikirim ulang saat edit -- berkas lama
	// dipertahankan (SAMA pola dengan updatePengajuanSuratKolektif), berkas
	// baru menggantikan yang lama kalau memang diupload ulang. Pengajuan
	// pertama kali (buatPengajuanBeritaAcara) TETAP mewajibkan upload --
	// hanya di alur edit & ajukan ulang ini yang boleh dikosongkan.
	if formFileHeader(r, "file") != nil {
		buktiNamaFile, buktiContentType, buktiFileData, errMsg := parseBuktiDukungUpload(r)
		if errMsg != "" {
			utils.Error(w, http.StatusBadRequest, errMsg)
			return
		}
		item.BuktiDukungNamaFile = buktiNamaFile
		item.BuktiDukungFile = buktiFileData
		item.BuktiDukungContentType = buktiContentType
	}

	// statusSebelumnya menentukan tujuan pengajuan ulang: KHUSUS yang
	// dikembalikan ADMIN (tahap akhir), atasan SUDAH menyetujui sebelumnya --
	// jadi pengajuan ulang LANGSUNG diteruskan lagi ke verifikasi admin (skip
	// atasan), nomor surat otomatis yang sudah ada tetap dipakai, sesuai
	// permintaan pengguna. Yang dikembalikan ATASAN tetap balik ke menunggu
	// persetujuan atasan seperti semula (nomor surat belum pernah dibuat pada
	// tahap ini, lihat setujuiPengajuanBeritaAcaraAtasan).
	statusSebelumnya := item.Status
	langsungKeAdmin := statusSebelumnya == models.PengajuanBeritaAcaraDikembalikanAdmin

	// nomorUntukPdf & tampilkanQR: KHUSUS yang langsung ke admin (atasan
	// sudah menyetujui, tanda tangan QR-nya MASIH berlaku, nomor surat
	// otomatis yang sudah ada tetap dipakai) -- yang balik ke atasan HARUS
	// menunggu atasan menyetujui ULANG dulu (data tanggal/alasan berubah),
	// jadi QR & nomor surat lama TIDAK ditampilkan sampai atasan menyetujui
	// kembali lewat setujuiPengajuanBeritaAcaraAtasan.
	nomorUntukPdf := ""
	tampilkanQR := false
	if langsungKeAdmin {
		tampilkanQR = true
		if item.NomorSurat != nil {
			nomorUntukPdf = *item.NomorSurat
		}
	}
	pdfBytes, err := buildPengajuanBeritaAcaraPDF(pegawai, tglKejadian, alasan, nomorUntukPdf, tampilkanQR, item.BuktiDukungFile, item.BuktiDukungContentType, item.BuktiDukungNamaFile)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat berkas PDF: "+err.Error())
		return
	}

	item.TanggalKejadian = tglKejadian
	item.Alasan = alasan
	item.NamaFile = generateBeritaAcaraNamaFile(tglKejadian)
	item.File = pdfBytes
	if langsungKeAdmin {
		item.Status = models.PengajuanBeritaAcaraMenungguAdmin
		item.CatatanAdmin = ""
		item.IDAdminApprove = nil
		item.TglAdminApprove = nil
	} else {
		item.Status = models.PengajuanBeritaAcaraMenungguAtasan
		item.CatatanAtasan = ""
		item.IDAtasanApprove = nil
		item.TglAtasanApprove = nil
		item.CatatanAdmin = ""
		item.IDAdminApprove = nil
		item.TglAdminApprove = nil
		item.NomorSurat = nil
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	pesan := "pengajuan berhasil diperbarui & dikirim ulang, menunggu persetujuan atasan"
	if langsungKeAdmin {
		pesan = "pengajuan berhasil diperbarui & dikirim ulang, langsung diteruskan ke verifikasi administrator/admin (tidak perlu persetujuan atasan lagi)"
	}
	utils.Success(w, pesan, nil)
}

// listPengajuanBeritaAcaraSaya menangani GET
// /api/pengajuan-berita-acara/saya -- daftar pengajuan milik pegawai yang
// login sendiri (semua status).
func listPengajuanBeritaAcaraSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", []pengajuanBeritaAcaraOut{})
		return
	}
	var items []models.PengajuanBeritaAcara
	pengajuanBeritaAcaraPreload(db).Where("id_pegawai = ?", *claims.IDPegawai).Order("created_at desc").Find(&items)
	out := make([]pengajuanBeritaAcaraOut, 0, len(items))
	for _, it := range items {
		out = append(out, toPengajuanBeritaAcaraOut(it))
	}
	utils.Success(w, "ok", out)
}

// listPengajuanBeritaAcaraBawahan menangani GET
// /api/pengajuan-berita-acara/bawahan -- daftar pengajuan SELURUH bawahan
// langsung akun atasan yang login (default hanya "menunggu_atasan",
// ?status=semua untuk semua status, dipakai menampilkan riwayat).
func listPengajuanBeritaAcaraBawahan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", []pengajuanBeritaAcaraOut{})
		return
	}
	query := pengajuanBeritaAcaraPreload(db).
		Joins("JOIN pegawai ON pegawai.id = pengajuan_berita_acara.id_pegawai").
		Where("pegawai.id_atasan = ?", *claims.IDPegawai)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "semua" {
		query = query.Where("pengajuan_berita_acara.status = ?", status)
	} else if status == "" {
		query = query.Where("pengajuan_berita_acara.status = ?", models.PengajuanBeritaAcaraMenungguAtasan)
	}
	var items []models.PengajuanBeritaAcara
	query.Order("pengajuan_berita_acara.created_at desc").Find(&items)
	out := make([]pengajuanBeritaAcaraOut, 0, len(items))
	for _, it := range items {
		out = append(out, toPengajuanBeritaAcaraOut(it))
	}
	utils.Success(w, "ok", out)
}

// listPengajuanBeritaAcaraAdmin menangani GET /api/pengajuan-berita-acara --
// daftar SEMUA pengajuan yang sudah disetujui atasan (default hanya
// "menunggu_admin", ?status=semua untuk semua status) untuk tahap akhir
// administrator/admin/IsAdminAbsensi/IsAdminVerifikasi.
func listPengajuanBeritaAcaraAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	query := pengajuanBeritaAcaraPreload(db)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "semua" {
		query = query.Where("status = ?", status)
	} else if status == "" {
		query = query.Where("status = ?", models.PengajuanBeritaAcaraMenungguAdmin)
	}
	var items []models.PengajuanBeritaAcara
	query.Order("created_at desc").Find(&items)
	out := make([]pengajuanBeritaAcaraOut, 0, len(items))
	for _, it := range items {
		out = append(out, toPengajuanBeritaAcaraOut(it))
	}
	utils.Success(w, "ok", out)
}

// nomorSuratPengajuanBeritaAcaraLengkap merangkai nomor surat LENGKAP khusus
// Berita Acara Sekolah mandiri ini -- BEDA dari nomorSuratBeritaAcaraLengkap
// (berita_acara.go, dipakai menu Berita Acara admin langsung, penandatangan
// Kepala Dinas) dalam SATU hal: segmen "Disdikbud" diganti singkatan Unit
// Kerja (sekolah) pegawai sendiri, SAMA pola dengan
// nomorSuratRekomendasiSekolahLengkap (surat_rekomendasi.go) -- karena
// Berita Acara Sekolah ditandatangani Kepala Sekolah ("Yang Mengetahui"),
// BUKAN Kepala Dinas, jadi "Disdikbud" tidak relevan di sini. Singkatan
// diisi manual admin di menu Unit Kerja (models.UnitKerja.Singkatan),
// tampil "-" kalau belum diisi.
func nomorSuratPengajuanBeritaAcaraLengkap(urutan string, tglSurat time.Time, singkatanUnitKerja string) string {
	return fmt.Sprintf("800/%s/%s/%s/%d", urutan, singkatanUnitKerja, romanMonth(tglSurat.Month()), tglSurat.Year())
}

// singkatanUnitKerjaPegawai: singkatan Unit Kerja (sekolah) pegawai, "-"
// kalau belum diisi admin -- lihat models.UnitKerja.Singkatan.
func singkatanUnitKerjaPegawai(pegawai models.Pegawai) string {
	if pegawai.UnitKerja != nil && pegawai.UnitKerja.Singkatan != nil && strings.TrimSpace(*pegawai.UnitKerja.Singkatan) != "" {
		return strings.TrimSpace(*pegawai.UnitKerja.Singkatan)
	}
	return "-"
}

// nomorUrutPengajuanBARegex mencocokkan nomor surat LENGKAP hasil
// nomorSuratPengajuanBeritaAcaraLengkap di atas, mis. "800/1/SDN-LMR/X/2026"
// -- grup 1 = nomor urut, grup 2 = tahun. Segmen singkatan Unit Kerja
// SENGAJA dicocokkan bebas ([^/]+, bisa "-" atau apa pun) karena berbeda-
// beda per sekolah, bukan string tetap seperti "Disdikbud" pada nomor
// Berita Acara admin langsung.
var nomorUrutPengajuanBARegex = regexp.MustCompile(`^800/(\d+)/[^/]+/[IVXLCDM]+/(\d{4})$`)

// nextNomorUrutPengajuanBeritaAcara menghitung nomor urut berikutnya untuk
// penomoran OTOMATIS Berita Acara Sekolah (diisi begitu atasan/Kepala
// Sekolah menyetujui tahap pertama, lihat setujuiPengajuanBeritaAcaraAtasan),
// sesuai permintaan pengguna ("kepala sekolah menyetujui dan memuat no
// otomatis"). SENGAJA memakai urutan TERPISAH dari nomor Berita Acara yang
// diinput admin langsung lewat menu Berita Acara (AbsensiDokumen.Nomor,
// bebas format non-numerik seperti "483.1" -- tidak aman diparse sebagai
// angka), supaya penomoran di sini selalu bisa diparse murni sebagai angka:
// cari nomor urut TERBESAR pada PengajuanBeritaAcara.NomorSurat tahun yang
// sama (mengikuti tahun tanggal kejadian, basis yang SAMA dipakai
// nomorSuratBeritaAcaraLengkap di sini), lalu +1.
func nextNomorUrutPengajuanBeritaAcara(db *gorm.DB, tahun int) (string, error) {
	var nomorList []string
	if err := db.Model(&models.PengajuanBeritaAcara{}).
		Where("nomor_surat IS NOT NULL").
		Pluck("nomor_surat", &nomorList).Error; err != nil {
		return "", err
	}
	maxUrut := 0
	for _, s := range nomorList {
		m := nomorUrutPengajuanBARegex.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		y, err := strconv.Atoi(m[2])
		if err != nil || y != tahun {
			continue
		}
		u, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if u > maxUrut {
			maxUrut = u
		}
	}
	return strconv.Itoa(maxUrut + 1), nil
}

// setujuiPengajuanBeritaAcaraAtasan menangani PUT
// /api/pengajuan-berita-acara/{id}/setujui-atasan -- atasan langsung pegawai
// menyetujui tahap pertama (status -> "menunggu_admin", PDF digenerate ulang
// DENGAN QR tanda tangan atasan/Kepala Sekolah LANGSUNG tampil -- SAMA pola
// dengan Surat Rekomendasi Sekolah, QR tidak perlu menunggu tahap akhir
// admin karena itu MILIK tanda tangan atasan sendiri -- DAN nomor surat
// OTOMATIS dibuat & diisi kalau belum ada, lihat
// nextNomorUrutPengajuanBeritaAcara).
func setujuiPengajuanBeritaAcaraAtasan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	if err := db.Preload("Pegawai.UnitKerja").Preload("Pegawai.Jabatan").Preload("Pegawai.PangkatGol.Pangkat").Preload("Pegawai.PangkatGol.Gol").
		Preload("Pegawai.Atasan").Preload("Pegawai.Atasan.Jabatan").Preload("Pegawai.Atasan.UnitKerja").
		Preload("Pegawai.Atasan.PangkatGol.Pangkat").Preload("Pegawai.Atasan.PangkatGol.Gol").
		First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Pegawai == nil || item.Pegawai.IDAtasan == nil || claims.IDPegawai == nil || *item.Pegawai.IDAtasan != *claims.IDPegawai {
		utils.Error(w, http.StatusForbidden, "anda bukan atasan langsung pegawai ini")
		return
	}
	if item.Status != models.PengajuanBeritaAcaraMenungguAtasan {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}

	// Nomor surat OTOMATIS: begitu atasan/Kepala Sekolah menyetujui, sistem
	// langsung merangkai nomor urut berikutnya TANPA perlu diketik siapa pun.
	// Kalau item.NomorSurat SUDAH terisi (semestinya tidak terjadi normal di
	// endpoint ini, tapi dijaga untuk keamanan), nomor LAMA dipertahankan,
	// TIDAK dibuat ulang.
	if item.NomorSurat == nil {
		urutan, numErr := nextNomorUrutPengajuanBeritaAcara(db, item.TanggalKejadian.Year())
		if numErr != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal membuat nomor surat otomatis: "+numErr.Error())
			return
		}
		nomorBaru := nomorSuratPengajuanBeritaAcaraLengkap(urutan, item.TanggalKejadian, singkatanUnitKerjaPegawai(*item.Pegawai))
		item.NomorSurat = &nomorBaru
	}
	nomor := *item.NomorSurat
	// tampilkanQR=true -- tanda tangan QR atasan/Kepala Sekolah LANGSUNG
	// tampil begitu mereka menyetujui (lihat komentar buildPengajuanBeritaAcaraPDF).
	pdfBytes, err := buildPengajuanBeritaAcaraPDF(*item.Pegawai, item.TanggalKejadian, item.Alasan, nomor, true, item.BuktiDukungFile, item.BuktiDukungContentType, item.BuktiDukungNamaFile)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat berkas PDF: "+err.Error())
		return
	}

	now := absensiNow()
	item.Status = models.PengajuanBeritaAcaraMenungguAdmin
	item.IDAtasanApprove = claims.IDPegawai
	item.TglAtasanApprove = &now
	item.CatatanAtasan = strings.TrimSpace(r.FormValue("catatan"))
	item.File = pdfBytes
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan persetujuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan disetujui, diteruskan ke administrator/admin untuk persetujuan tahap akhir", nil)
}

// kembalikanPengajuanBeritaAcaraAtasan menangani PUT
// /api/pengajuan-berita-acara/{id}/kembalikan-atasan -- atasan mengembalikan
// pengajuan untuk direvisi pegawai (catatan wajib diisi).
func kembalikanPengajuanBeritaAcaraAtasan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Pegawai == nil || item.Pegawai.IDAtasan == nil || claims.IDPegawai == nil || *item.Pegawai.IDAtasan != *claims.IDPegawai {
		utils.Error(w, http.StatusForbidden, "anda bukan atasan langsung pegawai ini")
		return
	}
	if item.Status != models.PengajuanBeritaAcaraMenungguAtasan {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}
	catatan := strings.TrimSpace(r.FormValue("catatan"))
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya pegawai tahu apa yang perlu diperbaiki")
		return
	}
	now := absensiNow()
	item.Status = models.PengajuanBeritaAcaraDikembalikanAtasan
	item.CatatanAtasan = catatan
	item.IDAtasanApprove = claims.IDPegawai
	item.TglAtasanApprove = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan dikembalikan ke pegawai untuk direvisi", nil)
}

// setujuiPengajuanBeritaAcaraAdmin menangani PUT
// /api/pengajuan-berita-acara/{id}/setujui-admin -- tahap akhir, administrator/
// admin/IsAdminAbsensi/IsAdminVerifikasi menyetujui: PDF digenerate ulang
// DENGAN QR, baris AbsensiDokumen (jenis "berita_acara") otomatis dibuat/
// diperbarui untuk tanggal kejadian ini -- inilah yang membuat tanggal itu
// tercatat DD (Dinas Dalam) - Berita Acara di Rekap Absen pegawai tsb.
// Menerima form-urlencoded/multipart opsional "nomor_surat" & "catatan".
func setujuiPengajuanBeritaAcaraAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	if err := db.Preload("Pegawai.UnitKerja").Preload("Pegawai.Jabatan").Preload("Pegawai.PangkatGol.Pangkat").Preload("Pegawai.PangkatGol.Gol").
		Preload("Pegawai.Atasan").Preload("Pegawai.Atasan.Jabatan").Preload("Pegawai.Atasan.UnitKerja").
		Preload("Pegawai.Atasan.PangkatGol.Pangkat").Preload("Pegawai.Atasan.PangkatGol.Gol").
		First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Pegawai == nil {
		utils.Error(w, http.StatusInternalServerError, "data pegawai pengajuan ini tidak ditemukan")
		return
	}
	if item.Status != models.PengajuanBeritaAcaraMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya, atau belum disetujui atasan (status: "+item.Status+")")
		return
	}

	// nomorSurat: SUDAH terisi otomatis sejak atasan/Kepala Sekolah menyetujui
	// (lihat setujuiPengajuanBeritaAcaraAtasan) -- admin tahap akhir di sini
	// HANYA perlu mengetik nomor urut kalau mau MENGOREKSI/mengubah nomor
	// yang sudah ada (opsional), dirangkai otomatis jadi format baku
	// "800/{urutan}/{singkatan unit kerja sekolah}/{bulan romawi}/{tahun}"
	// lewat nomorSuratPengajuanBeritaAcaraLengkap di atas (BEDA dari
	// nomorSuratBeritaAcaraLengkap di berita_acara.go yang dipakai menu
	// Berita Acara admin langsung -- di sana "Disdikbud" karena
	// penandatangannya Kepala Dinas, di sini singkatan sekolah karena
	// penandatangannya Kepala Sekolah). Kalau field "nomor_surat"
	// dikosongkan, nomor otomatis yang sudah tersimpan di item.NomorSurat
	// TETAP dipakai, TIDAK dikosongkan (bug lama: form kosong dulu menghapus
	// nomor otomatis yang sudah ada).
	nomorSurat := ""
	if item.NomorSurat != nil {
		nomorSurat = *item.NomorSurat
	}
	if manual := strings.TrimSpace(r.FormValue("nomor_surat")); manual != "" {
		nomorSurat = nomorSuratPengajuanBeritaAcaraLengkap(manual, item.TanggalKejadian, singkatanUnitKerjaPegawai(*item.Pegawai))
	}
	pdfBytes, err := buildPengajuanBeritaAcaraPDF(*item.Pegawai, item.TanggalKejadian, item.Alasan, nomorSurat, true, item.BuktiDukungFile, item.BuktiDukungContentType, item.BuktiDukungNamaFile)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat berkas PDF: "+err.Error())
		return
	}

	// Jenis Surat "berita_acara" HARUS sudah ada di master (seed bawaan, lihat
	// database/migrate.go) -- sama seperti buatBeritaAcara (menu admin).
	var jenisSurat models.JenisSurat
	if err := db.Where("slug = ?", models.AbsensiDokumenBeritaAcara).First(&jenisSurat).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "master Jenis Surat \"Berita Acara\" tidak ditemukan -- hubungi pengembang aplikasi")
		return
	}

	// Pengaman sama seperti buatBeritaAcara/setujuiPengajuanSuratKolektif:
	// kalau antara pengajuan & persetujuan tahap akhir ternyata pegawai sudah
	// tercatat absen masuk sungguhan pada tanggal ini, baris AbsensiDokumen
	// TIDAK ditimpa (tapi pengajuan tetap ditandai disetujui & PDF tetap
	// disimpan, supaya tidak mengganjal alur -- hanya baris Rekap Absen yang
	// dilewati, dengan peringatan pada pesan respons).
	var absensi models.Absensi
	sudahHadir := db.Where("id_pegawai = ? AND tanggal = ?", item.IDPegawai, item.TanggalKejadian).First(&absensi).Error == nil && absensi.JamMasuk != nil

	if !sudahHadir {
		var existing models.AbsensiDokumen
		found := db.Where("id_pegawai = ? AND tanggal = ?", item.IDPegawai, item.TanggalKejadian).First(&existing).Error == nil
		existing.IDPegawai = item.IDPegawai
		existing.Tanggal = item.TanggalKejadian
		existing.Jenis = models.AbsensiDokumenBeritaAcara
		existing.Label = jenisSurat.Nama
		existing.NamaFile = item.NamaFile
		existing.File = pdfBytes
		existing.Keterangan = item.Alasan
		if nomorSurat != "" {
			existing.Nomor = &nomorSurat
		}
		userID := claims.UserID
		existing.IDDiinputOleh = &userID
		if found {
			db.Save(&existing)
		} else {
			existing.ID = 0
			db.Create(&existing)
		}
	}

	now := absensiNow()
	item.Status = models.PengajuanBeritaAcaraDisetujui
	item.IDAdminApprove = &claims.UserID
	item.TglAdminApprove = &now
	item.CatatanAdmin = strings.TrimSpace(r.FormValue("catatan"))
	item.File = pdfBytes
	if nomorSurat != "" {
		item.NomorSurat = &nomorSurat
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan persetujuan: "+err.Error())
		return
	}

	pesan := "pengajuan Berita Acara disetujui -- otomatis tercatat DD (Dinas Dalam) - Berita Acara pada Rekap Absen pegawai"
	if sudahHadir {
		pesan = "pengajuan disetujui, TAPI Rekap Absen TIDAK diperbarui karena pegawai sudah tercatat absen masuk (hadir) sungguhan pada tanggal ini"
	}
	utils.Success(w, pesan, nil)
}

// kembalikanPengajuanBeritaAcaraAdmin menangani PUT
// /api/pengajuan-berita-acara/{id}/kembalikan-admin -- tahap akhir
// mengembalikan pengajuan untuk direvisi pegawai (catatan wajib diisi, balik
// ke "menunggu_atasan" lagi begitu pegawai mengedit & mengajukan ulang).
func kembalikanPengajuanBeritaAcaraAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Status != models.PengajuanBeritaAcaraMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya, atau belum disetujui atasan (status: "+item.Status+")")
		return
	}
	catatan := strings.TrimSpace(r.FormValue("catatan"))
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya pegawai tahu apa yang perlu diperbaiki")
		return
	}
	now := absensiNow()
	item.Status = models.PengajuanBeritaAcaraDikembalikanAdmin
	item.CatatanAdmin = catatan
	item.IDAdminApprove = &claims.UserID
	item.TglAdminApprove = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan dikembalikan ke pegawai untuk direvisi", nil)
}

// tolakPengajuanBeritaAcaraAdmin menangani PUT
// /api/pengajuan-berita-acara/{id}/tolak-admin -- tahap akhir MENOLAK
// pengajuan secara PERMANEN (beda dari "kembalikan" yang masih memberi
// pegawai kesempatan merevisi & mengajukan ulang), sesuai permintaan
// pengguna ("admin bisa menyetujui, menolak dan mengembalikan BA"). Catatan
// alasan penolakan wajib diisi, TIDAK ada AbsensiDokumen yang dibuat/
// diperbarui, dan status "ditolak" ini bersifat akhir: pegawai TIDAK bisa
// mengedit/mengajukan ulang pengajuan ini (lihat updatePengajuanBeritaAcara
// yang hanya menerima status "dikembalikan_atasan"/"dikembalikan_admin"),
// tapi TIDAK lagi menghalangi pegawai mengajukan Berita Acara baru untuk
// tanggal yang sama (lihat tanggalPengajuanBeritaAcaraValid).
func tolakPengajuanBeritaAcaraAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Status != models.PengajuanBeritaAcaraMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya, atau belum disetujui atasan (status: "+item.Status+")")
		return
	}
	catatan := strings.TrimSpace(r.FormValue("catatan"))
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya pegawai tahu alasan penolakan")
		return
	}
	now := absensiNow()
	item.Status = models.PengajuanBeritaAcaraDitolak
	item.CatatanAdmin = catatan
	item.IDAdminApprove = &claims.UserID
	item.TglAdminApprove = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan Berita Acara ditolak", nil)
}

// canAccessPengajuanBeritaAcara: pegawai pemilik, atasan langsungnya, atau
// admin tahap akhir (lihat canApproveFinalPengajuanBeritaAcara).
func canAccessPengajuanBeritaAcara(claims *utils.Claims, item models.PengajuanBeritaAcara) bool {
	if claims == nil {
		return false
	}
	if canApproveFinalPengajuanBeritaAcara(claims) {
		return true
	}
	if claims.IDPegawai == nil {
		return false
	}
	if *claims.IDPegawai == item.IDPegawai {
		return true
	}
	return item.Pegawai != nil && item.Pegawai.IDAtasan != nil && *item.Pegawai.IDAtasan == *claims.IDPegawai
}

// downloadPengajuanBeritaAcara menangani GET
// /api/pengajuan-berita-acara/{id}/file -- diunduh/preview pegawai pemilik,
// atasan langsungnya, atau admin tahap akhir.
func downloadPengajuanBeritaAcara(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	// Omit bukti_dukung_file -- endpoint ini cuma menyajikan item.File,
	// supaya "Lihat"/unduh tidak lambat menunggu transfer blob yang tidak
	// dipakai (lihat catatan serupa pada downloadAbsensiDokumen).
	if err := db.Omit("bukti_dukung_file").Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuanBeritaAcara(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.File) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+item.NamaFile+"\"")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+item.NamaFile+"\"")
	}
	w.Write(item.File)
}

// downloadPengajuanBeritaAcaraBuktiDukung menangani GET
// /api/pengajuan-berita-acara/{id}/bukti-dukung -- unduh/preview lampiran
// bukti dukung yang diupload pegawai (lihat buatPengajuanBeritaAcara),
// kontrol akses SAMA PERSIS dengan downloadPengajuanBeritaAcara (pegawai
// pemilik, atasan langsungnya, atau admin tahap akhir).
func downloadPengajuanBeritaAcaraBuktiDukung(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanBeritaAcara
	// Omit file -- endpoint ini cuma menyajikan item.BuktiDukungFile, PDF
	// Berita Acara-nya (item.File) tidak perlu ikut dimuat.
	if err := db.Omit("file").Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuanBeritaAcara(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.BuktiDukungFile) == 0 {
		utils.Error(w, http.StatusNotFound, "bukti dukung tidak ditemukan")
		return
	}
	contentType := item.BuktiDukungContentType
	if contentType == "" {
		contentType = dokumenContentType(item.BuktiDukungNamaFile)
	}
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", "inline; filename=\""+item.BuktiDukungNamaFile+"\"")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+item.BuktiDukungNamaFile+"\"")
	}
	w.Write(item.BuktiDukungFile)
}

// RegisterPengajuanBeritaAcaraRoutes mendaftarkan seluruh endpoint di bawah
// /api/pengajuan-berita-acara*.
func RegisterPengajuanBeritaAcaraRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai", "atasan") }
	atasanOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "atasan") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }
	// final: HANYA administrator/admin/akun ber-flag IsAdminAbsensi/
	// IsAdminVerifikasi -- lihat canApproveFinalPengajuanBeritaAcara.
	final := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := middleware.GetClaims(r)
			if !ok {
				utils.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !canApproveFinalPengajuanBeritaAcara(claims) {
				utils.Error(w, http.StatusForbidden, "hanya administrator, admin, admin absen, atau admin verifikasi yang bisa melakukan aksi ini")
				return
			}
			h(w, r)
		}, middleware.Auth, middleware.RequireActiveUser(db))
	}

	mux.Handle("POST /api/pengajuan-berita-acara", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { buatPengajuanBeritaAcara(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-berita-acara/{id}", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { updatePengajuanBeritaAcara(w, r, db) }))
	mux.Handle("GET /api/pengajuan-berita-acara/saya", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { listPengajuanBeritaAcaraSaya(w, r, db) }))
	mux.Handle("GET /api/pengajuan-berita-acara/bawahan", atasanOnly(func(w http.ResponseWriter, r *http.Request) { listPengajuanBeritaAcaraBawahan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-berita-acara/{id}/setujui-atasan", atasanOnly(func(w http.ResponseWriter, r *http.Request) { setujuiPengajuanBeritaAcaraAtasan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-berita-acara/{id}/kembalikan-atasan", atasanOnly(func(w http.ResponseWriter, r *http.Request) { kembalikanPengajuanBeritaAcaraAtasan(w, r, db) }))
	mux.Handle("GET /api/pengajuan-berita-acara", final(func(w http.ResponseWriter, r *http.Request) { listPengajuanBeritaAcaraAdmin(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-berita-acara/{id}/setujui-admin", final(func(w http.ResponseWriter, r *http.Request) { setujuiPengajuanBeritaAcaraAdmin(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-berita-acara/{id}/kembalikan-admin", final(func(w http.ResponseWriter, r *http.Request) { kembalikanPengajuanBeritaAcaraAdmin(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-berita-acara/{id}/tolak-admin", final(func(w http.ResponseWriter, r *http.Request) { tolakPengajuanBeritaAcaraAdmin(w, r, db) }))
	mux.Handle("GET /api/pengajuan-berita-acara/{id}/file", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadPengajuanBeritaAcara(w, r, db) }))
	mux.Handle("GET /api/pengajuan-berita-acara/{id}/bukti-dukung", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadPengajuanBeritaAcaraBuktiDukung(w, r, db) }))
}
