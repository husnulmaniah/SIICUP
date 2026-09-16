// Central config for the generic CrudManager component.
// Each entry describes one table: its API endpoint, the DataTable columns,
// and the add/edit form fields (including remote-lookup dropdowns for
// foreign keys). Adding a brand-new master table only means adding one
// entry here - no new Vue file needed.

// Pilihan Jenis Jabatan (statis, bukan lookup ke tabel referensi lain) --
// dipakai form select Jabatan di atas & untuk menampilkan label yang enak
// dibaca ("Pelaksana" dsb.) di kolom tabel & tempat lain yang butuh (mis.
// Detail Pegawai). Menentukan usia pensiun standar pegawai (lihat menu
// Pengajuan Pensiun): Pelaksana & Struktural pensiun di usia yang sama,
// Fungsional di usia yang berbeda (default 58 & 60, bisa diubah admin).
export const jenisJabatanOptions = [
  { id: 'pelaksana', label: 'Pelaksana' },
  { id: 'struktural', label: 'Struktural' },
  { id: 'fungsional', label: 'Fungsional' },
]
export const jenisJabatanLabels = jenisJabatanOptions.reduce((acc, o) => ({ ...acc, [o.id]: o.label }), {})

// Pilihan Tempat Kerja (statis, sama pola dengan Jenis Jabatan di atas) --
// kategori EKSPLISIT dinas/kantor vs sekolah untuk satu Unit Kerja (lihat
// formField 'tempat_kerja' pada tableConfigs['unit-kerja'] di bawah). Kalau
// diisi, backend memakai ini sebagai acuan UTAMA untuk menentukan jam kerja
// absen, 5/6 hari kerja, dan syarat dokumen cuti pegawai di unit kerja itu --
// menggantikan tebakan otomatis dari kata "sekolah" pada Tempat Tugas
// pegawai (lihat isSekolahPegawai di backend/handlers/pengajuan_cuti.go).
export const tempatKerjaOptions = [
  { id: 'dinas', label: 'Dinas/Kantor' },
  { id: 'sekolah', label: 'Sekolah' },
]
export const tempatKerjaLabels = tempatKerjaOptions.reduce((acc, o) => ({ ...acc, [o.id]: o.label }), {})

// Pilihan Kode Jenis Surat Kolektif (statis) -- menentukan bagaimana satu
// jenis surat kolektif (lihat tableConfigs['jenis-surat'] di bawah) dihitung
// & ditampilkan di rekap/PDF absen: DD (Dinas Dalam), I (Izin), S (Sakit).
// Harus sama persis dengan validasi backend (models.AbsensiDokumenKodeLabel).
export const jenisSuratKodeOptions = [
  { id: 'DD', label: 'DD -- Dinas Dalam' },
  { id: 'I', label: 'I -- Izin' },
  { id: 'S', label: 'S -- Sakit' },
]
export const jenisSuratKodeLabels = jenisSuratKodeOptions.reduce((acc, o) => ({ ...acc, [o.id]: o.label }), {})

