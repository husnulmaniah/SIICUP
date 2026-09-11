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
// administrator/admin (bisa kolektif -- beberapa pegawai & rentang tanggal
// sekaligus lewat inputAbsensiDokumenKolektif), tidak lagi diupload sendiri
// oleh pegawai -- pegawai hanya bisa melihat/mengunduh surat yang sudah
// diinput untuknya (listAbsensiDokumenSaya/downloadAbsensiDokumen).

var absensiDokumenLabels = map[string]string{
	models.AbsensiDokumenSKS:         "Surat Keterangan Sakit (SKS)",
	models.AbsensiDokumenSuratTugas:  "Surat Tugas",
	models.AbsensiDokumenBeritaAcara: "Berita Acara",
	models.AbsensiDokumenSuratIzin:   "Surat Izin",
}

func canAccessAbsensiDokumen(claims *utils.Claims, item models.AbsensiDokumen) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
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
	label, valid := absensiDokumenLabels[jenis]
	if !valid {
		utils.Error(w, http.StatusBadRequest, "jenis surat tidak valid (pilih: sks, surat_tugas, berita_acara, atau surat_izin)")
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
	keterangan := strings.TrimSpace(r.FormValue("keterangan"))

	// Pola hari kerja dihitung PER PEGAWAI dari tempat tugasnya (lihat
	// sixDayWeekForTempatTgs di pengajuan_cuti.go): pegawai yang bertugas di
	// sekolah masuk Senin-Sabtu (hanya Minggu yang libur), pegawai kantor
	// dinas masuk Senin-Jumat (Sabtu & Minggu libur). Tanggal merah juga
	// dilewati untuk keduanya. Tanggal di luar hari kerja TIDAK diinput
	// walaupun ikut dipilih admin dalam rentang tanggal -- aturannya sama
	// persis dengan perhitungan "tanggal terlewat" pada rekap absen, supaya
	// tidak ada surat untuk hari yang memang bukan hari kerja.
	var pegawaiTerpilih []models.Pegawai
	if err := db.Where("id IN ?", idList).Find(&pegawaiTerpilih).Error; err != nil || len(pegawaiTerpilih) == 0 {
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
		hariKerja := workingDaysWithHolidaySet(tglMulai, tglSelesai, sixDayWeekForTempatTgs(p.TempatTgs), holidaySet)
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

// listAbsensiDokumenAdmin dipakai administrator/admin untuk melihat/mengelola
// semua surat yang sudah diinput pada satu bulan (opsional filter
// id_pegawai) -- dipakai di halaman Rekap Absen supaya admin tahu surat apa
// yang sudah ada sebelum menginput lagi, dan bisa menghapusnya bila salah.
func listAbsensiDokumenAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
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

	query := db.Omit("file").Where("tanggal BETWEEN ? AND ?", start, end).Preload("Pegawai")
	if idStr := strings.TrimSpace(r.URL.Query().Get("id_pegawai")); idStr != "" {
		query = query.Where("id_pegawai = ?", idStr)
	}
	items := []models.AbsensiDokumen{}
	if err := query.Order("tanggal desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.Success(w, "ok", items)
}

func listAbsensiDokumenSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Success(w, "ok", []models.AbsensiDokumen{})
		return
	}
	items := []models.AbsensiDokumen{}
	if err := db.Omit("file").Where("id_pegawai = ?", *claims.IDPegawai).
		Order("tanggal desc").Find(&items).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}
	utils.Success(w, "ok", items)
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
