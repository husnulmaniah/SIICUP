package database

import (
	"log"
	"strings"
	"time"

	"cuti-app/models"

	"gorm.io/gorm"
)

// Migrate creates/updates all tables in dependency order.
func Migrate(db *gorm.DB) {
	err := db.AutoMigrate(
		&models.Role{},
		&models.Jabatan{},
		&models.Kecamatan{},
		&models.UnitKerja{},
		&models.ShiftKerja{},
		&models.ShiftKerjaHari{},
		&models.ShiftKerjaUnitKerja{},
		&models.ShiftKerjaJabatan{},
		&models.Status{},
		&models.Pangkat{},
		&models.Golongan{},
		&models.PangkatGol{},
		&models.JenisCuti{},
		&models.PolaHariKerja{},
		&models.TglMerah{},
		// SubJabatan dimigrasikan SEBELUM Pegawai -- Pegawai.SubJabatan
		// (lihat models.go) butuh tabel "sub_jabatan" SUDAH ADA supaya GORM
		// bisa memasang foreign key-nya saat AutoMigrate Pegawai di bawah
		// (SubJabatan sendiri hanya butuh UnitKerja & Jabatan yang sudah
		// dimigrasikan lebih dulu di atas, jadi aman diletakkan di sini).
		&models.SubJabatan{},
		&models.Pegawai{},
		&models.User{},
		&models.JatahCuti{},
		&models.PengajuanCuti{},
		&models.PengajuanDokumen{},
		&models.PengaturanSurat{},
		&models.PerubahanDataPegawai{},
		&models.NotifikasiPegawai{},
		&models.PengaturanPensiun{},
		&models.PengajuanPensiun{},
		&models.PengaturanKenaikanGajiBerkala{},
		&models.Absensi{},
		&models.AbsensiDokumen{},
		&models.PengaturanAbsensi{},
		&models.JenisSurat{},
		&models.PengajuanSuratKolektif{},
		&models.TemplateSurat{},
		&models.PengaturanTpp{},
		&models.PermintaanSk{},
		&models.PenerimaTpp{},
		&models.Kp4Data{},
		&models.Kp4Pasangan{},
		&models.Kp4Anak{},
		&models.PengaturanKp4{},
		&models.SuratRekomendasi{},
		&models.PengajuanBeritaAcara{},
		// Peta Jabatan Sekolah -- lihat komentar lengkap di models.go
		// (bagian "PETA JABATAN -- SEKOLAH") & handlers/peta_jabatan*.go.
		&models.FormasiJabatan{},
		&models.PengajuanKenaikanPangkat{},
		&models.PerubahanJabatanPegawai{},
	)
	if err != nil {
		log.Fatalf("gagal migrasi database: %v", err)
	}

	// Seed jenis surat kolektif bawaan -- slug-nya disamakan dengan konstanta
	// AbsensiDokumen* di models.go supaya data lama yang sudah tersimpan di
	// kolom absensi_dokumen.jenis tetap valid & tetap cocok dengan baris ini.
	//
	// PENTING: dicek SATU PER SATU per slug (bukan "kalau tabel masih kosong"
	// seperti sebelumnya) -- bug yang pernah terjadi di produksi: baris
	// "Berita Acara" ditambahkan ke daftar ini belakangan, setelah database
	// produksi SUDAH terisi 3 baris bawaan lainnya (Surat Tugas/Surat Izin/
	// SKS), sehingga pengecekan lama (jenisSuratCount == 0) selalu false dan
	// baris "Berita Acara" TIDAK PERNAH ikut ter-seed -- membuat menu Berita
	// Acara gagal total dengan error 500 ("master Jenis Surat \"Berita
	// Acara\" tidak ditemukan"). Dengan pengecekan per-slug ini, baris baru
	// yang ditambahkan ke daftar di masa depan akan otomatis ikut dibuat di
	// database manapun (baru maupun yang sudah lama berjalan), bukan hanya
	// database yang benar-benar kosong.
	defaultJenisSurat := []models.JenisSurat{
		{Slug: models.AbsensiDokumenSuratTugas, Nama: "Surat Tugas", Kode: "DD"},
		{Slug: models.AbsensiDokumenBeritaAcara, Nama: "Berita Acara", Kode: "DD"},
		{Slug: models.AbsensiDokumenSuratIzin, Nama: "Surat Izin", Kode: "I"},
		{Slug: models.AbsensiDokumenSKS, Nama: "SKS -- Surat Keterangan Sakit", Kode: "S"},
	}
	for _, js := range defaultJenisSurat {
		var existing models.JenisSurat
		err := db.Where("slug = ?", js.Slug).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := db.Create(&js).Error; err != nil {
				log.Printf("peringatan: gagal seed jenis_surat slug=%s: %v", js.Slug, err)
			}
		} else if err != nil {
			log.Printf("peringatan: gagal cek jenis_surat slug=%s: %v", js.Slug, err)
		}
	}

	// Shift Kerja tadinya (rilis pertama fitur ini) dipasang ke SATU unit
	// kerja lewat kolom shift_kerja.id_unit_kerja (NOT NULL). Fitur ini lalu
	// diubah supaya SATU shift bisa dipasang ke BANYAK unit kerja lewat
	// tabel penghubung shift_kerja_unit_kerja (lihat models.ShiftKerja) --
	// kolom id_unit_kerja lama itu sudah dibuang dari struct Go, tapi
	// AutoMigrate TIDAK PERNAH menghapus kolom yang sudah tidak ada di
	// struct, jadi di database yang sudah pernah dideploy sebelum perubahan
	// ini, kolom NOT NULL itu masih tertinggal -- membuat SEMUA insert shift
	// kerja baru gagal dengan error constraint (kolom lama itu tidak pernah
	// diisi lagi oleh kode yang sekarang). Baris di bawah ini menangani
	// migrasi itu secara aman & idempotent: kalau kolom itu masih ada, data
	// pemasangan shift<->unit kerja yang sudah tersimpan di sana dipindahkan
	// dulu ke tabel penghubung yang baru (supaya tidak hilang), baru
	// kolomnya dihapus. Aman dijalankan berkali-kali -- begitu kolomnya
	// sudah tidak ada, blok ini langsung dilewati.
	if db.Migrator().HasColumn(&models.ShiftKerja{}, "id_unit_kerja") {
		if err := db.Exec(`
			INSERT INTO shift_kerja_unit_kerja (id_shift, id_unit_kerja)
			SELECT id, id_unit_kerja FROM shift_kerja
			WHERE id_unit_kerja IS NOT NULL
			ON CONFLICT (id_unit_kerja) DO NOTHING
		`).Error; err != nil {
			log.Printf("peringatan: gagal memindahkan data shift_kerja.id_unit_kerja lama ke shift_kerja_unit_kerja: %v", err)
		}
		if err := db.Migrator().DropColumn(&models.ShiftKerja{}, "id_unit_kerja"); err != nil {
			log.Printf("peringatan: gagal menghapus kolom lama shift_kerja.id_unit_kerja: %v", err)
		} else {
			log.Println("migrasi: kolom lama shift_kerja.id_unit_kerja berhasil dipindahkan & dihapus")
		}
	}

	// Jabatan tadinya (rilis pertama fitur Shift Kerja per Jabatan) dibatasi
	// SATU jabatan hanya boleh dipasangi SATU shift (uniqueIndex di
	// shift_kerja_jabatan.id_jabatan) -- ternyata banyak jabatan (mis.
	// "Pengadministrasi Perkantoran") ada di SEMUA sekolah (6 hari kerja)
	// MAUPUN di kantor dinas (5 hari kerja) sekaligus, jadi perlu bisa
	// dipasangi shift BERBEDA tergantung unit kerja pegawainya -- lihat
	// shiftKerjaUntukJabatan di handlers/absensi.go untuk cara memilih shift
	// mana yang dipakai kalau satu jabatan ternyata ada di >1 shift.
	// uniqueIndex lama dihapus di sini (AutoMigrate TIDAK PERNAH menghapus
	// index lama walau tag structnya sudah diubah ke index biasa), lalu
	// AutoMigrate dijalankan ulang KHUSUS model ini supaya index barunya
	// (non-unique) langsung dibuat. Aman dijalankan berkali-kali -- begitu
	// uniqueIndex lama sudah tidak ada, blok ini langsung dilewati.
	if db.Migrator().HasIndex(&models.ShiftKerjaJabatan{}, "IDJabatan") {
		if err := db.Migrator().DropIndex(&models.ShiftKerjaJabatan{}, "IDJabatan"); err != nil {
			log.Printf("peringatan: gagal menghapus uniqueIndex lama shift_kerja_jabatan.id_jabatan: %v", err)
		} else if err := db.AutoMigrate(&models.ShiftKerjaJabatan{}); err != nil {
			log.Printf("peringatan: gagal membuat ulang index shift_kerja_jabatan.id_jabatan: %v", err)
		} else {
			log.Println("migrasi: uniqueIndex lama shift_kerja_jabatan.id_jabatan berhasil diganti jadi index biasa (satu jabatan kini boleh >1 shift)")
		}
	}

	// Jabatan yang kadung duplikat (hanya beda besar/kecil huruf, mis.
	// "Penelaah Teknis Kebijakan" vs "PENELAAH TEKNIS KEBIJAKAN") digabung
	// & diseragamkan jadi huruf kapital di sini -- lihat mergeJabatanDuplikat
	// di bawah. Dijalankan SETELAH blok di atas (uniqueIndex lama
	// shift_kerja_jabatan.id_jabatan sudah dilonggarkan) supaya baris yang
	// dialihkan ke jabatan yang dipertahankan tidak mentok constraint lama.
	// Pencegahan duplikat BARU ke depannya ditangani models.Jabatan.
	// BeforeSave (models/models.go), bukan di sini -- blok ini HANYA
	// membereskan data lama yang sudah kadung ada sebelum hook itu dipasang.
	mergeJabatanDuplikat(db)

	// uniqueIndex case-insensitive pada nama jabatan -- jaminan TERAKHIR di
	// level database supaya tidak ada duplikat baru lolos lewat jalur
	// manapun (termasuk jalur UPDATE/map yang tidak tersentuh hook model
	// models.Jabatan.BeforeSave, lihat komentar CrudConfig.BeforeUpdatePayload
	// di handlers/crud_generic.go), bukan cuma mengandalkan validasi di kode
	// Go. HARUS dibuat SETELAH mergeJabatanDuplikat di atas selesai
	// membereskan duplikat lama -- index ini gagal dibuat kalau masih ada
	// duplikat tersisa di tabel. IF NOT EXISTS -- aman dijalankan berkali-kali.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_jabatan_upper_unik ON jabatan (UPPER(jabatan))`).Error; err != nil {
		log.Printf("peringatan: gagal membuat uniqueIndex case-insensitive idx_jabatan_upper_unik (kemungkinan masih ada duplikat tersisa): %v", err)
	}

	// Fitur "Jam Kerja Khusus" per Unit Kerja/Sekolah sudah dihapus dari UI
	// (menu Master Data -> Unit Kerja) atas permintaan pengguna, karena
	// jam kerja sekarang cukup diatur lewat 2 jendela waktu global di menu
	// Rekap Absen -> Pengaturan (Dinas/Kantor & Sekolah). Supaya data lama
	// yang mungkin masih tersimpan di beberapa unit kerja tidak diam-diam
	// tetap aktif dan membingungkan, semua kolom jam khusus ini dikosongkan
	// setiap kali migrasi dijalankan. Kolom & fungsi baca di
	// handlers/absensi.go (jamAbsenUntukPegawai) SENGAJA tetap dibiarkan ada
	// untuk kompatibilitas, tapi karena kolomnya selalu NULL, tier "jam
	// khusus per unit" itu tidak akan pernah terpakai lagi.
	if err := db.Model(&models.UnitKerja{}).
		Where("jam_mulai_pagi IS NOT NULL OR jam_batas_pagi IS NOT NULL OR jam_tutup_pagi IS NOT NULL OR jam_mulai_pulang IS NOT NULL OR jam_tutup_pulang IS NOT NULL").
		Updates(map[string]interface{}{
			"jam_mulai_pagi":   nil,
			"jam_batas_pagi":   nil,
			"jam_tutup_pagi":   nil,
			"jam_mulai_pulang": nil,
			"jam_tutup_pulang": nil,
		}).Error; err != nil {
		log.Printf("peringatan: gagal mengosongkan jam kerja khusus unit_kerja lama: %v", err)
	}

	// Menu "Berita Acara" sekarang HANYA menampilkan baris AbsensiDokumen yang
	// dibuat administrator/admin LANGSUNG lewat menu itu (lihat listBeritaAcara
	// di handlers/berita_acara.go & komentar models.AbsensiDokumen.
	// IDPengajuanBeritaAcara) -- baris yang berasal dari persetujuan tahap
	// akhir pengajuan mandiri Berita Acara Sekolah disaring keluar lewat
	// kolom id_pengajuan_berita_acara. Kolom ini baru mulai diisi MULAI
	// SEKARANG oleh setujuiPengajuanBeritaAcaraAdmin -- baris historis yang
	// sudah lebih dulu disetujui lewat alur itu (sebelum perubahan ini)
	// masih NULL, sehingga tanpa backfill berikut, baris-baris lama itu akan
	// SALAH tetap muncul di menu "Berita Acara". Dicocokkan lewat id_pegawai
	// + tanggal (AbsensiDokumen.Tanggal == PengajuanBeritaAcara.
	// TanggalKejadian) pada pengajuan yang statusnya "disetujui" -- aman
	// dijalankan berkali-kali (hanya menyentuh baris yang masih NULL).
	if err := db.Exec(`
		UPDATE absensi_dokumen ad
		SET id_pengajuan_berita_acara = pba.id
		FROM pengajuan_berita_acara pba
		WHERE ad.jenis = ?
		  AND ad.id_pengajuan_berita_acara IS NULL
		  AND pba.status = 'disetujui'
		  AND pba.id_pegawai = ad.id_pegawai
		  AND pba.tanggal_kejadian = ad.tanggal
	`, models.AbsensiDokumenBeritaAcara).Error; err != nil {
		log.Printf("peringatan: gagal backfill absensi_dokumen.id_pengajuan_berita_acara: %v", err)
	}

	// Lanjutan dari backfill id_pengajuan_berita_acara di atas: menu "Berita
	// Acara" SEKARANG disaring lewat kolom diinput_langsung_menu_berita_acara
	// (lihat komentarnya di models.go) yang baru mulai diisi MULAI SEKARANG
	// oleh buatBeritaAcara -- bukan lagi hanya lewat id_pengajuan_berita_acara
	// IS NULL, karena ternyata ADA jalur lain yang juga bisa menghasilkan
	// baris jenis "berita_acara" tapi BUKAN dari menu Berita Acara: input
	// manual admin/admin absen lewat Rekap Absen -> "Input Surat Kolektif"
	// (inputAbsensiDokumenKolektif), maupun persetujuan admin verifikasi atas
	// pengajuan mandiri "Surat Kolektif" pegawai yang jenisnya kebetulan
	// Berita Acara (setujuiPengajuanSuratKolektif) -- baris dari jalur-jalur
	// ini tetap salah tampil di menu "Berita Acara" kalau hanya disaring
	// lewat id_pengajuan_berita_acara IS NULL. Baris historis (sebelum
	// kolom ini ada) di-backfill dengan heuristik nama berkas:
	// buatBeritaAcara (DAN HANYA buatBeritaAcara/alur pengajuan Berita Acara
	// Sekolah mandiri, yang sudah disaring lewat id_pengajuan_berita_acara
	// IS NULL di atas) SATU-SATUNYA yang menamai berkasnya otomatis lewat
	// generateBeritaAcaraNamaFile dengan pola tetap
	// "berita_acara_YYYYMMDD_<angka>.pdf" -- sedangkan inputAbsensiDokumenKolektif
	// & setujuiPengajuanSuratKolektif SELALU memakai nama berkas ASLI yang
	// diupload admin/pegawai, yang hampir pasti TIDAK mengikuti pola ini.
	// Aman dijalankan berkali-kali (hanya menyentuh baris yang masih cocok).
	if err := db.Exec(`
		UPDATE absensi_dokumen
		SET diinput_langsung_menu_berita_acara = true
		WHERE jenis = ?
		  AND id_pengajuan_berita_acara IS NULL
		  AND diinput_langsung_menu_berita_acara = false
		  AND nama_file ~ '^berita_acara_[0-9]{8}_[0-9]+\.pdf$'
	`, models.AbsensiDokumenBeritaAcara).Error; err != nil {
		log.Printf("peringatan: gagal backfill absensi_dokumen.diinput_langsung_menu_berita_acara: %v", err)
	}

	// Alur persetujuan Kenaikan Pangkat berubah (permintaan pengguna, lihat
	// komentar lengkap di handlers/peta_jabatan_kenaikan_pangkat.go): tahap
	// "menunggu_admin" (administrator menyetujui lagi SEBELUM pegawai boleh
	// upload SK) dihapus -- begitu atasan/Kepala Sekolah menyetujui,
	// pengajuan LANGSUNG "disetujui" & baris PerubahanJabatanPegawai
	// (menunggu_upload) langsung dibuat. Baris PengajuanKenaikanPangkat lama
	// yang terlanjur tersimpan dengan status "menunggu_admin" (dibuat
	// SEBELUM perubahan ini dideploy) perlu dimajukan otomatis di sini,
	// supaya tidak ada pengajuan pegawai yang macet tidak bisa diproses lagi
	// gara-gara endpoint setujui-admin/kembalikan-admin sudah tidak ada.
	// Aman dijalankan berkali-kali -- begitu tidak ada lagi baris berstatus
	// "menunggu_admin", blok ini tidak melakukan apa-apa.
	var pendingKP []models.PengajuanKenaikanPangkat
	if err := db.Where("status = ?", "menunggu_admin").Find(&pendingKP).Error; err != nil {
		log.Printf("peringatan: gagal cek pengajuan_kenaikan_pangkat lama berstatus menunggu_admin: %v", err)
	}
	for _, kp := range pendingKP {
		now := time.Now()
		if err := db.Model(&models.PengajuanKenaikanPangkat{}).Where("id = ?", kp.ID).
			Updates(map[string]interface{}{"status": models.KenaikanPangkatDisetujui}).Error; err != nil {
			log.Printf("peringatan: gagal memajukan pengajuan_kenaikan_pangkat id=%d ke status disetujui: %v", kp.ID, err)
			continue
		}
		var sudahAda int64
		db.Model(&models.PerubahanJabatanPegawai{}).Where("id_pengajuan_kenaikan_pangkat = ?", kp.ID).Count(&sudahAda)
		if sudahAda > 0 {
			continue
		}
		perubahan := models.PerubahanJabatanPegawai{
			IDPengajuanKenaikanPangkat: kp.ID,
			IDPegawai:                  kp.IDPegawai,
			IDJabatanBaru:              kp.IDJabatanTujuan,
			IDSubJabatanBaru:           kp.IDSubJabatanTujuan,
			Status:                     models.PerubahanJabatanMenungguUpload,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		}
		if err := db.Create(&perubahan).Error; err != nil {
			log.Printf("peringatan: gagal membuat perubahan_jabatan_pegawai backfill untuk pengajuan_kenaikan_pangkat id=%d: %v", kp.ID, err)
		}
	}
	if len(pendingKP) > 0 {
		log.Printf("migrasi: %d pengajuan kenaikan pangkat lama (menunggu_admin) dimajukan ke disetujui + perubahan jabatan menunggu_upload", len(pendingKP))
	}

	// Struktur awal "Peta Jabatan Dinas" (Sekretariat/Bidang/Seksi/Sub
	// Bagian + Jabatan Pelaksana/Fungsional-nya, ditranskripsi dari berkas
	// ANJAB) -- lihat SeedPetaJabatanDinas di seed_peta_jabatan_dinas.go.
	// Idempotent: dilewati total begitu root-nya sudah ada.
	SeedPetaJabatanDinas(db)

	log.Println("migrasi database berhasil")
}

