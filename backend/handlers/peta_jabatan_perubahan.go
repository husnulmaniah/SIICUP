package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// peta_jabatan_perubahan.go menangani PerubahanJabatanPegawai -- tahap
// KEDUA setelah PengajuanKenaikanPangkat disetujui penuh administrator
// (lihat setujuiKenaikanPangkatAdmin di
// handlers/peta_jabatan_kenaikan_pangkat.go yang membuat barisnya secara
// otomatis). Baris ini muncul sebagai PERINGATAN pada akun pegawai yang
// wajib diselesaikan dengan mengupload SK (surat keputusan) jabatan/
// pangkat barunya; begitu administrator menyetujuinya, Pegawai.IDJabatan/
// IDSubJabatan/IDPangkatGol BARU baru benar-benar diterapkan -- titik
// inilah "B" (Bezetting) pada Jabatan tujuan bertambah & Jabatan asal
// berkurang (karena B selalu dihitung langsung dari data pegawai saat ini,
// lihat hitungPetaJabatanSekolah).

func RegisterPetaJabatanPerubahanRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	pegawaiAtasan := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai", "atasan") }
	adminOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan", "pegawai") }

	mux.Handle("GET /api/peta-jabatan/perubahan/saya", pegawaiAtasan(func(w http.ResponseWriter, r *http.Request) { perubahanJabatanSaya(w, r, db) }))
	mux.Handle("POST /api/peta-jabatan/perubahan/saya/upload", pegawaiAtasan(func(w http.ResponseWriter, r *http.Request) { uploadPerubahanJabatanSaya(w, r, db) }))

	mux.Handle("GET /api/peta-jabatan/perubahan", adminOnly(func(w http.ResponseWriter, r *http.Request) { listPerubahanJabatan(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/perubahan/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getPerubahanJabatanDetail(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/perubahan/{id}/sk", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadSkPerubahanJabatan(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/perubahan/{id}/approve", adminOnly(func(w http.ResponseWriter, r *http.Request) { approvePerubahanJabatan(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/perubahan/{id}/reject", adminOnly(func(w http.ResponseWriter, r *http.Request) { rejectPerubahanJabatan(w, r, db) }))
}

func perubahanJabatanPreload(db *gorm.DB) *gorm.DB {
	omitDokumen := func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }
	return db.Omit("sk_file").
		Preload("Pegawai", omitDokumen).Preload("Pegawai.Jabatan").Preload("Pegawai.UnitKerja").
		Preload("PengajuanKenaikanPangkat", func(tx *gorm.DB) *gorm.DB { return tx.Omit("ukom_file", "file") }).
		Preload("PengajuanKenaikanPangkat.JabatanAsal").Preload("PengajuanKenaikanPangkat.SubJabatanAsal").
		Preload("JabatanBaru").Preload("SubJabatanBaru").Preload("PangkatGolBaru.Pangkat").Preload("PangkatGolBaru.Gol")
}

func canAccessPerubahanJabatan(claims *utils.Claims, item models.PerubahanJabatanPegawai) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "pegawai", "atasan":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	}
	return false
}

// perubahanJabatanSaya menangani GET /api/peta-jabatan/perubahan/saya --
// dipanggil frontend (dashboard/profil) untuk mengecek apakah akun yang
// login punya peringatan Perubahan Jabatan yang masih harus diselesaikan
// (status "menunggu_upload" ATAU "menunggu_admin"). Mengembalikan null
// kalau tidak ada apa-apa yang perlu ditindaklanjuti.
func perubahanJabatanSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", nil)
		return
	}
	var item models.PerubahanJabatanPegawai
	err := perubahanJabatanPreload(db).
		Where("id_pegawai = ? AND status IN ?", *claims.IDPegawai, []string{models.PerubahanJabatanMenungguUpload, models.PerubahanJabatanMenungguAdmin}).
		Order("created_at desc").First(&item).Error
	if err != nil {
		utils.Success(w, "ok", nil)
		return
	}
	utils.Success(w, "ok", item)
}

// uploadPerubahanJabatanSaya menangani POST
// /api/peta-jabatan/perubahan/saya/upload -- pegawai menyelesaikan
// peringatan Perubahan Jabatan dengan mengupload SK (surat keputusan)
// jabatan/pangkat barunya. HANYA bisa dilakukan selama status masih
// "menunggu_upload" (belum pernah upload, atau upload sebelumnya baru
// ditolak administrator -- lihat rejectPerubahanJabatan yang mengembalikan
// status ke sini supaya pegawai bisa upload ulang).
func uploadPerubahanJabatanSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}
	var item models.PerubahanJabatanPegawai
	if err := db.Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.PerubahanJabatanMenungguUpload).
		Order("created_at desc").First(&item).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "tidak ada peringatan perubahan jabatan yang menunggu upload SK")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}
	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas SK wajib diupload")
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
	if len(data) == 0 {
		utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
		return
	}

	item.SkNamaFile = fh.Filename
	item.SkFile = data
	item.Status = models.PerubahanJabatanMenungguAdmin
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan berkas: "+err.Error())
		return
	}
	utils.Success(w, "SK berhasil diupload, menunggu persetujuan administrator", nil)
}

