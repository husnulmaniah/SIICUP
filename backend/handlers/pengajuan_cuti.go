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

// "Pegawai.UnitKerja" (di samping "Pegawai" itu sendiri) sengaja ditambahkan
// supaya isSekolahPegawai (dipakai syncJumlahHariWithHolidays di bawah) bisa
// membaca UnitKerja.TempatKerja pegawai tanpa query tambahan.
var pengajuanPreloads = []string{"Pegawai", "Pegawai.UnitKerja", "JenisCuti", "PolaHariKerja", "AtasanApprove"}

// preloadPengajuan applies the standard set of relation preloads for
// PengajuanCuti, including the Dokumen relation with its (potentially large)
// bytea "file" column omitted -- the actual bytes are only fetched by the
// dedicated download endpoint below. "Pegawai" sendiri juga di-Omit dari
// dokumenFileFields-nya (SK, foto, ttd) -- dipakai oleh listPengajuan (menu
// utama Pengajuan Cuti, tabel paling sering dimuat) & ResyncActivePengajuanDays
// (jalan untuk SEMUA pengajuan aktif setiap kali tanggal merah diubah), jadi
// tanpa Omit ini setiap baris menarik berkas SK/foto/ttd yang tidak pernah
// dipakai di sini.
func preloadPengajuan(query *gorm.DB) *gorm.DB {
	omitDokumen := func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }
	for _, p := range pengajuanPreloads {
		// "Pegawai" dan "AtasanApprove" KEDUANYA bertipe *models.Pegawai (lihat
		// models.go) jadi keduanya perlu di-Omit dari dokumenFileFields-nya --
		// AtasanApprove sempat terlewat sebelumnya karena bukan field bernama
		// "Pegawai", padahal membawa kolom berkas (SK/foto/ttd) yang sama.
		if p == "Pegawai" || p == "AtasanApprove" {
			query = query.Preload(p, omitDokumen)
			continue
		}
		query = query.Preload(p)
	}
	return query.Preload("Dokumen", func(d *gorm.DB) *gorm.DB { return d.Omit("file") })
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

// isSekolahFromTempatTgs decides whether a pegawai's tempat tugas is a
// sekolah (school) as opposed to dinas/kantor by guessing from the free-text
// Tempat Tugas field. This is the FALLBACK path -- see isSekolahPegawai below,
// which is the function actually called everywhere now; this one is only
// still reached for a pegawai whose unit kerja hasn't been given an explicit
// Tempat Kerja category yet (models.UnitKerja.TempatKerja), which keeps old
// data working exactly as before.
func isSekolahFromTempatTgs(tempatTgs string) bool {
	return strings.Contains(strings.ToLower(tempatTgs), "sekolah")
}

// isSekolahPegawai decides whether a pegawai counts as "sekolah" (as opposed
// to dinas/kantor), with priority: (1) the EXPLICIT category the
// administrator set on the pegawai's unit kerja/sekolah
// (models.UnitKerja.TempatKerja, via menu Master Data -> Unit Kerja) when
// that's been filled in -- this is the authoritative source, since it's a
// deliberate choice rather than a guess; (2) falling back to guessing from
// the pegawai's free-text Tempat Tugas field (isSekolahFromTempatTgs) for a
// pegawai whose unit kerja has no Tempat Kerja set, or who isn't linked to
// any unit kerja at all -- keeping old data compatible without requiring
// administrator to fill in Tempat Kerja for every unit kerja right away.
//
// This is used to pick the 5-day/6-day work week (sixDayWeekForPegawai), the
// absen jam kerja window (jamAbsenUntukPegawai in absensi.go), and whether
// the "Surat Rekomendasi Kepala Sekolah" document applies (that
// recommendation only makes sense when the pegawai actually has a Kepala
// Sekolah, i.e. is posted at a school; pegawai posted directly at the Dinas
// don't have one and so are never asked to upload it).
func isSekolahPegawai(pegawai models.Pegawai) bool {
	if uk := pegawai.UnitKerja; uk != nil && uk.TempatKerja != nil && *uk.TempatKerja != "" {
		return *uk.TempatKerja == models.TempatKerjaSekolah
	}
	return isSekolahFromTempatTgs(pegawai.TempatTgs)
}

// sixDayWeekForPegawai decides whether a pegawai works a 6-day week
// (Senin-Sabtu) or the default 5-day week (Senin-Jumat), based on their
// dinas/sekolah status (isSekolahPegawai): pegawai sekolah work 6 days a
// week, everyone else (dinas/kantor/etc) works 5 days a week. This replaces
// manual pola-hari-kerja selection -- the work pattern is now derived
// automatically per pegawai.
func sixDayWeekForPegawai(pegawai models.Pegawai) bool {
	return isSekolahPegawai(pegawai)
}

