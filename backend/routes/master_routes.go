package routes

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"cuti-app/handlers"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// jamHHMMPattern memvalidasi format "HH:MM" untuk kolom Excel jam kerja
// khusus unit kerja/sekolah (lihat jamUnitKerjaColumn) -- format yang sama
// dipakai handlers.parseJamToMinutes untuk field jam sejenis pada
// PengaturanAbsensi, tapi fungsi itu tidak diekspor dari package handlers
// sehingga validasi format di sini ditulis ulang secara sederhana.
var jamHHMMPattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// tempatKerjaLabel/parseTempatKerjaLabel menerjemahkan kolom Excel "Tempat
// Kerja" (lihat models.UnitKerja.TempatKerja & models.TempatKerjaDinas/
// TempatKerjaSekolah) antara nilai tersimpan ("dinas"/"sekolah") dan label
// yang enak dibaca administrator di Excel ("Dinas/Kantor"/"Sekolah"). Parse
// menerima beberapa variasi teks umum (case-insensitive) supaya tidak
// terlalu kaku saat mengisi/meng-import.
func tempatKerjaLabel(v string) string {
	if v == models.TempatKerjaSekolah {
		return "Sekolah"
	}
	return "Dinas/Kantor"
}

func parseTempatKerjaLabel(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sekolah":
		return models.TempatKerjaSekolah, nil
	case "dinas", "kantor", "dinas/kantor", "dinas / kantor":
		return models.TempatKerjaDinas, nil
	default:
		return "", fmt.Errorf(`tempat kerja harus salah satu dari "Dinas/Kantor" atau "Sekolah", bukan "%s"`, raw)
	}
}

// jamUnitKerjaColumn membuat satu kolom Excel untuk salah satu dari kelima
// field jam kerja KHUSUS unit kerja/sekolah pada models.UnitKerja (lihat
// komentar pada struct itu) -- field yang mana ditentukan lewat fieldPtr
// (mengembalikan alamat field **string bersangkutan pada instance
// UnitKerja). Kosong berarti unit kerja ini belum diberi jam khusus untuk
// field itu (jatuh kembali ke default sekolah/dinas "global").
func jamUnitKerjaColumn(header, example string, fieldPtr func(*models.UnitKerja) **string) utils.ExcelColumn {
	return utils.ExcelColumn{
		Header:  header,
		Example: example,
		Get: func(i interface{}) string {
			uk := i.(models.UnitKerja)
			p := *fieldPtr(&uk)
			if p != nil {
				return *p
			}
			return ""
		},
		Set: func(i interface{}, raw string) error {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				return nil
			}
			if !jamHHMMPattern.MatchString(raw) {
				return fmt.Errorf("%s harus berformat HH:MM (contoh %s)", header, example)
			}
			*fieldPtr(i.(*models.UnitKerja)) = &raw
			return nil
		},
	}
}

