package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

var pengajuanPreloads = []string{"Pegawai", "JenisCuti", "PolaHariKerja", "AtasanApprove"}

// preloadPengajuan applies the standard set of relation preloads for
// PengajuanCuti, including the Dokumen relation with its (potentially large)
// bytea "file" column omitted -- the actual bytes are only fetched by the
// dedicated download endpoint below.
func preloadPengajuan(query *gorm.DB) *gorm.DB {
	for _, p := range pengajuanPreloads {
		query = query.Preload(p)
	}
	return query.Preload("Dokumen", func(d *gorm.DB) *gorm.DB { return d.Omit("file") })
}

// sixDayWeekForTempatTgs decides whether a pegawai works a 6-day week (Senin-Sabtu)
// or the default 5-day week (Senin-Jumat), based on their tempat tugas: staff
// posted at a school ("sekolah") work 6 days a week, everyone else (dinas/
// kantor/etc) works 5 days a week. This replaces manual pola-hari-kerja
// selection -- the work pattern is now derived automatically per pegawai.
func sixDayWeekForTempatTgs(tempatTgs string) bool {
	return strings.Contains(strings.ToLower(tempatTgs), "sekolah")
}

// autoPolaID finds the PolaHariKerja master row matching the auto-detected
// work pattern (for display/export consistency only -- it plays no part in
// the actual day-count calculation anymore).
func autoPolaID(db *gorm.DB, sixDayWeek bool) *uint {
	needle := "5"
	if sixDayWeek {
		needle = "6"
	}
	var pola models.PolaHariKerja
	if err := db.Where("pola LIKE ?", "%"+needle+"%").First(&pola).Error; err != nil {
		return nil
	}
	id := pola.ID
	return &id
}

// dokumenRequirement describes one required/optional supporting document for
// a given jenis cuti.
type dokumenRequirement struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// dokumenRequirementsForJenis returns the checklist of supporting documents
// for a jenis cuti, following the office's document-completeness rules.
// Order matters: more specific keywords (melahirkan/umroh/sakit/alasan
// penting) are checked before the generic "tahunan" fallback, since e.g.
// "Cuti Tahunan Umroh" contains both "tahunan" and "umroh".
func dokumenRequirementsForJenis(jenisNama string) []dokumenRequirement {
	j := strings.ToLower(jenisNama)
	switch {
	case strings.Contains(j, "melahirkan"):
		return []dokumenRequirement{
			{Key: "rekomendasi_kepsek", Label: "Surat Rekomendasi Kepala Sekolah", Required: true},
			{Key: "sk_terakhir", Label: "SK Terakhir", Required: true},
			{Key: "keterangan_hpl", Label: "Surat Keterangan HPL (Rumah Sakit/Puskesmas)", Required: true},
			{Key: "buku_kia", Label: "Buku KIA", Required: true},
			{Key: "hasil_usg", Label: "Hasil USG", Required: false},
		}
	case strings.Contains(j, "umroh"):
		return []dokumenRequirement{
			{Key: "rekomendasi_kepsek", Label: "Surat Rekomendasi Kepala Sekolah", Required: true},
			{Key: "sk_terakhir", Label: "SK Terakhir", Required: true},
			{Key: "keterangan_travel", Label: "Surat Keterangan dari Travel Pemberangkatan", Required: true},
		}
	case strings.Contains(j, "sakit"):
		return []dokumenRequirement{
			{Key: "sk_terakhir", Label: "SK Terakhir", Required: true},
			{Key: "surat_rujukan", Label: "Surat Rujukan", Required: true},
			{Key: "keterangan_rawat_inap", Label: "Surat Keterangan Rawat Inap", Required: true},
		}
	case strings.Contains(j, "alasan penting"):
		return []dokumenRequirement{
			{Key: "sk_terakhir", Label: "SK Terakhir", Required: true},
			{Key: "rekomendasi_kepsek", Label: "Surat Rekomendasi Kepala Sekolah", Required: true},
			{Key: "dokumen_pendukung", Label: "Dokumen Pendukung (surat ket. rawat inap keluarga / surat kematian / surat KUA / dokumen istri melahirkan)", Required: true},
		}
	case strings.Contains(j, "tahunan"):
		return []dokumenRequirement{
			{Key: "rekomendasi_kepsek", Label: "Surat Rekomendasi Kepala Sekolah", Required: true},
			{Key: "sk_terakhir", Label: "SK Terakhir", Required: true},
		}
	default:
		return nil
	}
}

