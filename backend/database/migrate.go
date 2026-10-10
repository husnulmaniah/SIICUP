package database

import (
	"log"

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

	log.Println("migrasi database berhasil")
}
