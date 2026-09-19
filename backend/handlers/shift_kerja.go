package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// shift_kerja.go implements menu Master Data -> Shift Kerja: administrator
// membuat shift kerja (nama, kategori, jam per hari) dan memasangnya ke SATU
// ATAU LEBIH Unit Kerja -- begitu terpasang, SEMUA pegawai yang tempat
// kerjanya salah satu unit tsb otomatis mengikuti jam & jendela kamera
// absen shift ini tanpa perlu diatur satu per satu (lihat komentar
// models.ShiftKerja, models.ShiftKerjaUnitKerja & jamAbsenUntukPegawai di
// handlers/absensi.go). Satu unit kerja tetap hanya boleh dipasangi SATU
// shift -- lihat validasiShiftKerjaPayload.

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
	return db.Preload("HariList", func(d *gorm.DB) *gorm.DB { return d.Order("hari asc") })
}

// isiUnitKerjaList mengisi field UnitKerjaList (gorm:"-", tidak ikut
// di-preload otomatis) untuk satu atau banyak ShiftKerja sekaligus lewat
// tabel penghubung shift_kerja_unit_kerja -- dilakukan manual (bukan lewat
// asosiasi many2many bawaan GORM) supaya tabel penghubung bisa diberi
// constraint unik non-standar (unik per IDUnitKerja, bukan per pasangan).
func isiUnitKerjaList(db *gorm.DB, items []*models.ShiftKerja) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uint, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	var joins []models.ShiftKerjaUnitKerja
	if err := db.Where("id_shift IN ?", ids).Find(&joins).Error; err != nil {
		return err
	}
	if len(joins) == 0 {
		return nil
	}
	unitIDs := make([]uint, 0, len(joins))
	for _, j := range joins {
		unitIDs = append(unitIDs, j.IDUnitKerja)
	}
	var units []models.UnitKerja
	if err := db.Where("id IN ?", unitIDs).Find(&units).Error; err != nil {
		return err
	}
	unitByID := map[uint]models.UnitKerja{}
	for _, u := range units {
		unitByID[u.ID] = u
	}
	unitIDsByShift := map[uint][]uint{}
	for _, j := range joins {
		unitIDsByShift[j.IDShift] = append(unitIDsByShift[j.IDShift], j.IDUnitKerja)
	}
	for _, it := range items {
		list := make([]models.UnitKerja, 0)
		for _, uid := range unitIDsByShift[it.ID] {
			if u, ok := unitByID[uid]; ok {
				list = append(list, u)
			}
		}
		sort.Slice(list, func(a, b int) bool { return list[a].Unit < list[b].Unit })
		it.UnitKerjaList = list
	}
	return nil
}