export const tableConfigs = {
  role: {
    title: 'Role',
    subtitle: 'Kelola daftar role pengguna sistem',
    endpoint: '/role',
    roles: ['administrator'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'role', header: 'Nama Role' }],
    formFields: [{ field: 'role', label: 'Nama Role', type: 'text', required: true }],
    searchPlaceholder: 'Cari role...',
  },

  jabatan: {
    title: 'Jabatan',
    subtitle: 'Kelola data master jabatan pegawai',
    endpoint: '/jabatan',
    roles: ['administrator'],
    columns: [
      { field: 'id', header: 'ID', width: '80px' },
      { field: 'jabatan', header: 'Nama Jabatan' },
      { field: 'jenis_jabatan', header: 'Jenis Jabatan', type: 'lookup', map: jenisJabatanLabels, width: '160px' },
    ],
    formFields: [
      { field: 'jabatan', label: 'Nama Jabatan', type: 'text', required: true },
      {
        field: 'jenis_jabatan',
        label: 'Jenis Jabatan',
        type: 'select',
        required: true,
        staticOptions: jenisJabatanOptions,
        optionLabel: 'label',
        hint: 'Menentukan usia pensiun standar jabatan ini (diatur di menu Pengajuan Pensiun): Pelaksana & Struktural, atau Fungsional.',
      },
    ],
    searchPlaceholder: 'Cari jabatan...',
  },

  kecamatan: {
    title: 'Kecamatan',
    subtitle: 'Kelola data master kecamatan, dipakai mengelompokkan unit kerja/sekolah untuk keperluan absen',
    endpoint: '/kecamatan',
    roles: ['administrator'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'nama', header: 'Nama Kecamatan' }],
    formFields: [{ field: 'nama', label: 'Nama Kecamatan', type: 'text', required: true }],
    searchPlaceholder: 'Cari kecamatan...',
  },

  'unit-kerja': {
    title: 'Unit Kerja',
    subtitle: 'Kelola data master unit kerja / sekolah, kecamatan, dan titik koordinat absen masing-masing',
    endpoint: '/unit-kerja',
    roles: ['administrator'],
    columns: [
      { field: 'id', header: 'ID', width: '80px' },
      { field: 'unit', header: 'Unit Kerja' },
      { field: 'kecamatan.nama', header: 'Kecamatan', width: '160px' },
      { field: 'tempat_kerja', header: 'Tempat Kerja', type: 'lookup', map: tempatKerjaLabels, width: '130px' },
      { field: 'lat', header: 'Titik Koordinat Absen', type: 'coords', latField: 'lat', lngField: 'lng', width: '160px' },
    ],
    formFields: [
      { field: 'unit', label: 'Unit Kerja / Sekolah', type: 'text', required: true },
      {
        field: 'id_kecamatan',
        label: 'Kecamatan',
        type: 'select',
        ref: 'kecamatan',
        optionLabel: 'nama',
        optionValue: 'id',
        hint: 'Opsional. Mengelompokkan unit kerja/sekolah ini, dipakai juga di Pengaturan Absen untuk memilih kecamatan mana saja yang boleh absen.',
      },
      {
        field: 'tempat_kerja',
        label: 'Tempat Kerja',
        type: 'select',
        staticOptions: tempatKerjaOptions,
        optionLabel: 'label',
        hint: 'Opsional. Kalau diisi, jam kerja absen, 5/6 hari kerja, & syarat dokumen cuti pegawai di unit kerja ini otomatis mengikuti kategori ini (lebih diutamakan daripada tebakan dari kata "sekolah" pada Tempat Tugas pegawai). Kosongkan untuk memakai tebakan otomatis seperti sebelumnya.',
      },
      {
        field: 'lat',
        type: 'coords',
        label: 'Titik Koordinat Absen',
        latField: 'lat',
        lngField: 'lng',
        radiusField: 'radius_meter',
        hint: 'Untuk unit kerja bertempat Dinas/Kantor: BOLEH dikosongkan -- otomatis memakai titik kantor pusat yang sudah ditetapkan di menu Rekap Absen -> Pengaturan, jadi tidak perlu isi satu-satu. Untuk unit bertempat Sekolah: SEBAIKNYA diisi sendiri satu-satu -- kalau dikosongkan, absen di sekolah itu TIDAK otomatis memakai titik kantor dinas (supaya tidak salah lokasi), geofence-nya dianggap belum diatur sampai titik ini diisi.',
      },
    ],
    searchPlaceholder: 'Cari unit kerja...',
  },

  status: {
    title: 'Status Pegawai',
    subtitle: 'Kelola data master status kepegawaian',
    endpoint: '/status',
    roles: ['administrator'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'status', header: 'Status' }],
    formFields: [{ field: 'status', label: 'Status', type: 'text', required: true }],
    searchPlaceholder: 'Cari status...',
  },

  pangkat: {
    title: 'Pangkat',
    subtitle: 'Kelola data master pangkat',
    endpoint: '/pangkat',
    roles: ['administrator'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'pangkat', header: 'Pangkat' }],
    formFields: [{ field: 'pangkat', label: 'Pangkat', type: 'text', required: true }],
    searchPlaceholder: 'Cari pangkat...',
  },

  golongan: {
    title: 'Golongan',
    subtitle: 'Kelola data master golongan',
    endpoint: '/golongan',
    roles: ['administrator'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'gol', header: 'Golongan' }],
    formFields: [{ field: 'gol', label: 'Golongan', type: 'text', required: true }],
    searchPlaceholder: 'Cari golongan...',
  },

  'pangkat-gol': {
    title: 'Kombinasi Pangkat / Golongan',
    subtitle: 'Pasangan pangkat dengan golongan, dipakai saat mengisi data pegawai',
    endpoint: '/pangkat-gol',
    roles: ['administrator'],
    columns: [
      { field: 'id', header: 'ID', width: '80px' },
      { field: 'pangkat.pangkat', header: 'Pangkat' },
      { field: 'gol.gol', header: 'Golongan' },
    ],
    formFields: [
      { field: 'id_pangkat', label: 'Pangkat', type: 'select', required: true, ref: 'pangkat', optionLabel: 'pangkat', optionValue: 'id' },
      { field: 'id_gol', label: 'Golongan', type: 'select', required: true, ref: 'golongan', optionLabel: 'gol', optionValue: 'id' },
    ],
  },

  'jenis-cuti': {
    title: 'Jenis Cuti',
    subtitle: 'Kelola jenis-jenis cuti beserta jatah default per tahun',
    endpoint: '/jenis-cuti',
    roles: ['administrator'],
    columns: [
      { field: 'id', header: 'ID', width: '80px' },
      { field: 'jenis', header: 'Jenis Cuti' },
      { field: 'default_jatah', header: 'Jatah Default', width: '140px' },
      { field: 'keterangan', header: 'Keterangan' },
    ],
    formFields: [
      { field: 'jenis', label: 'Jenis Cuti', type: 'text', required: true },
      { field: 'default_jatah', label: 'Jatah Default (hari/tahun)', type: 'number' },
      { field: 'keterangan', label: 'Keterangan', type: 'textarea' },
    ],
    searchPlaceholder: 'Cari jenis cuti...',
  },

  'pola-hari-kerja': {
    title: 'Pola Hari Kerja',
    subtitle: 'Kelola pola hari kerja (menentukan hari yang dihitung sebagai hari cuti)',
    endpoint: '/pola-hari-kerja',
    roles: ['administrator'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'pola', header: 'Pola Hari Kerja' }],
    formFields: [{ field: 'pola', label: 'Pola Hari Kerja', type: 'text', required: true, placeholder: 'contoh: 5 Hari Kerja (Senin-Jumat)' }],
    searchPlaceholder: 'Cari pola hari kerja...',
  },

  'jenis-surat': {
    title: 'Jenis Surat',
    subtitle: 'Kelola jenis-jenis surat kolektif yang bisa dipilih saat menginput/mengajukan Surat Kolektif pada menu Rekap Absen',
    endpoint: '/jenis-surat',
    roles: ['administrator'],
    columns: [
      { field: 'id', header: 'ID', width: '80px' },
      { field: 'nama', header: 'Nama Jenis Surat' },
      { field: 'slug', header: 'Slug', width: '180px' },
      { field: 'kode', header: 'Kode', type: 'lookup', map: jenisSuratKodeLabels, width: '160px' },
    ],
    formFields: [
      { field: 'nama', label: 'Nama Jenis Surat', type: 'text', required: true, hint: 'Nama yang tampil di dropdown Surat Kolektif, mis. "Surat Tugas" atau "Surat Dinas Luar".' },
      {
        field: 'slug',
        label: 'Slug (kode unik)',
        type: 'text',
        required: true,
        hint: 'Wajib unik, huruf kecil tanpa spasi (gunakan garis bawah), mis. "surat_dinas_luar". Ini yang tersimpan di data surat -- jangan diubah lagi setelah dipakai, supaya surat yang sudah ada tetap cocok.',
      },
      {
        field: 'kode',
        label: 'Kode',
        type: 'text',
        required: true,
        hint: 'Bebas, tentukan sendiri (maks. 10 karakter), mis. "DD", "I", "S", atau kode lain seperti "CT" untuk Cuti Tahunan. Khusus kode DD/I/S yang dihitung ke kolom Jumlah DD/Izin/Sakit di rekap & PDF absen; kode lain tetap menutup tanggal terlewat pegawai tapi tidak masuk ke tiga kolom itu.',
      },
    ],
    searchPlaceholder: 'Cari jenis surat...',
  },

  'tgl-merah': {
    title: 'Tanggal Merah / Hari Libur',
    subtitle: 'Kelola daftar hari libur nasional yang mengurangi hitungan hari cuti',
    endpoint: '/tgl-merah',
    roles: ['administrator', 'admin'],
    columns: [
      { field: 'id', header: 'ID', width: '80px' },
      { field: 'tgl', header: 'Tanggal', type: 'date' },
      { field: 'keterangan', header: 'Keterangan' },
    ],
    formFields: [
      { field: 'tgl', label: 'Tanggal', type: 'date', required: true },
      { field: 'keterangan', label: 'Keterangan', type: 'text', required: true },
    ],
  },

  pegawai: {
    title: 'Data Pegawai',
    subtitle: 'Kelola data induk pegawai',
    endpoint: '/pegawai',
    roles: ['administrator', 'admin'],
    columns: [
      { field: 'nip', header: 'NIP', width: '170px' },
      { field: 'nama', header: 'Nama' },
      { field: 'jabatan.jabatan', header: 'Jabatan' },
      { field: 'unit_kerja.unit', header: 'Unit Kerja' },
      { field: 'status.status', header: 'Status', type: 'badge' },
      { field: 'no_hp', header: 'No HP' },
    ],
    // filters: dropdown pencarian TAMBAHAN di atas tabel, di luar kotak cari
    // nama/NIP biasa (lihat searchPlaceholder di bawah) -- supaya Data
    // Pegawai bisa dicari berdasarkan Jenis Jabatan, Jabatan, Kecamatan,
    // maupun Unit Kerja sekaligus (dikombinasikan, bukan salah satu saja).
    // Jenis Jabatan pakai staticOptions (3 pilihan tetap, sama seperti form
    // Jabatan di atas); Jabatan/Unit Kerja pakai ref yang sama dengan form
    // tambah/edit pegawai; Kecamatan pakai ref 'kecamatan' (dicocokkan lewat
    // kecamatan unit kerja/sekolah pegawai di backend, karena pegawai tidak
    // punya kolom kecamatan sendiri).
    filters: [
      { field: 'jenis_jabatan', label: 'Jenis Jabatan', type: 'select', staticOptions: jenisJabatanOptions, optionLabel: 'label' },
      { field: 'id_jabatan', label: 'Jabatan', type: 'select', ref: 'jabatan', optionLabel: 'jabatan' },
      { field: 'id_kecamatan', label: 'Kecamatan', type: 'select', ref: 'kecamatan', optionLabel: 'nama' },
      { field: 'id_unit_kerja', label: 'Unit Kerja', type: 'select', ref: 'unit-kerja', optionLabel: 'unit' },
    ],
    formFields: [
      { field: 'nip', label: 'NIP', type: 'text', required: true },
      { field: 'nama', label: 'Nama Lengkap', type: 'text', required: true },
      { field: 'id_jabatan', label: 'Jabatan', type: 'select', ref: 'jabatan', optionLabel: 'jabatan', optionValue: 'id' },
      { field: 'id_unit_kerja', label: 'Unit Kerja', type: 'select', ref: 'unit-kerja', optionLabel: 'unit', optionValue: 'id' },
      { field: 'id_pangkat_gol', label: 'Pangkat / Golongan', type: 'select', ref: 'pangkat-gol', optionLabel: (o) => `${o.pangkat?.pangkat || '-'} / ${o.gol?.gol || '-'}`, optionValue: 'id' },
      { field: 'id_status', label: 'Status', type: 'select', ref: 'status', optionLabel: 'status', optionValue: 'id' },
      { field: 'id_atasan', label: 'Atasan Langsung', type: 'select', ref: 'pegawai', optionLabel: (o) => `${o.nama} (${o.nip})`, optionValue: 'id' },
      {
        field: 'tempat_tgs',
        label: 'Tempat Tugas (Dinas/Kantor atau Sekolah)',
        type: 'select',
        staticOptions: tempatKerjaOptions,
        optionLabel: 'label',
        // syncFrom: begitu admin memilih/mengubah Unit Kerja di atas, field
        // ini otomatis ikut kategori Tempat Kerja unit kerja tersebut (kalau
        // unit kerjanya sudah dikategorikan) -- lihat penanganan generik
        // "syncFrom" di CrudManager.vue. Ini menghubungkan langsung data
        // Tempat Tugas pegawai dengan Tempat Kerja Unit Kerja, jadi admin
        // tidak perlu isi manual dua kali & datanya selalu konsisten.
        syncFrom: { field: 'id_unit_kerja', ref: 'unit-kerja', pick: (uk) => uk?.tempat_kerja || null },
        hint: 'Otomatis ikut kategori "Tempat Kerja" dari Unit Kerja yang dipilih di atas. Kalau Unit Kerja belum dikategorikan (lihat menu Unit Kerja), pilih manual di sini.',
      },
      { field: 'tmt', label: 'TMT', type: 'date' },
      {
        field: 'tgl_lahir',
        label: 'Tanggal Lahir',
        type: 'date',
        hint: 'Dipakai untuk menghitung usia & kelayakan pensiun pegawai ini (lihat menu Pengajuan Pensiun).',
      },
      {
        field: 'tgl_kenaikan_gaji_berkala_terakhir',
        label: 'Kenaikan Gaji Berkala Terakhir',
        type: 'date',
        hint: 'Opsional. Dipakai menghitung kapan kenaikan gaji berkala berikutnya jatuh tempo (lihat menu Perubahan Data Pegawai -> Pengaturan Kenaikan Gaji Berkala).',
      },
      {
        field: 'tgl_kenaikan_pangkat_terakhir',
        label: 'Kenaikan Pangkat Terakhir',
        type: 'date',
        hint: 'Opsional. Dipakai menghitung kapan kenaikan pangkat berikutnya jatuh tempo.',
      },
      { field: 'no_hp', label: 'No HP', type: 'text' },
      { field: 'email', label: 'Email', type: 'text' },
    ],
    searchPlaceholder: 'Cari nama / NIP...',
  },

  user: {
    title: 'Akun Pengguna',
    subtitle: 'Kelola akun login untuk setiap role (administrator, admin, atasan, pegawai)',
    endpoint: '/user',
    roles: ['administrator'],
    columns: [
      { field: 'username', header: 'Username' },
      { field: 'nama', header: 'Nama' },
      { field: 'role.role', header: 'Role', type: 'badge' },
      { field: 'pegawai.nama', header: 'Terhubung ke Pegawai' },
      { field: 'is_admin_absensi', header: 'Admin Absensi', type: 'boolean' },
      { field: 'is_admin_verifikasi', header: 'Admin Verifikasi', type: 'boolean' },
    ],
    formFields: [
      { field: 'username', label: 'Username', type: 'text', required: true },
      { field: 'nama', label: 'Nama', type: 'text', required: true },
      { field: 'password', label: 'Password', type: 'password', requiredOnCreate: true, hint: 'Kosongkan saat edit jika tidak ingin mengubah password' },
      { field: 'id_role', label: 'Role', type: 'select', required: true, ref: 'role', optionLabel: 'role', optionValue: 'id' },
      { field: 'id_pegawai', label: 'Hubungkan ke Pegawai (opsional)', type: 'select', ref: 'pegawai', optionLabel: (o) => `${o.nama} (${o.nip})`, optionValue: 'id' },
      {
        field: 'is_admin_absensi',
        label: 'Admin Absensi',
        type: 'checkbox',
        hint: 'Jika dicentang, akun ini tetap punya menu biasa (Dashboard, Pengajuan Cuti, Absen, dst.) DITAMBAH 1 menu "Input Rekapan Absensi" untuk mengelola rekap absensi semua pegawai & surat kolektif.',
      },
      {
        field: 'is_admin_verifikasi',
        label: 'Admin Verifikasi',
        type: 'checkbox',
        hint: 'Jika dicentang, akun ini boleh memverifikasi (menyetujui/mengembalikan) Pengajuan Surat Kolektif dari pegawai sekolah. Independen dari Admin Absensi -- kalau KEDUANYA dicentang, akun ini bisa menginput Surat Kolektif dinas SEKALIGUS memverifikasi pengajuan surat kolektif sekolah.',
      },
    ],
    searchPlaceholder: 'Cari username / nama...',
  },

  'jatah-cuti': {
    title: 'Jatah Cuti Tahunan',
    subtitle: 'Kelola kuota cuti tahunan per pegawai per tahun',
    endpoint: '/jatah-cuti',
    roles: ['administrator', 'admin'],
    columns: [
      { field: 'pegawai.nip', header: 'NIP' },
      { field: 'pegawai.nama', header: 'Nama Pegawai' },
      { field: 'tahun', header: 'Tahun', width: '100px' },
      { field: 'jumlah_hari', header: 'Jumlah Hari', width: '120px' },
      { field: 'terpakai', header: 'Terpakai', width: '110px' },
    ],
    formFields: [
      { field: 'id_pegawai', label: 'Pegawai', type: 'select', required: true, ref: 'pegawai', optionLabel: (o) => `${o.nama} (${o.nip})`, optionValue: 'id' },
      { field: 'tahun', label: 'Tahun', type: 'number', required: true },
      { field: 'jumlah_hari', label: 'Jumlah Hari', type: 'number', required: true },
      { field: 'terpakai', label: 'Terpakai', type: 'number' },
    ],
    searchPlaceholder: 'Cari...',
  },
}

export function getConfig(tableKey) {
  return tableConfigs[tableKey]
}