// dokumenRequirementsForJenis returns the checklist of supporting documents
// for a jenis cuti, following the office's document-completeness rules.
// Order matters: more specific keywords (melahirkan/umroh/sakit/alasan
// penting) are checked before the generic "tahunan" fallback, since e.g.
// "Cuti Tahunan Umroh" contains both "tahunan" and "umroh".
//
// "Surat Rekomendasi Kepala Sekolah" used to be a mandatory manual upload
// here (for pegawai bertempat tugas Sekolah) -- permintaan pengguna: begitu
// Kepala Sekolah (atasan langsung pegawai) meng-ACC pengajuan ini lewat menu
// Pengajuan Cuti, surat itu sekarang dibuat OTOMATIS oleh sistem (lengkap
// dengan nomor surat otomatis & tanda tangan QR), jadi TIDAK LAGI ada di
// checklist ini -- lihat snapshot Kepsek.../NomorUrutKepsek di
// approvePengajuan & buildSuratRekomendasiKepsek di
// handlers/pengajuan_cuti_rekomendasi_kepsek.go. isSekolah TETAP diterima
// sebagai parameter (dipanggil dengan isSekolahPegawai(pegawai) di
// createPengajuan) walau sudah tidak dipakai di badan fungsi ini lagi --
// dibiarkan untuk kompatibilitas pemanggil & kalau suatu saat ada lagi
// syarat berkas yang hanya berlaku khusus pegawai sekolah.
func dokumenRequirementsForJenis(jenisNama string, isSekolah bool) []dokumenRequirement {
	j := strings.ToLower(jenisNama)
	switch {
	case strings.Contains(j, "melahirkan"):
		return []dokumenRequirement{
			{Key: "sk_terakhir", Label: "SK Terakhir", Required: true},
			{Key: "keterangan_hpl", Label: "Surat Keterangan HPL (Rumah Sakit/Puskesmas)", Required: true},
			{Key: "buku_kia", Label: "Buku KIA", Required: true},
			{Key: "hasil_usg", Label: "Hasil USG", Required: false},
		}
	case strings.Contains(j, "umroh"):
		return []dokumenRequirement{
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
			{Key: "dokumen_pendukung", Label: "Dokumen Pendukung (surat ket. rawat inap keluarga / surat kematian / surat KUA / dokumen istri melahirkan)", Required: true},
		}
	case strings.Contains(j, "tahunan"):
		return []dokumenRequirement{
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

// isFullCalendarLeave decides whether a jenis_cuti is counted in FULL
// CALENDAR DAYS -- every date from tgl_mulai to tgl_selesai, termasuk hari
// Sabtu/Minggu dan tanggal merah -- instead of hari kerja saja. Sesuai aturan
// kepegawaian, cuti sakit dan cuti melahirkan dihitung penuh per kalender
// (tidak dikurangi akhir pekan/libur), berbeda dari cuti tahunan dan jenis
// cuti lain yang cuma menghitung hari kerja.
func isFullCalendarLeave(jenisNama string) bool {
	j := strings.ToLower(jenisNama)
	return strings.Contains(j, "sakit") || strings.Contains(j, "melahirkan")
}

// calculateWorkingDays counts the days between start and end (inclusive) that
// should count against a pengajuan cuti's JumlahHari. For cuti sakit/
// melahirkan (isFullCalendarLeave true) EVERY date is counted, including
// Sabtu/Minggu/tanggal merah. For every other jenis cuti, only hari kerja are
// counted: Sundays are always excluded, Saturdays are excluded unless
// sixDayWeek is true (pegawai bertugas di sekolah), and any date listed in
// tgl_merah (public holidays) is always excluded.
func calculateWorkingDays(db *gorm.DB, start, end time.Time, sixDayWeek bool, jenisNama string) int {
	fullCalendar := isFullCalendarLeave(jenisNama)

	holidaySet := map[string]bool{}
	if !fullCalendar {
		var holidays []models.TglMerah
		db.Where("tgl BETWEEN ? AND ?", start, end).Find(&holidays)
		for _, h := range holidays {
			holidaySet[h.Tgl.Format("2006-01-02")] = true
		}
	}

	count := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if !fullCalendar {
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
		}
		count++
	}
	return count
}

// calculateWorkingDaysWithHariSet adalah padanan calculateWorkingDays di
// atas, tapi menerima pola hari kerja MINGGUAN pegawai yang sebenarnya
// (hariSet, lihat hariKerjaMingguanUntukPegawai di absensi.go -- ikut Shift
// Kerja yang terpasang pada Jabatan/Unit Kerja kalau ada, fallback ke
// sixDayWeekForPegawai kalau belum) alih-alih sekadar boolean sixDayWeek --
// dipakai di SEMUA tempat yang menghitung JumlahHari cuti supaya jatah cuti
// pegawai yang jam kerjanya diatur lewat Shift Kerja (mis. "Penilik" 6 hari
// kerja) terhitung benar, konsisten dengan rekap/PDF absen & jendela kamera
// absen (jamAbsenUntukPegawai).
func calculateWorkingDaysWithHariSet(db *gorm.DB, start, end time.Time, hariSet map[int]bool, jenisNama string) int {
	fullCalendar := isFullCalendarLeave(jenisNama)

	holidaySet := map[string]bool{}
	if !fullCalendar {
		var holidays []models.TglMerah
		db.Where("tgl BETWEEN ? AND ?", start, end).Find(&holidays)
		for _, h := range holidays {
			holidaySet[h.Tgl.Format("2006-01-02")] = true
		}
	}

	count := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if !fullCalendar {
			if !hariSet[int(d.Weekday())] {
				continue
			}
			if holidaySet[d.Format("2006-01-02")] {
				continue
			}
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

// syncJumlahHariWithHolidays recalculates JumlahHari using the current
// tgl_merah (hari libur) data and, if it no longer matches what was stored
// when the pengajuan was submitted/edited, persists the correction. This is
// what makes a newly-added tanggal merah get picked up automatically by an
// existing pengajuan cuti the moment the page is refreshed (GET list/detail)
// -- nobody needs to open and re-save the pengajuan by hand.
//
// Only "masih berjalan" statuses (menunggu/disetujui Kepala Sekolah/
// disetujui) are synced: a pengajuan yang sudah ditolak/dikembalikan is a
// closed-out record and its historical JumlahHari is left alone.
// StatusDisetujuiKepsek (tahap 1 pegawai sekolah sudah disetujui Kepala
// Sekolah, menunggu tahap 2/Dinas) ikut disinkronkan juga -- kuotanya memang
// belum dipotong di tahap ini (lihat approvePengajuan), jadi tidak ada
// penyesuaian jatah cuti yang perlu dilakukan untuk status ini, tapi
// JumlahHari-nya tetap harus akurat karena sudah tercetak di Surat
// Rekomendasi Kepala Sekolah. When the affected pengajuan is an approved cuti
// tahunan (StatusDisetuju, final), the difference is also applied to the
// pegawai's jatah cuti tahunan (kolom "terpakai") so a hari libur baru that
// falls inside an already disetujui range is no longer counted against the
// quota either.
func syncJumlahHariWithHolidays(db *gorm.DB, item *models.PengajuanCuti) {
	if item.Pegawai == nil || (item.Status != models.StatusPending && item.Status != models.StatusDisetujuiKepsek && item.Status != models.StatusDisetuju) {
		return
	}
	jenisNama := ""
	if item.JenisCuti != nil {
		jenisNama = item.JenisCuti.Jenis
	}
	newJumlahHari := calculateWorkingDaysWithHariSet(db, item.TglMulai, item.TglSelesai, hariKerjaMingguanUntukPegawai(db, *item.Pegawai), jenisNama)
	if newJumlahHari == item.JumlahHari {
		return
	}
	if item.Status == models.StatusDisetuju && item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
		delta := newJumlahHari - item.JumlahHari
		_ = adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, delta)
	}
	item.JumlahHari = newJumlahHari
	db.Model(item).Update("jumlah_hari", newJumlahHari)
}

// ResyncActivePengajuanDays re-checks every "masih berjalan" (menunggu/
// disetujui) pengajuan cuti against the current tgl_merah data and corrects
// JumlahHari (and, for disetujui cuti tahunan, jatah cuti terpakai) right
// away. It's wired up (see master_routes.go) to run immediately after a
// tanggal merah is created, edited, or deleted, so the effect is visible the
// moment that change is saved -- not only the next time someone happens to
// open/refresh a given pengajuan cuti (that lazy per-row correction still
// happens too, in listPengajuan/getPengajuan/approve/delete/return below;
// this is the eager, "push" counterpart so the numbers are already right
// even before anyone looks at the pengajuan cuti page again).
func ResyncActivePengajuanDays(db *gorm.DB) {
	var items []models.PengajuanCuti
	preloadPengajuan(db).
		Where("status IN ?", []string{models.StatusPending, models.StatusDisetujuiKepsek, models.StatusDisetuju}).
		Find(&items)
	for i := range items {
		syncJumlahHariWithHolidays(db, &items[i])
	}
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
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	// anyRole di sini sengaja TIDAK benar-benar "role apa saja" -- hanya 4
	// role dasar yang ada di sistem (administrator, admin, pegawai, atasan).
	// "Admin Absensi" bukan role tersendiri, melainkan tanda tambahan
	// (User.IsAdminAbsensi, lihat RegisterAbsensiRoutes di absensi.go) pada
	// akun ber-role pegawai/atasan/dll, jadi akun tersebut tetap otomatis
	// tercakup di sini lewat role dasarnya.
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "pegawai", "atasan") }
	// approve/reject/kembalikan/return: atasan (bawahannya sendiri saja,
	// BUKAN pengajuan miliknya sendiri -- lihat guard isSelfSubmission di
	// masing-masing handler) DAN admin/administrator (siapa saja, termasuk
	// pengajuan milik atasan sendiri).
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
	// formulir cetak -- hanya bisa diunduh setelah pengajuan disetujui, lihat
	// loadApprovedPengajuanForForm di formulir.go.
	mux.Handle("GET /api/pengajuan-cuti/{id}/form/rekomendasi", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadFormRekomendasi(w, r, db) }))
	mux.Handle("GET /api/pengajuan-cuti/{id}/form/cuti", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadFormCuti(w, r, db) }))
	// "Surat Rekomendasi Kepala Sekolah" -- permintaan pengguna, dibuat
	// OTOMATIS begitu pengajuan disetujui (lihat snapshot KepsekNama dkk. di
	// approvePengajuan di atas & buildSuratRekomendasiKepsek di
	// pengajuan_cuti_rekomendasi_kepsek.go), menggantikan upload manual
	// "rekomendasi_kepsek" yang dulu wajib.
	mux.Handle("GET /api/pengajuan-cuti/{id}/form/rekomendasi-kepsek", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadFormRekomendasiKepsek(w, r, db) }))
	// gabungan (Surat Rekomendasi + Formulir Cuti dalam SATU PDF 2 halaman)
	// untuk tombol "Download Sekaligus" di akun pegawai -- lihat
	// downloadFormGabungan & buildGabungan di formulir.go.
	mux.Handle("GET /api/pengajuan-cuti/{id}/form/gabungan", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadFormGabungan(w, r, db) }))
	// upload/hapus hasil scan formulir yang sudah ditandatangani basah oleh
	// Kepala Dinas -- hanya admin/administrator. Cukup SATU berkas per
	// pengajuan (Surat Rekomendasi + Formulir Cuti yang sudah ditandatangani
	// biasanya sudah discan jadi satu file), disimpan sebagai PengajuanDokumen
	// biasa (jenis "ttd_formulir") sehingga endpoint GET .../dokumen/{jenis}
	// di atas otomatis bisa dipakai semua role (termasuk pegawai pemilik
	// pengajuan) untuk melihat/mengunduhnya.
	mux.Handle("POST /api/pengajuan-cuti/{id}/form/ttd", manage(func(w http.ResponseWriter, r *http.Request) {
		uploadFormSigned(w, r, db, "ttd_formulir", "Berkas Pengajuan Cuti (Sudah TTD Kepala Dinas)")
	}))
	mux.Handle("DELETE /api/pengajuan-cuti/{id}/form/ttd", manage(func(w http.ResponseWriter, r *http.Request) {
		deleteFormSigned(w, r, db, "ttd_formulir")
	}))
	// nomor surat pada Surat Rekomendasi Izin Cuti -- boleh diisi manual atau
	// dibiarkan kosong, dan hanya administrator/admin yang boleh mengisinya.
	mux.Handle("PUT /api/pengajuan-cuti/{id}/nomor-surat", manage(func(w http.ResponseWriter, r *http.Request) { updateNomorSurat(w, r, db) }))
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
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
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
		// scope=saya: tab "Cuti Saya" di frontend -- atasan melihat pengajuan
		// MILIKNYA SENDIRI, persis seperti pegawai biasa (permintaan pengguna
		// "atasan dapat mengajukan cuti seperti pegawai biasa"). Tanpa
		// scope (default/"bawahan"): tab "Cuti Bawahan", perilaku lama --
		// hanya pengajuan bawahan langsungnya untuk ditinjau/disetujui.
		if q.Get("scope") == "saya" {
			query = query.Where("id_pegawai = ?", *claims.IDPegawai)
			countQuery = countQuery.Where("id_pegawai = ?", *claims.IDPegawai)
		} else {
			sub := "id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)"
			query = query.Where(sub, *claims.IDPegawai)
			countQuery = countQuery.Where(sub, *claims.IDPegawai)
		}
	}

	if status := q.Get("status"); status != "" {
		query = query.Where("status = ?", status)
		countQuery = countQuery.Where("status = ?", status)
	}

	var total int64
	countQuery.Count(&total)
	var items []models.PengajuanCuti
	query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items)
	for i := range items {
		syncJumlahHariWithHolidays(db, &items[i])
	}
	utils.SuccessMeta(w, "ok", items, map[string]interface{}{"page": page, "pageSize": pageSize, "total": total})
}