// mergeJabatanDuplikat menggabungkan baris models.Jabatan yang "kembar" --
// namanya SAMA PERSIS kalau besar/kecil hurufnya diabaikan (mis. "Penelaah
// Teknis Kebijakan" vs "PENELAAH TEKNIS KEBIJAKAN") -- jadi SATU baris saja,
// lalu menyeragamkan SEMUA nama jabatan (termasuk yang tidak duplikat) ke
// HURUF KAPITAL. Permintaan pengguna: "pada kolom jabatan mengapa bisa ada
// inputan yang sama... tolong diperbaiki agar jabatan tdk duplikat pakai yg
// menggunakan huruf kapital". Baris dengan ID PALING KECIL pada tiap grup
// duplikat yang dipertahankan (anggapannya: baris paling lama/pertama kali
// dibuat); baris lain di grup yang sama digabungkan ke situ.
//
// SEMUA tabel yang mengacu ke Jabatan.ID dialihkan dulu ke ID yang
// dipertahankan SEBELUM baris duplikatnya dihapus, supaya tidak ada acuan
// yatim piatu: pegawai.id_jabatan, shift_kerja_jabatan.id_jabatan,
// sub_jabatan.id_jabatan, formasi_jabatan.id_jabatan. Tabel sub_jabatan &
// formasi_jabatan punya uniqueIndex gabungan (unit kerja + jabatan [+
// nama/sub_jabatan]) -- kalau baris duplikat yang dialihkan ternyata
// SUDAH ADA kembarannya persis di ID yang dipertahankan (unit kerja &
// kolom pembeda lainnya sama), UPDATE-nya SENGAJA dilewati (guard NOT
// EXISTS) supaya tidak mentok constraint itu, lalu baris yang gagal
// dialihkan itu dihapus saja (bukan acuan yatim piatu -- kembarannya pada
// ID yang dipertahankan sudah mewakili kombinasi yang sama).
//
// Aman dijalankan berkali-kali -- begitu semua grup sudah digabung &
// semua nama sudah huruf kapital, fungsi ini tidak menemukan apa-apa lagi
// untuk dikerjakan. Dipanggil SETELAH uniqueIndex lama
// shift_kerja_jabatan.id_jabatan dilonggarkan (lihat pemanggil di atas).
func mergeJabatanDuplikat(db *gorm.DB) {
	var rows []models.Jabatan
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		log.Printf("peringatan: gagal membaca data jabatan untuk migrasi duplikat: %v", err)
		return
	}

	groups := map[string][]models.Jabatan{}
	var order []string
	for _, j := range rows {
		key := strings.ToUpper(strings.TrimSpace(j.Jabatan))
		if key == "" {
			continue
		}
		if _, ada := groups[key]; !ada {
			order = append(order, key)
		}
		groups[key] = append(groups[key], j)
	}

	digabung := 0
	for _, upper := range order {
		group := groups[upper]
		keep := group[0] // ID paling kecil (rows sudah "id asc")

		if keep.Jabatan != upper {
			// db.Exec (SQL mentah) SENGAJA dipakai di sini, BUKAN
			// db.Model(&models.Jabatan{}).Update(...) -- yang terakhir itu
			// akan memicu hook models.Jabatan.BeforeSave di atas lewat
			// struct Dest yang KOSONG (ID=0, Jabatan=""), yang salah
			// mengira baris ini baru/belum ada ID-nya (cek pengecualian diri
			// sendiri ikut terlewat) DAN masih bisa menabrak baris
			// duplikatnya sendiri yang BELUM SEMPAT dihapus (urutan
			// penggabungan ada di bawah) -- db.Exec melewati seluruh
			// callback/hook GORM sama sekali, aman dipakai di migrasi massal
			// seperti ini.
			if err := db.Exec(`UPDATE jabatan SET jabatan = ? WHERE id = ?`, upper, keep.ID).Error; err != nil {
				log.Printf("peringatan: gagal menyeragamkan huruf kapital jabatan id=%d: %v", keep.ID, err)
			}
		}

		for _, dupe := range group[1:] {
			db.Exec(`UPDATE pegawai SET id_jabatan = ? WHERE id_jabatan = ?`, keep.ID, dupe.ID)

			db.Exec(`
				UPDATE shift_kerja_jabatan SET id_jabatan = ? WHERE id_jabatan = ?
				AND NOT EXISTS (
					SELECT 1 FROM shift_kerja_jabatan s2
					WHERE s2.id_shift = shift_kerja_jabatan.id_shift AND s2.id_jabatan = ?
				)
			`, keep.ID, dupe.ID, keep.ID)
			db.Exec(`DELETE FROM shift_kerja_jabatan WHERE id_jabatan = ?`, dupe.ID)

			db.Exec(`
				UPDATE sub_jabatan SET id_jabatan = ? WHERE id_jabatan = ?
				AND NOT EXISTS (
					SELECT 1 FROM sub_jabatan s2
					WHERE s2.id_unit_kerja = sub_jabatan.id_unit_kerja
					  AND s2.id_jabatan = ? AND s2.nama = sub_jabatan.nama
				)
			`, keep.ID, dupe.ID, keep.ID)
			db.Exec(`DELETE FROM sub_jabatan WHERE id_jabatan = ?`, dupe.ID)

			db.Exec(`
				UPDATE formasi_jabatan SET id_jabatan = ? WHERE id_jabatan = ?
				AND NOT EXISTS (
					SELECT 1 FROM formasi_jabatan f2
					WHERE f2.id_unit_kerja = formasi_jabatan.id_unit_kerja
					  AND f2.id_jabatan = ?
					  AND f2.id_sub_jabatan IS NOT DISTINCT FROM formasi_jabatan.id_sub_jabatan
				)
			`, keep.ID, dupe.ID, keep.ID)
			db.Exec(`DELETE FROM formasi_jabatan WHERE id_jabatan = ?`, dupe.ID)

			if err := db.Exec(`DELETE FROM jabatan WHERE id = ?`, dupe.ID).Error; err != nil {
				log.Printf("peringatan: gagal menghapus jabatan duplikat id=%d ('%s') setelah digabung ke id=%d: %v", dupe.ID, dupe.Jabatan, keep.ID, err)
				continue
			}
			digabung++
			log.Printf("migrasi: jabatan duplikat id=%d ('%s') digabung ke id=%d ('%s')", dupe.ID, dupe.Jabatan, keep.ID, upper)
		}
	}
	if digabung > 0 {
		log.Printf("migrasi: %d baris jabatan duplikat berhasil digabung & nama jabatan diseragamkan ke huruf kapital", digabung)
	}
}
