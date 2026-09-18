package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// tpp.go menangani menu "Penerima TPP" (khusus administrator & admin):
// daftar pegawai yang SUDAH DITAMBAHKAN ke kategori penerima TPP (BUKAN
// seluruh pegawai -- lihat models.PenerimaTpp), beserta tombol "Kirim
// Permintaan SK" untuk anggota yang SK Terakhir-nya masih kosong.
// Permintaan yang dikirim muncul sebagai notifikasi pada dashboard pegawai
// ybs (lihat models.PermintaanSk, dashboardHandler case "pegawai" di
// dashboard.go).

func RegisterTppRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }

	mux.Handle("GET /api/tpp/penerima", manage(func(w http.ResponseWriter, r *http.Request) { listPenerimaTpp(w, r, db) }))
	mux.Handle("POST /api/tpp/penerima", manage(func(w http.ResponseWriter, r *http.Request) { tambahPenerimaTpp(w, r, db) }))
	mux.Handle("POST /api/tpp/penerima/by-kriteria", manage(func(w http.ResponseWriter, r *http.Request) { tambahPenerimaTppKriteria(w, r, db) }))
	mux.Handle("GET /api/tpp/tahun-tmt", manage(func(w http.ResponseWriter, r *http.Request) { listTahunTmtPegawai(w, r, db) }))
	mux.Handle("DELETE /api/tpp/penerima/{id}", manage(func(w http.ResponseWriter, r *http.Request) { hapusPenerimaTpp(w, r, db) }))
	mux.Handle("GET /api/tpp/calon-pegawai", manage(func(w http.ResponseWriter, r *http.Request) { listCalonPegawaiTpp(w, r, db) }))

	mux.Handle("POST /api/tpp/permintaan-sk", manage(func(w http.ResponseWriter, r *http.Request) { kirimPermintaanSk(w, r, db) }))
	mux.Handle("GET /api/pengaturan-tpp", manage(func(w http.ResponseWriter, r *http.Request) { getPengaturanTpp(w, r, db) }))

	// upload-sk-saya: dipakai PEGAWAI SENDIRI dari notifikasi di dashboard-nya
	// untuk mengupload SK Terakhir tanpa perlu lewat alur persetujuan
	// Perubahan Data Pegawai -- lihat izinnya langsung di dalam handler
	// (pakai claims.IDPegawai, bukan role).
	mux.Handle("POST /api/tpp/upload-sk-saya", anyRole(func(w http.ResponseWriter, r *http.Request) { uploadSkTerakhirSaya(w, r, db) }))
}

// penerimaTppPreload memuat relasi Pegawai TANPA kolom byte dokumennya
// (SkTerakhirFile dll) supaya daftar Penerima TPP tidak ikut menarik isi
// berkas yang bisa besar, cukup nama filenya saja (dipakai menentukan
// kosong/tidaknya SK Terakhir).
func penerimaTppPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Pegawai", func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }).
		Preload("Pegawai.Jabatan").Preload("Pegawai.UnitKerja")
}

// penerimaTppItem adalah satu baris pada tabel menu Penerima TPP.
type penerimaTppItem struct {
	// ID adalah ID baris models.PenerimaTpp (dipakai untuk aksi hapus),
	// BUKAN ID pegawai -- ID pegawai ada di field terpisah IDPegawai.
	ID             uint   `json:"id"`
	IDPegawai      uint   `json:"id_pegawai"`
	NIP            string `json:"nip"`
	Nama           string `json:"nama"`
	Jabatan        string `json:"jabatan"`
	UnitKerja      string `json:"unit_kerja"`
	SkTerakhirNama string `json:"sk_terakhir_nama"`
	// PermintaanSk berisi permintaan SK yang MASIH "menunggu" untuk pegawai
	// ini (kalau ada) supaya kolom aksi di frontend bisa menampilkan
	// "Menunggu upload (batas dd-mm-yyyy)" dan tidak mengizinkan pegawai yang
	// sama diminta dua kali sekaligus.
	PermintaanSk *models.PermintaanSk `json:"permintaan_sk,omitempty"`
}

