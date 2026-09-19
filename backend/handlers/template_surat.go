package handlers

import (
	"io"
	"net/http"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// template_surat.go menangani menu "Template Surat" -- administrator
// menambahkan template surat (judul + berkas PDF/Word) yang ditampilkan
// sebagai grid kartu, terlihat oleh SEMUA akun sekolah (pegawai maupun
// atasan bertugas di sekolah, lihat isSekolahPegawai) supaya bisa dilihat/
// dipreview langsung TANPA harus mendownload dulu -- preview berkas lewat
// endpoint file dengan ?inline=1 (sama seperti downloadPengajuanSuratKolektif),
// dibaca frontend sebagai blob terautentikasi (PDF ditampilkan lewat
// <iframe>/<embed>, DOCX lewat docx-preview, DOC lama fallback download saja).
//
// Hanya administrator/admin yang boleh menambah/mengubah/menghapus --
// pegawai/atasan (termasuk yang bertugas di sekolah) hanya boleh melihat.

// templateSuratOut adalah DTO respons list/detail -- TIDAK menyertakan isi
// berkas (field File di model sudah json:"-" juga, ini cuma dokumentasi
// tambahan supaya jelas daftar selalu ringan).
type templateSuratOut struct {
	ID        uint   `json:"id"`
	Judul     string `json:"judul"`
	NamaFile  string `json:"nama_file"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toTemplateSuratOut(item models.TemplateSurat) templateSuratOut {
	return templateSuratOut{
		ID:        item.ID,
		Judul:     item.Judul,
		NamaFile:  item.NamaFile,
		CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// canViewTemplateSurat: administrator/admin selalu boleh (supaya bisa
// memeriksa hasil kerjanya sendiri); selain itu HANYA akun yang terhubung ke
// data pegawai bertugas di sekolah (isSekolahPegawai) -- pegawai/atasan
// bertugas di dinas/kantor tidak melihat menu ini sama sekali, sesuai
// permintaan pengguna ("menu yang bisa terlihat di semua akun sekolah").
func canViewTemplateSurat(claims *utils.Claims, db *gorm.DB) bool {
	if claims.RoleName == "administrator" || claims.RoleName == "admin" {
		return true
	}
	if claims.IDPegawai == nil {
		return false
	}
	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").First(&pegawai, *claims.IDPegawai).Error; err != nil {
		return false
	}
	return isSekolahPegawai(pegawai)
}

func canManageTemplateSurat(claims *utils.Claims) bool {
	return claims.RoleName == "administrator" || claims.RoleName == "admin"
}

// listTemplateSurat menangani GET /api/template-surat.
func listTemplateSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if !canViewTemplateSurat(claims, db) {
		utils.Error(w, http.StatusForbidden, "menu Template Surat hanya untuk akun sekolah atau administrator")
		return
	}
	var items []models.TemplateSurat
	db.Omit("file").Order("created_at desc").Find(&items)
	out := make([]templateSuratOut, 0, len(items))
	for _, it := range items {
		out = append(out, toTemplateSuratOut(it))
	}
	utils.Success(w, "ok", out)
}

// buatTemplateSurat menangani POST /api/template-surat (multipart/form-data:
// "judul", "file" -- PDF, DOC, atau DOCX).
func buatTemplateSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if !canManageTemplateSurat(claims) {
		utils.Error(w, http.StatusForbidden, "hanya administrator yang bisa menambah template surat")
		return
	}
	utils.LimitBody(w, r, 20<<20)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal 20MB)")
		return
	}
	judul := strings.TrimSpace(r.FormValue("judul"))
	if judul == "" {
		utils.Error(w, http.StatusBadRequest, "judul surat wajib diisi")
		return
	}
	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas template (PDF/Word) wajib diupload")
		return
	}
	ext := strings.ToLower(fh.Filename[strings.LastIndex(fh.Filename, "."):])
	if ext != ".pdf" && ext != ".doc" && ext != ".docx" {
		utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, DOC, atau DOCX")
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
	// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering terjadi kalau
	// foto/dokumen dari WhatsApp/Google Photos/cloud belum selesai diunduh
	// ke perangkat saat dipilih lewat file picker.
	if len(fileData) == 0 {
		utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
		return
	}

	item := models.TemplateSurat{
		Judul:    judul,
		NamaFile: fh.Filename,
		File:     fileData,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan template surat: "+err.Error())
		return
	}
	utils.Created(w, "template surat berhasil ditambahkan", toTemplateSuratOut(item))
}

// updateTemplateSurat menangani PUT /api/template-surat/{id} (multipart/
// form-data: "judul", "file" opsional -- kalau tidak dikirim, berkas lama
// dipertahankan).
func updateTemplateSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if !canManageTemplateSurat(claims) {
		utils.Error(w, http.StatusForbidden, "hanya administrator yang bisa mengubah template surat")
		return
	}
	id := r.PathValue("id")
	var item models.TemplateSurat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "template surat tidak ditemukan")
		return
	}
	utils.LimitBody(w, r, 20<<20)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal 20MB)")
		return
	}
	judul := strings.TrimSpace(r.FormValue("judul"))
	if judul == "" {
		utils.Error(w, http.StatusBadRequest, "judul surat wajib diisi")
		return
	}
	item.Judul = judul
	if fh := formFileHeader(r, "file"); fh != nil {
		ext := strings.ToLower(fh.Filename[strings.LastIndex(fh.Filename, "."):])
		if ext != ".pdf" && ext != ".doc" && ext != ".docx" {
			utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, DOC, atau DOCX")
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
		// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering terjadi
		// kalau foto/dokumen dari WhatsApp/Google Photos/cloud belum selesai
		// diunduh ke perangkat saat dipilih lewat file picker.
		if len(fileData) == 0 {
			utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
			return
		}
		item.NamaFile = fh.Filename
		item.File = fileData
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan perubahan: "+err.Error())
		return
	}
	utils.Success(w, "template surat berhasil diperbarui", toTemplateSuratOut(item))
}

// hapusTemplateSurat menangani DELETE /api/template-surat/{id}.
func hapusTemplateSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if !canManageTemplateSurat(claims) {
		utils.Error(w, http.StatusForbidden, "hanya administrator yang bisa menghapus template surat")
		return
	}
	id := r.PathValue("id")
	if err := db.Delete(&models.TemplateSurat{}, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus template surat: "+err.Error())
		return
	}
	utils.Success(w, "template surat berhasil dihapus", nil)
}

// fileTemplateSurat menangani GET /api/template-surat/{id}/file -- default
// memaksa download (Content-Disposition: attachment), tapi dengan
// ?inline=1 disajikan "inline" dengan MIME type asli supaya bisa
// ditampilkan langsung (PDF via <iframe>/<embed>, DOCX diambil sebagai blob
// lalu dirender docx-preview) tanpa pegawai harus mendownloadnya dulu.
func fileTemplateSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if !canViewTemplateSurat(claims, db) {
		utils.Error(w, http.StatusForbidden, "menu Template Surat hanya untuk akun sekolah atau administrator")
		return
	}
	id := r.PathValue("id")
	var item models.TemplateSurat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "template surat tidak ditemukan")
		return
	}
	if len(item.File) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	w.Header().Set("Content-Type", dokumenContentType(item.NamaFile))
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+item.NamaFile+"\"")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+item.NamaFile+"\"")
	}
	w.Write(item.File)
}

// RegisterTemplateSuratRoutes mendaftarkan semua endpoint /api/template-surat*.
func RegisterTemplateSuratRoutes(mux *http.ServeMux, db *gorm.DB) {
	anyRole := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole())
	}
	mux.Handle("GET /api/template-surat", anyRole(func(w http.ResponseWriter, r *http.Request) { listTemplateSurat(w, r, db) }))
	mux.Handle("POST /api/template-surat", anyRole(func(w http.ResponseWriter, r *http.Request) { buatTemplateSurat(w, r, db) }))
	mux.Handle("PUT /api/template-surat/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { updateTemplateSurat(w, r, db) }))
	mux.Handle("DELETE /api/template-surat/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { hapusTemplateSurat(w, r, db) }))
	mux.Handle("GET /api/template-surat/{id}/file", anyRole(func(w http.ResponseWriter, r *http.Request) { fileTemplateSurat(w, r, db) }))
}