// formFileHeader returns the first uploaded file for a multipart form field,
// or nil if none was provided.
func formFileHeader(r *http.Request, key string) *multipart.FileHeader {
	if r.MultipartForm == nil {
		return nil
	}
	fhs := r.MultipartForm.File[key]
	if len(fhs) == 0 {
		return nil
	}
	return fhs[0]
}

func parseUintForm(r *http.Request, key string) uint {
	v, _ := strconv.ParseUint(r.FormValue(key), 10, 64)
	return uint(v)
}

// calculateWorkingDays counts the days between start and end (inclusive) that
// count as working days: Sundays are always excluded, Saturdays are excluded
// unless sixDayWeek is true (pegawai bertugas di sekolah), and any date
// listed in tgl_merah (public holidays) is always excluded.
func calculateWorkingDays(db *gorm.DB, start, end time.Time, sixDayWeek bool) int {
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
	// approve/reject/return: atasan (bawahannya sendiri) DAN admin/administrator
	// (siapa saja) -- canAccessPengajuan di bawah masih mengecek relasi
	// atasan-bawahan untuk role "atasan".
	approverRoles := func(h http.HandlerFunc) http.Handler { return authed(h, "atasan", "administrator", "admin") }

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
	mux.Handle("PUT /api/pengajuan-cuti/{id}/approve", approverRoles(func(w http.ResponseWriter, r *http.Request) { approvePengajuan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-cuti/{id}/reject", approverRoles(func(w http.ResponseWriter, r *http.Request) { rejectPengajuan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-cuti/{id}/kembalikan", approverRoles(func(w http.ResponseWriter, r *http.Request) { kembalikanPengajuan(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-cuti/{id}/return", approverRoles(func(w http.ResponseWriter, r *http.Request) { returnPengajuan(w, r, db) }))
	mux.Handle("GET /api/pengajuan-cuti/{id}/dokumen/{jenis}", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadDokumenPengajuan(w, r, db) }))
}

// dokumenContentType maps a stored filename's extension to a real MIME type
// so a browser can render it inline (view) instead of only being able to
// save it (download).
func dokumenContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// downloadDokumenPengajuan serves a pengajuan's supporting document. By
// default it forces a download (Content-Disposition: attachment); passing
// ?inline=1 instead serves it as "inline" with the real MIME type so a PDF
// or image can be viewed directly (e.g. in an <iframe>/<img>) without the
// user having to save it first.
func downloadDokumenPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	jenis := r.PathValue("jenis")
	var item models.PengajuanCuti
	if err := db.Preload("Pegawai").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	var doc models.PengajuanDokumen
	if err := db.Where("id_pengajuan = ? AND jenis = ?", item.ID, jenis).First(&doc).Error; err != nil || len(doc.File) == 0 {
		utils.Error(w, http.StatusNotFound, "dokumen tidak ditemukan")
		return
	}
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Type", dokumenContentType(doc.NamaFile))
		w.Header().Set("Content-Disposition", "inline; filename=\""+doc.NamaFile+"\"")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+doc.NamaFile+"\"")
	}
	w.Write(doc.File)
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

	query := preloadPengajuan(db.Model(&models.PengajuanCuti{}))
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
	if err := preloadPengajuan(db).First(&item, "id = ?", id).Error; err != nil {
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
	AlasanCuti       string `json:"alasan_cuti"`
	AlamatSelamaCuti string `json:"alamat_selama_cuti"`
}

// createPengajuan accepts multipart/form-data (rather than plain JSON)
// because self-service submissions (pegawai/atasan mengajukan untuk diri
// sendiri) must attach supporting documents whose checklist depends on the
// chosen jenis cuti. admin/administrator may submit on behalf of anyone
// without attaching any document.
func createPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 20MB)")
		return
	}

	p := pengajuanPayload{
		IDPegawai:        parseUintForm(r, "id_pegawai"),
		IDJenisCuti:      parseUintForm(r, "id_jenis_cuti"),
		TglMulai:         r.FormValue("tgl_mulai"),
		TglSelesai:       r.FormValue("tgl_selesai"),
		AlasanCuti:       r.FormValue("alasan_cuti"),
		AlamatSelamaCuti: r.FormValue("alamat_selama_cuti"),
	}

	// pegawai/atasan may only submit for themselves; admin/administrator may
	// submit on behalf of anyone. Self-submission is the case where
	// supporting documents become mandatory.
	selfSubmit := claims.RoleName == "pegawai" || claims.RoleName == "atasan"
	if selfSubmit {
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

	var pegawai models.Pegawai
	if err := db.First(&pegawai, p.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}

	var jenis models.JenisCuti
	if err := db.First(&jenis, p.IDJenisCuti).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "jenis cuti tidak ditemukan")
		return
	}

	// Validate & read supporting documents. Required documents are mandatory
	// only when the pegawai/atasan is submitting for themselves; admin and
	// administrator may create a pengajuan without any document.
	type pendingDoc struct {
		req  dokumenRequirement
		name string
		data []byte
	}
	var pending []pendingDoc
	var problems []string
	for _, req := range dokumenRequirementsForJenis(jenis.Jenis) {
		fh := formFileHeader(r, "dokumen_"+req.Key)
		if fh == nil {
			if req.Required && selfSubmit {
				problems = append(problems, "berkas '"+req.Label+"' wajib diupload")
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			problems = append(problems, "berkas '"+req.Label+"' harus berformat PDF, JPG, atau PNG")
			continue
		}
		f, err := fh.Open()
		if err != nil {
			problems = append(problems, "gagal membaca berkas '"+req.Label+"'")
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			problems = append(problems, "gagal membaca berkas '"+req.Label+"'")
			continue
		}
		pending = append(pending, pendingDoc{req: req, name: fh.Filename, data: data})
	}
	if len(problems) > 0 {
		utils.Error(w, http.StatusBadRequest, strings.Join(problems, "; "))
		return
	}

	sixDayWeek := sixDayWeekForTempatTgs(pegawai.TempatTgs)
	jumlahHari := calculateWorkingDays(db, start, end, sixDayWeek)
	if jumlahHari <= 0 {
		utils.Error(w, http.StatusBadRequest, "rentang tanggal yang dipilih tidak memiliki hari kerja")
		return
	}

	item := models.PengajuanCuti{
		IDPegawai:        p.IDPegawai,
		IDJenisCuti:      p.IDJenisCuti,
		TglMulai:         start,
		TglSelesai:       end,
		IDPolaHariKerja:  autoPolaID(db, sixDayWeek),
		AlasanCuti:       p.AlasanCuti,
		AlamatSelamaCuti: p.AlamatSelamaCuti,
		JumlahHari:       jumlahHari,
		Status:           models.StatusPending,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan pengajuan cuti: "+err.Error())
		return
	}
	for _, pd := range pending {
		doc := models.PengajuanDokumen{
			IDPengajuan: item.ID,
			Jenis:       pd.req.Key,
			Label:       pd.req.Label,
			NamaFile:    pd.name,
			File:        pd.data,
		}
		db.Create(&doc)
	}
	preloadPengajuan(db).First(&item, item.ID)
	utils.Created(w, "pengajuan cuti berhasil diajukan, menunggu persetujuan atasan", item)
}

func updatePengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.Preload("Pegawai").Preload("Dokumen", func(d *gorm.DB) *gorm.DB { return d.Omit("file") }).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuan(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if (claims.RoleName == "pegawai") && item.Status != models.StatusPending && item.Status != models.StatusDikembalikan {
		utils.Error(w, http.StatusBadRequest, "pengajuan yang sudah diproses tidak dapat diubah")
		return
	}

	// Saat mengedit pengajuan yang dikembalikan, boleh sekaligus mengupload
	// ulang berkas yang ditandai atasan/admin (id_pengajuan/dokumen.perlu_perbaikan)
	// -- ini butuh multipart/form-data. Edit biasa (tanpa berkas) tetap boleh
	// JSON seperti sebelumnya.
	isMultipart := strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
	var p pengajuanPayload
	if isMultipart {
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 20MB)")
			return
		}
		p = pengajuanPayload{
			IDPegawai:        parseUintForm(r, "id_pegawai"),
			IDJenisCuti:      parseUintForm(r, "id_jenis_cuti"),
			TglMulai:         r.FormValue("tgl_mulai"),
			TglSelesai:       r.FormValue("tgl_selesai"),
			AlasanCuti:       r.FormValue("alasan_cuti"),
			AlamatSelamaCuti: r.FormValue("alamat_selama_cuti"),
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			utils.Error(w, http.StatusBadRequest, "format data tidak valid")
			return
		}
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

	// Berkas yang ditandai "perlu diperbaiki" saat pengajuan dikembalikan wajib
	// diupload ulang (khusus pegawai pemilik pengajuan) sebelum pengajuan bisa
	// disimpan lagi. Validasi & baca semua file dulu sebelum menyentuh database
	// supaya tidak ada perubahan sebagian jika salah satu berkas gagal.
	type docReplace struct {
		id   uint
		name string
		data []byte
	}
	var replacements []docReplace
	if item.Status == models.StatusDikembalikan {
		var stillMissing []string
		for _, d := range item.Dokumen {
			if !d.PerluPerbaikan {
				continue
			}
			fh := formFileHeader(r, "dokumen_"+d.Jenis)
			if fh == nil {
				if claims.RoleName == "pegawai" {
					stillMissing = append(stillMissing, d.Label)
				}
				continue
			}
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				utils.Error(w, http.StatusBadRequest, "berkas '"+d.Label+"' harus berformat PDF, JPG, atau PNG")
				return
			}
			f, err := fh.Open()
			if err != nil {
				utils.Error(w, http.StatusBadRequest, "gagal membaca berkas '"+d.Label+"'")
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				utils.Error(w, http.StatusBadRequest, "gagal membaca berkas '"+d.Label+"'")
				return
			}
			replacements = append(replacements, docReplace{id: d.ID, name: fh.Filename, data: data})
		}
		if len(stillMissing) > 0 {
			utils.Error(w, http.StatusBadRequest, "silakan upload ulang berkas yang ditandai perlu diperbaiki: "+strings.Join(stillMissing, ", "))
			return
		}
	}
	for _, rep := range replacements {
		if err := db.Model(&models.PengajuanDokumen{}).Where("id = ?", rep.id).Updates(map[string]interface{}{
			"nama_file":       rep.name,
			"file":            rep.data,
			"perlu_perbaikan": false,
		}).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan berkas pengganti: "+err.Error())
			return
		}
	}

	item.IDJenisCuti = p.IDJenisCuti
	item.TglMulai = start
	item.TglSelesai = end
	item.AlasanCuti = p.AlasanCuti
	item.AlamatSelamaCuti = p.AlamatSelamaCuti
	if claims.RoleName == "administrator" || claims.RoleName == "admin" {
		// allow admin to re-set the target pegawai too
		if p.IDPegawai != 0 {
			item.IDPegawai = p.IDPegawai
		}
	}

	var pegawai models.Pegawai
	db.First(&pegawai, item.IDPegawai)
	sixDayWeek := sixDayWeekForTempatTgs(pegawai.TempatTgs)
	item.JumlahHari = calculateWorkingDays(db, start, end, sixDayWeek)
	item.IDPolaHariKerja = autoPolaID(db, sixDayWeek)

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
	preloadPengajuan(db).First(&item, item.ID)
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
	if claims.RoleName == "pegawai" && item.Status != models.StatusPending && item.Status != models.StatusDikembalikan {
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
	preloadPengajuan(db).First(&item, item.ID)
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
	preloadPengajuan(db).First(&item, item.ID)
	utils.Success(w, "pengajuan cuti telah ditolak", item)
}

