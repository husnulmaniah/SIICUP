package handlers

import (
	"net/http"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// RegisterReferenceRoutes exposes small read-only lookup lists needed by forms
// (e.g. the cuti submission dropdowns) to ANY authenticated role, regardless
// of who owns full CRUD rights over the underlying master table.
func RegisterReferenceRoutes(mux *http.ServeMux, db *gorm.DB) {
	any := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole())
	}

	mux.Handle("GET /api/ref/jenis-cuti", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.JenisCuti
		db.Order("jenis asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/pola-hari-kerja", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.PolaHariKerja
		db.Order("pola asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/jabatan", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.Jabatan
		db.Order("jabatan asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/unit-kerja", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.UnitKerja
		db.Order("unit asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/status", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.Status
		db.Order("status asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/pangkat", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.Pangkat
		db.Order("pangkat asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/golongan", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.Golongan
		db.Order("gol asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/pangkat-gol", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.PangkatGol
		db.Preload("Pangkat").Preload("Gol").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/pegawai", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.Pegawai
		db.Select("id", "nip", "nama").Order("nama asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
	mux.Handle("GET /api/ref/role", any(func(w http.ResponseWriter, r *http.Request) {
		var items []models.Role
		db.Order("role asc").Find(&items)
		utils.Success(w, "ok", items)
	}))
}
