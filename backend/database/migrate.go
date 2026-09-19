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
	)
	if err != nil {
		log.Fatalf("gagal migrasi database: %v", err)
	}

	// Seed 4 jenis surat kolektif bawaan (sekali saja, kalau tabel masih
	// kosong) -- slug-nya disamakan dengan konstanta AbsensiDokumen* di
	// models.go supaya data lama yang sudah tersimpan di kolom
	// absensi_dokumen.jenis tetap valid & tetap cocok dengan baris ini.
	var jenisSuratCount int64
	db.Model(&models.JenisSurat{}).Count(&jenisSuratCount)
	if jenisSuratCount == 0 {
		defaultJenisSurat := []models.JenisSurat{
			{Slug: models.AbsensiDokumenSuratTugas, Nama: "Surat Tugas", Kode: "DD"},
			{Slug: models.AbsensiDokumenBeritaAcara, Nama: "Berita Acara", Kode: "DD"},
			{Slug: models.AbsensiDokumenSuratIzin, Nama: "Surat Izin", Kode: "I"},
			{Slug: models.AbsensiDokumenSKS, Nama: "SKS -- Surat Keterangan Sakit", Kode: "S"},
		}
		if err := db.Create(&defaultJenisSurat).Error; err != nil {
			log.Printf("peringatan: gagal seed jenis_surat bawaan: %v", err)
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

	log.Println("migrasi database berhasil")
}
