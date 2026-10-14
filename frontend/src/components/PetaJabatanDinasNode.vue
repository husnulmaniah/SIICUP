<script setup>
import { ref, computed } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'

// PetaJabatanDinasNode -- SATU node pohon struktur "Peta Jabatan Dinas"
// (akar/Sekretariat/Bidang/Seksi/Sub Bagian), merender dirinya sendiri
// SECARA REKURSIF untuk seluruh anaknya (lihat <PetaJabatanDinasNode> di
// template bawah -- didukung otomatis oleh Vue 3 untuk SFC lewat nama yang
// diambil dari nama berkas, lihat defineOptions di bawah) supaya kedalaman
// pohon TIDAK dibatasi kode (administrator menambah cabang baru kapan pun
// tanpa perlu ubah komponen ini). Dipakai dari PetaJabatanView.vue, tab
// "Peta Jabatan Dinas".
//
// Seluruh aksi KELOLA (tambah/ubah nama/hapus struktur, ubah kepala, ubah
// KJ/K) HANYA tampil kalah bolehKelola true -- diteruskan dari induk,
// berasal dari petaJabatanDinasOut.boleh_kelola (administrator ONLY, lihat
// backend/handlers/peta_jabatan_dinas.go). Setiap aksi yang berhasil
// memancarkan event "changed" ke atas supaya PetaJabatanView.vue memuat
// ulang SELURUH pohon dari awal (paling sederhana & aman -- pohon ini kecil,
// tidak perlu optimisasi partial-refresh).
defineOptions({ name: 'PetaJabatanDinasNode' })

const props = defineProps({
  node: { type: Object, required: true },
  bolehKelola: { type: Boolean, default: false },
  pegawaiOptions: { type: Array, default: () => [] },
  // pegawaiBelumDitempatkan: daftar pegawai Dinas yang BELUM punya unit
  // struktur sama sekali (dari PetaJabatanView.vue, endpoint
  // /peta-jabatan/dinas/pegawai/belum-ditempatkan) -- dipakai sebagai opsi
  // pada dialog "Kelola Pegawai Unit Ini" di bawah, supaya pegawai yang
  // belum punya Bidang/Seksi bisa LANGSUNG ditambahkan dari kartu unit ini
  // juga (permintaan pengguna), tidak hanya lewat tabel terpisah di bagian
  // bawah halaman.
  pegawaiBelumDitempatkan: { type: Array, default: () => [] },
  // jabatanOptions: daftar SELURUH jabatan master data (dari PetaJabatanView.vue,
  // endpoint /ref/jabatan) -- dipakai dialog "Tambah Jabatan" di bawah supaya
  // administrator bisa menambahkan jabatan baru yang BELUM ada pada unit ini
  // (permintaan pengguna).
  jabatanOptions: { type: Array, default: () => [] },
})
const emit = defineEmits(['changed'])

const toast = useToast()
const confirm = useConfirm()

const tipeLabel = computed(() => {
  switch (props.node.tipe) {
    case 'dinas':
      return 'DINAS'
    case 'sekretariat':
      return 'SEKRETARIAT'
    case 'bidang':
      return 'BIDANG'
    case 'seksi':
      return 'SEKSI'
    case 'sub_bagian':
      return 'SUB BAGIAN'
    default:
      return (props.node.tipe || '-').toUpperCase()
  }
})
const tipeSeverity = computed(() => {
  if (props.node.tipe === 'dinas') return 'danger'
  if (props.node.tipe === 'sekretariat' || props.node.tipe === 'bidang') return 'info'
  return 'success'
})

// anak APA SAJA yang boleh ditambahkan di bawah node ini -- mengikuti garis
// komando yang sama dengan validateIndukTipeDinasStruktur di backend (akar
// -> Sekretariat/Bidang, Sekretariat -> Sub Bagian, Bidang -> Seksi).
const childTipeOptions = computed(() => {
  switch (props.node.tipe) {
    case 'dinas':
      return [
        { label: 'Sekretariat', value: 'sekretariat' },
        { label: 'Bidang', value: 'bidang' },
      ]
    case 'sekretariat':
      return [{ label: 'Sub Bagian', value: 'sub_bagian' }]
    case 'bidang':
      return [{ label: 'Seksi', value: 'seksi' }]
    default:
      return []
  }
})
const canHaveChildren = computed(() => childTipeOptions.value.length > 0)
const isRoot = computed(() => props.node.tipe === 'dinas')

function signed(n) {
  return n > 0 ? `+${n}` : `${n}`
}
function selisihSeverity(n) {
  return n < 0 ? 'danger' : n > 0 ? 'warn' : 'success'
}

