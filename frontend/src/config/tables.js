// Central config for the generic CrudManager component.
// Each entry describes one table: its API endpoint, the DataTable columns,
// and the add/edit form fields (including remote-lookup dropdowns for
// foreign keys). Adding a brand-new master table only means adding one
// entry here - no new Vue file needed.

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
    roles: ['administrator', 'admin'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'jabatan', header: 'Nama Jabatan' }],
    formFields: [{ field: 'jabatan', label: 'Nama Jabatan', type: 'text', required: true }],
    searchPlaceholder: 'Cari jabatan...',
  },

  'unit-kerja': {
    title: 'Unit Kerja',
    subtitle: 'Kelola data master unit kerja / bidang',
    endpoint: '/unit-kerja',
    roles: ['administrator', 'admin'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'unit', header: 'Unit Kerja' }],
    formFields: [{ field: 'unit', label: 'Unit Kerja', type: 'text', required: true }],
    searchPlaceholder: 'Cari unit kerja...',
  },

  status: {
    title: 'Status Pegawai',
    subtitle: 'Kelola data master status kepegawaian',
    endpoint: '/status',
    roles: ['administrator', 'admin'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'status', header: 'Status' }],
    formFields: [{ field: 'status', label: 'Status', type: 'text', required: true }],
    searchPlaceholder: 'Cari status...',
  },

  pangkat: {
    title: 'Pangkat',
    subtitle: 'Kelola data master pangkat',
    endpoint: '/pangkat',
    roles: ['administrator', 'admin'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'pangkat', header: 'Pangkat' }],
    formFields: [{ field: 'pangkat', label: 'Pangkat', type: 'text', required: true }],
    searchPlaceholder: 'Cari pangkat...',
  },

  golongan: {
    title: 'Golongan',
    subtitle: 'Kelola data master golongan',
    endpoint: '/golongan',
    roles: ['administrator', 'admin'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'gol', header: 'Golongan' }],
    formFields: [{ field: 'gol', label: 'Golongan', type: 'text', required: true }],
    searchPlaceholder: 'Cari golongan...',
  },

  'pangkat-gol': {
    title: 'Kombinasi Pangkat / Golongan',
    subtitle: 'Pasangan pangkat dengan golongan, dipakai saat mengisi data pegawai',
    endpoint: '/pangkat-gol',
    roles: ['administrator', 'admin'],
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
    roles: ['administrator', 'admin'],
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
    roles: ['administrator', 'admin'],
    columns: [{ field: 'id', header: 'ID', width: '80px' }, { field: 'pola', header: 'Pola Hari Kerja' }],
    formFields: [{ field: 'pola', label: 'Pola Hari Kerja', type: 'text', required: true, placeholder: 'contoh: 5 Hari Kerja (Senin-Jumat)' }],
    searchPlaceholder: 'Cari pola hari kerja...',
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
    formFields: [
      { field: 'nip', label: 'NIP', type: 'text', required: true },
      { field: 'nama', label: 'Nama Lengkap', type: 'text', required: true },
      { field: 'id_jabatan', label: 'Jabatan', type: 'select', ref: 'jabatan', optionLabel: 'jabatan', optionValue: 'id' },
      { field: 'id_unit_kerja', label: 'Unit Kerja', type: 'select', ref: 'unit-kerja', optionLabel: 'unit', optionValue: 'id' },
      { field: 'id_pangkat_gol', label: 'Pangkat / Golongan', type: 'select', ref: 'pangkat-gol', optionLabel: (o) => `${o.pangkat?.pangkat || '-'} / ${o.gol?.gol || '-'}`, optionValue: 'id' },
      { field: 'id_status', label: 'Status', type: 'select', ref: 'status', optionLabel: 'status', optionValue: 'id' },
      { field: 'id_atasan', label: 'Atasan Langsung', type: 'select', ref: 'pegawai', optionLabel: (o) => `${o.nama} (${o.nip})`, optionValue: 'id' },
      { field: 'tempat_tgs', label: 'Tempat Tugas', type: 'text' },
      { field: 'tmt', label: 'TMT', type: 'date' },
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
    ],
    formFields: [
      { field: 'username', label: 'Username', type: 'text', required: true },
      { field: 'nama', label: 'Nama', type: 'text', required: true },
      { field: 'password', label: 'Password', type: 'password', requiredOnCreate: true, hint: 'Kosongkan saat edit jika tidak ingin mengubah password' },
      { field: 'id_role', label: 'Role', type: 'select', required: true, ref: 'role', optionLabel: 'role', optionValue: 'id' },
      { field: 'id_pegawai', label: 'Hubungkan ke Pegawai (opsional)', type: 'select', ref: 'pegawai', optionLabel: (o) => `${o.nama} (${o.nip})`, optionValue: 'id' },
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
