package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// kp4.go implements the "KP4" menu -- Surat Keterangan Untuk Mendapatkan
// Pembayaran Tunjangan Keluarga. Field yang SUDAH ADA di tabel Pegawai
// (nama, NIP, pangkat/golongan, TMT, tanggal lahir, status kepegawaian,
// jenis jabatan) SENGAJA TIDAK diduplikasi ke sini -- selalu diambil live
// dari relasi Pegawai lewat kp4Ringkasan di bawah, supaya satu-satunya
// sumber kebenarannya tetap Data Pegawai. Kp4Data/Kp4Pasangan/Kp4Anak hanya
// menyimpan field TAMBAHAN yang memang belum ada di Pegawai.
//
// Alur pengisian: pegawai isi/ubah SENDIRI datanya sendiri kapan saja lewat
// menu KP4 (GET/PUT /api/kp4/saya) -- TIDAK ada proses approval, langsung
// tersimpan begitu disimpan (isinya pernyataan/tanggung jawab pegawai
// sendiri). administrator/admin boleh melihat rekap semua pegawai (GET
// /api/kp4) & mengedit langsung data KP4 pegawai manapun (GET/PUT
// /api/kp4/pegawai/{id}) -- dipakai kalau pegawai kesulitan mengisi
// sendiri. Saat dicetak (GET /api/kp4/pegawai/{id}/cetak), formulir
// ditandatangani otomatis oleh Kepala Dinas/PLT memakai data yang SAMA
// dengan yang dipakai formulir cuti (models.PengaturanSurat, lihat
// formulir.go) -- termasuk QR tanda tangan otomatis (lihat buildKp4SignatureQR).

// ============================================================
// Payload & ringkasan
// ============================================================

// TglLahir/TglPerkawinan/TglLahir (anak) di bawah SENGAJA bertipe string
// (bukan *time.Time) -- unmarshal JSON langsung ke time.Time mengharuskan
// format RFC3339 penuh (mis. "1990-01-02T00:00:00Z"), sementara frontend
// (PrimeVue DatePicker) & pola tanggal lain di app ini mengirim "YYYY-MM-DD"
// polos. Diparsing lewat utils.ParseDateCell (sama seperti pegawaiPayload di
// pegawai.go) di simpanKp4 di bawah.
type kp4PasanganPayload struct {
	Nama          string  `json:"nama"`
	TempatLahir   string  `json:"tempat_lahir"`
	TglLahir      string  `json:"tgl_lahir"`
	NIK           string  `json:"nik"`
	Pekerjaan     string  `json:"pekerjaan"`
	TglPerkawinan string  `json:"tgl_perkawinan"`
	PasanganKe    int     `json:"pasangan_ke"`
	Penghasilan   float64 `json:"penghasilan"`
}

// kosong melaporkan apakah payload pasangan ini dianggap "tidak diisi" --
// dipakai untuk memutuskan apakah baris Kp4Pasangan yang sudah ada perlu
// dihapus (pegawai mengosongkan kembali data suami/istrinya, mis. karena
// salah isi atau berpisah).
func (p *kp4PasanganPayload) kosong() bool {
	return p == nil || strings.TrimSpace(p.Nama) == ""
}

type kp4AnakPayload struct {
	Nama                string `json:"nama"`
	TempatLahir         string `json:"tempat_lahir"`
	TglLahir            string `json:"tgl_lahir"`
	StatusAnak          string `json:"status_anak"`
	DariPasanganKe      int    `json:"dari_pasangan_ke"`
	JenisKelamin        string `json:"jenis_kelamin"`
	DapatTunjangan      bool   `json:"dapat_tunjangan"`
	SudahKawin          bool   `json:"sudah_kawin"`
	SudahBekerja        bool   `json:"sudah_bekerja"`
	MasihSekolah        bool   `json:"masih_sekolah"`
	NoPutusanPengadilan string `json:"no_putusan_pengadilan"`
}

type kp4SavePayload struct {
	TempatLahir         string              `json:"tempat_lahir"`
	JenisKelamin        string              `json:"jenis_kelamin"`
	Agama               string              `json:"agama"`
	AlamatJalan         string              `json:"alamat_jalan"`
	Desa                string              `json:"desa"`
	Kecamatan           string              `json:"kecamatan"`
	Kabupaten           string              `json:"kabupaten"`
	Provinsi            string              `json:"provinsi"`
	DigajiMenurut       string              `json:"digaji_menurut"`
	BesarnyaPenghasilan float64             `json:"besarnya_penghasilan"`
	SkTerakhir          string              `json:"sk_terakhir"`
	Pasangan            *kp4PasanganPayload `json:"pasangan"`
	Anak                []kp4AnakPayload    `json:"anak"`
}

