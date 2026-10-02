package handlers

import (
	"encoding/json"
	"net/http"
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
}

func toKopSuratSekolahOut(uk models.UnitKerja) kopSuratSekolahOut {
	out := kopSuratSekolahOut{
		IDUnitKerja:          uk.ID,
		Unit:                 uk.Unit,
		PerataanKop:          models.PerataanKopTengah,
		TampilkanLogoTutwuri: uk.TampilkanLogoTutwuri,
		LogoTutwuriTersedia:  len(assets.TutWuriPNG) > 0,
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

// RegisterKopSuratRoutes mendaftarkan endpoint /api/kop-surat-sekolah --
// KHUSUS role "atasan" (lihat komentar di atas file ini perihal cakupan
// fitur ini).
func RegisterKopSuratRoutes(mux *http.ServeMux, db *gorm.DB) {
	atasanOnly := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("atasan"))
	}
	mux.Handle("GET /api/kop-surat-sekolah", atasanOnly(func(w http.ResponseWriter, r *http.Request) { getKopSuratSekolah(w, r, db) }))
	mux.Handle("PUT /api/kop-surat-sekolah", atasanOnly(func(w http.ResponseWriter, r *http.Request) { updateKopSuratSekolah(w, r, db) }))
}
