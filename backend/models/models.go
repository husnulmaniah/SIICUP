package models

import "time"

// ============================================================
// MASTER DATA TABLES (simple lookup tables)
// ============================================================

type Role struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Role string `json:"role" gorm:"size:50;not null;unique"`
}

func (Role) TableName() string { return "role" }

type Jabatan struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	Jabatan string `json:"jabatan" gorm:"size:100;not null"`
}

func (Jabatan) TableName() string { return "jabatan" }

type UnitKerja struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Unit string `json:"unit" gorm:"size:150;not null"`
}

func (UnitKerja) TableName() string { return "unit_kerja" }

type Status struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	Status string `json:"status" gorm:"size:50;not null"`
}

func (Status) TableName() string { return "status" }

type Pangkat struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	Pangkat string `json:"pangkat" gorm:"size:100;not null"`
}

func (Pangkat) TableName() string { return "pangkat" }

type Golongan struct {
	ID  uint   `json:"id" gorm:"primaryKey"`
	Gol string `json:"gol" gorm:"size:20;not null"`
}

func (Golongan) TableName() string { return "golongan" }

type PangkatGol struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	IDPangkat uint      `json:"id_pangkat" gorm:"column:id_pangkat;not null"`
	Pangkat   *Pangkat  `json:"pangkat,omitempty" gorm:"foreignKey:IDPangkat;references:ID"`
	IDGol     uint      `json:"id_gol" gorm:"column:id_gol;not null"`
	Gol       *Golongan `json:"gol,omitempty" gorm:"foreignKey:IDGol;references:ID"`
}

func (PangkatGol) TableName() string { return "pangkat_gol" }

type JenisCuti struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	Jenis        string `json:"jenis" gorm:"size:100;not null"`
	DefaultJatah int    `json:"default_jatah" gorm:"column:default_jatah;default:12"`
	Keterangan   string `json:"keterangan" gorm:"size:255"`
}

func (JenisCuti) TableName() string { return "jenis_cuti" }

type PolaHariKerja struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Pola string `json:"pola" gorm:"size:100;not null"`
}

func (PolaHariKerja) TableName() string { return "pola_hari_kerja" }

type TglMerah struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	Tgl        time.Time `json:"tgl" gorm:"type:date;not null"`
	Keterangan string    `json:"keterangan" gorm:"size:255"`
}

func (TglMerah) TableName() string { return "tgl_merah" }

// ============================================================
// PEGAWAI
// ============================================================

type Pegawai struct {
	ID           uint        `json:"id" gorm:"primaryKey"`
	NIP          string      `json:"nip" gorm:"column:nip;size:30;not null;unique"`
	Nama         string      `json:"nama" gorm:"size:150;not null"`
	IDJabatan    *uint       `json:"id_jabatan" gorm:"column:id_jabatan"`
	Jabatan      *Jabatan    `json:"jabatan,omitempty" gorm:"foreignKey:IDJabatan;references:ID"`
	IDUnitKerja  *uint       `json:"id_unit_kerja" gorm:"column:id_unit_kerja"`
	UnitKerja    *UnitKerja  `json:"unit_kerja,omitempty" gorm:"foreignKey:IDUnitKerja;references:ID"`
	IDPangkatGol *uint       `json:"id_pangkat_gol" gorm:"column:id_pangkat_gol"`
	PangkatGol   *PangkatGol `json:"pangkat_gol,omitempty" gorm:"foreignKey:IDPangkatGol;references:ID"`
	TempatTgs    string      `json:"tempat_tgs" gorm:"column:tempat_tgs;size:150"`
	TMT          *time.Time  `json:"tmt" gorm:"column:tmt;type:date"`
	NoHP         string      `json:"no_hp" gorm:"column:no_hp;size:20"`
	IDStatus     *uint       `json:"id_status" gorm:"column:id_status"`
	Status       *Status     `json:"status,omitempty" gorm:"foreignKey:IDStatus;references:ID"`
	IDAtasan     *uint       `json:"id_atasan" gorm:"column:id_atasan"`
	Atasan       *Pegawai    `json:"atasan,omitempty" gorm:"foreignKey:IDAtasan;references:ID"`
	Email        string      `json:"email" gorm:"size:100"`

	// Dokumen kepegawaian (disimpan langsung di database sebagai bytea agar
	// tidak hilang saat container backend di-redeploy/restart).
	SkTerakhirNama string `json:"sk_terakhir_nama" gorm:"column:sk_terakhir_nama;size:255"`
	SkTerakhirFile []byte `json:"-" gorm:"column:sk_terakhir_file;type:bytea"`
	SkKgbNama      string `json:"sk_kgb_nama" gorm:"column:sk_kgb_nama;size:255"`
	SkKgbFile      []byte `json:"-" gorm:"column:sk_kgb_file;type:bytea"`
	SkPensiunNama  string `json:"sk_pensiun_nama" gorm:"column:sk_pensiun_nama;size:255"`
	SkPensiunFile  []byte `json:"-" gorm:"column:sk_pensiun_file;type:bytea"`
}

