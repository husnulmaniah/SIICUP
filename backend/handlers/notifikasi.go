package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// RegisterNotifikasiRoutes menyediakan endpoint untuk tombol lonceng notifikasi
// di header (khusus administrator & admin/Admin Kepegawaian) -- menghitung &
// menampilkan daftar ringkas Pengajuan Cuti dan Perubahan Data Pegawai yang
// masih berstatus "pending" (baru masuk, belum diproses), supaya administrator
// dan admin kepegawaian langsung tahu ada pengajuan baru tanpa harus bolak-
// balik membuka menu Pengajuan Cuti / Perubahan Data Pegawai secara manual.
// Lihat frontend/src/layouts/AppLayout.vue (tombol lonceng + badge) dan
// frontend/src/api/notifikasi.js untuk sisi frontend.
func RegisterNotifikasiRoutes(mux *http.ServeMux, db *gorm.DB) {
	manage := middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
		pendingNotifikasi(w, r, db)
	}, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("administrator", "admin"))
	mux.Handle("GET /api/notifikasi/pending", manage)
}

// notifikasiItem: satu baris ringkas untuk daftar di panel lonceng notifikasi.
type notifikasiItem struct {
	Type       string    `json:"type"` // "pengajuan_cuti" atau "perubahan_data"
	ID         uint      `json:"id"`
	Nama       string    `json:"nama"`
	Keterangan string    `json:"keterangan"`
	CreatedAt  time.Time `json:"created_at"`
	Link       string    `json:"link"`
}

// maxNotifikasiItems: batas jumlah baris terbaru yang dikirim ke panel lonceng
// (badge tetap menampilkan TOTAL yang sesungguhnya, bukan dibatasi jumlah ini).
const maxNotifikasiItems = 15

func pendingNotifikasi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var cuti []models.PengajuanCuti
	db.Preload("Pegawai").Preload("JenisCuti").
		Where("status = ?", models.StatusPending).
		Order("created_at desc").Limit(maxNotifikasiItems).Find(&cuti)

	var perubahan []models.PerubahanDataPegawai
	db.Preload("Pegawai").
		Where("status = ?", models.StatusPending).
		Order("created_at desc").Limit(maxNotifikasiItems).Find(&perubahan)

	var pensiun []models.PengajuanPensiun
	db.Preload("Pegawai").
		Where("status = ?", models.StatusPending).
		Order("created_at desc").Limit(maxNotifikasiItems).Find(&pensiun)

	var totalCuti, totalPerubahan, totalPensiun int64
	db.Model(&models.PengajuanCuti{}).Where("status = ?", models.StatusPending).Count(&totalCuti)
	db.Model(&models.PerubahanDataPegawai{}).Where("status = ?", models.StatusPending).Count(&totalPerubahan)
	db.Model(&models.PengajuanPensiun{}).Where("status = ?", models.StatusPending).Count(&totalPensiun)

	items := make([]notifikasiItem, 0, len(cuti)+len(perubahan)+len(pensiun))
	for _, c := range cuti {
		nama := "Pegawai"
		if c.Pegawai != nil {
			nama = c.Pegawai.Nama
		}
		jenis := "Cuti"
		if c.JenisCuti != nil {
			jenis = c.JenisCuti.Jenis
		}
		items = append(items, notifikasiItem{
			Type:       "pengajuan_cuti",
			ID:         c.ID,
			Nama:       nama,
			Keterangan: jenis + " • " + strconv.Itoa(c.JumlahHari) + " hari",
			CreatedAt:  c.CreatedAt,
			Link:       "/pengajuan-cuti",
		})
	}
	for _, p := range perubahan {
		nama := "Pegawai"
		if p.Pegawai != nil {
			nama = p.Pegawai.Nama
		}
		items = append(items, notifikasiItem{
			Type:       "perubahan_data",
			ID:         p.ID,
			Nama:       nama,
			Keterangan: "Pengajuan perubahan data pegawai",
			CreatedAt:  p.CreatedAt,
			Link:       "/perubahan-data",
		})
	}

	for _, p := range pensiun {
		nama := "Pegawai"
		if p.Pegawai != nil {
			nama = p.Pegawai.Nama
		}
		keterangan := "Pengajuan pensiun"
		if p.IsPensiunDini {
			keterangan = "Pengajuan pensiun dini"
		}
		items = append(items, notifikasiItem{
			Type:       "pengajuan_pensiun",
			ID:         p.ID,
			Nama:       nama,
			Keterangan: keterangan,
			CreatedAt:  p.CreatedAt,
			Link:       "/pengajuan-pensiun",
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > maxNotifikasiItems {
		items = items[:maxNotifikasiItems]
	}

	utils.Success(w, "ok", map[string]interface{}{
		"total":                   totalCuti + totalPerubahan + totalPensiun,
		"pengajuan_cuti_count":    totalCuti,
		"perubahan_data_count":    totalPerubahan,
		"pengajuan_pensiun_count": totalPensiun,
		"items":                   items,
	})
}
