package handlers

import (
	"net/http"
	"strconv"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// notifikasi_pegawai.go: notifikasi untuk PEGAWAI YANG MENGAJUKAN begitu
// pengajuannya ditolak/dikembalikan -- lihat komentar panjang pada
// models.NotifikasiPegawai untuk perbedaannya dengan notifikasi lonceng
// administrator/admin di handlers/notifikasi.go, & 4 titik pemicu
// buatNotifikasiPegawai di bawah ini. Dipakai AppLayout.vue (lonceng kedua,
// khusus pegawai/atasan) lewat endpoint di RegisterNotifikasiPegawaiRoutes.

// buatNotifikasiPegawai menyimpan satu baris NotifikasiPegawai baru --
// dipanggil dari rejectPengajuan (handlers/pengajuan_cuti.go),
// kembalikanPengajuanSuratKolektif (handlers/pengajuan_surat_kolektif.go),
// tolakPengajuanBeritaAcaraAdmin (handlers/pengajuan_berita_acara.go), &
// rejectPerubahanData (handlers/perubahan_data.go). Kegagalan simpan
// notifikasi SENGAJA hanya diabaikan (tidak menggagalkan respons utama
// penolakan/pengembalian pengajuan) -- notifikasi ini bersifat pelengkap,
// bukan bagian inti dari alur persetujuan.
func buatNotifikasiPegawai(db *gorm.DB, idPegawai uint, jenis string, refID uint, judul, pesan, link string) {
	if idPegawai == 0 {
		return
	}
	notif := models.NotifikasiPegawai{
		IDPegawai: idPegawai,
		Jenis:     jenis,
		RefID:     refID,
		Judul:     judul,
		Pesan:     pesan,
		Link:      link,
	}
	db.Create(&notif)
}

// RegisterNotifikasiPegawaiRoutes menyediakan endpoint untuk lonceng
// notifikasi KHUSUS pegawai/atasan (BEDA dari /api/notifikasi/pending yang
// khusus administrator/admin, lihat handlers/notifikasi.go).
func RegisterNotifikasiPegawaiRoutes(mux *http.ServeMux, db *gorm.DB) {
	milikSendiri := middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
		notifikasiPegawaiPending(w, r, db)
	}, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("pegawai", "atasan"))
	mux.Handle("GET /api/notifikasi-pegawai/pending", milikSendiri)

	bacaSatu := middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
		bacaNotifikasiPegawai(w, r, db)
	}, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("pegawai", "atasan"))
	mux.Handle("PUT /api/notifikasi-pegawai/{id}/baca", bacaSatu)

	bacaSemua := middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
		bacaSemuaNotifikasiPegawai(w, r, db)
	}, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("pegawai", "atasan"))
	mux.Handle("PUT /api/notifikasi-pegawai/baca-semua", bacaSemua)
}

// maxNotifikasiPegawaiItems: batas jumlah baris terbaru yang dikirim ke
// panel lonceng -- sama pola dengan maxNotifikasiItems di
// handlers/notifikasi.go. Badge tetap menampilkan TOTAL belum dibaca yang
// sesungguhnya, bukan dibatasi jumlah ini.
const maxNotifikasiPegawaiItems = 20

func notifikasiPegawaiPending(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims == nil || claims.IDPegawai == nil {
		utils.Success(w, "ok", map[string]interface{}{"total_belum_dibaca": 0, "items": []models.NotifikasiPegawai{}})
		return
	}
	var items []models.NotifikasiPegawai
	db.Where("id_pegawai = ?", *claims.IDPegawai).
		Order("created_at desc").Limit(maxNotifikasiPegawaiItems).Find(&items)

	var totalBelumDibaca int64
	db.Model(&models.NotifikasiPegawai{}).
		Where("id_pegawai = ? AND dibaca = ?", *claims.IDPegawai, false).
		Count(&totalBelumDibaca)

	utils.Success(w, "ok", map[string]interface{}{
		"total_belum_dibaca": totalBelumDibaca,
		"items":              items,
	})
}

func bacaNotifikasiPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims == nil || claims.IDPegawai == nil {
		utils.Error(w, http.StatusForbidden, "akun ini tidak terhubung ke data pegawai")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	var item models.NotifikasiPegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "notifikasi tidak ditemukan")
		return
	}
	// hanya pemilik notifikasi (pegawai yang bersangkutan) yang boleh
	// menandainya dibaca -- mencegah pegawai lain menebak-nebak id dan
	// menandai dibaca notifikasi milik orang lain.
	if item.IDPegawai != *claims.IDPegawai {
		utils.Error(w, http.StatusForbidden, "notifikasi ini bukan milik Anda")
		return
	}
	if !item.Dibaca {
		db.Model(&item).Update("dibaca", true)
	}
	utils.Success(w, "ditandai dibaca", nil)
}

func bacaSemuaNotifikasiPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims == nil || claims.IDPegawai == nil {
		utils.Error(w, http.StatusForbidden, "akun ini tidak terhubung ke data pegawai")
		return
	}
	db.Model(&models.NotifikasiPegawai{}).
		Where("id_pegawai = ? AND dibaca = ?", *claims.IDPegawai, false).
		Update("dibaca", true)
	utils.Success(w, "semua notifikasi ditandai dibaca", nil)
}