// ============================================================
// permintaan pengguna: bedakan posisi Jabatan Pelaksana vs Jabatan
// Fungsional pada tabel KJ/B/K/+- unit ini, PERSIS gaya gambar referensi
// ("JABATAN PELAKSANA" & "JABATAN FUNGSIONAL" sebagai pita kelompok +
// baris "Jumlah Kebutuhan .." masing-masing) -- konsisten dengan
// pengelompokan yang sama pada PDF (drawDinasChartGroup) & Excel
// (writeDinasJabatanGroupExcel) di backend. Kelompok yang kosong tidak
// dirender sama sekali (lihat v-if pada template).
// ============================================================
const pelaksanaRows = computed(() => props.node.rows.filter((r) => r.jenis_jabatan !== 'fungsional'))
const fungsionalRows = computed(() => props.node.rows.filter((r) => r.jenis_jabatan === 'fungsional'))
function sumBK(rows) {
  return rows.reduce((acc, r) => ({ b: acc.b + r.b, k: acc.k + r.k }), { b: 0, k: 0 })
}
const pelaksanaSum = computed(() => sumBK(pelaksanaRows.value))
const fungsionalSum = computed(() => sumBK(fungsionalRows.value))

// ============================================================
// Tambah Unit Anak
// ============================================================
const tambahDialog = ref(false)
const tambahSubmitting = ref(false)
const tambahForm = ref({ unit: '', tipe: null })
function openTambahDialog() {
  tambahForm.value = { unit: '', tipe: childTipeOptions.value[0]?.value || null }
  tambahDialog.value = true
}
async function simpanTambah() {
  const unit = (tambahForm.value.unit || '').trim()
  if (!unit || !tambahForm.value.tipe) {
    toast.add({ severity: 'warn', summary: 'Lengkapi data', detail: 'Tipe & nama unit wajib diisi', life: 3000 })
    return
  }
  tambahSubmitting.value = true
  try {
    await http.post('/peta-jabatan/dinas/struktur', {
      unit,
      tipe: tambahForm.value.tipe,
      id_induk_unit_kerja: props.node.id,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `Unit "${unit}" ditambahkan`, life: 3000 })
    tambahDialog.value = false
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menambah unit', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    tambahSubmitting.value = false
  }
}

// ============================================================
// Ubah Nama
// ============================================================
const renameDialog = ref(false)
const renameSubmitting = ref(false)
const renameValue = ref('')
function openRenameDialog() {
  renameValue.value = props.node.unit
  renameDialog.value = true
}
async function simpanRename() {
  const unit = (renameValue.value || '').trim()
  if (!unit) {
    toast.add({ severity: 'warn', summary: 'Nama tidak boleh kosong', detail: '', life: 3000 })
    return
  }
  renameSubmitting.value = true
  try {
    await http.put(`/peta-jabatan/dinas/struktur/${props.node.id}`, { unit })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Nama unit disimpan', life: 3000 })
    renameDialog.value = false
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    renameSubmitting.value = false
  }
}