func canAccessPengajuan(claims *utils.Claims, item models.PengajuanCuti) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "pegawai":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	case "atasan":
		// atasan boleh akses pengajuan BAWAHAN langsungnya (pola lama) MAUPUN
		// pengajuan MILIKNYA SENDIRI (permintaan pengguna "atasan dapat
		// mengajukan cuti seperti pegawai biasa") -- lihat juga tab "Cuti
		// Saya" di PengajuanCutiView.vue & scope=saya di listPengajuan di bawah.
		if claims.IDPegawai == nil {
			return false
		}
		if item.IDPegawai == *claims.IDPegawai {
			return true
		}
		return item.Pegawai != nil && item.Pegawai.IDAtasan != nil && *item.Pegawai.IDAtasan == *claims.IDPegawai
	}
	return false
}

// isSelfSubmission: true kalau pengajuan ini diajukan sendiri oleh pemilik
// akun yang sedang login (pegawai ATAU atasan mengajukan untuk dirinya
// sendiri) -- dipakai untuk membatasi edit/hapus hanya saat status masih
// "pending"/"dikembalikan", SAMA seperti aturan pegawai biasa. Sengaja
// TIDAK true untuk atasan yang mengakses pengajuan BAWAHANNYA (itu
// ranahnya approve/reject/kembalikan, bukan edit/hapus isi pengajuan).
func isSelfSubmission(claims *utils.Claims, item models.PengajuanCuti) bool {
	if claims.IDPegawai == nil {
		return false
	}
	return (claims.RoleName == "pegawai" || claims.RoleName == "atasan") && item.IDPegawai == *claims.IDPegawai
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
	syncJumlahHariWithHolidays(db, &item)
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
	utils.LimitBody(w, r, 20<<20)
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
	if err := db.Preload("UnitKerja").First(&pegawai, p.IDPegawai).Error; err != nil {
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
	// skTerakhirFromPegawai: true kalau syarat "sk_terakhir" dipenuhi dengan
	// MENYALIN berkas yang sudah tersimpan di Data Pegawai (Pegawai.SkTerakhirFile),
	// bukan upload baru kali ini -- permintaan pengguna: "cek pada data
	// pegawai jika sknya sudah ada pada data pegawai maka munculkan sk
	// terakhir yang sudah ada di data pegawai jika ingin di ganti maka
	// buatkan inputan untuk mengupload kembali". Baris PengajuanDokumen
	// jenis "sk_terakhir" tetap selalu dibuat (lihat di bawah, setelah item
	// berhasil disimpan) supaya endpoint unduh/lihat dokumen yang sudah ada
	// (downloadDokumenPengajuan) tidak perlu berubah sama sekali -- baik
	// berkasnya baru diupload MAUPUN disalin dari Data Pegawai sama-sama
	// berakhir sebagai baris PengajuanDokumen biasa.
	var skTerakhirFromPegawai *dokumenRequirement
	var pending []pendingDoc
	var problems []string
	for _, req := range dokumenRequirementsForJenis(jenis.Jenis, isSekolahPegawai(pegawai)) {
		fh := formFileHeader(r, "dokumen_"+req.Key)
		if fh == nil {
			if req.Key == "sk_terakhir" && len(pegawai.SkTerakhirFile) > 0 {
				reqCopy := req
				skTerakhirFromPegawai = &reqCopy
				continue
			}
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
		// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering terjadi
		// kalau foto dari WhatsApp/Google Photos di HP belum selesai
		// diunduh ke perangkat saat dipilih lewat file picker.
		if len(data) == 0 {
			problems = append(problems, "berkas '"+req.Label+"' yang dipilih kosong (0 byte) -- coba pilih ulang")
			continue
		}
		pending = append(pending, pendingDoc{req: req, name: fh.Filename, data: data})
	}
	if len(problems) > 0 {
		utils.Error(w, http.StatusBadRequest, strings.Join(problems, "; "))
		return
	}

	sixDayWeek := sixDayWeekForPegawai(pegawai)
	jumlahHari := calculateWorkingDaysWithHariSet(db, start, end, hariKerjaMingguanUntukPegawai(db, pegawai), jenis.Jenis)
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
		if pd.req.Key == "sk_terakhir" {
			// Pegawai mengupload SK Terakhir BARU saat mengajukan cuti ini --
			// perbarui juga salinan master di Data Pegawai supaya pengajuan
			// cuti BERIKUTNYA otomatis memakai yang terbaru ini tanpa perlu
			// upload ulang lagi (permintaan pengguna, lihat komentar
			// skTerakhirFromPegawai di atas).
			db.Model(&models.Pegawai{}).Where("id = ?", item.IDPegawai).Updates(map[string]interface{}{
				"sk_terakhir_nama": pd.name,
				"sk_terakhir_file": pd.data,
			})
		}
	}
	if skTerakhirFromPegawai != nil {
		db.Create(&models.PengajuanDokumen{
			IDPengajuan: item.ID,
			Jenis:       skTerakhirFromPegawai.Key,
			Label:       skTerakhirFromPegawai.Label,
			NamaFile:    pegawai.SkTerakhirNama,
			File:        pegawai.SkTerakhirFile,
		})
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
	if isSelfSubmission(claims, item) && item.Status != models.StatusPending && item.Status != models.StatusDikembalikan {
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
		utils.LimitBody(w, r, 20<<20)
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
		id    uint
		jenis string
		name  string
		data  []byte
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
				if isSelfSubmission(claims, item) {
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
			// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering
			// terjadi kalau foto dari WhatsApp/Google Photos di HP belum
			// selesai diunduh ke perangkat saat dipilih lewat file picker.
			if len(data) == 0 {
				utils.Error(w, http.StatusBadRequest, "berkas '"+d.Label+"' yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
				return
			}
			replacements = append(replacements, docReplace{id: d.ID, jenis: d.Jenis, name: fh.Filename, data: data})
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
		if rep.jenis == "sk_terakhir" {
			// Pegawai mengganti SK Terakhir lewat upload ulang ini --
			// perbarui juga salinan master di Data Pegawai, sama seperti
			// jalur createPengajuan (lihat komentar skTerakhirFromPegawai
			// di sana), supaya pengajuan cuti berikutnya otomatis memakai
			// SK terbaru ini.
			db.Model(&models.Pegawai{}).Where("id = ?", item.IDPegawai).Updates(map[string]interface{}{
				"sk_terakhir_nama": rep.name,
				"sk_terakhir_file": rep.data,
			})
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
	db.Preload("UnitKerja").First(&pegawai, item.IDPegawai)
	sixDayWeek := sixDayWeekForPegawai(pegawai)
	var jenisForHari models.JenisCuti
	db.First(&jenisForHari, item.IDJenisCuti)
	item.JumlahHari = calculateWorkingDaysWithHariSet(db, start, end, hariKerjaMingguanUntukPegawai(db, pegawai), jenisForHari.Jenis)
	item.IDPolaHariKerja = autoPolaID(db, sixDayWeek)

	// editing resets it back to pending so the approval flow runs again --
	// snapshot penandatangan/ACC lama juga dikosongkan supaya persetujuan
	// berikutnya menghitung ulang dari Pengaturan Formulir yang berlaku saat
	// itu (lihat catatan pada field TtdNama dkk. di models.go).
	if item.Status != models.StatusPending {
		item.Status = models.StatusPending
		item.IDAtasanApprove = nil
		item.TglApproval = nil
		item.CatatanApproval = ""
		item.TtdNama = ""
		item.TtdNip = ""
		item.TtdJabatan = ""
		item.DisetujuiOlehUsername = ""
		item.DisetujuiOlehRole = ""
		// Snapshot Surat Rekomendasi Kepala Sekolah juga dikosongkan -- lihat
		// catatan lengkap pada field KepsekNama dkk. di models.go.
		item.KepsekNama = ""
		item.KepsekNip = ""
		item.KepsekPangkatGol = ""
		item.KepsekJabatan = ""
		item.NomorUrutKepsek = 0
		item.TahunSuratKepsek = 0
		item.TglApprovalKepsek = nil
		item.CatatanApprovalKepsek = ""
	}

	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal memperbarui data: "+err.Error())
		return
	}
	preloadPengajuan(db).First(&item, item.ID)
	utils.Success(w, "pengajuan cuti berhasil diperbarui", item)
}

type nomorSuratPayload struct {
	NomorSurat string `json:"nomor_surat"`
}

// updateNomorSurat sets (or clears) the nomor surat printed on the Surat
// Rekomendasi Izin Cuti (see buildSuratRekomendasi in formulir.go). It's
// intentionally optional -- an empty string is accepted and simply leaves the
// number blank on the printed letter, same as before this field existed --
// and restricted to administrator/admin only (see the route registration
// above, wrapped in manage(...)).
func updateNomorSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.PengajuanCuti
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	var p nomorSuratPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	item.NomorSurat = strings.TrimSpace(p.NomorSurat)
	if err := db.Model(&item).Update("nomor_surat", item.NomorSurat).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan nomor surat: "+err.Error())
		return
	}
	utils.Success(w, "nomor surat berhasil disimpan", item)
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
	if isSelfSubmission(claims, item) && item.Status != models.StatusPending && item.Status != models.StatusDikembalikan {
		utils.Error(w, http.StatusBadRequest, "pengajuan yang sudah diproses tidak dapat dihapus")
		return
	}
	syncJumlahHariWithHolidays(db, &item)
	if item.Status == models.StatusDisetuju && item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
		_ = adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, -item.JumlahHari)
	}
	// Dokumen kelengkapan (PengajuanDokumen) punya foreign key ke
	// pengajuan_cuti(id) TANPA cascade delete -- pengajuan yang masih punya
	// baris dokumen (berkas yang diupload pegawai, atau surat yang dihasilkan
	// sistem begitu disetujui) gagal dihapus dengan pelanggaran foreign key
	// kalau dokumennya tidak dihapus lebih dulu. Dilakukan dalam satu
	// transaksi supaya dokumen tidak ikut terhapus kalau baris pengajuannya
	// sendiri ternyata gagal dihapus.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id_pengajuan = ?", item.ID).Delete(&models.PengajuanDokumen{}).Error; err != nil {
			return err
		}
		return tx.Delete(&item).Error
	}); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus data: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan cuti berhasil dihapus", nil)
}

