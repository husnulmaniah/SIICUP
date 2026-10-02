package handlers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"cuti-app/assets"
	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// kop_surat.go menangani menu "Kop Surat Sekolah" -- KHUSUS akun atasan
// (Kepala Sekolah/Kepala Puskesmas) yang bertugas di unit kerja Sekolah/
// Puskesmas (lihat isSekolahPegawai di pengajuan_cuti.go). Atasan bisa
// mengajukan/mengatur SENDIRI kop surat sekolahnya (nama sekolah, alamat
// kop kiri/kanan, perataan teks, & opsi logo Tut Wuri Handayani di kanan)
// yang dipakai pada Lampiran 3 (lihat drawLetterheadUnitKerjaKustom &
// buildSuratRekomendasiSekolah di handlers/surat_rekomendasi.go).
//
// BERBEDA dari "Perubahan Data Pegawai": perubahan di sini LANGSUNG
// berlaku begitu disimpan, TIDAK ada proses persetujuan admin -- data
// disimpan langsung ke baris UnitKerja milik sekolah tersebut.

// kopSuratSekolahOut adalah bentuk respons GET/PUT -- field apa adanya dari
// models.UnitKerja (hanya yang relevan untuk kop surat) ditambah nama unit
// kerja untuk ditampilkan di form.
type kopSuratSekolahOut struct {
	IDUnitKerja          uint   `json:"id_unit_kerja"`
	Unit                 string `json:"unit"`
	NamaSekolahKop       string `json:"nama_sekolah_kop"`
	AlamatKopKiri        string `json:"alamat_kop_kiri"`
	AlamatKopKanan       string `json:"alamat_kop_kanan"`
	PerataanKop          string `json:"perataan_kop"`
	TampilkanLogoTutwuri bool   `json:"tampilkan_logo_tutwuri"`
	// LogoTutwuriTersedia: true kalau berkas logo Tut Wuri Handayani sudah
	// ditambahkan ke sistem (assets.TutWuriPNG) -- kalau false, frontend
	// SEBAIKNYA tetap mengizinkan centang opsinya (akan otomatis tampil
	// begitu berkasnya ditambahkan admin sistem) tapi beri catatan bahwa
	// logonya belum aktif.
	LogoTutwuriTersedia bool `json:"logo_tutwuri_tersedia"`
	// LogoKiriAda/LogoKananAda: true kalau sekolah sudah mengupload logo
	// kustomnya SENDIRI di slot itu (lihat models.UnitKerja.LogoKiriFile/
	// LogoKananFile) -- dipakai frontend untuk menampilkan pratinjau
	// (lewat GET /api/kop-surat-sekolah/logo/{sisi}) & tombol "Hapus Logo",
	// isi file-nya SENDIRI (bytea) sengaja TIDAK disertakan di sini.
	LogoKiriAda  bool `json:"logo_kiri_ada"`
	LogoKananAda bool `json:"logo_kanan_ada"`
}

func toKopSuratSekolahOut(uk models.UnitKerja) kopSuratSekolahOut {
	out := kopSuratSekolahOut{
		IDUnitKerja:          uk.ID,
		Unit:                 uk.Unit,
		PerataanKop:          models.PerataanKopTengah,
		TampilkanLogoTutwuri: uk.TampilkanLogoTutwuri,
		LogoTutwuriTersedia:  len(assets.TutWuriPNG) > 0,
		LogoKiriAda:          len(uk.LogoKiriFile) > 0,
		LogoKananAda:         len(uk.LogoKananFile) > 0,
	}
	if uk.NamaSekolahKop != nil {
		out.NamaSekolahKop = *uk.NamaSekolahKop
	}
	if uk.AlamatKopKiri != nil {
		out.AlamatKopKiri = *uk.AlamatKopKiri
	}
	if uk.AlamatKopKanan != nil {
		out.AlamatKopKanan = *uk.AlamatKopKanan
	}
	if uk.PerataanKop != nil && strings.TrimSpace(*uk.PerataanKop) != "" {
		out.PerataanKop = *uk.PerataanKop
	}
	return out
}

// unitKerjaSekolahAtasan mencari baris UnitKerja milik akun atasan yang
// login -- WAJIB pegawai (claims.IDPegawai) terhubung & bertugas di
// Sekolah/Puskesmas, kalau tidak dianggap tidak berhak memakai fitur ini.
func unitKerjaSekolahAtasan(db *gorm.DB, claims *utils.Claims) (models.UnitKerja, *models.Pegawai, error) {
	var uk models.UnitKerja
	if claims == nil || claims.IDPegawai == nil {
		return uk, nil, errNoAkses
	}
	var pegawai models.Pegawai
	if err := db.Preload("UnitKerja").First(&pegawai, "id = ?", *claims.IDPegawai).Error; err != nil {
		return uk, nil, errNoAkses
	}
	if !isSekolahPegawai(pegawai) || pegawai.UnitKerja == nil {
		return uk, nil, errBukanSekolah
	}
	return *pegawai.UnitKerja, &pegawai, nil
}

