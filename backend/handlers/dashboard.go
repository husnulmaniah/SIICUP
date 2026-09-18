package handlers

import (
	"net/http"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

func RegisterDashboardRoutes(mux *http.ServeMux, db *gorm.DB) {
	mux.Handle("GET /api/dashboard", middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
		dashboardHandler(w, r, db)
	}, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole()))
}

func dashboardHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	currentYear := time.Now().Year()

	switch claims.RoleName {
	case "administrator", "admin":
		var totalPegawai, totalPending, totalDisetujui, totalDitolak int64
		db.Model(&models.Pegawai{}).Count(&totalPegawai)
		db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusDisetuju).Count(&totalDisetujui)
		db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusDitolak).Count(&totalDitolak)

		var recent []models.PengajuanCuti
		db.Preload("Pegawai").Preload("JenisCuti").Order("created_at desc").Limit(8).Find(&recent)

		utils.Success(w, "ok", map[string]interface{}{
			"role":              claims.RoleName,
			"total_pegawai":     totalPegawai,
			"total_pending":     totalPending,
			"total_disetujui":   totalDisetujui,
			"total_ditolak":     totalDitolak,
			"pengajuan_terbaru": recent,
		})

	case "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", map[string]interface{}{"role": claims.RoleName})
			return
		}
		var totalBawahan, totalPending, totalDisetujui int64
		db.Model(&models.Pegawai{}).Where("id_atasan = ?", *claims.IDPegawai).Count(&totalBawahan)
		sub := "id_pegawai IN (SELECT id FROM pegawai WHERE id_atasan = ?)"
		db.Model(&models.PengajuanCuti{}).Where(sub, *claims.IDPegawai).Where("status = ?", models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where(sub, *claims.IDPegawai).Where("status = ?", models.StatusDisetuju).Count(&totalDisetujui)

		var menunggu []models.PengajuanCuti
		db.Preload("Pegawai").Preload("JenisCuti").Where(sub, *claims.IDPegawai).
			Where("status = ?", models.StatusPending).Order("created_at asc").Limit(10).Find(&menunggu)

		utils.Success(w, "ok", map[string]interface{}{
			"role":                 claims.RoleName,
			"total_bawahan":        totalBawahan,
			"total_pending":        totalPending,
			"total_disetujui":      totalDisetujui,
			"menunggu_persetujuan": menunggu,
		})

	case "pegawai":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", map[string]interface{}{"role": claims.RoleName})
			return
		}
		var jatah models.JatahCuti
		db.Where("id_pegawai = ? AND tahun = ?", *claims.IDPegawai, currentYear).First(&jatah)

		var riwayat []models.PengajuanCuti
		db.Preload("JenisCuti").Where("id_pegawai = ?", *claims.IDPegawai).
			Order("created_at desc").Limit(10).Find(&riwayat)

		var totalPending, totalDisetujui int64
		db.Model(&models.PengajuanCuti{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusDisetuju).Count(&totalDisetujui)

		// permintaanSk: kalau ada, tampilkan sebagai notifikasi "upload SK
		// Terakhir untuk Penerima TPP" di DashboardView.vue -- lihat menu
		// Penerima TPP & tombol "Kirim Permintaan SK" (handlers/tpp.go).
		var permintaanSk *models.PermintaanSk
		var psk models.PermintaanSk
		if err := db.Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPermintaanSkMenunggu).
			Order("created_at desc").First(&psk).Error; err == nil {
			permintaanSk = &psk
		}

		utils.Success(w, "ok", map[string]interface{}{
			"role":              claims.RoleName,
			"jatah_tahun_ini":   jatah.JumlahHari,
			"terpakai":          jatah.Terpakai,
			"sisa":              jatah.JumlahHari - jatah.Terpakai,
			"total_pending":     totalPending,
			"total_disetujui":   totalDisetujui,
			"riwayat_cuti":      riwayat,
			"statistik_absensi": statistikAbsensiBulanIni(db, *claims.IDPegawai),
			"permintaan_sk":     permintaanSk,
		})

	default:
		utils.Success(w, "ok", map[string]interface{}{"role": claims.RoleName})
	}
}

// statistikAbsensiPegawai adalah ringkasan kehadiran SATU pegawai untuk
// bulan berjalan, ditampilkan sebagai kartu statistik pada dashboard
// pegawai (DashboardView.vue).
type statistikAbsensiPegawai struct {
	Bulan          int `json:"bulan"`
	Tahun          int `json:"tahun"`
	TotalHariKerja int `json:"total_hari_kerja"`
	Hadir          int `json:"hadir"`
	Sakit          int `json:"sakit"`
	Izin           int `json:"izin"`
	Cuti           int `json:"cuti"`
	CutiMelahirkan int `json:"cuti_melahirkan"`
	// Lainnya: dokumen surat kolektif ber-Kode custom selain DD/I/S (lihat
	// models.JenisSurat) -- key-nya kode itu sendiri, sama seperti
	// jumlah_lainnya pada rekap admin (absensi_admin.go).
	Lainnya             map[string]int `json:"lainnya"`
	TidakAbsenPulang    int            `json:"tidak_absen_pulang"`
	TidakMelakukanAbsen int            `json:"tidak_melakukan_absensi"`
}

