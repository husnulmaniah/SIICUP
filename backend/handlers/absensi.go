package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// absensi.go implements menu "Absen": absen masuk/pulang lewat kamera +
// verifikasi kedipan mata (liveness check dijalankan di frontend -- lihat
// AbsensiView.vue -- backend hanya menerima hasilnya lewat field kedipan_ok
// dan menyimpan foto hasil capture sebagai bukti), riwayat absen milik
// sendiri (termasuk tanggal yang terlewat), upload surat pengganti untuk
// tanggal terlewat, serta rekap admin & pengaturan modul (jam & aktif/
// nonaktif) -- lihat absensi_admin.go untuk bagian admin/rekap/export.

// absensiLocation mengembalikan zona waktu WITA (Asia/Makassar, sama dengan
// TimeZone koneksi database -- lihat config/database.go) supaya "hari ini"
// dan perbandingan jam absen tidak tergantung timezone OS container backend
// (yang belum tentu WITA). Fallback ke offset UTC+8 tetap jika tzdata tidak
// tersedia di container.
func absensiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Makassar")
	if err != nil {
		return time.FixedZone("WITA", 8*3600)
	}
	return loc
}

func absensiNow() time.Time {
	return time.Now().In(absensiLocation())
}

func absensiToday() time.Time {
	now := absensiNow()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// parseJamToMinutes mengubah "HH:MM" jadi jumlah menit sejak tengah malam.
func parseJamToMinutes(s string) (int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func parseFloatForm(r *http.Request, key string) *float64 {
	raw := strings.TrimSpace(r.FormValue(key))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}

func getPengaturanAbsensi(db *gorm.DB) (models.PengaturanAbsensi, error) {
	var item models.PengaturanAbsensi
	err := db.First(&item, 1).Error
	return item, err
}

func RegisterAbsensiRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai", "atasan") }
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	// pengaturan (aktif/jam) -- dibaca semua role yang login supaya frontend
	// pegawai tahu jendela waktu & status aktif; hanya admin/administrator
	// yang boleh mengubahnya (lihat absensi_admin.go).
	mux.Handle("GET /api/absensi/pengaturan", anyRole(func(w http.ResponseWriter, r *http.Request) { getPengaturanAbsensiHandler(w, r, db) }))

	mux.Handle("POST /api/absensi/masuk", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { absenMasuk(w, r, db) }))
	mux.Handle("POST /api/absensi/pulang", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { absenPulang(w, r, db) }))
	mux.Handle("GET /api/absensi/saya", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { riwayatAbsenSaya(w, r, db) }))
	mux.Handle("GET /api/absensi/foto/{id}/{jenis}", anyRole(func(w http.ResponseWriter, r *http.Request) { fotoAbsensi(w, r, db) }))

	// surat pengganti tanggal terlewat
	mux.Handle("POST /api/absensi/dokumen", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { uploadAbsensiDokumen(w, r, db) }))
	mux.Handle("GET /api/absensi/dokumen", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { listAbsensiDokumenSaya(w, r, db) }))
	mux.Handle("GET /api/absensi/dokumen/{id}/file", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadAbsensiDokumen(w, r, db) }))
	mux.Handle("DELETE /api/absensi/dokumen/{id}", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { deleteAbsensiDokumen(w, r, db) }))

	// admin/administrator saja
	mux.Handle("PUT /api/absensi/pengaturan", manage(func(w http.ResponseWriter, r *http.Request) { updatePengaturanAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap", manage(func(w http.ResponseWriter, r *http.Request) { rekapAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap/export", manage(func(w http.ResponseWriter, r *http.Request) { exportRekapAbsensi(w, r, db) }))
}

func getPengaturanAbsensiHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, err := getPengaturanAbsensi(db)
	if err != nil {
		utils.Error(w, http.StatusNotFound, "pengaturan absensi belum tersedia")
		return
	}
	utils.Success(w, "ok", item)
}