// listPenerimaTpp menangani GET /api/tpp/penerima?q=..&page=..&pageSize=..
// &hanya_kosong=1 -- daftar pegawai yang SUDAH menjadi anggota Penerima TPP
// (Aktif=true pada models.PenerimaTpp), dilengkapi permintaan SK yang masih
// menunggu (kalau ada) untuk masing-masing baris.
func listPenerimaTpp(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 500 {
		pageSize = 25
	}
	search := strings.TrimSpace(q.Get("q"))
	hanyaKosong := q.Get("hanya_kosong") == "1"

	base := db.Model(&models.PenerimaTpp{}).
		Joins("JOIN pegawai ON pegawai.id = penerima_tpp.id_pegawai").
		Where("penerima_tpp.aktif = ?", true)
	kosongBase := db.Model(&models.PenerimaTpp{}).
		Joins("JOIN pegawai ON pegawai.id = penerima_tpp.id_pegawai").
		Where("penerima_tpp.aktif = ? AND (pegawai.sk_terakhir_nama = '' OR pegawai.sk_terakhir_nama IS NULL)", true)

	if search != "" {
		cond := "pegawai.nama ILIKE ? OR pegawai.nip ILIKE ?"
		args := []interface{}{"%" + search + "%", "%" + search + "%"}
		base = base.Where(cond, args...)
		kosongBase = kosongBase.Where(cond, args...)
	}
	if hanyaKosong {
		base = base.Where("pegawai.sk_terakhir_nama = '' OR pegawai.sk_terakhir_nama IS NULL")
	}

	var total, kosongCount int64
	base.Count(&total)
	kosongBase.Count(&kosongCount)

	var anggota []models.PenerimaTpp
	if err := penerimaTppPreload(base).
		Order("pegawai.nama asc").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&anggota).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data Penerima TPP")
		return
	}

	// permintaan SK yang masih menunggu untuk pegawai-pegawai pada halaman
	// ini saja, diambil sekali (bukan query per-baris di dalam loop).
	ids := make([]uint, 0, len(anggota))
	for _, a := range anggota {
		ids = append(ids, a.IDPegawai)
	}
	pendingByPegawai := map[uint]models.PermintaanSk{}
	if len(ids) > 0 {
		var pending []models.PermintaanSk
		db.Where("id_pegawai IN ? AND status = ?", ids, models.StatusPermintaanSkMenunggu).Find(&pending)
		for _, p := range pending {
			pendingByPegawai[p.IDPegawai] = p
		}
	}

	items := make([]penerimaTppItem, 0, len(anggota))
	for _, a := range anggota {
		item := penerimaTppItem{ID: a.ID, IDPegawai: a.IDPegawai}
		if a.Pegawai != nil {
			item.NIP = a.Pegawai.NIP
			item.Nama = a.Pegawai.Nama
			item.SkTerakhirNama = a.Pegawai.SkTerakhirNama
			if a.Pegawai.Jabatan != nil {
				item.Jabatan = a.Pegawai.Jabatan.Jabatan
			}
			if a.Pegawai.UnitKerja != nil {
				item.UnitKerja = a.Pegawai.UnitKerja.Unit
			}
		}
		if pending, ada := pendingByPegawai[a.IDPegawai]; ada {
			item.PermintaanSk = &pending
		}
		items = append(items, item)
	}

	utils.SuccessMeta(w, "ok", items, map[string]interface{}{
		"page":         page,
		"pageSize":     pageSize,
		"total":        total,
		"kosong_count": kosongCount,
	})
}

// listCalonPegawaiTpp menangani GET /api/tpp/calon-pegawai?q=..&pageSize=..
// -- daftar pegawai yang BELUM menjadi anggota aktif Penerima TPP, dipakai
// mengisi pilihan MultiSelect pada dialog "Tambah Pegawai" di frontend
// (pola pencarian server-side yang sama seperti filter pegawai pada Rekap
// Absen).
func listCalonPegawaiTpp(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	q := r.URL.Query()
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 100
	}
	search := strings.TrimSpace(q.Get("q"))

	sudahAnggota := db.Model(&models.PenerimaTpp{}).Where("aktif = ?", true).Select("id_pegawai")

	query := db.Model(&models.Pegawai{}).Omit(dokumenFileFields...).
		Preload("Jabatan").Preload("UnitKerja").
		Where("id NOT IN (?)", sudahAnggota)
	if search != "" {
		query = query.Where("nama ILIKE ? OR nip ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var items []models.Pegawai
	if err := query.Order("nama asc").Limit(pageSize).Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}
	utils.Success(w, "ok", items)
}

type tambahPenerimaTppPayload struct {
	IDPegawai []uint `json:"id_pegawai"`
}