func (Pegawai) TableName() string { return "pegawai" }

// ============================================================
// USER (login account)
// ============================================================

type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"size:50;not null;unique"`
	Pass      string    `json:"-" gorm:"column:pass;size:255;not null"`
	Nama      string    `json:"nama" gorm:"size:150;not null"`
	IDRole    uint      `json:"id_role" gorm:"column:id_role;not null"`
	Role      *Role     `json:"role,omitempty" gorm:"foreignKey:IDRole;references:ID"`
	IDPegawai *uint     `json:"id_pegawai" gorm:"column:id_pegawai"`
	Pegawai   *Pegawai  `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	TglDibuat time.Time `json:"tgl_dibuat" gorm:"column:tgl_dibuat;autoCreateTime"`
}

func (User) TableName() string { return "user" }

// ============================================================
// JATAH CUTI (leave quota per pegawai per year)
// ============================================================

type JatahCuti struct {
	ID         uint     `json:"id" gorm:"primaryKey"`
	IDPegawai  uint     `json:"id_pegawai" gorm:"column:id_pegawai;not null"`
	Pegawai    *Pegawai `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	Tahun      int      `json:"tahun" gorm:"not null"`
	JumlahHari int      `json:"jumlah_hari" gorm:"column:jumlah_hari;not null;default:12"`
	Terpakai   int      `json:"terpakai" gorm:"default:0"`
}

func (JatahCuti) TableName() string { return "jatah_cuti" }

func (j JatahCuti) Sisa() int {
	return j.JumlahHari - j.Terpakai
}

// ============================================================
// PENGAJUAN CUTI (leave request + approval workflow)
// ============================================================

const (
	StatusPending      = "pending"
	StatusDisetuju     = "disetujui"
	StatusDitolak      = "ditolak"
	StatusDikembalikan = "dikembalikan" // dikembalikan ke pegawai untuk diperbaiki (mis. berkas tidak sesuai), bukan ditolak final
)

type PengajuanCuti struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	IDPegawai        uint           `json:"id_pegawai" gorm:"column:id_pegawai;not null"`
	Pegawai          *Pegawai       `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	IDJenisCuti      uint           `json:"id_jenis_cuti" gorm:"column:id_jenis_cuti;not null"`
	JenisCuti        *JenisCuti     `json:"jenis_cuti,omitempty" gorm:"foreignKey:IDJenisCuti;references:ID"`
	TglMulai         time.Time      `json:"tgl_mulai" gorm:"column:tgl_mulai;type:date;not null"`
	TglSelesai       time.Time      `json:"tgl_selesai" gorm:"column:tgl_selesai;type:date;not null"`
	IDPolaHariKerja  *uint          `json:"id_pola_hari_kerja" gorm:"column:id_pola_hari_kerja"`
	PolaHariKerja    *PolaHariKerja `json:"pola_hari_kerja,omitempty" gorm:"foreignKey:IDPolaHariKerja;references:ID"`
	AlasanCuti       string         `json:"alasan_cuti" gorm:"column:alasan_cuti;size:255"`
	AlamatSelamaCuti string         `json:"alamat_selama_cuti" gorm:"column:alamat_selama_cuti;size:255"`
	JumlahHari       int            `json:"jumlah_hari" gorm:"column:jumlah_hari;default:0"`
	Status           string         `json:"status" gorm:"size:20;default:pending"`
	IDAtasanApprove  *uint          `json:"id_atasan_approve" gorm:"column:id_atasan_approve"`
	AtasanApprove    *Pegawai       `json:"atasan_approve,omitempty" gorm:"foreignKey:IDAtasanApprove;references:ID"`
	TglApproval      *time.Time     `json:"tgl_approval" gorm:"column:tgl_approval"`
	CatatanApproval  string         `json:"catatan_approval" gorm:"column:catatan_approval;size:255"`
	// NomorSurat: nomor surat pada Surat Rekomendasi Izin Cuti (mis.
	// "800.1.11.4/123/Disdikbud"). Opsional -- boleh dikosongkan (tetap
	// tampil sebagai kolom kosong di suratnya seperti sebelum field ini ada)
	// -- dan hanya administrator/admin yang boleh mengisi/mengubahnya
	// (lihat updateNomorSurat di handlers/pengajuan_cuti.go).
	NomorSurat string `json:"nomor_surat" gorm:"column:nomor_surat;size:100"`
	// Snapshot penandatangan (Kepala Dinas/pelaksana tugas) & siapa yang
	// meng-ACC pengajuan ini di sistem -- diisi SEKALI oleh approvePengajuan
	// (handlers/pengajuan_cuti.go) saat status berubah jadi disetujui, dari
	// Pengaturan Formulir yang berlaku SAAT ITU. Sengaja tidak ikut berubah
	// lagi walau Pengaturan Formulir diubah setelahnya -- supaya pengajuan
	// yang sudah disetujui dengan tanda tangan lama tetap menampilkan tanda
	// tangan lama di formulirnya; hanya pengajuan yang belum/akan disetujui
	// yang memakai pengaturan terbaru. Dikosongkan lagi oleh returnPengajuan
	// (batal setuju -> pending) atau updatePengajuan (reset ke pending saat
	// diedit), supaya persetujuan berikutnya menghitung ulang dari pengaturan
	// yang berlaku saat itu. Dipakai juga sebagai isi barcode/QR tanda tangan
	// otomatis pada formulir cetak (lihat buildSignatureQR di formulir.go).
	TtdNama               string    `json:"ttd_nama" gorm:"column:ttd_nama;size:150"`
	TtdNip                string    `json:"ttd_nip" gorm:"column:ttd_nip;size:30"`
	TtdJabatan            string    `json:"ttd_jabatan" gorm:"column:ttd_jabatan;size:150"`
	DisetujuiOlehUsername string    `json:"disetujui_oleh_username" gorm:"column:disetujui_oleh_username;size:100"`
	DisetujuiOlehRole     string    `json:"disetujui_oleh_role" gorm:"column:disetujui_oleh_role;size:20"`
	CreatedAt             time.Time `json:"created_at"`

	// Dokumen kelengkapan yang diupload saat pengajuan dibuat (wajib untuk
	// pengajuan mandiri oleh pegawai/atasan, opsional bila dibuatkan oleh admin).
	Dokumen []PengajuanDokumen `json:"dokumen,omitempty" gorm:"foreignKey:IDPengajuan;references:ID"`
}