// holidaySetInRange mengambil semua tgl_merah dalam rentang sekali saja,
// supaya rekap yang mengulang perhitungan hari kerja untuk banyak pegawai
// (lihat absensi_admin.go) tidak query tgl_merah berulang-ulang per pegawai.
func holidaySetInRange(db *gorm.DB, start, end time.Time) map[string]bool {
	var holidays []models.TglMerah
	db.Where("tgl BETWEEN ? AND ?", start, end).Find(&holidays)
	set := map[string]bool{}
	for _, h := range holidays {
		set[h.Tgl.Format("2006-01-02")] = true
	}
	return set
}

// workingDaysWithHolidaySet mengembalikan daftar TANGGAL (bukan cuma
// jumlahnya -- beda dengan calculateWorkingDays di pengajuan_cuti.go) yang
// terhitung hari kerja: Minggu selalu libur, Sabtu libur kecuali sixDayWeek
// (pegawai bertugas di sekolah), dan tanggal apapun di holidaySet.
func workingDaysWithHolidaySet(start, end time.Time, sixDayWeek bool, holidaySet map[string]bool) []time.Time {
	var days []time.Time
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
		days = append(days, d)
	}
	return days
}

// workingDaysInRange adalah workingDaysWithHolidaySet untuk satu pegawai
// (query tgl_merah langsung) -- dipakai saat hanya butuh satu pegawai
// (riwayatAbsenSaya).
func workingDaysInRange(db *gorm.DB, start, end time.Time, sixDayWeek bool) []time.Time {
	return workingDaysWithHolidaySet(start, end, sixDayWeek, holidaySetInRange(db, start, end))
}

// absenMasuk mencatat absen masuk pegawai: menerima multipart/form-data
// dengan field file "foto" (hasil auto-capture kamera setelah kedipan mata
// terdeteksi), "lat"/"lng" (titik koordinat GPS), dan "kedipan_ok"
// ("true"/"false" -- hasil deteksi liveness dari frontend, dipakai untuk
// menampilkan peringatan, bukan untuk menolak absennya).
func absenMasuk(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}

	setting, err := getPengaturanAbsensi(db)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "pengaturan absensi belum tersedia, hubungi administrator")
		return
	}
	if !setting.Aktif {
		utils.Error(w, http.StatusForbidden, "menu absen sedang dinonaktifkan oleh administrator")
		return
	}

	now := absensiNow()
	today := absensiToday()
	nowMin := now.Hour()*60 + now.Minute()

	if mulaiMin, ok := parseJamToMinutes(setting.JamMulaiPagi); ok && nowMin < mulaiMin {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf("belum waktunya absen masuk, dibuka mulai jam %s", setting.JamMulaiPagi))
		return
	}

	var existing models.Absensi
	found := db.Where("id_pegawai = ? AND tanggal = ?", *claims.IDPegawai, today).First(&existing).Error == nil
	if found && existing.JamMasuk != nil {
		utils.Error(w, http.StatusBadRequest, "anda sudah absen masuk hari ini")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 10MB)")
		return
	}
	fh := formFileHeader(r, "foto")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "foto wajib diambil dari kamera saat absen")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca foto")
		return
	}
	fotoBytes, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca foto")
		return
	}

	kedipanOk := r.FormValue("kedipan_ok") != "false"
	lat := parseFloatForm(r, "lat")
	lng := parseFloatForm(r, "lng")

	terlambat := 0
	if batasMin, ok := parseJamToMinutes(setting.JamBatasPagi); ok && nowMin > batasMin {
		terlambat = nowMin - batasMin
	}

	jamMasuk := now
	if found {
		existing.JamMasuk = &jamMasuk
		existing.TerlambatMenit = terlambat
		existing.FotoMasuk = fotoBytes
		existing.LatMasuk = lat
		existing.LngMasuk = lng
		existing.KedipanMasukOk = kedipanOk
		if err := db.Save(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen masuk: "+err.Error())
			return
		}
	} else {
		existing = models.Absensi{
			IDPegawai:      *claims.IDPegawai,
			Tanggal:        today,
			JamMasuk:       &jamMasuk,
			TerlambatMenit: terlambat,
			FotoMasuk:      fotoBytes,
			LatMasuk:       lat,
			LngMasuk:       lng,
			KedipanMasukOk: kedipanOk,
		}
		if err := db.Create(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen masuk: "+err.Error())
			return
		}
	}

	msg := "absen masuk berhasil dicatat"
	if terlambat > 0 {
		msg += fmt.Sprintf(" (terlambat %d menit)", terlambat)
	}
	if !kedipanOk {
		msg += " -- peringatan: kedipan mata tidak terdeteksi pada foto, pastikan wajah terlihat jelas oleh kamera"
	}
	existing.FotoMasuk = nil
	existing.FotoPulang = nil
	utils.Created(w, msg, existing)
}