// kp4Kelengkapan berisi status kelengkapan yang dipakai baik oleh menu KP4
// itu sendiri (banner di halamannya) maupun dashboard pegawai (lihat
// dashboard.go) -- dua pesan berbeda sesuai permintaan: (1) data KP4-nya
// sendiri belum lengkap -> arahkan ke menu KP4, (2) field dasar di Data
// Pegawai yang dipakai KP4 belum lengkap -> arahkan ke Profil Saya/Ajukan
// Perubahan Data (field itu bukan tanggung jawab menu KP4).
type kp4Kelengkapan struct {
	Kp4BelumLengkap   bool     `json:"kp4_belum_lengkap"`
	Kp4FieldKosong    []string `json:"kp4_field_kosong"`
	DataPegawaiKosong []string `json:"data_pegawai_kosong"`
}

func hitungKelengkapanKp4(pegawai models.Pegawai, kp4 *models.Kp4Data, punyaPasangan bool) kp4Kelengkapan {
	var dataPegawaiKosong []string
	if pegawai.IDPangkatGol == nil {
		dataPegawaiKosong = append(dataPegawaiKosong, "Pangkat/Golongan")
	}
	if pegawai.TMT == nil {
		dataPegawaiKosong = append(dataPegawaiKosong, "TMT")
	}
	if pegawai.TglLahir == nil {
		dataPegawaiKosong = append(dataPegawaiKosong, "Tanggal Lahir")
	}
	if pegawai.IDStatus == nil {
		dataPegawaiKosong = append(dataPegawaiKosong, "Status Kepegawaian")
	}
	if pegawai.IDJabatan == nil {
		dataPegawaiKosong = append(dataPegawaiKosong, "Jabatan")
	}

	var kp4Kosong []string
	if kp4 == nil || strings.TrimSpace(kp4.TempatLahir) == "" {
		kp4Kosong = append(kp4Kosong, "Tempat Lahir")
	}
	if kp4 == nil || strings.TrimSpace(kp4.JenisKelamin) == "" {
		kp4Kosong = append(kp4Kosong, "Jenis Kelamin")
	}
	if kp4 == nil || strings.TrimSpace(kp4.Agama) == "" {
		kp4Kosong = append(kp4Kosong, "Agama")
	}
	if kp4 == nil || strings.TrimSpace(kp4.AlamatJalan) == "" || strings.TrimSpace(kp4.Desa) == "" ||
		strings.TrimSpace(kp4.Kecamatan) == "" || strings.TrimSpace(kp4.Kabupaten) == "" || strings.TrimSpace(kp4.Provinsi) == "" {
		kp4Kosong = append(kp4Kosong, "Alamat Lengkap")
	}
	if kp4 == nil || strings.TrimSpace(kp4.DigajiMenurut) == "" {
		kp4Kosong = append(kp4Kosong, "Digaji Menurut (PP/SK)")
	}
	if kp4 == nil || kp4.BesarnyaPenghasilan <= 0 {
		kp4Kosong = append(kp4Kosong, "Besarnya Penghasilan")
	}
	if kp4 == nil || strings.TrimSpace(kp4.SkTerakhir) == "" {
		kp4Kosong = append(kp4Kosong, "SK Terakhir yang Dimiliki")
	}

	return kp4Kelengkapan{
		Kp4BelumLengkap:   len(kp4Kosong) > 0,
		Kp4FieldKosong:    kp4Kosong,
		DataPegawaiKosong: dataPegawaiKosong,
	}
}

// kp4Ringkasan adalah bentuk lengkap data KP4 satu pegawai yang dikirim ke
// frontend (self-service maupun admin) -- gabungan field dari Pegawai
// (live), Kp4Data, Kp4Pasangan (nullable), daftar Kp4Anak, plus field
// TURUNAN (jumlah_keluarga_tertanggung, masa_kerja_golongan, masa_kerja_
// keseluruhan) yang SENGAJA dihitung di sini setiap kali dibaca -- bukan
// disimpan sebagai kolom -- supaya tidak pernah basi/berbeda dari data
// sumbernya (lihat komentar Kp4Data di models.go).
type kp4Ringkasan struct {
	Pegawai                   models.Pegawai      `json:"pegawai"`
	Kp4Data                   *models.Kp4Data     `json:"kp4_data"`
	Pasangan                  *models.Kp4Pasangan `json:"pasangan"`
	Anak                      []models.Kp4Anak    `json:"anak"`
	JumlahKeluargaTertanggung int                 `json:"jumlah_keluarga_tertanggung"`
	MasaKerjaGolongan         string              `json:"masa_kerja_golongan"`
	MasaKerjaKeseluruhan      string              `json:"masa_kerja_keseluruhan"`
	Kelengkapan               kp4Kelengkapan      `json:"kelengkapan"`
}

