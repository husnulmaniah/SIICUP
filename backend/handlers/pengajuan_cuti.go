package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

var pengajuanPreloads = []string{"Pegawai", "JenisCuti", "PolaHariKerja", "AtasanApprove"}

// calculateWorkingDays counts the days between start and end (inclusive) that
// count as working days: Sundays are always excluded, Saturdays are excluded
// unless the chosen pola_hari_kerja explicitly mentions "6" (6-day work week),
// and any date listed in tgl_merah (public holidays) is always excluded.
func calculateWorkingDays(db *gorm.DB, start, end time.Time, polaID *uint) int {
	sixDayWeek := false
	if polaID != nil {
		var pola models.PolaHariKerja
		if err := db.First(&pola, *polaID).Error; err == nil {
			if strings.Contains(pola.Pola, "6") {
				sixDayWeek = true
			}
		}
	}

	var holidays []models.TglMerah
	db.Where("tgl BETWEEN ? AND ?", start, end).Find(&holidays)
	holidaySet := map[string]bool{}
	for _, h := range holidays {
		holidaySet[h.Tgl.Format("2006-01-02")] = true
	}

	count := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		wd := d.Weekday()
		if wd == time.Sunday {
			continue
		}
		if wd == time.Saturday && !sixDayWeek {
			continue
		}
		if holidaySet[d.Format("2006-01-02")] {
			continue
		}
		count++
	}
	return count
}

// isAnnualLeave decides whether a jenis_cuti counts against the yearly quota
// (jatah_cuti). By convention only "cuti tahunan" (annual leave) is quota-limited.
func isAnnualLeave(jenis models.JenisCuti) bool {
	return strings.Contains(strings.ToLower(jenis.Jenis), "tahunan")
}

func adjustQuotaUsage(db *gorm.DB, pegawaiID uint, tahun int, defaultJumlah int, delta int) error {
	var jatah models.JatahCuti
	err := db.Where("id_pegawai = ? AND tahun = ?", pegawaiID, tahun).First(&jatah).Error
	if err != nil {
		if delta <= 0 {
			return nil // nothing to roll back if no quota row exists
		}
		jatah = models.JatahCuti{IDPegawai: pegawaiID, Tahun: tahun, JumlahHari: defaultJumlah, Terpakai: 0}
		if err := db.Create(&jatah).Error; err != nil {
			return err
		}
	}
	newTerpakai := jatah.Terpakai + delta
	if newTerpakai < 0 {
		newTerpakai = 0
	}
	if delta > 0 && newTerpakai > jatah.JumlahHari {
		return fmt.Errorf("sisa jatah cuti tahunan pegawai tidak mencukupi (sisa: %d hari, diajukan: %d hari)", jatah.JumlahHari-jatah.Terpakai, delta)
	}
	return db.Model(&jatah).Update("terpakai", newTerpakai).Error
}

func RegisterPengajuanCutiRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }
	atasanOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "atasan") }

	mux.Handle("GET /api/pengajuan-cuti", anyRole(func(w http.ResponseWriter, r *http.Request) { listPengajuan(w, r, db) }))
	mux.Handle("GET /api/pengajuan-cuti/export", manage(func(w http.ResponseWriter, r *http.Request) { exportPengajuan(w, r, db) }))
	mux.Handle("GET /api/pengajuan-cuti/template", manage(func(w http.ResponseWriter, r *http.Request) {
		writeXlsxResponse(w, utils.GenerateTemplate(pengajuanExcelColumns(db)), "template_pengajuan_cuti.xlsx")
	}))
	mux.Handle("POST /api/pengajuan-cuti/import", manage(func(w http.ResponseWriter, r *http.Request) { importPengajuan(w, r, db) }))
	mux.Handle("GET /api/pengajuan-cuti/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getPengajuan(w, r, db) }))
	mux.Handle("POST /api/pengajuan-cuti", anyRole(func(w http.ResponseWriter, r *http.Request) { createPengajuan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-cuti/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { updatePengajuan(w, r, db) }))
	mux.Handle("DELETE /api/pengajuan-cuti/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { deletePengajuan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-cuti/{id}/approve", atasanOnly(func(w http.ResponseWriter, r *http.Request) { approvePengajuan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-cuti/{id}/reject", atasanOnly(func(w http.ResponseWriter, r *http.Request) { rejectPengajuan(w, r, db) }))
}

func listPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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

	query := db.Model(&models.PengajuanCuti{})
	for _, p := range pengajuanPreloads {
		query = query.Preload(p)
	}
	countQuery := db.Model(&models.PengajuanCuti{})

	switch claims.RoleName {
	case "pegawai":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.PengajuanCuti{})
			return
		}
		query = query.Where("id_pegawai = ?", *claims.IDPegawai)
		countQuery = countQuery.Where("id_pegawai = ?", *claims.IDPegawai)
	case "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.PengajuanCuti{})
			return
		}
		sub := "id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)"
		query = query.Where(sub, *claims.IDPegawai)
		countQuery = countQuery.Where(sub, *claims.IDPegawai)
	}

	if status := q.Get("status"); status != "" {
		query = query.Where("status = ?", status)
		countQuery = countQuery.Where("status = ?", status)
	}

	var total int64
	countQuery.Count(&total)
	var items []models.PengajuanCuti
	query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	utils.SuccessMeta(w, "ok", items, map[string]interface{}{"page": page, "pageSize": pageSize, "total": total})
}