func (PengajuanCuti) TableName() string { return "pengajuan_cuti" }

// ============================================================
// PENGAJUAN DOKUMEN (kelengkapan berkas cuti, per pengajuan)
// ============================================================

type PengajuanDokumen struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	IDPengajuan uint      `json:"id_pengajuan" gorm:"column:id_pengajuan;not null;index"`
	Jenis       string    `json:"jenis" gorm:"column:jenis;size:50;not null"`
	Label       string    `json:"label" gorm:"column:label;size:150"`
	NamaFile    string    `json:"nama_file" gorm:"column:nama_file;size:255"`
	File        []byte    `json:"-" gorm:"column:file;type:bytea"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`

	// PerluPerbaikan ditandai oleh atasan/admin saat mengembalikan pengajuan
	// (kembalikanPengajuan) untuk menunjukkan berkas ini yang tidak sesuai dan
	// harus diupload ulang oleh pegawai. Direset ke false begitu pegawai
	// mengupload ulang berkas penggantinya (lihat updatePengajuan).
	PerluPerbaikan bool `json:"perlu_perbaikan" gorm:"column:perlu_perbaikan;default:false"`
}

// ============================================================
// PENGATURAN SURAT (data penandatangan Kepala Dinas untuk formulir cetak)
// ============================================================