func muatKp4Ringkasan(db *gorm.DB, idPegawai uint) (*kp4Ringkasan, error) {
	var pegawai models.Pegawai
	if err := db.Preload("Jabatan").Preload("PangkatGol.Pangkat").Preload("PangkatGol.Gol").
		Preload("Status").Preload("UnitKerja").Omit(dokumenFileFields...).First(&pegawai, idPegawai).Error; err != nil {
		return nil, err
	}

	var kp4 *models.Kp4Data
	var kd models.Kp4Data
	if err := db.Where("id_pegawai = ?", idPegawai).First(&kd).Error; err == nil {
		kp4 = &kd
	}

	var pasangan *models.Kp4Pasangan
	var ps models.Kp4Pasangan
	if err := db.Where("id_pegawai = ?", idPegawai).First(&ps).Error; err == nil {
		pasangan = &ps
	}

	var anak []models.Kp4Anak
	db.Where("id_pegawai = ?", idPegawai).Order("urutan asc, id asc").Find(&anak)

	jumlahTunjangan := 0
	if pasangan != nil {
		jumlahTunjangan++
	}
	for _, a := range anak {
		if a.DapatTunjangan {
			jumlahTunjangan++
		}
	}

	now := time.Now()
	masaKerjaGolongan := masaKerjaText(pegawai.TglKenaikanPangkatTerakhir, now)
	masaKerjaKeseluruhan := masaKerjaText(pegawai.TMT, now)

	return &kp4Ringkasan{
		Pegawai:                   pegawai,
		Kp4Data:                   kp4,
		Pasangan:                  pasangan,
		Anak:                      anak,
		JumlahKeluargaTertanggung: jumlahTunjangan,
		MasaKerjaGolongan:         masaKerjaGolongan,
		MasaKerjaKeseluruhan:      masaKerjaKeseluruhan,
		Kelengkapan:               hitungKelengkapanKp4(pegawai, kp4, pasangan != nil),
	}, nil
}

// kp4KelengkapanUntukDashboard dipakai dashboard.go supaya tampilan
// "pemberitahuan" pada akun pegawai ini konsisten dengan yang ditampilkan di
// menu KP4 itu sendiri, tanpa perlu memuat seluruh ringkasan (anak/pasangan)
// yang tidak dibutuhkan di dashboard.
func kp4KelengkapanUntukDashboard(db *gorm.DB, idPegawai uint) *kp4Kelengkapan {
	var pegawai models.Pegawai
	if err := db.First(&pegawai, idPegawai).Error; err != nil {
		return nil
	}
	var kp4 *models.Kp4Data
	var kd models.Kp4Data
	if err := db.Where("id_pegawai = ?", idPegawai).First(&kd).Error; err == nil {
		kp4 = &kd
	}
	k := hitungKelengkapanKp4(pegawai, kp4, false)
	return &k
}

