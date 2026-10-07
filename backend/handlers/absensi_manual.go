package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// absensi_manual.go: input/edit absen manual oleh ADMINISTRATOR SAJA (bukan
// admin biasa, bukan akun IsAdminAbsensi) untuk tanggal yang belum/tidak ada
// absennya sama sekali, atau untuk memperbaiki baris absen yang sudah ada.
// Dipisahkan sengaja dari peran "admin"/IsAdminAbsensi karena menu ini bisa
// membuat/menimpa data absen pegawai lain secara langsung tanpa verifikasi
// kamera/lokasi/kedipan mata sama sekali -- lihat RegisterAbsensiRoutes di
// absensi.go (dibungkus administratorOnly, BUKAN manage/pegawaiOnly).
//
// Berbeda dari absenMasuk/absenPulang (dipakai pegawai sendiri lewat kamera
// HP, lihat absensi.go), endpoint ini SENGAJA TIDAK memvalidasi jendela jam
// absen (jamAbsenUntukPegawai), radius kantor, maupun jam kerja/hari libur --
// administrator mengisi data historis untuk tanggal apa pun atas tanggung
// jawabnya sendiri. TerlambatMenit juga diisi LANGSUNG oleh administrator
// sebagai angka (bukan dihitung otomatis dari jam), karena jamAbsenUntukPegawai
// bergantung pada HARI INI (absensiNow().Weekday()) untuk menentukan jam
// kerja Shift Kerja yang berlaku -- memakainya untuk tanggal mundur/maju bisa
// diam-diam memakai jadwal hari yang salah.
func absensiManualInputHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims == nil {
		utils.Error(w, http.StatusUnauthorized, "sesi tidak valid")
		return
	}

	utils.LimitBody(w, r, 16<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 16MB)")
		return
	}

	idPegawai := parseUintForm(r, "id_pegawai")
	if idPegawai == 0 {
		utils.Error(w, http.StatusBadRequest, "id_pegawai wajib diisi")
		return
	}
	var pegawai models.Pegawai
	if err := db.First(&pegawai, idPegawai).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	tanggalStr := strings.TrimSpace(r.FormValue("tanggal"))
	if tanggalStr == "" {
		utils.Error(w, http.StatusBadRequest, "tanggal wajib diisi")
		return
	}
	tanggal, err := time.ParseInLocation("2006-01-02", tanggalStr, absensiLocation())
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "format tanggal tidak valid (harus YYYY-MM-DD)")
		return
	}
	tanggal = time.Date(tanggal.Year(), tanggal.Month(), tanggal.Day(), 0, 0, 0, 0, tanggal.Location())

	jamMasukStr := strings.TrimSpace(r.FormValue("jam_masuk"))
	jamPulangStr := strings.TrimSpace(r.FormValue("jam_pulang"))
	if jamMasukStr == "" && jamPulangStr == "" {
		utils.Error(w, http.StatusBadRequest, "isi minimal salah satu: jam masuk atau jam pulang")
		return
	}

	var jamMasuk, jamPulang *time.Time
	if jamMasukStr != "" {
		menit, ok := parseJamToMinutes(jamMasukStr)
		if !ok {
			utils.Error(w, http.StatusBadRequest, "format jam masuk tidak valid (harus HH:MM)")
			return
		}
		t := tanggal.Add(time.Duration(menit) * time.Minute)
		jamMasuk = &t
	}
	if jamPulangStr != "" {
		menit, ok := parseJamToMinutes(jamPulangStr)
		if !ok {
			utils.Error(w, http.StatusBadRequest, "format jam pulang tidak valid (harus HH:MM)")
			return
		}
		t := tanggal.Add(time.Duration(menit) * time.Minute)
		jamPulang = &t
	}

	terlambat := int(parseUintForm(r, "terlambat_menit"))

	var existing models.Absensi
	found := db.Where("id_pegawai = ? AND tanggal = ?", idPegawai, tanggal).First(&existing).Error == nil

	// Lindungi absen ASLI (sudah absen masuk & pulang sungguhan, lihat
	// absensiDianggapHadir di absensi.go) dari tertimpa lewat form ini --
	// permintaan pengguna: "jika nama pegawai telah melakukan absensi
	// masuk dan absensi pulang walaupun namanya di input maka absensi
	// masuk dan absensi pulang tidak akan tertimpa", SAMA aturan yang
	// sudah berlaku untuk input Berita Acara/Surat Tugas/Surat Kolektif
	// (lihat buatBeritaAcara, updateBeritaAcara & inputAbsensiDokumenKolektif)
	// -- supaya konsisten di SEMUA jalur yang bisa menimpa data absen.
	// Baris TAP (absen masuk tanpa absen pulang, bukan dinas dalam, tanggal
	// sudah lewat) TETAP dianggap "belum hadir" oleh absensiDianggapHadir,
	// jadi tetap boleh dilengkapi/diperbaiki lewat form ini -- hanya baris
	// yang SUDAH benar-benar hadir penuh yang ditolak di sini. Kalau admin
	// memang harus mengganti absen asli ini, hapus dulu lewat tombol Hapus
	// (DELETE /absensi/manual/{id}) sebelum menginput ulang.
	if found && absensiDianggapHadir(existing) {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf(
			"%s sudah memiliki absen masuk dan pulang (hadir) pada tanggal ini -- data absen asli tidak bisa ditimpa lewat input manual. Hapus dulu absen yang ada (tombol Hapus) kalau memang harus diganti.",
			pegawai.Nama))
		return
	}

	if !found {
		existing = models.Absensi{
			IDPegawai: idPegawai,
			Tanggal:   tanggal,
		}
	}

	if jamMasuk != nil {
		existing.JamMasuk = jamMasuk
		existing.TerlambatMenit = terlambat
	}
	if jamPulang != nil {
		existing.JamPulang = jamPulang
	}
	existing.IsManual = true
	existing.DiinputOlehNama = claims.Username

	// Foto bersifat OPSIONAL untuk input manual (berbeda dari absen mandiri
	// pegawai yang mewajibkan foto kamera) -- hanya diproses/diganti kalau
	// administrator benar-benar mengirim file baru, supaya foto lama (kalau
	// ada) tidak tertimpa jadi kosong ketika admin cuma mengedit jam.
	if fotoBytes, ok := absensiManualDecodeFoto(r, "foto_masuk"); ok {
		if fotoBytes == nil {
			utils.Error(w, http.StatusBadRequest, "berkas foto masuk bukan gambar yang valid")
			return
		}
		existing.FotoMasuk = fotoBytes
	}
	if fotoBytes, ok := absensiManualDecodeFoto(r, "foto_pulang"); ok {
		if fotoBytes == nil {
			utils.Error(w, http.StatusBadRequest, "berkas foto pulang bukan gambar yang valid")
			return
		}
		existing.FotoPulang = fotoBytes
	}

	if found {
		if err := db.Save(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen: "+err.Error())
			return
		}
	} else {
		if err := db.Create(&existing).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan absen: "+err.Error())
			return
		}
	}

	existing.FotoMasuk = nil
	existing.FotoPulang = nil
	utils.Created(w, "absen manual berhasil disimpan", existing)
}