// PengaturanSurat menyimpan data Kepala Dinas yang dicetak sebagai
// penandatangan pada "Surat Rekomendasi Izin Cuti" dan "Formulir Permintaan
// dan Pemberian Cuti" begitu pengajuan disetujui. Selalu ada tepat satu baris
// (ID = 1) -- lihat database.EnsurePengaturanSurat -- dan hanya diubah lewat
// halaman Pengaturan Formulir (admin/administrator), bukan lewat CRUD daftar.
type PengaturanSurat struct {
	ID              uint   `json:"id" gorm:"primaryKey"`
	NamaKepalaDinas string `json:"nama_kepala_dinas" gorm:"column:nama_kepala_dinas;size:150"`
	NipKepalaDinas  string `json:"nip_kepala_dinas" gorm:"column:nip_kepala_dinas;size:30"`
}

func (PengaturanSurat) TableName() string { return "pengaturan_surat" }

func (PengajuanDokumen) TableName() string { return "pengajuan_dokumen" }

// ============================================================
// PERUBAHAN DATA PEGAWAI (self-service edit request + approval workflow)
// ============================================================

// PerubahanDataPegawai menyimpan pengajuan perubahan data diri yang dibuat
// sendiri oleh pegawai (lewat halaman "Profil Saya"). Data baru yang
// diajukan TIDAK langsung mengubah tabel pegawai -- baru diterapkan
// (ditulis ke tabel pegawai, dan disinkronkan ke akun user yang terhubung)
// begitu administrator/admin menyetujuinya. DataLama & DataBaru disimpan
// sebagai JSON text (lihat handlers/perubahan_data.go, struct
// pegawaiEditableData) supaya halaman review admin bisa menampilkan
// perbandingan sebelum/sesudah.
type PerubahanDataPegawai struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	IDPegawai      uint       `json:"id_pegawai" gorm:"column:id_pegawai;not null"`
	Pegawai        *Pegawai   `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	DataLama       string     `json:"data_lama" gorm:"column:data_lama;type:text"`
	DataBaru       string     `json:"data_baru" gorm:"column:data_baru;type:text"`
	SkNamaFile     string     `json:"sk_nama_file" gorm:"column:sk_nama_file;size:255"`
	SkFile         []byte     `json:"-" gorm:"column:sk_file;type:bytea"`
	Status         string     `json:"status" gorm:"size:20;default:pending"`
	CatatanAdmin   string     `json:"catatan_admin" gorm:"column:catatan_admin;size:255"`
	DiputuskanOleh string     `json:"diputuskan_oleh" gorm:"column:diputuskan_oleh;size:150"`
	TglKeputusan   *time.Time `json:"tgl_keputusan" gorm:"column:tgl_keputusan"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

func (PerubahanDataPegawai) TableName() string { return "perubahan_data_pegawai" }

// ============================================================
// ABSENSI (daily attendance: camera + blink-liveness check-in/out)
// ============================================================

// Absensi menyimpan satu baris absen per pegawai per tanggal -- diisi lewat
// menu "Absen" pegawai (kamera + verifikasi kedipan mata), bukan lewat CRUD
// admin. JamMasuk/JamPulang diisi terpisah (dua aksi berbeda: absen masuk di
// pagi hari, absen pulang di sore hari), sehingga keduanya nullable -- baris
// baru dibuat begitu pegawai absen masuk, lalu diperbarui (bukan dibuat lagi)
// saat pegawai absen pulang hari yang sama. Unique index (id_pegawai, tanggal)
// mencegah lebih dari satu baris absen per pegawai per hari.
type Absensi struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	IDPegawai uint      `json:"id_pegawai" gorm:"column:id_pegawai;not null;uniqueIndex:idx_absensi_pegawai_tgl"`
	Pegawai   *Pegawai  `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	Tanggal   time.Time `json:"tanggal" gorm:"column:tanggal;type:date;not null;uniqueIndex:idx_absensi_pegawai_tgl"`

	JamMasuk       *time.Time `json:"jam_masuk" gorm:"column:jam_masuk"`
	TerlambatMenit int        `json:"terlambat_menit" gorm:"column:terlambat_menit;default:0"`
	// FotoMasuk/FotoPulang disimpan sebagai bytea (bukan file di disk) supaya
	// tidak hilang saat container backend di-redeploy/restart, mengikuti pola
	// dokumen lain (lihat Pegawai.SkTerakhirFile, PengajuanDokumen.File).
	FotoMasuk []byte   `json:"-" gorm:"column:foto_masuk;type:bytea"`
	LatMasuk  *float64 `json:"lat_masuk" gorm:"column:lat_masuk"`
	LngMasuk  *float64 `json:"lng_masuk" gorm:"column:lng_masuk"`
	// KedipanMasukOk mencatat hasil verifikasi kedipan mata dari kamera pada
	// SAAT capture (dikirim oleh frontend) -- dipakai untuk menampilkan
	// peringatan "gambar tidak menunjukkan kedipan mata" pada riwayat absen,
	// bukan untuk menolak absennya (kamera/pencahayaan pegawai bisa saja gagal
	// mendeteksi kedipan walau pegawainya asli hadir).
	KedipanMasukOk bool `json:"kedipan_masuk_ok" gorm:"column:kedipan_masuk_ok;default:true"`

	JamPulang       *time.Time `json:"jam_pulang" gorm:"column:jam_pulang"`
	FotoPulang      []byte     `json:"-" gorm:"column:foto_pulang;type:bytea"`
	LatPulang       *float64   `json:"lat_pulang" gorm:"column:lat_pulang"`
	LngPulang       *float64   `json:"lng_pulang" gorm:"column:lng_pulang"`
	KedipanPulangOk bool       `json:"kedipan_pulang_ok" gorm:"column:kedipan_pulang_ok;default:true"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (Absensi) TableName() string { return "absensi" }