// simpanKp4 melakukan upsert Kp4Data + Kp4Pasangan (upsert atau hapus kalau
// dikosongkan) + REPLACE seluruh baris Kp4Anak (hapus semua punya pegawai
// ini lalu insert ulang daftar dari payload) dalam SATU transaksi -- pola
// "full replace" ini sama seperti unitKerjaJoinDariPayload pada Shift Kerja
// (handlers/shift_kerja.go): lebih sederhana & tetap benar untuk semantik
// "simpan seluruh form sekali jalan" dibanding diff per baris satu-satu.
// kp4ParseTanggalOpsional parses an optional "YYYY-MM-DD" (or the other
// layouts utils.ParseDateCell accepts) date string, returning nil for an
// empty string rather than an error.
func kp4ParseTanggalOpsional(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := utils.ParseDateCell(s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func simpanKp4(db *gorm.DB, idPegawai uint, p kp4SavePayload) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var kp4 models.Kp4Data
		tx.Where("id_pegawai = ?", idPegawai).First(&kp4)
		kp4.IDPegawai = idPegawai
		kp4.TempatLahir = strings.TrimSpace(p.TempatLahir)
		kp4.JenisKelamin = strings.TrimSpace(p.JenisKelamin)
		kp4.Agama = strings.TrimSpace(p.Agama)
		kp4.AlamatJalan = strings.TrimSpace(p.AlamatJalan)
		kp4.Desa = strings.TrimSpace(p.Desa)
		kp4.Kecamatan = strings.TrimSpace(p.Kecamatan)
		kp4.Kabupaten = strings.TrimSpace(p.Kabupaten)
		kp4.Provinsi = strings.TrimSpace(p.Provinsi)
		kp4.DigajiMenurut = strings.TrimSpace(p.DigajiMenurut)
		kp4.BesarnyaPenghasilan = p.BesarnyaPenghasilan
		kp4.SkTerakhir = strings.TrimSpace(p.SkTerakhir)
		if err := tx.Save(&kp4).Error; err != nil {
			return err
		}

		if p.Pasangan.kosong() {
			if err := tx.Where("id_pegawai = ?", idPegawai).Delete(&models.Kp4Pasangan{}).Error; err != nil {
				return err
			}
		} else {
			var pasangan models.Kp4Pasangan
			tx.Where("id_pegawai = ?", idPegawai).First(&pasangan)
			pasangan.IDPegawai = idPegawai
			pasangan.Nama = strings.TrimSpace(p.Pasangan.Nama)
			pasangan.TempatLahir = strings.TrimSpace(p.Pasangan.TempatLahir)
			tglLahir, err := kp4ParseTanggalOpsional(p.Pasangan.TglLahir)
			if err != nil {
				return err
			}
			pasangan.TglLahir = tglLahir
			pasangan.NIK = strings.TrimSpace(p.Pasangan.NIK)
			pasangan.Pekerjaan = strings.TrimSpace(p.Pasangan.Pekerjaan)
			tglKawin, err := kp4ParseTanggalOpsional(p.Pasangan.TglPerkawinan)
			if err != nil {
				return err
			}
			pasangan.TglPerkawinan = tglKawin
			if p.Pasangan.PasanganKe > 0 {
				pasangan.PasanganKe = p.Pasangan.PasanganKe
			} else {
				pasangan.PasanganKe = 1
			}
			pasangan.Penghasilan = p.Pasangan.Penghasilan
			if err := tx.Save(&pasangan).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("id_pegawai = ?", idPegawai).Delete(&models.Kp4Anak{}).Error; err != nil {
			return err
		}
		for i, a := range p.Anak {
			if strings.TrimSpace(a.Nama) == "" {
				continue
			}
			dariKe := a.DariPasanganKe
			if dariKe <= 0 {
				dariKe = 1
			}
			tglLahirAnak, err := kp4ParseTanggalOpsional(a.TglLahir)
			if err != nil {
				return err
			}
			row := models.Kp4Anak{
				IDPegawai:           idPegawai,
				Urutan:              i + 1,
				Nama:                strings.TrimSpace(a.Nama),
				TempatLahir:         strings.TrimSpace(a.TempatLahir),
				TglLahir:            tglLahirAnak,
				StatusAnak:          strings.TrimSpace(a.StatusAnak),
				DariPasanganKe:      dariKe,
				JenisKelamin:        strings.TrimSpace(a.JenisKelamin),
				DapatTunjangan:      a.DapatTunjangan,
				SudahKawin:          a.SudahKawin,
				SudahBekerja:        a.SudahBekerja,
				MasihSekolah:        a.MasihSekolah,
				NoPutusanPengadilan: strings.TrimSpace(a.NoPutusanPengadilan),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ============================================================
// HTTP handlers
// ============================================================

func RegisterKp4Routes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	pegawaiOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai") }
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }
	// cetak: pegawai sendiri ATAU admin/administrator (canAksesKp4Pegawai di
	// bawah mengecek lagi apakah {id} ini benar milik pegawai yang login).
	cetakRoles := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "pegawai") }

	mux.Handle("GET /api/kp4/saya", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { kp4Saya(w, r, db) }))
	mux.Handle("PUT /api/kp4/saya", pegawaiOnly(func(w http.ResponseWriter, r *http.Request) { simpanKp4Saya(w, r, db) }))
	mux.Handle("GET /api/kp4/saya/cetak", cetakRoles(func(w http.ResponseWriter, r *http.Request) { kp4CetakSaya(w, r, db) }))

	mux.Handle("GET /api/kp4", manage(func(w http.ResponseWriter, r *http.Request) { kp4AdminList(w, r, db) }))
	mux.Handle("GET /api/kp4/export", manage(func(w http.ResponseWriter, r *http.Request) { kp4Export(w, r, db) }))
	mux.Handle("GET /api/kp4/pegawai/{id}", manage(func(w http.ResponseWriter, r *http.Request) { kp4AdminGet(w, r, db) }))
	mux.Handle("PUT /api/kp4/pegawai/{id}", manage(func(w http.ResponseWriter, r *http.Request) { kp4AdminSave(w, r, db) }))
	mux.Handle("GET /api/kp4/pegawai/{id}/cetak", cetakRoles(func(w http.ResponseWriter, r *http.Request) { kp4CetakAdmin(w, r, db) }))

	mux.Handle("GET /api/pengaturan-kp4", manage(func(w http.ResponseWriter, r *http.Request) { getPengaturanKp4(w, r, db) }))
	mux.Handle("PUT /api/pengaturan-kp4", manage(func(w http.ResponseWriter, r *http.Request) { updatePengaturanKp4(w, r, db) }))
}