// statistikAbsensiBulanIni menghitung ringkasan kehadiran satu pegawai untuk
// bulan berjalan (WITA), sampai hari ini kalau bulan ini masih berjalan.
// Setiap hari kerja (mengikuti pola 5/6 hari & tanggal merah, sama seperti
// riwayatAbsenSaya/rekap absen admin) diklasifikasikan PERSIS SATU kategori,
// dengan urutan prioritas: Hadir (ada absen masuk sungguhan, termasuk yang
// ditandai Dinas Dalam) -> surat/dokumen kolektif (Sakit/Izin/lainnya, kode
// dari master Jenis Surat -- dokumen berkode DD dihitung "ada suratnya" tapi
// tidak masuk kategori manapun yang ditampilkan di sini) -> Cuti/Cuti
// Melahirkan (pengajuan cuti yang SUDAH DISETUJUI & rentang tanggalnya
// menutupi hari itu, dibedakan dari nama JenisCuti yang mengandung kata
// "melahirkan") -> kalau tidak satu pun ada, dihitung "tidak melakukan
// absensi". "Tidak absen pulang" BUKAN kategori tersendiri (tumpang tindih
// dengan Hadir, bukan pengurang dari total hari kerja) -- flag tambahan
// untuk hari yang sudah lewat (bukan hari ini, karena absen pulang hari ini
// masih mungkin belum waktunya) di mana pegawai absen masuk tapi tidak
// pernah absen pulang.
func statistikAbsensiBulanIni(db *gorm.DB, idPegawai uint) statistikAbsensiPegawai {
	now := absensiNow()
	bulan := now.Month()
	tahun := now.Year()
	loc := now.Location()
	start := time.Date(tahun, bulan, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)
	today := absensiToday()
	limit := end
	if today.Before(limit) {
		limit = today
	}

	out := statistikAbsensiPegawai{Bulan: int(bulan), Tahun: tahun, Lainnya: map[string]int{}}

	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").First(&pegawai, idPegawai).Error; err != nil {
		return out
	}
	sixDayWeek := sixDayWeekForPegawai(pegawai)
	out.TotalHariKerja = len(workingDaysInRange(db, start, end, sixDayWeek))
	if start.After(limit) {
		return out
	}

	var absensiRows []models.Absensi
	db.Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", idPegawai, start, end).Find(&absensiRows)
	absenByTanggal := map[string]models.Absensi{}
	for _, a := range absensiRows {
		absenByTanggal[a.Tanggal.Format("2006-01-02")] = a
	}

	var dokumenRows []models.AbsensiDokumen
	db.Omit("file").Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", idPegawai, start, limit).Find(&dokumenRows)
	jenisLookup := jenisSuratLookup(db)
	dokumenByTanggal := map[string]models.AbsensiDokumen{}
	for _, d := range dokumenRows {
		dokumenByTanggal[d.Tanggal.Format("2006-01-02")] = d
	}

	var cutiRows []models.PengajuanCuti
	db.Preload("JenisCuti").
		Where("id_pegawai = ? AND status = ? AND tgl_mulai <= ? AND tgl_selesai >= ?", idPegawai, models.StatusDisetuju, limit, start).
		Find(&cutiRows)
	// dipetakan per tanggal (string "2006-01-02"), BUKAN dibandingkan
	// langsung sebagai time.Time (d.Before/d.After) -- TglMulai/TglSelesai
	// dari database punya location Time.Time sendiri (biasanya UTC dari
	// driver Postgres), berbeda dengan `d` pada workingDaysWithHolidaySet di
	// bawah yang memakai `loc` (WITA). Membandingkan instant time.Time dari
	// dua location berbeda secara langsung bisa meleset satu hari pas di
	// tanggal mulai/selesai (mis. hari pertama cuti keliru dianggap belum
	// masuk rentang) -- diiterasi & diformat ke string di sini supaya cuma
	// tanggal kalendernya saja yang dibandingkan, sama seperti pola
	// tercoverSet/hadirSet di absensi.go.
	cutiByTanggal := map[string]bool{}
	melahirkanByTanggal := map[string]bool{}
	for _, pc := range cutiRows {
		melahirkan := pc.JenisCuti != nil && strings.Contains(strings.ToLower(pc.JenisCuti.Jenis), "melahirkan")
		for d := pc.TglMulai; !d.After(pc.TglSelesai); d = d.AddDate(0, 0, 1) {
			key := d.Format("2006-01-02")
			cutiByTanggal[key] = true
			if melahirkan {
				melahirkanByTanggal[key] = true
			}
		}
	}

	holidaySet := holidaySetInRange(db, start, limit)
	for _, d := range workingDaysWithHolidaySet(start, limit, sixDayWeek, holidaySet) {
		key := d.Format("2006-01-02")
		if a, ada := absenByTanggal[key]; ada && a.JamMasuk != nil {
			out.Hadir++
			if a.JamPulang == nil && d.Before(today) {
				out.TidakAbsenPulang++
			}
			continue
		}
		if dok, ada := dokumenByTanggal[key]; ada {
			switch kode := kodeUntukJenis(jenisLookup, dok.Jenis); kode {
			case "S":
				out.Sakit++
			case "I":
				out.Izin++
			case "DD":
				// dinas dalam lewat surat tugas/berita acara -- sudah ada
				// suratnya, sengaja tidak masuk kategori manapun yang
				// ditampilkan di kartu statistik ini.
			default:
				if kode != "" {
					out.Lainnya[kode]++
				}
			}
			continue
		}
		if cutiByTanggal[key] {
			if melahirkanByTanggal[key] {
				out.CutiMelahirkan++
			} else {
				out.Cuti++
			}
			continue
		}
		out.TidakMelakukanAbsen++
	}
	return out
}
