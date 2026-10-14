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
		db.Preload("Pegawai", func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }).Preload("JenisCuti").Order("created_at desc").Limit(8).Find(&recent)

		// ringkasanPegawai/rekapAbsensiHariIni/ringkasanPensiun/
		// ringkasanPetaJabatan/ringkasanKp4/ringkasanLainnya: permintaan
		// pengguna -- dashboard administrator/admin menampilkan statistik
		// dari SEMUA menu penting (Data Pegawai, Pengajuan Cuti, Rekap
		// Absensi, Pengajuan Pensiun, Peta Jabatan, KP4, dan menu lain),
		// bukan cuma Pengajuan Cuti seperti sebelumnya. Lihat masing-masing
		// fungsi di bawah untuk detail perhitungannya.
		ringkasanPegawai, rekapAbsensiHariIni := ringkasanPegawaiDanAbsensiHariIniAdmin(db)

		utils.Success(w, "ok", map[string]interface{}{
			"role":              claims.RoleName,
			"total_pegawai":     totalPegawai,
			"total_pending":     totalPending,
			"total_disetujui":   totalDisetujui,
			"total_ditolak":     totalDitolak,
			"pengajuan_terbaru": recent,

			"ringkasan_pegawai":      ringkasanPegawai,
			"ringkasan_cuti":         ringkasanCutiAdmin(db, totalPending, totalDisetujui, totalDitolak, currentYear),
			"rekap_absensi_hari_ini": rekapAbsensiHariIni,
			"ringkasan_pensiun":      ringkasanPensiunAdmin(db, currentYear),
			"ringkasan_peta_jabatan": ringkasanPetaJabatanAdmin(db),
			"ringkasan_kp4":          ringkasanKp4Admin(db),
			"ringkasan_lainnya":      ringkasanLainnyaAdmin(db),
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
		db.Preload("Pegawai", func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }).Preload("JenisCuti").Where(sub, *claims.IDPegawai).
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

		// jenisJabatan: dipakai DashboardView.vue untuk menyembunyikan kartu
		// "Statistik Absensi" bagi pegawai dengan jabatan fungsional (Guru
		// Ahli Pertama/Muda/Madya, dst) -- permintaan pengguna: dashboard
		// Guru cukup menampilkan statistik Pengajuan Cuti & Kenaikan Pangkat
		// saja, bukan statistik absensi (sama pola dengan Jabatan.JenisJabatan
		// yang sudah dipakai untuk usia pensiun, lihat pengajuan_pensiun.go).
		var pegawaiJabatan models.Pegawai
		jenisJabatan := ""
		db.Preload("Jabatan").Select("id", "id_jabatan").First(&pegawaiJabatan, *claims.IDPegawai)
		if pegawaiJabatan.Jabatan != nil {
			jenisJabatan = pegawaiJabatan.Jabatan.JenisJabatan
		}

		var riwayat []models.PengajuanCuti
		db.Preload("JenisCuti").Where("id_pegawai = ?", *claims.IDPegawai).
			Order("created_at desc").Limit(10).Find(&riwayat)

		var totalPending, totalDisetujui int64
		db.Model(&models.PengajuanCuti{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPending).Count(&totalPending)
		db.Model(&models.PengajuanCuti{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusDisetuju).Count(&totalDisetujui)

		// permintaanSk: kalau ada, tampilkan sebagai notifikasi "upload SK
		// Terakhir untuk Penerima TPP" di DashboardView.vue -- lihat menu
		// Penerima TPP & tombol "Kirim Permintaan SK" (handlers/tpp.go).
		// Preload Pegawai (TANPA kolom byte dokumennya, lihat
		// dokumenFileFields) supaya frontend tahu APAKAH pegawai ini sudah
		// punya SK Terakhir atau belum (psk.Pegawai.SkTerakhirNama) --
		// menentukan tampilan notifikasinya: "silakan upload" (belum ada SK)
		// vs. "SK Anda saat ini ... klik untuk melihat, upload ulang kalau
		// tidak sesuai" (SK sudah ada, administrator minta diperiksa ulang).
		var permintaanSk *models.PermintaanSk
		var psk models.PermintaanSk
		if err := db.Preload("Pegawai", func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }).
			Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.StatusPermintaanSkMenunggu).
			Order("created_at desc").First(&psk).Error; err == nil {
			permintaanSk = &psk
		}

		// ringkasanKenaikanPangkat: ditampilkan sebagai kartu statistik baru
		// "Pengajuan Kenaikan Pangkat" di dashboard pegawai -- permintaan
		// pengguna supaya pegawai jabatan Guru (yang tidak lagi melihat
		// Statistik Absensi, lihat jenisJabatan di atas) tetap punya sesuatu
		// yang relevan untuk dipantau selain Pengajuan Cuti. Menghitung dari
		// DUA entitas sekaligus (lihat komentar alur di
		// handlers/peta_jabatan_kenaikan_pangkat.go): PengajuanKenaikanPangkat
		// (tahap atasan) & PerubahanJabatanPegawai (tahap upload SK +
		// persetujuan administrator, yang sekarang benar-benar mengubah data
		// pegawai).
		var kpMenungguAtasan, kpDitolakAtasan int64
		db.Model(&models.PengajuanKenaikanPangkat{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.KenaikanPangkatMenungguAtasan).Count(&kpMenungguAtasan)
		db.Model(&models.PengajuanKenaikanPangkat{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.KenaikanPangkatDitolakAtasan).Count(&kpDitolakAtasan)
		var pjMenungguUpload, pjMenungguAdmin, pjSelesai int64
		db.Model(&models.PerubahanJabatanPegawai{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.PerubahanJabatanMenungguUpload).Count(&pjMenungguUpload)
		db.Model(&models.PerubahanJabatanPegawai{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.PerubahanJabatanMenungguAdmin).Count(&pjMenungguAdmin)
		db.Model(&models.PerubahanJabatanPegawai{}).Where("id_pegawai = ? AND status = ?", *claims.IDPegawai, models.PerubahanJabatanDisetujui).Count(&pjSelesai)
		ringkasanKenaikanPangkat := map[string]interface{}{
			"menunggu_atasan":    kpMenungguAtasan,
			"menunggu_upload_sk": pjMenungguUpload,
			"menunggu_admin":     pjMenungguAdmin,
			"selesai":            pjSelesai,
			"ditolak":            kpDitolakAtasan,
			"total":              kpMenungguAtasan + pjMenungguUpload + pjMenungguAdmin + pjSelesai + kpDitolakAtasan,
		}

		utils.Success(w, "ok", map[string]interface{}{
			"role":            claims.RoleName,
			"jatah_tahun_ini": jatah.JumlahHari,
			"terpakai":        jatah.Terpakai,
			"sisa":            jatah.JumlahHari - jatah.Terpakai,
			"total_pending":   totalPending,
			"total_disetujui": totalDisetujui,
			"riwayat_cuti":    riwayat,
			// jenis_jabatan: "fungsional"/"struktural"/"pelaksana" (lihat
			// komentar jenisJabatan di atas) -- dipakai DashboardView.vue untuk
			// menyembunyikan statistik_absensi bagi jabatan Guru/fungsional.
			"jenis_jabatan":              jenisJabatan,
			"statistik_absensi":          statistikAbsensiBulanIni(db, *claims.IDPegawai),
			"ringkasan_kenaikan_pangkat": ringkasanKenaikanPangkat,
			"permintaan_sk":              permintaanSk,
			// kelengkapan_data: SATU ringkasan gabungan (persentase + daftar
			// kategori yang belum lengkap) menggantikan 3 kartu "KP4 Belum
			// Terisi"/"Data Pegawai Belum Lengkap"/"Tanda Tangan Digital Belum
			// Dibuat" yang dulu SELALU tampil terpisah -- permintaan pengguna
			// supaya dashboard pegawai lebih bersih: sekarang satu kartu
			// persentase yang baru menampilkan rinciannya kalau diklik (lihat
			// hitungKelengkapanDashboard di bawah & DashboardView.vue).
			"kelengkapan_data": hitungKelengkapanDashboard(db, *claims.IDPegawai),
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

// kelengkapanKategoriOut adalah SATU kelompok data yang masih kosong pada
// kartu "Kelengkapan Data Pegawai" di dashboard -- Items kosong berarti
// kategori ini biner (cuma ada/tidak ada, mis. tanda tangan digital), bukan
// daftar field. Aksi/AksiLabel mengarahkan tombol "lengkapi" pada kategori
// itu ke menu yang tepat (persis tombol-tombol yang dulu ada di 3 kartu
// terpisah, lihat komentar kelengkapan_data di dashboardHandler).
type kelengkapanKategoriOut struct {
	Label     string   `json:"label"`
	Items     []string `json:"items"`
	Aksi      string   `json:"aksi"`
	AksiLabel string   `json:"aksi_label"`
}

type kelengkapanDashboardOut struct {
	Persentase int                      `json:"persentase"`
	Lengkap    bool                     `json:"lengkap"`
	Kategori   []kelengkapanKategoriOut `json:"kategori"`
}

// hitungKelengkapanDashboard menggabungkan kelengkapan KP4 (lihat
// hitungKelengkapanKp4 di handlers/kp4.go) & tanda tangan digital menjadi
// SATU persentase + daftar kategori yang masih kosong, dipakai dashboard
// pegawai untuk satu kartu ringkas ("Kelengkapan Data: NN%") yang baru
// menampilkan rinciannya kalau diklik -- menggantikan 3 kartu peringatan
// terpisah yang dulu selalu tampil sekaligus (permintaan pengguna: "agar
// tampilan dashboard pegawai lebih bersih"). Denominator TOTAL ITEM (13)
// SENGAJA ditulis tetap di sini (bukan dihitung dinamis) supaya persentase
// stabil & gampang ditelusuri: 7 field KP4 + 5 field Data Pegawai (lihat
// hitungKelengkapanKp4) + 1 tanda tangan digital.
func hitungKelengkapanDashboard(db *gorm.DB, idPegawai uint) kelengkapanDashboardOut {
	const totalItem = 13

	kp4 := kp4KelengkapanUntukDashboard(db, idPegawai)
	var ttdRow models.Pegawai
	db.Select("ttd_pegawai_nama").First(&ttdRow, idPegawai)
	ttdKosong := ttdRow.TtdPegawaiNama == ""

	missing := 0
	var kategori []kelengkapanKategoriOut
	if kp4 != nil && len(kp4.Kp4FieldKosong) > 0 {
		missing += len(kp4.Kp4FieldKosong)
		kategori = append(kategori, kelengkapanKategoriOut{
			Label: "KP4 Belum Terisi", Items: kp4.Kp4FieldKosong, Aksi: "/kp4", AksiLabel: "Lengkapi KP4",
		})
	}
	if kp4 != nil && len(kp4.DataPegawaiKosong) > 0 {
		missing += len(kp4.DataPegawaiKosong)
		kategori = append(kategori, kelengkapanKategoriOut{
			Label: "Data Pegawai Belum Lengkap", Items: kp4.DataPegawaiKosong, Aksi: "/profil-saya", AksiLabel: "Ajukan Perubahan Data",
		})
	}
	if ttdKosong {
		missing++
		kategori = append(kategori, kelengkapanKategoriOut{
			Label: "Tanda Tangan Digital Belum Dibuat", Items: []string{}, Aksi: "/profil-saya", AksiLabel: "Buat Tanda Tangan",
		})
	}

	return kelengkapanDashboardOut{
		Persentase: (totalItem - missing) * 100 / totalItem,
		Lengkap:    missing == 0,
		Kategori:   kategori,
	}
}

// ============================================================
// Dashboard administrator/admin -- ringkasan SEMUA menu penting (permintaan
// pengguna: "dashboard menampilkan statistik dari semua menu penting seperti
// data pegawai, pengajuan cuti, rekapan absensi, pengajuan pensiun, peta
// jabatan, kp4 dan menu penting lainnya, buat seinformatif mungkin").
// Masing-masing fungsi di bawah SENGAJA berdiri sendiri (bukan satu query
// raksasa) supaya mudah ditelusuri/diubah per menu, mengikuti gaya file ini
// -- hanya ringkasanPegawaiDanAbsensiHariIniAdmin yang menggabungkan DUA
// ringkasan sekaligus (Data Pegawai & Rekap Absensi Hari Ini) karena
// keduanya butuh baris Pegawai aktif yang SAMA, supaya tidak query dua kali.
// ============================================================

// ringkasanPegawaiAdminOut adalah kartu "Data Pegawai" pada dashboard.
type ringkasanPegawaiAdminOut struct {
	Total   int64 `json:"total"`
	Aktif   int64 `json:"aktif"`
	Sekolah int64 `json:"sekolah"`
	Dinas   int64 `json:"dinas"`
	Pensiun int64 `json:"pensiun"`
	Mutasi  int64 `json:"mutasi"`
}

// rekapAbsensiHariIniAdminOut adalah kartu "Rekap Absensi Hari Ini" pada
// dashboard -- ringkasan kehadiran SELURUH pegawai aktif (dinas & sekolah
// sekaligus, masing-masing dengan pola hari kerjanya sendiri, lihat
// sixDayWeekForPegawai) untuk HARI INI saja, bukan satu bulan penuh seperti
// statistikAbsensiBulanIni (itu punya pegawai, ini punya administrator).
// WajibHadir bisa 0 kalau hari ini libur untuk SEMUA pegawai (Minggu, atau
// tanggal merah) -- itulah sebabnya tidak ada flag "hari_libur" terpisah,
// kartu di frontend cukup menampilkan apa adanya kalau wajib_hadir == 0.
type rekapAbsensiHariIniAdminOut struct {
	Tanggal    string `json:"tanggal"`
	WajibHadir int    `json:"wajib_hadir"`
	Hadir      int    `json:"hadir"`
	Terlambat  int    `json:"terlambat"`
	Sakit      int    `json:"sakit"`
	Izin       int    `json:"izin"`
	Cuti       int    `json:"cuti"`
	BelumAbsen int    `json:"belum_absen"`
}

// ringkasanPegawaiDanAbsensiHariIniAdmin menghitung kartu "Data Pegawai" &
// "Rekap Absensi Hari Ini" SEKALIGUS dari satu daftar pegawai aktif yang
// sama (scopePegawaiAktifDinas) -- pegawai Pensiun/Mutasi tetap dihitung
// terpisah di Total vs Aktif, tapi TIDAK ikut Rekap Absensi Hari Ini (sudah
// tidak lagi bertugas).
func ringkasanPegawaiDanAbsensiHariIniAdmin(db *gorm.DB) (ringkasanPegawaiAdminOut, rekapAbsensiHariIniAdminOut) {
	var pegawai ringkasanPegawaiAdminOut
	db.Model(&models.Pegawai{}).Count(&pegawai.Total)
	// ILIKE tanpa wildcard (persis seperti statusNonaktifIDs di
	// handlers/pegawai.go) -- pencocokan EXACT nama status, cuma case-
	// insensitive, bukan substring, supaya status lain yang kebetulan
	// mengandung kata "pensiun"/"mutasi" (kalau ada) tidak ikut terhitung.
	var idsPensiun, idsMutasi []uint
	db.Model(&models.Status{}).Where("status ILIKE ?", "pensiun").Pluck("id", &idsPensiun)
	db.Model(&models.Status{}).Where("status ILIKE ?", "mutasi").Pluck("id", &idsMutasi)
	if len(idsPensiun) > 0 {
		db.Model(&models.Pegawai{}).Where("id_status IN (?)", idsPensiun).Count(&pegawai.Pensiun)
	}
	if len(idsMutasi) > 0 {
		db.Model(&models.Pegawai{}).Where("id_status IN (?)", idsMutasi).Count(&pegawai.Mutasi)
	}

	var aktifRows []models.Pegawai
	scopePegawaiAktifDinas(db, db.Preload("UnitKerja", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "tempat_kerja") }).
		Select("id", "id_unit_kerja", "tempat_tgs", "id_status")).Find(&aktifRows)
	pegawai.Aktif = int64(len(aktifRows))

	today := absensiToday()
	holidaySet := holidaySetInRange(db, today, today)
	wajibSekolahHariIni := len(workingDaysWithHolidaySet(today, today, true, holidaySet)) > 0
	wajibDinasHariIni := len(workingDaysWithHolidaySet(today, today, false, holidaySet)) > 0

	allIDs := make([]uint, 0, len(aktifRows))
	wajibByID := map[uint]bool{}
	for _, p := range aktifRows {
		allIDs = append(allIDs, p.ID)
		sekolah := isSekolahPegawai(p)
		if sekolah {
			pegawai.Sekolah++
		} else {
			pegawai.Dinas++
		}
		if sekolah && wajibSekolahHariIni {
			wajibByID[p.ID] = true
		} else if !sekolah && wajibDinasHariIni {
			wajibByID[p.ID] = true
		}
	}

	rekap := rekapAbsensiHariIniAdminOut{Tanggal: today.Format("2006-01-02"), WajibHadir: len(wajibByID)}
	if len(allIDs) == 0 || rekap.WajibHadir == 0 {
		return pegawai, rekap
	}

	var absensiRows []models.Absensi
	db.Where("id_pegawai IN (?) AND tanggal = ?", allIDs, today).Find(&absensiRows)
	absenByID := map[uint]models.Absensi{}
	for _, a := range absensiRows {
		absenByID[a.IDPegawai] = a
	}

	var dokumenRows []models.AbsensiDokumen
	db.Omit("file").Where("id_pegawai IN (?) AND tanggal = ?", allIDs, today).Find(&dokumenRows)
	jenisLookup := jenisSuratLookup(db)
	dokumenByID := map[uint]models.AbsensiDokumen{}
	for _, d := range dokumenRows {
		dokumenByID[d.IDPegawai] = d
	}

	var cutiRows []models.PengajuanCuti
	db.Where("id_pegawai IN (?) AND status = ? AND tgl_mulai <= ? AND tgl_selesai >= ?", allIDs, models.StatusDisetuju, today, today).Find(&cutiRows)
	cutiByID := map[uint]bool{}
	for _, c := range cutiRows {
		cutiByID[c.IDPegawai] = true
	}

	for id := range wajibByID {
		if a, ada := absenByID[id]; ada && a.JamMasuk != nil {
			rekap.Hadir++
			if a.TerlambatMenit > 0 {
				rekap.Terlambat++
			}
			continue
		}
		if dok, ada := dokumenByID[id]; ada {
			switch kodeUntukJenis(jenisLookup, dok.Jenis) {
			case "S":
				rekap.Sakit++
			case "I":
				rekap.Izin++
			}
			continue
		}
		if cutiByID[id] {
			rekap.Cuti++
			continue
		}
		rekap.BelumAbsen++
	}
	return pegawai, rekap
}

// ringkasanCutiAdminOut adalah kartu "Pengajuan Cuti" pada dashboard --
// Pending/Disetujui/Ditolak dihitung SEKALI di dashboardHandler (dipakai
// juga oleh field total_pending/total_disetujui/total_ditolak lama di level
// atas supaya tidak berubah/pecah kompatibilitas), DiajukanBulanIni dihitung
// khusus di sini.
type ringkasanCutiAdminOut struct {
	Pending          int64 `json:"pending"`
	Disetujui        int64 `json:"disetujui"`
	Ditolak          int64 `json:"ditolak"`
	DiajukanBulanIni int64 `json:"diajukan_bulan_ini"`
}

func ringkasanCutiAdmin(db *gorm.DB, pending, disetujui, ditolak int64, tahun int) ringkasanCutiAdminOut {
	out := ringkasanCutiAdminOut{Pending: pending, Disetujui: disetujui, Ditolak: ditolak}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	db.Model(&models.PengajuanCuti{}).Where("created_at >= ?", start).Count(&out.DiajukanBulanIni)
	return out
}

// ringkasanPensiunAdminOut adalah kartu "Pengajuan Pensiun" pada dashboard.
// AkanPensiun1Tahun: PERINGATAN DINI -- pegawai aktif yang akan mencapai
// usia pensiun standar (usiaPensiunPegawai, berbeda Pelaksana/Struktural vs
// Fungsional) dalam 1 tahun ke depan ATAU SUDAH lewat tapi belum diajukan
// pensiunnya -- supaya administrator bisa menyiapkan berkasnya lebih awal,
// bukan cuma menunggu pegawai/atasan mengajukan sendiri.
type ringkasanPensiunAdminOut struct {
	Pending           int64 `json:"pending"`
	DisetujuiTahunIni int64 `json:"disetujui_tahun_ini"`
	Ditolak           int64 `json:"ditolak"`
	AkanPensiun1Tahun int64 `json:"akan_pensiun_1_tahun"`
}

func ringkasanPensiunAdmin(db *gorm.DB, tahun int) ringkasanPensiunAdminOut {
	var out ringkasanPensiunAdminOut
	db.Model(&models.PengajuanPensiun{}).Where("status = ?", models.StatusPending).Count(&out.Pending)
	db.Model(&models.PengajuanPensiun{}).
		Where("status = ? AND EXTRACT(YEAR FROM tgl_keputusan) = ?", models.StatusDisetuju, tahun).
		Count(&out.DisetujuiTahunIni)
	db.Model(&models.PengajuanPensiun{}).Where("status = ?", models.StatusDitolak).Count(&out.Ditolak)

	pengaturan, err := getPengaturanPensiunRow(db)
	if err != nil {
		return out
	}
	var pegawaiRows []models.Pegawai
	scopePegawaiAktifDinas(db, db.Preload("Jabatan").Select("id", "id_jabatan", "tgl_lahir").Where("tgl_lahir IS NOT NULL")).Find(&pegawaiRows)
	now := time.Now()
	batas := now.AddDate(1, 0, 0)
	for _, p := range pegawaiRows {
		if p.TglLahir == nil {
			continue
		}
		usia := usiaPensiunPegawai(p, pengaturan)
		tglPensiun := p.TglLahir.AddDate(usia, 0, 0)
		if !tglPensiun.After(batas) {
			out.AkanPensiun1Tahun++
		}
	}
	return out
}

// ringkasanPetaJabatanAdminOut adalah kartu "Peta Jabatan" pada dashboard --
// ringkasan GLOBAL (seluruh Sekolah + Dinas sekaligus) dari Kebutuhan (K,
// SUM seluruh baris FormasiJabatan) vs Terisi (B, jumlah pegawai aktif yang
// sudah punya Jabatan & Unit Kerja) -- SENGAJA dihitung lewat dua query
// agregat langsung (bukan memanggil hitungPetaJabatanSekolah per unit kerja
// satu-satu, yang bisa jadi puluhan/ratusan query kalau sekolahnya banyak)
// supaya kartu dashboard ini tetap ringan tanpa N+1 query.
type ringkasanPetaJabatanAdminOut struct {
	TotalKebutuhan int64 `json:"total_kebutuhan"`
	TotalTerisi    int64 `json:"total_terisi"`
	Selisih        int64 `json:"selisih"`
}

func ringkasanPetaJabatanAdmin(db *gorm.DB) ringkasanPetaJabatanAdminOut {
	var out ringkasanPetaJabatanAdminOut
	db.Model(&models.FormasiJabatan{}).Select("COALESCE(SUM(kebutuhan), 0)").Row().Scan(&out.TotalKebutuhan)
	scopePegawaiAktifDinas(db, db.Model(&models.Pegawai{}).Where("id_jabatan IS NOT NULL AND id_unit_kerja IS NOT NULL")).Count(&out.TotalTerisi)
	out.Selisih = out.TotalTerisi - out.TotalKebutuhan
	return out
}

// ringkasanKp4AdminOut adalah kartu "KP4" pada dashboard -- persentase
// pegawai aktif yang SUDAH mengisi data KP4-nya (punya baris Kp4Data sendiri,
// lihat models.Kp4Data) dari seluruh pegawai aktif. Ringkasan cepat org-wide,
// BUKAN pengecekan kelengkapan field satu-satu (itu punya hitungKelengkapanKp4,
// dipakai per pegawai di menu KP4 & dashboard pegawai sendiri -- memanggilnya
// untuk SETIAP pegawai di sini akan jadi N+1 query untuk kartu yang cuma
// butuh satu angka ringkas).
type ringkasanKp4AdminOut struct {
	SudahIsi   int64 `json:"sudah_isi"`
	BelumIsi   int64 `json:"belum_isi"`
	Persentase int   `json:"persentase"`
}

func ringkasanKp4Admin(db *gorm.DB) ringkasanKp4AdminOut {
	var out ringkasanKp4AdminOut
	var totalAktif int64
	scopePegawaiAktifDinas(db, db.Model(&models.Pegawai{})).Count(&totalAktif)
	subQuery := scopePegawaiAktifDinas(db, db.Model(&models.Pegawai{})).Select("id")
	db.Model(&models.Kp4Data{}).Where("id_pegawai IN (?)", subQuery).Count(&out.SudahIsi)
	out.BelumIsi = totalAktif - out.SudahIsi
	if totalAktif > 0 {
		out.Persentase = int(out.SudahIsi * 100 / totalAktif)
	}
	return out
}

// ringkasanLainnyaAdminOut adalah kartu "Menu Lainnya" pada dashboard --
// jumlah pengajuan yang SAAT INI menunggu tindakan administrator/admin pada
// menu-menu lain yang belum punya kartu statistik sendiri di atas. Masing-
// masing dihitung pada tahap antrean administrator SAJA (bukan tahap atasan
// yang bukan tanggung jawab administrator), KECUALI PermintaanSkPending
// yang memang menunggu PEGAWAI (upload SK Terakhir Penerima TPP) -- tetap
// ditampilkan supaya administrator tahu berapa yang masih belum direspons.
type ringkasanLainnyaAdminOut struct {
	KenaikanPangkatPending int64 `json:"kenaikan_pangkat_pending"`
	PerubahanDataPending   int64 `json:"perubahan_data_pending"`
	BeritaAcaraPending     int64 `json:"berita_acara_pending"`
	SuratKolektifPending   int64 `json:"surat_kolektif_pending"`
	PermintaanSkPending    int64 `json:"permintaan_sk_pending"`
}

func ringkasanLainnyaAdmin(db *gorm.DB) ringkasanLainnyaAdminOut {
	var out ringkasanLainnyaAdminOut
	db.Model(&models.PerubahanJabatanPegawai{}).Where("status = ?", models.PerubahanJabatanMenungguAdmin).Count(&out.KenaikanPangkatPending)
	db.Model(&models.PerubahanDataPegawai{}).Where("status = ?", models.StatusPending).Count(&out.PerubahanDataPending)
	db.Model(&models.PengajuanBeritaAcara{}).Where("status = ?", models.PengajuanBeritaAcaraMenungguAdmin).Count(&out.BeritaAcaraPending)
	db.Model(&models.PengajuanSuratKolektif{}).Where("status = ?", models.PengajuanSuratKolektifMenunggu).Count(&out.SuratKolektifPending)
	db.Model(&models.PermintaanSk{}).Where("status = ?", models.StatusPermintaanSkMenunggu).Count(&out.PermintaanSkPending)
	return out
}