func kp4Saya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun ini tidak terhubung ke data pegawai")
		return
	}
	ringkasan, err := muatKp4Ringkasan(db, *claims.IDPegawai)
	if err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}
	utils.Success(w, "ok", ringkasan)
}

func simpanKp4Saya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun ini tidak terhubung ke data pegawai")
		return
	}
	var p kp4SavePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := simpanKp4(db, *claims.IDPegawai, p); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan data KP4: "+err.Error())
		return
	}
	ringkasan, err := muatKp4Ringkasan(db, *claims.IDPegawai)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "data tersimpan tapi gagal memuat ulang")
		return
	}
	utils.Success(w, "data KP4 berhasil disimpan", ringkasan)
}

// kp4AdminListItem adalah baris ringkas untuk rekap KP4 semua pegawai (menu
// KP4 tampilan administrator/admin) -- tidak menyertakan anak/pasangan
// (lihat kp4Ringkasan) supaya daftar tetap ringan; detailnya baru dimuat
// saat admin membuka satu pegawai.
type kp4AdminListItem struct {
	IDPegawai       uint   `json:"id_pegawai"`
	NIP             string `json:"nip"`
	Nama            string `json:"nama"`
	Jabatan         string `json:"jabatan"`
	UnitKerja       string `json:"unit_kerja"`
	Kp4BelumLengkap bool   `json:"kp4_belum_lengkap"`
	PegawaiKosong   bool   `json:"data_pegawai_kosong"`
}

// kp4AdminList: rekap kelengkapan KP4 SEMUA pegawai, dengan filter
// ?lengkap=sudah|belum (mengikuti pola filter status kelengkapan yang sudah
// dipakai menu Unit Kerja -- lihat handlers/crud_generic.go ExtraFilters)
// dan pencarian ?q= (nama/NIP).
func kp4AdminList(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var pegawaiList []models.Pegawai
	q := db.Preload("Jabatan").Preload("UnitKerja").Omit(dokumenFileFields...).Order("nama asc")
	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		q = q.Where("LOWER(nama) LIKE ? OR LOWER(nip) LIKE ?", like, like)
	}
	if err := q.Find(&pegawaiList).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}

	var kp4Rows []models.Kp4Data
	db.Find(&kp4Rows)
	kp4ByPegawai := map[uint]models.Kp4Data{}
	for _, k := range kp4Rows {
		kp4ByPegawai[k.IDPegawai] = k
	}

	filterLengkap := r.URL.Query().Get("lengkap")
	items := make([]kp4AdminListItem, 0, len(pegawaiList))
	totalSudahLengkap := 0
	for _, pg := range pegawaiList {
		kd, ada := kp4ByPegawai[pg.ID]
		var kp4Ptr *models.Kp4Data
		if ada {
			kp4Ptr = &kd
		}
		kel := hitungKelengkapanKp4(pg, kp4Ptr, false)
		if !kel.Kp4BelumLengkap {
			totalSudahLengkap++
		}
		if filterLengkap == "sudah" && kel.Kp4BelumLengkap {
			continue
		}
		if filterLengkap == "belum" && !kel.Kp4BelumLengkap {
			continue
		}
		jabatanNama := "-"
		if pg.Jabatan != nil {
			jabatanNama = pg.Jabatan.Jabatan
		}
		unitNama := "-"
		if pg.UnitKerja != nil {
			unitNama = pg.UnitKerja.Unit
		}
		items = append(items, kp4AdminListItem{
			IDPegawai:       pg.ID,
			NIP:             pg.NIP,
			Nama:            pg.Nama,
			Jabatan:         jabatanNama,
			UnitKerja:       unitNama,
			Kp4BelumLengkap: kel.Kp4BelumLengkap,
			PegawaiKosong:   len(kel.DataPegawaiKosong) > 0,
		})
	}

	utils.SuccessMeta(w, "ok", items, map[string]interface{}{
		"total":               len(pegawaiList),
		"total_sudah_lengkap": totalSudahLengkap,
		"total_belum_lengkap": len(pegawaiList) - totalSudahLengkap,
	})
}

