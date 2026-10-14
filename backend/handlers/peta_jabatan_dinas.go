package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	mux.Handle("DELETE /api/peta-jabatan/dinas/formasi/{idUnit}/{idJabatan}", kelola(func(w http.ResponseWriter, r *http.Request) { hapusFormasiJabatanDinasHandler(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/dinas/formasi/{idUnit}/{idJabatan}/geser", kelola(func(w http.ResponseWriter, r *http.Request) { geserJabatanDinasHandler(w, r, db) }))
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
	// SayaIDUnitKerja/SayaIDJabatan: permintaan pengguna -- "beri tanda
	// jika nama pegawai berada di posisi yang mana agar masing-masing
	// pegawai dapat mengetahui posisi mereka dalam peta jabatan". Posisi
	// (unit + jabatan) akun yang SEDANG LOGIN SAAT INI (claims.IDPegawai
	// pada token, lihat getPetaJabatanDinas) -- BUKAN daftar seluruh
	// pegawai di setiap unit (itu tetap HANYA lewat endpoint
	// administrator-only di peta_jabatan_dinas_pegawai.go, demi privasi
	// pegawai lain). Frontend memakai dua nilai ini untuk menyorot SATU
	// kartu unit & SATU baris jabatan milik akun ini sendiri --
	// PetaJabatanDinasNode.vue membandingkan node.id/row.id_jabatan
	// terhadap nilai ini. nil kalau akun ini tidak terhubung ke baris
	// Pegawai manapun, ATAU unit kerjanya saat ini bukan bagian dari
	// pohon struktur ini (tidak ada yang disorot, bukan error).
	SayaIDUnitKerja *uint `json:"saya_id_unit_kerja"`
	SayaIDJabatan   *uint `json:"saya_id_jabatan"`
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

	// posisi akun yang sedang login -- lihat komentar field
	// SayaIDUnitKerja/SayaIDJabatan pada petaJabatanDinasOut.
	var sayaIDUnitKerja, sayaIDJabatan *uint
	if claims != nil && claims.IDPegawai != nil {
		var saya models.Pegawai
		if err := db.Select("id", "id_unit_kerja", "id_jabatan").First(&saya, "id = ?", *claims.IDPegawai).Error; err == nil {
			sayaIDUnitKerja = saya.IDUnitKerja
			sayaIDJabatan = saya.IDJabatan
		}
	}

	utils.Success(w, "ok", petaJabatanDinasOut{
		BolehKelola:     claims != nil && claims.RoleName == "administrator",
		Root:            *out,
		SayaIDUnitKerja: sayaIDUnitKerja,
		SayaIDJabatan:   sayaIDJabatan,
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

// kepalaPegawaiIDsDinas: kumpulan SELURUH id Pegawai yang SAAT INI
// menjabat sebagai kepala MANA PUN node pada pohon struktur Peta Jabatan
// Dinas -- akar (Kepala Dinas/Plt. Kepala Dinas, diresolusi dari NIP pada
// PengaturanSurat) maupun Sekretaris/Kepala Bidang/Kepala Seksi/Kepala Sub
// Bagian (UnitKerja.IDAtasanLangsung), lihat resolveKepalaPegawaiIDDinas
// untuk logika resolusi tiap tipe node.
//
// Dipakai listPegawaiDinasBelumDitempatkan (handlers/
// peta_jabatan_dinas_pegawai.go) -- permintaan pengguna: "hilangkan
// nama-nama yang sudah termasuk plt kepala dinas/kepala dinas, sekretaris,
// kepala bidang, kepala seksi dan kasubag kepegawaian" dari daftar
// "Pegawai Dinas Belum Ditempatkan" -- kelima jabatan kepala ini (PERSIS
// kelima tipe node StrukturDinas* yang ada) sudah terwakili lewat pita
// judul kartu unitnya masing-masing, jadi tidak perlu lagi muncul di
// daftar "pekerjaan rumah" penempatan staf itu, SEKALIPUN Pegawai.
// IDUnitKerja mereka sendiri kebetulan belum/tidak menunjuk ke unit mana
// pun pada pohon struktur baru ini.
func kepalaPegawaiIDsDinas(db *gorm.DB) map[uint]bool {
	ids := map[uint]bool{}
	var unitRows []models.UnitKerja
	db.Select("id", "tipe_struktur_dinas", "id_atasan_langsung").
		Where("tempat_kerja = ? AND tipe_struktur_dinas IS NOT NULL AND tipe_struktur_dinas <> ?", models.TempatKerjaDinas, "").
		Find(&unitRows)
	for _, uk := range unitRows {
		if idKepala := resolveKepalaPegawaiIDDinas(db, uk); idKepala != nil {
			ids[*idKepala] = true
		}
	}
	return ids
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

// hapusFormasiJabatanDinasHandler menangani DELETE
// /api/peta-jabatan/dinas/formasi/{idUnit}/{idJabatan} -- permintaan
// pengguna: "buat agar administrator dapat menghapus jabatan yang tidak
// diperlukan" (baris KJ/B/K/+- pada kartu unit, lihat tombol hapus di
// PetaJabatanDinasNode.vue). Menghapus baris FormasiJabatan (unit, jabatan)
// supaya jabatan ini TIDAK LAGI muncul pada tabel unit ini sama sekali --
// hitungPetaJabatanSekolah (dipakai bersama Sekolah & Dinas) menyertakan
// sebuah jabatan pada tabel kalau SALAH SATU dari: ada pegawai yang
// menjabatnya di unit ini (B), ADA baris FormasiJabatan (K, walau K=0), atau
// ada SubJabatan (Dinas tidak pernah memakai SubJabatan) -- jadi menghapus
// baris FormasiJabatan SAJA baru benar-benar menghilangkan jabatan ini dari
// tabel KALAU tidak ada pegawai (B=0) yang masih menjabatnya di unit ini.
// DITOLAK (400) kalau B>0 -- SENGAJA TIDAK memindahkan/mengeluarkan pegawai
// itu secara otomatis (beda jenis aksi, bisa tidak disengaja) -- administrator
// diminta mengeluarkan pegawai itu dulu lewat dialog "Kelola Pegawai Unit
// Ini"/klik kolom B, baru jabatan ini bisa dihapus dari unit.
func hapusFormasiJabatanDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	idUnit := r.PathValue("idUnit")
	idJabatan := r.PathValue("idJabatan")

	var uk models.UnitKerja
	if err := db.Select("id", "tempat_kerja").First(&uk, "id = ?", idUnit).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "unit kerja tidak ditemukan")
		return
	}
	if uk.TempatKerja == nil || *uk.TempatKerja != models.TempatKerjaDinas {
		utils.Error(w, http.StatusBadRequest, "unit ini bukan bagian struktur Peta Jabatan Dinas")
		return
	}
	var jab models.Jabatan
	if err := db.Select("id", "jabatan").First(&jab, "id = ?", idJabatan).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "jabatan tidak ditemukan")
		return
	}

	var jumlahPegawai int64
	scopePegawaiAktifDinas(db, db.Model(&models.Pegawai{})).
		Where("id_unit_kerja = ? AND id_jabatan = ?", uk.ID, jab.ID).
		Count(&jumlahPegawai)
	if jumlahPegawai > 0 {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf(
			"tidak bisa menghapus \"%s\" -- masih ada %d pegawai dengan jabatan ini di unit ini, keluarkan dulu lewat \"Kelola Pegawai Unit Ini\" atau klik kolom B jabatan ini",
			jab.Jabatan, jumlahPegawai))
		return
	}

	if err := db.Where("id_unit_kerja = ? AND id_jabatan = ?", uk.ID, jab.ID).Delete(&models.FormasiJabatan{}).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menghapus jabatan: "+err.Error())
		return
	}
	utils.Success(w, fmt.Sprintf("jabatan \"%s\" dihapus dari unit ini", jab.Jabatan), nil)
}

