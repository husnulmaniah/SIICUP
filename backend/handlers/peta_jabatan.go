package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// peta_jabatan.go menangani menu "Peta Jabatan" (dropdown Administrasi
// Kepegawaian) -- KHUSUS tab "Peta Jabatan Sekolah" untuk saat ini (tab
// "Peta Jabatan Dinas" menyusul kemudian, lihat permintaan pengguna & tab
// nonaktif "Segera Hadir" di frontend). Lihat komentar lengkap alur di
// models.go (bagian "PETA JABATAN -- SEKOLAH") untuk gambaran besar fitur
// ini; file ini sendiri berisi: resolusi sekolah per role, agregasi
// B (Bezetting)/K (Kebutuhan)/+- per Jabatan & SubJabatan, CRUD SubJabatan
// ("Kelola Sub-Jabatan"), pengaturan manual K (FormasiJabatan), dan
// penempatan pegawai ke SubJabatan tertentu.
//
// Alur Kenaikan Pangkat & Perubahan Jabatan (mengubah K lalu B) ada di
// handlers/peta_jabatan_kenaikan_pangkat.go & handlers/peta_jabatan_perubahan.go.
// Pembuatan dokumen PDF (kop sekolah + garis komando + QR tanda tangan) ada
// di handlers/peta_jabatan_pdf.go.

func RegisterPetaJabatanRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	// lihat: siapa saja yang boleh MELIHAT peta jabatan suatu sekolah --
	// administrator/admin (sekolah manapun, lewat parameter id_unit_kerja),
	// atasan & pegawai (HANYA sekolah tempat mereka sendiri bertugas,
	// resolveUnitKerjaSekolahForRequest di bawah mengabaikan parameter
	// id_unit_kerja untuk kedua role ini).
	lihat := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan", "pegawai") }
	// kelola: siapa saja yang boleh MENGATUR (Kelola Sub-Jabatan, atur K,
	// tempatkan pegawai ke sub-jabatan) -- administrator/admin (sekolah
	// manapun) ATAU atasan HANYA untuk sekolahnya sendiri (dicek ulang di
	// masing-masing handler lewat canManagePetaJabatanSekolah).
	kelola := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan") }
	adminOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	mux.Handle("GET /api/peta-jabatan/sekolah/daftar", adminOnly(func(w http.ResponseWriter, r *http.Request) { daftarSekolahPetaJabatan(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/sekolah", lihat(func(w http.ResponseWriter, r *http.Request) { getPetaJabatanSekolah(w, r, db) }))
	// sinkronkan: permintaan pengguna -- tombol "Sinkronkan Data" di
	// frontend, menarik ulang data Jabatan & nama pegawai yang ada di tabel
	// pegawai (lewat id_unit_kerja sekolah ini) lalu memastikan setiap
	// jabatan/sub-jabatan yang BENAR-BENAR dipegang pegawai sudah punya
	// baris kebutuhan (K) -- lihat sinkronkanPetaJabatanSekolah di bawah.
	// HANYA administrator/admin/atasan (yang berhak mengelola, sama seperti
	// aksi Atur K/Kelola Sub-Jabatan lainnya) -- bukan pegawai, karena ini
	// bisa membuat baris FormasiJabatan baru.
	mux.Handle("POST /api/peta-jabatan/sekolah/sinkronkan", kelola(func(w http.ResponseWriter, r *http.Request) { sinkronkanPetaJabatanSekolah(w, r, db) }))

	mux.Handle("GET /api/peta-jabatan/sub-jabatan", lihat(func(w http.ResponseWriter, r *http.Request) { listSubJabatanHandler(w, r, db) }))
	mux.Handle("POST /api/peta-jabatan/sub-jabatan", kelola(func(w http.ResponseWriter, r *http.Request) { createSubJabatanHandler(w, r, db) }))
	mux.Handle("DELETE /api/peta-jabatan/sub-jabatan/{id}", kelola(func(w http.ResponseWriter, r *http.Request) { deleteSubJabatanHandler(w, r, db) }))

	mux.Handle("PUT /api/peta-jabatan/formasi", kelola(func(w http.ResponseWriter, r *http.Request) { setFormasiJabatanHandler(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/pegawai/{id}/sub-jabatan", kelola(func(w http.ResponseWriter, r *http.Request) { setPegawaiSubJabatanHandler(w, r, db) }))
}

// ============================================================
// resolusi sekolah & hak kelola
// ============================================================

// resolveUnitKerjaSekolahForRequest menentukan UnitKerja (sekolah) mana yang
// berlaku untuk request ini: administrator/admin WAJIB mengirim parameter
// query "id_unit_kerja" (bisa sekolah manapun); atasan & pegawai SELALU
// memakai unit kerja tempat mereka SENDIRI bertugas (parameter diabaikan,
// supaya tidak ada akun pegawai/atasan yang bisa mengintip peta jabatan
// sekolah lain hanya dengan mengganti parameter URL).
func resolveUnitKerjaSekolahForRequest(claims *utils.Claims, r *http.Request, db *gorm.DB) (uint, error) {
	switch claims.RoleName {
	case "administrator", "admin":
		idStr := strings.TrimSpace(r.URL.Query().Get("id_unit_kerja"))
		if idStr == "" {
			return 0, errNeedUnitKerjaParam
		}
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || id == 0 {
			return 0, errNeedUnitKerjaParam
		}
		return uint(id), nil
	default: // atasan, pegawai
		if claims.IDPegawai == nil {
			return 0, errAkunBelumTerhubungPegawai
		}
		var pegawai models.Pegawai
		if err := db.Select("id", "id_unit_kerja").First(&pegawai, "id = ?", *claims.IDPegawai).Error; err != nil {
			return 0, errAkunBelumTerhubungPegawai
		}
		if pegawai.IDUnitKerja == nil {
			return 0, errPegawaiBelumPunyaUnitKerja
		}
		return *pegawai.IDUnitKerja, nil
	}
}

var (
	errNeedUnitKerjaParam         = &handlerError{status: http.StatusBadRequest, message: "parameter id_unit_kerja wajib diisi"}
	errAkunBelumTerhubungPegawai  = &handlerError{status: http.StatusBadRequest, message: "akun ini belum terhubung dengan data pegawai"}
	errPegawaiBelumPunyaUnitKerja = &handlerError{status: http.StatusBadRequest, message: "data pegawai anda belum memiliki unit kerja, hubungi administrator"}
)

// handlerError: kesalahan sederhana dengan kode HTTP sendiri, dipakai
// resolveUnitKerjaSekolahForRequest supaya pemanggilnya bisa langsung
// meneruskan status & pesannya ke utils.Error tanpa switch-case berulang.
type handlerError struct {
	status  int
	message string
}

func (e *handlerError) Error() string { return e.message }

func writeResolveError(w http.ResponseWriter, err error) {
	if he, ok := err.(*handlerError); ok {
		utils.Error(w, he.status, he.message)
		return
	}
	utils.Error(w, http.StatusInternalServerError, "gagal memproses permintaan: "+err.Error())
}

// canManagePetaJabatanSekolah: administrator/admin boleh mengelola sekolah
// MANAPUN; atasan HANYA boleh mengelola sekolah tempat MEREKA SENDIRI
// bertugas (dicocokkan dari Pegawai.IDUnitKerja akun yang login, BUKAN dari
// Pegawai.IDAtasan bawahannya -- Kepala Sekolah ditandai cukup lewat
// bertugas di sekolah itu + Role akun "atasan", sama seperti pola
// kewenangan "Kop Surat Sekolah"/approve Arsip Surat yang sudah ada).
func canManagePetaJabatanSekolah(claims *utils.Claims, idUnitKerja uint, db *gorm.DB) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "atasan":
		if claims.IDPegawai == nil {
			return false
		}
		var pegawai models.Pegawai
		if err := db.Select("id", "id_unit_kerja").First(&pegawai, "id = ?", *claims.IDPegawai).Error; err != nil {
			return false
		}
		return pegawai.IDUnitKerja != nil && *pegawai.IDUnitKerja == idUnitKerja
	}
	return false
}

// ============================================================
// daftar sekolah (picker administrator)
// ============================================================

type sekolahPetaJabatanOut struct {
	ID   uint   `json:"id"`
	Unit string `json:"unit"`
}

// daftarSekolahPetaJabatan menangani GET /api/peta-jabatan/sekolah/daftar --
// KHUSUS administrator/admin, daftar seluruh UnitKerja yang eksplisit
// ditandai "Sekolah" (UnitKerja.TempatKerja, lihat menu Unit Kerja) supaya
// administrator bisa memilih sekolah mana yang ingin dilihat/dikelola peta
// jabatannya. Atasan/pegawai TIDAK perlu endpoint ini -- sekolah mereka
// sendiri sudah otomatis diresolusi (lihat resolveUnitKerjaSekolahForRequest).
func daftarSekolahPetaJabatan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var rows []models.UnitKerja
	db.Select("id", "unit").Where("tempat_kerja = ?", models.TempatKerjaSekolah).Order("unit asc").Find(&rows)
	out := make([]sekolahPetaJabatanOut, 0, len(rows))
	for _, it := range rows {
		out = append(out, sekolahPetaJabatanOut{ID: it.ID, Unit: it.Unit})
	}
	utils.Success(w, "ok", out)
}

// ============================================================
// agregasi B (Bezetting) / K (Kebutuhan) / +- per Jabatan & SubJabatan
// ============================================================

type subJabatanPetaOut struct {
	ID      uint   `json:"id"`
	Nama    string `json:"nama"`
	B       int    `json:"b"`
	K       int    `json:"k"`
	Selisih int    `json:"selisih"`
}

type jabatanPetaOut struct {
	IDJabatan       uint                `json:"id_jabatan"`
	Jabatan         string              `json:"jabatan"`
	B               int                 `json:"b"`
	K               int                 `json:"k"`
	Selisih         int                 `json:"selisih"`
	PunyaSubJabatan bool                `json:"punya_sub_jabatan"`
	SubJabatan      []subJabatanPetaOut `json:"sub_jabatan"`
}

type petaJabatanSekolahOut struct {
	UnitKerja   sekolahPetaJabatanOut `json:"unit_kerja"`
	BolehKelola bool                  `json:"boleh_kelola"`
	Rows        []jabatanPetaOut      `json:"rows"`
}

// hitungPetaJabatanSekolah: inti perhitungan B/K/+- -- dipakai bersama oleh
// getPetaJabatanSekolah (tabel di menu) DAN handlers/peta_jabatan_pdf.go
// (dokumen cetak) supaya KEDUANYA selalu menunjukkan angka yang identik.
//
// "B" (Bezetting, jumlah pegawai SAAT INI) dihitung LANGSUNG dari tabel
// pegawai (COUNT ... GROUP BY id_jabatan, id_sub_jabatan) -- TIDAK pernah
// disimpan terpisah, supaya selalu akurat tanpa risiko data dobel. Pegawai
// yang IDSubJabatan-nya kosong ATAU menunjuk SubJabatan yang sudah dihapus
// ("yatim") tetap ikut terhitung penuh pada baris Jabatan INDUknya (hanya
// tidak muncul di salah satu baris pecahan sub-jabatan) -- lihat totalB di
// bawah yang menjumlahkan SEMUA baris count, bukan hanya yang cocok dengan
// SubJabatan yang masih ada.
//
// "K" (Kebutuhan) dibaca dari FormasiJabatan: kalau Jabatan itu SUDAH
// dipecah jadi SubJabatan di sekolah ini, K Jabatan induk = SUM seluruh K
// SubJabatan-nya (baris FormasiJabatan ber-IDSubJabatan NULL untuk Jabatan
// itu diabaikan/tidak dipakai); kalau BELUM dipecah, K dibaca langsung dari
// baris ber-IDSubJabatan NULL.
func hitungPetaJabatanSekolah(db *gorm.DB, idUnitKerja uint) (*petaJabatanSekolahOut, error) {
	var uk models.UnitKerja
	if err := db.Select("id", "unit").First(&uk, "id = ?", idUnitKerja).Error; err != nil {
		return nil, &handlerError{status: http.StatusNotFound, message: "unit kerja/sekolah tidak ditemukan"}
	}

	type bRow struct {
		IDJabatan    uint
		IDSubJabatan uint // 0 berarti NULL (belum/tidak dikategorikan ke sub-jabatan apa pun)
		Jumlah       int
	}
	var bRows []bRow
	db.Model(&models.Pegawai{}).
		Select("id_jabatan, COALESCE(id_sub_jabatan, 0) AS id_sub_jabatan, COUNT(*) AS jumlah").
		Where("id_unit_kerja = ? AND id_jabatan IS NOT NULL", idUnitKerja).
		Group("id_jabatan, COALESCE(id_sub_jabatan, 0)").
		Scan(&bRows)

	bTotalPerJabatan := map[uint]int{}
	bPerSub := map[uint]map[uint]int{} // [idJabatan][idSubJabatan] -> jumlah
	for _, row := range bRows {
		bTotalPerJabatan[row.IDJabatan] += row.Jumlah
		if bPerSub[row.IDJabatan] == nil {
			bPerSub[row.IDJabatan] = map[uint]int{}
		}
		bPerSub[row.IDJabatan][row.IDSubJabatan] += row.Jumlah
	}

	var subRows []models.SubJabatan
	db.Where("id_unit_kerja = ?", idUnitKerja).Order("id_jabatan asc, urutan asc, id asc").Find(&subRows)
	subPerJabatan := map[uint][]models.SubJabatan{}
	for _, s := range subRows {
		subPerJabatan[s.IDJabatan] = append(subPerJabatan[s.IDJabatan], s)
	}

	var formasiRows []models.FormasiJabatan
	db.Where("id_unit_kerja = ?", idUnitKerja).Find(&formasiRows)
	kPerSub := map[uint]map[uint]int{} // [idJabatan][idSubJabatan-or-0] -> kebutuhan
	for _, f := range formasiRows {
		subID := uint(0)
		if f.IDSubJabatan != nil {
			subID = *f.IDSubJabatan
		}
		if kPerSub[f.IDJabatan] == nil {
			kPerSub[f.IDJabatan] = map[uint]int{}
		}
		kPerSub[f.IDJabatan][subID] = f.Kebutuhan
	}

	jabatanIDSet := map[uint]bool{}
	for id := range bTotalPerJabatan {
		jabatanIDSet[id] = true
	}
	for id := range subPerJabatan {
		jabatanIDSet[id] = true
	}
	for id := range kPerSub {
		jabatanIDSet[id] = true
	}
	jabatanIDs := make([]uint, 0, len(jabatanIDSet))
	for id := range jabatanIDSet {
		jabatanIDs = append(jabatanIDs, id)
	}

	var jabatanMaster []models.Jabatan
	if len(jabatanIDs) > 0 {
		db.Where("id IN ?", jabatanIDs).Find(&jabatanMaster)
	}
	namaJabatan := map[uint]string{}
	for _, j := range jabatanMaster {
		namaJabatan[j.ID] = j.Jabatan
	}

	rows := make([]jabatanPetaOut, 0, len(jabatanIDs))
	for _, idJabatan := range jabatanIDs {
		subs := subPerJabatan[idJabatan]
		totalB := bTotalPerJabatan[idJabatan]
		totalK := 0
		subOut := make([]subJabatanPetaOut, 0, len(subs))
		if len(subs) > 0 {
			for _, s := range subs {
				subB := bPerSub[idJabatan][s.ID]
				subK := kPerSub[idJabatan][s.ID]
				totalK += subK
				subOut = append(subOut, subJabatanPetaOut{ID: s.ID, Nama: s.Nama, B: subB, K: subK, Selisih: subB - subK})
			}
		} else {
			totalK = kPerSub[idJabatan][0]
		}
		if totalB == 0 && totalK == 0 && len(subs) == 0 {
			continue
		}
		nama := namaJabatan[idJabatan]
		if nama == "" {
			nama = "(jabatan tidak ditemukan)"
		}
		rows = append(rows, jabatanPetaOut{
			IDJabatan:       idJabatan,
			Jabatan:         nama,
			B:               totalB,
			K:               totalK,
			Selisih:         totalB - totalK,
			PunyaSubJabatan: len(subs) > 0,
			SubJabatan:      subOut,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Jabatan < rows[j].Jabatan })

	return &petaJabatanSekolahOut{
		UnitKerja: sekolahPetaJabatanOut{ID: uk.ID, Unit: uk.Unit},
		Rows:      rows,
	}, nil
}

func getPetaJabatanSekolah(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	idUnitKerja, err := resolveUnitKerjaSekolahForRequest(claims, r, db)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	out, err := hitungPetaJabatanSekolah(db, idUnitKerja)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	out.BolehKelola = canManagePetaJabatanSekolah(claims, idUnitKerja, db)
	utils.Success(w, "ok", out)
}

// sinkronkanPetaJabatanSekolah menangani POST
// /api/peta-jabatan/sekolah/sinkronkan -- permintaan pengguna: "tarik data
// dan hitung berdasarkan data jabatan dan nama pegawai yang ada di unit
// kerja pada data pegawai ... ketika diklik sinkronkan data maka data peta
// jabatan akan terupdate sesuai data yang ada pada data pegawai".
//
// "B" (Bezetting) SUDAH SELALU dihitung langsung dari tabel pegawai pada
// SETIAP kali tabel Peta Jabatan dimuat (lihat hitungPetaJabatanSekolah) --
// tidak pernah basi/butuh disinkronkan terpisah. Yang disinkronkan di sini
// adalah baris "K" (FormasiJabatan): memastikan SETIAP Jabatan (dan, kalau
// sudah dipecah, SETIAP SubJabatan) yang BENAR-BENAR dipegang oleh pegawai
// pada sekolah ini saat ini (ditarik langsung dari kolom id_jabatan/nama
// pegawai pada tabel pegawai, bukan dari catatan lama yang mungkin sudah
// tidak relevan) langsung punya baris kebutuhan (K) -- default 0 kalau
// memang belum pernah diatur -- supaya baris itu terlihat & siap diatur
// administrator/atasan tanpa harus menunggu salah satu pegawainya
// "kebetulan" membuka dialog Atur Kebutuhan duluan. Baris FormasiJabatan
// yang SUDAH ada (K-nya sudah diisi manual) TIDAK PERNAH disentuh/ditimpa.
func sinkronkanPetaJabatanSekolah(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	idUnitKerja, err := resolveUnitKerjaSekolahForRequest(claims, r, db)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	if !canManagePetaJabatanSekolah(claims, idUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses mengelola sekolah ini")
		return
	}

	// Jabatan yang BENAR-BENAR dipegang pegawai pada sekolah ini SAAT INI --
	// ditarik langsung dari data Jabatan & nama pegawai pada tabel pegawai.
	var idJabatanDipegang []uint
	db.Model(&models.Pegawai{}).
		Distinct("id_jabatan").
		Where("id_unit_kerja = ? AND id_jabatan IS NOT NULL", idUnitKerja).
		Pluck("id_jabatan", &idJabatanDipegang)

	var subRows []models.SubJabatan
	db.Where("id_unit_kerja = ?", idUnitKerja).Find(&subRows)
	subPerJabatan := map[uint][]models.SubJabatan{}
	for _, s := range subRows {
		subPerJabatan[s.IDJabatan] = append(subPerJabatan[s.IDJabatan], s)
	}

	dibuat := 0
	for _, idJabatan := range idJabatanDipegang {
		subs := subPerJabatan[idJabatan]
		if len(subs) == 0 {
			if _, created, err := ensureFormasiJabatan(db, idUnitKerja, idJabatan, nil); err == nil && created {
				dibuat++
			}
			continue
		}
		for _, s := range subs {
			subID := s.ID
			if _, created, err := ensureFormasiJabatan(db, idUnitKerja, idJabatan, &subID); err == nil && created {
				dibuat++
			}
		}
	}

	out, err := hitungPetaJabatanSekolah(db, idUnitKerja)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	out.BolehKelola = true
	pesan := "data Peta Jabatan sudah sesuai dengan data Jabatan & Pegawai terbaru"
	if dibuat > 0 {
		pesan = fmt.Sprintf("data Peta Jabatan disinkronkan -- %d baris kebutuhan (K) baru dibuat (default 0) untuk jabatan/sub-jabatan yang belum pernah diatur", dibuat)
	}
	utils.Success(w, pesan, out)
}

// ensureFormasiJabatan: seperti upsertFormasiJabatan, TAPI tidak pernah
// mengubah nilai Kebutuhan yang sudah ada -- hanya membuat baris baru
// (default Kebutuhan 0) kalau memang belum ada sama sekali. Dipakai KHUSUS
// oleh sinkronkanPetaJabatanSekolah supaya K yang sudah diatur manual
// administrator/atasan tidak pernah tertimpa balik ke 0.
func ensureFormasiJabatan(db *gorm.DB, idUnitKerja, idJabatan uint, idSubJabatan *uint) (*models.FormasiJabatan, bool, error) {
	query := db.Where("id_unit_kerja = ? AND id_jabatan = ?", idUnitKerja, idJabatan)
	if idSubJabatan != nil {
		query = query.Where("id_sub_jabatan = ?", *idSubJabatan)
	} else {
		query = query.Where("id_sub_jabatan IS NULL")
	}
	var item models.FormasiJabatan
	err := query.First(&item).Error
	if err == nil {
		return &item, false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	item = models.FormasiJabatan{IDUnitKerja: idUnitKerja, IDJabatan: idJabatan, IDSubJabatan: idSubJabatan, Kebutuhan: 0}
	if err := db.Create(&item).Error; err != nil {
		return nil, false, err
	}
	return &item, true, nil
}

// ============================================================
// CRUD SubJabatan ("Kelola Sub-Jabatan")
// ============================================================

func listSubJabatanHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	idUnitKerja, err := resolveUnitKerjaSekolahForRequest(claims, r, db)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	// Administrator boleh menyaring ke SATU Jabatan tertentu (dipakai dialog
	// "Kelola Sub-Jabatan" yang dibuka per-baris Jabatan) lewat parameter
	// opsional "id_jabatan"; kalau kosong, seluruh pecahan sekolah ini
	// dikembalikan sekaligus.
	query := db.Where("id_unit_kerja = ?", idUnitKerja)
	if idJabatan := strings.TrimSpace(r.URL.Query().Get("id_jabatan")); idJabatan != "" {
		query = query.Where("id_jabatan = ?", idJabatan)
	}
	var items []models.SubJabatan
	query.Order("id_jabatan asc, urutan asc, id asc").Find(&items)
	utils.Success(w, "ok", items)
}

type subJabatanPayload struct {
	IDUnitKerja uint   `json:"id_unit_kerja"`
	IDJabatan   uint   `json:"id_jabatan"`
	Nama        string `json:"nama"`
}

func createSubJabatanHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	var p subJabatanPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	p.Nama = strings.TrimSpace(p.Nama)
	if p.IDUnitKerja == 0 || p.IDJabatan == 0 || p.Nama == "" {
		utils.Error(w, http.StatusBadRequest, "unit kerja, jabatan, dan nama sub-jabatan wajib diisi")
		return
	}
	if !canManagePetaJabatanSekolah(claims, p.IDUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses mengelola sekolah ini")
		return
	}
	var maxUrutan int
	db.Model(&models.SubJabatan{}).
		Where("id_unit_kerja = ? AND id_jabatan = ?", p.IDUnitKerja, p.IDJabatan).
		Select("COALESCE(MAX(urutan), 0)").Scan(&maxUrutan)
	item := models.SubJabatan{IDUnitKerja: p.IDUnitKerja, IDJabatan: p.IDJabatan, Nama: p.Nama, Urutan: maxUrutan + 1}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menambah sub-jabatan (kemungkinan nama sudah dipakai untuk jabatan ini): "+err.Error())
		return
	}
	utils.Created(w, "sub-jabatan berhasil ditambahkan", item)
}

// deleteSubJabatanHandler menghapus satu pecahan SubJabatan -- pegawai yang
// SEBELUMNYA ditempatkan ke sub-jabatan ini TIDAK ikut dihapus/diubah
// (IDSubJabatan pegawai itu jadi "yatim", tetap terhitung penuh di baris
// Jabatan induknya, lihat komentar hitungPetaJabatanSekolah). Baris
// FormasiJabatan (K) milik sub-jabatan ini juga ikut dihapus sekaligus
// supaya tidak ada K nyasar yang tidak pernah ditampilkan lagi.
func deleteSubJabatanHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.SubJabatan
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "sub-jabatan tidak ditemukan")
		return
	}
	if !canManagePetaJabatanSekolah(claims, item.IDUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses mengelola sekolah ini")
		return
	}
	db.Where("id_sub_jabatan = ?", item.ID).Delete(&models.FormasiJabatan{})
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menghapus sub-jabatan: "+err.Error())
		return
	}
	utils.Success(w, "sub-jabatan berhasil dihapus", nil)
}

