package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"sort"
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

// formatJamAbsensi memformat jam absen (jam masuk/pulang) ke "HH:MM" dalam
// zona WITA. Konversi zonanya WAJIB: driver database mengembalikan kolom
// timestamptz apa adanya (UTC), jadi memformat langsung tanpa .In() membuat
// jam pada export Excel/PDF meleset 8 jam dari jam absen sebenarnya. Di sisi
// frontend hal ini tidak terasa karena browser sudah mengubah sendiri string
// ISO-nya ke waktu lokal.
func formatJamAbsensi(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(absensiLocation()).Format("15:04")
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

// distanceMeters menghitung jarak (meter) antara dua titik koordinat bumi
// memakai formula Haversine -- dipakai untuk memvalidasi radius absen
// terhadap titik koordinat kantor (lihat absensiCekRadius).
func distanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const bumiRadiusMeter = 6371000.0
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return bumiRadiusMeter * c
}

// absensiCekRadius memvalidasi titik koordinat (lat,lng) hasil GPS pegawai
// terhadap titik koordinat kantor yang diatur administrator. Kalau
// KantorLat/KantorLng belum diatur (nil), geofence dianggap belum aktif dan
// absen tetap diperbolehkan tanpa validasi jarak (default terbuka, sama
// seperti filter tempat tugas/jabatan). Kalau geofence aktif tapi lat/lng
// pegawai tidak terdeteksi (GPS ditolak/gagal), absen ditolak karena jarak
// tidak bisa dipastikan.
func absensiCekRadius(setting models.PengaturanAbsensi, lat, lng *float64) (ok bool, pesan string) {
	if setting.KantorLat == nil || setting.KantorLng == nil {
		return true, ""
	}
	radius := setting.RadiusMeter
	if radius <= 0 {
		radius = 20
	}
	if lat == nil || lng == nil {
		return false, "lokasi GPS tidak terdeteksi. Aktifkan layanan lokasi pada perangkat/browser Anda dan izinkan akses lokasi, lalu coba lagi."
	}
	jarak := distanceMeters(*setting.KantorLat, *setting.KantorLng, *lat, *lng)
	if jarak > float64(radius) {
		return false, fmt.Sprintf("Anda berada di luar radius kantor (jarak sekitar %.0f meter, maksimal %d meter dari titik kantor). Absen tidak dapat dilakukan dari lokasi ini.", jarak, radius)
	}
	return true, ""
}

// absensiAllowedTempatTugas/absensiAllowedJabatanIDs mem-parse kolom JSON
// text PengaturanAbsensi.TempatTugasAllowed/JabatanAllowedIDs -- lihat
// komentar pada model untuk format & artinya (daftar kosong = filter itu
// tidak diberlakukan).
func absensiAllowedTempatTugas(item models.PengaturanAbsensi) []string {
	if strings.TrimSpace(item.TempatTugasAllowed) == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(item.TempatTugasAllowed), &out)
	return out
}

func absensiAllowedJabatanIDs(item models.PengaturanAbsensi) []uint {
	if strings.TrimSpace(item.JabatanAllowedIDs) == "" {
		return nil
	}
	var out []uint
	_ = json.Unmarshal([]byte(item.JabatanAllowedIDs), &out)
	return out
}

