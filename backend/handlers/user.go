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

func userExcelColumns(db *gorm.DB) []utils.ExcelColumn {
	return []utils.ExcelColumn{
		{Header: "Username", Required: true, Example: "budi.santoso",
			Get: func(i interface{}) string { return i.(models.User).Username },
			Set: func(i interface{}, raw string) error { i.(*models.User).Username = raw; return nil }},
		{Header: "Nama", Required: true, Example: "Budi Santoso",
			Get: func(i interface{}) string { return i.(models.User).Nama },
			Set: func(i interface{}, raw string) error { i.(*models.User).Nama = raw; return nil }},
		{Header: "Password", Required: true, Example: "rahasia123",
			Get: func(i interface{}) string { return "" }, // never export real/hashed password
			Set: func(i interface{}, raw string) error {
				hash, err := utils.HashPassword(raw)
				if err != nil {
					return fmt.Errorf("gagal memproses password")
				}
				i.(*models.User).Pass = hash
				return nil
			}},
		{Header: "Role", Required: true, Example: "pegawai",
			Get: func(i interface{}) string {
				u := i.(models.User)
				if u.Role != nil {
					return u.Role.Role
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var role models.Role
				if err := db.Where("role ILIKE ?", raw).First(&role).Error; err != nil {
					return fmt.Errorf("role '%s' tidak ditemukan (gunakan: administrator/admin/pegawai/atasan)", raw)
				}
				i.(*models.User).IDRole = role.ID
				return nil
			}},
		{Header: "NIP Pegawai (opsional)", Example: "198501012010011001",
			Get: func(i interface{}) string {
				u := i.(models.User)
				if u.Pegawai != nil {
					return u.Pegawai.NIP
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				if raw == "" {
					return nil
				}
				var peg models.Pegawai
				if err := db.Where("nip = ?", raw).First(&peg).Error; err != nil {
					return fmt.Errorf("pegawai dengan NIP '%s' tidak ditemukan", raw)
				}
				id := peg.ID
				i.(*models.User).IDPegawai = &id
				return nil
			}},
	}
}

func RegisterUserRoutes(mux *http.ServeMux, db *gorm.DB) {
	protect := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole("administrator"))
	}

	mux.Handle("GET /api/user", protect(func(w http.ResponseWriter, r *http.Request) { listUsers(w, r, db) }))
	mux.Handle("GET /api/user/export", protect(func(w http.ResponseWriter, r *http.Request) { exportUsers(w, r, db) }))
	mux.Handle("GET /api/user/template", protect(func(w http.ResponseWriter, r *http.Request) { templateCrud(w, handlers2Cfg(db)) }))
	mux.Handle("POST /api/user/import", protect(func(w http.ResponseWriter, r *http.Request) { importUsers(w, r, db) }))
	mux.Handle("GET /api/user/{id}", protect(func(w http.ResponseWriter, r *http.Request) { getUser(w, r, db) }))
	mux.Handle("POST /api/user", protect(func(w http.ResponseWriter, r *http.Request) { createUser(w, r, db) }))
	mux.Handle("PUT /api/user/{id}", protect(func(w http.ResponseWriter, r *http.Request) { updateUser(w, r, db) }))
	mux.Handle("DELETE /api/user/{id}", protect(func(w http.ResponseWriter, r *http.Request) { deleteUser(w, r, db) }))
}

// small helper so we can reuse the generic templateCrud renderer for the excel column spec
func handlers2Cfg(db *gorm.DB) CrudConfig[models.User] {
	return CrudConfig[models.User]{FileBaseName: "user", Columns: userExcelColumns(db)}
}

func listUsers(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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

	query := db.Model(&models.User{}).Preload("Role").Preload("Pegawai")
	countQuery := db.Model(&models.User{})
	if search != "" {
		cond := "username ILIKE ? OR nama ILIKE ?"
		query = query.Where(cond, "%"+search+"%", "%"+search+"%")
		countQuery = countQuery.Where(cond, "%"+search+"%", "%"+search+"%")
	}
	var total int64
	countQuery.Count(&total)

	var items []models.User
	if err := query.Order("id asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.SuccessMeta(w, "ok", items, map[string]interface{}{"page": page, "pageSize": pageSize, "total": total})
}

func getUser(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var user models.User
	if err := db.Preload("Role").Preload("Pegawai").First(&user, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "user tidak ditemukan")
		return
	}
	utils.Success(w, "ok", user)
}

type userPayload struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Nama      string `json:"nama"`
	IDRole    uint   `json:"id_role"`
	IDPegawai *uint  `json:"id_pegawai"`
}

func createUser(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p userPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.Username == "" || p.Password == "" || p.Nama == "" || p.IDRole == 0 {
		utils.Error(w, http.StatusBadRequest, "username, password, nama, dan role wajib diisi")
		return
	}
	hash, err := utils.HashPassword(p.Password)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memproses password")
		return
	}
	user := models.User{Username: p.Username, Pass: hash, Nama: p.Nama, IDRole: p.IDRole, IDPegawai: p.IDPegawai}
	if err := db.Create(&user).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan user (username mungkin sudah dipakai): "+err.Error())
		return
	}
	db.Preload("Role").Preload("Pegawai").First(&user, user.ID)
	utils.Created(w, "user berhasil dibuat", user)
}

func updateUser(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var existing models.User
	if err := db.First(&existing, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "user tidak ditemukan")
		return
	}
	var p userPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	updates := map[string]interface{}{}
	if p.Username != "" {
		updates["username"] = p.Username
	}
	if p.Nama != "" {
		updates["nama"] = p.Nama
	}
	if p.IDRole != 0 {
		updates["id_role"] = p.IDRole
	}
	updates["id_pegawai"] = p.IDPegawai
	if p.Password != "" {
		hash, err := utils.HashPassword(p.Password)
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal memproses password")
			return
		}
		updates["pass"] = hash
	}
	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal memperbarui user: "+err.Error())
		return
	}
	db.Preload("Role").Preload("Pegawai").First(&existing, "id = ?", id)
	utils.Success(w, "user berhasil diperbarui", existing)
}

func deleteUser(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	if err := db.Delete(&models.User{}, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus user: "+err.Error())
		return
	}
	utils.Success(w, "user berhasil dihapus", nil)
}

func exportUsers(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var items []models.User
	db.Preload("Role").Preload("Pegawai").Order("id asc").Find(&items)
	f, err := utils.ExportData(items, userExcelColumns(db))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXlsxResponse(w, f, "data_user.xlsx")
}

func importUsers(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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
		if err := deleteAllRows(db, &models.User{}); err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal menghapus data lama: "+err.Error())
			return
		}
	}
	cols := userExcelColumns(db)

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
		var user models.User
		errs := utils.ImportRow(&user, row, cols)
		if len(errs) > 0 {
			rowErrors = append(rowErrors, rowError{Row: i + 2, Errors: errs})
			continue
		}
		if err := db.Create(&user).Error; err != nil {
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
