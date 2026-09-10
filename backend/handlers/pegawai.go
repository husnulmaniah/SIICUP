package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// dokumenFileFields lists the GORM struct field names (bytea columns) that
// must be excluded from ordinary list/detail queries so we don't drag large
// binary blobs along with every request; they're only fetched by the
// dedicated download endpoint below.
var dokumenFileFields = []string{"SkTerakhirFile", "SkKgbFile", "SkPensiunFile"}

var pegawaiPreloads = []string{"Jabatan", "UnitKerja", "PangkatGol.Pangkat", "PangkatGol.Gol", "Status", "Atasan"}

func pegawaiExcelColumns(db *gorm.DB) []utils.ExcelColumn {
	return []utils.ExcelColumn{
		{Header: "NIP", Required: true, Example: "198501012010011001",
			Get: func(i interface{}) string { return i.(models.Pegawai).NIP },
			Set: func(i interface{}, raw string) error { i.(*models.Pegawai).NIP = raw; return nil }},
		{Header: "Nama", Required: true, Example: "Budi Santoso",
			Get: func(i interface{}) string { return i.(models.Pegawai).Nama },
			Set: func(i interface{}, raw string) error { i.(*models.Pegawai).Nama = raw; return nil }},
		{Header: "Jabatan", Example: "Kepala Bidang",
			Get: func(i interface{}) string {
				p := i.(models.Pegawai)
				if p.Jabatan != nil {
					return p.Jabatan.Jabatan
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var j models.Jabatan
				if err := db.Where("jabatan ILIKE ?", raw).First(&j).Error; err != nil {
					return fmt.Errorf("jabatan '%s' belum terdaftar, tambahkan dulu di menu Jabatan", raw)
				}
				id := j.ID
				i.(*models.Pegawai).IDJabatan = &id
				return nil
			}},
		{Header: "Unit Kerja", Example: "Bidang Pelayanan",
			Get: func(i interface{}) string {
				p := i.(models.Pegawai)
				if p.UnitKerja != nil {
					return p.UnitKerja.Unit
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var u models.UnitKerja
				if err := db.Where("unit ILIKE ?", raw).First(&u).Error; err != nil {
					return fmt.Errorf("unit kerja '%s' belum terdaftar, tambahkan dulu di menu Unit Kerja", raw)
				}
				id := u.ID
				i.(*models.Pegawai).IDUnitKerja = &id
				return nil
			}},
		{Header: "Pangkat", Example: "Penata Muda",
			Get: func(i interface{}) string {
				p := i.(models.Pegawai)
				if p.PangkatGol != nil && p.PangkatGol.Pangkat != nil {
					return p.PangkatGol.Pangkat.Pangkat
				}
				return ""
			},
			Set: func(i interface{}, raw string) error { return nil }}, // resolved together with Golongan below
		{Header: "Golongan", Example: "III/a",
			Get: func(i interface{}) string {
				p := i.(models.Pegawai)
				if p.PangkatGol != nil && p.PangkatGol.Gol != nil {
					return p.PangkatGol.Gol.Gol
				}
				return ""
			},
			Set: func(i interface{}, raw string) error { return nil }}, // handled in resolvePangkatGol pass
		{Header: "Tempat Tugas", Example: "Kantor Pusat",
			Get: func(i interface{}) string { return i.(models.Pegawai).TempatTgs },
			Set: func(i interface{}, raw string) error { i.(*models.Pegawai).TempatTgs = raw; return nil }},
		{Header: "TMT (DD-MM-YYYY)", Example: "01-01-2010",
			Get: func(i interface{}) string { return utils.FormatDateCell(i.(models.Pegawai).TMT) },
			Set: func(i interface{}, raw string) error {
				t, err := utils.ParseDateCell(raw)
				if err != nil {
					return err
				}
				i.(*models.Pegawai).TMT = &t
				return nil
			}},
		{Header: "No HP", Example: "081234567890",
			Get: func(i interface{}) string { return i.(models.Pegawai).NoHP },
			Set: func(i interface{}, raw string) error { i.(*models.Pegawai).NoHP = raw; return nil }},
		{Header: "Email", Example: "budi@instansi.go.id",
			Get: func(i interface{}) string { return i.(models.Pegawai).Email },
			Set: func(i interface{}, raw string) error { i.(*models.Pegawai).Email = raw; return nil }},
		{Header: "Status", Example: "Aktif",
			Get: func(i interface{}) string {
				p := i.(models.Pegawai)
				if p.Status != nil {
					return p.Status.Status
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var s models.Status
				if err := db.Where("status ILIKE ?", raw).First(&s).Error; err != nil {
					return fmt.Errorf("status '%s' belum terdaftar, tambahkan dulu di menu Status", raw)
				}
				id := s.ID
				i.(*models.Pegawai).IDStatus = &id
				return nil
			}},
		{Header: "NIP Atasan (opsional)", Example: "197001011995011001",
			Get: func(i interface{}) string {
				p := i.(models.Pegawai)
				if p.Atasan != nil {
					return p.Atasan.NIP
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				if raw == "" {
					return nil
				}
				var atasan models.Pegawai
				if err := db.Where("nip = ?", raw).First(&atasan).Error; err != nil {
					return fmt.Errorf("atasan dengan NIP '%s' tidak ditemukan", raw)
				}
				id := atasan.ID
				i.(*models.Pegawai).IDAtasan = &id
				return nil
			}},
	}
}

// resolvePangkatGol looks up (or creates) a pangkat_gol combination row from the
// raw "Pangkat" and "Golongan" text columns of an import row, since the pair maps
// to a single id_pangkat_gol foreign key on pegawai.
func resolvePangkatGol(db *gorm.DB, row []string, pangkatColIdx, golColIdx int) (*uint, error) {
	if pangkatColIdx >= len(row) || golColIdx >= len(row) {
		return nil, nil
	}
	pangkatName := strings.TrimSpace(row[pangkatColIdx])
	golName := strings.TrimSpace(row[golColIdx])
	if pangkatName == "" && golName == "" {
		return nil, nil
	}
	var pangkat models.Pangkat
	if err := db.Where("pangkat ILIKE ?", pangkatName).First(&pangkat).Error; err != nil {
		return nil, fmt.Errorf("pangkat '%s' belum terdaftar", pangkatName)
	}
	var gol models.Golongan
	if err := db.Where("gol ILIKE ?", golName).First(&gol).Error; err != nil {
		return nil, fmt.Errorf("golongan '%s' belum terdaftar", golName)
	}
	var pg models.PangkatGol
	err := db.Where("id_pangkat = ? AND id_gol = ?", pangkat.ID, gol.ID).First(&pg).Error
	if err != nil {
		// auto-create the combination if it doesn't exist yet
		pg = models.PangkatGol{IDPangkat: pangkat.ID, IDGol: gol.ID}
		if err := db.Create(&pg).Error; err != nil {
			return nil, fmt.Errorf("gagal membuat kombinasi pangkat/golongan")
		}
	}
	id := pg.ID
	return &id, nil
}

func RegisterPegawaiRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }

	mux.Handle("GET /api/pegawai", anyRole(func(w http.ResponseWriter, r *http.Request) { listPegawai(w, r, db) }))
	mux.Handle("GET /api/pegawai/me", anyRole(func(w http.ResponseWriter, r *http.Request) { mePegawai(w, r, db) }))
	mux.Handle("GET /api/pegawai/export", manage(func(w http.ResponseWriter, r *http.Request) { exportPegawai(w, r, db) }))
	mux.Handle("GET /api/pegawai/template", manage(func(w http.ResponseWriter, r *http.Request) {
		writeXlsxResponse(w, utils.GenerateTemplate(pegawaiExcelColumns(db)), "template_pegawai.xlsx")
	}))
	mux.Handle("POST /api/pegawai/import", manage(func(w http.ResponseWriter, r *http.Request) { importPegawai(w, r, db) }))
	mux.Handle("GET /api/pegawai/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getPegawai(w, r, db) }))
	mux.Handle("POST /api/pegawai", manage(func(w http.ResponseWriter, r *http.Request) { createPegawai(w, r, db) }))
	mux.Handle("PUT /api/pegawai/{id}", manage(func(w http.ResponseWriter, r *http.Request) { updatePegawai(w, r, db) }))
	mux.Handle("DELETE /api/pegawai/{id}", manage(func(w http.ResponseWriter, r *http.Request) { deletePegawai(w, r, db) }))

	mux.Handle("GET /api/pegawai/{id}/dokumen/{jenis}", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadDokumenPegawai(w, r, db) }))
	mux.Handle("POST /api/pegawai/{id}/dokumen/{jenis}", manage(func(w http.ResponseWriter, r *http.Request) { uploadDokumenPegawai(w, r, db) }))
	mux.Handle("DELETE /api/pegawai/{id}/dokumen/{jenis}", manage(func(w http.ResponseWriter, r *http.Request) { deleteDokumenPegawai(w, r, db) }))
}

// validDokumenJenis restricts the {jenis} path segment to the three known
// document slots described in the UI: SK Terakhir, SK Kenaikan Gaji
// Berkala, and SK Pensiun.
func validDokumenJenis(jenis string) bool {
	switch jenis {
	case "sk-terakhir", "sk-kgb", "sk-pensiun":
		return true
	}
	return false
}

func canAccessPegawaiRow(claims *utils.Claims, item *models.Pegawai) bool {
	if claims.RoleName == "administrator" || claims.RoleName == "admin" {
		return true
	}
	if claims.RoleName == "atasan" {
		return item.IDAtasan != nil && claims.IDPegawai != nil && *item.IDAtasan == *claims.IDPegawai
	}
	return claims.IDPegawai != nil && item.ID == *claims.IDPegawai
}

func downloadDokumenPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	jenis := r.PathValue("jenis")
	if !validDokumenJenis(jenis) {
		utils.Error(w, http.StatusBadRequest, "jenis dokumen tidak dikenal")
		return
	}
	var item models.Pegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPegawaiRow(claims, &item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}

	var filename string
	var data []byte
	switch jenis {
	case "sk-terakhir":
		filename, data = item.SkTerakhirNama, item.SkTerakhirFile
	case "sk-kgb":
		filename, data = item.SkKgbNama, item.SkKgbFile
	case "sk-pensiun":
		filename, data = item.SkPensiunNama, item.SkPensiunFile
	}
	if len(data) == 0 {
		utils.Error(w, http.StatusNotFound, "dokumen belum diupload")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Write(data)
}

func uploadDokumenPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	jenis := r.PathValue("jenis")
	if !validDokumenJenis(jenis) {
		utils.Error(w, http.StatusBadRequest, "jenis dokumen tidak dikenal")
		return
	}
	var item models.Pegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca file upload (maksimal 15MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "file tidak ditemukan (field 'file')")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "format file harus PDF, JPG, atau PNG")
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membaca isi file")
		return
	}

	updates := map[string]interface{}{}
	switch jenis {
	case "sk-terakhir":
		updates["sk_terakhir_nama"] = header.Filename
		updates["sk_terakhir_file"] = data
	case "sk-kgb":
		updates["sk_kgb_nama"] = header.Filename
		updates["sk_kgb_file"] = data
	case "sk-pensiun":
		updates["sk_pensiun_nama"] = header.Filename
		updates["sk_pensiun_file"] = data
	}
	if err := db.Model(&item).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan dokumen: "+err.Error())
		return
	}
	utils.Success(w, "dokumen berhasil diupload", map[string]string{"nama_file": header.Filename})
}

func deleteDokumenPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	jenis := r.PathValue("jenis")
	if !validDokumenJenis(jenis) {
		utils.Error(w, http.StatusBadRequest, "jenis dokumen tidak dikenal")
		return
	}
	var item models.Pegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	updates := map[string]interface{}{}
	switch jenis {
	case "sk-terakhir":
		updates["sk_terakhir_nama"] = ""
		updates["sk_terakhir_file"] = nil
	case "sk-kgb":
		updates["sk_kgb_nama"] = ""
		updates["sk_kgb_file"] = nil
	case "sk-pensiun":
		updates["sk_pensiun_nama"] = ""
		updates["sk_pensiun_file"] = nil
	}
	if err := db.Model(&item).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus dokumen: "+err.Error())
		return
	}
	utils.Success(w, "dokumen berhasil dihapus", nil)
}

func listPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 500 {
		pageSize = 25
	}
	search := strings.TrimSpace(q.Get("q"))

	query := db.Model(&models.Pegawai{}).Omit(dokumenFileFields...)
	for _, p := range pegawaiPreloads {
		query = query.Preload(p)
	}
	countQuery := db.Model(&models.Pegawai{})

	switch claims.RoleName {
	case "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.Pegawai{})
			return
		}
		query = query.Where("id_atasan = ?", *claims.IDPegawai)
		countQuery = countQuery.Where("id_atasan = ?", *claims.IDPegawai)
	case "pegawai":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.Pegawai{})
			return
		}
		query = query.Where("id = ?", *claims.IDPegawai)
		countQuery = countQuery.Where("id = ?", *claims.IDPegawai)
	}

	if search != "" {
		cond := "nama ILIKE ? OR nip ILIKE ?"
		query = query.Where(cond, "%"+search+"%", "%"+search+"%")
		countQuery = countQuery.Where(cond, "%"+search+"%", "%"+search+"%")
	}

	var total int64
	countQuery.Count(&total)
	var items []models.Pegawai
	if err := query.Order("nama asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}
	utils.SuccessMeta(w, "ok", items, map[string]interface{}{"page": page, "pageSize": pageSize, "total": total})
}

func mePegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusNotFound, "akun ini belum terhubung dengan data pegawai")
		return
	}
	var item models.Pegawai
	query := db.Omit(dokumenFileFields...)
	for _, p := range pegawaiPreloads {
		query = query.Preload(p)
	}
	if err := query.First(&item, "id = ?", *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}
	utils.Success(w, "ok", item)
}

func canAccessPegawai(claims *utils.Claims, id string) bool {
	if claims.RoleName == "administrator" || claims.RoleName == "admin" {
		return true
	}
	idNum, _ := strconv.Atoi(id)
	if claims.IDPegawai != nil && uint(idNum) == *claims.IDPegawai {
		return true
	}
	return false
}

func getPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")

	var item models.Pegawai
	query := db.Omit(dokumenFileFields...)
	for _, p := range pegawaiPreloads {
		query = query.Preload(p)
	}
	if err := query.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}

	if claims.RoleName == "atasan" {
		if item.IDAtasan == nil || claims.IDPegawai == nil || *item.IDAtasan != *claims.IDPegawai {
			utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
			return
		}
	} else if !canAccessPegawai(claims, id) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	utils.Success(w, "ok", item)
}

type pegawaiPayload struct {
	NIP          string `json:"nip"`
	Nama         string `json:"nama"`
	IDJabatan    *uint  `json:"id_jabatan"`
	IDUnitKerja  *uint  `json:"id_unit_kerja"`
	IDPangkatGol *uint  `json:"id_pangkat_gol"`
	TempatTgs    string `json:"tempat_tgs"`
	TMT          string `json:"tmt"`
	NoHP         string `json:"no_hp"`
	Email        string `json:"email"`
	IDStatus     *uint  `json:"id_status"`
	IDAtasan     *uint  `json:"id_atasan"`
}