func listPerubahanJabatan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	query := perubahanJabatanPreload(db)
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" && status != "semua" {
		query = query.Where("status = ?", status)
	}
	var items []models.PerubahanJabatanPegawai
	query.Order("created_at desc").Find(&items)
	utils.Success(w, "ok", items)
}

func getPerubahanJabatanDetail(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanJabatanPegawai
	if err := perubahanJabatanPreload(db).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPerubahanJabatan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	utils.Success(w, "ok", item)
}

func downloadSkPerubahanJabatan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanJabatanPegawai
	if err := db.Select("id", "id_pegawai", "sk_nama_file", "sk_file").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPerubahanJabatan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.SkFile) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas belum diupload")
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

type approvePerubahanJabatanPayload struct {
	IDPangkatGolBaru *uint `json:"id_pangkat_gol_baru"`
}

// approvePerubahanJabatan menangani PUT
// /api/peta-jabatan/perubahan/{id}/approve -- titik SATU-SATUNYA di seluruh
// alur Peta Jabatan di mana Pegawai.IDJabatan/IDSubJabatan (dan opsional
// IDPangkatGol, lihat komentar PangkatGolBaru di models.go) benar-benar
// DITERAPKAN. Begitu diterapkan, "B" Jabatan asal otomatis berkurang 1 &
// "B" Jabatan tujuan otomatis bertambah 1 pada tabel Peta Jabatan -- TANPA
// perlu ditulis manual di sini, karena B selalu DIHITUNG LANGSUNG dari
// kolom Pegawai.IDJabatan/IDSubJabatan saat ini (lihat
// hitungPetaJabatanSekolah).
func approvePerubahanJabatan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanJabatanPegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.PerubahanJabatanMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "data ini belum diupload SK-nya, atau sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}
	var p approvePerubahanJabatanPayload
	// body opsional -- boleh dikirim kosong kalau tidak ada perubahan
	// pangkat/golongan yang menyertai kenaikan jabatan ini.
	_ = json.NewDecoder(r.Body).Decode(&p)
	if p.IDPangkatGolBaru != nil {
		var pg models.PangkatGol
		if err := db.First(&pg, *p.IDPangkatGolBaru).Error; err != nil {
			utils.Error(w, http.StatusBadRequest, "pangkat/golongan baru tidak valid")
			return
		}
	}

	now := absensiNow()
	err := db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"id_jabatan":     item.IDJabatanBaru,
			"id_sub_jabatan": item.IDSubJabatanBaru,
		}
		if p.IDPangkatGolBaru != nil {
			updates["id_pangkat_gol"] = *p.IDPangkatGolBaru
		}
		if err := tx.Model(&models.Pegawai{}).Where("id = ?", item.IDPegawai).Updates(updates).Error; err != nil {
			return err
		}
		item.IDPangkatGolBaru = p.IDPangkatGolBaru
		item.Status = models.PerubahanJabatanDisetujui
		item.DiputuskanOleh = claims.Username
		item.TglKeputusan = &now
		return tx.Save(&item).Error
	})
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menerapkan perubahan jabatan: "+err.Error())
		return
	}
	utils.Success(w, "perubahan jabatan disetujui & diterapkan ke data pegawai", nil)
}

// rejectPerubahanJabatan: mengembalikan status ke "menunggu_upload" (BUKAN
// status "ditolak" final) supaya pegawai bisa mengupload SK yang benar/
// baru -- SK yang salah/kurang jelas lebih sering butuh diperbaiki pegawai
// daripada benar-benar dibatalkan seluruh kenaikan pangkatnya (yang sudah
// terlanjur disetujui dua tahap sebelumnya). Administrator yang memang
// ingin membatalkan total sebaiknya menolaknya sejak tahap
// PengajuanKenaikanPangkat, bukan di sini.
func rejectPerubahanJabatan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PerubahanJabatanPegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if item.Status != models.PerubahanJabatanMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "data ini belum diupload SK-nya, atau sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}
	var payload struct {
		Catatan string `json:"catatan"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	catatan := strings.TrimSpace(payload.Catatan)
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya pegawai tahu apa yang perlu diperbaiki")
		return
	}
	now := absensiNow()
	item.Status = models.PerubahanJabatanMenungguUpload
	item.CatatanAdmin = catatan
	item.DiputuskanOleh = claims.Username
	item.TglKeputusan = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "SK dikembalikan ke pegawai untuk diupload ulang", nil)
}
