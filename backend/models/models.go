package models

import (
	"encoding/json"
	"time"
)

// ============================================================
// MASTER DATA TABLES (simple lookup tables)
// ============================================================

type Role struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Role string `json:"role" gorm:"size:50;not null;unique"`
}

func (Role) TableName() string { return "role" }

// JenisJabatan* mengelompokkan jabatan untuk menentukan usia pensiun
// otomatis (lihat PengaturanPensiun & usiaPensiunPegawai di
// handlers/pengajuan_pensiun.go): Pelaksana & Struktural pensiun di usia
// yang sama (default 58 tahun), Fungsional pensiun di usia lebih tua
// (default 60 tahun, sesuai jabatan fungsional tertentu seperti guru/dosen).
const (
	JenisJabatanPelaksana  = "pelaksana"
	JenisJabatanStruktural = "struktural"
	JenisJabatanFungsional = "fungsional"
)

type Jabatan struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	Jabatan string `json:"jabatan" gorm:"size:100;not null"`
	// JenisJabatan: salah satu dari JenisJabatanPelaksana/Struktural/
	// Fungsional di atas -- dipakai untuk menghitung usia pensiun otomatis
	// pegawai dengan jabatan ini. Default "pelaksana" untuk data lama yang
	// belum diisi administrator.
	JenisJabatan string `json:"jenis_jabatan" gorm:"column:jenis_jabatan;size:20;not null;default:'pelaksana'"`
}

func (Jabatan) TableName() string { return "jabatan" }

// Kecamatan: daftar kecamatan tempat unit kerja/sekolah berada -- dipakai
// mengelompokkan UnitKerja (lihat di bawah) supaya administrator bisa
// mengelola & memilih titik koordinat absen per kecamatan/sekolah, alih-alih
// hanya satu titik kantor tunggal (PengaturanAbsensi.KantorLat/KantorLng).
// Lihat juga PengaturanAbsensi.KecamatanAllowedIDs untuk memilih kecamatan
// mana saja yang menu Absen-nya diaktifkan.
type Kecamatan struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Nama string `json:"nama" gorm:"size:150;not null"`
}

func (Kecamatan) TableName() string { return "kecamatan" }

// TempatKerjaDinas/TempatKerjaSekolah: nilai kolom UnitKerja.TempatKerja --
// kategori EKSPLISIT dinas/kantor vs sekolah untuk unit kerja tersebut,
// diisi administrator lewat menu Master Data -> Unit Kerja (lihat komentar
// pada field TempatKerja di bawah). Dipakai sebagai PRIORITAS UTAMA oleh
// isSekolahPegawai (handlers/pengajuan_cuti.go) untuk menentukan jam kerja
// absen, 5/6 hari kerja, & syarat dokumen cuti pegawai -- menggantikan
// tebakan otomatis dari kata "sekolah" pada Tempat Tugas pegawai
// (isSekolahFromTempatTgs) untuk unit kerja yang sudah diberi kategori ini.
const (
	TempatKerjaDinas   = "dinas"
	TempatKerjaSekolah = "sekolah"
)