// kp4Export menghasilkan file .xlsx satu baris per pegawai berisi SELURUH
// data KP4 (bukan cuma status kelengkapan seperti kp4AdminList di atas) --
// dipakai administrator/admin lewat tombol "Download Excel" pada menu KP4
// utk keperluan administrasi/verifikasi tunjangan keluarga di luar aplikasi
// (mis. dilampirkan ke SPM/berkas gaji). Menerima filter ?q= dan
// ?lengkap=sudah|belum yang sama dengan kp4AdminList supaya file yang
// diunduh konsisten dengan apa yang sedang ditampilkan/difilter di tabel.
func kp4Export(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var pegawaiList []models.Pegawai
	q := db.Preload("Jabatan").Preload("UnitKerja").Omit(dokumenFileFields...).Order("nama asc")
	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		q = q.Where("LOWER(nama) LIKE ? OR LOWER(nip) LIKE ?", like, like)
	}
	if err := q.Find(&pegawaiList).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data pegawai")
		return
	}
	filterLengkap := r.URL.Query().Get("lengkap")

	type kp4ExportRow struct {
		NIP                  string
		Nama                 string
		Jabatan              string
		UnitKerja            string
		TempatLahir          string
		JenisKelamin         string
		Agama                string
		AlamatLengkap        string
		DigajiMenurut        string
		BesarnyaPenghasilan  string
		SkTerakhir           string
		JumlahKeluarga       string
		MasaKerjaGolongan    string
		MasaKerjaKeseluruhan string
		NamaPasangan         string
		NikPasangan          string
		PekerjaanPasangan    string
		CatatanPasangan      string
		JumlahAnak           string
		DaftarNamaAnak       string
		StatusKp4            string
		DataPegawai          string
	}

	// kp4CatatanTanggungan: label "TERTANGGUNG"/"TIDAK TERTANGGUNG" yang
	// dipakai kolom "Catatan" pada rekap KP4 -- mengikuti format yang sudah
	// biasa dipakai sekolah secara manual (contoh: rekap KP4 SMPN 1 Bungku
	// Utara/Butar yang diberikan pengguna): pasangan SELALU "Tertanggung"
	// begitu datanya ada (sama dengan aturan jumlah_keluarga_tertanggung di
	// muatKp4Ringkasan -- pasangan tidak punya flag dapat/tidak tunjangan
	// sendiri seperti anak), sedangkan anak ikut/tidak ikut "Tertanggung"
	// sesuai Kp4Anak.DapatTunjangan yang memang sudah diisi pegawai di form.
	kp4CatatanTanggungan := func(tertanggung bool) string {
		if tertanggung {
			return "Tertanggung"
		}
		return "Tidak Tertanggung"
	}

	var rows []kp4ExportRow
	for _, pg := range pegawaiList {
		// dipanggil per-pegawai (bukan query massal) supaya field turunan
		// (jumlah_keluarga_tertanggung/masa_kerja_*) & kelengkapan dihitung
		// oleh SATU fungsi yang sama dipakai tampilan KP4 lainnya (lihat
		// catatan "dihitung di sini setiap kali dibaca" pada kp4Ringkasan) --
		// jumlah pegawai kecil jadi N+1 query di sini bukan masalah performa.
		ring, err := muatKp4Ringkasan(db, pg.ID)
		if err != nil {
			continue
		}
		if filterLengkap == "sudah" && ring.Kelengkapan.Kp4BelumLengkap {
			continue
		}
		if filterLengkap == "belum" && !ring.Kelengkapan.Kp4BelumLengkap {
			continue
		}

		jabatanNama := "-"
		if pg.Jabatan != nil {
			jabatanNama = pg.Jabatan.Jabatan
		}
		unitNama := "-"
		if pg.UnitKerja != nil {
			unitNama = pg.UnitKerja.Unit
		}

		out := kp4ExportRow{
			NIP:                  namaOrDash(pg.NIP),
			Nama:                 pg.Nama,
			Jabatan:              jabatanNama,
			UnitKerja:            unitNama,
			TempatLahir:          "-",
			JenisKelamin:         "-",
			Agama:                "-",
			AlamatLengkap:        "-",
			DigajiMenurut:        "-",
			BesarnyaPenghasilan:  "-",
			SkTerakhir:           "-",
			JumlahKeluarga:       strconv.Itoa(ring.JumlahKeluargaTertanggung),
			MasaKerjaGolongan:    ring.MasaKerjaGolongan,
			MasaKerjaKeseluruhan: ring.MasaKerjaKeseluruhan,
			NamaPasangan:         "-",
			NikPasangan:          "-",
			PekerjaanPasangan:    "-",
			CatatanPasangan:      "-",
			JumlahAnak:           strconv.Itoa(len(ring.Anak)),
			DaftarNamaAnak:       "-",
		}
		if ring.Kp4Data != nil {
			kp4 := ring.Kp4Data
			out.TempatLahir = namaOrDash(kp4.TempatLahir)
			out.JenisKelamin = jenisKelaminLabel(kp4.JenisKelamin)
			out.Agama = namaOrDash(kp4.Agama)
			out.AlamatLengkap = fmt.Sprintf("%s, Desa/Kel. %s, Kec. %s, %s, %s",
				namaOrDash(kp4.AlamatJalan), namaOrDash(kp4.Desa), namaOrDash(kp4.Kecamatan), namaOrDash(kp4.Kabupaten), namaOrDash(kp4.Provinsi))
			out.DigajiMenurut = namaOrDash(kp4.DigajiMenurut)
			out.BesarnyaPenghasilan = formatRupiahKp4(kp4.BesarnyaPenghasilan)
			out.SkTerakhir = namaOrDash(kp4.SkTerakhir)
		}
		if ring.Pasangan != nil {
			out.NamaPasangan = namaOrDash(ring.Pasangan.Nama)
			out.NikPasangan = namaOrDash(ring.Pasangan.NIK)
			out.PekerjaanPasangan = namaOrDash(ring.Pasangan.Pekerjaan)
			// pasangan ada -> selalu Tertanggung (lihat catatan
			// kp4CatatanTanggungan di atas).
			out.CatatanPasangan = kp4CatatanTanggungan(true)
		}
		if len(ring.Anak) > 0 {
			namaAnak := make([]string, 0, len(ring.Anak))
			for _, a := range ring.Anak {
				namaAnak = append(namaAnak, fmt.Sprintf("%s (%s)", a.Nama, kp4CatatanTanggungan(a.DapatTunjangan)))
			}
			out.DaftarNamaAnak = strings.Join(namaAnak, "; ")
		}
		if ring.Kelengkapan.Kp4BelumLengkap {
			out.StatusKp4 = "Belum Lengkap"
		} else {
			out.StatusKp4 = "Sudah Lengkap"
		}
		if len(ring.Kelengkapan.DataPegawaiKosong) > 0 {
			out.DataPegawai = "Ada Data Kosong"
		} else {
			out.DataPegawai = "Lengkap"
		}
		rows = append(rows, out)
	}

	columns := []utils.ExcelColumn{
		{Header: "NIP", Get: func(item interface{}) string { return item.(kp4ExportRow).NIP }},
		{Header: "Nama", Get: func(item interface{}) string { return item.(kp4ExportRow).Nama }},
		{Header: "Jabatan", Get: func(item interface{}) string { return item.(kp4ExportRow).Jabatan }},
		{Header: "Unit Kerja", Get: func(item interface{}) string { return item.(kp4ExportRow).UnitKerja }},
		{Header: "Tempat Lahir", Get: func(item interface{}) string { return item.(kp4ExportRow).TempatLahir }},
		{Header: "Jenis Kelamin", Get: func(item interface{}) string { return item.(kp4ExportRow).JenisKelamin }},
		{Header: "Agama", Get: func(item interface{}) string { return item.(kp4ExportRow).Agama }},
		{Header: "Alamat Lengkap", Get: func(item interface{}) string { return item.(kp4ExportRow).AlamatLengkap }},
		{Header: "Digaji Menurut (PP/SK)", Get: func(item interface{}) string { return item.(kp4ExportRow).DigajiMenurut }},
		{Header: "Besarnya Penghasilan", Get: func(item interface{}) string { return item.(kp4ExportRow).BesarnyaPenghasilan }},
		{Header: "SK Terakhir yang Dimiliki", Get: func(item interface{}) string { return item.(kp4ExportRow).SkTerakhir }},
		{Header: "Jumlah Keluarga Tertanggung", Get: func(item interface{}) string { return item.(kp4ExportRow).JumlahKeluarga }},
		{Header: "Masa Kerja Golongan", Get: func(item interface{}) string { return item.(kp4ExportRow).MasaKerjaGolongan }},
		{Header: "Masa Kerja Keseluruhan", Get: func(item interface{}) string { return item.(kp4ExportRow).MasaKerjaKeseluruhan }},
		{Header: "Nama Isteri/Suami", Get: func(item interface{}) string { return item.(kp4ExportRow).NamaPasangan }},
		{Header: "NIK Isteri/Suami", Get: func(item interface{}) string { return item.(kp4ExportRow).NikPasangan }},
		{Header: "Pekerjaan Isteri/Suami", Get: func(item interface{}) string { return item.(kp4ExportRow).PekerjaanPasangan }},
		{Header: "Catatan Isteri/Suami", Get: func(item interface{}) string { return item.(kp4ExportRow).CatatanPasangan }},
		{Header: "Jumlah Anak", Get: func(item interface{}) string { return item.(kp4ExportRow).JumlahAnak }},
		{Header: "Daftar Anak (Nama & Catatan Tanggungan)", Get: func(item interface{}) string { return item.(kp4ExportRow).DaftarNamaAnak }},
		{Header: "Status KP4", Get: func(item interface{}) string { return item.(kp4ExportRow).StatusKp4 }},
		{Header: "Data Pegawai", Get: func(item interface{}) string { return item.(kp4ExportRow).DataPegawai }},
	}
	f, err := utils.ExportData(rows, columns)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat file excel: "+err.Error())
		return
	}
	writeXlsxResponse(w, f, "rekap_kp4_"+time.Now().Format("2006-01-02")+".xlsx")
}