func applyPegawaiPayload(item *models.Pegawai, p pegawaiPayload) error {
	item.NIP = p.NIP
	item.Nama = p.Nama
	item.IDJabatan = p.IDJabatan
	item.IDUnitKerja = p.IDUnitKerja
	item.IDPangkatGol = p.IDPangkatGol
	item.TempatTgs = p.TempatTgs
	item.NoHP = p.NoHP
	item.Email = p.Email
	item.IDStatus = p.IDStatus
	item.IDAtasan = p.IDAtasan
	if p.TMT != "" {
		t, err := utils.ParseDateCell(p.TMT)
		if err != nil {
			return err
		}
		item.TMT = &t
	}
	return nil
}

func createPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p pegawaiPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.NIP == "" || p.Nama == "" {
		utils.Error(w, http.StatusBadRequest, "NIP dan nama wajib diisi")
		return
	}
	var item models.Pegawai
	if err := applyPegawaiPayload(&item, p); err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan data (NIP mungkin sudah dipakai): "+err.Error())
		return
	}
	utils.Created(w, "pegawai berhasil ditambahkan", item)
}

func updatePegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.Pegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	var p pegawaiPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := applyPegawaiPayload(&item, p); err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal memperbarui data: "+err.Error())
		return
	}
	utils.Success(w, "pegawai berhasil diperbarui", item)
}

func deletePegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	if err := db.Delete(&models.Pegawai{}, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus data (kemungkinan masih memiliki data cuti/user terkait): "+err.Error())
		return
	}
	utils.Success(w, "pegawai berhasil dihapus", nil)
}

func exportPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var items []models.Pegawai
	query := db.Omit(dokumenFileFields...)
	for _, p := range pegawaiPreloads {
		query = query.Preload(p)
	}
	query.Order("nama asc").Find(&items)
	f, err := utils.ExportData(items, pegawaiExcelColumns(db))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXlsxResponse(w, f, "data_pegawai.xlsx")
}

func importPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca file upload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "file excel tidak ditemukan")
		return
	}
	defer file.Close()

	rows, err := utils.ReadRows(file)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if isReplaceMode(r) {
		if err := deleteAllRows(db, &models.Pegawai{}); err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal menghapus data lama (kemungkinan masih ada akun user/pengajuan cuti yang terhubung): "+err.Error())
			return
		}
	}
	cols := pegawaiExcelColumns(db)
	// column indexes: 0 NIP,1 Nama,2 Jabatan,3 UnitKerja,4 Pangkat,5 Golongan,6 TempatTugas,7 TMT,8 NoHP,9 Email,10 Status,11 NIP Atasan

	type rowError struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	}
	var rowErrors []rowError
	successCount := 0

	for i, row := range rows {
		blank := true
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		var item models.Pegawai
		errs := utils.ImportRow(&item, row, cols)

		if pgID, err := resolvePangkatGol(db, row, 4, 5); err != nil {
			errs = append(errs, err.Error())
		} else {
			item.IDPangkatGol = pgID
		}

		if len(errs) > 0 {
			rowErrors = append(rowErrors, rowError{Row: i + 2, Errors: errs})
			continue
		}
		if err := db.Create(&item).Error; err != nil {
			rowErrors = append(rowErrors, rowError{Row: i + 2, Errors: []string{"gagal simpan: " + err.Error()}})
			continue
		}
		successCount++
	}

	utils.JSON(w, http.StatusOK, utils.APIResponse{
		Success: len(rowErrors) == 0,
		Message: fmt.Sprintf("%d baris berhasil diimport, %d baris gagal", successCount, len(rowErrors)),
		Data:    map[string]interface{}{"success_count": successCount, "failed_rows": rowErrors},
	})
}
