package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// peta_jabatan_dinas.go menangani tab "Peta Jabatan Dinas" (menu Peta
// Jabatan) -- permintaan pengguna: pohon struktur organisasi Dinas
// Pendidikan dan Kebudayaan (Kepala Dinas -> Sekretariat/Bidang -> Sub
// Bagian/Seksi) lengkap dengan garis komando & tabel B (Bezetting)/K
// (Kebutuhan) per unit, SENGAJA DIPISAH TOTAL dari handlers/peta_jabatan.go
// (Peta Jabatan Sekolah) alih-alih memakai ulang endpoint yang sama, karena
// hak kelolanya BEDA & LEBIH KETAT:
//   - Peta Jabatan Sekolah: administrator/admin/atasan (sekolahnya sendiri)
//     boleh MENGELOLA.
//   - Peta Jabatan Dinas: HANYA administrator boleh MENGELOLA (termasuk
//     admin TIDAK boleh -- cukup MELIHAT, sama seperti atasan & pegawai
//     Dinas). Memakai ulang closure "kelola" Sekolah yang sudah mengizinkan
//     admin/atasan akan salah & berisiko regresi pada Peta Jabatan Sekolah
//     yang sudah stabil, karena itulah closure & handler di file ini
//     SENGAJA baru/berdiri sendiri sama sekali.
//
// Struktur pohonnya sendiri memakai models.UnitKerja yang SAMA (kolom
// id_induk_unit_kerja & tipe_struktur_dinas, lihat komentar lengkap di
// models.go) -- satu-satunya baris berjenis StrukturDinasRoot dibuat sekali
// lewat SeedPetaJabatanDinas (database/seed_peta_jabatan_dinas.go) dari data
// ANJAB yang diunggah pengguna; SELURUH perubahan struktur SETELAHNYA (tambah/
// ubah/hapus Sekretariat/Bidang/Seksi/Sub Bagian, atau tambah cabang baru)
// lewat endpoint-endpoint di file ini.
//
// "Kepala" setiap unit (siapa yang menjabat) memakai mekanisme
// UnitKerja.IDAtasanLangsung yang SAMA dengan Peta Jabatan Sekolah (bulk
// menyamakan Atasan Langsung ke seluruh pegawai yang bertugas di unit itu) --
// KECUALI untuk node akar (Kepala Dinas/Plt. Kepala Dinas), yang TETAP
// memakai sumber kebenaran terpusat resolveSignerFullRekomendasi/
// PengaturanSurat (sama seperti tanda tangan Kepala Dinas di surat-surat
// lain, lihat Phase E) -- BUKAN IDAtasanLangsung, supaya tidak ada DUA
// tempat berbeda yang bisa saling tidak sinkron soal siapa Kepala Dinas saat
// ini. Lihat setKepalaStrukturDinasHandler untuk bagaimana "garis komando"
// (Kepala Seksi melapor ke Kepala Bidang, Kepala Bidang/Sekretaris melapor
// ke Kepala Dinas, dst) disebarkan ke Pegawai.IDAtasan begitu seorang kepala
// unit ditetapkan -- PERSIS permintaan pengguna: "buat agar memiliki garis
// komando sesuai dengan unit kerjanya".
//
// Tabel B/K per unit memakai ULANG hitungPetaJabatanSekolah (generik,
// menerima idUnitKerja APA PUN) dari handlers/peta_jabatan.go -- Dinas
// SENGAJA TIDAK memakai SubJabatan sama sekali (data ANJAB tidak menunjukkan
// kelas jabatan yang berjenjang per unit untuk jabatan staf yang sama,
// berbeda dari Guru di Sekolah), jadi seluruh FormasiJabatan Dinas selalu
// IDSubJabatan NULL.

func RegisterPetaJabatanDinasRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	// lihat: SIAPA PUN yang berstatus aktif boleh melihat pohon struktur
	// Dinas (satu pohon TUNGGAL untuk seluruh instansi, beda dari Sekolah
	// yang per-sekolah & dibatasi ke sekolah sendiri untuk atasan/pegawai) --
	// penyaringan "hanya pegawai Dinas yang melihat tab ini" sudah ditangani
	// di frontend (lihat canSeeDinasTab pada PetaJabatanView.vue).
	lihat := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan", "pegawai") }
	// kelola: permintaan pengguna -- HANYA administrator (admin TIDAK
	// termasuk, beda dengan Peta Jabatan Sekolah).
	kelola := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator") }

	mux.Handle("GET /api/peta-jabatan/dinas", lihat(func(w http.ResponseWriter, r *http.Request) { getPetaJabatanDinas(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/dinas/struktur/daftar", kelola(func(w http.ResponseWriter, r *http.Request) { daftarStrukturDinasHandler(w, r, db) }))

	mux.Handle("POST /api/peta-jabatan/dinas/struktur", kelola(func(w http.ResponseWriter, r *http.Request) { createStrukturDinasHandler(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/dinas/struktur/{id}", kelola(func(w http.ResponseWriter, r *http.Request) { updateStrukturDinasHandler(w, r, db) }))
	mux.Handle("DELETE /api/peta-jabatan/dinas/struktur/{id}", kelola(func(w http.ResponseWriter, r *http.Request) { deleteStrukturDinasHandler(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/dinas/struktur/{id}/kepala", kelola(func(w http.ResponseWriter, r *http.Request) { setKepalaStrukturDinasHandler(w, r, db) }))

	mux.Handle("PUT /api/peta-jabatan/dinas/jabatan/{id}/kelas-jabatan", kelola(func(w http.ResponseWriter, r *http.Request) { updateJabatanKJDinasHandler(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/dinas/formasi", kelola(func(w http.ResponseWriter, r *http.Request) { setFormasiJabatanDinasHandler(w, r, db) }))
}

// ============================================================
// pohon struktur + tabel B/K + kepala per node
// ============================================================

type kepalaDinasOut struct {
	Nama       string `json:"nama"`
	NIP        string `json:"nip"`
	Jabatan    string `json:"jabatan"`
	PangkatGol string `json:"pangkat_gol"`
	// IDPegawai: nil UNTUK NODE AKAR (Kepala Dinas/Plt. Kepala Dinas --
	// lihat komentar di atas file, sumbernya PengaturanSurat/NIP, bukan
	// baris Pegawai yang terikat langsung ke node ini) -- frontend memakai
	// nil ini untuk tahu tombol "Ganti Kepala" tidak berlaku di node akar.
	IDPegawai *uint `json:"id_pegawai"`
}

type nodeDinasOut struct {
	ID     uint             `json:"id"`
	Unit   string           `json:"unit"`
	Tipe   string           `json:"tipe"`
	Kepala *kepalaDinasOut  `json:"kepala"`
	Rows   []jabatanPetaOut `json:"rows"`
	Anak   []nodeDinasOut   `json:"anak"`
}

type petaJabatanDinasOut struct {
	BolehKelola bool         `json:"boleh_kelola"`
	Root        nodeDinasOut `json:"root"`
}

// kepalaDinasDariPegawai: bentuk keluaran kepalaDinasOut dari satu baris
// Pegawai (dipakai untuk node SELAIN akar, lihat IDAtasanLangsung).
func kepalaDinasDariPegawai(pegawai models.Pegawai) kepalaDinasOut {
	jabatan := "-"
	if pegawai.Jabatan != nil {
		jabatan = pegawai.Jabatan.Jabatan
	}
	pangkatGol := "-"
	if pegawai.PangkatGol != nil {
		pk, gol := "-", "-"
		if pegawai.PangkatGol.Pangkat != nil {
			pk = pegawai.PangkatGol.Pangkat.Pangkat
		}
		if pegawai.PangkatGol.Gol != nil {
			gol = pegawai.PangkatGol.Gol.Gol
		}
		pangkatGol = pk + " / " + gol
	}
	id := pegawai.ID
	return kepalaDinasOut{Nama: pegawai.Nama, NIP: pegawai.NIP, Jabatan: jabatan, PangkatGol: pangkatGol, IDPegawai: &id}
}

// buildNodeDinasOut: bentuk SATU node (+ seluruh anaknya, rekursif) untuk
// keluaran GET /api/peta-jabatan/dinas.
func buildNodeDinasOut(db *gorm.DB, uk models.UnitKerja) (*nodeDinasOut, error) {
	peta, err := hitungPetaJabatanSekolah(db, uk.ID)
	if err != nil {
		return nil, err
	}
	tipe := ""
	if uk.TipeStrukturDinas != nil {
		tipe = *uk.TipeStrukturDinas
	}
	out := &nodeDinasOut{ID: uk.ID, Unit: uk.Unit, Tipe: tipe, Rows: peta.Rows, Anak: []nodeDinasOut{}}

	if tipe == models.StrukturDinasRoot {
		pengaturan := pengaturanSuratOrDefault(db)
		nama, nip, jabatan, pangkatGol, _ := resolveSignerFullRekomendasi(db, pengaturan)
		out.Kepala = &kepalaDinasOut{Nama: nama, NIP: nip, Jabatan: jabatan, PangkatGol: pangkatGol}
	} else if uk.IDAtasanLangsung != nil {
		var pegawai models.Pegawai
		if err := db.Preload("Jabatan").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
			First(&pegawai, "id = ?", *uk.IDAtasanLangsung).Error; err == nil {
			k := kepalaDinasDariPegawai(pegawai)
			out.Kepala = &k
		}
	}

	var anakRows []models.UnitKerja
	db.Where("id_induk_unit_kerja = ?", uk.ID).Order("unit asc").Find(&anakRows)
	for _, anak := range anakRows {
		anakOut, err := buildNodeDinasOut(db, anak)
		if err != nil {
			return nil, err
		}
		out.Anak = append(out.Anak, *anakOut)
	}
	return out, nil
}

// getPetaJabatanDinas menangani GET /api/peta-jabatan/dinas -- seluruh role
// yang diizinkan (lihat closure "lihat" di atas) melihat PERSIS pohon yang
// sama (tidak ada penyaringan per-pegawai seperti Sekolah, karena hanya ada
// SATU Dinas).
func getPetaJabatanDinas(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	var root models.UnitKerja
	err := db.Where("tempat_kerja = ? AND tipe_struktur_dinas = ?", models.TempatKerjaDinas, models.StrukturDinasRoot).
		First(&root).Error
	if err != nil {
		utils.Error(w, http.StatusNotFound, "struktur Peta Jabatan Dinas belum dibuat, hubungi administrator")
		return
	}
	out, err := buildNodeDinasOut(db, root)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal memuat struktur: "+err.Error())
		return
	}
	utils.Success(w, "ok", petaJabatanDinasOut{
		BolehKelola: claims != nil && claims.RoleName == "administrator",
		Root:        *out,
	})
}

// daftarStrukturDinasHandler menangani GET
// /api/peta-jabatan/dinas/struktur/daftar?tipe=... -- dipakai dialog
// "Tambah Struktur" administrator untuk memilih unit INDUK yang valid
// (mis. tipe=bidang untuk memilih Bidang induk saat menambah Seksi baru).
func daftarStrukturDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	tipe := strings.TrimSpace(r.URL.Query().Get("tipe"))
	query := db.Model(&models.UnitKerja{}).Where("tempat_kerja = ?", models.TempatKerjaDinas)
	if tipe != "" {
		query = query.Where("tipe_struktur_dinas = ?", tipe)
	}
	var rows []models.UnitKerja
	query.Select("id", "unit").Order("unit asc").Find(&rows)
	out := make([]sekolahPetaJabatanOut, 0, len(rows))
	for _, it := range rows {
		out = append(out, sekolahPetaJabatanOut{ID: it.ID, Unit: it.Unit})
	}
	utils.Success(w, "ok", out)
}

// ============================================================
// CRUD struktur (Sekretariat/Bidang/Seksi/Sub Bagian)
// ============================================================

// labelTipeStrukturDinas: label Indonesia untuk pesan error, supaya
// administrator langsung mengerti letak kesalahannya tanpa perlu membaca
// nilai mentah kode tipe.
func labelTipeStrukturDinas(tipe string) string {
	switch tipe {
	case models.StrukturDinasRoot:
		return "Dinas"
	case models.StrukturDinasSekretariat:
		return "Sekretariat"
	case models.StrukturDinasBidang:
		return "Bidang"
	case models.StrukturDinasSeksi:
		return "Seksi"
	case models.StrukturDinasSubBagian:
		return "Sub Bagian"
	}
	return tipe
}

// validateIndukTipeDinasStruktur memastikan GARIS KOMANDO tetap konsisten:
// Sekretariat & Bidang HANYA boleh langsung di bawah akar (Dinas); Sub
// Bagian HANYA di bawah Sekretariat; Seksi HANYA di bawah Bidang -- sesuai
// struktur nyata yang diminta pengguna, mencegah administrator membuat
// pohon yang rancu (mis. Seksi langsung di bawah Dinas, atau Bidang di
// bawah Bidang lain).
func validateIndukTipeDinasStruktur(db *gorm.DB, tipe string, idInduk uint) error {
	expectedIndukTipe := map[string]string{
		models.StrukturDinasSekretariat: models.StrukturDinasRoot,
		models.StrukturDinasBidang:      models.StrukturDinasRoot,
		models.StrukturDinasSubBagian:   models.StrukturDinasSekretariat,
		models.StrukturDinasSeksi:       models.StrukturDinasBidang,
	}
	expected, ok := expectedIndukTipe[tipe]
	if !ok {
		return fmt.Errorf("tipe struktur tidak dikenal")
	}
	var induk models.UnitKerja
	if err := db.Select("id", "tempat_kerja", "tipe_struktur_dinas").First(&induk, "id = ?", idInduk).Error; err != nil {
		return fmt.Errorf("unit induk tidak ditemukan")
	}
	if induk.TempatKerja == nil || *induk.TempatKerja != models.TempatKerjaDinas {
		return fmt.Errorf("unit induk bukan bagian struktur Peta Jabatan Dinas")
	}
	indukTipe := ""
	if induk.TipeStrukturDinas != nil {
		indukTipe = *induk.TipeStrukturDinas
	}
	if indukTipe != expected {
		return fmt.Errorf("%s harus berada langsung di bawah %s", labelTipeStrukturDinas(tipe), labelTipeStrukturDinas(expected))
	}
	return nil
}

type strukturDinasPayload struct {
	Unit             string `json:"unit"`
	Tipe             string `json:"tipe"`
	IDIndukUnitKerja *uint  `json:"id_induk_unit_kerja"`
}

// createStrukturDinasHandler menangani POST /api/peta-jabatan/dinas/struktur
// -- menambah SATU node baru (Sekretariat/Bidang/Seksi/Sub Bagian) ke pohon
// struktur Dinas yang sudah ada. Node akar (Dinas) TIDAK bisa ditambah lagi
// lewat sini -- sudah dibuat sekali oleh SeedPetaJabatanDinas & bersifat
// tunggal.
func createStrukturDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p strukturDinasPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	p.Unit = strings.TrimSpace(p.Unit)
	p.Tipe = strings.TrimSpace(p.Tipe)
	if p.Unit == "" {
		utils.Error(w, http.StatusBadRequest, "nama unit wajib diisi")
		return
	}
	if p.Tipe == models.StrukturDinasRoot {
		utils.Error(w, http.StatusBadRequest, "struktur akar (Dinas) sudah dibuat otomatis & tidak bisa ditambah lagi")
		return
	}
	if p.IDIndukUnitKerja == nil || *p.IDIndukUnitKerja == 0 {
		utils.Error(w, http.StatusBadRequest, "unit induk wajib dipilih")
		return
	}
	if err := validateIndukTipeDinasStruktur(db, p.Tipe, *p.IDIndukUnitKerja); err != nil {
		utils.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	tempatKerja := models.TempatKerjaDinas
	item := models.UnitKerja{
		Unit:              p.Unit,
		TempatKerja:       &tempatKerja,
		TipeStrukturDinas: &p.Tipe,
		IDIndukUnitKerja:  p.IDIndukUnitKerja,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menambah struktur: "+err.Error())
		return
	}
	utils.Created(w, "struktur berhasil ditambahkan", item)
}

type strukturDinasUpdatePayload struct {
	Unit             *string `json:"unit"`
	IDIndukUnitKerja *uint   `json:"id_induk_unit_kerja"`
}

// updateStrukturDinasHandler menangani PUT
// /api/peta-jabatan/dinas/struktur/{id} -- mengganti nama unit DAN/ATAU
// memindahkannya ke induk lain (keduanya opsional, tipe node sendiri
// SENGAJA tidak bisa diubah lewat sini -- hapus & buat ulang kalau memang
// perlu ganti tipe, supaya validasi garis komando tidak perlu menangani
// perubahan tipe sekaligus pemindahan induk dalam satu permintaan).
func updateStrukturDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.UnitKerja
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "struktur tidak ditemukan")
		return
	}
	if item.TempatKerja == nil || *item.TempatKerja != models.TempatKerjaDinas {
		utils.Error(w, http.StatusBadRequest, "unit ini bukan bagian struktur Peta Jabatan Dinas")
		return
	}
	if item.TipeStrukturDinas != nil && *item.TipeStrukturDinas == models.StrukturDinasRoot {
		utils.Error(w, http.StatusBadRequest, "nama struktur akar (Dinas) diatur lewat menu Unit Kerja biasa")
		return
	}
	var p strukturDinasUpdatePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.Unit != nil {
		nama := strings.TrimSpace(*p.Unit)
		if nama == "" {
			utils.Error(w, http.StatusBadRequest, "nama unit tidak boleh kosong")
			return
		}
		item.Unit = nama
	}
	if p.IDIndukUnitKerja != nil {
		if *p.IDIndukUnitKerja == item.ID {
			utils.Error(w, http.StatusBadRequest, "unit tidak bisa menjadi induk dirinya sendiri")
			return
		}
		tipe := models.StrukturDinasBidang
		if item.TipeStrukturDinas != nil {
			tipe = *item.TipeStrukturDinas
		}
		if err := validateIndukTipeDinasStruktur(db, tipe, *p.IDIndukUnitKerja); err != nil {
			utils.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		item.IDIndukUnitKerja = p.IDIndukUnitKerja
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "struktur berhasil disimpan", item)
}

// deleteStrukturDinasHandler menangani DELETE
// /api/peta-jabatan/dinas/struktur/{id} -- SENGAJA menolak menghapus node
// akar, node yang masih punya anak, ATAU node yang masih ditempati pegawai
// (Pegawai.IDUnitKerja), supaya administrator tidak bisa diam-diam
// menghilangkan cabang pohon yang masih "hidup" -- harus beres-beres
// (pindahkan pegawai & hapus anaknya dulu) secara eksplisit.
func deleteStrukturDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.UnitKerja
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "struktur tidak ditemukan")
		return
	}
	if item.TempatKerja == nil || *item.TempatKerja != models.TempatKerjaDinas {
		utils.Error(w, http.StatusBadRequest, "unit ini bukan bagian struktur Peta Jabatan Dinas")
		return
	}
	if item.TipeStrukturDinas != nil && *item.TipeStrukturDinas == models.StrukturDinasRoot {
		utils.Error(w, http.StatusBadRequest, "struktur akar (Dinas) tidak bisa dihapus")
		return
	}
	var jumlahAnak int64
	db.Model(&models.UnitKerja{}).Where("id_induk_unit_kerja = ?", item.ID).Count(&jumlahAnak)
	if jumlahAnak > 0 {
		utils.Error(w, http.StatusBadRequest, "hapus seluruh unit anak di bawahnya dahulu")
		return
	}
	var jumlahPegawai int64
	db.Model(&models.Pegawai{}).Where("id_unit_kerja = ?", item.ID).Count(&jumlahPegawai)
	if jumlahPegawai > 0 {
		utils.Error(w, http.StatusBadRequest, "pindahkan seluruh pegawai di unit ini dahulu sebelum menghapus")
		return
	}
	db.Where("id_unit_kerja = ?", item.ID).Delete(&models.FormasiJabatan{})
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus struktur: "+err.Error())
		return
	}
	utils.Success(w, "struktur berhasil dihapus", nil)
}

// ============================================================
// kepala unit & garis komando
// ============================================================

// resolveKepalaPegawaiIDDinas: ID Pegawai yang SAAT INI menjabat sebagai
// kepala unit "uk" -- utk node akar diresolusi lewat NIP pada
// PengaturanSurat (sama seperti resolveSignerFullRekomendasi), utk node
// lain langsung dari IDAtasanLangsung. nil kalau belum ada yang menjabat.
// Dipakai KHUSUS untuk komputasi garis komando (siapa atasan dari kepala
// unit anak) -- bukan untuk tampilan (lihat buildNodeDinasOut yang
// menampilkan detail lengkap lewat resolveSignerFullRekomendasi untuk akar,
// supaya nama/NIP yang tampil tetap bersumber dari PengaturanSurat apa adanya
// meski belum cocok dengan baris Pegawai manapun).
func resolveKepalaPegawaiIDDinas(db *gorm.DB, uk models.UnitKerja) *uint {
	tipe := ""
	if uk.TipeStrukturDinas != nil {
		tipe = *uk.TipeStrukturDinas
	}
	if tipe != models.StrukturDinasRoot {
		return uk.IDAtasanLangsung
	}
	nip := strings.TrimSpace(pengaturanSuratOrDefault(db).NipKepalaDinas)
	if nip == "" {
		return nil
	}
	var pegawai models.Pegawai
	if err := db.Select("id").Where("nip = ?", nip).First(&pegawai).Error; err != nil {
		return nil
	}
	id := pegawai.ID
	return &id
}

type kepalaDinasPayload struct {
	IDPegawai *uint `json:"id_pegawai"`
}

// setKepalaStrukturDinasHandler menangani PUT
// /api/peta-jabatan/dinas/struktur/{id}/kepala -- menetapkan (atau
// mengosongkan, kalau id_pegawai dikirim null) siapa yang menjabat sebagai
// kepala SATU unit (Sekretaris/Kepala Bidang/Kepala Seksi/Kepala Sub
// Bagian). Node akar (Kepala Dinas) SENGAJA ditolak di sini -- lihat
// komentar panjang di atas file.
//
// TIGA efek otomatis begitu kepala baru ditetapkan/diganti (permintaan
// pengguna: "buat agar memiliki garis komando sesuai dengan unit
// kerjanya"):
//  1. SELURUH pegawai yang SAAT INI bertugas di unit ini (Pegawai.
//     IDUnitKerja) disamakan Atasan Langsungnya ke kepala baru --
//     IDENTIK dengan syncAtasanUnitKerja pada Peta Jabatan Sekolah
//     (routes/master_routes.go), ditulis ulang di sini karena fungsi
//     aslinya ada di package routes (tidak bisa diimpor balik dari
//     package handlers tanpa membuat import siklik).
//  2. Kepala baru ITU SENDIRI diset Atasan Langsungnya ke kepala unit
//     INDUK-nya (kalau induk sudah punya kepala) -- inilah yang membuat
//     Kepala Seksi melapor ke Kepala Bidang, Kepala Bidang/Sekretaris
//     melapor ke Kepala Dinas, dst, BUKAN hanya staf biasa di tiap unit.
//  3. SEBALIKNYA (turun): seluruh anak LANGSUNG unit ini yang SUDAH lebih
//     dulu punya kepala sendiri (mis. Seksi sudah punya Kepala Seksi
//     SEBELUM Bidang-nya punya Kepala Bidang, atau Kepala Bidang baru
//     DIGANTI padahal Kepala Seksi di bawahnya sudah ada lebih dulu) ikut
//     disamakan Atasan Langsungnya ke kepala baru unit ini -- tanpa
//     langkah ini, garis komando anak yang sudah lebih dulu diisi akan
//     tertinggal/tidak sinkron begitu kepala unit di atasnya baru
//     ditetapkan atau diganti kemudian.
func setKepalaStrukturDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.UnitKerja
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "struktur tidak ditemukan")
		return
	}
	if item.TempatKerja == nil || *item.TempatKerja != models.TempatKerjaDinas {
		utils.Error(w, http.StatusBadRequest, "unit ini bukan bagian struktur Peta Jabatan Dinas")
		return
	}
	if item.TipeStrukturDinas != nil && *item.TipeStrukturDinas == models.StrukturDinasRoot {
		utils.Error(w, http.StatusBadRequest, "Kepala Dinas/Plt. Kepala Dinas diatur lewat menu Pengaturan Formulir/Data Pegawai, bukan di sini")
		return
	}
	var p kepalaDinasPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.IDPegawai != nil {
		var cek models.Pegawai
		if err := db.Select("id").First(&cek, "id = ?", *p.IDPegawai).Error; err != nil {
			utils.Error(w, http.StatusBadRequest, "pegawai tidak ditemukan")
			return
		}
	}

	old := item.IDAtasanLangsung
	if err := db.Model(&models.UnitKerja{}).Where("id = ?", item.ID).Update("id_atasan_langsung", p.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan kepala unit: "+err.Error())
		return
	}

	berubah := (old == nil) != (p.IDPegawai == nil) || (old != nil && p.IDPegawai != nil && *old != *p.IDPegawai)
	if berubah && p.IDPegawai != nil {
		// (1) sebar ke staf yang sudah bertugas di unit ini.
		db.Model(&models.Pegawai{}).
			Where("id_unit_kerja = ? AND id <> ?", item.ID, *p.IDPegawai).
			Update("id_atasan", *p.IDPegawai)
		PromoteAkunKeAtasan(db, *p.IDPegawai)

		// (2) garis komando naik: kepala baru ini melapor ke kepala unit induknya.
		if item.IDIndukUnitKerja != nil {
			var induk models.UnitKerja
			if err := db.First(&induk, "id = ?", *item.IDIndukUnitKerja).Error; err == nil {
				if idAtasanInduk := resolveKepalaPegawaiIDDinas(db, induk); idAtasanInduk != nil && *idAtasanInduk != *p.IDPegawai {
					db.Model(&models.Pegawai{}).Where("id = ?", *p.IDPegawai).Update("id_atasan", *idAtasanInduk)
				}
			}
		}

		// (3) garis komando turun: anak langsung yang sudah lebih dulu
		// punya kepala sendiri ikut disamakan ke kepala baru unit ini
		// (lihat komentar panjang di atas fungsi ini).
		var anakRows []models.UnitKerja
		db.Select("id", "id_atasan_langsung").Where("id_induk_unit_kerja = ?", item.ID).Find(&anakRows)
		for _, anak := range anakRows {
			if anak.IDAtasanLangsung != nil && *anak.IDAtasanLangsung != *p.IDPegawai {
				db.Model(&models.Pegawai{}).Where("id = ?", *anak.IDAtasanLangsung).Update("id_atasan", *p.IDPegawai)
			}
		}
	}
	utils.Success(w, "kepala unit berhasil disimpan", nil)
}

// ============================================================
// KJ (Jabatan.KelasJabatan, global) & K (FormasiJabatan) -- administrator only
// ============================================================

// updateJabatanKJDinasHandler: SAMA PERSIS logikanya dengan
// updateJabatanKJHandler (Peta Jabatan Sekolah), ditulis ulang BERDIRI
// SENDIRI di sini semata supaya hak aksesnya (lihat closure "kelola" di
// atas, administrator ONLY) tidak tercampur dengan hak akses
// admin/atasan milik endpoint Sekolah.
func updateJabatanKJDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	id := r.PathValue("id")
	var item models.Jabatan
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "jabatan tidak ditemukan")
		return
	}
	var p jabatanKJPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := db.Model(&item).Update("kelas_jabatan", p.KelasJabatan).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan kelas jabatan: "+err.Error())
		return
	}
	item.KelasJabatan = p.KelasJabatan
	utils.Success(w, "kelas jabatan (KJ) berhasil disimpan", item)
}