func kp4IDFromPath(r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

func kp4AdminGet(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	idPegawai, ok := kp4IDFromPath(r)
	if !ok {
		utils.Error(w, http.StatusBadRequest, "id pegawai tidak valid")
		return
	}
	ringkasan, err := muatKp4Ringkasan(db, idPegawai)
	if err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}
	utils.Success(w, "ok", ringkasan)
}

func kp4AdminSave(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	idPegawai, ok := kp4IDFromPath(r)
	if !ok {
		utils.Error(w, http.StatusBadRequest, "id pegawai tidak valid")
		return
	}
	var p kp4SavePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	if err := simpanKp4(db, idPegawai, p); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan data KP4: "+err.Error())
		return
	}
	ringkasan, err := muatKp4Ringkasan(db, idPegawai)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "data tersimpan tapi gagal memuat ulang")
		return
	}
	utils.Success(w, "data KP4 berhasil disimpan", ringkasan)
}

func kp4CetakSaya(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun ini tidak terhubung ke data pegawai")
		return
	}
	kp4CetakUntukPegawai(w, r, db, *claims.IDPegawai)
}

func kp4CetakAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	idPegawai, ok := kp4IDFromPath(r)
	if !ok {
		utils.Error(w, http.StatusBadRequest, "id pegawai tidak valid")
		return
	}
	claims, _ := middleware.GetClaims(r)
	if claims.RoleName == "pegawai" && (claims.IDPegawai == nil || *claims.IDPegawai != idPegawai) {
		utils.Error(w, http.StatusForbidden, "tidak boleh mencetak KP4 pegawai lain")
		return
	}
	kp4CetakUntukPegawai(w, r, db, idPegawai)
}