// ============================================================
// Hapus
// ============================================================
function konfirmasiHapus() {
  confirm.require({
    message: `Hapus unit "${props.node.unit}"? Pastikan tidak ada unit anak atau pegawai yang masih ditempatkan di sini.`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await http.delete(`/peta-jabatan/dinas/struktur/${props.node.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Unit dihapus', life: 3000 })
        emit('changed')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 5000 })
      }
    },
  })
}

// ============================================================
// Ubah Kepala Unit
// ============================================================
const kepalaDialog = ref(false)
const kepalaSubmitting = ref(false)
const kepalaIdPegawai = ref(null)
const pegawaiSelectOptions = computed(() => props.pegawaiOptions.map((p) => ({ label: `${p.nama} (${p.nip})`, value: p.id })))
function openKepalaDialog() {
  kepalaIdPegawai.value = props.node.kepala?.id_pegawai ?? null
  kepalaDialog.value = true
}
async function simpanKepala() {
  kepalaSubmitting.value = true
  try {
    await http.put(`/peta-jabatan/dinas/struktur/${props.node.id}/kepala`, { id_pegawai: kepalaIdPegawai.value })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Kepala unit disimpan', life: 3000 })
    kepalaDialog.value = false
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    kepalaSubmitting.value = false
  }
}

// ============================================================
// Kelola Pegawai Unit Ini -- permintaan pengguna: mengedit pegawai mana
// yang masuk ke Bidang/Seksi (dst) apa dapat LANGSUNG diedit dari kartu
// unit itu sendiri, persis alur "Ubah Kepala" di atas (dialog per node),
// bukan hanya lewat tabel terpisah "Pegawai Dinas Belum Ditempatkan" di
// bagian bawah halaman. Dialog ini menampilkan DUA hal: (1) roster pegawai
// yang SAAT INI ada di unit ini (GET .../pegawai/di-unit/{id}, bisa
// dikeluarkan satu per satu), (2) Select untuk menambahkan pegawai yang
// BELUM punya Bidang/Seksi (prop pegawaiBelumDitempatkan) ke unit ini --
// keduanya memakai ULANG endpoint PUT .../pegawai/{id}/unit-kerja yang sama
// dengan tabel "belum ditempatkan" di PetaJabatanView.vue (mengosongkan =
// id_unit_kerja null, menambahkan = id_unit_kerja id unit ini), supaya
// tidak ada logika penempatan ganda yang bisa tidak sinkron.
// ============================================================
const pegawaiDialog = ref(false)
const pegawaiDiUnit = ref([])
const pegawaiDiUnitLoading = ref(false)
// tambahPegawaiIds: ARRAY (permintaan pengguna -- bisa menambahkan LEBIH
// DARI SATU pegawai sekaligus, tidak hanya satu per satu).
const tambahPegawaiIds = ref([])
const tambahPegawaiSubmitting = ref(false)
const keluarkanSubmitting = ref({})

const pegawaiBelumOptions = computed(() =>
  props.pegawaiBelumDitempatkan.map((p) => ({ label: `${p.nama} (${p.nip})`, value: p.id })),
)

async function loadPegawaiDiUnit() {
  pegawaiDiUnitLoading.value = true
  try {
    const { data } = await http.get(`/peta-jabatan/dinas/pegawai/di-unit/${props.node.id}`)
    pegawaiDiUnit.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    pegawaiDiUnitLoading.value = false
  }
}
function openPegawaiDialog() {
  tambahPegawaiIds.value = []
  pegawaiDialog.value = true
  loadPegawaiDiUnit()
}
// tambahPegawaiKeUnit: permintaan pengguna -- bisa menambahkan LEBIH DARI
// SATU pegawai sekaligus. Backend TIDAK punya endpoint "batch" tersendiri,
// jadi tiap pegawai terpilih dikirim lewat PUT .../unit-kerja yang sama
// (SEJAJAR lewat Promise.allSettled supaya satu pegawai gagal tidak
// membatalkan pegawai lain yang berhasil), baru SEKALI muat ulang roster +
// pancarkan 'changed' di akhir.
async function tambahPegawaiKeUnit() {
  if (!tambahPegawaiIds.value.length) return
  tambahPegawaiSubmitting.value = true
  try {
    const ids = [...tambahPegawaiIds.value]
    const hasil = await Promise.allSettled(
      ids.map((id) => http.put(`/peta-jabatan/dinas/pegawai/${id}/unit-kerja`, { id_unit_kerja: props.node.id })),
    )
    const gagal = hasil.filter((h) => h.status === 'rejected').length
    const berhasil = ids.length - gagal
    if (berhasil > 0) {
      toast.add({ severity: 'success', summary: 'Berhasil', detail: `${berhasil} pegawai ditambahkan ke unit ini`, life: 3000 })
    }
    if (gagal > 0) {
      toast.add({ severity: 'error', summary: 'Sebagian gagal', detail: `${gagal} dari ${ids.length} pegawai gagal ditambahkan`, life: 5000 })
    }
    tambahPegawaiIds.value = []
    await loadPegawaiDiUnit()
    emit('changed')
  } finally {
    tambahPegawaiSubmitting.value = false
  }
}
async function keluarkanDariUnit(p) {
  keluarkanSubmitting.value = { ...keluarkanSubmitting.value, [p.id]: true }
  try {
    await http.put(`/peta-jabatan/dinas/pegawai/${p.id}/unit-kerja`, { id_unit_kerja: null })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `${p.nama} dikeluarkan dari unit ini`, life: 3000 })
    await loadPegawaiDiUnit()
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengeluarkan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    const copy = { ...keluarkanSubmitting.value }
    delete copy[p.id]
    keluarkanSubmitting.value = copy
  }
}

// ============================================================
// Roster per-jabatan (kolom "B") -- permintaan pengguna: "pada kolom B
// dapat di lihat siapa saja pegawai yang masuk dalam jabatan tersebut".
// Beda dari dialog "Kelola Pegawai Unit Ini" di atas (yang menampilkan
// SEMUA pegawai unit ini apa pun jabatannya) -- dialog ini DISARING ke
// SATU jabatan saja (query "jabatan" pada endpoint yang sama, dibandingkan
// TANPA memandang huruf besar/kecil oleh backend, lihat
// listPegawaiDiUnitDinasHandler). Pilihan "tambahkan" juga DISARING ke
// pegawai yang Jabatan-nya PERSIS sama (case-insensitive, dihitung DI SINI
// di frontend dari props.pegawaiBelumDitempatkan yang sudah dimuat --
// permintaan pengguna: "munculkan hanya pegawai dengan jabatan yang sesuai
// ... walaupun penulisannya menggunakan huruf besar maupun huruf kecil"),
// jadi TIDAK perlu panggilan API baru untuk sisi "tambah"-nya.
// ============================================================
const rosterDialog = ref(false)
const rosterRow = ref(null)
const rosterPegawai = ref([])
const rosterLoading = ref(false)
// rosterTambahIds: ARRAY (permintaan pengguna -- bisa menambahkan LEBIH
// DARI SATU pegawai sekaligus, tidak hanya satu per satu).
const rosterTambahIds = ref([])
const rosterTambahSubmitting = ref(false)
const rosterKeluarkanSubmitting = ref({})

const rosterTambahOptions = computed(() => {
  const jab = (rosterRow.value?.jabatan || '').trim().toLowerCase()
  if (!jab) return []
  return props.pegawaiBelumDitempatkan
    .filter((p) => (p.jabatan || '').trim().toLowerCase() === jab)
    .map((p) => ({ label: `${p.nama} (${p.nip})`, value: p.id }))
})

async function loadRoster() {
  if (!rosterRow.value) return
  rosterLoading.value = true
  try {
    const { data } = await http.get(`/peta-jabatan/dinas/pegawai/di-unit/${props.node.id}`, {
      params: { jabatan: rosterRow.value.jabatan },
    })
    rosterPegawai.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    rosterLoading.value = false
  }
}
function openRosterDialog(row) {
  rosterRow.value = row
  rosterTambahIds.value = []
  rosterDialog.value = true
  loadRoster()
}
// rosterTambahKeUnit: permintaan pengguna -- bisa menambahkan LEBIH DARI
// SATU pegawai sekaligus ke jabatan ini (sama persis pola
// tambahPegawaiKeUnit di atas -- Promise.allSettled supaya satu pegawai
// gagal tidak membatalkan pegawai lain yang berhasil).
async function rosterTambahKeUnit() {
  if (!rosterTambahIds.value.length) return
  rosterTambahSubmitting.value = true
  try {
    const ids = [...rosterTambahIds.value]
    const hasil = await Promise.allSettled(
      ids.map((id) => http.put(`/peta-jabatan/dinas/pegawai/${id}/unit-kerja`, { id_unit_kerja: props.node.id })),
    )
    const gagal = hasil.filter((h) => h.status === 'rejected').length
    const berhasil = ids.length - gagal
    if (berhasil > 0) {
      toast.add({ severity: 'success', summary: 'Berhasil', detail: `${berhasil} pegawai ditambahkan ke unit ini`, life: 3000 })
    }
    if (gagal > 0) {
      toast.add({ severity: 'error', summary: 'Sebagian gagal', detail: `${gagal} dari ${ids.length} pegawai gagal ditambahkan`, life: 5000 })
    }
    rosterTambahIds.value = []
    await loadRoster()
    emit('changed')
  } finally {
    rosterTambahSubmitting.value = false
  }
}
async function rosterKeluarkan(p) {
  rosterKeluarkanSubmitting.value = { ...rosterKeluarkanSubmitting.value, [p.id]: true }
  try {
    await http.put(`/peta-jabatan/dinas/pegawai/${p.id}/unit-kerja`, { id_unit_kerja: null })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `${p.nama} dikeluarkan dari unit ini`, life: 3000 })
    await loadRoster()
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengeluarkan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    const copy = { ...rosterKeluarkanSubmitting.value }
    delete copy[p.id]
    rosterKeluarkanSubmitting.value = copy
  }
}

// ============================================================
// Tambah Jabatan -- permintaan pengguna: "tambahkan agar administrator
// dapat menambahkan jabatan baru pada bidang atau seksi yang
// membutuhkan". TIDAK perlu endpoint baru -- PUT /peta-jabatan/dinas/formasi
// (dipakai ULANG, sama persis dengan yang dipanggil ubahK di bawah) SUDAH
// membuat baris FormasiJabatan baru kalau belum ada (lihat
// upsertFormasiJabatan di backend), jadi jabatan baru ini otomatis muncul
// pada tabel begitu pohon dimuat ulang (emit 'changed').
// ============================================================
const tambahJabatanDialog = ref(false)
const tambahJabatanSubmitting = ref(false)
const tambahJabatanForm = ref({ id_jabatan: null, k: 0 })

const tambahJabatanOptions = computed(() => {
  const existingIds = new Set(props.node.rows.map((r) => r.id_jabatan))
  return props.jabatanOptions.filter((j) => !existingIds.has(j.value))
})

function openTambahJabatanDialog() {
  tambahJabatanForm.value = { id_jabatan: null, k: 0 }
  tambahJabatanDialog.value = true
}
async function simpanTambahJabatan() {
  if (!tambahJabatanForm.value.id_jabatan) {
    toast.add({ severity: 'warn', summary: 'Pilih jabatan', detail: '', life: 3000 })
    return
  }
  tambahJabatanSubmitting.value = true
  try {
    await http.put('/peta-jabatan/dinas/formasi', {
      id_unit_kerja: props.node.id,
      id_jabatan: tambahJabatanForm.value.id_jabatan,
      kebutuhan: tambahJabatanForm.value.k || 0,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Jabatan baru ditambahkan ke unit ini', life: 3000 })
    tambahJabatanDialog.value = false
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menambahkan jabatan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    tambahJabatanSubmitting.value = false
  }
}

// ============================================================
// Hapus Jabatan -- permintaan pengguna: "buat agar administrator dapat
// menghapus jabatan yang tidak diperlukan" (mis. baris jabatan dobel
// akibat salah ketik huruf besar/kecil pada data master, atau jabatan yang
// memang tidak dipakai unit ini). Backend (hapusFormasiJabatanDinasHandler)
// MENOLAK (400) kalau B (bezetting) jabatan ini di unit ini masih > 0 --
// pesan errornya ditampilkan apa adanya supaya administrator tahu harus
// mengeluarkan pegawainya dulu lewat dialog roster (klik kolom B).
// ============================================================
const hapusJabatanSubmitting = ref({})
function konfirmasiHapusJabatan(row) {
  confirm.require({
    message: `Hapus jabatan "${row.jabatan}" dari unit "${props.node.unit}"? Baris KJ/B/K/+- jabatan ini akan hilang dari tabel.`,
    header: 'Konfirmasi Hapus Jabatan',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      hapusJabatanSubmitting.value = { ...hapusJabatanSubmitting.value, [row.id_jabatan]: true }
      try {
        await http.delete(`/peta-jabatan/dinas/formasi/${props.node.id}/${row.id_jabatan}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: `Jabatan "${row.jabatan}" dihapus dari unit ini`, life: 3000 })
        emit('changed')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menghapus jabatan', detail: e.response?.data?.message || e.message, life: 6000 })
      } finally {
        const copy = { ...hapusJabatanSubmitting.value }
        delete copy[row.id_jabatan]
        hapusJabatanSubmitting.value = copy
      }
    },
  })
}