// kembalikanPayload extends approvalPayload with the list of PengajuanDokumen
// IDs the atasan/admin ticked as bermasalah (tidak sesuai) -- these get
// PerluPerbaikan=true so the pegawai's edit form knows exactly which berkas
// must be re-uploaded.
type kembalikanPayload struct {
	Catatan    string `json:"catatan_approval"`
	DokumenIDs []uint `json:"dokumen_ids"`
}

// kembalikanPengajuan sends a still-pending pengajuan back to the pegawai for
// correction (e.g. some uploaded documents don't match/aren't valid) instead
// of rejecting it outright. Unlike a rejection, the pegawai can then edit or
// delete-and-resubmit it (see updatePengajuan/deletePengajuan), and a reason
// is mandatory so the pegawai knows what to fix. Optionally, specific
// documents can be flagged (dokumen_ids) so the pegawai's edit form prompts
// exactly those berkas to be re-uploaded instead of the whole submission.
func kembalikanPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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
	var p kembalikanPayload
	_ = json.NewDecoder(r.Body).Decode(&p)
	if strings.TrimSpace(p.Catatan) == "" {
		utils.Error(w, http.StatusBadRequest, "alasan pengembalian wajib diisi (misal: ada berkas yang tidak sesuai)")
		return
	}

	// Reset semua tanda lama dulu, lalu tandai ulang hanya berkas yang dipilih
	// kali ini (kalau ada) sebagai perlu diperbaiki/diupload ulang.
	if err := db.Model(&models.PengajuanDokumen{}).Where("id_pengajuan = ?", item.ID).Update("perlu_perbaikan", false).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menandai berkas: "+err.Error())
		return
	}
	if len(p.DokumenIDs) > 0 {
		if err := db.Model(&models.PengajuanDokumen{}).Where("id_pengajuan = ? AND id IN ?", item.ID, p.DokumenIDs).Update("perlu_perbaikan", true).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menandai berkas: "+err.Error())
			return
		}
	}

	now := time.Now()
	item.Status = models.StatusDikembalikan
	item.IDAtasanApprove = claims.IDPegawai
	item.TglApproval = &now
	item.CatatanApproval = p.Catatan
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengembalikan pengajuan: "+err.Error())
		return
	}
	preloadPengajuan(db).First(&item, item.ID)
	utils.Success(w, "pengajuan cuti dikembalikan ke pegawai untuk diperbaiki", item)
}