// ============================================================
// Pengaturan KP4 (singleton, sama pola dengan Pengaturan Formulir)
// ============================================================

type pengaturanKp4Payload struct {
	NamaInstansi     string `json:"nama_instansi"`
	AlamatInstansi   string `json:"alamat_instansi"`
	InstansiInduk    string `json:"instansi_induk"`
	BendaharawanGaji string `json:"bendaharawan_gaji"`
}

func getPengaturanKp4(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var item models.PengaturanKp4
	if err := db.First(&item, 1).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pengaturan belum tersedia")
		return
	}
	utils.Success(w, "ok", item)
}

func updatePengaturanKp4(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p pengaturanKp4Payload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	var item models.PengaturanKp4
	if err := db.First(&item, 1).Error; err != nil {
		item = models.PengaturanKp4{ID: 1}
	}
	item.NamaInstansi = p.NamaInstansi
	item.AlamatInstansi = p.AlamatInstansi
	item.InstansiInduk = p.InstansiInduk
	item.BendaharawanGaji = p.BendaharawanGaji
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan pengaturan: "+err.Error())
		return
	}
	utils.Success(w, "pengaturan KP4 berhasil disimpan", item)
}

func pengaturanKp4OrDefault(db *gorm.DB) models.PengaturanKp4 {
	var p models.PengaturanKp4
	db.First(&p, 1)
	return p
}