var errNoAkses = &kopSuratError{"akun anda tidak terhubung ke data pegawai manapun"}
var errBukanSekolah = &kopSuratError{"fitur kop surat ini hanya untuk atasan yang bertugas di Sekolah/Puskesmas"}

type kopSuratError struct{ msg string }

func (e *kopSuratError) Error() string { return e.msg }

func getKopSuratSekolah(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	uk, _, err := unitKerjaSekolahAtasan(db, claims)
	if err != nil {
		utils.Error(w, http.StatusForbidden, err.Error())
		return
	}
	utils.Success(w, "ok", toKopSuratSekolahOut(uk))
}

type updateKopSuratPayload struct {
	NamaSekolahKop       string `json:"nama_sekolah_kop"`
	AlamatKopKiri        string `json:"alamat_kop_kiri"`
	AlamatKopKanan       string `json:"alamat_kop_kanan"`
	PerataanKop          string `json:"perataan_kop"`
	TampilkanLogoTutwuri bool   `json:"tampilkan_logo_tutwuri"`
}

func updateKopSuratSekolah(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	uk, _, err := unitKerjaSekolahAtasan(db, claims)
	if err != nil {
		utils.Error(w, http.StatusForbidden, err.Error())
		return
	}
	var p updateKopSuratPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	perataan := strings.ToLower(strings.TrimSpace(p.PerataanKop))
	switch perataan {
	case models.PerataanKopKiri, models.PerataanKopKanan, models.PerataanKopTengah:
		// valid
	case "":
		perataan = models.PerataanKopTengah
	default:
		utils.Error(w, http.StatusBadRequest, `perataan kop surat harus salah satu dari "kiri", "kanan", atau "tengah"`)
		return
	}

	namaSekolahKop := strings.TrimSpace(p.NamaSekolahKop)
	alamatKopKiri := strings.TrimSpace(p.AlamatKopKiri)
	alamatKopKanan := strings.TrimSpace(p.AlamatKopKanan)

	updates := map[string]interface{}{
		"perataan_kop":           perataan,
		"tampilkan_logo_tutwuri": p.TampilkanLogoTutwuri,
	}
	if namaSekolahKop == "" {
		updates["nama_sekolah_kop"] = nil
	} else {
		updates["nama_sekolah_kop"] = namaSekolahKop
	}
	if alamatKopKiri == "" {
		updates["alamat_kop_kiri"] = nil
	} else {
		updates["alamat_kop_kiri"] = alamatKopKiri
	}
	if alamatKopKanan == "" {
		updates["alamat_kop_kanan"] = nil
	} else {
		updates["alamat_kop_kanan"] = alamatKopKanan
	}

	if err := db.Model(&models.UnitKerja{}).Where("id = ?", uk.ID).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan kop surat: "+err.Error())
		return
	}

	var fresh models.UnitKerja
	db.First(&fresh, uk.ID)
	utils.Success(w, "kop surat sekolah berhasil disimpan & langsung berlaku untuk Lampiran 3", toKopSuratSekolahOut(fresh))
}

// validLogoSisi membatasi path segment {sisi} pada endpoint logo kop surat
// di bawah ke dua slot yang didukung drawLetterheadUnitKerjaKustom (lihat
// handlers/surat_rekomendasi.go).
func validLogoSisi(sisi string) bool {
	return sisi == "kiri" || sisi == "kanan"
}