// tambahPenerimaTpp menangani POST /api/tpp/penerima -- menambahkan satu
// atau beberapa pegawai terpilih (dari dialog "Tambah Pegawai", pilih nama
// langsung) sebagai anggota BARU Penerima TPP.
func tambahPenerimaTpp(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p tambahPenerimaTppPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if len(p.IDPegawai) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu pegawai")
		return
	}
	claims, _ := middleware.GetClaims(r)
	ditambahkan := tambahkanKePenerimaTpp(db, p.IDPegawai, claims)
	if ditambahkan == 0 {
		utils.Error(w, http.StatusBadRequest, "pegawai terpilih sudah menjadi anggota Penerima TPP")
		return
	}
	utils.Success(w, strconv.Itoa(ditambahkan)+" pegawai berhasil ditambahkan ke Penerima TPP", map[string]interface{}{"ditambahkan": ditambahkan})
}

type tambahPenerimaTppKriteriaPayload struct {
	IDJabatan         []uint `json:"id_jabatan"`         // WAJIB, minimal 1 -- boleh beberapa jabatan sekaligus
	TahunPengangkatan []int  `json:"tahun_pengangkatan"` // kosong = tidak difilter tahun, cocokkan seluruh tahun TMT; kalau diisi, boleh beberapa tahun sekaligus
}

// tambahPenerimaTppKriteria menangani POST /api/tpp/penerima/by-kriteria --
// menambahkan SEKALIGUS semua pegawai dengan salah satu Jabatan terpilih
// (WAJIB, boleh pilih beberapa jabatan) dan, kalau diisi, salah satu tahun
// TMT ("tahun pengangkatan") terpilih (boleh pilih beberapa tahun -- lihat
// listTahunTmtPegawai untuk daftar tahun yang tersedia dari data pegawai),
// sebagai anggota Penerima TPP. Pegawai yang sudah menjadi anggota aktif
// dilewati begitu saja (tidak dobel).
func tambahPenerimaTppKriteria(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p tambahPenerimaTppKriteriaPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if len(p.IDJabatan) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu jabatan terlebih dahulu")
		return
	}
	for _, tahun := range p.TahunPengangkatan {
		if tahun < 1950 || tahun > time.Now().Year()+1 {
			utils.Error(w, http.StatusBadRequest, "tahun pengangkatan tidak valid")
			return
		}
	}

	query := db.Model(&models.Pegawai{}).Where("id_jabatan IN ?", p.IDJabatan)
	if len(p.TahunPengangkatan) > 0 {
		query = query.Where("tmt IS NOT NULL AND EXTRACT(YEAR FROM tmt) IN ?", p.TahunPengangkatan)
	}
	var calon []models.Pegawai
	if err := query.Find(&calon).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}
	if len(calon) == 0 {
		utils.Error(w, http.StatusNotFound, "tidak ada pegawai yang cocok dengan jabatan/tahun pengangkatan tersebut")
		return
	}
	ids := make([]uint, 0, len(calon))
	for _, c := range calon {
		ids = append(ids, c.ID)
	}

	claims, _ := middleware.GetClaims(r)
	ditambahkan := tambahkanKePenerimaTpp(db, ids, claims)
	if ditambahkan == 0 {
		utils.Error(w, http.StatusBadRequest, strconv.Itoa(len(calon))+" pegawai cocok dengan kriteria ini, tapi semuanya sudah menjadi anggota Penerima TPP")
		return
	}
	dilewati := len(calon) - ditambahkan
	msg := strconv.Itoa(ditambahkan) + " pegawai berhasil ditambahkan ke Penerima TPP"
	if dilewati > 0 {
		msg += " (" + strconv.Itoa(dilewati) + " lainnya dilewati karena sudah menjadi anggota)"
	}
	utils.Success(w, msg, map[string]interface{}{"ditambahkan": ditambahkan, "dilewati": dilewati})
}

// listTahunTmtPegawai menangani GET /api/tpp/tahun-tmt -- daftar tahun TMT
// ("tahun pengangkatan") yang BENAR-BENAR ADA pada data pegawai saat ini
// (bukan rentang tahun bebas), diurutkan terbaru dulu. Dipakai untuk mengisi
// pilihan tahun pada dialog "Tambah dari Jabatan & Tahun" di menu Penerima
// TPP, supaya administrator memilih dari tahun yang memang tersedia alih-
// alih mengetik tahun bebas yang mungkin tidak cocok dengan data siapa pun.
func listTahunTmtPegawai(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var tahun []int
	if err := db.Raw(`SELECT DISTINCT EXTRACT(YEAR FROM tmt)::int AS tahun FROM pegawai WHERE tmt IS NOT NULL ORDER BY tahun DESC`).Scan(&tahun).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil daftar tahun TMT")
		return
	}
	if tahun == nil {
		tahun = []int{}
	}
	utils.Success(w, "ok", tahun)
}

