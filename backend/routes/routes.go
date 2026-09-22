package routes

import (
	"net/http"

	"cuti-app/handlers"
	"cuti-app/middleware"

	"gorm.io/gorm"
)

// SetupRoutes builds the full HTTP handler (mux + CORS) for the application.
func SetupRoutes(db *gorm.DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/login", handlers.LoginHandler(db))
	mux.Handle("GET /api/me", middleware.Chain(handlers.MeHandler(db), middleware.Auth, middleware.RequireActiveUser(db)))

	handlers.RegisterDashboardRoutes(mux, db)
	handlers.RegisterReferenceRoutes(mux, db)
	RegisterMasterRoutes(mux, db)
	handlers.RegisterUserRoutes(mux, db)
	handlers.RegisterPegawaiRoutes(mux, db)
	handlers.RegisterJatahCutiRoutes(mux, db)
	handlers.RegisterPengajuanCutiRoutes(mux, db)
	handlers.RegisterPengaturanSuratRoutes(mux, db)
	handlers.RegisterPerubahanDataRoutes(mux, db)
	handlers.RegisterPengajuanPensiunRoutes(mux, db)
	handlers.RegisterPengaturanKenaikanGajiBerkalaRoutes(mux, db)
	handlers.RegisterAbsensiRoutes(mux, db)
	handlers.RegisterPengajuanSuratKolektifRoutes(mux, db)
	handlers.RegisterNotifikasiRoutes(mux, db)
	handlers.RegisterTemplateSuratRoutes(mux, db)
	handlers.RegisterTppRoutes(mux, db)
	handlers.RegisterShiftKerjaRoutes(mux, db)
	handlers.RegisterKp4Routes(mux, db)

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	return middleware.CORS(mux)
}
