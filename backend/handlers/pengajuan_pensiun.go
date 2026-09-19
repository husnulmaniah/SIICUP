package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// ============================================================
// Pengaturan Pensiun (usia pensiun standar, bisa diubah administrator)
// ============================================================

func getPengaturanPensiunRow(db *gorm.DB) (models.PengaturanPensiun, error) {
	var item models.PengaturanPensiun
	err := db.First(&item, 1).Error
	return item, err
}

// usiaPensiunPegawai menentukan usia pensiun standar seorang pegawai
// berdasarkan Jabatan.JenisJabatan-nya -- Fungsional pensiun di usia lebih
// tua (default 60 tahun), Pelaksana & Struktural di usia yang sama (default
// 58 tahun). Jabatan yang belum diisi/tidak dikenal dianggap Pelaksana/
// Struktural (paling umum) supaya tidak salah menaikkan usia pensiunnya.
func usiaPensiunPegawai(pegawai models.Pegawai, pengaturan models.PengaturanPensiun) int {
	if pegawai.Jabatan != nil && pegawai.Jabatan.JenisJabatan == models.JenisJabatanFungsional {
		return pengaturan.UsiaFungsional
	}
	return pengaturan.UsiaPelaksanaStruktural
}

// hitungUsia mengembalikan usia (tahun lengkap, dibulatkan ke bawah) dari
// tglLahir pada tanggal "pada".
func hitungUsia(tglLahir, pada time.Time) int {
	usia := pada.Year() - tglLahir.Year()
	if pada.Month() < tglLahir.Month() || (pada.Month() == tglLahir.Month() && pada.Day() < tglLahir.Day()) {
		usia--
	}
	if usia < 0 {
		return 0
	}
	return usia
}

func getPengaturanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, err := getPengaturanPensiunRow(db)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memuat pengaturan pensiun")
		return
	}
	utils.Success(w, "ok", item)
}

type pengaturanPensiunPayload struct {
	UsiaPelaksanaStruktural int `json:"usia_pelaksana_struktural"`
	UsiaFungsional          int `json:"usia_fungsional"`
}

func updatePengaturanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p pengaturanPensiunPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.UsiaPelaksanaStruktural < 40 || p.UsiaPelaksanaStruktural > 75 || p.UsiaFungsional < 40 || p.UsiaFungsional > 75 {
		utils.Error(w, http.StatusBadRequest, "usia pensiun harus dalam rentang yang wajar (40-75 tahun)")
		return
	}
	updates := map[string]interface{}{
		"usia_pelaksana_struktural": p.UsiaPelaksanaStruktural,
		"usia_fungsional":           p.UsiaFungsional,
	}
	if err := db.Model(&models.PengaturanPensiun{}).Where("id = ?", 1).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengaturan: "+err.Error())
		return
	}
	item, _ := getPengaturanPensiunRow(db)
	utils.Success(w, "pengaturan pensiun berhasil disimpan", item)
}

// ============================================================
// Pengajuan Pensiun (pegawai mengajukan, administrator/admin menyetujui)
// ============================================================

func RegisterPengajuanPensiunRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	// anyRole di sini juga TIDAK benar-benar "role apa saja" -- lihat catatan
	// yang sama di RegisterPerubahanDataRoutes (perubahan_data.go).
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "pegawai", "atasan") }
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai") }
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	// Pengaturan usia pensiun -- GET dibuka untuk semua role terautentikasi
	// karena dipakai juga di halaman Profil Saya pegawai untuk menghitung &
	// menampilkan perkiraan usia pensiunnya sendiri, bukan cuma di menu
	// administrator.
	mux.Handle("GET /api/pengaturan-pensiun", anyRole(func(w http.ResponseWriter, r *http.Request) { getPengaturanPensiun(w, r, db) }))
	mux.Handle("PUT /api/pengaturan-pensiun", manage(func(w http.ResponseWriter, r *http.Request) { updatePengaturanPensiun(w, r, db) }))

	mux.Handle("GET /api/pengajuan-pensiun", anyRole(func(w http.ResponseWriter, r *http.Request) { listPengajuanPensiun(w, r, db) }))
	mux.Handle("GET /api/pengajuan-pensiun/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getPengajuanPensiunDetail(w, r, db) }))
	mux.Handle("POST /api/pengajuan-pensiun", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { createPengajuanPensiun(w, r, db) }))
	mux.Handle("DELETE /api/pengajuan-pensiun/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { batalkanPengajuanPensiun(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-pensiun/{id}/approve", manage(func(w http.ResponseWriter, r *http.Request) { approvePengajuanPensiun(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-pensiun/{id}/reject", manage(func(w http.ResponseWriter, r *http.Request) { rejectPengajuanPensiun(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-pensiun/{id}/batalkan-persetujuan", manage(func(w http.ResponseWriter, r *http.Request) { batalkanPersetujuanPensiun(w, r, db) }))
	mux.Handle("GET /api/pengajuan-pensiun/{id}/dokumen", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadDokumenPengajuanPensiun(w, r, db) }))
}

