package handlers

import (
	"encoding/json"
	"net/http"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// ============================================================
// Pengaturan Kenaikan Gaji Berkala & Kenaikan Pangkat (interval standar,
// bisa diubah administrator/admin) -- lihat models.PengaturanKenaikanGajiBerkala.
//
// BERBEDA dari Pengajuan Pensiun: tidak ada alur pengajuan/persetujuan
// tersendiri di sini. Tanggal kenaikan terakhir & berkas SK-nya diajukan
// pegawai lewat Profil Saya -> Ajukan Perubahan Data (lihat
// handlers/perubahan_data.go, field TglKenaikanGajiBerkalaTerakhir/
// TglKenaikanPangkatTerakhir pada pegawaiEditableData) atau diisi langsung
// oleh administrator lewat menu Data Pegawai. Pengaturan di sini HANYA
// menyimpan interval (dalam tahun) yang dipakai frontend (ProfilSayaView,
// CrudManager, PerubahanDataView) untuk MENGHITUNG & MENAMPILKAN kapan
// kenaikan berikutnya jatuh tempo -- murni tampilan, tidak ada validasi
// server-side yang menggantungkan alur lain padanya.
// ============================================================

func RegisterPengaturanKenaikanGajiBerkalaRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	// GET dibuka untuk semua role terautentikasi karena dipakai juga di
	// halaman Profil Saya pegawai untuk menghitung & menampilkan kelayakan
	// kenaikan gaji berkala/pangkatnya sendiri, sama seperti GET
	// /pengaturan-pensiun.
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "pegawai", "atasan") }
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	mux.Handle("GET /api/pengaturan-kenaikan-gaji-berkala", anyRole(func(w http.ResponseWriter, r *http.Request) {
		getPengaturanKenaikanGajiBerkala(w, r, db)
	}))
	mux.Handle("PUT /api/pengaturan-kenaikan-gaji-berkala", manage(func(w http.ResponseWriter, r *http.Request) {
		updatePengaturanKenaikanGajiBerkala(w, r, db)
	}))
}

func getPengaturanKenaikanGajiBerkalaRow(db *gorm.DB) (models.PengaturanKenaikanGajiBerkala, error) {
	var item models.PengaturanKenaikanGajiBerkala
	err := db.First(&item, 1).Error
	return item, err
}

func getPengaturanKenaikanGajiBerkala(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, err := getPengaturanKenaikanGajiBerkalaRow(db)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memuat pengaturan kenaikan gaji berkala")
		return
	}
	utils.Success(w, "ok", item)
}

type pengaturanKenaikanGajiBerkalaPayload struct {
	GajiBerkalaFungsionalTahun          int `json:"gaji_berkala_fungsional_tahun"`
	GajiBerkalaPelaksanaStrukturalTahun int `json:"gaji_berkala_pelaksana_struktural_tahun"`
	PangkatFungsionalTahun              int `json:"pangkat_fungsional_tahun"`
	PangkatPelaksanaStrukturalTahun     int `json:"pangkat_pelaksana_struktural_tahun"`
}

func updatePengaturanKenaikanGajiBerkala(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p pengaturanKenaikanGajiBerkalaPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	nilaiValid := func(n int) bool { return n >= 1 && n <= 10 }
	if !nilaiValid(p.GajiBerkalaFungsionalTahun) || !nilaiValid(p.GajiBerkalaPelaksanaStrukturalTahun) ||
		!nilaiValid(p.PangkatFungsionalTahun) || !nilaiValid(p.PangkatPelaksanaStrukturalTahun) {
		utils.Error(w, http.StatusBadRequest, "interval harus dalam rentang yang wajar (1-10 tahun)")
		return
	}
	updates := map[string]interface{}{
		"gaji_berkala_fungsional_tahun":           p.GajiBerkalaFungsionalTahun,
		"gaji_berkala_pelaksana_struktural_tahun": p.GajiBerkalaPelaksanaStrukturalTahun,
		"pangkat_fungsional_tahun":                p.PangkatFungsionalTahun,
		"pangkat_pelaksana_struktural_tahun":      p.PangkatPelaksanaStrukturalTahun,
	}
	if err := db.Model(&models.PengaturanKenaikanGajiBerkala{}).Where("id = ?", 1).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengaturan: "+err.Error())
		return
	}
	item, _ := getPengaturanKenaikanGajiBerkalaRow(db)
	utils.Success(w, "pengaturan kenaikan gaji berkala berhasil disimpan", item)
}
