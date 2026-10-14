package database

import (
	"log"

	"cuti-app/models"

	"gorm.io/gorm"
)

// ============================================================
// Seed struktur awal "Peta Jabatan Dinas" -- HANYA dijalankan SEKALI (lihat
// SeedPetaJabatanDinas di bawah: dilewati total begitu node akar/root sudah
// ada) untuk mengisi pohon Sekretariat/Bidang/Seksi/Sub Bagian milik Dinas
// Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara beserta daftar
// Jabatan Pelaksana/Fungsional & kebutuhannya ("K"), ditranskripsi dari
// berkas ANJAB yang diunggah pengguna. Setelah seed awal ini,
// administrator BEBAS mengubah/menambah/menghapus struktur lewat menu
// "Peta Jabatan Dinas" (lihat handlers/peta_jabatan_dinas.go) -- baris ini
// tidak pernah dijalankan ulang/menimpa perubahan administrator.
//
// CATATAN PENTING soal lingkup data: berkas ANJAB sumbernya juga memuat
// tabel "REKAPITULASI" (total per jenjang eselon JPT Utama/Madya/Pratama,
// Jabatan Administrator, Jabatan Pengawas, Jabatan Fungsional, Jabatan
// Pelaksana) -- tabel itu SENGAJA TIDAK diikutkan di sini (di luar lingkup
// permintaan pengguna, & skema 2 kategori JenisJabatan yang ada sekarang
// tidak cocok dipakai ulang untuk jenjang eselon tanpa perubahan model yang
// lebih besar). Begitu juga Bidang Kebudayaan TIDAK memiliki data Seksi di
// berkas ANJAB (hanya Jabatan Pelaksana/Fungsional langsung di bawah
// Bidang), jadi node ini sengaja dibuat tanpa anak.
//
// "NamaKepala"/"KelasKepala" pada tiap node BUKAN disimpan sebagai kolom di
// UnitKerja -- field itu di sini HANYA dipakai untuk memastikan baris
// master models.Jabatan untuk jabatan KEPALA unit tersebut sudah tersedia
// (dibuat kalau belum ada), supaya begitu administrator menugaskan seorang
// pegawai sebagai Atasan Langsung (lihat UnitKerja.IDAtasanLangsung) pada
// node ini, administrator bisa langsung memilih Jabatan yang sudah pas
// tanpa perlu membuatnya manual dulu lewat Master Data Jabatan. Siapa yang
// benar-benar menjabat (nama pegawai) TETAP diisi administrator sendiri
// lewat menu Unit Kerja/Peta Jabatan Dinas, sama seperti Peta Jabatan
// Sekolah yang sudah ada.
type jabatanDinasSeed struct {
	Nama      string
	Jenis     string
	Kelas     int
	Kebutuhan int
}

type nodeDinasSeed struct {
	Unit        string
	Tipe        string
	NamaKepala  string // kosong HANYA untuk root (Kepala Dinas dipakai lewat resolveSignerFullRekomendasi, bukan Jabatan master biasa)
	KelasKepala int
	Jabatan     []jabatanDinasSeed
	Anak        []nodeDinasSeed
}

