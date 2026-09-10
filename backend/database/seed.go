package database

import (
	"log"
	"time"

	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// Seed inserts a minimal working demo dataset (roles, one account per role,
// a small amount of reference/master data) ONLY the first time the app runs
// against an empty database, so it is safe to call on every startup.
func Seed(db *gorm.DB) {
	var roleCount int64
	db.Model(&models.Role{}).Count(&roleCount)
	if roleCount > 0 {
		log.Println("data sudah ada, lewati seeding")
		return
	}
	log.Println("database kosong, menjalankan seeding data awal...")

	roles := []models.Role{{Role: "administrator"}, {Role: "admin"}, {Role: "pegawai"}, {Role: "atasan"}}
	db.Create(&roles)
	roleByName := map[string]uint{}
	for _, r := range roles {
		roleByName[r.Role] = r.ID
	}

	statuses := []models.Status{{Status: "Aktif"}, {Status: "Nonaktif"}}
	db.Create(&statuses)

	jabatans := []models.Jabatan{{Jabatan: "Kepala Dinas"}, {Jabatan: "Kepala Bidang"}, {Jabatan: "Staff"}}
	db.Create(&jabatans)

	units := []models.UnitKerja{{Unit: "Sekretariat"}, {Unit: "Bidang Pelayanan"}}
	db.Create(&units)

	pangkats := []models.Pangkat{{Pangkat: "Pembina"}, {Pangkat: "Penata Muda"}}
	db.Create(&pangkats)

	golongans := []models.Golongan{{Gol: "IV/a"}, {Gol: "III/a"}}
	db.Create(&golongans)

	pangkatGols := []models.PangkatGol{
		{IDPangkat: pangkats[0].ID, IDGol: golongans[0].ID},
		{IDPangkat: pangkats[1].ID, IDGol: golongans[1].ID},
	}
	db.Create(&pangkatGols)

	jenisCutis := []models.JenisCuti{
		{Jenis: "Cuti Tahunan", DefaultJatah: 12, Keterangan: "Jatah cuti tahunan reguler, dipotong dari kuota tahunan"},
		{Jenis: "Cuti Sakit", DefaultJatah: 0, Keterangan: "Tidak memotong kuota cuti tahunan"},
		{Jenis: "Cuti Melahirkan", DefaultJatah: 0, Keterangan: "Tidak memotong kuota cuti tahunan"},
		{Jenis: "Cuti Besar", DefaultJatah: 0, Keterangan: "Tidak memotong kuota cuti tahunan"},
	}
	db.Create(&jenisCutis)

	polas := []models.PolaHariKerja{
		{Pola: "5 Hari Kerja (Senin-Jumat)"},
		{Pola: "6 Hari Kerja (Senin-Sabtu)"},
	}
	db.Create(&polas)

	year := time.Now().Year()
	holidays := []models.TglMerah{
		{Tgl: time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), Keterangan: "Tahun Baru Masehi"},
		{Tgl: time.Date(year, 8, 17, 0, 0, 0, 0, time.UTC), Keterangan: "Hari Kemerdekaan RI"},
		{Tgl: time.Date(year, 12, 25, 0, 0, 0, 0, time.UTC), Keterangan: "Hari Raya Natal"},
	}
	db.Create(&holidays)

	statusAktif := statuses[0].ID
	jabatanKabid := jabatans[1].ID
	jabatanStaff := jabatans[2].ID
	unitID := units[1].ID
	pgAtasan := pangkatGols[0].ID
	pgStaff := pangkatGols[1].ID

	atasan := models.Pegawai{
		NIP: "197001011995011001", Nama: "Andi Wijaya",
		IDJabatan: &jabatanKabid, IDUnitKerja: &unitID, IDPangkatGol: &pgAtasan,
		IDStatus: &statusAktif, TempatTgs: "Kantor Pusat", NoHP: "081200000001",
		Email: "andi.wijaya@instansi.go.id",
	}
	db.Create(&atasan)

	staff := models.Pegawai{
		NIP: "199001012015012002", Nama: "Siti Rahma",
		IDJabatan: &jabatanStaff, IDUnitKerja: &unitID, IDPangkatGol: &pgStaff,
		IDStatus: &statusAktif, IDAtasan: &atasan.ID, TempatTgs: "Kantor Pusat", NoHP: "081200000002",
		Email: "siti.rahma@instansi.go.id",
	}
	db.Create(&staff)

	defaultPass, _ := utils.HashPassword("admin123")

	users := []models.User{
		{Username: "administrator", Pass: defaultPass, Nama: "Administrator Sistem", IDRole: roleByName["administrator"]},
		{Username: "admin", Pass: defaultPass, Nama: "Admin Kepegawaian", IDRole: roleByName["admin"]},
		{Username: "andi", Pass: defaultPass, Nama: "Andi Wijaya", IDRole: roleByName["atasan"], IDPegawai: &atasan.ID},
		{Username: "siti", Pass: defaultPass, Nama: "Siti Rahma", IDRole: roleByName["pegawai"], IDPegawai: &staff.ID},
	}
	db.Create(&users)

	db.Create(&models.JatahCuti{IDPegawai: staff.ID, Tahun: year, JumlahHari: 12, Terpakai: 0})
	db.Create(&models.JatahCuti{IDPegawai: atasan.ID, Tahun: year, JumlahHari: 12, Terpakai: 0})

	log.Println("seeding selesai. akun default (password semua: admin123):")
	log.Println("  administrator / admin123")
	log.Println("  admin         / admin123")
	log.Println("  andi (atasan) / admin123")
	log.Println("  siti (pegawai)/ admin123")
}