// absensiEligible menentukan apakah seorang pegawai boleh memakai menu
// Absen berdasarkan filter tempat tugas & jabatan yang diatur administrator.
// Kalau KEDUA filter kosong (belum pernah diatur), menu Absen terbuka untuk
// semua pegawai -- filter baru berlaku begitu administrator mengisi salah
// satu/kedua daftarnya lewat halaman Rekap Absen.
func absensiEligible(setting models.PengaturanAbsensi, pegawai models.Pegawai) bool {
	if allowed := absensiAllowedTempatTugas(setting); len(allowed) > 0 {
		match := false
		for _, t := range allowed {
			if strings.EqualFold(strings.TrimSpace(t), strings.TrimSpace(pegawai.TempatTgs)) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if allowed := absensiAllowedJabatanIDs(setting); len(allowed) > 0 {
		if pegawai.IDJabatan == nil {
			return false
		}
		match := false
		for _, id := range allowed {
			if id == *pegawai.IDJabatan {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// pengaturanAbsensiOut adalah bentuk PengaturanAbsensi yang dikirim ke
// frontend: TempatTugasAllowed/JabatanAllowedIDs (kolom JSON text mentah,
// gorm json:"-") diparsing jadi array asli, dan Eligible dihitung khusus
// untuk pegawai/atasan yang login (true untuk administrator/admin/pegawai
// yang belum diketahui kelayakannya, supaya default aman dan endpoint ini
// tidak mengubah perilaku untuk role yang tidak relevan dengan filter ini).
type pengaturanAbsensiOut struct {
	ID                 uint     `json:"id"`
	Aktif              bool     `json:"aktif"`
	JamMulaiPagi       string   `json:"jam_mulai_pagi"`
	JamBatasPagi       string   `json:"jam_batas_pagi"`
	JamMulaiPulang     string   `json:"jam_mulai_pulang"`
	TempatTugasAllowed []string `json:"tempat_tugas_allowed"`
	JabatanAllowedIDs  []uint   `json:"jabatan_allowed_ids"`
	KantorLat          *float64 `json:"kantor_lat"`
	KantorLng          *float64 `json:"kantor_lng"`
	RadiusMeter        int      `json:"radius_meter"`
	Eligible           bool     `json:"eligible"`
}

func toPengaturanAbsensiOut(item models.PengaturanAbsensi) pengaturanAbsensiOut {
	tempat := absensiAllowedTempatTugas(item)
	if tempat == nil {
		tempat = []string{}
	}
	jabatan := absensiAllowedJabatanIDs(item)
	if jabatan == nil {
		jabatan = []uint{}
	}
	radius := item.RadiusMeter
	if radius <= 0 {
		radius = 20
	}
	return pengaturanAbsensiOut{
		ID:                 item.ID,
		Aktif:              item.Aktif,
		JamMulaiPagi:       item.JamMulaiPagi,
		JamBatasPagi:       item.JamBatasPagi,
		JamMulaiPulang:     item.JamMulaiPulang,
		TempatTugasAllowed: tempat,
		JabatanAllowedIDs:  jabatan,
		KantorLat:          item.KantorLat,
		KantorLng:          item.KantorLng,
		RadiusMeter:        radius,
		Eligible:           true,
	}
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

	// surat pendukung (BA/Surat Tugas/Surat Izin/SKS) -- pegawai hanya bisa
	// melihat/mengunduh, input & hapus khusus admin/administrator (lihat
	// handlers/absensi_dokumen.go).
	mux.Handle("GET /api/absensi/dokumen", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { listAbsensiDokumenSaya(w, r, db) }))
	mux.Handle("GET /api/absensi/dokumen/rekap", manage(func(w http.ResponseWriter, r *http.Request) { listAbsensiDokumenAdmin(w, r, db) }))
	mux.Handle("POST /api/absensi/dokumen/kolektif", manage(func(w http.ResponseWriter, r *http.Request) { inputAbsensiDokumenKolektif(w, r, db) }))
	mux.Handle("GET /api/absensi/dokumen/{id}/file", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadAbsensiDokumen(w, r, db) }))
	mux.Handle("DELETE /api/absensi/dokumen/{id}", manage(func(w http.ResponseWriter, r *http.Request) { deleteAbsensiDokumen(w, r, db) }))

	// admin/administrator saja
	mux.Handle("PUT /api/absensi/pengaturan", manage(func(w http.ResponseWriter, r *http.Request) { updatePengaturanAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/opsi-tempat-tugas", manage(func(w http.ResponseWriter, r *http.Request) { opsiTempatTugasAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap", manage(func(w http.ResponseWriter, r *http.Request) { rekapAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap/export", manage(func(w http.ResponseWriter, r *http.Request) { exportRekapAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap/pdf", manage(func(w http.ResponseWriter, r *http.Request) { exportRekapAbsensiPegawaiPDF(w, r, db) }))
}

func getPengaturanAbsensiHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	item, err := getPengaturanAbsensi(db)
	if err != nil {
		utils.Error(w, http.StatusNotFound, "pengaturan absensi belum tersedia")
		return
	}
	out := toPengaturanAbsensiOut(item)
	if claims, ok := middleware.GetClaims(r); ok && claims.IDPegawai != nil {
		var pegawai models.Pegawai
		if err := db.First(&pegawai, *claims.IDPegawai).Error; err == nil {
			out.Eligible = absensiEligible(item, pegawai)
		}
	}
	utils.Success(w, "ok", out)
}

// opsiTempatTugasAbsensi mengambil daftar nilai tempat_tgs unik yang benar-
// benar ada di data pegawai, dipakai administrator untuk memilih tempat
// tugas mana yang boleh memakai menu Absen (lihat pengaturanAbsensiPayload
// di absensi_admin.go) -- tempat_tgs adalah field teks bebas (bukan tabel
// referensi), jadi tidak ada daftar master untuk itu.
func opsiTempatTugasAbsensi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var values []string
	db.Model(&models.Pegawai{}).
		Where("tempat_tgs IS NOT NULL AND tempat_tgs <> ''").
		Distinct().Pluck("tempat_tgs", &values)
	sort.Strings(values)
	utils.Success(w, "ok", values)
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

	var pegawaiSelf models.Pegawai
	if err := db.First(&pegawaiSelf, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}
	if !absensiEligible(setting, pegawaiSelf) {
		utils.Error(w, http.StatusForbidden, "menu absen bukan untuk anda")
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

	if ok, pesan := absensiCekRadius(setting, lat, lng); !ok {
		utils.Error(w, http.StatusForbidden, pesan)
		return
	}

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

	var pegawaiSelf models.Pegawai
	if err := db.First(&pegawaiSelf, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}
	if !absensiEligible(setting, pegawaiSelf) {
		utils.Error(w, http.StatusForbidden, "menu absen bukan untuk anda")
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

	if ok, pesan := absensiCekRadius(setting, lat, lng); !ok {
		utils.Error(w, http.StatusForbidden, pesan)
		return
	}

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

// tanggalTercoverEntry menjelaskan satu tanggal hari kerja yang tertutup
// oleh dokumen (surat) yang diinput administrator (Surat Tugas/Berita
// Acara/Surat Izin/SKS) -- Kode/Label mengikuti models.AbsensiDokumenKode
// (DD = Dinas Dalam, I = Izin, S = Sakit).
type tanggalTercoverEntry struct {
	Tanggal string `json:"tanggal"`
	Jenis   string `json:"jenis"`
	Kode    string `json:"kode"`
	Label   string `json:"label"`
}

func tercoverEntryFromDokumen(d models.AbsensiDokumen) tanggalTercoverEntry {
	kode := models.AbsensiDokumenKode[d.Jenis]
	return tanggalTercoverEntry{
		Tanggal: d.Tanggal.Format("2006-01-02"),
		Jenis:   d.Jenis,
		Kode:    kode,
		Label:   models.AbsensiDokumenKodeLabel[kode],
	}
}

type riwayatAbsenResponse struct {
	Bulan           int                    `json:"bulan"`
	Tahun           int                    `json:"tahun"`
	Absensi         []models.Absensi       `json:"absensi"`
	TanggalTerlewat []string               `json:"tanggal_terlewat"`
	TanggalTercover []tanggalTercoverEntry `json:"tanggal_tercover"`
}

// riwayatAbsenSaya mengembalikan riwayat absen pegawai yang login untuk satu
// bulan (default bulan & tahun berjalan, WITA), plus dua daftar: tanggal_
// terlewat (hari kerja -- mengikuti pola 5/6 hari sesuai tempat tugas, sama
// seperti pengajuan cuti -- yang TIDAK punya baris Absensi dan TIDAK punya
// AbsensiDokumen sama sekali, sehingga pegawai hanya perlu diberi peringatan
// karena surat pendukungnya sekarang diinput administrator, bukan diupload
// sendiri -- lihat handlers/absensi_dokumen.go) dan tanggal_tercover (hari
// kerja yang sudah ada AbsensiDokumen-nya, ditampilkan dengan kode DD/I/S).
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
	db.Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", *claims.IDPegawai, start, limit).
		Order("tanggal desc").Find(&dokumen)
	tercoverSet := map[string]bool{}
	tercover := []tanggalTercoverEntry{}
	for _, d := range dokumen {
		tercoverSet[d.Tanggal.Format("2006-01-02")] = true
		tercover = append(tercover, tercoverEntryFromDokumen(d))
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
		TanggalTercover: tercover,
	})
}

// resizeJPEG mengecilkan gambar JPEG ke lebar targetW (tinggi mengikuti
// rasio aslinya) dengan merata-ratakan blok piksel sumber. Sengaja hanya
// memakai stdlib (image/jpeg) tanpa dependensi tambahan -- dipakai untuk
// thumbnail foto absen di tabel riwayat/detail yang menampilkan banyak foto
// sekaligus. Kalau gambar sudah lebih kecil dari targetW, data aslinya
// dikembalikan apa adanya.
func resizeJPEG(data []byte, targetW int) ([]byte, error) {
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if src.Bounds().Dx() <= targetW {
		return data, nil
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resizeImageBox(src, targetW), &jpeg.Options{Quality: 75}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fotoThumbPNG mengubah foto absen (JPEG di database) jadi PNG kecil --
// dipakai saat menyusun PDF rekap absen per pegawai, karena penulis PDF
// bawaan aplikasi (utils/pdfwriter.go) hanya menerima gambar PNG.
func fotoThumbPNG(data []byte, targetW int) ([]byte, error) {
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, resizeImageBox(src, targetW)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// resizeImageBox mengecilkan gambar ke lebar targetW dengan merata-ratakan
// blok piksel sumber (tinggi mengikuti rasio aslinya). Gambar yang sudah
// lebih kecil dari targetW dikembalikan apa adanya (disalin ke RGBA).
func resizeImageBox(src image.Image, targetW int) *image.RGBA {
	b := src.Bounds()
	if b.Dx() < targetW {
		targetW = b.Dx()
	}
	if targetW < 1 {
		targetW = 1
	}
	targetH := b.Dy() * targetW / b.Dx()
	if targetH < 1 {
		targetH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		y0 := b.Min.Y + y*b.Dy()/targetH
		y1 := b.Min.Y + (y+1)*b.Dy()/targetH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < targetW; x++ {
			x0 := b.Min.X + x*b.Dx()/targetW
			x1 := b.Min.X + (x+1)*b.Dx()/targetW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sumR, sumG, sumB, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					sumR += uint64(cr >> 8)
					sumG += uint64(cg >> 8)
					sumB += uint64(cb >> 8)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.Set(x, y, color.RGBA{R: uint8(sumR / n), G: uint8(sumG / n), B: uint8(sumB / n), A: 255})
		}
	}
	return dst
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

	// ?w=96 mengecilkan foto jadi thumbnail -- dipakai tabel riwayat/detail
	// absen yang menampilkan banyak foto sekaligus supaya tidak menarik
	// puluhan foto ukuran penuh (apalagi di HP). Tanpa parameter ini foto
	// dikirim apa adanya (dipakai dialog "lihat foto" ukuran besar).
	if wq := r.URL.Query().Get("w"); wq != "" {
		if targetW, err := strconv.Atoi(wq); err == nil && targetW >= 16 && targetW <= 1000 {
			if kecil, err := resizeJPEG(foto, targetW); err == nil {
				foto = kecil
			}
		}
	}
	// foto absen tidak pernah berubah setelah tersimpan, jadi aman di-cache
	// browser (private -- hanya pemilik/admin yang boleh melihatnya).
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"absen_%s_%s.jpg\"", id, jenis))
	w.Write(foto)
}