var strukturDinasSeedAwal = nodeDinasSeed{
	Unit: "Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara",
	Tipe: models.StrukturDinasRoot,
	Anak: []nodeDinasSeed{
		{
			Unit: "Sekretariat", Tipe: models.StrukturDinasSekretariat,
			NamaKepala: "Sekretaris", KelasKepala: 12,
			Anak: []nodeDinasSeed{
				{
					Unit: "Sub Bagian Umum dan Kepegawaian", Tipe: models.StrukturDinasSubBagian,
					NamaKepala: "Kepala Sub Bagian Umum dan Kepegawaian", KelasKepala: 9,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 8},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 1},
						{"Pengolah Data dan Informasi", models.JenisJabatanPelaksana, 6, 0},
						{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 3},
						{"Operator Layanan Operasional", models.JenisJabatanPelaksana, 5, 0},
						{"Perencana Ahli Madya", models.JenisJabatanFungsional, 10, 2},
						{"Perencana Ahli Muda", models.JenisJabatanFungsional, 9, 2},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 2},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 4},
						{"Arsiparis Ahli Pertama", models.JenisJabatanFungsional, 8, 4},
					},
				},
			},
		},
		{
			Unit: "Bidang Pembinaan Pendidikan Dasar", Tipe: models.StrukturDinasBidang,
			NamaKepala: "Kepala Bidang Pembinaan Pendidikan Dasar", KelasKepala: 11,
			Jabatan: []jabatanDinasSeed{
				{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 2},
				{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
				{"Pengembang Kurikulum", models.JenisJabatanFungsional, 9, 1},
			},
			Anak: []nodeDinasSeed{
				{
					Unit: "Seksi Kurikulum dan Penilaian Pendidikan Dasar", Tipe: models.StrukturDinasSeksi,
					NamaKepala: "Kepala Seksi Kurikulum dan Penilaian Pendidikan Dasar", KelasKepala: 11,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 1},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 1},
						{"Pengolah Data dan Informasi", models.JenisJabatanPelaksana, 6, 1},
						{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 1},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
					},
				},
				{
					Unit: "Seksi Kelembagaan Sarana dan Prasarana Pendidikan Dasar", Tipe: models.StrukturDinasSeksi,
					NamaKepala: "Kepala Seksi Kelembagaan Sarana dan Prasarana Pendidikan Dasar", KelasKepala: 11,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 2},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 2},
						{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 1},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
					},
				},
			},
		},
		{
			Unit: "Bidang PAUD dan PNF", Tipe: models.StrukturDinasBidang,
			NamaKepala: "Kepala Bidang PAUD dan PNF", KelasKepala: 11,
			Jabatan: []jabatanDinasSeed{
				{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 4},
				{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 1},
				{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 1},
				{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
				{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
			},
			Anak: []nodeDinasSeed{
				{
					Unit: "Seksi Kurikulum dan Pendidikan PAUD dan PNF", Tipe: models.StrukturDinasSeksi,
					NamaKepala: "Kepala Seksi Kurikulum dan Pendidikan PAUD dan PNF", KelasKepala: 11,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 1},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 3},
						{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 2},
						{"Pamong Belajar Ahli Pertama", models.JenisJabatanFungsional, 8, 2},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 2},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
					},
				},
				{
					Unit: "Seksi Kelembagaan Sarana dan Prasarana PAUD dan PNF", Tipe: models.StrukturDinasSeksi,
					NamaKepala: "Kepala Seksi Kelembagaan Sarana dan Prasarana PAUD dan PNF", KelasKepala: 11,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 1},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 2},
						{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 2},
						{"Pamong Belajar Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
					},
				},
			},
		},
		{
			Unit: "Bidang Pembinaan Ketenagaan (GTK)", Tipe: models.StrukturDinasBidang,
			NamaKepala: "Kepala Bidang Pembinaan Ketenagaan (GTK)", KelasKepala: 11,
			Jabatan: []jabatanDinasSeed{
				{"Pamong Budaya Ahli Muda", models.JenisJabatanFungsional, 9, 1},
				{"Pengawas Sekolah Ahli Utama", models.JenisJabatanFungsional, 13, 4},
				{"Pengawas Sekolah Ahli Madya", models.JenisJabatanFungsional, 11, 15},
				{"Pengawas Sekolah Ahli Muda", models.JenisJabatanFungsional, 9, 21},
				{"Pengawas Sekolah Ahli Pertama", models.JenisJabatanFungsional, 8, 0},
				{"Penilik Sekolah Ahli Madya", models.JenisJabatanFungsional, 11, 10},
				{"Penilik Sekolah Ahli Muda", models.JenisJabatanFungsional, 9, 10},
				{"Penilik Sekolah Ahli Pertama", models.JenisJabatanFungsional, 8, 5},
			},
			Anak: []nodeDinasSeed{
				{
					Unit: "Seksi Pendidik dan Tenaga Kependidikan Pendidikan Dasar", Tipe: models.StrukturDinasSeksi,
					NamaKepala: "Kepala Seksi Pendidik dan Tenaga Kependidikan Pendidikan Dasar", KelasKepala: 11,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 4},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 2},
						{"Pengolah Data dan Informasi", models.JenisJabatanPelaksana, 6, 1},
						{"Pengadministrasi Perkantoran", models.JenisJabatanPelaksana, 5, 1},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 2},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
					},
				},
				{
					Unit: "Seksi Pendidik dan Tenaga Kependidikan PAUD dan PNF", Tipe: models.StrukturDinasSeksi,
					NamaKepala: "Kepala Seksi Pendidik dan Tenaga Kependidikan PAUD dan PNF", KelasKepala: 11,
					Jabatan: []jabatanDinasSeed{
						{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 1},
						{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 2},
						{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
						{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
					},
				},
			},
		},
		{
			Unit: "Bidang Kebudayaan", Tipe: models.StrukturDinasBidang,
			NamaKepala: "Kepala Bidang Kebudayaan", KelasKepala: 11,
			Jabatan: []jabatanDinasSeed{
				{"Penelaah Teknis Kebijakan", models.JenisJabatanPelaksana, 7, 4},
				{"Penata Layanan Operasional", models.JenisJabatanPelaksana, 7, 2},
				{"Pengolah Data dan Informasi", models.JenisJabatanPelaksana, 6, 1},
				{"Pamong Budaya Ahli Muda", models.JenisJabatanFungsional, 9, 1},
				{"Pamong Budaya Ahli Pertama", models.JenisJabatanFungsional, 8, 7},
				{"Pengembang Penilaian Pendidikan", models.JenisJabatanFungsional, 9, 1},
				{"Perencana Ahli Pertama", models.JenisJabatanFungsional, 8, 1},
				{"Pranata Komputer Ahli Pertama", models.JenisJabatanFungsional, 8, 2},
			},
		},
	},
}