// returnPengajuan reverts a pengajuan that was already disetujui/ditolak back
// to pending, so it can go through the approval flow again (e.g. an approval
// made by mistake). If it had been approved as annual leave, the quota that
// was deducted is rolled back first.
func returnPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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
	if item.Status == models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini masih menunggu, tidak perlu dikembalikan")
		return
	}
	if item.Status == models.StatusDisetuju && item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
		if err := adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, -item.JumlahHari); err != nil {
			utils.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	item.Status = models.StatusPending
	item.IDAtasanApprove = nil
	item.TglApproval = nil
	item.CatatanApproval = ""
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengembalikan pengajuan: "+err.Error())
		return
	}
	preloadPengajuan(db).First(&item, item.ID)
	utils.Success(w, "pengajuan cuti dikembalikan ke status menunggu", item)
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
		{Header: "Tanggal Mulai (DD-MM-YYYY)", Required: true, Example: "10-01-2026",
			Get: func(i interface{}) string { t := i.(models.PengajuanCuti).TglMulai; return t.Format("02-01-2006") },
			Set: func(i interface{}, raw string) error {
				t, err := utils.ParseDateCell(raw)
				if err != nil {
					return err
				}
				i.(*models.PengajuanCuti).TglMulai = t
				return nil
			}},
		{Header: "Tanggal Selesai (DD-MM-YYYY)", Required: true, Example: "12-01-2026",
			Get: func(i interface{}) string { t := i.(models.PengajuanCuti).TglSelesai; return t.Format("02-01-2006") },
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
	if isReplaceMode(r) {
		if err := deleteAllRows(db, &models.PengajuanCuti{}); err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal menghapus data lama: "+err.Error())
			return
		}
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
			var peg models.Pegawai
			sixDayWeek := false
			if err := db.First(&peg, item.IDPegawai).Error; err == nil {
				sixDayWeek = sixDayWeekForTempatTgs(peg.TempatTgs)
			}
			item.IDPolaHariKerja = autoPolaID(db, sixDayWeek)
			item.JumlahHari = calculateWorkingDays(db, item.TglMulai, item.TglSelesai, sixDayWeek)
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