type formasiJabatanDinasPayload struct {
	IDUnitKerja uint `json:"id_unit_kerja"`
	IDJabatan   uint `json:"id_jabatan"`
	Kebutuhan   int  `json:"kebutuhan"`
}

// setFormasiJabatanDinasHandler menangani PUT /api/peta-jabatan/dinas/formasi
// -- upsert K untuk (unit, jabatan) pada struktur Dinas, IDSubJabatan selalu
// NULL (lihat komentar di atas file soal Dinas tidak memakai SubJabatan).
// Memakai ulang upsertFormasiJabatan (sama dgn Peta Jabatan Sekolah) --
// HANYA logika hak aksesnya (administrator only + wajib unit Dinas) yang
// berbeda & ditulis sendiri di sini.
func setFormasiJabatanDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p formasiJabatanDinasPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.IDUnitKerja == 0 || p.IDJabatan == 0 {
		utils.Error(w, http.StatusBadRequest, "unit kerja dan jabatan wajib diisi")
		return
	}
	if p.Kebutuhan < 0 {
		utils.Error(w, http.StatusBadRequest, "kebutuhan tidak boleh negatif")
		return
	}
	var uk models.UnitKerja
	if err := db.Select("id", "tempat_kerja").First(&uk, "id = ?", p.IDUnitKerja).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "unit kerja tidak ditemukan")
		return
	}
	if uk.TempatKerja == nil || *uk.TempatKerja != models.TempatKerjaDinas {
		utils.Error(w, http.StatusBadRequest, "unit ini bukan bagian struktur Peta Jabatan Dinas")
		return
	}
	item, err := upsertFormasiJabatan(db, p.IDUnitKerja, p.IDJabatan, nil, func(_ int) int { return p.Kebutuhan })
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan kebutuhan: "+err.Error())
		return
	}
	utils.Success(w, "kebutuhan (K) berhasil disimpan", item)
}