// UnitKerja merepresentasikan unit kerja/sekolah tempat pegawai bertugas.
//   - IDKecamatan/Kecamatan: kecamatan tempat unit kerja/sekolah ini berada
//     (opsional -- nullable untuk data lama yang belum diisi administrator).
//   - TempatKerja: kategori EKSPLISIT "dinas" (TempatKerjaDinas) atau
//     "sekolah" (TempatKerjaSekolah) untuk unit kerja/sekolah ini (opsional,
//     nullable). Kalau diisi, inilah yang dipakai (PRIORITAS UTAMA, lebih
//     diutamakan daripada menebak dari kata "sekolah" pada Tempat Tugas
//     pegawai) untuk menentukan: jam kerja absen (Dinas/Kantor vs Sekolah,
//     lihat jamAbsenUntukPegawai di handlers/absensi.go), 5/6 hari kerja
//     (sixDayWeekForPegawai), dan syarat dokumen cuti/Surat Rekomendasi
//     Kepala Sekolah (dokumenRequirementsForJenis) -- lihat isSekolahPegawai
//     di handlers/pengajuan_cuti.go yang memilih ini kalau sudah diisi, atau
//     jatuh kembali (fallback) ke tebakan dari Tempat Tugas pegawai kalau
//     unit kerja ini belum diberi kategori (kompatibel dengan data lama).
//   - Lat/Lng/RadiusMeter: titik koordinat & radius absen KHUSUS untuk unit
//     kerja/sekolah ini. Kalau diisi, absen pegawai yang id_unit_kerja-nya
//     menunjuk ke sini divalidasi terhadap titik ini (lihat absensiCekRadius
//     di handlers/absensi.go), BUKAN titik kantor tunggal di
//     PengaturanAbsensi -- sehingga instansi dengan banyak sekolah di
//     beberapa kecamatan bisa punya titik koordinat sendiri-sendiri per
//     sekolah. RadiusMeter nullable/kosong berarti memakai RadiusMeter
//     global di PengaturanAbsensi. Kalau Lat/Lng unit kerja ini kosong,
//     absen pegawainya jatuh kembali (fallback) ke titik kantor tunggal di
//     PengaturanAbsensi seperti sebelumnya (kompatibel dengan data lama).
//   - JamMulaiPagi/JamBatasPagi/JamTutupPagi/JamMulaiPulang/JamTutupPulang:
//     jendela waktu absen (format "HH:MM", sama seperti field sejenis pada
//     PengaturanAbsensi) KHUSUS untuk unit kerja/sekolah ini. Kalau KELIMA
//     field ini diisi lengkap, jam kerja unit kerja/sekolah ini dipakai
//     sebagai PRIORITAS UTAMA untuk pegawai yang id_unit_kerja-nya menunjuk
//     ke sini (lihat jamAbsenUntukPegawai di handlers/absensi.go) -- artinya
//     tiap sekolah bisa punya jam masuk/terlambat/pulang/tutup sendiri-
//     sendiri, tidak harus disamakan semua. Kalau salah satu saja dari
//     kelima field ini kosong, jam kerja unit kerja ini dianggap BELUM
//     diatur & pegawainya jatuh kembali (fallback) ke set Sekolah/Dinas
//     "global" di PengaturanAbsensi berdasarkan Tempat Tugas pegawai seperti
//     sebelumnya (kompatibel dengan data lama) -- lihat jamAbsenUntukPegawai.
type UnitKerja struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	Unit           string     `json:"unit" gorm:"size:150;not null"`
	IDKecamatan    *uint      `json:"id_kecamatan" gorm:"column:id_kecamatan"`
	Kecamatan      *Kecamatan `json:"kecamatan,omitempty" gorm:"foreignKey:IDKecamatan;references:ID"`
	TempatKerja    *string    `json:"tempat_kerja" gorm:"column:tempat_kerja;size:20"`
	Lat            *float64   `json:"lat" gorm:"column:lat"`
	Lng            *float64   `json:"lng" gorm:"column:lng"`
	RadiusMeter    *int       `json:"radius_meter" gorm:"column:radius_meter"`
	JamMulaiPagi   *string    `json:"jam_mulai_pagi" gorm:"column:jam_mulai_pagi;size:5"`
	JamBatasPagi   *string    `json:"jam_batas_pagi" gorm:"column:jam_batas_pagi;size:5"`
	JamTutupPagi   *string    `json:"jam_tutup_pagi" gorm:"column:jam_tutup_pagi;size:5"`
	JamMulaiPulang *string    `json:"jam_mulai_pulang" gorm:"column:jam_mulai_pulang;size:5"`
	JamTutupPulang *string    `json:"jam_tutup_pulang" gorm:"column:jam_tutup_pulang;size:5"`
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
	// TglLahir: tanggal lahir pegawai -- dipakai untuk menghitung usia &
	// menentukan otomatis apakah pegawai ini sudah/akan mencapai usia
	// pensiun (lihat usiaPensiunPegawai di handlers/pengajuan_pensiun.go,
	// PengaturanPensiun, dan Jabatan.JenisJabatan). Nullable karena data
	// pegawai lama mungkin belum diisi administrator.
	TglLahir *time.Time `json:"tgl_lahir" gorm:"column:tgl_lahir;type:date"`
	// TglKenaikanGajiBerkalaTerakhir / TglKenaikanPangkatTerakhir: tanggal
	// kenaikan gaji berkala & kenaikan pangkat TERAKHIR pegawai ini -- opsional
	// (boleh kosong), dipakai sebagai dasar menghitung kapan kenaikan
	// berikutnya jatuh tempo berdasarkan interval per Jabatan.JenisJabatan
	// (lihat models.PengaturanKenaikanGajiBerkala &
	// handlers/kenaikan_gaji_berkala.go). Bisa diisi langsung oleh
	// administrator lewat menu Data Pegawai, ATAU diajukan pegawai sendiri
	// lewat Profil Saya -> Ajukan Perubahan Data (baru berlaku setelah
	// disetujui, lihat handlers/perubahan_data.go).
	TglKenaikanGajiBerkalaTerakhir *time.Time `json:"tgl_kenaikan_gaji_berkala_terakhir" gorm:"column:tgl_kenaikan_gaji_berkala_terakhir;type:date"`
	TglKenaikanPangkatTerakhir     *time.Time `json:"tgl_kenaikan_pangkat_terakhir" gorm:"column:tgl_kenaikan_pangkat_terakhir;type:date"`
	NoHP                           string     `json:"no_hp" gorm:"column:no_hp;size:20"`
	IDStatus                       *uint      `json:"id_status" gorm:"column:id_status"`
	Status                         *Status    `json:"status,omitempty" gorm:"foreignKey:IDStatus;references:ID"`
	IDAtasan                       *uint      `json:"id_atasan" gorm:"column:id_atasan"`
	Atasan                         *Pegawai   `json:"atasan,omitempty" gorm:"foreignKey:IDAtasan;references:ID"`
	Email                          string     `json:"email" gorm:"size:100"`

	// Dokumen kepegawaian (disimpan langsung di database sebagai bytea agar
	// tidak hilang saat container backend di-redeploy/restart).
	SkTerakhirNama string `json:"sk_terakhir_nama" gorm:"column:sk_terakhir_nama;size:255"`
	SkTerakhirFile []byte `json:"-" gorm:"column:sk_terakhir_file;type:bytea"`
	SkKgbNama      string `json:"sk_kgb_nama" gorm:"column:sk_kgb_nama;size:255"`
	SkKgbFile      []byte `json:"-" gorm:"column:sk_kgb_file;type:bytea"`
	SkPangkatNama  string `json:"sk_pangkat_nama" gorm:"column:sk_pangkat_nama;size:255"`
	SkPangkatFile  []byte `json:"-" gorm:"column:sk_pangkat_file;type:bytea"`
	SkPensiunNama  string `json:"sk_pensiun_nama" gorm:"column:sk_pensiun_nama;size:255"`
	SkPensiunFile  []byte `json:"-" gorm:"column:sk_pensiun_file;type:bytea"`

	// FotoProfil: foto profil pegawai, ditampilkan di avatar topbar &
	// halaman Profil Saya masing-masing pegawai (lihat AppLayout.vue,
	// ProfilSayaView.vue). BERBEDA dari dokumen SK di atas -- pegawai boleh
	// mengganti/menghapus foto profilnya SENDIRI kapan saja lewat endpoint
	// khusus (lihat uploadFotoProfilPegawai di handlers/pegawai.go), TIDAK
	// lewat alur pengajuan Perubahan Data Pegawai yang butuh persetujuan
	// administrator/admin.
	FotoProfilNama string `json:"foto_profil_nama" gorm:"column:foto_profil_nama;size:255"`
	FotoProfilFile []byte `json:"-" gorm:"column:foto_profil_file;type:bytea"`
}

