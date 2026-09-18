package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// absensi_dokumen.go menangani surat pendukung (SKS/Surat Tugas/Berita
// Acara/Surat Izin) untuk tanggal absen yang terlewat -- lihat
// riwayatAbsenSaya (absensi.go) untuk bagaimana tanggal_terlewat/tanggal_
// tercover dihitung. Sejak fitur ini diubah, surat HANYA diinput oleh
// administrator/admin/akun IsAdminAbsensi (bisa kolektif -- beberapa
// pegawai & rentang tanggal sekaligus lewat inputAbsensiDokumenKolektif),
// tidak lagi diupload sendiri oleh pegawai -- pegawai hanya bisa
// melihat/mengunduh surat yang sudah diinput untuknya
// (listAbsensiDokumenSaya/downloadAbsensiDokumen).

// jenisSuratLookup memuat semua master Jenis Surat (menu Master Data ->
// Jenis Surat) sekaligus jadi map by slug -- dipakai riwayat/rekap/PDF
// supaya kode (DD/I/S) tidak lagi hardcode dan otomatis ikut jenis surat
// tambahan yang dibuat administrator, tanpa query berulang per baris.
func jenisSuratLookup(db *gorm.DB) map[string]models.JenisSurat {
	var rows []models.JenisSurat
	db.Find(&rows)
	out := make(map[string]models.JenisSurat, len(rows))
	for _, row := range rows {
		out[row.Slug] = row
	}
	return out
}

// kodeUntukJenis mengembalikan kode (DD/I/S) untuk satu slug jenis surat dari
// hasil jenisSuratLookup. Kalau slug-nya sudah tidak ada di master (misalnya
// baris JenisSurat-nya dihapus administrator) tapi masih dipakai baris
// AbsensiDokumen lama, coba tebak dari peta 4 jenis bawaan supaya data lama
// tetap tampil benar di rekap/PDF.
//
// js.Kode SENGAJA di-trim & dianggap "tidak ada" kalau hasilnya kosong --
// menyusul kasus nyata: baris master Jenis Surat "Dinas Dalam" tersimpan
// dengan Kode cuma berisi whitespace (mis. satu spasi, entah salah ketik
// atau lolos dari validasi wajib-isi versi lama di form Master Data ->
// Jenis Surat -- lihat perbaikan saveForm() di CrudManager.vue), sehingga
// Kode tampil sebagai Tag kosong tanpa terdeteksi ada isinya sama sekali
// (whitespace bukan string kosong secara literal). Tanpa trim di sini,
// kode " " akan lolos begitu saja tanpa jatuh ke fallback peta bawaan di
// bawah, padahal secara efektif tetap tidak berguna.
func kodeUntukJenis(lookup map[string]models.JenisSurat, jenis string) string {
	if js, ok := lookup[jenis]; ok {
		if kode := strings.TrimSpace(js.Kode); kode != "" {
			return kode
		}
	}
	return models.AbsensiDokumenKode[jenis]
}

// labelUntukJenis mengembalikan label yang ditampilkan di riwayat/rekap/PDF
// untuk satu slug jenis surat: diambil dari Nama pada master Jenis Surat
// (jadi otomatis benar untuk kode apapun yang diketik administrator sendiri,
// bukan cuma DD/I/S). Kalau slug-nya sudah tidak ada di master, coba tebak
// dari peta label 3 kode bawaan supaya data lama tetap tampil, dan kalau itu
// pun tidak ketemu, kode-nya sendiri dipakai sebagai label supaya tidak
// pernah tampil kosong.
func labelUntukJenis(lookup map[string]models.JenisSurat, jenis string, kode string) string {
	if js, ok := lookup[jenis]; ok && js.Nama != "" {
		return js.Nama
	}
	if lbl, ok := models.AbsensiDokumenKodeLabel[kode]; ok {
		return lbl
	}
	return kode
}

func canAccessAbsensiDokumen(claims *utils.Claims, item models.AbsensiDokumen) bool {
	if claims.RoleName == "administrator" || claims.RoleName == "admin" || claims.IsAdminAbsensi {
		return true
	}
	switch claims.RoleName {
	case "pegawai", "atasan":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	}
	return false
}

