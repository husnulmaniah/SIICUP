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
	NomorSurat string    `json:"nomor_surat" gorm:"column:nomor_surat;size:100"`
	CreatedAt  time.Time `json:"created_at"`

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
