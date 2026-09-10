package handlers

import (
	"encoding/json"
	"net/http"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// settings.go exposes the small "Pengaturan Formulir" singleton (nama & NIP
// Kepala Dinas yang dicetak sebagai penandatangan pada formulir cuti) -- a
// single row (ID=1) rather than a list, so it doesn't go through the generic
// CRUD engine used for the other master data tables.

type pengaturanSuratPayload struct {
	NamaKepalaDinas string `json:"nama_kepala_dinas"`
	NipKepalaDinas  string `json:"nip_kepala_dinas"`
}

func RegisterPengaturanSuratRoutes(mux *http.ServeMux, db *gorm.DB) {
	manage := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole("administrator", "admin"))
	}
	mux.Handle("GET /api/pengaturan-surat", manage(func(w http.ResponseWriter, r *http.Request) { getPengaturanSurat(w, r, db) }))
	mux.Handle("PUT /api/pengaturan-surat", manage(func(w http.ResponseWriter, r *http.Request) { updatePengaturanSurat(w, r, db) }))
}

func getPengaturanSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var item models.PengaturanSurat
	if err := db.First(&item, 1).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pengaturan belum tersedia")
		return
	}
	utils.Success(w, "ok", item)
}

func updatePengaturanSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p pengaturanSuratPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	var item models.PengaturanSurat
	if err := db.First(&item, 1).Error; err != nil {
		// belum ada (seharusnya sudah dibuat EnsurePengaturanSurat saat startup) -- buat sekarang.
		item = models.PengaturanSurat{ID: 1}
	}
	item.NamaKepalaDinas = p.NamaKepalaDinas
	item.NipKepalaDinas = p.NipKepalaDinas
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan pengaturan: "+err.Error())
		return
	}
	utils.Success(w, "pengaturan formulir berhasil disimpan", item)
}