// inputAbsensiDokumenKolektif dipakai administrator/admin untuk menginput
// surat pendukung (BA/Surat Tugas/Surat Izin/SKS) sekaligus untuk beberapa
// pegawai & rentang tanggal -- menerima multipart/form-data: "id_pegawai"
// (bisa dikirim berulang untuk pilih beberapa pegawai), "tanggal_mulai" &
// "tanggal_selesai" (format DD-MM-YYYY atau YYYY-MM-DD; tanggal_selesai
// opsional, default sama dengan tanggal_mulai kalau hanya satu tanggal),
// "jenis" (sks/surat_tugas/berita_acara/surat_izin), "keterangan" (opsional)
// dan file "file" -- satu berkas yang sama dipakai untuk semua pegawai &
// tanggal yang dipilih (mis. satu Berita Acara untuk banyak pegawai).
// Kalau untuk (pegawai, tanggal) tertentu sudah ada dokumen sebelumnya, baris
// itu DIPERBARUI (bukan dibuat lagi) supaya tidak dobel.
func inputAbsensiDokumenKolektif(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	// claims.UserID dicatat sebagai IDDiinputOleh pada setiap baris yang
	// dibuat/ditimpa di bawah -- lihat komentar IDDiinputOleh pada
	// models.AbsensiDokumen untuk alasan & siapa yang boleh melihatnya.
	claims, _ := middleware.GetClaims(r)
	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	idPegawaiRaw := r.Form["id_pegawai"]
	idList := make([]uint, 0, len(idPegawaiRaw))
	for _, raw := range idPegawaiRaw {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			continue
		}
		idList = append(idList, uint(v))
	}
	if len(idList) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu pegawai")
		return
	}

	mulaiRaw := strings.TrimSpace(r.FormValue("tanggal_mulai"))
	if mulaiRaw == "" {
		utils.Error(w, http.StatusBadRequest, "tanggal mulai wajib diisi")
		return
	}
	tglMulai, err := utils.ParseDateCell(mulaiRaw)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal mulai tidak valid")
		return
	}
	tglSelesai := tglMulai
	if selesaiRaw := strings.TrimSpace(r.FormValue("tanggal_selesai")); selesaiRaw != "" {
		tglSelesai, err = utils.ParseDateCell(selesaiRaw)
		if err != nil {
			utils.Error(w, http.StatusBadRequest, "tanggal selesai tidak valid")
			return
		}
	}
	if tglSelesai.Before(tglMulai) {
		utils.Error(w, http.StatusBadRequest, "tanggal selesai tidak boleh sebelum tanggal mulai")
		return
	}
	if tglSelesai.Sub(tglMulai) > 366*24*time.Hour {
		utils.Error(w, http.StatusBadRequest, "rentang tanggal maksimal 1 tahun")
		return
	}

	jenis := strings.TrimSpace(r.FormValue("jenis"))
	var jenisSurat models.JenisSurat
	if jenis == "" || db.Where("slug = ?", jenis).First(&jenisSurat).Error != nil {
		utils.Error(w, http.StatusBadRequest, "jenis surat tidak valid -- pilih dari daftar Jenis Surat yang tersedia (kelola di menu Master Data -> Jenis Surat)")
		return
	}
	label := jenisSurat.Nama

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
	// Keterangan sekarang WAJIB diisi (dulu opsional) -- diisi lewat dropdown
	// pilihan tetap di frontend (lihat composables/keteranganSurat.js), atau
	// teks bebas kalau pilihan "Lainnya".
	keterangan := strings.TrimSpace(r.FormValue("keterangan"))
	if keterangan == "" {
		utils.Error(w, http.StatusBadRequest, "keterangan wajib diisi")
		return
	}

	// Pola hari kerja dihitung PER PEGAWAI dari status dinas/sekolahnya (lihat
	// sixDayWeekForPegawai/isSekolahPegawai di pengajuan_cuti.go): pegawai
	// sekolah masuk Senin-Sabtu (hanya Minggu yang libur), pegawai kantor
	// dinas masuk Senin-Jumat (Sabtu & Minggu libur). Tanggal merah juga
	// dilewati untuk keduanya. Tanggal di luar hari kerja TIDAK diinput
	// walaupun ikut dipilih admin dalam rentang tanggal -- aturannya sama
	// persis dengan perhitungan "tanggal terlewat" pada rekap absen, supaya
	// tidak ada surat untuk hari yang memang bukan hari kerja.
	var pegawaiTerpilih []models.Pegawai
	if err := db.Preload("UnitKerja").Where("id IN ?", idList).Find(&pegawaiTerpilih).Error; err != nil || len(pegawaiTerpilih) == 0 {
		utils.Error(w, http.StatusBadRequest, "data pegawai yang dipilih tidak ditemukan")
		return
	}

	totalHari := 0
	for d := tglMulai; !d.After(tglSelesai); d = d.AddDate(0, 0, 1) {
		totalHari++
	}
	holidaySet := holidaySetInRange(db, tglMulai, tglSelesai)

	// tanggal yang SUDAH punya absen masuk sungguhan (lewat kamera/menu
	// Absen) tidak boleh ditimpa surat pendukung -- kalau pegawai memang
	// hadir dan absen sendiri pada tanggal itu, absensinya harus tetap yang
	// tampil di riwayat/rekap, bukan surat DD/Izin/Sakit. Diambil sekaligus
	// (bukan per tanggal) supaya tidak query berulang di dalam loop.
	var absensiRows []models.Absensi
	db.Where("id_pegawai IN ? AND tanggal BETWEEN ? AND ?", idList, tglMulai, tglSelesai).Find(&absensiRows)
	hadirSet := map[string]bool{} // key: "<id_pegawai>|<yyyy-mm-dd>"
	for _, a := range absensiRows {
		if a.JamMasuk != nil {
			hadirSet[fmt.Sprintf("%d|%s", a.IDPegawai, a.Tanggal.Format("2006-01-02"))] = true
		}
	}

	jumlah := 0
	dilewati := 0
	dilewatiHadir := 0
	contohDilewatiHadir := []string{}
	for _, p := range pegawaiTerpilih {
		hariKerja := workingDaysWithHolidaySet(tglMulai, tglSelesai, sixDayWeekForPegawai(p), holidaySet)
		dilewati += totalHari - len(hariKerja)
		for _, tgl := range hariKerja {
			if hadirSet[fmt.Sprintf("%d|%s", p.ID, tgl.Format("2006-01-02"))] {
				dilewatiHadir++
				if len(contohDilewatiHadir) < 8 {
					contohDilewatiHadir = append(contohDilewatiHadir, fmt.Sprintf("%s (%s)", p.Nama, tgl.Format("02-01-2006")))
				}
				continue
			}
			var existing models.AbsensiDokumen
			found := db.Where("id_pegawai = ? AND tanggal = ?", p.ID, tgl).First(&existing).Error == nil
			existing.IDPegawai = p.ID
			existing.Tanggal = tgl
			existing.Jenis = jenis
			existing.Label = label
			existing.NamaFile = fh.Filename
			existing.File = fileData
			existing.Keterangan = keterangan
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
	}

	peringatanHadir := ""
	if dilewatiHadir > 0 {
		peringatanHadir = fmt.Sprintf(" -- PERINGATAN: %d tanggal dilewati karena pegawai bersangkutan sudah tercatat absen masuk pada tanggal tersebut (%s%s)",
			dilewatiHadir, strings.Join(contohDilewatiHadir, ", "), func() string {
				if dilewatiHadir > len(contohDilewatiHadir) {
					return fmt.Sprintf(", +%d lainnya", dilewatiHadir-len(contohDilewatiHadir))
				}
				return ""
			}())
	}

	if jumlah == 0 {
		if dilewatiHadir > 0 && dilewati == 0 {
			// satu-satunya alasan tidak ada baris tersimpan adalah karena
			// semua tanggal hari kerja pada rentang ini sudah punya absen
			// masuk sungguhan (bukan karena bukan hari kerja).
			utils.Error(w, http.StatusBadRequest,
				"surat tidak diinput -- semua tanggal pada rentang ini sudah tercatat absen masuk (hadir) untuk pegawai yang dipilih, jadi tidak boleh ditimpa surat pendukung."+peringatanHadir)
			return
		}
		utils.Error(w, http.StatusBadRequest,
			"tidak ada tanggal yang bisa diinput -- semua tanggal pada rentang itu bukan hari kerja bagi pegawai yang dipilih (Sabtu/Minggu untuk pegawai kantor dinas, Minggu untuk pegawai sekolah, atau tanggal merah), atau sudah tercatat absen masuk."+peringatanHadir)
		return
	}

	pesan := fmt.Sprintf("surat berhasil diinput untuk %d pegawai (%d baris)", len(pegawaiTerpilih), jumlah)
	if dilewati > 0 {
		pesan += fmt.Sprintf(" -- %d tanggal dilewati karena bukan hari kerja pegawai bersangkutan (Sabtu/Minggu/tanggal merah)", dilewati)
	}
	pesan += peringatanHadir
	utils.Created(w, pesan, nil)
}

