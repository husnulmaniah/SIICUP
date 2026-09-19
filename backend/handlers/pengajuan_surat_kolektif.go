package handlers

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// pengajuan_surat_kolektif.go menangani pengajuan Surat Kolektif MANDIRI oleh
// pegawai bertugas di SEKOLAH untuk tanggal absen yang terlewat (lihat
// riwayatAbsenSaya di absensi.go untuk definisi "tanggal terlewat" yang
// sama-sama dipakai di sini). Pegawai bertugas di DINAS/KANTOR TIDAK bisa
// mengajukan lewat sini sama sekali -- surat kolektif untuk mereka tetap
// hanya lewat inputAbsensiDokumenKolektif (menu Rekap Absen, admin/
// administrator/IsAdminAbsensi), lihat buatPengajuanSuratKolektif di bawah.
//
// Alur status: menunggu -> disetujui (baris AbsensiDokumen dibuat otomatis
// untuk setiap tanggal yang diajukan, absensi "berubah" jadi bersurat) ATAU
// menunggu -> dikembalikan (pegawai mengedit & mengajukan ulang lewat
// updatePengajuanSuratKolektif, balik jadi menunggu). Yang boleh menyetujui/
// mengembalikan HANYA administrator atau akun IsAdminVerifikasi -- lihat
// verifikasi() di RegisterPengajuanSuratKolektifRoutes.