// absensiManualDeleteHandler menghapus data absen -- KHUSUS administrator
// (lihat administratorOnly di RegisterAbsensiRoutes, absensi.go), dipakai
// tombol hapus pada kolom Aksi di dialog Detail Absen (RekapAbsensiView.vue).
// Query param "bagian" menentukan cakupannya:
//   - "pulang": HANYA menghapus absen pulang (jam, foto, koordinat, kedipan,
//     dinas dalam pulang) pada baris itu -- absen masuknya tetap ada, baris
//     tidak dihapus. Dipakai kalau admin cuma salah input/mau mengosongkan
//     absen pulang pegawai tapi absen masuknya tetap benar.
//   - "semua": menghapus SELURUH baris absen hari itu (masuk & pulang
//     sekaligus) -- tanggal itu otomatis kembali tampil di daftar "Tidak
//     Melakukan Absensi" pada rekap bulan tersebut.
//
// Berlaku untuk SEMUA baris absensi, bukan cuma yang IsManual=true -- baris
// absen mandiri pegawai lewat kamera pun bisa dihapus lewat sini, sama
// seperti tombol edit (Aksi) pada baris yang sama juga berlaku untuk semua
// baris, bukan cuma yang input manual.
func absensiManualDeleteHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.Absensi
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data absen tidak ditemukan")
		return
	}

	bagian := strings.TrimSpace(r.URL.Query().Get("bagian"))
	switch bagian {
	case "pulang":
		if item.JamPulang == nil {
			utils.Error(w, http.StatusBadRequest, "baris ini belum ada absen pulang")
			return
		}
		updates := map[string]interface{}{
			"jam_pulang":         nil,
			"foto_pulang":        nil,
			"lat_pulang":         nil,
			"lng_pulang":         nil,
			"kedipan_pulang_ok":  true,
			"dinas_dalam_pulang": false,
		}
		if err := db.Model(&item).Updates(updates).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menghapus absen pulang: "+err.Error())
			return
		}
		utils.Success(w, "absen pulang berhasil dihapus", nil)
	case "semua":
		if err := db.Delete(&item).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menghapus absen: "+err.Error())
			return
		}
		utils.Success(w, "absen tanggal ini berhasil dihapus", nil)
	default:
		utils.Error(w, http.StatusBadRequest, "parameter bagian harus 'pulang' atau 'semua'")
		return
	}
}

// absensiManualDecodeFoto membaca file multipart pada field key (kalau ada),
// memvalidasi ekstensi & isinya benar-benar gambar, lalu mengubah ukurannya
// (resizeImageBox, lebar maksimal 1000px, sama seperti foto absen mandiri
// pegawai) dan meng-encode ulang sebagai JPEG. Mengembalikan ok=false kalau
// field-nya memang tidak dikirim (artinya: biarkan foto lama, tidak diubah),
// dan fotoBytes=nil kalau field DIKIRIM tapi isinya bukan gambar yang valid
// (pemanggil menolak requestnya, bukan diam-diam menyimpan data rusak).
func absensiManualDecodeFoto(r *http.Request, key string) (fotoBytes []byte, ok bool) {
	fh := formFileHeader(r, key)
	if fh == nil {
		return nil, false
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil, true
	}
	f, err := fh.Open()
	if err != nil {
		return nil, true
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil || len(raw) == 0 {
		return nil, true
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, true
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resizeImageBox(src, 1000), &jpeg.Options{Quality: 85}); err != nil {
		return nil, true
	}
	return buf.Bytes(), true
}