// absensiDokumenAdminOut membungkus models.AbsensiDokumen APA ADANYA (lewat
// embedding) ditambah Kode yang dihitung LIVE lewat kodeUntukJenis -- SEBELUM
// perbaikan ini, kolom "Kode" pada tabel "Surat yang Sudah Diinput" (tab
// Surat Kolektif, RekapAbsensiView.vue) dihitung sendiri di frontend lewat
// peta jenisSuratKodeMap (dari /ref/jenis-surat) + fallback hardcode 4 slug
// bawaan -- terpisah dari logika yang sama persis di AbsensiView.vue (dua
// sumber kebenaran yang gampang tidak sinkron). Menyusul laporan nyata: baris
// "Dinas Dalam" tampil Kode kosong (Tag tanpa isi) -- entah karena kolom Kode
// pada master Jenis Surat "Dinas Dalam" kosong/cuma whitespace, atau slug-nya
// custom & tak dikenali peta hardcode manapun. Dipindah ke sini (satu-satunya
// tempat yang sudah dipakai bersama oleh listAbsensiDokumenSaya/riwayat/
// rekap/PDF -- lihat kodeUntukJenis) supaya SELALU konsisten & satu sumber
// kebenaran, tidak bergantung pada peta terpisah di tiap file frontend.
type absensiDokumenAdminOut struct {
	models.AbsensiDokumen
	Kode string `json:"kode"`
}