// AbsensiJenisDokumen adalah jenis-jenis surat pendukung yang diinput
// administrator/admin (lihat handlers/absensi_dokumen.go) untuk menutupi
// tanggal absen yang terlewat (hari kerja tanpa baris Absensi sama sekali).
const (
	AbsensiDokumenSKS         = "sks"
	AbsensiDokumenSuratTugas  = "surat_tugas"
	AbsensiDokumenBeritaAcara = "berita_acara"
	AbsensiDokumenSuratIzin   = "surat_izin"
)

// AbsensiDokumenKode memetakan jenis dokumen ke kode singkat yang tampil di
// riwayat/rekap absen: Surat Tugas & Berita Acara sama-sama dibaca sebagai
// "DD" (Dinas Dalam) karena keduanya adalah bukti dukung pegawai sedang
// bertugas di luar kantor/dinas, hanya beda bentuk dokumennya; Surat Izin
// dibaca "I" (Izin) dan SKS dibaca "S" (Sakit).
var AbsensiDokumenKode = map[string]string{
	AbsensiDokumenSKS:         "S",
	AbsensiDokumenSuratTugas:  "DD",
	AbsensiDokumenBeritaAcara: "DD",
	AbsensiDokumenSuratIzin:   "I",
}

// AbsensiDokumenKodeLabel memetakan kode singkat ke label lengkapnya.
var AbsensiDokumenKodeLabel = map[string]string{
	"DD": "Dinas Dalam",
	"I":  "Izin",
	"S":  "Sakit",
}

// AbsensiDokumen menyimpan surat yang diupload pegawai untuk tanggal absen
// yang terlewat (SKS/Surat Tugas/Berita Acara/Surat Izin) -- mengikuti pola
// file-di-database yang sama dengan PengajuanDokumen.
type AbsensiDokumen struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	IDPegawai  uint      `json:"id_pegawai" gorm:"column:id_pegawai;not null;index"`
	Pegawai    *Pegawai  `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	Tanggal    time.Time `json:"tanggal" gorm:"column:tanggal;type:date;not null;index"`
	Jenis      string    `json:"jenis" gorm:"column:jenis;size:30;not null"`
	Label      string    `json:"label" gorm:"column:label;size:150"`
	NamaFile   string    `json:"nama_file" gorm:"column:nama_file;size:255"`
	File       []byte    `json:"-" gorm:"column:file;type:bytea"`
	Keterangan string    `json:"keterangan" gorm:"column:keterangan;size:255"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (AbsensiDokumen) TableName() string { return "absensi_dokumen" }