// tambahkanKePenerimaTpp adalah inti penambahan anggota Penerima TPP yang
// dipakai bersama oleh tambahPenerimaTpp (pilih nama manual) &
// tambahPenerimaTppKriteria (bulk lewat Jabatan/tahun) -- melewati begitu
// saja pegawai yang sudah menjadi anggota AKTIF (tidak membuat baris dobel),
// TAPI mengaktifkan kembali (Aktif=true) baris yang sebelumnya pernah
// dikeluarkan (Aktif=false) alih-alih membuat baris baru, supaya
// riwayat/timestamp lama tidak hilang begitu saja saat pegawai yang sama
// ditambahkan lagi di kemudian hari.
func tambahkanKePenerimaTpp(db *gorm.DB, idPegawaiList []uint, claims *utils.Claims) int {
	var idPengirim *uint
	if claims != nil {
		userID := claims.UserID
		idPengirim = &userID
	}

	ditambahkan := 0
	for _, idPegawai := range idPegawaiList {
		var existing models.PenerimaTpp
		err := db.Where("id_pegawai = ?", idPegawai).Order("id desc").First(&existing).Error
		if err == nil {
			if existing.Aktif {
				continue // sudah anggota aktif, lewati
			}
			// pernah jadi anggota, sempat dikeluarkan -- aktifkan lagi.
			existing.Aktif = true
			existing.AlasanNonaktif = ""
			existing.TglNonaktif = nil
			existing.IDDitambahkanOleh = idPengirim
			db.Save(&existing)
			ditambahkan++
			continue
		}
		baru := models.PenerimaTpp{IDPegawai: idPegawai, Aktif: true, IDDitambahkanOleh: idPengirim}
		if err := db.Create(&baru).Error; err == nil {
			ditambahkan++
		}
	}
	return ditambahkan
}

type hapusPenerimaTppPayload struct {
	Alasan string `json:"alasan"`
}

// hapusPenerimaTpp menangani DELETE /api/tpp/penerima/{id} -- mengeluarkan
// SATU pegawai dari daftar Penerima TPP (mis. karena pensiun/mutasi). {id}
// adalah ID baris models.PenerimaTpp (BUKAN ID pegawai). Baris tidak
// dihapus permanen (soft-delete lewat Aktif=false) supaya riwayat kapan &
// kenapa seorang pegawai dikeluarkan tetap tersimpan.
func hapusPenerimaTpp(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.PenerimaTpp
	if err := db.Where("aktif = ?", true).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan atau sudah dikeluarkan sebelumnya")
		return
	}
	var p hapusPenerimaTppPayload
	_ = json.NewDecoder(r.Body).Decode(&p) // body boleh kosong -- alasan opsional

	now := time.Now()
	item.Aktif = false
	item.AlasanNonaktif = strings.TrimSpace(p.Alasan)
	item.TglNonaktif = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengeluarkan pegawai dari Penerima TPP")
		return
	}
	utils.Success(w, "pegawai berhasil dikeluarkan dari Penerima TPP", nil)
}

// getPengaturanTpp menangani GET /api/pengaturan-tpp -- selalu berhasil,
// membuat baris ID=1 dengan batas tanggal default (30 hari dari sekarang)
// kalau belum pernah diisi sebelumnya.
func getPengaturanTpp(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var item models.PengaturanTpp
	if err := db.FirstOrCreate(&item, models.PengaturanTpp{ID: 1}).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil pengaturan TPP")
		return
	}
	if item.BatasTanggalUpload.IsZero() {
		item.BatasTanggalUpload = time.Now().AddDate(0, 0, 30)
		db.Model(&item).Update("batas_tanggal_upload", item.BatasTanggalUpload)
	}
	utils.Success(w, "ok", item)
}

type kirimPermintaanSkPayload struct {
	IDPegawai    []uint `json:"id_pegawai"`
	BatasTanggal string `json:"batas_tanggal"` // format "2006-01-02"
}