// listAbsensiDokumenAdmin dipakai administrator/admin/IsAdminAbsensi untuk
// melihat/mengelola semua surat yang sudah diinput pada satu bulan (opsional
// filter id_pegawai) -- dipakai di halaman Rekap Absen supaya admin tahu
// surat apa yang sudah ada sebelum menginput lagi, dan bisa menghapusnya
// bila salah.
//
// Riwayat "siapa yang menginput" (DiinputOleh) HANYA di-preload & dikirim
// kalau requester-nya benar-benar role "administrator" -- lihat komentar
// IDDiinputOleh pada models.AbsensiDokumen. Akun admin/IsAdminAbsensi lain
// yang mengakses endpoint yang sama TIDAK ikut menerima field ini sama
// sekali (bukan cuma disembunyikan di frontend, supaya tidak bisa dilihat
// juga lewat DevTools/panggilan API langsung).
func listAbsensiDokumenAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	now := absensiNow()
	bulan := int(now.Month())
	tahun := now.Year()
	if v, err := strconv.Atoi(r.URL.Query().Get("bulan")); err == nil && v >= 1 && v <= 12 {
		bulan = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("tahun")); err == nil && v > 2000 {
		tahun = v
	}
	loc := now.Location()
	start := time.Date(tahun, time.Month(bulan), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)

	// Preload("Pegawai.UnitKerja") (bukan cuma "Pegawai") -- supaya frontend
	// bisa menyaring tabel "Surat yang Sudah Diinput Bulan Ini" lewat kotak
	// pencarian nama/NIP/unit kerja yang sama seperti tab Rekap Absen.
	query := db.Omit("file").Where("tanggal BETWEEN ? AND ?", start, end).Preload("Pegawai.UnitKerja")
	isAdministrator := claims != nil && claims.RoleName == "administrator"
	if isAdministrator {
		query = query.Preload("DiinputOleh")
	}
	if idStr := strings.TrimSpace(r.URL.Query().Get("id_pegawai")); idStr != "" {
		query = query.Where("id_pegawai = ?", idStr)
	}
	// Diurutkan berdasarkan PENGINPUTAN paling baru (updated_at, lihat komentar
	// UpdatedAt pada models.AbsensiDokumen) -- BUKAN tanggal absennya sendiri --
	// supaya surat yang baru saja diinput/diedit selalu tampil paling atas,
	// baik itu diinput administrator/admin/admin absen langsung lewat menu
	// ini, maupun lewat pengajuan mandiri pegawai sekolah yang baru disetujui.
	items := []models.AbsensiDokumen{}
	if err := query.Order("updated_at desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	jenisLookup := jenisSuratLookup(db)
	out := make([]absensiDokumenAdminOut, 0, len(items))
	for _, item := range items {
		if !isAdministrator {
			// pengaman ganda -- pastikan field ini benar-benar nil (jadi ikut
			// dihilangkan dari JSON lewat "omitempty") walau suatu saat ada
			// baris yang preloaded dari jalur lain. IDDiinputOleh (angka
			// mentahnya) ikut disembunyikan juga, bukan cuma DiinputOleh
			// (objek nama) -- supaya akun non-administrator tidak bisa
			// menebak siapa penginputnya lewat ID-nya sendiri (mis.
			// dicocokkan manual ke menu Pengguna).
			item.DiinputOleh = nil
			item.IDDiinputOleh = nil
		}
		out = append(out, absensiDokumenAdminOut{
			AbsensiDokumen: item,
			Kode:           kodeUntukJenis(jenisLookup, item.Jenis),
		})
	}
	utils.Success(w, "ok", out)
}