// EnsureStatusPegawai melengkapi data master Status (status kepegawaian)
// dengan kategori PNS/PPPK/PPPK Paruh Waktu yang dipakai untuk memfilter
// export data pegawai, tanpa menghapus/mengubah nilai yang sudah ada
// (misalnya "Aktif"/"Nonaktif"). Idempotent, dijalankan setiap server start.
func EnsureStatusPegawai(db *gorm.DB) {
	wanted := []string{"PNS", "PPPK", "PPPK Paruh Waktu"}
	for _, nama := range wanted {
		var existing models.Status
		if err := db.Where("status ILIKE ?", nama).First(&existing).Error; err == nil {
			continue
		}
		if err := db.Create(&models.Status{Status: nama}).Error; err != nil {
			log.Printf("gagal menambahkan status pegawai '%s': %v", nama, err)
		} else {
			log.Printf("status pegawai '%s' ditambahkan", nama)
		}
	}
}

// EnsureJenisCuti melengkapi data master Jenis Cuti dengan jenis-jenis yang
// dibutuhkan alur kelengkapan berkas (Cuti Tahunan Umroh, Cuti Alasan
// Penting), tanpa mengganggu database yang sudah berjalan. Dijalankan setiap
// kali server start (bukan hanya saat database masih kosong seperti Seed()),
// dan idempotent: hanya menambahkan jenis yang namanya belum ada.
func EnsureJenisCuti(db *gorm.DB) {
	wanted := []models.JenisCuti{
		{Jenis: "Cuti Tahunan", DefaultJatah: 12, Keterangan: "Cuti tahunan reguler, wajib SK terakhir & rekomendasi kepala sekolah bila diajukan sendiri"},
		{Jenis: "Cuti Tahunan Umroh", DefaultJatah: 12, Keterangan: "Cuti tahunan untuk keperluan umroh, wajib SK terakhir, rekomendasi kepala sekolah & surat keterangan travel"},
		{Jenis: "Cuti Sakit", DefaultJatah: 0, Keterangan: "Tidak memotong kuota cuti tahunan, wajib SK terakhir, surat rujukan & surat keterangan rawat inap"},
		{Jenis: "Cuti Melahirkan", DefaultJatah: 0, Keterangan: "Tidak memotong kuota cuti tahunan, wajib rekomendasi kepala sekolah, SK terakhir, surat keterangan HPL & buku KIA"},
		{Jenis: "Cuti Alasan Penting", DefaultJatah: 0, Keterangan: "Tidak memotong kuota cuti tahunan, wajib SK terakhir, rekomendasi kepala sekolah & dokumen pendukung"},
	}
	for _, jc := range wanted {
		var existing models.JenisCuti
		err := db.Where("jenis ILIKE ?", jc.Jenis).First(&existing).Error
		if err == nil {
			continue // sudah ada, jangan diubah (mungkin sudah disesuaikan admin)
		}
		if err := db.Create(&jc).Error; err != nil {
			log.Printf("gagal menambahkan jenis cuti '%s': %v", jc.Jenis, err)
		} else {
			log.Printf("jenis cuti '%s' ditambahkan", jc.Jenis)
		}
	}
}

// EnsurePengaturanSurat memastikan selalu ada tepat satu baris pengaturan
// (ID=1) berisi data Kepala Dinas yang dicetak sebagai penandatangan pada
// formulir cetak (Surat Rekomendasi & Formulir Permintaan/Pemberian Cuti).
// Idempotent -- hanya membuat baris default sekali; setelah itu nilainya
// hanya diubah lewat halaman Pengaturan Formulir (admin/administrator).
func EnsurePengaturanSurat(db *gorm.DB) {
	var count int64
	db.Model(&models.PengaturanSurat{}).Count(&count)
	if count > 0 {
		return
	}
	if err := db.Create(&models.PengaturanSurat{
		ID:              1,
		NamaKepalaDinas: "MOH. RIDWAN DM. S.Ag",
		NipKepalaDinas:  "19740111 199803 1 004",
	}).Error; err != nil {
		log.Printf("gagal membuat pengaturan surat default: %v", err)
	} else {
		log.Println("pengaturan surat default dibuat (bisa diubah dari menu Pengaturan Formulir)")
	}
}
