package handlers

import (
	"encoding/json"
	"net/http"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// peta_jabatan_dinas_pegawai.go: penempatan pegawai Dinas ke unit struktur
// (Sekretariat/Bidang/Seksi/Sub Bagian) pada pohon Peta Jabatan Dinas --
// lihat handlers/peta_jabatan_dinas.go untuk pohon struktur & kepala unit
// itu sendiri. File ini KHUSUS mengubah Pegawai.IDUnitKerja (siapa bertugas
// di unit mana), SELALU administrator ONLY (sama seperti seluruh endpoint
// "kelola" Peta Jabatan Dinas lain) -- berbeda dari menu Data Pegawai biasa
// yang dibuka untuk administrator/admin, supaya penempatan ke struktur
// Dinas yang baru ini tetap terjaga 100% di tangan administrator sesuai
// permintaan pengguna.

func RegisterPetaJabatanDinasPegawaiRoutes(mux *http.ServeMux, db *gorm.DB) {
	kelola := func(h http.HandlerFunc) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole("administrator"))
	}
	mux.Handle("GET /api/peta-jabatan/dinas/pegawai/belum-ditempatkan", kelola(func(w http.ResponseWriter, r *http.Request) { listPegawaiDinasBelumDitempatkan(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/dinas/pegawai/{id}/unit-kerja", kelola(func(w http.ResponseWriter, r *http.Request) { setPegawaiUnitKerjaDinasHandler(w, r, db) }))
}

// ============================================================
// daftar pegawai Dinas yang belum ditempatkan ke struktur
// ============================================================

type pegawaiDinasBelumDitempatkanOut struct {
	ID        uint   `json:"id"`
	NIP       string `json:"nip"`
	Nama      string `json:"nama"`
	Jabatan   string `json:"jabatan"`
	TempatTgs string `json:"tempat_tgs"`
	// IDUnitKerjaSaatIni: unit kerja pegawai ini SAAT INI, kalau ADA tapi
	// BUKAN bagian dari pohon struktur Dinas (mis. baris UnitKerja lama
	// sebelum pohon ini dibuat) -- nil kalau benar-benar belum punya unit
	// kerja sama sekali. Ditampilkan supaya administrator tahu dari mana
	// pegawai ini harus "dipindahkan", bukan hanya "ditempatkan".
	IDUnitKerjaSaatIni *uint  `json:"id_unit_kerja_saat_ini"`
	UnitKerjaSaatIni   string `json:"unit_kerja_saat_ini"`
}

// listPegawaiDinasBelumDitempatkan menangani GET
// /api/peta-jabatan/dinas/pegawai/belum-ditempatkan -- daftar pegawai AKTIF
// (bukan Pensiun/Mutasi, lihat scopePegawaiAktifDinas) yang BUKAN pegawai
// sekolah (dicek dengan isSekolahPegawai, PERSIS logika yang sama dipakai
// jam kerja/syarat cuti, supaya definisi "pegawai Dinas" konsisten di
// seluruh sistem) TAPI unit kerjanya SAAT INI belum/bukan salah satu node
// pohon struktur Peta Jabatan Dinas (UnitKerja.TipeStrukturDinas kosong) --
// baik karena belum punya unit kerja sama sekali, maupun karena masih
// menunjuk ke baris Unit Kerja lama di luar pohon baru ini. Administrator
// memakai daftar ini sebagai "pekerjaan rumah" penempatan.
func listPegawaiDinasBelumDitempatkan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var pegawaiRows []models.Pegawai
	scopePegawaiAktifDinas(db, db.Preload("UnitKerja").Preload("Jabatan")).
		Order("nama asc").Find(&pegawaiRows)

	out := make([]pegawaiDinasBelumDitempatkanOut, 0)
	for _, p := range pegawaiRows {
		if isSekolahPegawai(p) {
			continue
		}
		sudahDiStrukturDinas := p.UnitKerja != nil && p.UnitKerja.TipeStrukturDinas != nil && *p.UnitKerja.TipeStrukturDinas != ""
		if sudahDiStrukturDinas {
			continue
		}
		jabatan := "-"
		if p.Jabatan != nil {
			jabatan = p.Jabatan.Jabatan
		}
		unitSaatIni := ""
		if p.UnitKerja != nil {
			unitSaatIni = p.UnitKerja.Unit
		}
		out = append(out, pegawaiDinasBelumDitempatkanOut{
			ID: p.ID, NIP: p.NIP, Nama: p.Nama, Jabatan: jabatan, TempatTgs: p.TempatTgs,
			IDUnitKerjaSaatIni: p.IDUnitKerja, UnitKerjaSaatIni: unitSaatIni,
		})
	}
	utils.Success(w, "ok", out)
}

// ============================================================
// penempatan pegawai ke unit struktur Dinas
// ============================================================

type pegawaiUnitKerjaDinasPayload struct {
	IDUnitKerja *uint `json:"id_unit_kerja"`
}

// setPegawaiUnitKerjaDinasHandler menangani PUT
// /api/peta-jabatan/dinas/pegawai/{id}/unit-kerja -- menempatkan (atau
// mengosongkan, kalau id_unit_kerja dikirim null) SATU pegawai ke SATU node
// pohon struktur Dinas. id_unit_kerja (kalau tidak null) WAJIB benar-benar
// salah satu node struktur Dinas (TempatKerja == dinas) -- endpoint ini
// SENGAJA tidak dibatasi lebih jauh ke node Dinas yang "sudah masuk pohon"
// (TipeStrukturDinas terisi) supaya administrator tetap bisa menempatkan ke
// baris Unit Kerja Dinas lama (di luar pohon) kalau memang perlu, meski
// kasus itu seharusnya jarang terjadi setelah pohon ini ada.
//
// Begitu ditempatkan, Atasan Langsung pegawai ini diisi OTOMATIS dari
// Atasan Langsung unit tujuan (reuse autoFillAtasanDariUnitKerja, PERSIS
// logika yang sama dipakai createPegawai/updatePegawai biasa) -- TAPI HANYA
// kalau pegawai ini belum punya Atasan Langsung sama sekali; pilihan atasan
// yang sudah ada (mis. dari penempatan sebelumnya) tidak pernah ditimpa
// diam-diam oleh aksi ini.
func setPegawaiUnitKerjaDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.Pegawai
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pegawai tidak ditemukan")
		return
	}
	var p pegawaiUnitKerjaDinasPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.IDUnitKerja != nil {
		var uk models.UnitKerja
		if err := db.Select("id", "tempat_kerja").First(&uk, "id = ?", *p.IDUnitKerja).Error; err != nil {
			utils.Error(w, http.StatusBadRequest, "unit kerja tidak ditemukan")
			return
		}
		if uk.TempatKerja == nil || *uk.TempatKerja != models.TempatKerjaDinas {
			utils.Error(w, http.StatusBadRequest, "unit kerja tujuan bukan unit Dinas")
			return
		}
	}
	item.IDUnitKerja = p.IDUnitKerja
	autoFillAtasanDariUnitKerja(db, &item)
	if err := db.Model(&models.Pegawai{}).Where("id = ?", item.ID).
		Updates(map[string]interface{}{"id_unit_kerja": item.IDUnitKerja, "id_atasan": item.IDAtasan}).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan penempatan: "+err.Error())
		return
	}
	utils.Success(w, "penempatan pegawai berhasil disimpan", nil)
}
