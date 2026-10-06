package handlers

import (
	"bytes"
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