// absenPulang mencatat absen pulang, mengikuti pola yang sama dengan
// absenMasuk. Baris absen hari ini harus sudah ada dari absen masuk --
// kalau belum ada sama sekali baris dibuat langsung dengan JamMasuk kosong,
// supaya pegawai yang lupa/gagal absen masuk tetap bisa merekam kepulangannya.
func absenPulang(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}

	setting, err := getPengaturanAbsensi(db)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "pengaturan absensi belum tersedia, hubungi administrator")
		return
	}
	if !setting.Aktif {
		utils.Error(w, http.StatusForbidden, "menu absen sedang dinonaktifkan oleh administrator")
		return
	}

	now := absensiNow()
	today := absensiToday()
	nowMin := now.Hour()*60 + now.Minute()

	if mulaiMin, ok := parseJamToMinutes(setting.JamMulaiPulang); ok && nowMin < mulaiMin {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf("belum waktunya absen pulang, dibuka mulai jam %s", setting.JamMulaiPulang))
		return
	}

	var existing models.Absensi
	found := db.Where("id_pegawai = ? AND tanggal = ?", *claims.IDPegawai, today).First(&existing).Error == nil
	if found && existing.JamPulang != nil {
		utils.Error(w, http.StatusBadRequest, "anda sudah absen pulang hari ini")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 10MB)")
		return
	}
	fh := formFileHeader(r, "foto")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "foto wajib diambil dari kamera saat absen")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca foto")
		return
	}
	fotoBytes, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca foto")
		return
	}

	kedipanOk := r.FormValue("kedipan_ok") != "false"
	lat := parseFloatForm(r, "lat")
	lng := parseFloatForm(r, "lng")
	jamPulang := now

	if found {
		existing.JamPulang = &jamPulang
		existing.FotoPulang = fotoBytes
		existing.LatPulang = lat
		existing.LngPulang = lng
		existing.KedipanPulangOk = kedipanOk
		if err := db.Save(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen pulang: "+err.Error())
			return
		}
	} else {
		existing = models.Absensi{
			IDPegawai:       *claims.IDPegawai,
			Tanggal:         today,
			JamPulang:       &jamPulang,
			FotoPulang:      fotoBytes,
			LatPulang:       lat,
			LngPulang:       lng,
			KedipanPulangOk: kedipanOk,
		}
		if err := db.Create(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen pulang: "+err.Error())
			return
		}
	}

	msg := "absen pulang berhasil dicatat"
	if !kedipanOk {
		msg += " -- peringatan: kedipan mata tidak terdeteksi pada foto, pastikan wajah terlihat jelas oleh kamera"
	}
	existing.FotoMasuk = nil
	existing.FotoPulang = nil
	utils.Created(w, msg, existing)
}

