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
		&models.UnitKerja{},
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
	)
	if err != nil {
		log.Fatalf("gagal migrasi database: %v", err)
	}
	log.Println("migrasi database berhasil")
}