type geserJabatanDinasPayload struct {
	Arah string `json:"arah"` // "atas" atau "bawah"
}

// geserJabatanDinasHandler menangani PUT
// /api/peta-jabatan/dinas/formasi/{idUnit}/{idJabatan}/geser -- permintaan
// pengguna: "buat agar administrator dapat memindahkan jabatan ke atas
// atau ke bawah" pada tabel KJ/B/K/+- satu kartu unit.
//
// Perpindahan HANYA berlaku DI DALAM kelompoknya sendiri (Jabatan
// Pelaksana ATAU Jabatan Fungsional, lihat splitPetaJabatanByJenis &
// drawDinasChartGroup) -- posisi kelompok itu sendiri (Pelaksana SELALU di
// atas Fungsional, lihat PetaJabatanDinasNode.vue) tidak pernah berubah,
// hanya urutan baris DI DALAM satu kelompok yang sama.
//
// Memakai ulang hitungPetaJabatanSekolah (fungsi generik yang sama dipakai
// tabel di layar/PDF/Excel) untuk mendapatkan urutan TAMPIL SAAT INI, SUPAYA
// baris yang Urutan-nya masih default 0 (belum pernah digeser sama sekali,
// termasuk baris yang baru saja ditambah lewat "Tambah Jabatan") tetap
// mendapat posisi yang BENAR (abjad) sebagai titik awal sebelum ditukar.
// Setiap kali dipanggil, SELURUH baris pada kelompok yang sama dinormalkan
// ulang jadi kelipatan 10 sesuai urutan tampil itu (lihat
// setFormasiJabatanUrutan), lalu Urutan baris yang digeser & tetangganya
// ditukar -- supaya hasilnya konsisten tidak peduli baris mana yang sudah/
// belum pernah disimpan Urutan-nya sebelumnya.
func geserJabatanDinasHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	idUnitStr := r.PathValue("idUnit")
	idJabatanStr := r.PathValue("idJabatan")

	var p geserJabatanDinasPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.Arah != "atas" && p.Arah != "bawah" {
		utils.Error(w, http.StatusBadRequest, "arah wajib \"atas\" atau \"bawah\"")
		return
	}

	var uk models.UnitKerja
	if err := db.Select("id", "tempat_kerja").First(&uk, "id = ?", idUnitStr).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "unit kerja tidak ditemukan")
		return
	}
	if uk.TempatKerja == nil || *uk.TempatKerja != models.TempatKerjaDinas {
		utils.Error(w, http.StatusBadRequest, "unit ini bukan bagian struktur Peta Jabatan Dinas")
		return
	}
	idJabatan64, err := strconv.ParseUint(idJabatanStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "id jabatan tidak valid")
		return
	}
	idJabatan := uint(idJabatan64)

	peta, err := hitungPetaJabatanSekolah(db, uk.ID)
	if err != nil {
		writeResolveError(w, err)
		return
	}

	fungsional, pelaksana := splitPetaJabatanByJenis(peta.Rows)
	kelompok := pelaksana
	ketemu := false
	for _, row := range kelompok {
		if row.IDJabatan == idJabatan {
			ketemu = true
			break
		}
	}
	if !ketemu {
		kelompok = fungsional
		for _, row := range kelompok {
			if row.IDJabatan == idJabatan {
				ketemu = true
				break
			}
		}
	}
	if !ketemu {
		utils.Error(w, http.StatusNotFound, "jabatan ini tidak ditemukan pada unit ini")
		return
	}

	idx := -1
	for i, row := range kelompok {
		if row.IDJabatan == idJabatan {
			idx = i
			break
		}
	}
	target := idx - 1
	posisi := "paling atas"
	if p.Arah == "bawah" {
		target = idx + 1
		posisi = "paling bawah"
	}
	if target < 0 || target >= len(kelompok) {
		utils.Error(w, http.StatusBadRequest, fmt.Sprintf(
			"jabatan \"%s\" sudah berada di posisi %s pada kelompoknya", kelompok[idx].Jabatan, posisi))
		return
	}

	urutanBaru := make([]int, len(kelompok))
	for i := range kelompok {
		urutanBaru[i] = (i + 1) * 10
	}
	urutanBaru[idx], urutanBaru[target] = urutanBaru[target], urutanBaru[idx]

	for i, row := range kelompok {
		if err := setFormasiJabatanUrutan(db, uk.ID, row.IDJabatan, urutanBaru[i]); err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan urutan: "+err.Error())
			return
		}
	}
	utils.Success(w, fmt.Sprintf("jabatan \"%s\" dipindahkan", kelompok[idx].Jabatan), nil)
}
