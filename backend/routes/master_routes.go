package routes

import (
	"fmt"
	"net/http"

	"cuti-app/handlers"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// RegisterMasterRoutes wires up all simple master/lookup tables through the
// generic CRUD + excel engine. "role" is restricted to administrator only;
// the rest are administrator + admin.
func RegisterMasterRoutes(mux *http.ServeMux, db *gorm.DB) {

	// ---- role ----
	handlers.RegisterCrud(mux, db, "/api/role", handlers.CrudConfig[models.Role]{
		FileBaseName: "role",
		SearchFields: []string{"role"},
		Columns: []utils.ExcelColumn{
			{Header: "Nama Role", Required: true, Example: "pegawai",
				Get: func(i interface{}) string { return i.(models.Role).Role },
				Set: func(i interface{}, raw string) error { i.(*models.Role).Role = raw; return nil }},
		},
	}, "administrator")

	// ---- jabatan ----
	handlers.RegisterCrud(mux, db, "/api/jabatan", handlers.CrudConfig[models.Jabatan]{
		FileBaseName: "jabatan",
		SearchFields: []string{"jabatan"},
		Columns: []utils.ExcelColumn{
			{Header: "Nama Jabatan", Required: true, Example: "Kepala Bidang",
				Get: func(i interface{}) string { return i.(models.Jabatan).Jabatan },
				Set: func(i interface{}, raw string) error { i.(*models.Jabatan).Jabatan = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- unit_kerja ----
	handlers.RegisterCrud(mux, db, "/api/unit-kerja", handlers.CrudConfig[models.UnitKerja]{
		FileBaseName: "unit_kerja",
		SearchFields: []string{"unit"},
		Columns: []utils.ExcelColumn{
			{Header: "Unit Kerja", Required: true, Example: "Bidang Pelayanan",
				Get: func(i interface{}) string { return i.(models.UnitKerja).Unit },
				Set: func(i interface{}, raw string) error { i.(*models.UnitKerja).Unit = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- status ----
	handlers.RegisterCrud(mux, db, "/api/status", handlers.CrudConfig[models.Status]{
		FileBaseName: "status",
		SearchFields: []string{"status"},
		Columns: []utils.ExcelColumn{
			{Header: "Status", Required: true, Example: "Aktif",
				Get: func(i interface{}) string { return i.(models.Status).Status },
				Set: func(i interface{}, raw string) error { i.(*models.Status).Status = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- pangkat ----
	handlers.RegisterCrud(mux, db, "/api/pangkat", handlers.CrudConfig[models.Pangkat]{
		FileBaseName: "pangkat",
		SearchFields: []string{"pangkat"},
		Columns: []utils.ExcelColumn{
			{Header: "Pangkat", Required: true, Example: "Penata Muda",
				Get: func(i interface{}) string { return i.(models.Pangkat).Pangkat },
				Set: func(i interface{}, raw string) error { i.(*models.Pangkat).Pangkat = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- golongan ----
	handlers.RegisterCrud(mux, db, "/api/golongan", handlers.CrudConfig[models.Golongan]{
		FileBaseName: "golongan",
		SearchFields: []string{"gol"},
		Columns: []utils.ExcelColumn{
			{Header: "Golongan", Required: true, Example: "III/a",
				Get: func(i interface{}) string { return i.(models.Golongan).Gol },
				Set: func(i interface{}, raw string) error { i.(*models.Golongan).Gol = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- pangkat_gol (foreign keys resolved by name during import/export) ----
	handlers.RegisterCrud(mux, db, "/api/pangkat-gol", handlers.CrudConfig[models.PangkatGol]{
		FileBaseName: "pangkat_gol",
		Preloads:     []string{"Pangkat", "Gol"},
		Columns: []utils.ExcelColumn{
			{Header: "Pangkat", Required: true, Example: "Penata Muda",
				Get: func(i interface{}) string {
					pg := i.(models.PangkatGol)
					if pg.Pangkat != nil {
						return pg.Pangkat.Pangkat
					}
					return ""
				},
				Set: func(i interface{}, raw string) error {
					var p models.Pangkat
					if err := db.Where("pangkat ILIKE ?", raw).First(&p).Error; err != nil {
						return fmt.Errorf("pangkat '%s' belum terdaftar, tambahkan dulu di menu Pangkat", raw)
					}
					i.(*models.PangkatGol).IDPangkat = p.ID
					return nil
				}},
			{Header: "Golongan", Required: true, Example: "III/a",
				Get: func(i interface{}) string {
					pg := i.(models.PangkatGol)
					if pg.Gol != nil {
						return pg.Gol.Gol
					}
					return ""
				},
				Set: func(i interface{}, raw string) error {
					var g models.Golongan
					if err := db.Where("gol ILIKE ?", raw).First(&g).Error; err != nil {
						return fmt.Errorf("golongan '%s' belum terdaftar, tambahkan dulu di menu Golongan", raw)
					}
					i.(*models.PangkatGol).IDGol = g.ID
					return nil
				}},
		},
	}, "administrator", "admin")

	// ---- jenis_cuti ----
	handlers.RegisterCrud(mux, db, "/api/jenis-cuti", handlers.CrudConfig[models.JenisCuti]{
		FileBaseName: "jenis_cuti",
		SearchFields: []string{"jenis"},
		Columns: []utils.ExcelColumn{
			{Header: "Jenis Cuti", Required: true, Example: "Cuti Tahunan",
				Get: func(i interface{}) string { return i.(models.JenisCuti).Jenis },
				Set: func(i interface{}, raw string) error { i.(*models.JenisCuti).Jenis = raw; return nil }},
			{Header: "Jatah Default (hari)", Example: "12",
				Get: func(i interface{}) string { return fmt.Sprintf("%d", i.(models.JenisCuti).DefaultJatah) },
				Set: func(i interface{}, raw string) error {
					v, err := utils.ParseIntCell(raw)
					if err != nil {
						return fmt.Errorf("harus berupa angka")
					}
					i.(*models.JenisCuti).DefaultJatah = v
					return nil
				}},
			{Header: "Keterangan", Example: "Maksimal 12 hari per tahun",
				Get: func(i interface{}) string { return i.(models.JenisCuti).Keterangan },
				Set: func(i interface{}, raw string) error { i.(*models.JenisCuti).Keterangan = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- pola_hari_kerja ----
	handlers.RegisterCrud(mux, db, "/api/pola-hari-kerja", handlers.CrudConfig[models.PolaHariKerja]{
		FileBaseName: "pola_hari_kerja",
		SearchFields: []string{"pola"},
		Columns: []utils.ExcelColumn{
			{Header: "Pola Hari Kerja", Required: true, Example: "5 Hari Kerja (Senin-Jumat)",
				Get: func(i interface{}) string { return i.(models.PolaHariKerja).Pola },
				Set: func(i interface{}, raw string) error { i.(*models.PolaHariKerja).Pola = raw; return nil }},
		},
	}, "administrator", "admin")

	// ---- tgl_merah ----
	handlers.RegisterCrud(mux, db, "/api/tgl-merah", handlers.CrudConfig[models.TglMerah]{
		FileBaseName: "tgl_merah",
		OrderBy:      "tgl asc",
		Columns: []utils.ExcelColumn{
			{Header: "Tanggal (YYYY-MM-DD)", Required: true, Example: "2026-01-01",
				Get: func(i interface{}) string { t := i.(models.TglMerah).Tgl; return t.Format("2006-01-02") },
				Set: func(i interface{}, raw string) error {
					t, err := utils.ParseDateCell(raw)
					if err != nil {
						return err
					}
					i.(*models.TglMerah).Tgl = t
					return nil
				}},
			{Header: "Keterangan", Required: true, Example: "Tahun Baru Masehi",
				Get: func(i interface{}) string { return i.(models.TglMerah).Keterangan },
				Set: func(i interface{}, raw string) error { i.(*models.TglMerah).Keterangan = raw; return nil }},
		},
	}, "administrator", "admin")
}
