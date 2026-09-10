package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

func jatahCutiExcelColumns(db *gorm.DB) []utils.ExcelColumn {
	return []utils.ExcelColumn{
		{Header: "NIP Pegawai", Required: true, Example: "198501012010011001",
			Get: func(i interface{}) string {
				j := i.(models.JatahCuti)
				if j.Pegawai != nil {
					return j.Pegawai.NIP
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var peg models.Pegawai
				if err := db.Where("nip = ?", raw).First(&peg).Error; err != nil {
					return fmt.Errorf("pegawai dengan NIP '%s' tidak ditemukan", raw)
				}
				i.(*models.JatahCuti).IDPegawai = peg.ID
				return nil
			}},
		{Header: "Nama Pegawai", Example: "(otomatis, hanya untuk referensi)",
			Get: func(i interface{}) string {
				j := i.(models.JatahCuti)
				if j.Pegawai != nil {
					return j.Pegawai.Nama
				}
				return ""
			},
			Set: func(i interface{}, raw string) error { return nil }},
		{Header: "Tahun", Required: true, Example: "2026",
			Get: func(i interface{}) string { return fmt.Sprintf("%d", i.(models.JatahCuti).Tahun) },
			Set: func(i interface{}, raw string) error {
				v, err := utils.ParseIntCell(raw)
				if err != nil {
					return fmt.Errorf("tahun harus berupa angka")
				}
				i.(*models.JatahCuti).Tahun = v
				return nil
			}},
		{Header: "Jumlah Hari", Required: true, Example: "12",
			Get: func(i interface{}) string { return fmt.Sprintf("%d", i.(models.JatahCuti).JumlahHari) },
			Set: func(i interface{}, raw string) error {
				v, err := utils.ParseIntCell(raw)
				if err != nil {
					return fmt.Errorf("jumlah hari harus berupa angka")
				}
				i.(*models.JatahCuti).JumlahHari = v
				return nil
			}},
		{Header: "Terpakai", Example: "0",
			Get: func(i interface{}) string { return fmt.Sprintf("%d", i.(models.JatahCuti).Terpakai) },
			Set: func(i interface{}, raw string) error {
				v, err := utils.ParseIntCell(raw)
				if err != nil {
					return fmt.Errorf("terpakai harus berupa angka")
				}
				i.(*models.JatahCuti).Terpakai = v
				return nil
			}},
	}
}

func RegisterJatahCutiRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }

	mux.Handle("GET /api/jatah-cuti", anyRole(func(w http.ResponseWriter, r *http.Request) { listJatahCuti(w, r, db) }))
	mux.Handle("GET /api/jatah-cuti/export", manage(func(w http.ResponseWriter, r *http.Request) { exportJatahCuti(w, r, db) }))
	mux.Handle("GET /api/jatah-cuti/template", manage(func(w http.ResponseWriter, r *http.Request) {
		writeXlsxResponse(w, utils.GenerateTemplate(jatahCutiExcelColumns(db)), "template_jatah_cuti.xlsx")
	}))
	mux.Handle("POST /api/jatah-cuti/import", manage(func(w http.ResponseWriter, r *http.Request) { importJatahCuti(w, r, db) }))
	mux.Handle("GET /api/jatah-cuti/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getJatahCuti(w, r, db) }))
	mux.Handle("POST /api/jatah-cuti", manage(func(w http.ResponseWriter, r *http.Request) { createJatahCuti(w, r, db) }))
	mux.Handle("PUT /api/jatah-cuti/{id}", manage(func(w http.ResponseWriter, r *http.Request) { updateJatahCuti(w, r, db) }))
	mux.Handle("DELETE /api/jatah-cuti/{id}", manage(func(w http.ResponseWriter, r *http.Request) { deleteJatahCuti(w, r, db) }))
}

func listJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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

	query := db.Model(&models.JatahCuti{}).Preload("Pegawai")
	countQuery := db.Model(&models.JatahCuti{})

	switch claims.RoleName {
	case "pegawai":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.JatahCuti{})
			return
		}
		query = query.Where("id_pegawai = ?", *claims.IDPegawai)
		countQuery = countQuery.Where("id_pegawai = ?", *claims.IDPegawai)
	case "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.JatahCuti{})
			return
		}
		query = query.Where("id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)", *claims.IDPegawai)
		countQuery = countQuery.Where("id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)", *claims.IDPegawai)
	}

	if tahun := q.Get("tahun"); tahun != "" {
		query = query.Where("tahun = ?", tahun)
		countQuery = countQuery.Where("tahun = ?", tahun)
	}

	var total int64
	countQuery.Count(&total)
	var items []models.JatahCuti
	query.Order("tahun desc, id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	utils.SuccessMeta(w, "ok", items, map[string]interface{}{"page": page, "pageSize": pageSize, "total": total})
}

func getJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.JatahCuti
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	utils.Success(w, "ok", item)
}

func createJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var item models.JatahCuti
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan data (kemungkinan sudah ada jatah cuti tahun tersebut): "+err.Error())
		return
	}
	utils.Created(w, "jatah cuti berhasil ditambahkan", item)
}

func updateJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var existing models.JatahCuti
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
	db.Preload("Pegawai").First(&existing, "id = ?", id)
	utils.Success(w, "jatah cuti berhasil diperbarui", existing)
}

func deleteJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	if err := db.Delete(&models.JatahCuti{}, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus data: "+err.Error())
		return
	}
	utils.Success(w, "jatah cuti berhasil dihapus", nil)
}

func exportJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var items []models.JatahCuti
	db.Preload("Pegawai").Order("tahun desc, id asc").Find(&items)
	f, err := utils.ExportData(items, jatahCutiExcelColumns(db))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXlsxResponse(w, f, "data_jatah_cuti.xlsx")
}

func importJatahCuti(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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
	cols := jatahCutiExcelColumns(db)

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
		var item models.JatahCuti
		errs := utils.ImportRow(&item, row, cols)
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
