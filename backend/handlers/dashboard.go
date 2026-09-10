package handlers

import (
	"net/http"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

func RegisterDashboardRoutes(mux *http.ServeMux, db *gorm.DB) {
	mux.Handle("GET /api/dashboard", middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
		dashboardHandler(w, r, db)
	}, middleware.Auth, middleware.RequireRole()))
}

func dashboardHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	currentYear := time.Now().Year()

	switch claims.RoleName {
	case "administrator", "admin":
		var totalPegawai, totalPending, totalDisetujui, totalDitolak int64
		db.Model(&models.Pegawai{}).Count(&totalPegawai)
		db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusDisetuju).Count(&totalDisetujui)
		db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusDitolak).Count(&totalDitolak)

		var recent []models.PengajuanCuti
		db.Preload("Pegawai").Preload("JenisCuti").Order("created_at desc").Limit(8).Find(&recent)

		utils.Success(w, "ok", map[string]interface{}{
			"role":            claims.RoleName,
			"total_pegawai":   totalPegawai,
			"total_pending":   totalPending,
			"total_disetujui": totalDisetujui,
			"total_ditolak":   totalDitolak,
			"pengajuan_terbaru": recent,
		})

	case "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", map[string]interface{}{"role": claims.RoleName})
			return
		}
		var totalBawahan, totalPending, totalDisetujui int64
		db.Model(&models.Pegawai{}).Where("id_atasan = ?", *claims.IDPegawai).Count(&totalBawahan)
		sub := "id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)"
		db.Model(&models.PengajuanCuti{}).Where(sub, *claims.IDPegawai).Where("status = ?", models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where(sub, *claims.IDPegawai).Where("status = ?", models.StatusDisetuju).Count(&totalDisetujui)

		var menunggu []models.PengajuanCuti
		db.Preload("Pegawai").Preload("JenisCuti").Where(sub, *claims.IDPegawai).
			Where("status = ?", models.StatusPending).Order("created_at asc").Limit(10).Find(&menunggu)

		utils.Success(w, "ok", map[string]interface{}{
			"role":                claims.RoleName,
			"total_bawahan":       totalBawahan,
			"total_pending":       totalPending,
			"total_disetujui":     totalDisetujui,
			"menunggu_persetujuan": menunggu,
		})

	case "pegawai":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", map[string]interface{}{"role": claims.RoleName})
			return
		}
		var jatah models.JatahCuti
		db.Where("id_pegawai = ? AND tahun = ?", *claims.IDPegawai, currentYear).First(&jatah)

		var riwayat []models.PengajuanCuti
		db.Preload("JenisCuti").Where("id_pegawai = ?", *claims.IDPegawai).
			Order("created_at desc").Limit(10).Find(&riwayat)

		var totalPending, totalDisetujui int64
		db.Model(&models.PengajuanCuti{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusDisetuju).Count(&totalDisetujui)

		utils.Success(w, "ok", map[string]interface{}{
			"role":            claims.RoleName,
			"jatah_tahun_ini": jatah.JumlahHari,
			"terpakai":        jatah.Terpakai,
			"sisa":            jatah.JumlahHari - jatah.Terpakai,
			"total_pending":   totalPending,
			"total_disetujui": totalDisetujui,
			"riwayat_cuti":    riwayat,
		})

	default:
		utils.Success(w, "ok", map[string]interface{}{"role": claims.RoleName})
	}
}