type riwayatAbsenResponse struct {
	Bulan           int              `json:"bulan"`
	Tahun           int              `json:"tahun"`
	Absensi         []models.Absensi `json:"absensi"`
	TanggalTerlewat []string         `json:"tanggal_terlewat"`
}

// riwayatAbsenSaya mengembalikan riwayat absen pegawai yang login untuk satu
// bulan (default bulan & tahun berjalan, WITA), plus daftar tanggal_terlewat
// -- hari kerja pegawai (mengikuti pola 5/6 hari sesuai tempat tugas, sama
// seperti pengajuan cuti) sampai hari ini yang tidak punya baris Absensi
// (jam_masuk terisi) DAN tidak punya AbsensiDokumen (surat pengganti) --
// itulah tanggal yang perlu diupload suratnya.
func riwayatAbsenSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}

	now := absensiNow()
	bulan := now.Month()
	tahun := now.Year()
	if v, err := strconv.Atoi(r.URL.Query().Get("bulan")); err == nil && v >= 1 && v <= 12 {
		bulan = time.Month(v)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("tahun")); err == nil && v > 2000 {
		tahun = v
	}

	loc := now.Location()
	start := time.Date(tahun, bulan, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)
	today := absensiToday()
	limit := end
	if today.Before(limit) {
		limit = today
	}

	var pegawai models.Pegawai
	if err := db.First(&pegawai, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	// diinisialisasi sebagai slice kosong (bukan nil) supaya di-encode JSON
	// sebagai [] -- slice nil di Go ter-encode sebagai null, yang bikin
	// frontend (riwayat.absensi.find(...)/.filter(...)) crash saat pegawai
	// belum punya absen sama sekali di bulan yang dipilih.
	rows := []models.Absensi{}
	if err := db.Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", *claims.IDPegawai, start, end).
		Order("tanggal desc").Find(&rows).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil riwayat absen")
		return
	}
	// jangan kirim foto (bytea besar) di daftar riwayat -- dilihat lewat
	// endpoint fotoAbsensi tersendiri saat dibutuhkan.
	for i := range rows {
		rows[i].FotoMasuk = nil
		rows[i].FotoPulang = nil
	}

	hadirSet := map[string]bool{}
	for _, row := range rows {
		if row.JamMasuk != nil {
			hadirSet[row.Tanggal.Format("2006-01-02")] = true
		}
	}
	var dokumen []models.AbsensiDokumen
	db.Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", *claims.IDPegawai, start, limit).Find(&dokumen)
	tercoverSet := map[string]bool{}
	for _, d := range dokumen {
		tercoverSet[d.Tanggal.Format("2006-01-02")] = true
	}

	sixDayWeek := sixDayWeekForTempatTgs(pegawai.TempatTgs)
	terlewat := []string{}
	if !start.After(limit) {
		for _, d := range workingDaysInRange(db, start, limit, sixDayWeek) {
			key := d.Format("2006-01-02")
			if !hadirSet[key] && !tercoverSet[key] {
				terlewat = append(terlewat, key)
			}
		}
	}

	utils.Success(w, "ok", riwayatAbsenResponse{
		Bulan:           int(bulan),
		Tahun:           tahun,
		Absensi:         rows,
		TanggalTerlewat: terlewat,
	})
}

func fotoAbsensi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	jenis := r.PathValue("jenis")

	var item models.Absensi
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data absen tidak ditemukan")
		return
	}
	isOwner := claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	isAdmin := claims.RoleName == "administrator" || claims.RoleName == "admin"
	if !isOwner && !isAdmin {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}

	var foto []byte
	switch jenis {
	case "masuk":
		foto = item.FotoMasuk
	case "pulang":
		foto = item.FotoPulang
	default:
		utils.Error(w, http.StatusBadRequest, "jenis foto tidak valid")
		return
	}
	if len(foto) == 0 {
		utils.Error(w, http.StatusNotFound, "foto tidak ditemukan")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"absen_%s_%s.jpg\"", id, jenis))
	w.Write(foto)
}
