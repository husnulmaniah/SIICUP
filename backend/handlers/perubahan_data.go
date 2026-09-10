package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// pegawaiEditableData adalah field-field data pegawai yang boleh diajukan
// pegawai untuk diubah sendiri lewat halaman "Profil Saya". NIP TIDAK
// termasuk -- NIP dipakai sebagai username akun login sehingga tetap hanya
// bisa diubah administrator lewat menu Data Pegawai / Akun Pengguna.
// Disimpan sebagai JSON text di kolom data_lama/data_baru (lihat
// models.PerubahanDataPegawai) baik sebagai snapshot data lama (saat
// pengajuan dibuat) maupun data baru yang diajukan.
type pegawaiEditableData struct {
	Nama         string `json:"nama"`
	IDJabatan    *uint  `json:"id_jabatan"`
	IDUnitKerja  *uint  `json:"id_unit_kerja"`
	IDPangkatGol *uint  `json:"id_pangkat_gol"`
	TempatTgs    string `json:"tempat_tgs"`
	TMT          string `json:"tmt"` // format "2006-01-02", boleh kosong
	NoHP         string `json:"no_hp"`
	IDStatus     *uint  `json:"id_status"`
	Email        string `json:"email"`
}

func pegawaiSnapshot(p models.Pegawai) pegawaiEditableData {
	tmt := ""
	if p.TMT != nil {
		tmt = p.TMT.Format("2006-01-02")
	}
	return pegawaiEditableData{
		Nama:         p.Nama,
		IDJabatan:    p.IDJabatan,
		IDUnitKerja:  p.IDUnitKerja,
		IDPangkatGol: p.IDPangkatGol,
		TempatTgs:    p.TempatTgs,
		TMT:          tmt,
		NoHP:         p.NoHP,
		IDStatus:     p.IDStatus,
		Email:        p.Email,
	}
}

func RegisterPerubahanDataRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai") }
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	mux.Handle("GET /api/perubahan-data", anyRole(func(w http.ResponseWriter, r *http.Request) { listPerubahanData(w, r, db) }))
	mux.Handle("GET /api/perubahan-data/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getPerubahanData(w, r, db) }))
	mux.Handle("POST /api/perubahan-data", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { createPerubahanData(w, r, db) }))
	mux.Handle("DELETE /api/perubahan-data/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { deletePerubahanData(w, r, db) }))
	mux.Handle("PUT /api/perubahan-data/{id}/approve", manage(func(w http.ResponseWriter, r *http.Request) { approvePerubahanData(w, r, db) }))
	mux.Handle("PUT /api/perubahan-data/{id}/reject", manage(func(w http.ResponseWriter, r *http.Request) { rejectPerubahanData(w, r, db) }))
	mux.Handle("GET /api/perubahan-data/{id}/dokumen", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadDokumenPerubahanData(w, r, db) }))
}

func canAccessPerubahanData(claims *utils.Claims, item models.PerubahanDataPegawai) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "pegawai", "atasan":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	}
	return false
}

func listPerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	query := db.Model(&models.PerubahanDataPegawai{}).Preload("Pegawai")

	switch claims.RoleName {
	case "pegawai", "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.PerubahanDataPegawai{})
			return
		}
		query = query.Where("id_pegawai = ?", *claims.IDPegawai)
	}

	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var items []models.PerubahanDataPegawai
	if err := query.Order("created_at desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.Success(w, "ok", items)
}

func getPerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanDataPegawai
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPerubahanData(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	utils.Success(w, "ok", item)
}

// createPerubahanData menerima multipart/form-data: field "data" berisi JSON
// pegawaiEditableData (nilai BARU yang diajukan, lengkap -- bukan hanya yang
// berubah), dan field file "file" berisi scan SK terakhir yang menjadi dasar
// perubahan tersebut (wajib diupload setiap kali mengajukan perubahan data).
func createPerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}

	var existingPending models.PerubahanDataPegawai
	if err := db.Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPending).First(&existingPending).Error; err == nil {
		utils.Error(w, http.StatusBadRequest, "anda masih memiliki pengajuan perubahan data yang sedang menunggu persetujuan")
		return
	}

	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	raw := r.FormValue("data")
	if strings.TrimSpace(raw) == "" {
		utils.Error(w, http.StatusBadRequest, "data perubahan wajib diisi")
		return
	}
	var baru pegawaiEditableData
	if err := json.Unmarshal([]byte(raw), &baru); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if strings.TrimSpace(baru.Nama) == "" {
		utils.Error(w, http.StatusBadRequest, "nama wajib diisi")
		return
	}
	if baru.TMT != "" {
		if _, err := utils.ParseDateCell(baru.TMT); err != nil {
			utils.Error(w, http.StatusBadRequest, "TMT tidak valid")
			return
		}
	}

	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas SK terakhir wajib diupload untuk setiap pengajuan perubahan data")
		return
	}
	ext := strings.ToLower(fh.Filename[strings.LastIndex(fh.Filename, "."):])
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "berkas SK terakhir harus berformat PDF, JPG, atau PNG")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas SK terakhir")
		return
	}
	fileData, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas SK terakhir")
		return
	}

	var pegawai models.Pegawai
	if err := db.First(&pegawai, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}

	lamaJSON, _ := json.Marshal(pegawaiSnapshot(pegawai))
	baruJSON, _ := json.Marshal(baru)

	item := models.PerubahanDataPegawai{
		IDPegawai:  *claims.IDPegawai,
		DataLama:   string(lamaJSON),
		DataBaru:   string(baruJSON),
		SkNamaFile: fh.Filename,
		SkFile:     fileData,
		Status:     models.StatusPending,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").First(&item, item.ID)
	utils.Created(w, "pengajuan perubahan data berhasil dikirim, menunggu persetujuan administrator/admin", item)
}

func deletePerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanDataPegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPerubahanData(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "hanya pengajuan yang masih menunggu persetujuan yang bisa dibatalkan/dihapus")
		return
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus pengajuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan perubahan data berhasil dibatalkan", nil)
}

type perubahanDataApprovalPayload struct {
	Catatan string `json:"catatan_admin"`
}

// approvePerubahanData menerapkan data_baru ke tabel pegawai (termasu
// mengganti dokumen SK Terakhir dengan berkas yang diupload pada pengajuan
// ini) DAN menyinkronkan nama ke akun user yang terhubung, karena tabel
// user punya kolom nama sendiri yang tidak otomatis ikut berubah.
func approvePerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanDataPegawai
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya")
		return
	}
	if item.Pegawai == nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai terkait tidak ditemukan")
		return
	}

	var baru pegawaiEditableData
	if err := json.Unmarshal([]byte(item.DataBaru), &baru); err != nil {
		utils.Error(w, http.StatusInternalServerError, "data perubahan tersimpan tidak valid")
		return
	}

	var p perubahanDataApprovalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)

	updates := map[string]interface{}{
		"nama":           baru.Nama,
		"id_jabatan":     baru.IDJabatan,
		"id_unit_kerja":  baru.IDUnitKerja,
		"id_pangkat_gol": baru.IDPangkatGol,
		"tempat_tgs":     baru.TempatTgs,
		"no_hp":          baru.NoHP,
		"id_status":      baru.IDStatus,
		"email":          baru.Email,
		// SK terakhir yang diupload pegawai saat mengajukan perubahan ini
		// menjadi dokumen SK Terakhir resmi begitu disetujui.
		"sk_terakhir_nama": item.SkNamaFile,
		"sk_terakhir_file": item.SkFile,
	}
	if baru.TMT != "" {
		tmt, err := utils.ParseDateCell(baru.TMT)
		if err != nil {
			utils.Error(w, http.StatusBadRequest, "TMT pada pengajuan tidak valid")
			return
		}
		updates["tmt"] = tmt
	} else {
		updates["tmt"] = nil
	}

	if err := db.Model(&models.Pegawai{}).Where("id = ?", item.IDPegawai).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memperbarui data pegawai: "+err.Error())
		return
	}
	// tabel user punya kolom nama tersendiri (tidak sinkron otomatis dengan
	// tabel pegawai) -- perbarui juga supaya akun pegawai ikut menampilkan
	// nama terbaru begitu perubahan disetujui.
	if err := db.Model(&models.User{}).Where("id_pegawai = ?", item.IDPegawai).Update("nama", baru.Nama).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyinkronkan nama ke akun user: "+err.Error())
		return
	}

	now := time.Now()
	item.Status = models.StatusDisetuju
	item.CatatanAdmin = p.Catatan
	item.DiputuskanOleh = claims.Username
	item.TglKeputusan = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan status persetujuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").First(&item, item.ID)
	utils.Success(w, "perubahan data berhasil disetujui, data pegawai & akun pegawai telah diperbarui", item)
}

func rejectPerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanDataPegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya")
		return
	}
	var p perubahanDataApprovalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)
	if strings.TrimSpace(p.Catatan) == "" {
		utils.Error(w, http.StatusBadRequest, "alasan penolakan wajib diisi")
		return
	}

	now := time.Now()
	item.Status = models.StatusDitolak
	item.CatatanAdmin = p.Catatan
	item.DiputuskanOleh = claims.Username
	item.TglKeputusan = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menolak pengajuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").First(&item, item.ID)
	utils.Success(w, "pengajuan perubahan data telah ditolak", item)
}

func downloadDokumenPerubahanData(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanDataPegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPerubahanData(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.SkFile) == 0 {
		utils.Error(w, http.StatusNotFound, "dokumen tidak ditemukan")
		return
	}
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Type", dokumenContentType(item.SkNamaFile))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", item.SkNamaFile))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.SkNamaFile))
	}
	w.Write(item.SkFile)
}