// getLogoKopSurat menyajikan logo kustom (kiri/kanan) sekolah milik akun
// atasan yang login sebagai gambar PNG langsung (bukan dibungkus
// utils.Success) -- sama seperti fotoProfilPegawai/ttdPegawaiPegawai di
// handlers/pegawai.go, supaya bisa langsung dipakai sebagai src <img>
// setelah diambil lewat axios (responseType 'blob') di frontend.
func getLogoKopSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	sisi := r.PathValue("sisi")
	if !validLogoSisi(sisi) {
		utils.Error(w, http.StatusBadRequest, "sisi logo tidak dikenal")
		return
	}
	uk, _, err := unitKerjaSekolahAtasan(db, claims)
	if err != nil {
		utils.Error(w, http.StatusForbidden, err.Error())
		return
	}
	data := uk.LogoKiriFile
	if sisi == "kanan" {
		data = uk.LogoKananFile
	}
	if len(data) == 0 {
		utils.Error(w, http.StatusNotFound, "belum ada logo kustom di sisi ini")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

// uploadLogoKopSurat menerima gambar logo (JPG/PNG) dari akun atasan,
// menyimpannya ulang sebagai PNG lebar maksimum 300px dengan kanal alpha
// dipertahankan (resizeImageBoxAlpha, fungsi yang SAMA dipakai
// uploadTtdPegawai di handlers/pegawai.go) supaya latar transparan logo
// tidak berubah jadi kotak putih/hitam, lalu menyimpannya sebagai
// LogoKiriFile/LogoKananFile pada baris UnitKerja sekolah ybs sesuai
// {sisi} -- begitu tersimpan, LANGSUNG menggantikan logo bawaan pada slot
// itu di kop surat Lampiran 3 (lihat drawLetterheadUnitKerjaKustom),
// TANPA perlu persetujuan admin.
func uploadLogoKopSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	sisi := r.PathValue("sisi")
	if !validLogoSisi(sisi) {
		utils.Error(w, http.StatusBadRequest, "sisi logo tidak dikenal")
		return
	}
	uk, _, err := unitKerjaSekolahAtasan(db, claims)
	if err != nil {
		utils.Error(w, http.StatusForbidden, err.Error())
		return
	}
	utils.LimitBody(w, r, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca file upload (maksimal 8MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "file tidak ditemukan (field 'file')")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "format logo harus JPG atau PNG")
		return
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membaca isi file")
		return
	}
	if len(raw) == 0 {
		utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
		return
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "berkas bukan gambar yang valid")
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, resizeImageBoxAlpha(src, 300)); err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memproses gambar")
		return
	}

	updates := map[string]interface{}{}
	if sisi == "kiri" {
		updates["logo_kiri_nama"] = header.Filename
		updates["logo_kiri_file"] = buf.Bytes()
	} else {
		updates["logo_kanan_nama"] = header.Filename
		updates["logo_kanan_file"] = buf.Bytes()
	}
	if err := db.Model(&models.UnitKerja{}).Where("id = ?", uk.ID).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan logo: "+err.Error())
		return
	}
	utils.Success(w, "logo berhasil disimpan & langsung berlaku untuk Lampiran 3", map[string]string{"nama_file": header.Filename})
}

// hapusLogoKopSurat mengembalikan slot logo ({sisi}) ke bawaan sistem (logo
// Kabupaten yang selalu tampil di kiri, atau kosong/Tut Wuri Handayani
// sesuai TampilkanLogoTutwuri di kanan) dengan menghapus logo kustom yang
// tersimpan di slot itu.
func hapusLogoKopSurat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	sisi := r.PathValue("sisi")
	if !validLogoSisi(sisi) {
		utils.Error(w, http.StatusBadRequest, "sisi logo tidak dikenal")
		return
	}
	uk, _, err := unitKerjaSekolahAtasan(db, claims)
	if err != nil {
		utils.Error(w, http.StatusForbidden, err.Error())
		return
	}
	updates := map[string]interface{}{}
	if sisi == "kiri" {
		updates["logo_kiri_nama"] = ""
		updates["logo_kiri_file"] = nil
	} else {
		updates["logo_kanan_nama"] = ""
		updates["logo_kanan_file"] = nil
	}
	if err := db.Model(&models.UnitKerja{}).Where("id = ?", uk.ID).Updates(updates).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus logo: "+err.Error())
		return
	}
	utils.Success(w, "logo berhasil dihapus, kembali memakai logo bawaan", nil)
}

// RegisterKopSuratRoutes mendaftarkan endpoint /api/kop-surat-sekolah --
// KHUSUS role "atasan" (lihat komentar di atas file ini perihal cakupan
// fitur ini).
func RegisterKopSuratRoutes(mux *http.ServeMux, db *gorm.DB) {
	atasanOnly := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("atasan"))
	}
	mux.Handle("GET /api/kop-surat-sekolah", atasanOnly(func(w http.ResponseWriter, r *http.Request) { getKopSuratSekolah(w, r, db) }))
	mux.Handle("PUT /api/kop-surat-sekolah", atasanOnly(func(w http.ResponseWriter, r *http.Request) { updateKopSuratSekolah(w, r, db) }))
	mux.Handle("GET /api/kop-surat-sekolah/logo/{sisi}", atasanOnly(func(w http.ResponseWriter, r *http.Request) { getLogoKopSurat(w, r, db) }))
	mux.Handle("POST /api/kop-surat-sekolah/logo/{sisi}", atasanOnly(func(w http.ResponseWriter, r *http.Request) { uploadLogoKopSurat(w, r, db) }))
	mux.Handle("DELETE /api/kop-surat-sekolah/logo/{sisi}", atasanOnly(func(w http.ResponseWriter, r *http.Request) { hapusLogoKopSurat(w, r, db) }))
}