// ============================================================
// pengaturan manual K (FormasiJabatan)
// ============================================================

type formasiJabatanPayload struct {
	IDUnitKerja  uint  `json:"id_unit_kerja"`
	IDJabatan    uint  `json:"id_jabatan"`
	IDSubJabatan *uint `json:"id_sub_jabatan"`
	Kebutuhan    int   `json:"kebutuhan"`
}

// setFormasiJabatanHandler menangani PUT /api/peta-jabatan/formasi --
// upsert nilai K untuk satu kombinasi (Jabatan, SubJabatan opsional) pada
// satu sekolah. Dipanggil manual dari dialog "Atur Kebutuhan" ATAU otomatis
// (increment, bukan replace) begitu atasan menyetujui Kenaikan Pangkat --
// lihat tambahKebutuhanFormasiJabatan di
// handlers/peta_jabatan_kenaikan_pangkat.go yang memakai pola upsert yang
// sama tapi menambah, bukan mengganti nilai.
func setFormasiJabatanHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	var p formasiJabatanPayload
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
	if !canManagePetaJabatanSekolah(claims, p.IDUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses mengelola sekolah ini")
		return
	}
	item, err := upsertFormasiJabatan(db, p.IDUnitKerja, p.IDJabatan, p.IDSubJabatan, func(current int) int { return p.Kebutuhan })
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan kebutuhan: "+err.Error())
		return
	}
	utils.Success(w, "kebutuhan (K) berhasil disimpan", item)
}

