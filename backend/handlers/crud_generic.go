package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cuti-app/middleware"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// CrudConfig describes everything the generic engine needs to expose full
// CRUD + excel import/export for a simple GORM model T.
type CrudConfig[T any] struct {
	Columns      []utils.ExcelColumn // excel column mapping (export/template/import)
	SearchFields []string            // raw SQL column names searched with ILIKE
	Preloads     []string            // GORM relations to preload
	OrderBy      string              // default: "id asc"
	FileBaseName string              // e.g. "jabatan" -> template_jabatan.xlsx / data_jabatan.xlsx
	BeforeSave   func(item *T) error // optional hook run before create/update save
}

// RegisterCrud wires up GET (list+search+pagination), GET/{id}, POST, PUT/{id},
// DELETE/{id}, GET/export, GET/template and POST/import for model T on base path.
func RegisterCrud[T any](mux *http.ServeMux, db *gorm.DB, base string, cfg CrudConfig[T], roles ...string) {
	if cfg.OrderBy == "" {
		cfg.OrderBy = "id asc"
	}
	protect := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}

	mux.Handle("GET "+base, protect(func(w http.ResponseWriter, r *http.Request) { listCrud(w, r, db, cfg) }))
	mux.Handle("GET "+base+"/export", protect(func(w http.ResponseWriter, r *http.Request) { exportCrud(w, r, db, cfg) }))
	mux.Handle("GET "+base+"/template", protect(func(w http.ResponseWriter, r *http.Request) { templateCrud(w, cfg) }))
	mux.Handle("POST "+base+"/import", protect(func(w http.ResponseWriter, r *http.Request) { importCrud(w, r, db, cfg) }))
	mux.Handle("GET "+base+"/{id}", protect(func(w http.ResponseWriter, r *http.Request) { getCrud(w, r, db, cfg) }))
	mux.Handle("POST "+base, protect(func(w http.ResponseWriter, r *http.Request) { createCrud(w, r, db, cfg) }))
	mux.Handle("PUT "+base+"/{id}", protect(func(w http.ResponseWriter, r *http.Request) { updateCrud(w, r, db, cfg) }))
	mux.Handle("DELETE "+base+"/{id}", protect(func(w http.ResponseWriter, r *http.Request) { deleteCrud(w, r, db, cfg) }))
}

func applyPreloads(db *gorm.DB, preloads []string) *gorm.DB {
	for _, p := range preloads {
		db = db.Preload(p)
	}
	return db
}

func listCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
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

	var items []T
	var total int64

	query := applyPreloads(db.Model(new(T)), cfg.Preloads)
	countQuery := db.Model(new(T))

	if search != "" && len(cfg.SearchFields) > 0 {
		var clauses []string
		var args []interface{}
		for _, f := range cfg.SearchFields {
			clauses = append(clauses, fmt.Sprintf("%s ILIKE ?", f))
			args = append(args, "%"+search+"%")
		}
		cond := strings.Join(clauses, " OR ")
		query = query.Where(cond, args...)
		countQuery = countQuery.Where(cond, args...)
	}

	countQuery.Count(&total)

	if err := query.Order(cfg.OrderBy).Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data: "+err.Error())
		return
	}

	utils.SuccessMeta(w, "berhasil mengambil data", items, map[string]interface{}{
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

func getCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
	id := r.PathValue("id")
	var item T
	if err := applyPreloads(db, cfg.Preloads).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	utils.Success(w, "berhasil mengambil data", item)
}

func createCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
	var item T
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if cfg.BeforeSave != nil {
		if err := cfg.BeforeSave(&item); err != nil {
			utils.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan data: "+err.Error())
		return
	}
	utils.Created(w, "data berhasil ditambahkan", item)
}

func updateCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
	id := r.PathValue("id")
	var existing T
	if err := db.First(&existing, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	delete(payload, "id")
	if err := db.Model(&existing).Updates(payload).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal memperbarui data: "+err.Error())
		return
	}
	db.First(&existing, "id = ?", id)
	utils.Success(w, "data berhasil diperbarui", existing)
}

func deleteCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
	id := r.PathValue("id")
	var item T
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus data (kemungkinan masih dipakai data lain): "+err.Error())
		return
	}
	utils.Success(w, "data berhasil dihapus", nil)
}

func exportCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
	var items []T
	if err := applyPreloads(db, cfg.Preloads).Order(cfg.OrderBy).Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	f, err := utils.ExportData(items, cfg.Columns)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXlsxResponse(w, f, "data_"+cfg.FileBaseName+".xlsx")
}

func templateCrud[T any](w http.ResponseWriter, cfg CrudConfig[T]) {
	f := utils.GenerateTemplate(cfg.Columns)
	writeXlsxResponse(w, f, "template_"+cfg.FileBaseName+".xlsx")
}

func importCrud[T any](w http.ResponseWriter, r *http.Request, db *gorm.DB, cfg CrudConfig[T]) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca file upload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "file excel tidak ditemukan (field 'file')")
		return
	}
	defer file.Close()

	rows, err := utils.ReadRows(file)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	type rowError struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	}
	var rowErrors []rowError
	successCount := 0

	for i, row := range rows {
		isBlank := true
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				isBlank = false
				break
			}
		}
		if isBlank {
			continue
		}
		var item T
		errs := utils.ImportRow(&item, row, cfg.Columns)
		if len(errs) > 0 {
			rowErrors = append(rowErrors, rowError{Row: i + 2, Errors: errs})
			continue
		}
		if cfg.BeforeSave != nil {
			if err := cfg.BeforeSave(&item); err != nil {
				rowErrors = append(rowErrors, rowError{Row: i + 2, Errors: []string{err.Error()}})
				continue
			}
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
		Data: map[string]interface{}{
			"success_count": successCount,
			"failed_rows":   rowErrors,
		},
	})
}