// PengaturanAbsensi menyimpan pengaturan menu Absen -- selalu ada tepat satu
// baris (ID = 1), mengikuti pola PengaturanSurat. Jam disimpan sebagai teks
// "HH:MM" (bukan time.Time) karena hanya dipakai sebagai jam patokan harian,
// bukan tanggal tertentu.
//
//   - Aktif: administrator bisa menonaktifkan seluruh menu Absen (mis. kalau
//     sudah tidak dipakai) tanpa menghapus data riwayat yang sudah ada.
//   - JamMulaiPagi..JamBatasPagi..JamTutupPagi: tiga jam absen masuk.
//     Sebelum JamMulaiPagi absen masuk ditolak (belum waktunya). Antara
//     JamMulaiPagi s.d JamBatasPagi dianggap TEPAT WAKTU. Antara JamBatasPagi
//     s.d JamTutupPagi masih diterima tapi dihitung TERLAMBAT sejumlah menit
//     dari JamBatasPagi. Setelah JamTutupPagi absen masuk otomatis DITUTUP --
//     ditolak sama sekali, tidak ada lagi absen masuk untuk hari itu (lihat
//     absenMasuk di handlers/absensi.go). Ini juga berarti absen pulang tidak
//     akan tersedia untuk hari itu (lihat absenPulang -- absen pulang
//     mensyaratkan sudah ada absen masuk).
//   - JamMulaiPulang..JamTutupPulang: absen pulang baru dibuka (tombolnya
//     aktif) mulai JamMulaiPulang -- sebelum itu pegawai belum bisa absen
//     pulang. Setelah JamTutupPulang absen pulang otomatis DITUTUP -- ditolak
//     sama sekali walaupun pegawai sudah absen masuk dan belum sempat absen
//     pulang hari itu (lihat absenPulang di handlers/absensi.go).
//   - TempatTugasAllowed/JabatanAllowedIDs: filter siapa yang boleh memakai
//     menu Absen, disimpan sebagai teks JSON ("[\"Kantor Pusat\"]" / "[3,5]")
//     mengikuti pola DataLama/DataBaru di PerubahanDataPegawai -- diparsing
//     lewat absensiAllowedTempatTugas/absensiAllowedJabatanIDs di
//     handlers/absensi.go. Daftar KOSONG pada salah satu berarti filter itu
//     tidak diberlakukan (semua tempat tugas/jabatan lolos filter itu); kalau
//     KEDUA daftar kosong maka menu Absen terbuka untuk semua pegawai (default
//     sebelum administrator mengatur apa pun) -- lihat absensiEligible.
//   - KantorLat/KantorLng/RadiusMeter: satu titik koordinat kantor (berlaku
//     untuk seluruh pegawai, bukan per tempat tugas) dipakai untuk membatasi
//     absen hanya boleh dilakukan dalam radius tersebut dari kantor -- lihat
//     distanceMeters di handlers/absensi.go. KantorLat/KantorLng NULL berarti
//     geofence belum diatur (default sebelum administrator mengisi) sehingga
//     absen tetap boleh dilakukan tanpa validasi jarak, mengikuti pola
//     default-terbuka yang sama seperti filter tempat tugas/jabatan di atas.
type PengaturanAbsensi struct {
	ID                 uint     `json:"id" gorm:"primaryKey"`
	Aktif              bool     `json:"aktif" gorm:"column:aktif;default:true"`
	JamMulaiPagi       string   `json:"jam_mulai_pagi" gorm:"column:jam_mulai_pagi;size:5;default:'06:00'"`
	JamBatasPagi       string   `json:"jam_batas_pagi" gorm:"column:jam_batas_pagi;size:5;default:'07:30'"`
	JamTutupPagi       string   `json:"jam_tutup_pagi" gorm:"column:jam_tutup_pagi;size:5;default:'09:00'"`
	JamMulaiPulang     string   `json:"jam_mulai_pulang" gorm:"column:jam_mulai_pulang;size:5;default:'15:00'"`
	JamTutupPulang     string   `json:"jam_tutup_pulang" gorm:"column:jam_tutup_pulang;size:5;default:'20:00'"`
	TempatTugasAllowed string   `json:"-" gorm:"column:tempat_tugas_allowed;type:text"`
	JabatanAllowedIDs  string   `json:"-" gorm:"column:jabatan_allowed_ids;type:text"`
	KantorLat          *float64 `json:"-" gorm:"column:kantor_lat"`
	KantorLng          *float64 `json:"-" gorm:"column:kantor_lng"`
	RadiusMeter        int      `json:"-" gorm:"column:radius_meter;default:20"`
}

func (PengaturanAbsensi) TableName() string { return "pengaturan_absensi" }