// upsertFormasiJabatan: helper umum cari-atau-buat baris FormasiJabatan
// untuk (idUnitKerja, idJabatan, idSubJabatan), lalu terapkan ubah(nilaiLama)
// untuk mendapatkan nilai Kebutuhan BARU -- dipakai baik untuk mengganti
// nilai langsung (setFormasiJabatanHandler, ubah = func(_ int) int { return
// nilaiBaru }) MAUPUN menambah (kenaikan pangkat, ubah = func(lama int) int
// { return lama + 1 }), supaya logika "cari-atau-buat baris" ini tidak perlu
// ditulis ulang di dua tempat.
func upsertFormasiJabatan(db *gorm.DB, idUnitKerja, idJabatan uint, idSubJabatan *uint, ubah func(nilaiLama int) int) (*models.FormasiJabatan, error) {
	query := db.Where("id_unit_kerja = ? AND id_jabatan = ?", idUnitKerja, idJabatan)
	if idSubJabatan != nil {
		query = query.Where("id_sub_jabatan = ?", *idSubJabatan)
	} else {
		query = query.Where("id_sub_jabatan IS NULL")
	}
	var item models.FormasiJabatan
	err := query.First(&item).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
		item = models.FormasiJabatan{IDUnitKerja: idUnitKerja, IDJabatan: idJabatan, IDSubJabatan: idSubJabatan, Kebutuhan: ubah(0)}
		if err := db.Create(&item).Error; err != nil {
			return nil, err
		}
		return &item, nil
	}
	item.Kebutuhan = ubah(item.Kebutuhan)
	if err := db.Save(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// ============================================================
// penempatan pegawai ke SubJabatan
// ============================================================

type pegawaiSubJabatanPayload struct {
	IDSubJabatan *uint `json:"id_sub_jabatan"`
}

// setPegawaiSubJabatanHandler menangani PUT
// /api/peta-jabatan/pegawai/{id}/sub-jabatan -- menempatkan (atau
// mengosongkan, kalau id_sub_jabatan dikirim null) seorang pegawai ke satu
// pecahan SubJabatan TERTENTU. HANYA mengubah penempatan pecahannya saja,
// BUKAN Jabatan utama pegawai (itu hanya berubah lewat alur Kenaikan
// Pangkat -> Perubahan Jabatan yang disetujui, lihat
// handlers/peta_jabatan_perubahan.go) -- dipakai, misalnya, saat atasan
// pertama kali mengelompokkan guru-guru "Guru Ahli Pertama" yang sudah ada
// ke pecahan Guru Kelas/Guru Agama/dst begitu pecahan itu baru dibuat.
func setPegawaiSubJabatanHandler(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var pegawai models.Pegawai
	if err := db.Select("id", "id_jabatan", "id_unit_kerja", "id_sub_jabatan").First(&pegawai, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pegawai tidak ditemukan")
		return
	}
	if pegawai.IDUnitKerja == nil || !canManagePetaJabatanSekolah(claims, *pegawai.IDUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses mengelola sekolah pegawai ini")
		return
	}
	var p pegawaiSubJabatanPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if p.IDSubJabatan != nil {
		var sub models.SubJabatan
		if err := db.First(&sub, "id = ?", *p.IDSubJabatan).Error; err != nil {
			utils.Error(w, http.StatusBadRequest, "sub-jabatan tidak ditemukan")
			return
		}
		if pegawai.IDJabatan == nil || sub.IDJabatan != *pegawai.IDJabatan {
			utils.Error(w, http.StatusBadRequest, "sub-jabatan ini bukan pecahan dari jabatan pegawai yang bersangkutan")
			return
		}
		if sub.IDUnitKerja != *pegawai.IDUnitKerja {
			utils.Error(w, http.StatusBadRequest, "sub-jabatan ini bukan milik sekolah pegawai yang bersangkutan")
			return
		}
	}
	if err := db.Model(&pegawai).Update("id_sub_jabatan", p.IDSubJabatan).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan penempatan: "+err.Error())
		return
	}
	utils.Success(w, "penempatan sub-jabatan berhasil disimpan", nil)
}
