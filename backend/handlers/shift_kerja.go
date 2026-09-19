package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// shift_kerja.go implements menu Master Data -> Shift Kerja: administrator
// membuat shift kerja (nama, kategori, jam per hari) dan memasangnya ke SATU
// Unit Kerja -- begitu terpasang, SEMUA pegawai yang tempat kerjanya unit
// tsb otomatis mengikuti jam & jendela kamera absen shift ini tanpa perlu
// diatur satu per satu (lihat komentar models.ShiftKerja & jamAbsenUntukPegawai
// di handlers/absensi.go).

func RegisterShiftKerjaRoutes(mux *http.ServeMux, db *gorm.DB) {
	// Sama seperti menu master data referensi inti lain (Jabatan, Unit
	// Kerja, dst di master_routes.go) -- hanya administrator yang boleh
	// mengelola, supaya konsisten dengan pembagian wewenang yang sudah ada.
	manage := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("administrator"))
	}

	mux.Handle("GET /api/shift-kerja", manage(func(w http.ResponseWriter, r *http.Request) { listShiftKerja(w, r, db) }))
	mux.Handle("GET /api/shift-kerja/{id}", manage(func(w http.ResponseWriter, r *http.Request) { getShiftKerja(w, r, db) }))
	mux.Handle("POST /api/shift-kerja", manage(func(w http.ResponseWriter, r *http.Request) { createShiftKerja(w, r, db) }))
	mux.Handle("PUT /api/shift-kerja/{id}", manage(func(w http.ResponseWriter, r *http.Request) { updateShiftKerja(w, r, db) }))
	mux.Handle("DELETE /api/shift-kerja/{id}", manage(func(w http.ResponseWriter, r *http.Request) { deleteShiftKerja(w, r, db) }))
}

func shiftKerjaPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("UnitKerja").Preload("HariList", func(d *gorm.DB) *gorm.DB { return d.Order("hari asc") })
}