// absensiDokumenSayaOut adalah bentuk tampil satu baris "Surat Pendukung"
// pada menu Absen milik pegawai sendiri (AbsensiView.vue) -- field Kode
// SENGAJA ditambahkan & dihitung ulang di sini (tidak ada di kolom tabel
// absensi_dokumen), mengikuti pola yang sama dengan tercoverEntryFromDokumen
// (absensi.go) dan exportRekapAbsensiPegawaiPDF (absensi_pdf.go): Kode & Label
// selalu diambil LIVE dari master Jenis Surat (menu Master Data -> Jenis
// Surat) lewat kodeUntukJenis/labelUntukJenis, BUKAN dihardcode di frontend.
//
// SEBELUM perbaikan ini, endpoint ini mengembalikan baris models.AbsensiDokumen
// APA ADANYA (cuma field jenis mentah, tanpa kode) -- AbsensiView.vue lalu
// mencoba menerka Kode sendiri lewat peta JENIS_KODE yang hardcode cuma 4
// slug bawaan (sks/surat_tugas/berita_acara/surat_izin). Begitu administrator
// membuat Jenis Surat baru lewat menu Master Data (mis. slug "dinas_dalam"
// dengan Label "Dinas Dalam", BUKAN "surat_tugas"/"berita_acara" bawaan),
// peta hardcode di frontend itu tidak mengenalinya sama sekali -- Jenis Surat
// tetap tampil benar (field label yang dibekukan saat surat diinput, ikut
// terkirim apa adanya), tapi Kode selalu kosong karena kuncinya tidak ada di
// peta tersebut. Sekarang Kode dihitung di backend (satu-satunya tempat yang
// tahu isi master Jenis Surat terkini) dan dikirim langsung, jadi benar untuk
// jenis surat apa pun yang dibuat administrator, tidak dibatasi 4 bawaan.
type absensiDokumenSayaOut struct {
	ID         uint      `json:"id"`
	Tanggal    time.Time `json:"tanggal"`
	Jenis      string    `json:"jenis"`
	Kode       string    `json:"kode"`
	Label      string    `json:"label"`
	NamaFile   string    `json:"nama_file"`
	Keterangan string    `json:"keterangan"`
	CreatedAt  time.Time `json:"created_at"`
}

func listAbsensiDokumenSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", []absensiDokumenSayaOut{})
		return
	}
	items := []models.AbsensiDokumen{}
	if err := db.Omit("file").Where("id_pegawai = ?", *claims.IDPegawai).
		Order("tanggal desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	jenisLookup := jenisSuratLookup(db)
	out := make([]absensiDokumenSayaOut, 0, len(items))
	for _, item := range items {
		kode := kodeUntukJenis(jenisLookup, item.Jenis)
		out = append(out, absensiDokumenSayaOut{
			ID:         item.ID,
			Tanggal:    item.Tanggal,
			Jenis:      item.Jenis,
			Kode:       kode,
			Label:      labelUntukJenis(jenisLookup, item.Jenis, kode),
			NamaFile:   item.NamaFile,
			Keterangan: item.Keterangan,
			CreatedAt:  item.CreatedAt,
		})
	}
	utils.Success(w, "ok", out)
}

func downloadAbsensiDokumen(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.AbsensiDokumen
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessAbsensiDokumen(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.File) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Type", dokumenContentType(item.NamaFile))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", item.NamaFile))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.NamaFile))
	}
	w.Write(item.File)
}

func deleteAbsensiDokumen(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.AbsensiDokumen
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data tidak ditemukan")
		return
	}
	if !canAccessAbsensiDokumen(claims, item) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus surat: "+err.Error())
		return
	}
	utils.Success(w, "surat pengganti berhasil dihapus", nil)
}