func canAccessPengajuanPensiun(claims *utils.Claims, item models.PengajuanPensiun) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "pegawai", "atasan":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	}
	return false
}

func listPengajuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	query := db.Model(&models.PengajuanPensiun{}).Preload("Pegawai")

	switch claims.RoleName {
	case "pegawai", "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.PengajuanPensiun{})
			return
		}
		query = query.Where("id_pegawai = ?", *claims.IDPegawai)
	}

	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var items []models.PengajuanPensiun
	if err := query.Order("created_at desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.Success(w, "ok", items)
}

func getPengajuanPensiunDetail(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanPensiun
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuanPensiun(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	utils.Success(w, "ok", item)
}

// createPengajuanPensiun menerima multipart/form-data: "is_pensiun_dini"
// ("true"/"false"), "alasan" (wajib diisi kalau pensiun dini), dan file "file"
// berisi SK/usulan pensiun. Kalau BUKAN pensiun dini, usia pegawai (dari
// Pegawai.TglLahir) harus sudah mencapai usia pensiun standar jabatannya
// (lihat usiaPensiunPegawai) -- kalau belum, pengajuan ditolak dan pegawai
// diarahkan mencentang opsi Pensiun Dini kalau memang itu maksudnya.
func createPengajuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}

	var existingPending models.PengajuanPensiun
	if err := db.Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPending).First(&existingPending).Error; err == nil {
		utils.Error(w, http.StatusBadRequest, "anda masih memiliki pengajuan pensiun yang sedang menunggu persetujuan")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	isPensiunDini := r.FormValue("is_pensiun_dini") == "true"
	alasan := strings.TrimSpace(r.FormValue("alasan"))
	if isPensiunDini && alasan == "" {
		utils.Error(w, http.StatusBadRequest, "alasan wajib diisi untuk pengajuan pensiun dini")
		return
	}

	var pegawai models.Pegawai
	if err := db.Preload("Jabatan").First(&pegawai, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}

	pengaturan, err := getPengaturanPensiunRow(db)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memuat pengaturan usia pensiun")
		return
	}

	if !isPensiunDini {
		if pegawai.TglLahir == nil {
			utils.Error(w, http.StatusBadRequest, "tanggal lahir anda belum diisi administrator sehingga usia pensiun tidak bisa dihitung otomatis. Hubungi administrator untuk melengkapinya, atau centang \"Pensiun Dini\" kalau memang itu maksud pengajuan ini")
			return
		}
		usia := hitungUsia(*pegawai.TglLahir, time.Now())
		usiaPensiun := usiaPensiunPegawai(pegawai, pengaturan)
		if usia < usiaPensiun {
			utils.Error(w, http.StatusBadRequest, fmt.Sprintf(
				"usia anda saat ini %d tahun, belum mencapai usia pensiun (%d tahun) untuk jabatan anda. Kalau pengajuan ini memang disengaja lebih awal, centang opsi \"Pensiun Dini\" dan isi alasannya",
				usia, usiaPensiun))
			return
		}
	}

	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas SK/usulan pensiun wajib diupload")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, JPG, atau PNG")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering terjadi kalau
	// foto dari WhatsApp/Google Photos di HP belum selesai diunduh ke
	// perangkat saat dipilih lewat file picker.
	if len(data) == 0 {
		utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
		return
	}

	item := models.PengajuanPensiun{
		IDPegawai:     *claims.IDPegawai,
		IsPensiunDini: isPensiunDini,
		Alasan:        alasan,
		SkNamaFile:    fh.Filename,
		SkFile:        data,
		Status:        models.StatusPending,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").First(&item, item.ID)
	utils.Created(w, "pengajuan pensiun berhasil dikirim, menunggu persetujuan administrator/admin", item)
}