func canAccessPengajuan(claims *utils.Claims, item models.PengajuanCuti) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "pegawai":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	case "atasan":
		return claims.IDPegawai != nil && item.Pegawai != nil && item.Pegawai.IDAtasan != nil && *item.Pegawai.IDAtasan == *claims.IDPegawai
	}
	return false
}

func getPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	query := db
	for _, p := range pengajuanPreloads {
		query = query.Preload(p)
	}
	if err := query.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	utils.Success(w, "ok", item)
}

type pengajuanPayload struct {
	IDPegawai        uint   `json:"id_pegawai"`
	IDJenisCuti      uint   `json:"id_jenis_cuti"`
	TglMulai         string `json:"tgl_mulai"`
	TglSelesai       string `json:"tgl_selesai"`
	IDPolaHariKerja  *uint  `json:"id_pola_hari_kerja"`
	AlasanCuti       string `json:"alasan_cuti"`
	AlamatSelamaCuti string `json:"alamat_selama_cuti"`
}

func createPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	var p pengajuanPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}

	// pegawai may only submit for themselves; admin/administrator may submit on behalf of anyone
	if claims.RoleName == "pegawai" || claims.RoleName == "atasan" {
		if claims.IDPegawai == nil {
			utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
			return
		}
		p.IDPegawai = *claims.IDPegawai
	}
	if p.IDPegawai == 0 || p.IDJenisCuti == 0 || p.TglMulai == "" || p.TglSelesai == "" {
		utils.Error(w, http.StatusBadRequest, "pegawai, jenis cuti, tanggal mulai, dan tanggal selesai wajib diisi")
		return
	}

	start, err := utils.ParseDateCell(p.TglMulai)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal mulai tidak valid")
		return
	}
	end, err := utils.ParseDateCell(p.TglSelesai)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal selesai tidak valid")
		return
	}
	if end.Before(start) {
		utils.Error(w, http.StatusBadRequest, "tanggal selesai tidak boleh sebelum tanggal mulai")
		return
	}

	var jenis models.JenisCuti
	if err := db.First(&jenis, p.IDJenisCuti).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "jenis cuti tidak ditemukan")
		return
	}

	jumlahHari := calculateWorkingDays(db, start, end, p.IDPolaHariKerja)
	if jumlahHari <= 0 {
		utils.Error(w, http.StatusBadRequest, "rentang tanggal yang dipilih tidak memiliki hari kerja")
		return
	}

	item := models.PengajuanCuti{
		IDPegawai:        p.IDPegawai,
		IDJenisCuti:      p.IDJenisCuti,
		TglMulai:         start,
		TglSelesai:       end,
		IDPolaHariKerja:  p.IDPolaHariKerja,
		AlasanCuti:       p.AlasanCuti,
		AlamatSelamaCuti: p.AlamatSelamaCuti,
		JumlahHari:       jumlahHari,
		Status:           models.StatusPending,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan pengajuan cuti: "+err.Error())
		return
	}
	db.Preload("Pegawai").Preload("JenisCuti").Preload("PolaHariKerja").First(&item, item.ID)
	utils.Created(w, "pengajuan cuti berhasil diajukan, menunggu persetujuan atasan", item)
}

func updatePengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if (claims.RoleName == "pegawai") && item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan yang sudah diproses tidak dapat diubah")
		return
	}

	var p pengajuanPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	start, err := utils.ParseDateCell(p.TglMulai)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal mulai tidak valid")
		return
	}
	end, err := utils.ParseDateCell(p.TglSelesai)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal selesai tidak valid")
		return
	}
	if end.Before(start) {
		utils.Error(w, http.StatusBadRequest, "tanggal selesai tidak boleh sebelum tanggal mulai")
		return
	}

	item.IDJenisCuti = p.IDJenisCuti
	item.TglMulai = start
	item.TglSelesai = end
	item.IDPolaHariKerja = p.IDPolaHariKerja
	item.AlasanCuti = p.AlasanCuti
	item.AlamatSelamaCuti = p.AlamatSelamaCuti
	item.JumlahHari = calculateWorkingDays(db, start, end, p.IDPolaHariKerja)
	if claims.RoleName == "administrator" || claims.RoleName == "admin" {
		// allow admin to re-set the target pegawai too
		if p.IDPegawai != 0 {
			item.IDPegawai = p.IDPegawai
		}
	}
	// editing resets it back to pending so the approval flow runs again
	if item.Status != models.StatusPending {
		item.Status = models.StatusPending
		item.IDAtasanApprove = nil
		item.TglApproval = nil
		item.CatatanApproval = ""
	}

	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal memperbarui data: "+err.Error())
		return
	}
	db.Preload("Pegawai").Preload("JenisCuti").Preload("PolaHariKerja").First(&item, item.ID)
	utils.Success(w, "pengajuan cuti berhasil diperbarui", item)
}

func deletePengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.Preload("Pegawai").Preload("JenisCuti").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if claims.RoleName == "pegawai" && item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan yang sudah diproses tidak dapat dihapus")
		return
	}
	if item.Status == models.StatusDisetuju && item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
		_ = adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, -item.JumlahHari)
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus data: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan cuti berhasil dihapus", nil)
}

type approvalPayload struct {
	Catatan string `json:"catatan_approval"`
}

func approvePengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.Preload("Pegawai").Preload("JenisCuti").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda hanya dapat memproses pengajuan cuti bawahan anda")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya")
		return
	}
	var p approvalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)

	if item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
		if err := adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, item.JumlahHari); err != nil {
			utils.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	now := time.Now()
	item.Status = models.StatusDisetuju
	item.IDAtasanApprove = claims.IDPegawai
	item.TglApproval = &now
	item.CatatanApproval = p.Catatan
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyetujui pengajuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").Preload("JenisCuti").Preload("PolaHariKerja").Preload("AtasanApprove").First(&item, item.ID)
	utils.Success(w, "pengajuan cuti berhasil disetujui", item)
}

func rejectPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda hanya dapat memproses pengajuan cuti bawahan anda")
		return
	}
	if item.Status != models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya")
		return
	}
	var p approvalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)

	now := time.Now()
	item.Status = models.StatusDitolak
	item.IDAtasanApprove = claims.IDPegawai
	item.TglApproval = &now
	item.CatatanApproval = p.Catatan
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menolak pengajuan: "+err.Error())
		return
	}
	db.Preload("Pegawai").Preload("JenisCuti").Preload("PolaHariKerja").Preload("AtasanApprove").First(&item, item.ID)
	utils.Success(w, "pengajuan cuti telah ditolak", item)
}

