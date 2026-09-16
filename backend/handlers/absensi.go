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

// mapsURL membuat tautan Google Maps ke satu titik koordinat. Format
// "?api=1&query=lat,lng" adalah format resmi Google Maps yang langsung
// membuka penanda pada titik tersebut, baik di browser maupun di aplikasi
// Google Maps pada HP -- dipakai pada riwayat/rekap absen, export Excel, dan
// PDF supaya titik koordinat tidak perlu disalin-tempel manual.
func mapsURL(lat, lng float64) string {
	return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", lat, lng)
}

// koordinatTerakhir mengambil titik koordinat dari absen TERAKHIR pada satu
// baris absensi: koordinat absen pulang bila sudah ada, kalau belum memakai
// koordinat absen masuk.
func koordinatTerakhir(a models.Absensi) (lat, lng float64, ok bool) {
	if a.LatPulang != nil && a.LngPulang != nil {
		return *a.LatPulang, *a.LngPulang, true
	}
	if a.LatMasuk != nil && a.LngMasuk != nil {
		return *a.LatMasuk, *a.LngMasuk, true
	}
	return 0, 0, false
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

// jamAbsenSet adalah satu set jendela waktu absen (mulai pagi s.d tutup
// pulang) -- lihat jamAbsenUntukPegawai.
type jamAbsenSet struct {
	MulaiPagi   string
	BatasPagi   string
	TutupPagi   string
	MulaiPulang string
	TutupPulang string
}

// jamAbsenUntukPegawai memilih set jendela waktu absen yang berlaku untuk
// seorang pegawai, dengan urutan prioritas: (1) jam kerja KHUSUS unit kerja/
// sekolah pegawai (UnitKerja.JamMulaiPagi dkk, hanya dipakai kalau KELIMA
// field itu diisi lengkap -- lihat komentar pada models.UnitKerja), lalu
// (2) fallback ke set Sekolah "global" di PengaturanAbsensi kalau pegawai ini
// berstatus sekolah (lihat isSekolahPegawai di handlers/pengajuan_cuti.go --
// PRIORITAS UTAMA-nya kategori Tempat Kerja pada Unit Kerja pegawai, fallback
// ke tebakan dari kata "sekolah" pada Tempat Tugas untuk data lama), atau
// (3) set Dinas/Kantor "global" untuk pegawai lainnya. Dengan ini tiap
// sekolah bisa mengatur jam masuk/terlambat/pulang/tutup sendiri-sendiri
// lewat menu Master Data -> Unit Kerja, sekaligus tetap kompatibel dengan
// unit kerja yang belum diberi jam khusus (jatuh kembali ke default
// sekolah/dinas seperti sebelumnya). Dipakai di absenMasuk/absenPulang
// (untuk validasi) dan getPengaturanAbsensiHandler (untuk ditampilkan ke
// pegawai yang bersangkutan lewat AbsensiView.vue).
func jamAbsenUntukPegawai(setting models.PengaturanAbsensi, pegawai models.Pegawai) jamAbsenSet {
	if uk := pegawai.UnitKerja; uk != nil &&
		uk.JamMulaiPagi != nil && *uk.JamMulaiPagi != "" &&
		uk.JamBatasPagi != nil && *uk.JamBatasPagi != "" &&
		uk.JamTutupPagi != nil && *uk.JamTutupPagi != "" &&
		uk.JamMulaiPulang != nil && *uk.JamMulaiPulang != "" &&
		uk.JamTutupPulang != nil && *uk.JamTutupPulang != "" {
		return jamAbsenSet{
			MulaiPagi:   *uk.JamMulaiPagi,
			BatasPagi:   *uk.JamBatasPagi,
			TutupPagi:   *uk.JamTutupPagi,
			MulaiPulang: *uk.JamMulaiPulang,
			TutupPulang: *uk.JamTutupPulang,
		}
	}
	if isSekolahPegawai(pegawai) {
		return jamAbsenSet{
			MulaiPagi:   setting.JamMulaiPagiSekolah,
			BatasPagi:   setting.JamBatasPagiSekolah,
			TutupPagi:   setting.JamTutupPagiSekolah,
			MulaiPulang: setting.JamMulaiPulangSekolah,
			TutupPulang: setting.JamTutupPulangSekolah,
		}
	}
	return jamAbsenSet{
		MulaiPagi:   setting.JamMulaiPagi,
		BatasPagi:   setting.JamBatasPagi,
		TutupPagi:   setting.JamTutupPagi,
		MulaiPulang: setting.JamMulaiPulang,
		TutupPulang: setting.JamTutupPulang,
	}
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

// toleransiAkurasiMaksimal membatasi seberapa besar toleransi jarak yang
// diberikan karena ketidakpastian GPS (lihat absensiCekRadius) -- tanpa
// batas ini, pegawai dengan lokasi berbasis menara seluler (akurasi bisa
// ribuan meter) bisa lolos geofence dari lokasi manapun.
//
// Nilai ini SENGAJA diturunkan dari 100m ke 50m. Sebelumnya, administrator
// yang mengatur radius kecil (mis. 50m, sesuai luas gedung kantor
// sebenarnya) tetap mendapati toleransi ini menambah hingga 100m ekstra --
// sehingga absen tetap diterima sampai +-150m dari titik kantor. Ini memaksa
// administrator memperbesar radius (mis. jadi 350m) supaya pegawai yang
// sungguh di kantor tidak tertolak, padahal efeknya, digabung toleransi,
// absen jadi bisa dilakukan dari ratusan meter (bisa sampai ke rumah
// terdekat) -- toleransi seharusnya HANYA menutupi noise GPS, bukan dipakai
// mengompensasi radius yang sengaja diperbesar. Sejak pengambilan GPS di
// frontend memakai beberapa sampel dan mengambil accuracy terbaik (lihat
// getLocationOnce di AbsensiView.vue), pembacaan yang sampai ke sini
// biasanya sudah cukup presisi sehingga toleransi maksimal tidak perlu
// sebesar dulu.
const toleransiAkurasiMaksimal = 50.0

// toleransiAkurasiMinimum adalah batas BAWAH toleransi -- dipakai walau
// accuracy yang dilaporkan perangkat sangat kecil (mis. 13m) atau tidak
// dilaporkan sama sekali. Nilai accuracy dari Geolocation API browser sering
// terlalu percaya diri terutama DI DALAM GEDUNG (sinyal GPS memantul di
// dinding/lantai beton -- dikenal sebagai multipath), sehingga posisi
// sebenarnya bisa meleset 50-150m dari titik yang dilaporkan walau accuracy
// tertulis hanya belasan meter. Tanpa batas bawah ini, pegawai yang memang
// berada di kantor bisa berulang kali ditolak absen hanya karena GPS
// ponselnya melaporkan accuracy yang (secara keliru) sangat kecil.
const toleransiAkurasiMinimum = 30.0

// absensiCekRadius memvalidasi titik koordinat (lat,lng) hasil GPS pegawai
// terhadap titik koordinat acuan absen. Kalau unitKerja (unit kerja/sekolah
// pegawai yang bersangkutan) sudah diberi titik koordinat sendiri (lihat
// UnitKerja.Lat/Lng/RadiusMeter), titik itulah yang jadi PRIORITAS UTAMA --
// ini yang memungkinkan absen dilakukan di banyak sekolah/kecamatan sekaligus
// dengan titik masing-masing, bukan cuma satu titik kantor tunggal. Kalau
// unitKerja nil atau belum diberi titik koordinat, jatuh kembali (fallback)
// ke titik kantor tunggal setting.KantorLat/KantorLng/RadiusMeter (kompatibel
// dengan instansi yang hanya punya satu titik kantor). Kalau KEDUANYA belum
// diatur, geofence dianggap belum aktif dan absen tetap diperbolehkan tanpa
// validasi jarak (default terbuka, sama seperti filter tempat tugas/jabatan/
// kecamatan). Kalau geofence aktif tapi lat/lng pegawai tidak terdeteksi (GPS
// ditolak/gagal), absen ditolak karena jarak tidak bisa dipastikan.
//
// akurasi adalah nilai accuracy (meter) dari Geolocation API browser --
// radius kemungkinan posisi ASLI pegawai di sekitar titik (lat,lng) yang
// dilaporkan. GPS ponsel, terutama di dalam gedung, sering meleset 50-150m
// meski pegawai tidak bergerak sama sekali. Supaya pegawai yang benar-benar
// berada di kantor/sekolah tidak ditolak berulang kali hanya karena noise
// GPS, jarak yang dibandingkan dengan radius dikurangi toleransi sebesar
// akurasi tersebut, dengan batas bawah toleransiAkurasiMinimum meter (lihat
// komentarnya) dan batas atas toleransiAkurasiMaksimal meter supaya geofence
// tetap berarti untuk lokasi yang jelas-jelas jauh dari titik acuan).
//
// Fallback ke titik kantor tunggal (setting.KantorLat/KantorLng) HANYA
// berlaku untuk unit kerja berkategori Dinas/Kantor (UnitKerja.TempatKerja ==
// "dinas") atau yang belum dikategorikan sama sekali (kompatibel dengan data
// lama) -- inilah yang membuat unit kerja Dinas otomatis memakai titik
// kantor pusat yang sudah ditetapkan di Pengaturan Absen, tanpa perlu admin
// mengisi titik koordinat satu-satu untuk setiap unit dinas. Unit kerja
// berkategori Sekolah TIDAK ikut fallback ini -- kalau titik koordinat
// sekolah itu sendiri belum diisi, geofence untuk pegawai di sekolah tsb
// dianggap belum aktif (absen tetap diperbolehkan tanpa validasi jarak,
// bukan malah memvalidasi terhadap titik kantor dinas yang jelas salah
// lokasi) sampai admin mengisi titik koordinat sekolah itu di menu Unit
// Kerja.
func absensiCekRadius(setting models.PengaturanAbsensi, unitKerja *models.UnitKerja, lat, lng, akurasi *float64) (ok bool, pesan string) {
	var targetLat, targetLng *float64
	radius := setting.RadiusMeter
	sumberTitik := "kantor"
	switch {
	case unitKerja != nil && unitKerja.Lat != nil && unitKerja.Lng != nil:
		targetLat, targetLng = unitKerja.Lat, unitKerja.Lng
		if unitKerja.RadiusMeter != nil && *unitKerja.RadiusMeter > 0 {
			radius = *unitKerja.RadiusMeter
		}
		sumberTitik = "unit kerja " + unitKerja.Unit
	case unitKerja == nil || unitKerja.TempatKerja == nil || *unitKerja.TempatKerja != models.TempatKerjaSekolah:
		targetLat, targetLng = setting.KantorLat, setting.KantorLng
	}
	if targetLat == nil || targetLng == nil {
		return true, ""
	}
	if radius <= 0 {
		radius = 20
	}
	if lat == nil || lng == nil {
		return false, "lokasi GPS tidak terdeteksi. Aktifkan layanan lokasi pada perangkat/browser Anda dan izinkan akses lokasi, lalu coba lagi."
	}
	jarak := distanceMeters(*targetLat, *targetLng, *lat, *lng)

	// toleransi minimal toleransiAkurasiMinimum berlaku SELALU (lihat
	// komentarnya) -- kalau accuracy yang dilaporkan lebih besar dari itu,
	// pakai accuracy tersebut (dibatasi maksimal toleransiAkurasiMaksimal).
	toleransi := toleransiAkurasiMinimum
	if akurasi != nil && *akurasi > toleransi {
		toleransi = math.Min(*akurasi, toleransiAkurasiMaksimal)
	}
	jarakEfektif := math.Max(jarak-toleransi, 0)

	if jarakEfektif > float64(radius) {
		infoAkurasi := ""
		if akurasi != nil && *akurasi > 0 {
			infoAkurasi = fmt.Sprintf(" (akurasi GPS perangkat Anda saat ini sekitar %.0f meter)", *akurasi)
		}
		return false, fmt.Sprintf("Anda berada di luar radius %s (jarak sekitar %.0f meter, maksimal %d meter dari titik acuan)%s. Absen tidak dapat dilakukan dari lokasi ini.", sumberTitik, jarak, radius, infoAkurasi)
	}
	return true, ""
}

// absensiAllowedTempatTugas/absensiAllowedJabatanIDs/absensiAllowedKecamatanIDs
// mem-parse kolom JSON text PengaturanAbsensi.TempatTugasAllowed/
// JabatanAllowedIDs/KecamatanAllowedIDs -- lihat komentar pada model untuk
// format & artinya (daftar kosong = filter itu tidak diberlakukan).
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

func absensiAllowedKecamatanIDs(item models.PengaturanAbsensi) []uint {
	if strings.TrimSpace(item.KecamatanAllowedIDs) == "" {
		return nil
	}
	var out []uint
	_ = json.Unmarshal([]byte(item.KecamatanAllowedIDs), &out)
	return out
}

// absensiEligible menentukan apakah seorang pegawai boleh memakai menu
// Absen berdasarkan filter tempat tugas, jabatan, & kecamatan (unit kerja/
// sekolah pegawai) yang diatur administrator. Kalau SEMUA filter kosong
// (belum pernah diatur), menu Absen terbuka untuk semua pegawai -- filter
// baru berlaku begitu administrator mengisi salah satu/lebih daftarnya lewat
// halaman Rekap Absen. pegawai HARUS sudah di-preload dengan UnitKerja kalau
// ingin filter kecamatan/tempat tugas berfungsi (lihat pemanggil di
// absenMasuk/absenPulang/getPengaturanAbsensiHandler) -- kalau tidak,
// pegawai.UnitKerja nil dan pegawai otomatis tidak lolos begitu filter itu
// diisi (fallback ke tebakan teks Tempat Tugas untuk filter tempat tugas,
// lihat isSekolahPegawai).
//
// Filter tempat tugas dicocokkan lewat isSekolahPegawai (kategori Dinas/
// Sekolah yang sama dipakai jam kerja absen, 5/6 hari kerja, & syarat
// dokumen cuti), BUKAN lagi teks Pegawai.TempatTgs mentah -- sebelumnya
// pegawai yang Unit Kerja/sekolahnya sudah benar dikategorikan & dikonfigurasi
// lengkap tetap bisa gagal lolos filter ini kalau teks Tempat Tugas
// mentahnya tidak persis sama dengan yang dicentang administrator (lihat
// opsiTempatTugasAbsensi di atas).
func absensiEligible(setting models.PengaturanAbsensi, pegawai models.Pegawai) bool {
	if allowed := absensiAllowedTempatTugas(setting); len(allowed) > 0 {
		kategori := models.TempatKerjaDinas
		if isSekolahPegawai(pegawai) {
			kategori = models.TempatKerjaSekolah
		}
		match := false
		for _, t := range allowed {
			if strings.EqualFold(strings.TrimSpace(t), kategori) {
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
	if allowed := absensiAllowedKecamatanIDs(setting); len(allowed) > 0 {
		if pegawai.UnitKerja == nil || pegawai.UnitKerja.IDKecamatan == nil {
			return false
		}
		match := false
		for _, id := range allowed {
			if id == *pegawai.UnitKerja.IDKecamatan {
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
//
// JamMulaiPagi..JamTutupPulang (tanpa akhiran "Sekolah") SELALU berisi set
// Dinas/Kantor mentah dari database KECUALI kalau getPengaturanAbsensiHandler
// menimpanya dengan set Sekolah untuk pegawai/atasan yang tempat tugasnya
// sekolah (lihat jamAbsenUntukPegawai) -- ini yang dipakai AbsensiView.vue
// (pegawai) sehingga TIDAK perlu tahu ada dua set sama sekali, cukup pakai
// field yang sudah "resolved" untuk dirinya. Field ...Sekolah selalu berisi
// set Sekolah APA ADANYA (tidak pernah ditimpa) -- ini yang dipakai form
// Pengaturan Absen administrator untuk menampilkan & mengubah KEDUA set
// sekaligus.
type pengaturanAbsensiOut struct {
	ID                    uint     `json:"id"`
	Aktif                 bool     `json:"aktif"`
	JamMulaiPagi          string   `json:"jam_mulai_pagi"`
	JamBatasPagi          string   `json:"jam_batas_pagi"`
	JamTutupPagi          string   `json:"jam_tutup_pagi"`
	JamMulaiPulang        string   `json:"jam_mulai_pulang"`
	JamTutupPulang        string   `json:"jam_tutup_pulang"`
	JamMulaiPagiSekolah   string   `json:"jam_mulai_pagi_sekolah"`
	JamBatasPagiSekolah   string   `json:"jam_batas_pagi_sekolah"`
	JamTutupPagiSekolah   string   `json:"jam_tutup_pagi_sekolah"`
	JamMulaiPulangSekolah string   `json:"jam_mulai_pulang_sekolah"`
	JamTutupPulangSekolah string   `json:"jam_tutup_pulang_sekolah"`
	TempatTugasAllowed    []string `json:"tempat_tugas_allowed"`
	JabatanAllowedIDs     []uint   `json:"jabatan_allowed_ids"`
	KecamatanAllowedIDs   []uint   `json:"kecamatan_allowed_ids"`
	KantorLat             *float64 `json:"kantor_lat"`
	KantorLng             *float64 `json:"kantor_lng"`
	RadiusMeter           int      `json:"radius_meter"`
	Eligible              bool     `json:"eligible"`
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
	kecamatan := absensiAllowedKecamatanIDs(item)
	if kecamatan == nil {
		kecamatan = []uint{}
	}
	radius := item.RadiusMeter
	if radius <= 0 {
		radius = 20
	}
	return pengaturanAbsensiOut{
		ID:                    item.ID,
		Aktif:                 item.Aktif,
		JamMulaiPagi:          item.JamMulaiPagi,
		JamBatasPagi:          item.JamBatasPagi,
		JamTutupPagi:          item.JamTutupPagi,
		JamMulaiPulang:        item.JamMulaiPulang,
		JamTutupPulang:        item.JamTutupPulang,
		JamMulaiPagiSekolah:   item.JamMulaiPagiSekolah,
		JamBatasPagiSekolah:   item.JamBatasPagiSekolah,
		JamTutupPagiSekolah:   item.JamTutupPagiSekolah,
		JamMulaiPulangSekolah: item.JamMulaiPulangSekolah,
		JamTutupPulangSekolah: item.JamTutupPulangSekolah,
		TempatTugasAllowed:    tempat,
		JabatanAllowedIDs:     jabatan,
		KecamatanAllowedIDs:   kecamatan,
		KantorLat:             item.KantorLat,
		KantorLng:             item.KantorLng,
		RadiusMeter:           radius,
		Eligible:              true,
	}
}

func RegisterAbsensiRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireRole(roles...))
	}
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai", "atasan") }
	// manage: administrator & admin (kepegawaian penuh) DAN akun mana pun
	// (biasanya pegawai/atasan) yang ditandai IsAdminAbsensi = true -- lihat
	// models.User.IsAdminAbsensi. Akun dengan tanda ini TIDAK berubah role-
	// nya, jadi tetap bisa absen & mengajukan cuti sendiri lewat akun yang
	// sama, hanya ditambah akses ke menu "Input Rekapan Absensi" (rekap
	// kehadiran SELURUH pegawai & input surat kolektif: berita acara, surat
	// tugas, SKS, dll) -- tidak ada akses ke modul lain (Pengajuan Cuti
	// milik pegawai lain, Data Pegawai, dst).
	manage := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := middleware.GetClaims(r)
			if !ok {
				utils.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if claims.RoleName != "administrator" && claims.RoleName != "admin" && !claims.IsAdminAbsensi {
				utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses untuk aksi ini")
				return
			}
			h(w, r)
		}, middleware.Auth, middleware.RequireActiveUser(db))
	}
	// administratorOnly: khusus untuk mengubah Pengaturan Absen (jendela
	// waktu, filter siapa yang boleh absen, titik koordinat kantor) --
	// role admin dan akun IsAdminAbsensi TIDAK boleh melihat maupun
	// mengubah pengaturan ini, berbeda dari fitur rekap/dokumen lain yang
	// tetap boleh diakses admin & administrator (manage di atas).
	administratorOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator") }
	// pdfOnly: unduh PDF rekap absen (satu pegawai) khusus administrator &
	// admin (Admin Kepegawaian) -- SENGAJA tidak memakai manage() di atas,
	// jadi akun yang HANYA ditandai IsAdminAbsensi (tanpa role administrator
	// atau admin) tidak bisa mengunduh PDF ini walau tetap bisa melihat
	// rekap & export Excel (lihat frontend/src/views/RekapAbsensiView.vue).
	pdfOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

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

	// Pengaturan Absen: administrator SAJA (lihat administratorOnly di atas)
	mux.Handle("PUT /api/absensi/pengaturan", administratorOnly(func(w http.ResponseWriter, r *http.Request) { updatePengaturanAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/opsi-tempat-tugas", administratorOnly(func(w http.ResponseWriter, r *http.Request) { opsiTempatTugasAbsensi(w, r, db) }))

	// admin/administrator saja
	mux.Handle("GET /api/absensi/rekap", manage(func(w http.ResponseWriter, r *http.Request) { rekapAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap/export", manage(func(w http.ResponseWriter, r *http.Request) { exportRekapAbsensi(w, r, db) }))
	mux.Handle("GET /api/absensi/rekap/pdf", pdfOnly(func(w http.ResponseWriter, r *http.Request) { exportRekapAbsensiPegawaiPDF(w, r, db) }))
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
		if err := db.Preload("UnitKerja").First(&pegawai, *claims.IDPegawai).Error; err == nil {
			out.Eligible = absensiEligible(item, pegawai)
			// Kalau pegawai yang bersangkutan bertugas di sekolah, timpa field
			// jam_* (tanpa akhiran "_sekolah") dengan jam kerja sekolah supaya
			// AbsensiView.vue (yang hanya membaca field jam_* biasa) otomatis
			// menampilkan & memvalidasi jam kerja yang sesuai untuknya, tanpa
			// perlu tahu soal pembagian dinas/sekolah sama sekali -- lihat
			// jamAbsenUntukPegawai dan komentar pada models.PengaturanAbsensi.
			jam := jamAbsenUntukPegawai(item, pegawai)
			out.JamMulaiPagi = jam.MulaiPagi
			out.JamBatasPagi = jam.BatasPagi
			out.JamTutupPagi = jam.TutupPagi
			out.JamMulaiPulang = jam.MulaiPulang
			out.JamTutupPulang = jam.TutupPulang
		}
	}
	utils.Success(w, "ok", out)
}

// opsiTempatTugasAbsensi mengembalikan 2 pilihan TETAP (Dinas/Kantor,
// Sekolah) untuk filter "Tempat Tugas yang Boleh Absen" di Pengaturan Absen.
//
// Sebelumnya endpoint ini mengambil daftar nilai Pegawai.TempatTgs unik APA
// ADANYA dari data pegawai (teks bebas, mis. "Kantor Cabang Utara", nama
// sekolah masing-masing, dst.) -- administrator harus mencocokkan teks bebas
// itu satu-satu, dan pegawai yang teks Tempat Tugas mentahnya tidak persis
// sama dengan pilihan yang dicentang jadi TIDAK LOLOS filter ini walau Unit
// Kerja-nya sendiri sudah benar dikategorikan & dikonfigurasi lengkap (mis.
// pegawai di sekolah yang tempat_tgs mentahnya nama sekolah spesifik, bukan
// literal "Sekolah") -- ini yang membuat menu Absen tertutup untuk pegawai
// yang sekolahnya sudah diatur admin dengan benar. Sekarang filter ini
// dicocokkan lewat isSekolahPegawai (kategori Dinas/Sekolah yang sama dipakai
// jam kerja absen, 5/6 hari kerja, & syarat dokumen cuti -- lihat
// absensiEligible di bawah), bukan lagi teks Tempat Tugas mentah.
func opsiTempatTugasAbsensi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	utils.Success(w, "ok", []map[string]string{
		{"value": models.TempatKerjaDinas, "label": "Dinas/Kantor"},
		{"value": models.TempatKerjaSekolah, "label": "Sekolah"},
	})
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
	if err := db.Preload("UnitKerja").First(&pegawaiSelf, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}
	if !absensiEligible(setting, pegawaiSelf) {
		utils.Error(w, http.StatusForbidden, "menu absen bukan untuk anda")
		return
	}
	// jam kerja yang berlaku untuk pegawai ini (dinas/kantor vs sekolah) --
	// lihat jamAbsenUntukPegawai.
	jam := jamAbsenUntukPegawai(setting, pegawaiSelf)

	now := absensiNow()
	today := absensiToday()
	nowMin := now.Hour()*60 + now.Minute()

	if mulaiMin, ok := parseJamToMinutes(jam.MulaiPagi); ok && nowMin < mulaiMin {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf("belum waktunya absen masuk, dibuka mulai jam %s", jam.MulaiPagi))
		return
	}

	var existing models.Absensi
	found := db.Where("id_pegawai = ? AND tanggal = ?", *claims.IDPegawai, today).First(&existing).Error == nil
	if found && existing.JamMasuk != nil {
		utils.Error(w, http.StatusBadRequest, "anda sudah absen masuk hari ini")
		return
	}

	// batas waktu keras: lewat JamTutupPagi, absen masuk otomatis DITUTUP
	// untuk hari itu (berbeda dari JamBatasPagi yang hanya menandai
	// terlambat tapi absen masuk tetap diterima -- lihat komentar pada
	// models.PengaturanAbsensi). Karena absen pulang mensyaratkan sudah ada
	// absen masuk (lihat absenPulang), menutup absen masuk otomatis juga
	// menutup absen pulang untuk hari itu.
	if tutupMin, ok := parseJamToMinutes(jam.TutupPagi); ok && nowMin > tutupMin {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf("batas waktu absen masuk sudah lewat (ditutup otomatis mulai jam %s), absen masuk untuk hari ini tidak lagi tersedia", jam.TutupPagi))
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
	akurasi := parseFloatForm(r, "accuracy")
	// dinas_dalam: pegawai mencentang tombol "Dinas Dalam" pada dialog kamera
	// (lihat AbsensiView.vue) -- kalau true, validasi radius kantor DILEWATI
	// (boleh absen dari mana saja) dan hari itu akan tercatat sebagai "Dinas
	// Dalam", bukan "Hadir", di rekap/riwayat/export (lihat models.Absensi.
	// IsDinasDalam & pemakaiannya di absensi_admin.go/absensi_pdf.go).
	dinasDalam := r.FormValue("dinas_dalam") == "true"

	if !dinasDalam {
		if ok, pesan := absensiCekRadius(setting, pegawaiSelf.UnitKerja, lat, lng, akurasi); !ok {
			utils.Error(w, http.StatusForbidden, pesan)
			return
		}
	}

	terlambat := 0
	if batasMin, ok := parseJamToMinutes(jam.BatasPagi); ok && nowMin > batasMin {
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
		existing.DinasDalamMasuk = dinasDalam
		if err := db.Save(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen masuk: "+err.Error())
			return
		}
	} else {
		existing = models.Absensi{
			IDPegawai:       *claims.IDPegawai,
			Tanggal:         today,
			JamMasuk:        &jamMasuk,
			TerlambatMenit:  terlambat,
			FotoMasuk:       fotoBytes,
			LatMasuk:        lat,
			LngMasuk:        lng,
			KedipanMasukOk:  kedipanOk,
			DinasDalamMasuk: dinasDalam,
		}
		if err := db.Create(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen masuk: "+err.Error())
			return
		}
	}

	msg := "absen masuk berhasil dicatat"
	if dinasDalam {
		msg += " (Dinas Dalam)"
	}
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
// absenMasuk. Absen pulang HANYA tersedia kalau pegawai sudah absen masuk
// pada hari yang sama (lihat pengecekan found/JamMasuk di bawah) -- kalau
// belum absen masuk sama sekali, absen pulang ditolak supaya tidak ada
// baris absen yang cuma berisi jam pulang tanpa jam masuk sama sekali.
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
	if err := db.Preload("UnitKerja").First(&pegawaiSelf, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}
	if !absensiEligible(setting, pegawaiSelf) {
		utils.Error(w, http.StatusForbidden, "menu absen bukan untuk anda")
		return
	}
	// jam kerja yang berlaku untuk pegawai ini (dinas/kantor vs sekolah) --
	// lihat jamAbsenUntukPegawai.
	jam := jamAbsenUntukPegawai(setting, pegawaiSelf)

	now := absensiNow()
	today := absensiToday()
	nowMin := now.Hour()*60 + now.Minute()

	if mulaiMin, ok := parseJamToMinutes(jam.MulaiPulang); ok && nowMin < mulaiMin {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf("belum waktunya absen pulang, dibuka mulai jam %s", jam.MulaiPulang))
		return
	}

	var existing models.Absensi
	found := db.Where("id_pegawai = ? AND tanggal = ?", *claims.IDPegawai, today).First(&existing).Error == nil
	if found && existing.JamPulang != nil {
		utils.Error(w, http.StatusBadRequest, "anda sudah absen pulang hari ini")
		return
	}
	if !found || existing.JamMasuk == nil {
		utils.Error(w, http.StatusBadRequest, "anda belum absen masuk hari ini -- absen pulang hanya tersedia setelah absen masuk berhasil dicatat")
		return
	}

	// batas waktu keras: lewat JamTutupPulang, absen pulang otomatis DITUTUP
	// untuk hari itu -- walaupun pegawai sudah absen masuk dan belum sempat
	// absen pulang (lihat komentar pada models.PengaturanAbsensi).
	if tutupMin, ok := parseJamToMinutes(jam.TutupPulang); ok && nowMin > tutupMin {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf("batas waktu absen pulang sudah lewat (ditutup otomatis mulai jam %s), absen pulang untuk hari ini tidak lagi tersedia", jam.TutupPulang))
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
	akurasi := parseFloatForm(r, "accuracy")
	// dinas_dalam: lihat komentar yang sama pada absenMasuk.
	dinasDalam := r.FormValue("dinas_dalam") == "true"

	if !dinasDalam {
		if ok, pesan := absensiCekRadius(setting, pegawaiSelf.UnitKerja, lat, lng, akurasi); !ok {
			utils.Error(w, http.StatusForbidden, pesan)
			return
		}
	}

	jamPulang := now

	// existing dijamin sudah ada (found == true) berkat pengecekan di atas --
	// absen pulang tidak lagi bisa membuat baris absen baru tanpa jam masuk.
	existing.JamPulang = &jamPulang
	existing.FotoPulang = fotoBytes
	existing.LatPulang = lat
	existing.LngPulang = lng
	existing.KedipanPulangOk = kedipanOk
	existing.DinasDalamPulang = dinasDalam
	if err := db.Save(&existing).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen pulang: "+err.Error())
		return
	}

	msg := "absen pulang berhasil dicatat"
	if dinasDalam {
		msg += " (Dinas Dalam)"
	}
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

func tercoverEntryFromDokumen(d models.AbsensiDokumen, jenisLookup map[string]models.JenisSurat) tanggalTercoverEntry {
	kode := kodeUntukJenis(jenisLookup, d.Jenis)
	return tanggalTercoverEntry{
		Tanggal: d.Tanggal.Format("2006-01-02"),
		Jenis:   d.Jenis,
		Kode:    kode,
		Label:   labelUntukJenis(jenisLookup, d.Jenis, kode),
	}
}

type riwayatAbsenResponse struct {
	Bulan           int                    `json:"bulan"`
	Tahun           int                    `json:"tahun"`
	Absensi         []models.Absensi       `json:"absensi"`
	TanggalTerlewat []string               `json:"tanggal_terlewat"`
	TanggalTercover []tanggalTercoverEntry `json:"tanggal_tercover"`
	// IsSekolah: hasil isSekolahPegawai (sudah memperhitungkan UnitKerja.
	// TempatKerja sebagai prioritas utama, bukan cuma tebakan teks Tempat
	// Tugas) -- dikirim di sini karena riwayatAbsenSaya sudah memuat pegawai
	// + UnitKerja-nya, supaya frontend (AbsensiView.vue) bisa menampilkan
	// kartu "Ajukan Surat Kolektif" HANYA untuk pegawai sekolah tanpa perlu
	// menduga-duga sendiri dari data yang mungkin belum lengkap di sisi
	// klien (lihat auth store yang cuma menyimpan hasil login, belum tentu
	// memuat UnitKerja pegawai).
	IsSekolah bool `json:"is_sekolah"`
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
	if err := db.Preload("UnitKerja").First(&pegawai, *claims.IDPegawai).Error; err != nil {
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
	jenisLookup := jenisSuratLookup(db)
	tercoverSet := map[string]bool{}
	tercover := []tanggalTercoverEntry{}
	for _, d := range dokumen {
		key := d.Tanggal.Format("2006-01-02")
		// kalau tanggal itu sudah punya absen masuk sungguhan, jangan
		// ditampilkan lagi di daftar "Bersurat" -- Hadir lebih diutamakan
		// (lihat juga inputAbsensiDokumenKolektif yang sejak sekarang tidak
		// lagi mengizinkan surat diinput untuk tanggal yang sudah ada absen
		// masuknya, tapi baris lama yang sudah kepencet dobel sebelum
		// perbaikan ini tetap harus disaring di sini).
		if hadirSet[key] {
			continue
		}
		tercoverSet[key] = true
		tercover = append(tercover, tercoverEntryFromDokumen(d, jenisLookup))
	}

	sixDayWeek := sixDayWeekForPegawai(pegawai)
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
		IsSekolah:       isSekolahPegawai(pegawai),
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
	isAdmin := claims.RoleName == "administrator" || claims.RoleName == "admin" || claims.IsAdminAbsensi
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