// kirimPermintaanSk menangani POST /api/tpp/permintaan-sk -- dipanggil
// administrator/admin saat klik tombol "Kirim Permintaan SK" untuk satu
// atau beberapa anggota Penerima TPP terpilih sekaligus. BOLEH dikirim ke
// pegawai yang SK Terakhir-nya SUDAH ADA sekalipun -- dipakai untuk minta
// pegawai memeriksa ulang/mengganti SK yang sudah terupload kalau ternyata
// kurang sesuai (lihat dashboardHandler case "pegawai" & DashboardView.vue,
// yang membedakan tampilan notifikasinya berdasarkan ada/tidaknya SK saat
// ini). Batas tanggal yang dikirim SEKALIGUS disimpan sebagai PengaturanTpp
// (dipakai sebagai nilai default saat mengirim permintaan berikutnya).
func kirimPermintaanSk(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p kirimPermintaanSkPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if len(p.IDPegawai) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu pegawai")
		return
	}
	batas, err := time.Parse("2006-01-02", strings.TrimSpace(p.BatasTanggal))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "batas tanggal tidak valid")
		return
	}

	claims, _ := middleware.GetClaims(r)
	var idPengirim *uint
	if claims != nil {
		userID := claims.UserID
		idPengirim = &userID
	}

	// simpan sebagai pengaturan default berikutnya
	var pengaturan models.PengaturanTpp
	if err := db.FirstOrCreate(&pengaturan, models.PengaturanTpp{ID: 1}).Error; err == nil {
		db.Model(&pengaturan).Update("batas_tanggal_upload", batas)
	}

	terkirim := 0
	for _, idPegawai := range p.IDPegawai {
		var pegawai models.Pegawai
		if err := db.First(&pegawai, "id = ?", idPegawai).Error; err != nil {
			continue // pegawai tidak ditemukan, lewati
		}
		// SENGAJA tidak lagi melewati pegawai yang SK Terakhir-nya sudah ada
		// -- administrator boleh mengirim permintaan ke pegawai ini juga,
		// mis. untuk minta diperiksa ulang/diganti kalau SK yang ada kurang
		// sesuai (lihat komentar fungsi ini).

		var existing models.PermintaanSk
		found := db.Where("id_pegawai = ? AND status = ?", idPegawai, models.StatusPermintaanSkMenunggu).First(&existing).Error == nil
		if found {
			// permintaan sebelumnya masih menunggu -- perbarui saja batas
			// tanggalnya & siapa yang terakhir mengirim, tidak buat baris baru.
			existing.BatasTanggal = batas
			existing.IDDikirimOleh = idPengirim
			db.Save(&existing)
		} else {
			baru := models.PermintaanSk{
				IDPegawai:     idPegawai,
				BatasTanggal:  batas,
				Status:        models.StatusPermintaanSkMenunggu,
				IDDikirimOleh: idPengirim,
			}
			db.Create(&baru)
		}
		terkirim++
	}

	if terkirim == 0 {
		utils.Error(w, http.StatusBadRequest, "tidak ada permintaan yang dikirim -- data pegawai terpilih tidak ditemukan")
		return
	}
	utils.Success(w, "permintaan SK berhasil dikirim ke "+strconv.Itoa(terkirim)+" pegawai", map[string]interface{}{"terkirim": terkirim})
}

// fulfillPermintaanSk menandai SELESAI ("terpenuhi") setiap PermintaanSk
// yang masih "menunggu" milik satu pegawai -- dipanggil begitu SK
// Terakhir-nya berhasil diupload, baik lewat uploadDokumenPegawai
// (administrator/admin mengupload atas nama pegawai) maupun
// uploadSkTerakhirSaya (pegawai sendiri, lewat notifikasi dashboard-nya).
func fulfillPermintaanSk(db *gorm.DB, idPegawai uint) {
	now := time.Now()
	db.Model(&models.PermintaanSk{}).
		Where("id_pegawai = ? AND status = ?", idPegawai, models.StatusPermintaanSkMenunggu).
		Updates(map[string]interface{}{"status": models.StatusPermintaanSkTerpenuhi, "dipenuhi_pada": now})
}

// uploadSkTerakhirSaya menangani POST /api/tpp/upload-sk-saya -- pegawai
// mengupload SK Terakhir milik akun yang sedang login sendiri (dari
// notifikasi permintaan SK di dashboard-nya), TANPA lewat alur persetujuan
// Perubahan Data Pegawai. Sengaja dibatasi hanya untuk kolom "SK Terakhir"
// (bukan SK KGB/Pangkat/Pensiun lain, yang tetap harus lewat administrator).
func uploadSkTerakhirSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims == nil || claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun ini tidak terhubung ke data pegawai")
		return
	}
	var item models.Pegawai
	if err := db.First(&item, "id = ?", *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca file upload (maksimal 15MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "file tidak ditemukan (field 'file')")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "format file harus PDF, JPG, atau PNG")
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membaca isi file")
		return
	}

	if err := db.Model(&item).Updates(map[string]interface{}{
		"sk_terakhir_nama": header.Filename,
		"sk_terakhir_file": data,
	}).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan dokumen: "+err.Error())
		return
	}
	fulfillPermintaanSk(db, item.ID)
	utils.Success(w, "SK Terakhir berhasil diupload", map[string]string{"nama_file": header.Filename})
}