// ============================================================
// KJ (Jabatan.KelasJabatan, global) & K (FormasiJabatan) -- inline langsung
// pada tabel, tanpa dialog terpisah (beda dari Peta Jabatan Sekolah yang
// butuh dialog "Atur K" karena harus menangani pecahan Sub-Jabatan -- Dinas
// sengaja tidak memakai Sub-Jabatan sama sekali, lihat komentar backend).
// ============================================================
const kjSaving = ref({})
async function ubahKJ(row) {
  kjSaving.value = { ...kjSaving.value, [row.id_jabatan]: true }
  try {
    await http.put(`/peta-jabatan/dinas/jabatan/${row.id_jabatan}/kelas-jabatan`, { kelas_jabatan: row.kj })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `KJ "${row.jabatan}" disimpan`, life: 2500 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan KJ', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    const copy = { ...kjSaving.value }
    delete copy[row.id_jabatan]
    kjSaving.value = copy
  }
}
const kSaving = ref({})
async function ubahK(row) {
  kSaving.value = { ...kSaving.value, [row.id_jabatan]: true }
  try {
    await http.put('/peta-jabatan/dinas/formasi', {
      id_unit_kerja: props.node.id,
      id_jabatan: row.id_jabatan,
      kebutuhan: row.k,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `Kebutuhan (K) "${row.jabatan}" disimpan`, life: 2500 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan K', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    const copy = { ...kSaving.value }
    delete copy[row.id_jabatan]
    kSaving.value = copy
  }
}
</script>

<template>
  <div class="dinas-node" :class="`dinas-node--${node.tipe}`">
    <div class="dinas-node-header">
      <div class="dinas-node-title">
        <Tag :value="tipeLabel" :severity="tipeSeverity" />
        <span class="dinas-node-unit">{{ node.unit }}</span>
      </div>
      <div class="dinas-node-kepala">
        <i class="pi pi-user" style="margin-right: 0.3rem"></i>
        <span v-if="node.kepala?.nama">{{ node.kepala.nama }} -- {{ node.kepala.jabatan }}</span>
        <span v-else style="font-style: italic; color: var(--p-text-muted-color)">belum ditetapkan</span>
      </div>
      <div v-if="bolehKelola" class="table-actions">
        <Button v-if="!isRoot" icon="pi pi-user-edit" size="small" severity="secondary" outlined title="Ubah Kepala Unit" @click="openKepalaDialog" />
        <Button v-if="!isRoot" icon="pi pi-users" size="small" severity="secondary" outlined title="Kelola Pegawai Unit Ini" @click="openPegawaiDialog" />
        <Button v-if="!isRoot" icon="pi pi-briefcase" size="small" severity="secondary" outlined title="Tambah Jabatan" @click="openTambahJabatanDialog" />
        <Button v-if="canHaveChildren" icon="pi pi-plus" size="small" severity="secondary" outlined title="Tambah Unit Anak" @click="openTambahDialog" />
        <Button v-if="!isRoot" icon="pi pi-pencil" size="small" severity="secondary" outlined title="Ubah Nama Unit" @click="openRenameDialog" />
        <Button
          v-if="!isRoot"
          icon="pi pi-trash"
          size="small"
          severity="danger"
          outlined
          title="Hapus Unit"
          :disabled="node.anak.length > 0"
          @click="konfirmasiHapus"
        />
      </div>
    </div>

    <div class="responsive-table-wrap" style="margin-top: 0.5rem">
      <table v-if="node.rows.length" class="dinas-row-table">
        <thead>
          <tr>
            <th>Jabatan</th>
            <th style="width: 90px">KJ</th>
            <th style="width: 70px">B</th>
            <th style="width: 90px">K</th>
            <th style="width: 80px">+/-</th>
            <th v-if="bolehKelola" style="width: 60px">Aksi</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="pelaksanaRows.length">
            <tr class="dinas-group-bar dinas-group-bar--pelaksana">
              <td :colspan="bolehKelola ? 6 : 5">JABATAN PELAKSANA</td>
            </tr>
            <tr v-for="row in pelaksanaRows" :key="`pel-${row.id_jabatan}`">
              <td>{{ row.jabatan }}</td>
              <td>
                <InputNumber
                  v-if="bolehKelola"
                  v-model="row.kj"
                  placeholder="-"
                  :min="1"
                  :max="17"
                  :disabled="kjSaving[row.id_jabatan]"
                  style="width: 5.5rem"
                  inputStyle="width: 2.5rem; padding: 0.25rem 0.4rem"
                  @update:modelValue="ubahKJ(row)"
                />
                <span v-else>{{ row.kj ?? '-' }}</span>
              </td>
              <td>
                <button v-if="bolehKelola" type="button" class="dinas-b-link" title="Lihat pegawai dengan jabatan ini" @click="openRosterDialog(row)">{{ row.b }}</button>
                <span v-else>{{ row.b }}</span>
              </td>
              <td>
                <InputNumber
                  v-if="bolehKelola"
                  v-model="row.k"
                  :min="0"
                  :disabled="kSaving[row.id_jabatan]"
                  style="width: 5.5rem"
                  inputStyle="width: 2.5rem; padding: 0.25rem 0.4rem"
                  @update:modelValue="ubahK(row)"
                />
                <span v-else>{{ row.k }}</span>
              </td>
              <td><Tag :value="signed(row.selisih)" :severity="selisihSeverity(row.selisih)" /></td>
              <td v-if="bolehKelola">
                <Button
                  icon="pi pi-trash"
                  size="small"
                  severity="danger"
                  outlined
                  title="Hapus Jabatan Ini dari Unit"
                  :loading="hapusJabatanSubmitting[row.id_jabatan]"
                  @click="konfirmasiHapusJabatan(row)"
                />
              </td>
            </tr>
            <tr class="dinas-group-subtotal">
              <td>Jumlah Kebutuhan JP</td>
              <td></td>
              <td>{{ pelaksanaSum.b }}</td>
              <td>{{ pelaksanaSum.k }}</td>
              <td><Tag :value="signed(pelaksanaSum.b - pelaksanaSum.k)" :severity="selisihSeverity(pelaksanaSum.b - pelaksanaSum.k)" /></td>
              <td v-if="bolehKelola"></td>
            </tr>
          </template>
          <template v-if="fungsionalRows.length">
            <tr class="dinas-group-bar dinas-group-bar--fungsional">
              <td :colspan="bolehKelola ? 6 : 5">JABATAN FUNGSIONAL</td>
            </tr>
            <tr v-for="row in fungsionalRows" :key="`fun-${row.id_jabatan}`">
              <td>{{ row.jabatan }}</td>
              <td>
                <InputNumber
                  v-if="bolehKelola"
                  v-model="row.kj"
                  placeholder="-"
                  :min="1"
                  :max="17"
                  :disabled="kjSaving[row.id_jabatan]"
                  style="width: 5.5rem"
                  inputStyle="width: 2.5rem; padding: 0.25rem 0.4rem"
                  @update:modelValue="ubahKJ(row)"
                />
                <span v-else>{{ row.kj ?? '-' }}</span>
              </td>
              <td>
                <button v-if="bolehKelola" type="button" class="dinas-b-link" title="Lihat pegawai dengan jabatan ini" @click="openRosterDialog(row)">{{ row.b }}</button>
                <span v-else>{{ row.b }}</span>
              </td>
              <td>
                <InputNumber
                  v-if="bolehKelola"
                  v-model="row.k"
                  :min="0"
                  :disabled="kSaving[row.id_jabatan]"
                  style="width: 5.5rem"
                  inputStyle="width: 2.5rem; padding: 0.25rem 0.4rem"
                  @update:modelValue="ubahK(row)"
                />
                <span v-else>{{ row.k }}</span>
              </td>
              <td><Tag :value="signed(row.selisih)" :severity="selisihSeverity(row.selisih)" /></td>
              <td v-if="bolehKelola">
                <Button
                  icon="pi pi-trash"
                  size="small"
                  severity="danger"
                  outlined
                  title="Hapus Jabatan Ini dari Unit"
                  :loading="hapusJabatanSubmitting[row.id_jabatan]"
                  @click="konfirmasiHapusJabatan(row)"
                />
              </td>
            </tr>
            <tr class="dinas-group-subtotal">
              <td>Jumlah Kebutuhan JF</td>
              <td></td>
              <td>{{ fungsionalSum.b }}</td>
              <td>{{ fungsionalSum.k }}</td>
              <td><Tag :value="signed(fungsionalSum.b - fungsionalSum.k)" :severity="selisihSeverity(fungsionalSum.b - fungsionalSum.k)" /></td>
              <td v-if="bolehKelola"></td>
            </tr>
          </template>
        </tbody>
      </table>
      <div v-else style="font-size: 0.8rem; color: var(--p-text-muted-color); padding: 0.25rem 0">
        Belum ada data jabatan pada unit ini.
      </div>
    </div>

    <div v-if="node.anak.length" class="dinas-node-children">
      <PetaJabatanDinasNode
        v-for="anak in node.anak"
        :key="anak.id"
        :node="anak"
        :boleh-kelola="bolehKelola"
        :pegawai-options="pegawaiOptions"
        :pegawai-belum-ditempatkan="pegawaiBelumDitempatkan"
        :jabatan-options="jabatanOptions"
        @changed="$emit('changed')"
      />
    </div>

    <!-- dialog Tambah Unit Anak -->
    <Dialog v-model:visible="tambahDialog" modal header="Tambah Unit" style="width: 26rem; max-width: 95vw">
      <div class="field-label">Tipe</div>
      <Select v-model="tambahForm.tipe" :options="childTipeOptions" optionLabel="label" optionValue="value" style="width: 100%" />
      <div class="field-label" style="margin-top: 0.75rem">Nama Unit</div>
      <InputText v-model="tambahForm.unit" style="width: 100%" placeholder="mis. Bidang Pembinaan PAUD" @keyup.enter="simpanTambah" />
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="tambahDialog = false" />
        <Button label="Simpan" :loading="tambahSubmitting" @click="simpanTambah" />
      </template>
    </Dialog>

    <!-- dialog Ubah Nama Unit -->
    <Dialog v-model:visible="renameDialog" modal header="Ubah Nama Unit" style="width: 26rem; max-width: 95vw">
      <InputText v-model="renameValue" style="width: 100%" @keyup.enter="simpanRename" />
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="renameDialog = false" />
        <Button label="Simpan" :loading="renameSubmitting" @click="simpanRename" />
      </template>
    </Dialog>

    <!-- dialog Ubah Kepala Unit -->
    <Dialog v-model:visible="kepalaDialog" modal :header="`Ubah Kepala -- ${node.unit}`" style="width: 28rem; max-width: 95vw">
      <Select
        v-model="kepalaIdPegawai"
        :options="pegawaiSelectOptions"
        optionLabel="label"
        optionValue="value"
        filter
        showClear
        placeholder="Pilih pegawai (kosongkan untuk mengosongkan)"
        style="width: 100%"
      />
      <p style="font-size: 0.78rem; color: var(--p-text-muted-color); margin: 0.6rem 0 0">
        Seluruh pegawai yang saat ini bertugas di unit ini akan otomatis disamakan Atasan Langsungnya ke pegawai yang
        dipilih, dan kepala ini sendiri akan melapor ke kepala unit induknya.
      </p>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="kepalaDialog = false" />
        <Button label="Simpan" :loading="kepalaSubmitting" @click="simpanKepala" />
      </template>
    </Dialog>

    <!-- dialog Kelola Pegawai Unit Ini -->
    <Dialog v-model:visible="pegawaiDialog" modal :header="`Kelola Pegawai -- ${node.unit}`" style="width: 32rem; max-width: 95vw">
      <div class="field-label">Pegawai di unit ini</div>
      <div v-if="pegawaiDiUnitLoading" style="padding: 0.5rem 0; font-size: 0.82rem; color: var(--p-text-muted-color)">Memuat...</div>
      <div v-else-if="!pegawaiDiUnit.length" style="padding: 0.25rem 0 0.75rem; font-size: 0.82rem; color: var(--p-text-muted-color)">
        Belum ada pegawai yang ditempatkan di unit ini.
      </div>
      <ul v-else style="list-style: none; padding: 0; margin: 0.35rem 0 0.9rem; display: flex; flex-direction: column; gap: 0.4rem">
        <li v-for="p in pegawaiDiUnit" :key="p.id" style="display: flex; align-items: center; gap: 0.6rem">
          <div style="flex: 1; font-size: 0.82rem">
            <div style="font-weight: 600">{{ p.nama }}</div>
            <div style="color: var(--p-text-muted-color)">{{ p.nip }} -- {{ p.jabatan }}</div>
          </div>
          <Button
            icon="pi pi-sign-out"
            size="small"
            severity="danger"
            outlined
            title="Keluarkan dari unit ini"
            :loading="keluarkanSubmitting[p.id]"
            @click="keluarkanDariUnit(p)"
          />
        </li>
      </ul>
      <div class="field-label" style="margin-top: 0.5rem">Tambahkan pegawai (belum punya Bidang/Seksi)</div>
      <MultiSelect
        v-model="tambahPegawaiIds"
        :options="pegawaiBelumOptions"
        optionLabel="label"
        optionValue="value"
        filter
        display="chip"
        filterPlaceholder="Ketik nama / NIP"
        placeholder="Pilih satu atau beberapa pegawai"
        style="width: 100%; margin-top: 0.3rem"
      />
      <p v-if="!pegawaiBelumOptions.length" style="font-size: 0.78rem; color: var(--p-text-muted-color); margin: 0.5rem 0 0">
        Semua pegawai Dinas sudah ditempatkan ke salah satu unit struktur.
      </p>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="pegawaiDialog = false" />
        <Button
          :label="tambahPegawaiIds.length ? `Tambahkan (${tambahPegawaiIds.length})` : 'Tambahkan'"
          icon="pi pi-user-plus"
          :disabled="!tambahPegawaiIds.length"
          :loading="tambahPegawaiSubmitting"
          @click="tambahPegawaiKeUnit"
        />
      </template>
    </Dialog>

    <!-- dialog roster per-jabatan (klik kolom B) -->
    <Dialog v-model:visible="rosterDialog" modal :header="`Pegawai -- ${rosterRow?.jabatan || ''}`" style="width: 32rem; max-width: 95vw">
      <div class="field-label">Pegawai dengan jabatan ini di unit "{{ node.unit }}"</div>
      <div v-if="rosterLoading" style="padding: 0.5rem 0; font-size: 0.82rem; color: var(--p-text-muted-color)">Memuat...</div>
      <div v-else-if="!rosterPegawai.length" style="padding: 0.25rem 0 0.75rem; font-size: 0.82rem; color: var(--p-text-muted-color)">
        Belum ada pegawai dengan jabatan ini di unit ini.
      </div>
      <ul v-else style="list-style: none; padding: 0; margin: 0.35rem 0 0.9rem; display: flex; flex-direction: column; gap: 0.4rem">
        <li v-for="p in rosterPegawai" :key="p.id" style="display: flex; align-items: center; gap: 0.6rem">
          <div style="flex: 1; font-size: 0.82rem">
            <div style="font-weight: 600">{{ p.nama }}</div>
            <div style="color: var(--p-text-muted-color)">{{ p.nip }}</div>
          </div>
          <Button
            icon="pi pi-sign-out"
            size="small"
            severity="danger"
            outlined
            title="Keluarkan dari unit ini"
            :loading="rosterKeluarkanSubmitting[p.id]"
            @click="rosterKeluarkan(p)"
          />
        </li>
      </ul>
      <div class="field-label" style="margin-top: 0.5rem">Tambahkan pegawai dengan jabatan "{{ rosterRow?.jabatan }}"</div>
      <MultiSelect
        v-model="rosterTambahIds"
        :options="rosterTambahOptions"
        optionLabel="label"
        optionValue="value"
        filter
        display="chip"
        filterPlaceholder="Ketik nama / NIP"
        placeholder="Pilih satu atau beberapa pegawai"
        style="width: 100%; margin-top: 0.3rem"
      />
      <p v-if="!rosterTambahOptions.length" style="font-size: 0.78rem; color: var(--p-text-muted-color); margin: 0.5rem 0 0">
        Tidak ada pegawai belum ditempatkan dengan jabatan ini (perbandingan jabatan tidak memandang huruf besar/kecil).
      </p>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="rosterDialog = false" />
        <Button
          :label="rosterTambahIds.length ? `Tambahkan (${rosterTambahIds.length})` : 'Tambahkan'"
          icon="pi pi-user-plus"
          :disabled="!rosterTambahIds.length"
          :loading="rosterTambahSubmitting"
          @click="rosterTambahKeUnit"
        />
      </template>
    </Dialog>

    <!-- dialog Tambah Jabatan -->
    <Dialog v-model:visible="tambahJabatanDialog" modal :header="`Tambah Jabatan -- ${node.unit}`" style="width: 28rem; max-width: 95vw">
      <div class="field-label">Jabatan</div>
      <Select
        v-model="tambahJabatanForm.id_jabatan"
        :options="tambahJabatanOptions"
        optionLabel="label"
        optionValue="value"
        filter
        placeholder="Pilih jabatan"
        style="width: 100%"
      />
      <div class="field-label" style="margin-top: 0.75rem">Kebutuhan (K)</div>
      <InputNumber v-model="tambahJabatanForm.k" :min="0" style="width: 100%" inputStyle="width: 100%" />
      <p v-if="!tambahJabatanOptions.length" style="font-size: 0.78rem; color: var(--p-text-muted-color); margin: 0.6rem 0 0">
        Seluruh jabatan master data sudah ada pada unit ini.
      </p>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="tambahJabatanDialog = false" />
        <Button label="Simpan" :disabled="!tambahJabatanForm.id_jabatan" :loading="tambahJabatanSubmitting" @click="simpanTambahJabatan" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.dinas-node {
  border: 1px solid var(--p-content-border-color, #e2e8f0);
  border-radius: 10px;
  padding: 0.75rem 0.9rem;
  margin-bottom: 0.75rem;
  background: var(--p-content-background, #fff);
}
.dinas-node--dinas {
  border-width: 2px;
  border-color: #dc2626;
}
.dinas-node-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.6rem;
}
.dinas-node-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
}
.dinas-node-kepala {
  font-size: 0.82rem;
  color: var(--p-text-muted-color);
  flex: 1;
  min-width: 160px;
}
.dinas-node-children {
  margin-top: 0.6rem;
  margin-left: 1.5rem;
  padding-left: 0.9rem;
  border-left: 2px dashed var(--p-content-border-color, #e2e8f0);
}
.dinas-row-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}
.dinas-row-table th {
  text-align: left;
  font-weight: 600;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid var(--p-content-border-color, #e2e8f0);
}
.dinas-row-table td {
  padding: 0.3rem 0.5rem;
  border-bottom: 1px solid var(--p-content-border-color, #f1f5f9);
}
/* permintaan pengguna: bedakan posisi "JABATAN PELAKSANA" vs "JABATAN
   FUNGSIONAL", warna PERSIS sama dengan dinasGroupBarColor pada PDF &
   pelaksanaBarStyle/fungsionalBarStyle pada Excel (biru tua utk Pelaksana,
   abu-abu utk Fungsional), supaya ketiga tampilan tetap konsisten. */
.dinas-group-bar td {
  color: #fff;
  font-weight: 700;
  font-size: 0.78rem;
  letter-spacing: 0.02em;
  padding: 0.3rem 0.5rem;
}
.dinas-group-bar--pelaksana td {
  background: #266399;
}
.dinas-group-bar--fungsional td {
  background: #6b7280;
}
.dinas-group-subtotal td {
  font-weight: 700;
  background: var(--p-content-background-alt, #f8fafc);
}
.dinas-b-link {
  border: none;
  background: none;
  padding: 0;
  margin: 0;
  color: var(--p-primary-color, #2563eb);
  font-weight: 600;
  text-decoration: underline;
  cursor: pointer;
  font-size: inherit;
}
.field-label {
  font-weight: 600;
  font-size: 0.85rem;
  margin-bottom: 0.3rem;
}
</style>