func listShiftKerja(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var items []models.ShiftKerja
	if err := shiftKerjaPreload(db).Order("nama_shift asc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.Success(w, "ok", items)
}

func getShiftKerja(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.ShiftKerja
	if err := shiftKerjaPreload(db).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	utils.Success(w, "ok", item)
}

// namaHariIndo memetakan int hari (urutan time.Weekday: 0=Minggu..6=Sabtu)
// ke nama hari dalam Bahasa Indonesia, dipakai pada pesan error validasi.
var namaHariIndo = map[int]string{
	0: "Minggu", 1: "Senin", 2: "Selasa", 3: "Rabu", 4: "Kamis", 5: "Jumat", 6: "Sabtu",
}

type shiftKerjaHariPayload struct {
	Hari                int    `json:"hari"`
	Aktif               bool   `json:"aktif"`
	JamMulaiPagi        string `json:"jam_mulai_pagi"`
	JamBatasPagi        string `json:"jam_batas_pagi"`
	JamTutupPagi        string `json:"jam_tutup_pagi"`
	JamIstirahatMulai   string `json:"jam_istirahat_mulai"`
	JamIstirahatSelesai string `json:"jam_istirahat_selesai"`
	JamMulaiPulang      string `json:"jam_mulai_pulang"`
	JamTutupPulang      string `json:"jam_tutup_pulang"`
}

type shiftKerjaPayload struct {
	NamaShift     string                  `json:"nama_shift"`
	KategoriShift string                  `json:"kategori_shift"`
	IDUnitKerja   uint                    `json:"id_unit_kerja"`
	HariList      []shiftKerjaHariPayload `json:"hari_list"`
}

// validasiShiftKerjaPayload memvalidasi payload create/update: nama &
// kategori wajib diisi (kategori harus salah satu dari
// models.ShiftKategoriPilihan), unit kerja wajib ada & belum dipasangi shift
// lain (satu unit kerja hanya boleh punya SATU shift -- lihat komentar
// models.ShiftKerja), hari_list wajib mencakup ketujuh hari (0-6) masing-
// masing tepat satu kali, dan untuk hari yang aktif (bukan libur) kelima jam
// wajib diisi format HH:MM dengan urutan yang masuk akal -- persis seperti
// validasi PengaturanAbsensi pada updatePengaturanAbsensi di
// absensi_admin.go, supaya perilakunya konsisten & familiar bagi
// administrator yang sudah terbiasa dengan menu Pengaturan Absen.
func validasiShiftKerjaPayload(db *gorm.DB, p shiftKerjaPayload, excludeID uint) error {
	if strings.TrimSpace(p.NamaShift) == "" {
		return fmt.Errorf("nama shift wajib diisi")
	}
	kategoriValid := false
	for _, k := range models.ShiftKategoriPilihan {
		if k == p.KategoriShift {
			kategoriValid = true
			break
		}
	}
	if !kategoriValid {
		return fmt.Errorf("kategori shift tidak valid")
	}
	if p.IDUnitKerja == 0 {
		return fmt.Errorf("unit kerja wajib dipilih")
	}
	var uk models.UnitKerja
	if err := db.First(&uk, p.IDUnitKerja).Error; err != nil {
		return fmt.Errorf("unit kerja tidak ditemukan")
	}
	var dupe models.ShiftKerja
	dupeQuery := db.Where("id_unit_kerja = ?", p.IDUnitKerja)
	if excludeID > 0 {
		dupeQuery = dupeQuery.Where("id <> ?", excludeID)
	}
	if err := dupeQuery.First(&dupe).Error; err == nil {
		return fmt.Errorf("unit kerja '%s' sudah dipasangi shift lain ('%s') -- satu unit kerja hanya boleh punya satu shift, ubah/hapus shift yang lama dulu", uk.Unit, dupe.NamaShift)
	}
	if len(p.HariList) != 7 {
		return fmt.Errorf("ketentuan jam kerja wajib diisi untuk ketujuh hari (Minggu s.d Sabtu)")
	}
	seen := map[int]bool{}
	for _, h := range p.HariList {
		if h.Hari < 0 || h.Hari > 6 {
			return fmt.Errorf("hari tidak valid")
		}
		if seen[h.Hari] {
			return fmt.Errorf("hari %s dobel diisi", namaHariIndo[h.Hari])
		}
		seen[h.Hari] = true
		if !h.Aktif {
			// hari libur -- jam tidak divalidasi/dipakai sama sekali.
			continue
		}
		label := namaHariIndo[h.Hari]
		for fieldLabel, v := range map[string]string{
			"jam mulai absen pagi":   h.JamMulaiPagi,
			"jam batas absen pagi":   h.JamBatasPagi,
			"jam tutup absen pagi":   h.JamTutupPagi,
			"jam mulai absen pulang": h.JamMulaiPulang,
			"jam tutup absen pulang": h.JamTutupPulang,
		} {
			if _, ok := parseJamToMinutes(v); !ok {
				return fmt.Errorf("%s hari %s tidak valid, gunakan format HH:MM", fieldLabel, label)
			}
		}
		mulaiMin, _ := parseJamToMinutes(h.JamMulaiPagi)
		batasMin, _ := parseJamToMinutes(h.JamBatasPagi)
		tutupMin, _ := parseJamToMinutes(h.JamTutupPagi)
		mulaiPulangMin, _ := parseJamToMinutes(h.JamMulaiPulang)
		tutupPulangMin, _ := parseJamToMinutes(h.JamTutupPulang)
		if batasMin <= mulaiMin {
			return fmt.Errorf("hari %s: jam batas absen pagi harus lebih besar dari jam mulai absen pagi", label)
		}
		if tutupMin <= batasMin {
			return fmt.Errorf("hari %s: jam tutup absen pagi (batas absen masuk otomatis ditutup) harus lebih besar dari jam batas absen pagi", label)
		}
		if tutupPulangMin <= mulaiPulangMin {
			return fmt.Errorf("hari %s: jam tutup absen pulang (batas absen pulang otomatis ditutup) harus lebih besar dari jam mulai absen pulang", label)
		}
		// istirahat SEPENUHNYA opsional & murni informasi jadwal (tidak ikut
		// menggerbang kamera) -- kalau salah satu diisi, keduanya wajib
		// diisi & urut, tapi boleh saja berada di luar jendela masuk/pulang.
		imFilled := strings.TrimSpace(h.JamIstirahatMulai) != ""
		isFilled := strings.TrimSpace(h.JamIstirahatSelesai) != ""
		if imFilled != isFilled {
			return fmt.Errorf("hari %s: jam istirahat mulai & selesai harus diisi berdua atau dikosongkan berdua", label)
		}
		if imFilled {
			im, imOk := parseJamToMinutes(h.JamIstirahatMulai)
			is, isOk := parseJamToMinutes(h.JamIstirahatSelesai)
			if !imOk || !isOk {
				return fmt.Errorf("hari %s: jam istirahat tidak valid, gunakan format HH:MM", label)
			}
			if is <= im {
				return fmt.Errorf("hari %s: jam istirahat selesai harus lebih besar dari jam istirahat mulai", label)
			}
		}
	}
	return nil
}

func hariListDariPayload(idShift uint, list []shiftKerjaHariPayload) []models.ShiftKerjaHari {
	out := make([]models.ShiftKerjaHari, 0, len(list))
	for _, h := range list {
		out = append(out, models.ShiftKerjaHari{
			IDShift:             idShift,
			Hari:                h.Hari,
			Aktif:               h.Aktif,
			JamMulaiPagi:        h.JamMulaiPagi,
			JamBatasPagi:        h.JamBatasPagi,
			JamTutupPagi:        h.JamTutupPagi,
			JamIstirahatMulai:   h.JamIstirahatMulai,
			JamIstirahatSelesai: h.JamIstirahatSelesai,
			JamMulaiPulang:      h.JamMulaiPulang,
			JamTutupPulang:      h.JamTutupPulang,
		})
	}
	return out
}

func createShiftKerja(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p shiftKerjaPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := validasiShiftKerjaPayload(db, p, 0); err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	item := models.ShiftKerja{
		NamaShift:     strings.TrimSpace(p.NamaShift),
		KategoriShift: p.KategoriShift,
		IDUnitKerja:   p.IDUnitKerja,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan shift kerja: "+err.Error())
		return
	}
	hariList := hariListDariPayload(item.ID, p.HariList)
	if err := db.Create(&hariList).Error; err != nil {
		db.Delete(&item)
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan ketentuan jam kerja: "+err.Error())
		return
	}
	shiftKerjaPreload(db).First(&item, item.ID)
	utils.Created(w, "shift kerja berhasil ditambahkan", item)
}

func updateShiftKerja(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.ShiftKerja
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	var p shiftKerjaPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := validasiShiftKerjaPayload(db, p, item.ID); err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&item).Updates(map[string]interface{}{
			"nama_shift":     strings.TrimSpace(p.NamaShift),
			"kategori_shift": p.KategoriShift,
			"id_unit_kerja":  p.IDUnitKerja,
		}).Error; err != nil {
			return err
		}
		// ganti seluruh baris hari lama dengan yang baru -- lebih sederhana
		// & aman dibanding mencocokkan baris mana yang berubah satu per
		// satu, dan jumlahnya selalu kecil (tepat 7 baris per shift).
		if err := tx.Where("id_shift = ?", item.ID).Delete(&models.ShiftKerjaHari{}).Error; err != nil {
			return err
		}
		hariList := hariListDariPayload(item.ID, p.HariList)
		return tx.Create(&hariList).Error
	})
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memperbarui shift kerja: "+err.Error())
		return
	}
	shiftKerjaPreload(db).First(&item, "id = ?", id)
	utils.Success(w, "shift kerja berhasil diperbarui", item)
}

func deleteShiftKerja(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.ShiftKerja
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id_shift = ?", item.ID).Delete(&models.ShiftKerjaHari{}).Error; err != nil {
			return err
		}
		return tx.Delete(&item).Error
	})
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus shift kerja: "+err.Error())
		return
	}
	utils.Success(w, "shift kerja berhasil dihapus -- pegawai di unit kerja terkait otomatis kembali memakai jam kerja lama (jam unit kerja atau default sekolah/dinas)", nil)
}