func pengajuanExcelColumns(db *gorm.DB) []utils.ExcelColumn {
	return []utils.ExcelColumn{
		{Header: "NIP Pegawai", Required: true, Example: "198501012010011001",
			Get: func(i interface{}) string {
				p := i.(models.PengajuanCuti)
				if p.Pegawai != nil {
					return p.Pegawai.NIP
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var peg models.Pegawai
				if err := db.Where("nip = ?", raw).First(&peg).Error; err != nil {
					return fmt.Errorf("pegawai dengan NIP '%s' tidak ditemukan", raw)
				}
				i.(*models.PengajuanCuti).IDPegawai = peg.ID
				return nil
			}},
		{Header: "Jenis Cuti", Required: true, Example: "Cuti Tahunan",
			Get: func(i interface{}) string {
				p := i.(models.PengajuanCuti)
				if p.JenisCuti != nil {
					return p.JenisCuti.Jenis
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var jenis models.JenisCuti
				if err := db.Where("jenis ILIKE ?", raw).First(&jenis).Error; err != nil {
					return fmt.Errorf("jenis cuti '%s' tidak ditemukan", raw)
				}
				i.(*models.PengajuanCuti).IDJenisCuti = jenis.ID
				return nil
			}},
		{Header: "Tanggal Mulai (YYYY-MM-DD)", Required: true, Example: "2026-01-10",
			Get: func(i interface{}) string { t := i.(models.PengajuanCuti).TglMulai; return t.Format("2006-01-02") },
			Set: func(i interface{}, raw string) error {
				t, err := utils.ParseDateCell(raw)
				if err != nil {
					return err
				}
				i.(*models.PengajuanCuti).TglMulai = t
				return nil
			}},
		{Header: "Tanggal Selesai (YYYY-MM-DD)", Required: true, Example: "2026-01-12",
			Get: func(i interface{}) string { t := i.(models.PengajuanCuti).TglSelesai; return t.Format("2006-01-02") },
			Set: func(i interface{}, raw string) error {
				t, err := utils.ParseDateCell(raw)
				if err != nil {
					return err
				}
				i.(*models.PengajuanCuti).TglSelesai = t
				return nil
			}},
		{Header: "Pola Hari Kerja", Example: "5 Hari Kerja (Senin-Jumat)",
			Get: func(i interface{}) string {
				p := i.(models.PengajuanCuti)
				if p.PolaHariKerja != nil {
					return p.PolaHariKerja.Pola
				}
				return ""
			},
			Set: func(i interface{}, raw string) error {
				var pola models.PolaHariKerja
				if err := db.Where("pola ILIKE ?", raw).First(&pola).Error; err != nil {
					return fmt.Errorf("pola hari kerja '%s' tidak ditemukan", raw)
				}
				id := pola.ID
				i.(*models.PengajuanCuti).IDPolaHariKerja = &id
				return nil
			}},
		{Header: "Alasan Cuti", Example: "Keperluan keluarga",
			Get: func(i interface{}) string { return i.(models.PengajuanCuti).AlasanCuti },
			Set: func(i interface{}, raw string) error { i.(*models.PengajuanCuti).AlasanCuti = raw; return nil }},
		{Header: "Alamat Selama Cuti", Example: "Jl. Merdeka No. 1, Jakarta",
			Get: func(i interface{}) string { return i.(models.PengajuanCuti).AlamatSelamaCuti },
			Set: func(i interface{}, raw string) error { i.(*models.PengajuanCuti).AlamatSelamaCuti = raw; return nil }},
		{Header: "Status", Example: "pending",
			Get: func(i interface{}) string { return i.(models.PengajuanCuti).Status },
			Set: func(i interface{}, raw string) error {
				raw = strings.ToLower(strings.TrimSpace(raw))
				if raw == "" {
					raw = models.StatusPending
				}
				i.(*models.PengajuanCuti).Status = raw
				return nil
			}},
	}
}

func exportPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var items []models.PengajuanCuti
	query := db
	for _, p := range pengajuanPreloads {
		query = query.Preload(p)
	}
	query.Order("created_at desc").Find(&items)
	f, err := utils.ExportData(items, pengajuanExcelColumns(db))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXlsxResponse(w, f, "data_pengajuan_cuti.xlsx")
}

func importPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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
	cols := pengajuanExcelColumns(db)

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
		var item models.PengajuanCuti
		errs := utils.ImportRow(&item, row, cols)
		if len(errs) == 0 {
			item.JumlahHari = calculateWorkingDays(db, item.TglMulai, item.TglSelesai, item.IDPolaHariKerja)
			if item.JumlahHari <= 0 {
				errs = append(errs, "rentang tanggal tidak memiliki hari kerja")
			}
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
