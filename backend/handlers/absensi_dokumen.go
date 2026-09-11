package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// absensi_dokumen.go menangani upload surat pengganti (SKS/Surat Tugas/
// Berita Acara/Surat Izin) untuk tanggal absen yang terlewat -- lihat
// riwayatAbsenSaya (absensi.go) untuk bagaimana tanggal_terlewat dihitung.

var absensiDokumenLabels = map[string]string{
	models.AbsensiDokumenSKS:         "Surat Keterangan Sakit (SKS)",
	models.AbsensiDokumenSuratTugas:  "Surat Tugas",
	models.AbsensiDokumenBeritaAcara: "Berita Acara",
	models.AbsensiDokumenSuratIzin:   "Surat Izin",
}

func canAccessAbsensiDokumen(claims *utils.Claims, item models.AbsensiDokumen) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "pegawai", "atasan":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	}
	return false
}

// uploadAbsensiDokumen menerima multipart/form-data: "tanggal" (tanggal yang
// terlewat, format DD-MM-YYYY atau YYYY-MM-DD), "jenis" (sks/surat_tugas/
// berita_acara/surat_izin), "keterangan" (opsional), dan file "file".
func uploadAbsensiDokumen(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}

	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	tanggalRaw := strings.TrimSpace(r.FormValue("tanggal"))
	if tanggalRaw == "" {
		utils.Error(w, http.StatusBadRequest, "tanggal wajib diisi")
		return
	}
	tanggal, err := utils.ParseDateCell(tanggalRaw)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal tidak valid")
		return
	}

	jenis := strings.TrimSpace(r.FormValue("jenis"))
	label, valid := absensiDokumenLabels[jenis]
	if !valid {
		utils.Error(w, http.StatusBadRequest, "jenis surat tidak valid (pilih: sks, surat_tugas, berita_acara, atau surat_izin)")
		return
	}

	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas surat wajib diupload")
		return
	}
	ext := strings.ToLower(fh.Filename[strings.LastIndex(fh.Filename, "."):])
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, JPG, atau PNG")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	fileData, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}

	item := models.AbsensiDokumen{
		IDPegawai:  *claims.IDPegawai,
		Tanggal:    tanggal,
		Jenis:      jenis,
		Label:      label,
		NamaFile:   fh.Filename,
		File:       fileData,
		Keterangan: strings.TrimSpace(r.FormValue("keterangan")),
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan surat: "+err.Error())
		return
	}
	item.File = nil
	utils.Created(w, "surat pengganti berhasil diupload untuk tanggal "+tanggalRaw, item)
}

func listAbsensiDokumenSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", []models.AbsensiDokumen{})
		return
	}
	items := []models.AbsensiDokumen{}
	if err := db.Omit("file").Where("id_pegawai = ?", *claims.IDPegawai).
		Order("tanggal desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.Success(w, "ok", items)
}

func downloadAbsensiDokumen(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.AbsensiDokumen
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessAbsensiDokumen(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.File) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Type", dokumenContentType(item.NamaFile))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", item.NamaFile))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.NamaFile))
	}
	w.Write(item.File)
}

func deleteAbsensiDokumen(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.AbsensiDokumen
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessAbsensiDokumen(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus surat: "+err.Error())
		return
	}
	utils.Success(w, "surat pengganti berhasil dihapus", nil)
}