// batalkanPengajuanPensiun membatalkan pengajuan yang MASIH pending (dipakai
// pegawai untuk menarik pengajuannya sendiri sebelum diproses, atau
// administrator/admin). Pengajuan yang SUDAH disetujui tidak dibatalkan lewat
// endpoint ini -- lihat batalkanPersetujuanPensiun.
func batalkanPengajuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanPensiun
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuanPensiun(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "hanya pengajuan yang masih menunggu persetujuan yang bisa dibatalkan di sini -- pengajuan yang sudah disetujui dibatalkan lewat tombol \"Batalkan Persetujuan\" oleh administrator/admin")
		return
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membatalkan pengajuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan pensiun berhasil dibatalkan", nil)
}

type pensiunApprovalPayload struct {
	Catatan string `json:"catatan_admin"`
}

// approvePengajuanPensiun menerapkan efek pensiun ke pegawai TERKAIT: status
// kepegawaiannya diubah jadi "Pensiun" (dokumen SK yang diupload pada
// pengajuan ini turut disalin jadi SK Pensiun resmi, mengikuti pola
// approvePerubahanData di perubahan_data.go), dan akun login-nya
// dinonaktifkan (models.User.Aktif) SEKETIKA -- lihat middleware.
// RequireActiveUser untuk bagaimana ini langsung berlaku walau token JWT
// pegawai tersebut belum kedaluwarsa. id_status pegawai SEBELUM diubah
// disimpan di item.IDStatusSebelum supaya bisa dikembalikan kalau
// administrator membatalkan persetujuan ini nanti (lihat
// batalkanPersetujuanPensiun).
func approvePengajuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanPensiun
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

	var p pensiunApprovalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)

	sid, err := pensiunStatusID(db)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyiapkan status Pensiun: "+err.Error())
		return
	}
	statusSebelum := item.Pegawai.IDStatus // snapshot SEBELUM diubah di bawah

	updates := map[string]interface{}{
		"id_status":       sid,
		"sk_pensiun_nama": item.SkNamaFile,
		"sk_pensiun_file": item.SkFile,
	}
	if err := db.Model(&models.Pegawai{}).Where("id = ?", item.IDPegawai).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memperbarui data pegawai: "+err.Error())
		return
	}
	setAkunAktifByPegawai(db, item.IDPegawai, false)

	now := time.Now()
	item.Status = models.StatusDisetuju
	item.CatatanAdmin = p.Catatan
	item.DiputuskanOleh = claims.Username
	item.TglKeputusan = &now
	item.IDStatusSebelum = statusSebelum
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan status persetujuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").First(&item, item.ID)
	utils.Success(w, "pengajuan pensiun disetujui -- status pegawai diubah jadi Pensiun & akun login dinonaktifkan", item)
}

func rejectPengajuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanPensiun
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya")
		return
	}
	var p pensiunApprovalPayload
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
	utils.Success(w, "pengajuan pensiun telah ditolak", item)
}

// batalkanPersetujuanPensiun membatalkan pengajuan yang SUDAH disetujui --
// mengembalikan status kepegawaian pegawai ke id_status SEBELUM disetujui
// (item.IDStatusSebelum, lihat approvePengajuanPensiun) dan mengaktifkan
// kembali akun login-nya, lalu mengembalikan pengajuan ini ke status
// "pending" (bisa diproses ulang -- disetujui lagi atau ditolak) mengikuti
// pola returnPengajuan untuk Pengajuan Cuti di pengajuan_cuti.go.
func batalkanPersetujuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.PengajuanPensiun
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.StatusDisetuju {
		utils.Error(w, http.StatusBadRequest, "hanya pengajuan yang sudah disetujui yang bisa dibatalkan persetujuannya")
		return
	}

	updates := map[string]interface{}{"id_status": item.IDStatusSebelum}
	if err := db.Model(&models.Pegawai{}).Where("id = ?", item.IDPegawai).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengembalikan status pegawai: "+err.Error())
		return
	}
	setAkunAktifByPegawai(db, item.IDPegawai, true)

	item.Status = models.StatusPending
	item.CatatanAdmin = ""
	item.DiputuskanOleh = ""
	item.TglKeputusan = nil
	item.IDStatusSebelum = nil
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membatalkan persetujuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").First(&item, item.ID)
	utils.Success(w, "persetujuan pensiun dibatalkan -- status pegawai & akun login pegawai dikembalikan seperti semula", item)
}

func downloadDokumenPengajuanPensiun(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanPensiun
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuanPensiun(claims, item) {
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