// RegisterMasterRoutes wires up all simple master/lookup tables through the
// generic CRUD + excel engine. Only "tgl_merah" (Tanggal Merah / hari libur,
// which role "admin"/Admin Kepegawaian keeps updated day-to-day) is opened to
// administrator + admin; every other master/reference table (role, jabatan,
// unit_kerja, status, pangkat, golongan, pangkat_gol, jenis_cuti,
// pola_hari_kerja) is administrator-only. This does NOT affect the read-only
// "/api/ref/*" lookup endpoints (reference.go) that dropdowns elsewhere
// (Pegawai, Pengajuan Cuti, Profil Saya, dsb.) use -- those stay open to any
// authenticated role regardless of who owns full CRUD rights here.
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
			{Header: "Jenis Jabatan (pelaksana/struktural/fungsional)", Example: "pelaksana",
				Get: func(i interface{}) string {
					jj := i.(models.Jabatan).JenisJabatan
					if jj == "" {
						return models.JenisJabatanPelaksana
					}
					return jj
				},
				Set: func(i interface{}, raw string) error {
					jj := strings.ToLower(strings.TrimSpace(raw))
					switch jj {
					case models.JenisJabatanPelaksana, models.JenisJabatanStruktural, models.JenisJabatanFungsional:
						i.(*models.Jabatan).JenisJabatan = jj
					case "":
						i.(*models.Jabatan).JenisJabatan = models.JenisJabatanPelaksana
					default:
						return fmt.Errorf("jenis jabatan harus salah satu dari: pelaksana, struktural, fungsional")
					}
					return nil
				}},
		},
	}, "administrator")

	// ---- kecamatan ----
	handlers.RegisterCrud(mux, db, "/api/kecamatan", handlers.CrudConfig[models.Kecamatan]{
		FileBaseName: "kecamatan",
		SearchFields: []string{"nama"},
		Columns: []utils.ExcelColumn{
			{Header: "Nama Kecamatan", Required: true, Example: "Bungku Utara",
				Get: func(i interface{}) string { return i.(models.Kecamatan).Nama },
				Set: func(i interface{}, raw string) error { i.(*models.Kecamatan).Nama = raw; return nil }},
		},
	}, "administrator")

	// ---- unit_kerja (unit kerja/sekolah -- boleh dikelompokkan per
	// kecamatan & diberi titik koordinat + radius absen sendiri-sendiri,
	// lihat komentar pada models.UnitKerja) ----
	handlers.RegisterCrud(mux, db, "/api/unit-kerja", handlers.CrudConfig[models.UnitKerja]{
		FileBaseName: "unit_kerja",
		SearchFields: []string{"unit"},
		Preloads:     []string{"Kecamatan"},
		Columns: []utils.ExcelColumn{
			{Header: "Unit Kerja", Required: true, Example: "Bidang Pelayanan",
				Get: func(i interface{}) string { return i.(models.UnitKerja).Unit },
				Set: func(i interface{}, raw string) error { i.(*models.UnitKerja).Unit = raw; return nil }},
			{Header: "Kecamatan", Example: "Bungku Utara",
				Get: func(i interface{}) string {
					uk := i.(models.UnitKerja)
					if uk.Kecamatan != nil {
						return uk.Kecamatan.Nama
					}
					return ""
				},
				Set: func(i interface{}, raw string) error {
					raw = strings.TrimSpace(raw)
					if raw == "" {
						return nil
					}
					var k models.Kecamatan
					if err := db.Where("nama ILIKE ?", raw).First(&k).Error; err != nil {
						return fmt.Errorf("kecamatan '%s' belum terdaftar, tambahkan dulu di menu Kecamatan", raw)
					}
					id := k.ID
					i.(*models.UnitKerja).IDKecamatan = &id
					return nil
				}},
			{Header: "Tempat Kerja (Dinas/Kantor atau Sekolah)", Example: "Dinas/Kantor",
				Get: func(i interface{}) string {
					uk := i.(models.UnitKerja)
					if uk.TempatKerja == nil {
						return ""
					}
					return tempatKerjaLabel(*uk.TempatKerja)
				},
				Set: func(i interface{}, raw string) error {
					raw = strings.TrimSpace(raw)
					if raw == "" {
						return nil
					}
					v, err := parseTempatKerjaLabel(raw)
					if err != nil {
						return err
					}
					i.(*models.UnitKerja).TempatKerja = &v
					return nil
				}},
			{Header: "Latitude", Example: "-1.976688",
				Get: func(i interface{}) string {
					uk := i.(models.UnitKerja)
					if uk.Lat != nil {
						return fmt.Sprintf("%f", *uk.Lat)
					}
					return ""
				},
				Set: func(i interface{}, raw string) error {
					raw = strings.TrimSpace(raw)
					if raw == "" {
						return nil
					}
					v, err := strconv.ParseFloat(raw, 64)
					if err != nil || v < -90 || v > 90 {
						return fmt.Errorf("latitude harus berupa angka antara -90 dan 90")
					}
					i.(*models.UnitKerja).Lat = &v
					return nil
				}},
			{Header: "Longitude", Example: "121.335284",
				Get: func(i interface{}) string {
					uk := i.(models.UnitKerja)
					if uk.Lng != nil {
						return fmt.Sprintf("%f", *uk.Lng)
					}
					return ""
				},
				Set: func(i interface{}, raw string) error {
					raw = strings.TrimSpace(raw)
					if raw == "" {
						return nil
					}
					v, err := strconv.ParseFloat(raw, 64)
					if err != nil || v < -180 || v > 180 {
						return fmt.Errorf("longitude harus berupa angka antara -180 dan 180")
					}
					i.(*models.UnitKerja).Lng = &v
					return nil
				}},
			{Header: "Radius Absen (meter)", Example: "50",
				Get: func(i interface{}) string {
					uk := i.(models.UnitKerja)
					if uk.RadiusMeter != nil {
						return fmt.Sprintf("%d", *uk.RadiusMeter)
					}
					return ""
				},
				Set: func(i interface{}, raw string) error {
					raw = strings.TrimSpace(raw)
					if raw == "" {
						return nil
					}
					v, err := utils.ParseIntCell(raw)
					if err != nil || v <= 0 {
						return fmt.Errorf("radius absen harus berupa angka lebih dari 0")
					}
					i.(*models.UnitKerja).RadiusMeter = &v
					return nil
				}},
			jamUnitKerjaColumn("Jam Mulai Absen Pagi", "06:30", func(uk *models.UnitKerja) **string { return &uk.JamMulaiPagi }),
			jamUnitKerjaColumn("Jam Batas Absen Pagi (terlambat)", "07:00", func(uk *models.UnitKerja) **string { return &uk.JamBatasPagi }),
			jamUnitKerjaColumn("Jam Tutup Absen Masuk", "08:00", func(uk *models.UnitKerja) **string { return &uk.JamTutupPagi }),
			jamUnitKerjaColumn("Jam Mulai Absen Pulang", "12:30", func(uk *models.UnitKerja) **string { return &uk.JamMulaiPulang }),
			jamUnitKerjaColumn("Jam Tutup Absen Pulang", "15:00", func(uk *models.UnitKerja) **string { return &uk.JamTutupPulang }),
		},
	}, "administrator")

	// ---- status ----
	handlers.RegisterCrud(mux, db, "/api/status", handlers.CrudConfig[models.Status]{
		FileBaseName: "status",
		SearchFields: []string{"status"},
		Columns: []utils.ExcelColumn{
			{Header: "Status", Required: true, Example: "Aktif",
				Get: func(i interface{}) string { return i.(models.Status).Status },
				Set: func(i interface{}, raw string) error { i.(*models.Status).Status = raw; return nil }},
		},
	}, "administrator")

	// ---- pangkat ----
	handlers.RegisterCrud(mux, db, "/api/pangkat", handlers.CrudConfig[models.Pangkat]{
		FileBaseName: "pangkat",
		SearchFields: []string{"pangkat"},
		Columns: []utils.ExcelColumn{
			{Header: "Pangkat", Required: true, Example: "Penata Muda",
				Get: func(i interface{}) string { return i.(models.Pangkat).Pangkat },
				Set: func(i interface{}, raw string) error { i.(*models.Pangkat).Pangkat = raw; return nil }},
		},
	}, "administrator")

	// ---- golongan ----
	handlers.RegisterCrud(mux, db, "/api/golongan", handlers.CrudConfig[models.Golongan]{
		FileBaseName: "golongan",
		SearchFields: []string{"gol"},
		Columns: []utils.ExcelColumn{
			{Header: "Golongan", Required: true, Example: "III/a",
				Get: func(i interface{}) string { return i.(models.Golongan).Gol },
				Set: func(i interface{}, raw string) error { i.(*models.Golongan).Gol = raw; return nil }},
		},
	}, "administrator")

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
	}, "administrator")

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
	}, "administrator")

	// ---- pola_hari_kerja ----
	handlers.RegisterCrud(mux, db, "/api/pola-hari-kerja", handlers.CrudConfig[models.PolaHariKerja]{
		FileBaseName: "pola_hari_kerja",
		SearchFields: []string{"pola"},
		Columns: []utils.ExcelColumn{
			{Header: "Pola Hari Kerja", Required: true, Example: "5 Hari Kerja (Senin-Jumat)",
				Get: func(i interface{}) string { return i.(models.PolaHariKerja).Pola },
				Set: func(i interface{}, raw string) error { i.(*models.PolaHariKerja).Pola = raw; return nil }},
		},
	}, "administrator")

	// ---- tgl_merah ----
	// AfterChange: setiap kali daftar tanggal merah berubah (ditambah, diedit,
	// dihapus, atau diimport), langsung hitung ulang jumlah hari & jatah cuti
	// terpakai untuk semua pengajuan cuti yang masih menunggu/disetujui --
	// supaya efeknya langsung terlihat begitu tanggal merah disimpan, tanpa
	// harus membuka/mengedit pengajuan cuti yang bersangkutan.
	handlers.RegisterCrud(mux, db, "/api/tgl-merah", handlers.CrudConfig[models.TglMerah]{
		FileBaseName: "tgl_merah",
		OrderBy:      "tgl asc",
		AfterChange: func(db *gorm.DB) {
			handlers.ResyncActivePengajuanDays(db)
		},
		Columns: []utils.ExcelColumn{
			{Header: "Tanggal (DD-MM-YYYY)", Required: true, Example: "01-01-2026",
				Get: func(i interface{}) string { t := i.(models.TglMerah).Tgl; return t.Format("02-01-2006") },
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