func listShiftKerja(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var items []models.ShiftKerja
	if err := shiftKerjaPreload(db).Order("nama_shift asc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	ptrs := make([]*models.ShiftKerja, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}
	if err := isiUnitKerjaList(db, ptrs); err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data unit kerja shift")
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
	if err := isiUnitKerjaList(db, []*models.ShiftKerja{&item}); err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data unit kerja shift")
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
	NamaShift       string                  `json:"nama_shift"`
	KategoriShift   string                  `json:"kategori_shift"`
	IDUnitKerjaList []uint                  `json:"id_unit_kerja_list"`
	HariList        []shiftKerjaHariPayload `json:"hari_list"`
}

// validasiShiftKerjaPayload memvalidasi payload create/update: nama &
// kategori wajib diisi (kategori harus salah satu dari
// models.ShiftKategoriPilihan), minimal satu unit kerja wajib dipilih & tiap
// unit kerja yang dipilih wajib ada & belum dipasangi shift LAIN (satu unit
// kerja hanya boleh punya SATU shift -- lihat komentar
// models.ShiftKerjaUnitKerja), hari_list wajib mencakup ketujuh hari (0-6)
// masing-masing tepat satu kali, dan untuk hari yang aktif (bukan libur)
// kelima jam wajib diisi format HH:MM dengan urutan yang masuk akal --
// persis seperti validasi PengaturanAbsensi pada updatePengaturanAbsensi di
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
	if len(p.IDUnitKerjaList) == 0 {
		return fmt.Errorf("minimal satu unit kerja wajib dipilih")
	}
	idSeen := map[uint]bool{}
	idUnik := make([]uint, 0, len(p.IDUnitKerjaList))
	for _, id := range p.IDUnitKerjaList {
		if id == 0 || idSeen[id] {
			continue
		}
		idSeen[id] = true
		idUnik = append(idUnik, id)
	}
	var unitList []models.UnitKerja
	if err := db.Where("id IN ?", idUnik).Find(&unitList).Error; err != nil {
		return fmt.Errorf("gagal memeriksa unit kerja")
	}
	if len(unitList) != len(idUnik) {
		return fmt.Errorf("ada unit kerja yang tidak ditemukan")
	}
	unitByID := map[uint]models.UnitKerja{}
	for _, u := range unitList {
		unitByID[u.ID] = u
	}
	var dupeJoins []models.ShiftKerjaUnitKerja
	dupeQuery := db.Where("id_unit_kerja IN ?", idUnik)
	if excludeID > 0 {
		dupeQuery = dupeQuery.Where("id_shift <> ?", excludeID)
	}
	if err := dupeQuery.Find(&dupeJoins).Error; err != nil {
		return fmt.Errorf("gagal memeriksa unit kerja")
	}
	if len(dupeJoins) > 0 {
		bentrok := dupeJoins[0]
		var shiftLain models.ShiftKerja
		db.First(&shiftLain, bentrok.IDShift)
		namaUnit := unitByID[bentrok.IDUnitKerja].Unit
		return fmt.Errorf("unit kerja '%s' sudah dipasangi shift lain ('%s') -- satu unit kerja hanya boleh punya satu shift, ubah/hapus shift yang lama dulu", namaUnit, shiftLain.NamaShift)
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

// unitKerjaJoinDariPayload membuang duplikat id (kalau ada) supaya tidak
// melanggar uniqueIndex di ShiftKerjaUnitKerja.IDUnitKerja saat insert.
func unitKerjaJoinDariPayload(idShift uint, idList []uint) []models.ShiftKerjaUnitKerja {
	seen := map[uint]bool{}
	out := make([]models.ShiftKerjaUnitKerja, 0, len(idList))
	for _, id := range idList {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, models.ShiftKerjaUnitKerja{IDShift: idShift, IDUnitKerja: id})
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
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		joinList := unitKerjaJoinDariPayload(item.ID, p.IDUnitKerjaList)
		if err := tx.Create(&joinList).Error; err != nil {
			return err
		}
		hariList := hariListDariPayload(item.ID, p.HariList)
		return tx.Create(&hariList).Error
	})
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan shift kerja: "+err.Error())
		return
	}
	shiftKerjaPreload(db).First(&item, item.ID)
	isiUnitKerjaList(db, []*models.ShiftKerja{&item})
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
		}).Error; err != nil {
			return err
		}
		// ganti seluruh baris hari & pasangan unit kerja lama dengan yang
		// baru -- lebih sederhana & aman dibanding mencocokkan baris mana
		// yang berubah satu per satu, dan jumlahnya selalu kecil.
		if err := tx.Where("id_shift = ?", item.ID).Delete(&models.ShiftKerjaHari{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id_shift = ?", item.ID).Delete(&models.ShiftKerjaUnitKerja{}).Error; err != nil {
			return err
		}
		joinList := unitKerjaJoinDariPayload(item.ID, p.IDUnitKerjaList)
		if err := tx.Create(&joinList).Error; err != nil {
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
	isiUnitKerjaList(db, []*models.ShiftKerja{&item})
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
		if err := tx.Where("id_shift = ?", item.ID).Delete(&models.ShiftKerjaUnitKerja{}).Error; err != nil {
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