func (Pegawai) TableName() string { return "pegawai" }

// ============================================================
// USER (login account)
// ============================================================

type User struct {
	ID        uint     `json:"id" gorm:"primaryKey"`
	Username  string   `json:"username" gorm:"size:50;not null;unique"`
	Pass      string   `json:"-" gorm:"column:pass;size:255;not null"`
	Nama      string   `json:"nama" gorm:"size:150;not null"`
	IDRole    uint     `json:"id_role" gorm:"column:id_role;not null"`
	Role      *Role    `json:"role,omitempty" gorm:"foreignKey:IDRole;references:ID"`
	IDPegawai *uint    `json:"id_pegawai" gorm:"column:id_pegawai"`
	Pegawai   *Pegawai `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	// IsAdminAbsensi menandai akun (biasanya milik role "pegawai"/"atasan",
	// bisa juga "admin"/"administrator" meski tidak berpengaruh karena
	// mereka sudah punya akses penuh) sebagai tambahan boleh mengelola menu
	// "Input Rekapan Absensi" (rekap kehadiran SELURUH pegawai & input surat
	// kolektif: berita acara, surat tugas, SKS, dll) TANPA mengubah role
	// utamanya -- jadi pegawai yang sama tetap bisa absen & mengajukan cuti
	// sendiri lewat akun yang sama (tidak perlu akun terpisah). Lihat
	// RegisterAbsensiRoutes (absensi.go) & utils.Claims.IsAdminAbsensi.
	IsAdminAbsensi bool `json:"is_admin_absensi" gorm:"column:is_admin_absensi;default:false"`
	// IsAdminVerifikasi menandai akun (role apa pun, sama pola dengan
	// IsAdminAbsensi di atas & independen darinya) sebagai tambahan boleh
	// memverifikasi (menyetujui/mengembalikan) Pengajuan Surat Kolektif yang
	// diajukan sendiri oleh pegawai sekolah (lihat
	// handlers/pengajuan_surat_kolektif.go). Kalau IsAdminAbsensi & ini
	// SAMA-SAMA dicentang pada satu akun, pegawai itu bisa menginput Surat
	// Kolektif dinas (lewat IsAdminAbsensi) SEKALIGUS memverifikasi
	// pengajuan surat kolektif sekolah (lewat ini) -- dua kewenangan yang
	// tetap terpisah, hanya kebetulan dipegang orang yang sama.
	IsAdminVerifikasi bool `json:"is_admin_verifikasi" gorm:"column:is_admin_verifikasi;default:false"`
	// Aktif: false berarti akun ini TERTUTUP -- tidak bisa login (lihat
	// LoginHandler) dan setiap request API dari akun ini langsung ditolak
	// (lihat middleware.RequireActiveUser), walau token JWT-nya masih
	// berlaku. Diset false OTOMATIS begitu pengajuan pensiun pegawai pemilik
	// akun ini disetujui (lihat approvePengajuanPensiun di
	// handlers/pengajuan_pensiun.go), dan dikembalikan ke true otomatis kalau
	// persetujuan itu dibatalkan administrator.
	Aktif     bool      `json:"aktif" gorm:"column:aktif;not null;default:true"`
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
	ID         uint     `json:"id" gorm:"primaryKey"`
	IDPegawai  uint     `json:"id_pegawai" gorm:"column:id_pegawai;not null"`
	Pegawai    *Pegawai `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	DataLama   string   `json:"data_lama" gorm:"column:data_lama;type:text"`
	DataBaru   string   `json:"data_baru" gorm:"column:data_baru;type:text"`
	SkNamaFile string   `json:"sk_nama_file" gorm:"column:sk_nama_file;size:255"`
	SkFile     []byte   `json:"-" gorm:"column:sk_file;type:bytea"`
	// SkKgbNama/SkKgbFile: berkas SK Kenaikan Gaji Berkala yang diupload
	// pegawai bersamaan dengan pengajuan ini -- SEPENUHNYA opsional (berbeda
	// dari SkNamaFile/SkFile di atas yang wajib), jadi boleh kosong. Kalau
	// diisi, disalin jadi dokumen SK Kenaikan Gaji Berkala resmi pegawai
	// begitu pengajuan ini disetujui (lihat approvePerubahanData).
	SkKgbNama string `json:"sk_kgb_nama" gorm:"column:sk_kgb_nama;size:255"`
	SkKgbFile []byte `json:"-" gorm:"column:sk_kgb_file;type:bytea"`
	// SkPangkatNama/SkPangkatFile: berkas SK Kenaikan Pangkat, SEPENUHNYA
	// opsional sama seperti SkKgbNama/SkKgbFile di atas -- lihat
	// approvePerubahanData.
	SkPangkatNama  string     `json:"sk_pangkat_nama" gorm:"column:sk_pangkat_nama;size:255"`
	SkPangkatFile  []byte     `json:"-" gorm:"column:sk_pangkat_file;type:bytea"`
	Status         string     `json:"status" gorm:"size:20;default:pending"`
	CatatanAdmin   string     `json:"catatan_admin" gorm:"column:catatan_admin;size:255"`
	DiputuskanOleh string     `json:"diputuskan_oleh" gorm:"column:diputuskan_oleh;size:150"`
	TglKeputusan   *time.Time `json:"tgl_keputusan" gorm:"column:tgl_keputusan"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

func (PerubahanDataPegawai) TableName() string { return "perubahan_data_pegawai" }

// ============================================================
// PENGAJUAN PENSIUN (retirement request + approval workflow)
// ============================================================

// PengaturanPensiun adalah baris tunggal (id=1, sama seperti
// PengaturanAbsensi) berisi usia pensiun standar yang bisa diubah
// administrator (lihat handlers/pengajuan_pensiun.go) -- dipakai untuk
// menghitung otomatis apakah seorang pegawai sudah/akan mencapai usia
// pensiun berdasarkan Jabatan.JenisJabatan-nya.
type PengaturanPensiun struct {
	ID                      uint `json:"id" gorm:"primaryKey"`
	UsiaPelaksanaStruktural int  `json:"usia_pelaksana_struktural" gorm:"column:usia_pelaksana_struktural;not null;default:58"`
	UsiaFungsional          int  `json:"usia_fungsional" gorm:"column:usia_fungsional;not null;default:60"`
}

func (PengaturanPensiun) TableName() string { return "pengaturan_pensiun" }

// ============================================================
// PENGATURAN KENAIKAN GAJI BERKALA & KENAIKAN PANGKAT
// ============================================================

// PengaturanKenaikanGajiBerkala adalah baris tunggal (id=1, sama seperti
// PengaturanPensiun) berisi interval (dalam tahun) kenaikan gaji berkala &
// kenaikan pangkat standar, masing-masing dibedakan per
// Jabatan.JenisJabatan: Fungsional vs Pelaksana/Struktural -- lihat
// handlers/kenaikan_gaji_berkala.go. Hanya bisa diubah administrator/admin
// lewat menu Perubahan Data Pegawai -> tab Pengaturan Kenaikan Gaji
// Berkala. Dipakai murni untuk menghitung & menampilkan kapan kenaikan
// berikutnya jatuh tempo (berdasarkan Pegawai.TglKenaikanGajiBerkalaTerakhir
// / TglKenaikanPangkatTerakhir) -- bukan alur pengajuan/persetujuan
// tersendiri seperti PengajuanPensiun.
type PengaturanKenaikanGajiBerkala struct {
	ID                                  uint `json:"id" gorm:"primaryKey"`
	GajiBerkalaFungsionalTahun          int  `json:"gaji_berkala_fungsional_tahun" gorm:"column:gaji_berkala_fungsional_tahun;not null;default:1"`
	GajiBerkalaPelaksanaStrukturalTahun int  `json:"gaji_berkala_pelaksana_struktural_tahun" gorm:"column:gaji_berkala_pelaksana_struktural_tahun;not null;default:2"`
	PangkatFungsionalTahun              int  `json:"pangkat_fungsional_tahun" gorm:"column:pangkat_fungsional_tahun;not null;default:2"`
	PangkatPelaksanaStrukturalTahun     int  `json:"pangkat_pelaksana_struktural_tahun" gorm:"column:pangkat_pelaksana_struktural_tahun;not null;default:4"`
}

func (PengaturanKenaikanGajiBerkala) TableName() string { return "pengaturan_kenaikan_gaji_berkala" }

// PengajuanPensiun: pegawai mengajukan pensiun sendiri lewat halaman Profil
// Saya (mengupload SK/usulan pensiun), administrator/admin lalu
// menyetujui/menolaknya di menu Pengajuan Pensiun (lihat
// handlers/pengajuan_pensiun.go). BERBEDA dari upload SK Pensiun langsung
// oleh administrator di Data Pegawai (lihat uploadDokumenPegawai di
// handlers/pegawai.go, yang efeknya instan tanpa alur persetujuan karena
// administrator sendiri yang melakukannya) -- jalur ini WAJIB lewat
// persetujuan karena diajukan pegawai sendiri.
//
// Begitu disetujui: status kepegawaian pegawai diubah jadi "Pensiun", akun
// login pegawai (models.User.Aktif) dinonaktifkan, dan SK yang diupload di
// pengajuan ini disalin jadi dokumen SK Pensiun resmi pegawai (mengikuti
// pola approvePerubahanData). IDStatusSebelum menyimpan id_status pegawai
// SEBELUM disetujui, supaya kalau administrator membatalkan persetujuan ini
// (lihat batalkanPersetujuanPensiun), status & akunnya bisa dikembalikan
// seperti semula.
type PengajuanPensiun struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	IDPegawai       uint       `json:"id_pegawai" gorm:"column:id_pegawai;not null"`
	Pegawai         *Pegawai   `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	IsPensiunDini   bool       `json:"is_pensiun_dini" gorm:"column:is_pensiun_dini;default:false"`
	Alasan          string     `json:"alasan" gorm:"column:alasan;size:255"`
	SkNamaFile      string     `json:"sk_nama_file" gorm:"column:sk_nama_file;size:255"`
	SkFile          []byte     `json:"-" gorm:"column:sk_file;type:bytea"`
	Status          string     `json:"status" gorm:"size:20;default:pending"`
	IDStatusSebelum *uint      `json:"id_status_sebelum" gorm:"column:id_status_sebelum"`
	CatatanAdmin    string     `json:"catatan_admin" gorm:"column:catatan_admin;size:255"`
	DiputuskanOleh  string     `json:"diputuskan_oleh" gorm:"column:diputuskan_oleh;size:150"`
	TglKeputusan    *time.Time `json:"tgl_keputusan" gorm:"column:tgl_keputusan"`
	CreatedAt       time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

func (PengajuanPensiun) TableName() string { return "pengajuan_pensiun" }

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
	// DinasDalamMasuk: pegawai mencentang tombol "Dinas Dalam" saat mengambil
	// foto absen masuk -- kalau true, validasi radius kantor (absensiCekRadius)
	// DILEWATI untuk absen masuk hari itu (boleh absen dari mana saja), dan
	// rekap/riwayat/export menampilkan status "Dinas Dalam" untuk hari itu,
	// BUKAN "Hadir". Ini terpisah dari sistem dokumen Surat Tugas/Berita Acara
	// (AbsensiDokumen, kode "DD") yang diinput administrator -- ini laporan
	// mandiri pegawai pada saat absen, bukan dokumen yang diinput admin.
	DinasDalamMasuk bool `json:"dinas_dalam_masuk" gorm:"column:dinas_dalam_masuk;default:false"`

	JamPulang       *time.Time `json:"jam_pulang" gorm:"column:jam_pulang"`
	FotoPulang      []byte     `json:"-" gorm:"column:foto_pulang;type:bytea"`
	LatPulang       *float64   `json:"lat_pulang" gorm:"column:lat_pulang"`
	LngPulang       *float64   `json:"lng_pulang" gorm:"column:lng_pulang"`
	KedipanPulangOk bool       `json:"kedipan_pulang_ok" gorm:"column:kedipan_pulang_ok;default:true"`
	// DinasDalamPulang: sama seperti DinasDalamMasuk, tapi untuk absen pulang
	// (pegawai bisa mencentang salah satu/kedua-duanya secara independen,
	// misalnya dinas dalam saat pulang walau absen masuk normal di kantor).
	DinasDalamPulang bool `json:"dinas_dalam_pulang" gorm:"column:dinas_dalam_pulang;default:false"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// IsDinasDalam mengembalikan true kalau salah satu (atau kedua) absen masuk/
// pulang hari itu ditandai Dinas Dalam oleh pegawai -- dipakai di rekap/
// export untuk menampilkan status "Dinas Dalam" alih-alih "Hadir".
func (a Absensi) IsDinasDalam() bool {
	return a.DinasDalamMasuk || a.DinasDalamPulang
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

// JenisSurat adalah master data jenis surat kolektif yang bisa dipilih saat
// admin/administrator menginput Surat Kolektif (lihat
// handlers/absensi_dokumen.go) -- dikelola lewat menu Master Data -> Jenis
// Surat (CRUD generik, administrator only). Slug dipakai sebagai nilai yang
// tersimpan di AbsensiDokumen.Jenis (harus unik, dipakai juga sebagai "value"
// dropdown di frontend), Nama adalah label yang tampil di dropdown & rekap,
// Kode adalah kode singkat BEBAS yang diketik sendiri oleh administrator
// (tidak wajib salah satu dari DD/I/S -- lihat handlers/labelUntukJenis
// untuk bagaimana label tampilannya dihitung tanpa bergantung pada tiga kode
// bawaan itu). Kode "DD"/"I"/"S" tetap dikenali khusus di rekap/PDF absen
// untuk dihitung ke kolom Jumlah DD/Izin/Sakit (lihat handlers/absensi_admin.go
// & handlers/absensi_pdf.go); kode lain di luar itu tetap menutup tanggal
// terlewat pegawai (tidak dianggap tidak hadir) tapi tidak ikut masuk ke tiga
// kolom hitungan tersebut. 4 jenis bawaan (Surat Tugas, Berita Acara, Surat
// Izin, SKS) di-seed otomatis saat migrasi kalau tabel masih kosong --
// slug-nya SENGAJA disamakan dengan konstanta AbsensiDokumen* di atas supaya
// data lama tetap valid.
type JenisSurat struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Slug string `json:"slug" gorm:"column:slug;size:40;not null;uniqueIndex"`
	Nama string `json:"nama" gorm:"column:nama;size:150;not null"`
	Kode string `json:"kode" gorm:"column:kode;size:10;not null"`
}

func (JenisSurat) TableName() string { return "jenis_surat" }

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

// PengajuanSuratKolektifStatus adalah 3 status pengajuan surat kolektif
// mandiri (lihat handlers/pengajuan_surat_kolektif.go) -- mengikuti pola
// yang sama seperti PengajuanCuti: Menunggu -> Disetujui (baris AbsensiDokumen
// otomatis dibuat untuk tiap tanggal) atau Menunggu -> Dikembalikan (pegawai
// bisa mengedit & mengajukan ulang, balik jadi Menunggu lagi).
const (
	PengajuanSuratKolektifMenunggu     = "menunggu"
	PengajuanSuratKolektifDisetujui    = "disetujui"
	PengajuanSuratKolektifDikembalikan = "dikembalikan"
)

// PengajuanSuratKolektif menyimpan pengajuan surat kolektif MANDIRI oleh
// pegawai bertugas di SEKOLAH (pegawai dinas/kantor TIDAK boleh mengajukan
// sendiri lewat sini -- lihat pembatasan isSekolahPegawai di
// buatPengajuanSuratKolektif) untuk menutup beberapa tanggal absen yang
// terlewat sekaligus, dengan 1 jenis surat & 1 berkas untuk semua tanggal
// yang dipilih -- meniru bentuk inputAbsensiDokumenKolektif (menu Rekap
// Absen) tapi diajukan sendiri oleh pegawai & butuh persetujuan administrator
// atau akun IsAdminVerifikasi sebelum baris AbsensiDokumen sungguhan dibuat.
//
//   - TanggalListRaw: daftar tanggal (format "YYYY-MM-DD") yang diajukan,
//     disimpan sebagai teks JSON (mis. ["2026-09-01","2026-09-02"]) --
//     mengikuti pola TempatTugasAllowed dkk pada PengaturanAbsensi. Diparsing
//     lewat TanggalList()/diisi lewat SetTanggalList() di bawah; endpoint API
//     mengekspos daftar tanggal ini lewat DTO terpisah (bukan field ini
//     langsung, makanya json:"-").
//   - Status: menunggu/disetujui/dikembalikan (lihat konstanta di atas).
//   - IDVerifikator/Verifikator/DiverifikasiAt: siapa & kapan pengajuan ini
//     disetujui/dikembalikan (administrator atau akun IsAdminVerifikasi).
//   - CatatanVerifikasi: wajib diisi verifikator saat mengembalikan (supaya
//     pegawai tahu apa yang perlu diperbaiki), opsional saat menyetujui.
type PengajuanSuratKolektif struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	IDPegawai         uint       `json:"id_pegawai" gorm:"column:id_pegawai;not null;index"`
	Pegawai           *Pegawai   `json:"pegawai,omitempty" gorm:"foreignKey:IDPegawai;references:ID"`
	TanggalListRaw    string     `json:"-" gorm:"column:tanggal_list;type:text;not null"`
	Jenis             string     `json:"jenis" gorm:"column:jenis;size:40;not null"`
	Label             string     `json:"label" gorm:"column:label;size:150"`
	NamaFile          string     `json:"nama_file" gorm:"column:nama_file;size:255"`
	File              []byte     `json:"-" gorm:"column:file;type:bytea"`
	Keterangan        string     `json:"keterangan" gorm:"column:keterangan;size:255"`
	Status            string     `json:"status" gorm:"column:status;size:20;not null;default:'menunggu';index"`
	CatatanVerifikasi string     `json:"catatan_verifikasi" gorm:"column:catatan_verifikasi;size:255"`
	IDVerifikator     *uint      `json:"id_verifikator" gorm:"column:id_verifikator"`
	Verifikator       *User      `json:"verifikator,omitempty" gorm:"foreignKey:IDVerifikator;references:ID"`
	DiverifikasiAt    *time.Time `json:"diverifikasi_at" gorm:"column:diverifikasi_at"`
	CreatedAt         time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (PengajuanSuratKolektif) TableName() string { return "pengajuan_surat_kolektif" }

// TanggalList mengurai TanggalListRaw (teks JSON) jadi slice string
// "YYYY-MM-DD". Dipakai handler untuk validasi & ditampilkan ke frontend
// lewat DTO, dan untuk membuat baris AbsensiDokumen saat disetujui.
func (p PengajuanSuratKolektif) TanggalList() []string {
	var out []string
	if p.TanggalListRaw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(p.TanggalListRaw), &out)
	return out
}

// SetTanggalList menyimpan slice tanggal "YYYY-MM-DD" ke TanggalListRaw
// sebagai teks JSON.
func (p *PengajuanSuratKolektif) SetTanggalList(tanggal []string) {
	b, _ := json.Marshal(tanggal)
	p.TanggalListRaw = string(b)
}

// PengaturanAbsensi menyimpan pengaturan menu Absen -- selalu ada tepat satu
// baris (ID = 1), mengikuti pola PengaturanSurat. Jam disimpan sebagai teks
// "HH:MM" (bukan time.Time) karena hanya dipakai sebagai jam patokan harian,
// bukan tanggal tertentu.
//
//   - Aktif: administrator bisa menonaktifkan seluruh menu Absen (mis. kalau
//     sudah tidak dipakai) tanpa menghapus data riwayat yang sudah ada.
//   - JamMulaiPagi..JamBatasPagi..JamTutupPagi: tiga jam absen masuk UNTUK
//     PEGAWAI DINAS/KANTOR (bukan sekolah -- lihat JamMulaiPagiSekolah dst di
//     bawah untuk jam absen pegawai sekolah, yang jam kerjanya memang beda).
//     Sebelum JamMulaiPagi absen masuk ditolak (belum waktunya). Antara
//     JamMulaiPagi s.d JamBatasPagi dianggap TEPAT WAKTU. Antara JamBatasPagi
//     s.d JamTutupPagi masih diterima tapi dihitung TERLAMBAT sejumlah menit
//     dari JamBatasPagi. Setelah JamTutupPagi absen masuk otomatis DITUTUP --
//     ditolak sama sekali, tidak ada lagi absen masuk untuk hari itu (lihat
//     absenMasuk di handlers/absensi.go). Ini juga berarti absen pulang tidak
//     akan tersedia untuk hari itu (lihat absenPulang -- absen pulang
//     mensyaratkan sudah ada absen masuk).
//   - JamMulaiPulang..JamTutupPulang: absen pulang (DINAS/KANTOR) baru
//     dibuka (tombolnya aktif) mulai JamMulaiPulang -- sebelum itu pegawai
//     belum bisa absen pulang. Setelah JamTutupPulang absen pulang otomatis
//     DITUTUP -- ditolak sama sekali walaupun pegawai sudah absen masuk dan
//     belum sempat absen pulang hari itu (lihat absenPulang di
//     handlers/absensi.go).
//   - JamMulaiPagiSekolah..JamTutupPulangSekolah: SET KEDUA, jam kerja penuh
//     (mulai pagi s.d tutup pulang) khusus pegawai berstatus SEKOLAH (lihat
//     isSekolahPegawai di handlers/pengajuan_cuti.go -- kategori Tempat
//     Kerja pada Unit Kerja pegawai kalau sudah diisi, atau fallback tebakan
//     dari kata "sekolah" pada Tempat Tugas untuk data lama; dipakai juga
//     untuk menentukan 5/6 hari kerja & syarat dokumen cuti). Artinya &
//     urutannya identik dengan set Dinas/Kantor di atas, hanya berlaku untuk
//     pegawai sekolah -- lihat jamAbsenUntukPegawai di handlers/absensi.go
//     yang memilih set mana dipakai per pegawai secara otomatis. Ini
//     dipakai sebagai FALLBACK kalau unit kerja/sekolah pegawai belum diberi
//     jam kerja sendiri -- lihat UnitKerja.JamMulaiPagi dkk & fungsi
//     jamAbsenUntukPegawai (yang memilih antara jam khusus unit kerja ini
//     atau jatuh kembali ke set Sekolah/Dinas di sini).
//   - TempatTugasAllowed/JabatanAllowedIDs/KecamatanAllowedIDs: filter siapa
//     yang boleh memakai menu Absen, disimpan sebagai teks JSON
//     ("[\"Kantor Pusat\"]" / "[3,5]") mengikuti pola DataLama/DataBaru di
//     PerubahanDataPegawai -- diparsing lewat
//     absensiAllowedTempatTugas/absensiAllowedJabatanIDs/
//     absensiAllowedKecamatanIDs di handlers/absensi.go. KecamatanAllowedIDs
//     dicocokkan lewat UnitKerja.IDKecamatan milik unit kerja/sekolah pegawai
//     (pegawai tanpa unit kerja, atau unit kerja tanpa kecamatan, otomatis
//     TIDAK lolos filter ini begitu filter ini diisi). Daftar KOSONG pada
//     salah satu filter berarti filter itu tidak diberlakukan (semua
//     tempat tugas/jabatan/kecamatan lolos filter itu); kalau SEMUA daftar
//     kosong maka menu Absen terbuka untuk semua pegawai (default sebelum
//     administrator mengatur apa pun) -- lihat absensiEligible.
//   - KantorLat/KantorLng/RadiusMeter: titik koordinat & radius absen
//     DEFAULT/FALLBACK yang berlaku untuk pegawai yang unit kerjanya belum
//     /tidak diberi titik koordinat sendiri (lihat UnitKerja.Lat/Lng/
//     RadiusMeter, yang kalau diisi jadi prioritas utama per sekolah) --
//     lihat distanceMeters & absensiCekRadius di handlers/absensi.go.
//     KantorLat/KantorLng NULL DAN unit kerja pegawai juga tidak punya
//     titik koordinat berarti geofence belum diatur sehingga absen tetap
//     boleh dilakukan tanpa validasi jarak, mengikuti pola default-terbuka
//     yang sama seperti filter tempat tugas/jabatan/kecamatan di atas.
type PengaturanAbsensi struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	Aktif          bool   `json:"aktif" gorm:"column:aktif;default:true"`
	JamMulaiPagi   string `json:"jam_mulai_pagi" gorm:"column:jam_mulai_pagi;size:5;default:'06:00'"`
	JamBatasPagi   string `json:"jam_batas_pagi" gorm:"column:jam_batas_pagi;size:5;default:'07:30'"`
	JamTutupPagi   string `json:"jam_tutup_pagi" gorm:"column:jam_tutup_pagi;size:5;default:'09:00'"`
	JamMulaiPulang string `json:"jam_mulai_pulang" gorm:"column:jam_mulai_pulang;size:5;default:'15:00'"`
	JamTutupPulang string `json:"jam_tutup_pulang" gorm:"column:jam_tutup_pulang;size:5;default:'20:00'"`
	// Set kedua jam absen khusus pegawai sekolah -- lihat komentar
	// JamMulaiPagiSekolah di atas struct. Default sengaja dibuat berbeda dari
	// set Dinas/Kantor (mengikuti pola jam sekolah pada umumnya), tapi tetap
	// harus disesuaikan administrator lewat menu Rekap Absen -> Pengaturan.
	JamMulaiPagiSekolah   string   `json:"jam_mulai_pagi_sekolah" gorm:"column:jam_mulai_pagi_sekolah;size:5;default:'06:30'"`
	JamBatasPagiSekolah   string   `json:"jam_batas_pagi_sekolah" gorm:"column:jam_batas_pagi_sekolah;size:5;default:'07:00'"`
	JamTutupPagiSekolah   string   `json:"jam_tutup_pagi_sekolah" gorm:"column:jam_tutup_pagi_sekolah;size:5;default:'08:00'"`
	JamMulaiPulangSekolah string   `json:"jam_mulai_pulang_sekolah" gorm:"column:jam_mulai_pulang_sekolah;size:5;default:'12:30'"`
	JamTutupPulangSekolah string   `json:"jam_tutup_pulang_sekolah" gorm:"column:jam_tutup_pulang_sekolah;size:5;default:'15:00'"`
	TempatTugasAllowed    string   `json:"-" gorm:"column:tempat_tugas_allowed;type:text"`
	JabatanAllowedIDs     string   `json:"-" gorm:"column:jabatan_allowed_ids;type:text"`
	KecamatanAllowedIDs   string   `json:"-" gorm:"column:kecamatan_allowed_ids;type:text"`
	KantorLat             *float64 `json:"-" gorm:"column:kantor_lat"`
	KantorLng             *float64 `json:"-" gorm:"column:kantor_lng"`
	RadiusMeter           int      `json:"-" gorm:"column:radius_meter;default:20"`
}

func (PengaturanAbsensi) TableName() string { return "pengaturan_absensi" }