// getOrCreateJabatanDinas: cari baris models.Jabatan berdasarkan nama persis
// (case-sensitive -- sejalan dengan cara Master Data Jabatan menyimpan nama
// apa adanya di tempat lain pada kode ini), buat baru kalau belum ada.
// Dipakai BERULANG untuk jabatan staf yang sama munculnya di banyak node
// (mis. "Penelaah Teknis Kebijakan") supaya semuanya menunjuk ke SATU baris
// Jabatan yang sama, bukan baris duplikat per node.
func getOrCreateJabatanDinas(db *gorm.DB, cache map[string]uint, nama, jenis string, kelas int) (uint, error) {
	if id, ok := cache[nama]; ok {
		return id, nil
	}
	var existing models.Jabatan
	err := db.Where("jabatan = ?", nama).First(&existing).Error
	if err == nil {
		cache[nama] = existing.ID
		return existing.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}
	kelasCopy := kelas
	baru := models.Jabatan{Jabatan: nama, JenisJabatan: jenis, KelasJabatan: &kelasCopy}
	if err := db.Create(&baru).Error; err != nil {
		return 0, err
	}
	cache[nama] = baru.ID
	return baru.ID, nil
}

// buatNodeDinas: buat (rekursif) satu node UnitKerja beserta seluruh
// anaknya dari definisi nodeDinasSeed, lengkap dengan baris Jabatan master
// (kepala & staf) dan FormasiJabatan (K) staf-nya. idInduk nil untuk root.
func buatNodeDinas(db *gorm.DB, cacheJabatan map[string]uint, node nodeDinasSeed, idInduk *uint) error {
	tempatKerja := models.TempatKerjaDinas
	tipe := node.Tipe
	unitBaru := models.UnitKerja{
		Unit:              node.Unit,
		TempatKerja:       &tempatKerja,
		TipeStrukturDinas: &tipe,
		IDIndukUnitKerja:  idInduk,
	}
	if err := db.Create(&unitBaru).Error; err != nil {
		return err
	}

	// Jabatan KEPALA node ini -- hanya dipastikan ADA di Master Data Jabatan
	// (lihat komentar jabatanDinasSeed di atas), tidak dibuat FormasiJabatan
	// apa pun untuknya (bukan bagian dari tabel staf Pelaksana/Fungsional).
	if node.NamaKepala != "" {
		if _, err := getOrCreateJabatanDinas(db, cacheJabatan, node.NamaKepala, models.JenisJabatanStruktural, node.KelasKepala); err != nil {
			return err
		}
	}

	for _, j := range node.Jabatan {
		idJabatan, err := getOrCreateJabatanDinas(db, cacheJabatan, j.Nama, j.Jenis, j.Kelas)
		if err != nil {
			return err
		}
		formasi := models.FormasiJabatan{
			IDUnitKerja: unitBaru.ID,
			IDJabatan:   idJabatan,
			Kebutuhan:   j.Kebutuhan,
		}
		if err := db.Create(&formasi).Error; err != nil {
			return err
		}
	}

	for _, anak := range node.Anak {
		idBaru := unitBaru.ID
		if err := buatNodeDinas(db, cacheJabatan, anak, &idBaru); err != nil {
			return err
		}
	}
	return nil
}

// SeedPetaJabatanDinas: membuat pohon struktur awal "Peta Jabatan Dinas"
// (lihat strukturDinasSeedAwal di atas) HANYA kalau belum ada SATU PUN
// UnitKerja berjenis StrukturDinasRoot di database -- begitu root sudah ada
// (baik dari seed ini maupun dibuat manual administrator lewat menu Peta
// Jabatan Dinas), fungsi ini langsung berhenti tanpa melakukan apa-apa,
// aman dipanggil berkali-kali setiap kali server start (dipanggil dari
// Migrate di migrate.go).
func SeedPetaJabatanDinas(db *gorm.DB) {
	var jumlahRoot int64
	if err := db.Model(&models.UnitKerja{}).
		Where("tipe_struktur_dinas = ?", models.StrukturDinasRoot).
		Count(&jumlahRoot).Error; err != nil {
		log.Printf("peringatan: gagal cek root Peta Jabatan Dinas: %v", err)
		return
	}
	if jumlahRoot > 0 {
		return
	}
	cacheJabatan := map[string]uint{}
	if err := buatNodeDinas(db, cacheJabatan, strukturDinasSeedAwal, nil); err != nil {
		log.Printf("peringatan: gagal seed struktur awal Peta Jabatan Dinas: %v", err)
		return
	}
	log.Println("migrasi: struktur awal Peta Jabatan Dinas berhasil dibuat dari data ANJAB")
}