// pengajuanSuratKolektifOut adalah DTO respons API -- menambahkan TanggalList
// (hasil parse TanggalListRaw, yang json-nya "-") supaya frontend bisa
// menampilkan daftar tanggal tanpa perlu tahu soal penyimpanan JSON mentahnya.
type pengajuanSuratKolektifOut struct {
	ID                uint            `json:"id"`
	IDPegawai         uint            `json:"id_pegawai"`
	Pegawai           *models.Pegawai `json:"pegawai,omitempty"`
	TanggalList       []string        `json:"tanggal_list"`
	Jenis             string          `json:"jenis"`
	Label             string          `json:"label"`
	NamaFile          string          `json:"nama_file"`
	Keterangan        string          `json:"keterangan"`
	Status            string          `json:"status"`
	CatatanVerifikasi string          `json:"catatan_verifikasi"`
	IDVerifikator     *uint           `json:"id_verifikator"`
	Verifikator       *models.User    `json:"verifikator,omitempty"`
	DiverifikasiAt    *time.Time      `json:"diverifikasi_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func toPengajuanSuratKolektifOut(item models.PengajuanSuratKolektif) pengajuanSuratKolektifOut {
	return pengajuanSuratKolektifOut{
		ID:                item.ID,
		IDPegawai:         item.IDPegawai,
		Pegawai:           item.Pegawai,
		TanggalList:       item.TanggalList(),
		Jenis:             item.Jenis,
		Label:             item.Label,
		NamaFile:          item.NamaFile,
		Keterangan:        item.Keterangan,
		Status:            item.Status,
		CatatanVerifikasi: item.CatatanVerifikasi,
		IDVerifikator:     item.IDVerifikator,
		Verifikator:       item.Verifikator,
		DiverifikasiAt:    item.DiverifikasiAt,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}

// tanggalTerlewatValid memeriksa satu tanggal untuk SATU pegawai: apakah
// tanggal itu benar-benar hari kerja pegawai tsb (mengikuti pola 5/6 hari
// dari sixDayWeekForPegawai, sama dengan cuti & rekap absen), bukan tanggal
// merah, belum ada absen masuk sungguhan, belum ada AbsensiDokumen, dan
// belum ada pengajuan surat kolektif lain (menunggu/disetujui) yang sudah
// mencakup tanggal ini -- definisi yang sama persis dengan "Tanggal
// Terlewat" pada riwayatAbsenSaya, supaya pegawai tidak bisa mengajukan
// surat untuk tanggal yang sebenarnya sudah hadir/bersurat/bukan hari kerja.
// excludeID dilewatkan saat mengedit pengajuan yang sudah ada sendiri
// (supaya baris pengajuan itu sendiri tidak dianggap "sudah ada pengajuan
// lain" untuk tanggalnya sendiri).
func tanggalTerlewatValid(db *gorm.DB, pegawai models.Pegawai, tanggal time.Time, excludeID uint) (bool, string) {
	today := absensiToday()
	if tanggal.After(today) {
		return false, "tanggal di masa depan"
	}
	wd := tanggal.Weekday()
	if wd == time.Sunday {
		return false, "hari Minggu (bukan hari kerja)"
	}
	if wd == time.Saturday && !sixDayWeekForPegawai(pegawai) {
		return false, "hari Sabtu (bukan hari kerja untuk pegawai kantor dinas)"
	}
	var holidayCount int64
	db.Model(&models.TglMerah{}).Where("tgl = ?", tanggal).Count(&holidayCount)
	if holidayCount > 0 {
		return false, "tanggal merah/hari libur"
	}
	var absensi models.Absensi
	if err := db.Where("id_pegawai = ? AND tanggal = ?", pegawai.ID, tanggal).First(&absensi).Error; err == nil && absensi.JamMasuk != nil {
		return false, "sudah tercatat absen masuk (hadir) pada tanggal ini"
	}
	var dokCount int64
	db.Model(&models.AbsensiDokumen{}).Where("id_pegawai = ? AND tanggal = ?", pegawai.ID, tanggal).Count(&dokCount)
	if dokCount > 0 {
		return false, "sudah ada surat (diinput admin) untuk tanggal ini"
	}
	tglStr := tanggal.Format("2006-01-02")
	var pendingRows []models.PengajuanSuratKolektif
	db.Where("id_pegawai = ? AND status IN ? AND tanggal_list LIKE ?", pegawai.ID,
		[]string{models.PengajuanSuratKolektifMenunggu, models.PengajuanSuratKolektifDisetujui},
		"%\""+tglStr+"\"%").Find(&pendingRows)
	for _, row := range pendingRows {
		if row.ID == excludeID {
			continue
		}
		for _, t := range row.TanggalList() {
			if t == tglStr {
				return false, "sudah ada pengajuan surat kolektif lain untuk tanggal ini"
			}
		}
	}
	return true, ""
}

// parseTanggalListForm mengurai & memvalidasi field berulang "tanggal" dari
// multipart form (format DD-MM-YYYY atau YYYY-MM-DD), mengembalikan slice
// time.Time terurut & sudah dihapus duplikatnya.
func parseTanggalListForm(r *http.Request) ([]time.Time, error) {
	raw := r.Form["tanggal"]
	seen := map[string]bool{}
	var out []time.Time
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		t, err := utils.ParseDateCell(s)
		if err != nil {
			return nil, err
		}
		key := t.Format("2006-01-02")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, nil
}

// buatPengajuanSuratKolektif menangani POST /api/pengajuan-surat-kolektif --
// pegawai bertugas di SEKOLAH mengajukan sendiri surat kolektif untuk
// beberapa tanggal terlewat sekaligus. Menerima multipart/form-data:
// "tanggal" (bisa dikirim berulang untuk pilih beberapa tanggal, format
// DD-MM-YYYY/YYYY-MM-DD), "jenis" (slug dari master Jenis Surat),
// "keterangan" (opsional), dan file "file" (satu berkas untuk semua
// tanggal).
func buatPengajuanSuratKolektif(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}
	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").First(&pegawai, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}
	// Pembatasan inti (poin 6 permintaan pengguna): pegawai bertugas di
	// DINAS/KANTOR tidak boleh mengajukan surat kolektif mandiri sama sekali
	// -- hanya admin/administrator/IsAdminAbsensi yang boleh menginputkan
	// untuk mereka lewat inputAbsensiDokumenKolektif (menu Rekap Absen).
	if !isSekolahPegawai(pegawai) {
		utils.Error(w, http.StatusForbidden,
			"pengajuan surat kolektif mandiri hanya untuk pegawai bertugas di sekolah -- pegawai dengan tempat tugas dinas/kantor tidak bisa mengajukan sendiri, surat kolektif untuk anda hanya bisa diinput oleh admin absensi/administrator.")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	jenis := strings.TrimSpace(r.FormValue("jenis"))
	var jenisSurat models.JenisSurat
	if jenis == "" || db.Where("slug = ?", jenis).First(&jenisSurat).Error != nil {
		utils.Error(w, http.StatusBadRequest, "jenis surat tidak valid -- pilih dari daftar Jenis Surat yang tersedia")
		return
	}

	tanggalList, err := parseTanggalListForm(r)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "format tanggal tidak valid")
		return
	}
	if len(tanggalList) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu tanggal terlewat")
		return
	}

	// Keterangan sekarang WAJIB diisi (dulu opsional) -- diisi lewat dropdown
	// pilihan tetap di frontend (lihat composables/keteranganSurat.js), atau
	// teks bebas kalau pilihan "Lainnya" -- keduanya sama-sama dikirim lewat
	// field "keterangan" ini, jadi validasinya cukup satu baris di sini.
	keterangan := strings.TrimSpace(r.FormValue("keterangan"))
	if keterangan == "" {
		utils.Error(w, http.StatusBadRequest, "keterangan wajib diisi")
		return
	}

	var invalid []string
	for _, t := range tanggalList {
		if ok, reason := tanggalTerlewatValid(db, pegawai, t, 0); !ok {
			invalid = append(invalid, t.Format("02-01-2006")+" ("+reason+")")
		}
	}
	if len(invalid) > 0 {
		utils.Error(w, http.StatusBadRequest,
			"tanggal berikut tidak bisa diajukan: "+strings.Join(invalid, ", ")+
				" -- hanya tanggal terlewat (hari kerja anda yang belum ada absen masuk maupun surat) yang bisa diajukan surat kolektif.")
		return
	}

	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas surat wajib diupload")
		return
	}
	ext := strings.ToLower(fh.Filename[strings.LastIndex(fh.Filename, "."):])
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, JPG, atau PNG")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	fileData, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering terjadi kalau
	// foto dari WhatsApp/Google Photos di HP belum selesai diunduh ke
	// perangkat saat dipilih lewat file picker.
	if len(fileData) == 0 {
		utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
		return
	}

	tanggalStr := make([]string, 0, len(tanggalList))
	for _, t := range tanggalList {
		tanggalStr = append(tanggalStr, t.Format("2006-01-02"))
	}

	item := models.PengajuanSuratKolektif{
		IDPegawai:  pegawai.ID,
		Jenis:      jenisSurat.Slug,
		Label:      jenisSurat.Nama,
		NamaFile:   fh.Filename,
		File:       fileData,
		Keterangan: keterangan,
		Status:     models.PengajuanSuratKolektifMenunggu,
	}
	item.SetTanggalList(tanggalStr)
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	utils.Created(w, "pengajuan surat kolektif berhasil dikirim, menunggu verifikasi", toPengajuanSuratKolektifOut(item))
}

// updatePengajuanSuratKolektif menangani PUT
// /api/pengajuan-surat-kolektif/{id} -- pegawai pemilik mengedit &
// mengajukan ulang pengajuannya sendiri yang statusnya "dikembalikan"
// (balik jadi "menunggu" lagi, catatan verifikasi sebelumnya dihapus).
// Menerima multipart/form-data yang sama seperti buatPengajuanSuratKolektif;
// berkas boleh tidak dikirim ulang (berkas lama dipertahankan).
func updatePengajuanSuratKolektif(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}
	id := r.PathValue("id")
	var item models.PengajuanSuratKolektif
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.IDPegawai != *claims.IDPegawai {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke pengajuan ini")
		return
	}
	if item.Status != models.PengajuanSuratKolektifDikembalikan {
		utils.Error(w, http.StatusBadRequest, "hanya pengajuan yang berstatus \"dikembalikan\" yang boleh diedit & diajukan ulang")
		return
	}
	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").First(&pegawai, item.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	jenis := strings.TrimSpace(r.FormValue("jenis"))
	var jenisSurat models.JenisSurat
	if jenis == "" || db.Where("slug = ?", jenis).First(&jenisSurat).Error != nil {
		utils.Error(w, http.StatusBadRequest, "jenis surat tidak valid -- pilih dari daftar Jenis Surat yang tersedia")
		return
	}

	tanggalList, err := parseTanggalListForm(r)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "format tanggal tidak valid")
		return
	}
	if len(tanggalList) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu tanggal terlewat")
		return
	}
	// Keterangan wajib diisi -- lihat komentar yang sama pada
	// buatPengajuanSuratKolektif.
	keterangan := strings.TrimSpace(r.FormValue("keterangan"))
	if keterangan == "" {
		utils.Error(w, http.StatusBadRequest, "keterangan wajib diisi")
		return
	}
	var invalid []string
	for _, t := range tanggalList {
		if ok, reason := tanggalTerlewatValid(db, pegawai, t, item.ID); !ok {
			invalid = append(invalid, t.Format("02-01-2006")+" ("+reason+")")
		}
	}
	if len(invalid) > 0 {
		utils.Error(w, http.StatusBadRequest,
			"tanggal berikut tidak bisa diajukan: "+strings.Join(invalid, ", ")+
				" -- hanya tanggal terlewat (hari kerja anda yang belum ada absen masuk maupun surat) yang bisa diajukan surat kolektif.")
		return
	}

	if fh := formFileHeader(r, "file"); fh != nil {
		ext := strings.ToLower(fh.Filename[strings.LastIndex(fh.Filename, "."):])
		if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, JPG, atau PNG")
			return
		}
		f, err := fh.Open()
		if err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
			return
		}
		fileData, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
			return
		}
		// Berkas 0 byte lolos dari io.ReadAll tanpa error -- sering terjadi
		// kalau foto dari WhatsApp/Google Photos di HP belum selesai
		// diunduh ke perangkat saat dipilih lewat file picker.
		if len(fileData) == 0 {
			utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
			return
		}
		item.NamaFile = fh.Filename
		item.File = fileData
	}

	tanggalStr := make([]string, 0, len(tanggalList))
	for _, t := range tanggalList {
		tanggalStr = append(tanggalStr, t.Format("2006-01-02"))
	}
	item.Jenis = jenisSurat.Slug
	item.Label = jenisSurat.Nama
	item.Keterangan = keterangan
	item.SetTanggalList(tanggalStr)
	item.Status = models.PengajuanSuratKolektifMenunggu
	item.CatatanVerifikasi = ""
	item.IDVerifikator = nil
	item.DiverifikasiAt = nil
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan berhasil diperbarui & dikirim ulang, menunggu verifikasi", toPengajuanSuratKolektifOut(item))
}

// listPengajuanSuratKolektifSaya menangani GET
// /api/pengajuan-surat-kolektif/saya -- daftar pengajuan milik pegawai yang
// login sendiri (semua status).
func listPengajuanSuratKolektifSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", []pengajuanSuratKolektifOut{})
		return
	}
	items := []models.PengajuanSuratKolektif{}
	db.Omit("file").Where("id_pegawai = ?", *claims.IDPegawai).Order("created_at desc").Find(&items)
	out := make([]pengajuanSuratKolektifOut, 0, len(items))
	for _, it := range items {
		out = append(out, toPengajuanSuratKolektifOut(it))
	}
	utils.Success(w, "ok", out)
}

// listPengajuanSuratKolektifAdmin menangani GET /api/pengajuan-surat-kolektif
// -- daftar SEMUA pengajuan (default hanya "menunggu", ?status=semua untuk
// semua status) untuk administrator/akun IsAdminVerifikasi memverifikasi.
//
// Riwayat "siapa yang memverifikasi" (Verifikator, berisi nama akun) HANYA
// dikirim kalau requester-nya benar-benar role "administrator" -- akun
// IsAdminVerifikasi lain yang sama-sama boleh menyetujui/mengembalikan
// pengajuan lewat tab ini TIDAK ikut melihat siapa (akun mana) yang
// memverifikasi pengajuan lain, hanya administrator yang bisa audit. Baik
// field objek (Verifikator) maupun ID mentahnya (IDVerifikator) disembunyikan
// bersamaan supaya tidak bisa ditebak lewat ID-nya sendiri.
func listPengajuanSuratKolektifAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	isAdministrator := claims != nil && claims.RoleName == "administrator"
	// Preload("Pegawai.UnitKerja") -- supaya frontend bisa menyaring tabel
	// Verifikasi lewat kotak pencarian nama/NIP/unit kerja yang sama seperti
	// tab Rekap Absen/Surat Kolektif.
	query := db.Omit("file").Preload("Pegawai.UnitKerja")
	if isAdministrator {
		query = query.Preload("Verifikator")
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "semua" {
		query = query.Where("status = ?", status)
	} else if status == "" {
		query = query.Where("status = ?", models.PengajuanSuratKolektifMenunggu)
	}
	items := []models.PengajuanSuratKolektif{}
	query.Order("created_at desc").Find(&items)
	out := make([]pengajuanSuratKolektifOut, 0, len(items))
	for _, it := range items {
		if !isAdministrator {
			it.Verifikator = nil
			it.IDVerifikator = nil
		}
		out = append(out, toPengajuanSuratKolektifOut(it))
	}
	utils.Success(w, "ok", out)
}

// countPengajuanSuratKolektifMenunggu menangani GET
// /api/pengajuan-surat-kolektif/count-menunggu -- jumlah pengajuan surat
// kolektif sekolah yang berstatus "menunggu" verifikasi, INDEPENDEN dari
// filter status apapun yang sedang dipilih administrator/admin verifikasi
// pada tabel tab "Verifikasi Surat Kolektif Sekolah" (menu Rekap Absen).
// Dipakai untuk menampilkan badge angka pada label tab itu sendiri (lihat
// RekapAbsensiView.vue) supaya administrator/admin verifikasi langsung tahu
// ada berapa banyak pengajuan baru tanpa perlu membuka tabnya dulu.
func countPengajuanSuratKolektifMenunggu(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var jumlah int64
	db.Model(&models.PengajuanSuratKolektif{}).
		Where("status = ?", models.PengajuanSuratKolektifMenunggu).
		Count(&jumlah)
	utils.Success(w, "ok", map[string]interface{}{"menunggu": jumlah})
}

// setujuiPengajuanSuratKolektif menangani PUT
// /api/pengajuan-surat-kolektif/{id}/setujui -- administrator/akun
// IsAdminVerifikasi menyetujui pengajuan: baris AbsensiDokumen dibuat/
// diperbarui untuk setiap tanggal yang diajukan (upsert, sama seperti
// inputAbsensiDokumenKolektif), KECUALI tanggal yang di antara pengajuan &
// verifikasi ternyata sudah tercatat absen masuk sungguhan (dilewati,
// diberi peringatan) -- absensi pegawai "berubah" jadi bersurat begitu
// disetujui, sesuai permintaan pengguna.
func setujuiPengajuanSuratKolektif(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanSuratKolektif
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Status != models.PengajuanSuratKolektifMenunggu {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}

	var pegawai models.Pegawai
	if err := db.First(&pegawai, item.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	dilewati := []string{}
	jumlah := 0
	for _, tglStr := range item.TanggalList() {
		tgl, err := utils.ParseDateCell(tglStr)
		if err != nil {
			continue
		}
		var absensi models.Absensi
		if err := db.Where("id_pegawai = ? AND tanggal = ?", item.IDPegawai, tgl).First(&absensi).Error; err == nil && absensi.JamMasuk != nil {
			dilewati = append(dilewati, tglStr)
			continue
		}
		var existing models.AbsensiDokumen
		found := db.Where("id_pegawai = ? AND tanggal = ?", item.IDPegawai, tgl).First(&existing).Error == nil
		existing.IDPegawai = item.IDPegawai
		existing.Tanggal = tgl
		existing.Jenis = item.Jenis
		existing.Label = item.Label
		existing.NamaFile = item.NamaFile
		existing.File = item.File
		existing.Keterangan = item.Keterangan
		// IDDiinputOleh diisi verifikator (claims.UserID) -- baris
		// AbsensiDokumen di sini justru BARU tercatat/berubah lewat aksi
		// menyetujui ini, bukan lewat inputAbsensiDokumenKolektif, jadi tanpa
		// ini baris yang berasal dari pengajuan mandiri pegawai sekolah akan
		// selalu tampil "Diinput Oleh: -" pada tab "Surat Kolektif" walau
		// sudah tercatat -- padahal permintaan awal fitur ini eksplisit minta
		// riwayat penginput di KEDUA menu (Input Surat Kolektif & Verifikasi
		// Surat Kolektif Sekolah). Ini terpisah dari IDVerifikator pada
		// PengajuanSuratKolektif itu sendiri (kolom "Diverifikasi Oleh" di
		// tab Verifikasi) -- sama akunnya, tapi ditampilkan di tab yang
		// berbeda.
		if claims != nil {
			userID := claims.UserID
			existing.IDDiinputOleh = &userID
		}
		if found {
			db.Save(&existing)
		} else {
			existing.ID = 0
			db.Create(&existing)
		}
		jumlah++
	}

	now := absensiNow()
	item.Status = models.PengajuanSuratKolektifDisetujui
	item.IDVerifikator = &claims.UserID
	item.DiverifikasiAt = &now
	item.CatatanVerifikasi = strings.TrimSpace(r.FormValue("catatan"))
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan persetujuan: "+err.Error())
		return
	}

	pesan := "pengajuan surat kolektif disetujui"
	if jumlah > 0 {
		pesan += " -- absen pegawai untuk tanggal yang diajukan diperbarui"
	}
	if len(dilewati) > 0 {
		pesan += " -- PERINGATAN: tanggal " + strings.Join(dilewati, ", ") + " dilewati karena sudah tercatat absen masuk sungguhan"
	}
	utils.Success(w, pesan, toPengajuanSuratKolektifOut(item))
}

// kembalikanPengajuanSuratKolektif menangani PUT
// /api/pengajuan-surat-kolektif/{id}/kembalikan -- administrator/akun
// IsAdminVerifikasi mengembalikan pengajuan untuk direvisi pegawai (wajib
// mengisi catatan supaya pegawai tahu apa yang perlu diperbaiki). Menerima
// form-urlencoded atau multipart dengan field "catatan".
func kembalikanPengajuanSuratKolektif(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanSuratKolektif
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Status != models.PengajuanSuratKolektifMenunggu {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}
	// r.FormValue sendiri sudah otomatis mem-parsing body sebagai
	// multipart/form-data ATAU application/x-www-form-urlencoded (dan
	// keduanya dipakai frontend/pengujian di sini) -- SENGAJA tidak
	// memanggil r.ParseForm() secara eksplisit lebih dulu, karena itu
	// hanya menangani application/x-www-form-urlencoded & query string,
	// membuat r.Form langsung terisi (walau kosong) sehingga r.FormValue
	// tidak lagi mencoba mem-parsing body multipart sama sekali -- catatan
	// jadi selalu terbaca kosong walau sudah dikirim.
	catatan := strings.TrimSpace(r.FormValue("catatan"))
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya pegawai tahu apa yang perlu diperbaiki")
		return
	}
	now := absensiNow()
	item.Status = models.PengajuanSuratKolektifDikembalikan
	item.CatatanVerifikasi = catatan
	item.IDVerifikator = &claims.UserID
	item.DiverifikasiAt = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan dikembalikan ke pegawai untuk direvisi", toPengajuanSuratKolektifOut(item))
}

func canAccessPengajuanSuratKolektif(claims *utils.Claims, item models.PengajuanSuratKolektif) bool {
	if claims.RoleName == "administrator" || claims.IsAdminVerifikasi {
		return true
	}
	return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
}

// downloadPengajuanSuratKolektif menangani GET
// /api/pengajuan-surat-kolektif/{id}/file -- diunduh pegawai pemilik atau
// administrator/akun IsAdminVerifikasi.
func downloadPengajuanSuratKolektif(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanSuratKolektif
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessPengajuanSuratKolektif(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.File) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	w.Header().Set("Content-Type", dokumenContentType(item.NamaFile))
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+item.NamaFile+"\"")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+item.NamaFile+"\"")
	}
	w.Write(item.File)
}

// RegisterPengajuanSuratKolektifRoutes mendaftarkan semua endpoint
// /api/pengajuan-surat-kolektif*.
func RegisterPengajuanSuratKolektifRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai", "atasan") }
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h) }
	// verifikasi: HANYA administrator atau akun mana pun yang ditandai
	// IsAdminVerifikasi = true (lihat models.User.IsAdminVerifikasi) --
	// SENGAJA berbeda dari manage() di absensi.go (yang juga meluluskan
	// role admin & IsAdminAbsensi), karena permintaan pengguna secara
	// eksplisit membatasi siapa yang boleh menyetujui/mengembalikan
	// pengajuan surat kolektif sekolah HANYA administrator & IsAdminVerifikasi.
	verifikasi := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := middleware.GetClaims(r)
			if !ok {
				utils.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if claims.RoleName != "administrator" && !claims.IsAdminVerifikasi {
				utils.Error(w, http.StatusForbidden, "hanya administrator atau admin verifikasi yang bisa melakukan aksi ini")
				return
			}
			h(w, r)
		}, middleware.Auth, middleware.RequireActiveUser(db))
	}

	mux.Handle("POST /api/pengajuan-surat-kolektif", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { buatPengajuanSuratKolektif(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-surat-kolektif/{id}", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { updatePengajuanSuratKolektif(w, r, db) }))
	mux.Handle("GET /api/pengajuan-surat-kolektif/saya", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { listPengajuanSuratKolektifSaya(w, r, db) }))
	mux.Handle("GET /api/pengajuan-surat-kolektif", verifikasi(func(w http.ResponseWriter, r *http.Request) { listPengajuanSuratKolektifAdmin(w, r, db) }))
	mux.Handle("GET /api/pengajuan-surat-kolektif/count-menunggu", verifikasi(func(w http.ResponseWriter, r *http.Request) { countPengajuanSuratKolektifMenunggu(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-surat-kolektif/{id}/setujui", verifikasi(func(w http.ResponseWriter, r *http.Request) { setujuiPengajuanSuratKolektif(w, r, db) }))
	mux.Handle("PUT /api/pengajuan-surat-kolektif/{id}/kembalikan", verifikasi(func(w http.ResponseWriter, r *http.Request) { kembalikanPengajuanSuratKolektif(w, r, db) }))
	mux.Handle("GET /api/pengajuan-surat-kolektif/{id}/file", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadPengajuanSuratKolektif(w, r, db) }))
}