type approvalPayload struct {
	Catatan string `json:"catatan_approval"`
}

// pengajuanStageGateOK memeriksa apakah pengajuan ini boleh diproses
// (approve/reject/kembalikan) SEKARANG oleh role yang memanggil, berdasarkan
// status-nya SAAT INI:
//   - pending: tahap 1 -- boleh atasan (Kepala Sekolah/atasan langsung) ATAU
//     admin/administrator (override, pola lama).
//   - StatusDisetujuiKepsek: tahap 2 (Dinas) -- Kepala Sekolah SUDAH
//     menuntaskan bagiannya di tahap 1, jadi HANYA admin/administrator yang
//     boleh memproses tahap ini, atasan ditolak dengan pesan yang jelas.
//   - status lain (sudah final: disetujui/ditolak/dikembalikan): tidak boleh
//     diproses lagi siapa pun -- sama seperti perilaku lama, harus pakai
//     returnPengajuan dulu untuk mengembalikan ke pending.
//
// Mengirim response error sendiri & mengembalikan ok=false kalau tidak boleh
// diproses sama sekali. isStage2=true menandakan tindakan ini tindakan tahap
// 2 (Dinas) -- dipakai pemanggil (approve/reject/kembalikanPengajuan) untuk
// memutuskan field snapshot mana yang diisi (lihat catatan pada
// IDAtasanApprove/DisetujuiOlehUsername di handler masing-masing).
func pengajuanStageGateOK(w http.ResponseWriter, claims *utils.Claims, item models.PengajuanCuti) (isStage2 bool, ok bool) {
	switch item.Status {
	case models.StatusPending:
		return false, true
	case models.StatusDisetujuiKepsek:
		if claims.RoleName != "administrator" && claims.RoleName != "admin" {
			utils.Error(w, http.StatusForbidden, "pengajuan ini sudah disetujui Kepala Sekolah, menunggu persetujuan Dinas (hanya administrator/admin yang dapat memproses tahap ini)")
			return false, false
		}
		return true, true
	default:
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya")
		return false, false
	}
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
	// atasan TIDAK BOLEH menyetujui/menolak/mengembalikan pengajuan cuti
	// MILIKNYA SENDIRI (canAccessPengajuan di atas sengaja mengizinkan akses
	// lihat/edit/hapus untuk pengajuan sendiri -- lihat isSelfSubmission --
	// tapi memproses/approval tetap harus administrator/admin, bukan
	// menyetujui pengajuannya sendiri).
	if isSelfSubmission(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak dapat memproses pengajuan cuti milik sendiri, menunggu persetujuan administrator/admin")
		return
	}
	isStage2, ok := pengajuanStageGateOK(w, claims, item)
	if !ok {
		return
	}
	var p approvalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)

	syncJumlahHariWithHolidays(db, &item)
	now := time.Now()

	if isStage2 {
		// Tahap 2 (Dinas): pengajuan ini sudah disetujui Kepala Sekolah
		// sebelumnya (StatusDisetujuiKepsek), sekarang administrator/admin
		// menuntaskan/memfinalkan. Snapshot penandatangan Kepala Dinas DAN
		// pemotongan kuota cuti tahunan baru terjadi DI SINI, bukan di tahap
		// 1 -- supaya kuota tidak ikut terpotong untuk pengajuan yang
		// ternyata ditolak Dinas di tahap akhir ini. Snapshot Surat
		// Rekomendasi Kepala Sekolah (KepsekNama dkk.) TIDAK disentuh lagi,
		// sudah diisi sejak tahap 1.
		if item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
			if err := adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, item.JumlahHari); err != nil {
				utils.Error(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		pengaturan := pengaturanSuratOrDefault(db)
		item.TtdNama, item.TtdNip, item.TtdJabatan = resolveSignerInfo(db, pengaturan)
		item.DisetujuiOlehUsername = claims.Username
		item.DisetujuiOlehRole = claims.RoleName
		item.Status = models.StatusDisetuju
		item.TglApproval = &now
		item.CatatanApproval = p.Catatan

		if err := db.Save(&item).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyetujui pengajuan: "+err.Error())
			return
		}
		preloadPengajuan(db).First(&item, item.ID)
		utils.Success(w, "pengajuan cuti disetujui Dinas, proses selesai", item)
		return
	}

	// Tahap 1 (status masih pending): tentukan dulu apakah pengajuan ini
	// perlu DUA tahap (Kepala Sekolah -> Dinas, khusus pegawai bertempat
	// tugas Sekolah yang sudah punya Atasan/Kepala Sekolah terisi di Data
	// Pegawai) atau cukup SATU tahap seperti sebelum fitur dua tahap ini ada
	// (pegawai Dinas/Kantor, ATAU pegawai sekolah yang Atasannya belum
	// diisi administrator -- fallback supaya tidak memblokir proses).
	var pegawaiUntukKepsek models.Pegawai
	twoStage := false
	if err := db.Preload("UnitKerja").
		Preload("Atasan.Jabatan").
		Preload("Atasan.PangkatGol.Pangkat").
		Preload("Atasan.PangkatGol.Gol").
		First(&pegawaiUntukKepsek, item.IDPegawai).Error; err == nil &&
		isSekolahPegawai(pegawaiUntukKepsek) && pegawaiUntukKepsek.Atasan != nil && pegawaiUntukKepsek.IDUnitKerja != nil {
		twoStage = true
	}

	item.IDAtasanApprove = claims.IDPegawai
	item.TglApproval = &now
	item.CatatanApproval = p.Catatan

	if twoStage {
		// Kepala Sekolah meng-ACC -- BELUM final, menunggu tahap 2 (Dinas).
		// Snapshot Surat Rekomendasi Kepala Sekolah (KepsekNama dkk.) sudah
		// otomatis siap begitu tahap ini disimpan -- lihat catatan lengkap
		// pada field-field itu di models.go.
		item.Status = models.StatusDisetujuiKepsek
		item.TglApprovalKepsek = &now
		item.CatatanApprovalKepsek = p.Catatan

		atasan := pegawaiUntukKepsek.Atasan
		item.KepsekNama = atasan.Nama
		item.KepsekNip = atasan.NIP
		item.KepsekJabatan = "Kepala Sekolah"
		if atasan.Jabatan != nil && strings.TrimSpace(atasan.Jabatan.Jabatan) != "" {
			item.KepsekJabatan = atasan.Jabatan.Jabatan
		}
		if atasan.PangkatGol != nil {
			pk, gol := "-", "-"
			if atasan.PangkatGol.Pangkat != nil {
				pk = atasan.PangkatGol.Pangkat.Pangkat
			}
			if atasan.PangkatGol.Gol != nil {
				gol = atasan.PangkatGol.Gol.Gol
			}
			item.KepsekPangkatGol = pk + " / " + gol
		}
		item.TahunSuratKepsek = now.Year()
		item.NomorUrutKepsek = nextNomorUrutSuratKepsek(db, *pegawaiUntukKepsek.IDUnitKerja, now.Year())
	} else {
		// Satu tahap saja (pegawai Dinas/Kantor, atau pegawai sekolah yang
		// Atasannya belum diisi) -- langsung final, sama seperti sebelum
		// fitur dua tahap ini ada: snapshot penandatangan Kepala Dinas &
		// pemotongan kuota cuti tahunan terjadi langsung di tahap ini.
		if item.JenisCuti != nil && isAnnualLeave(*item.JenisCuti) {
			if err := adjustQuotaUsage(db, item.IDPegawai, item.TglMulai.Year(), item.JenisCuti.DefaultJatah, item.JumlahHari); err != nil {
				utils.Error(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		pengaturan := pengaturanSuratOrDefault(db)
		item.TtdNama, item.TtdNip, item.TtdJabatan = resolveSignerInfo(db, pengaturan)
		item.DisetujuiOlehUsername = claims.Username
		item.DisetujuiOlehRole = claims.RoleName
		item.Status = models.StatusDisetuju
	}

	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyetujui pengajuan: "+err.Error())
		return
	}
	preloadPengajuan(db).First(&item, item.ID)
	if twoStage {
		utils.Success(w, "pengajuan cuti disetujui Kepala Sekolah, menunggu persetujuan Dinas", item)
	} else {
		utils.Success(w, "pengajuan cuti berhasil disetujui", item)
	}
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
	// atasan TIDAK BOLEH memproses pengajuan cuti MILIKNYA SENDIRI -- lihat
	// catatan selengkapnya pada approvePengajuan.
	if isSelfSubmission(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak dapat memproses pengajuan cuti milik sendiri, menunggu persetujuan administrator/admin")
		return
	}
	isStage2, ok := pengajuanStageGateOK(w, claims, item)
	if !ok {
		return
	}
	var p approvalPayload
	_ = json.NewDecoder(r.Body).Decode(&p)

	now := time.Now()
	item.Status = models.StatusDitolak
	item.TglApproval = &now
	item.CatatanApproval = p.Catatan
	// Tahap 2 (Dinas): JANGAN timpa IDAtasanApprove -- field itu tetap
	// menyimpan identitas Kepala Sekolah yang menyetujui tahap 1, dipakai
	// status bar frontend (lihat cutiStepperSteps di PengajuanCutiView.vue).
	// Tahap 1 (atasan, atau admin yang override): catat seperti biasa.
	if isStage2 {
		item.DisetujuiOlehUsername = claims.Username
		item.DisetujuiOlehRole = claims.RoleName
	} else {
		item.IDAtasanApprove = claims.IDPegawai
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menolak pengajuan: "+err.Error())
		return
	}
	preloadPengajuan(db).First(&item, item.ID)
	// beri tahu pegawai yang mengajukan lewat lonceng notifikasi di
	// dashboard-nya -- lihat models.NotifikasiPegawai.
	jenisCutiNama := "Cuti"
	if item.JenisCuti != nil {
		jenisCutiNama = item.JenisCuti.Jenis
	}
	pesan := "Pengajuan " + jenisCutiNama + " Anda ditolak"
	if isStage2 {
		pesan = "Pengajuan " + jenisCutiNama + " Anda ditolak Dinas"
	}
	if p.Catatan != "" {
		pesan += " dengan catatan: " + p.Catatan
	}
	buatNotifikasiPegawai(db, item.IDPegawai, models.NotifikasiPegawaiJenisPengajuanCuti, item.ID,
		"Pengajuan Cuti Ditolak", pesan, "/pengajuan-cuti")
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
	// atasan TIDAK BOLEH memproses pengajuan cuti MILIKNYA SENDIRI -- lihat
	// catatan selengkapnya pada approvePengajuan.
	if isSelfSubmission(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak dapat memproses pengajuan cuti milik sendiri, menunggu persetujuan administrator/admin")
		return
	}
	isStage2, ok := pengajuanStageGateOK(w, claims, item)
	if !ok {
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
	item.TglApproval = &now
	item.CatatanApproval = p.Catatan
	// Tahap 2 (Dinas): JANGAN timpa IDAtasanApprove -- sama alasan dengan
	// rejectPengajuan di atas (tetap menyimpan identitas Kepala Sekolah
	// yang menyetujui tahap 1). Tahap 1: catat seperti biasa.
	if isStage2 {
		item.DisetujuiOlehUsername = claims.Username
		item.DisetujuiOlehRole = claims.RoleName
	} else {
		item.IDAtasanApprove = claims.IDPegawai
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengembalikan pengajuan: "+err.Error())
		return
	}
	preloadPengajuan(db).First(&item, item.ID)
	msg := "pengajuan cuti dikembalikan ke pegawai untuk diperbaiki"
	if isStage2 {
		msg = "pengajuan cuti dikembalikan Dinas ke pegawai untuk diperbaiki"
	}
	utils.Success(w, msg, item)
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
	// atasan TIDAK BOLEH memproses pengajuan cuti MILIKNYA SENDIRI -- lihat
	// catatan selengkapnya pada approvePengajuan.
	if isSelfSubmission(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak dapat memproses pengajuan cuti milik sendiri, menunggu persetujuan administrator/admin")
		return
	}
	if item.Status == models.StatusPending {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini masih menunggu, tidak perlu dikembalikan")
		return
	}
	syncJumlahHariWithHolidays(db, &item)
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
	// snapshot penandatangan/ACC lama juga dikosongkan -- kalau nanti
	// disetujui ulang, dihitung ulang dari Pengaturan Formulir yang berlaku
	// saat itu (lihat catatan pada field TtdNama dkk. di models.go).
	item.TtdNama = ""
	item.TtdNip = ""
	item.TtdJabatan = ""
	item.DisetujuiOlehUsername = ""
	item.DisetujuiOlehRole = ""
	// Snapshot Surat Rekomendasi Kepala Sekolah juga dikosongkan -- lihat
	// catatan lengkap pada field KepsekNama dkk. di models.go.
	item.KepsekNama = ""
	item.KepsekNip = ""
	item.KepsekPangkatGol = ""
	item.KepsekJabatan = ""
	item.NomorUrutKepsek = 0
	item.TahunSuratKepsek = 0
	item.TglApprovalKepsek = nil
	item.CatatanApprovalKepsek = ""
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
	preloadPengajuan(db).Order("created_at desc").Find(&items)
	f, err := utils.ExportData(items, pengajuanExcelColumns(db))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXlsxResponse(w, f, "data_pengajuan_cuti.xlsx")
}

func importPengajuan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	utils.LimitBody(w, r, 10<<20)
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
		// Sama seperti deletePengajuan -- PengajuanDokumen punya foreign key
		// ke pengajuan_cuti(id) tanpa cascade delete, jadi harus dihapus lebih
		// dulu supaya menghapus seluruh pengajuan_cuti lama tidak gagal karena
		// pelanggaran foreign key.
		if err := deleteAllRows(db, &models.PengajuanDokumen{}); err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal menghapus dokumen lama: "+err.Error())
			return
		}
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
			if err := db.Preload("UnitKerja").First(&peg, item.IDPegawai).Error; err == nil {
				sixDayWeek = sixDayWeekForPegawai(peg)
			}
			item.IDPolaHariKerja = autoPolaID(db, sixDayWeek)
			var jenisForHari models.JenisCuti
			db.First(&jenisForHari, item.IDJenisCuti)
			item.JumlahHari = calculateWorkingDaysWithHariSet(db, item.TglMulai, item.TglSelesai, hariKerjaMingguanUntukPegawai(db, peg), jenisForHari.Jenis)
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
