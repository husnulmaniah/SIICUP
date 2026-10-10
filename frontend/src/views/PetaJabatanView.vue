<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { useAuthStore } from '../stores/auth'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Textarea from 'primevue/textarea'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'

// PetaJabatanView -- menu "Peta Jabatan" (dropdown Administrasi Kepegawaian
// untuk administrator/admin, + menu langsung untuk pegawai & atasan). Lihat
// komentar alur lengkap di backend/models/models.go (bagian "PETA JABATAN --
// SEKOLAH") & backend/handlers/peta_jabatan*.go.
//
// Tab "Peta Jabatan Dinas" BELUM dikerjakan (menyusul kemudian, permintaan
// pengguna) -- ditampilkan nonaktif dengan badge "Segera Hadir". Seluruh
// komponen di bawah ini KHUSUS tab "Peta Jabatan Sekolah".
const toast = useToast()
const confirm = useConfirm()
const auth = useAuthStore()

function formatTanggal(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'long', day: 'numeric' })
}
function signed(n) {
  if (n > 0) return `+${n}`
  return `${n}`
}
function selisihSeverity(n) {
  return n < 0 ? 'danger' : n > 0 ? 'warn' : 'success'
}

// ============================================================
// pemilihan sekolah -- administrator/admin WAJIB memilih sekolah (dropdown,
// lewat GET /peta-jabatan/sekolah/daftar); atasan & pegawai otomatis
// memakai sekolah tempat mereka sendiri bertugas (lihat
// resolveUnitKerjaSekolahForRequest di backend), tanpa dropdown sama sekali.
// ============================================================
const daftarSekolah = ref([])
const selectedUnitKerja = ref(null)
const petaData = ref(null)
const petaLoading = ref(false)
const bolehKelola = computed(() => !!petaData.value?.boleh_kelola)
const currentUnitKerjaId = computed(() => (auth.canManageMaster ? selectedUnitKerja.value : petaData.value?.unit_kerja?.id))

async function loadDaftarSekolah() {
  try {
    const { data } = await http.get('/peta-jabatan/sekolah/daftar')
    daftarSekolah.value = data.data || []
    if (daftarSekolah.value.length && !selectedUnitKerja.value) {
      selectedUnitKerja.value = daftarSekolah.value[0].id
    } else if (selectedUnitKerja.value) {
      loadPeta()
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat daftar sekolah', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

async function loadPeta() {
  if (auth.canManageMaster && !selectedUnitKerja.value) {
    petaData.value = null
    return
  }
  petaLoading.value = true
  try {
    const params = {}
    if (auth.canManageMaster) params.id_unit_kerja = selectedUnitKerja.value
    const { data } = await http.get('/peta-jabatan/sekolah', { params })
    petaData.value = data.data
  } catch (e) {
    petaData.value = null
    toast.add({ severity: 'error', summary: 'Gagal memuat Peta Jabatan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    petaLoading.value = false
  }
}

watch(selectedUnitKerja, () => {
  if (auth.canManageMaster) loadPeta()
})

// ============================================================
// "Sinkronkan Data" -- permintaan pengguna: tarik ulang data Jabatan & nama
// pegawai yang ada di unit kerja pada menu Data Pegawai, lalu perbarui
// tabel Peta Jabatan supaya sesuai. "B" sebenarnya SUDAH SELALU dihitung
// langsung dari data pegawai setiap tabel ini dimuat (lihat
// hitungPetaJabatanSekolah di backend) -- tombol ini terutama memastikan
// SETIAP jabatan/sub-jabatan yang benar-benar dipegang pegawai sekolah ini
// langsung punya baris kebutuhan (K, default 0 kalau belum pernah diatur)
// supaya langsung terlihat & bisa diatur, TANPA menimpa K yang sudah diisi
// manual (lihat sinkronkanPetaJabatanSekolah di
// backend/handlers/peta_jabatan.go). HANYA untuk administrator/admin/
// atasan (sama seperti aksi Atur K/Kelola Sub-Jabatan lainnya).
const sinkronkanSubmitting = ref(false)
async function sinkronkanData() {
  sinkronkanSubmitting.value = true
  try {
    const params = {}
    if (auth.canManageMaster) params.id_unit_kerja = selectedUnitKerja.value
    const { data } = await http.post('/peta-jabatan/sekolah/sinkronkan', null, { params })
    petaData.value = data.data
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyinkronkan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    sinkronkanSubmitting.value = false
  }
}

// ============================================================
// "Cetak Excel" / "Cetak PDF" -- permintaan pengguna: administrator &
// atasan/Kepala Sekolah bisa mengunduh tabel Peta Jabatan Sekolah (Jabatan/
// Sub-Jabatan/B/K/+-) dalam bentuk Excel ATAU PDF, keduanya diset ukuran
// kertas Legal di backend (lihat handlers/peta_jabatan_export.go) supaya
// seluruh tabel tetap muat rapi. HANYA untuk yang bolehKelola (sama seperti
// tombol "Sinkronkan Data").
// ============================================================
const cetakExcelSubmitting = ref(false)
const cetakPdfSubmitting = ref(false)
async function unduhPetaJabatan(jenis) {
  const loadingRef = jenis === 'excel' ? cetakExcelSubmitting : cetakPdfSubmitting
  loadingRef.value = true
  try {
    const endpoint = jenis === 'excel' ? '/peta-jabatan/sekolah/export-excel' : '/peta-jabatan/sekolah/cetak-pdf'
    const params = {}
    if (auth.canManageMaster) params.id_unit_kerja = selectedUnitKerja.value
    const res = await http.get(endpoint, { params, responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = url
    const namaSekolah = (petaData.value?.unit_kerja?.unit || 'sekolah').replace(/[^a-zA-Z0-9]+/g, '_')
    link.download = `peta_jabatan_${namaSekolah}.${jenis === 'excel' ? 'xlsx' : 'pdf'}`
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loadingRef.value = false
  }
}

const expandedRows = ref({})

// ============================================================
// dialog "Kelola Sub-Jabatan" -- tambah/hapus pecahan SubJabatan untuk SATU
// Jabatan pada sekolah yang sedang dilihat.
// ============================================================
const kelolaSubDialog = ref(false)
const kelolaSubRow = ref(null)
const subJabatanItems = ref([])
const subJabatanLoading = ref(false)
const newSubNama = ref('')
const subJabatanSubmitting = ref(false)

async function loadSubJabatanItems() {
  if (!kelolaSubRow.value) return
  subJabatanLoading.value = true
  try {
    const { data } = await http.get('/peta-jabatan/sub-jabatan', {
      params: { id_unit_kerja: currentUnitKerjaId.value, id_jabatan: kelolaSubRow.value.id_jabatan },
    })
    subJabatanItems.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    subJabatanLoading.value = false
  }
}
// daftar pegawai pemegang Jabatan ini (ditarik dari data pegawai yang
// berhasil disinkronkan -- sama sumbernya dengan B) -- dipakai untuk
// menempatkan pegawai ke salah satu pecahan Sub-Jabatan yang baru dibuat
// (permintaan pengguna: "tambahkan inputan untuk menarik data pegawai...
// sesuai data B yang berhasil disinkronkan"). Dulu ini dialog "Kelola
// Penempatan" terpisah -- sekarang digabung langsung ke dialog "Kelola
// Sub-Jabatan" supaya begitu sub-jabatan baru dibuat, penempatannya bisa
// langsung diatur di tempat yang sama.
const kelolaSubPegawaiList = ref([])
const kelolaSubPegawaiLoading = ref(false)
async function loadKelolaSubPegawaiList() {
  if (!kelolaSubRow.value) return
  kelolaSubPegawaiLoading.value = true
  try {
    const { data } = await http.get('/pegawai', {
      params: { id_unit_kerja: currentUnitKerjaId.value, id_jabatan: kelolaSubRow.value.id_jabatan, pageSize: 200 },
    })
    kelolaSubPegawaiList.value = (Array.isArray(data.data) ? data.data : []).map((p) => ({
      id: p.id,
      nama: p.nama,
      nip: p.nip,
      id_sub_jabatan: p.id_sub_jabatan,
    }))
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    kelolaSubPegawaiLoading.value = false
  }
}
const subJabatanOptionsKelola = computed(() => [
  { label: '(Belum dikategorikan)', value: null },
  ...subJabatanItems.value.map((s) => ({ label: s.nama, value: s.id })),
])
async function ubahPenempatanSub(pg) {
  try {
    await http.put(`/peta-jabatan/pegawai/${pg.id}/sub-jabatan`, { id_sub_jabatan: pg.id_sub_jabatan })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `Penempatan ${pg.nama} disimpan`, life: 3000 })
    loadPeta()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
function openKelolaSub(row) {
  kelolaSubRow.value = row
  newSubNama.value = ''
  kelolaSubDialog.value = true
  loadSubJabatanItems()
  loadKelolaSubPegawaiList()
}
async function tambahSubJabatan() {
  const nama = newSubNama.value.trim()
  if (!nama) return
  subJabatanSubmitting.value = true
  try {
    await http.post('/peta-jabatan/sub-jabatan', {
      id_unit_kerja: currentUnitKerjaId.value,
      id_jabatan: kelolaSubRow.value.id_jabatan,
      nama,
    })
    newSubNama.value = ''
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Sub-jabatan ditambahkan', life: 3000 })
    await Promise.all([loadSubJabatanItems(), loadPeta()])
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menambah', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    subJabatanSubmitting.value = false
  }
}
function konfirmasiHapusSub(item) {
  confirm.require({
    message: `Hapus sub-jabatan "${item.nama}"? Pegawai yang sudah ditempatkan di sini TIDAK akan terhapus -- tetap terhitung penuh pada jabatan induknya.`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await http.delete(`/peta-jabatan/sub-jabatan/${item.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Sub-jabatan dihapus', life: 3000 })
        await Promise.all([loadSubJabatanItems(), loadKelolaSubPegawaiList(), loadPeta()])
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 5000 })
      }
    },
  })
}

// ============================================================
// dialog "Atur Kebutuhan (K)" -- satu Jabatan (tanpa pecahan) ATAU satu
// pecahan SubJabatan tertentu.
// ============================================================
const aturKDialog = ref(false)
const aturKTarget = ref({ id_jabatan: null, id_sub_jabatan: null, label: '', kebutuhan: 0 })
const aturKSubmitting = ref(false)
function openAturK(row, sub) {
  aturKTarget.value = {
    id_jabatan: row.id_jabatan,
    id_sub_jabatan: sub ? sub.id : null,
    label: sub ? `${row.jabatan} -- ${sub.nama}` : row.jabatan,
    kebutuhan: sub ? sub.k : row.k,
  }
  aturKDialog.value = true
}
async function simpanAturK() {
  aturKSubmitting.value = true
  try {
    await http.put('/peta-jabatan/formasi', {
      id_unit_kerja: currentUnitKerjaId.value,
      id_jabatan: aturKTarget.value.id_jabatan,
      id_sub_jabatan: aturKTarget.value.id_sub_jabatan,
      kebutuhan: aturKTarget.value.kebutuhan,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Kebutuhan (K) disimpan', life: 3000 })
    aturKDialog.value = false
    loadPeta()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    aturKSubmitting.value = false
  }
}

// ============================================================
// modal "Daftar Pegawai" -- permintaan pengguna: klik nilai B atau K pada
// baris Jabatan (tabel utama) ATAU baris Sub-Jabatan (panel expand)
// menampilkan nama & NIP pegawai yang masuk hitungan baris tersebut.
// Menggunakan GET /pegawai yang sama (sudah otomatis membatasi hasil sesuai
// role pemanggil -- atasan hanya bawahannya, pegawai hanya dirinya sendiri,
// administrator/admin seluruhnya), jadi tidak perlu endpoint baru di
// backend. Kalau sub diisi (klik baris sub-jabatan), hasil disaring lagi di
// sisi frontend supaya hanya pegawai yang sudah ditempatkan ke sub-jabatan
// itu yang tampil; kalau tidak (klik baris Jabatan induk), seluruh pegawai
// pemegang jabatan tersebut tampil (termasuk yang sudah dipecah ke
// sub-jabatan manapun).
// ============================================================
const pegawaiListDialog = ref(false)
const pegawaiListTitle = ref('')
const pegawaiListLoading = ref(false)
const pegawaiListItems = ref([])
async function openPegawaiList(row, sub) {
  pegawaiListTitle.value = sub ? `${row.jabatan} -- ${sub.nama}` : row.jabatan
  pegawaiListDialog.value = true
  pegawaiListLoading.value = true
  try {
    const { data } = await http.get('/pegawai', {
      params: { id_unit_kerja: currentUnitKerjaId.value, id_jabatan: row.id_jabatan, pageSize: 200 },
    })
    let items = Array.isArray(data.data) ? data.data : []
    if (sub) {
      items = items.filter((p) => p.id_sub_jabatan === sub.id)
    }
    pegawaiListItems.value = items.map((p) => ({ id: p.id, nama: p.nama, nip: p.nip }))
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    pegawaiListLoading.value = false
  }
}

// ============================================================
// preview berkas umum (UKOM, SK, cetak PDF) -- satu dialog dipakai bersama
// seperti pola PerubahanDataView.vue/PengajuanPensiunView.vue.
// ============================================================
const previewDialog = ref(false)
const previewTitle = ref('')
const previewType = ref('pdf')
const previewUrl = ref('')
function closePreview() {
  if (previewUrl.value) window.URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
}
function extensionOf(name) {
  return (name || '').split('.').pop().toLowerCase()
}

// ============================================================
// referensi Jabatan (dropdown "Jabatan Tujuan") & Pangkat/Golongan
// (dropdown opsional administrator saat approve Perubahan Jabatan).
// ============================================================
const refJabatan = ref([])
async function loadRefJabatan() {
  try {
    const { data } = await http.get('/ref/jabatan')
    refJabatan.value = data.data || []
  } catch {
    // diamkan -- dropdown tetap kosong, tidak mengganggu halaman
  }
}
const jabatanOptions = computed(() => refJabatan.value.map((j) => ({ label: j.jabatan, value: j.id })))

const refPangkatGol = ref([])
async function loadRefPangkatGol() {
  try {
    const { data } = await http.get('/ref/pangkat-gol')
    refPangkatGol.value = data.data || []
  } catch {
    // diamkan
  }
}
const pangkatGolOptions = computed(() =>
  refPangkatGol.value.map((pg) => ({ label: `${pg.pangkat?.pangkat || '-'} / ${pg.gol?.gol || '-'}`, value: pg.id })),
)

// ============================================================
// "Ajukan Kenaikan Pangkat" (pegawai/atasan, untuk diri sendiri)
// ============================================================
const ajukanKPDialog = ref(false)
const ajukanKPForm = ref({ id_jabatan_tujuan: null, alasan: '' })
const ajukanKPFile = ref(null)
const ajukanKPFileInput = ref(null)
const ajukanKPSubmitting = ref(false)
function pickAjukanKPFile() {
  ajukanKPFileInput.value?.click()
}
function onAjukanKPFileChosen(e) {
  const file = e.target.files?.[0] || null
  if (file) {
    const ext = extensionOf(file.name)
    if (!['pdf', 'jpg', 'jpeg', 'png'].includes(ext)) {
      toast.add({ severity: 'error', summary: 'Format tidak didukung', detail: 'Berkas harus PDF, JPG, atau PNG.', life: 6000 })
      e.target.value = ''
      ajukanKPFile.value = null
      return
    }
  }
  ajukanKPFile.value = file
}
function openAjukanKP() {
  ajukanKPForm.value = { id_jabatan_tujuan: null, alasan: '' }
  ajukanKPFile.value = null
  if (refJabatan.value.length === 0) loadRefJabatan()
  ajukanKPDialog.value = true
}
async function submitAjukanKP() {
  if (!ajukanKPForm.value.id_jabatan_tujuan) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih jabatan tujuan', life: 4000 })
    return
  }
  if (!ajukanKPFile.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Bukti lulus UKOM wajib diupload', life: 4000 })
    return
  }
  ajukanKPSubmitting.value = true
  try {
    const fd = new FormData()
    fd.append('id_jabatan_tujuan', ajukanKPForm.value.id_jabatan_tujuan)
    fd.append('alasan', ajukanKPForm.value.alasan.trim())
    fd.append('file', ajukanKPFile.value)
    const { data } = await http.post('/peta-jabatan/kenaikan-pangkat', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
    ajukanKPDialog.value = false
    loadKenaikanPangkat()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengajukan', detail: e.response?.data?.message || e.message, life: 6000 })
  } finally {
    ajukanKPSubmitting.value = false
  }
}

// ============================================================
// daftar & detail Kenaikan Pangkat -- SATU tabel yang sama untuk SEMUA role
// (dibatasi otomatis sesuai role oleh backend, lihat listKenaikanPangkat):
// pegawai hanya lihat pengajuannya sendiri, atasan lihat seluruh sekolahnya,
// administrator/admin lihat lintas sekolah.
// ============================================================
const kpItems = ref([])
const kpLoading = ref(false)
async function loadKenaikanPangkat() {
  kpLoading.value = true
  try {
    const { data } = await http.get('/peta-jabatan/kenaikan-pangkat')
    kpItems.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    kpLoading.value = false
  }
}
function kpStatusLabel(s) {
  return (
    {
      menunggu_atasan: 'Menunggu Atasan',
      menunggu_admin: 'Menunggu Administrator',
      disetujui: 'Disetujui',
      ditolak_atasan: 'Dikembalikan Atasan',
      ditolak_admin: 'Dikembalikan Administrator',
    }[s] || s
  )
}
function kpStatusSeverity(s) {
  return (
    {
      menunggu_atasan: 'warn',
      menunggu_admin: 'warn',
      disetujui: 'success',
      ditolak_atasan: 'danger',
      ditolak_admin: 'danger',
    }[s] || 'secondary'
  )
}

const kpDetailDialog = ref(false)
const kpDetailRow = ref(null)
const kpCatatan = ref('')
const kpSubTujuanOptions = ref([])
const kpSubTujuanWajib = ref(false)
const kpIdSubTujuan = ref(null)
const kpProcessing = ref(false)

const kpBisaDibatalkan = computed(
  () => kpDetailRow.value?.status === 'menunggu_atasan' && auth.idPegawai != null && auth.idPegawai === kpDetailRow.value?.id_pegawai,
)
const kpBisaDiprosesAtasan = computed(() => kpDetailRow.value?.status === 'menunggu_atasan' && auth.isAtasan)
const kpBisaDiprosesAdmin = computed(() => kpDetailRow.value?.status === 'menunggu_admin' && auth.canManageMaster)

async function openKpDetail(row) {
  kpDetailRow.value = row
  kpCatatan.value = ''
  kpIdSubTujuan.value = null
  kpSubTujuanOptions.value = []
  kpSubTujuanWajib.value = false
  kpDetailDialog.value = true
  if (row.status === 'menunggu_atasan' && auth.isAtasan) {
    try {
      const { data } = await http.get('/peta-jabatan/sub-jabatan', {
        params: { id_unit_kerja: row.id_unit_kerja, id_jabatan: row.id_jabatan_tujuan },
      })
      kpSubTujuanOptions.value = data.data || []
      kpSubTujuanWajib.value = kpSubTujuanOptions.value.length > 0
    } catch {
      // diamkan -- tetap boleh menyetujui tanpa pecahan kalau memang belum ada
    }
  }
}
function kpConfirmBatalkan() {
  confirm.require({
    message: 'Batalkan pengajuan kenaikan pangkat ini?',
    header: 'Konfirmasi Batalkan',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Batalkan',
    rejectLabel: 'Tidak',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      kpProcessing.value = true
      try {
        await http.delete(`/peta-jabatan/kenaikan-pangkat/${kpDetailRow.value.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan dibatalkan', life: 4000 })
        kpDetailDialog.value = false
        loadKenaikanPangkat()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      } finally {
        kpProcessing.value = false
      }
    },
  })
}
async function kpSetujuiAtasan() {
  if (kpSubTujuanWajib.value && !kpIdSubTujuan.value) {
    toast.add({
      severity: 'warn',
      summary: 'Belum lengkap',
      detail: 'Jabatan tujuan sudah dipecah menjadi beberapa sub-jabatan di sekolah ini, pilih salah satu sub-jabatan tujuan',
      life: 6000,
    })
    return
  }
  kpProcessing.value = true
  try {
    const body = new URLSearchParams()
    if (kpIdSubTujuan.value) body.append('id_sub_jabatan_tujuan', kpIdSubTujuan.value)
    body.append('catatan', kpCatatan.value || '')
    await http.put(`/peta-jabatan/kenaikan-pangkat/${kpDetailRow.value.id}/setujui-atasan`, body)
    toast.add({ severity: 'success', summary: 'Disetujui', detail: 'Kebutuhan (K) jabatan tujuan bertambah, diteruskan ke administrator', life: 7000 })
    kpDetailDialog.value = false
    loadKenaikanPangkat()
    loadPeta()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyetujui', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    kpProcessing.value = false
  }
}
async function kpKembalikanAtasan() {
  if (!kpCatatan.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Catatan wajib diisi', detail: 'Isi catatan supaya pegawai tahu apa yang perlu diperbaiki', life: 4000 })
    return
  }
  kpProcessing.value = true
  try {
    const body = new URLSearchParams({ catatan: kpCatatan.value })
    await http.put(`/peta-jabatan/kenaikan-pangkat/${kpDetailRow.value.id}/kembalikan-atasan`, body)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan dikembalikan', life: 4000 })
    kpDetailDialog.value = false
    loadKenaikanPangkat()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    kpProcessing.value = false
  }
}
async function kpSetujuiAdmin() {
  kpProcessing.value = true
  try {
    const body = new URLSearchParams({ catatan: kpCatatan.value || '' })
    await http.put(`/peta-jabatan/kenaikan-pangkat/${kpDetailRow.value.id}/setujui-admin`, body)
    toast.add({
      severity: 'success',
      summary: 'Disetujui',
      detail: 'Dokumen final siap dicetak. Peringatan Perubahan Jabatan (wajib upload SK) sudah muncul pada akun pegawai',
      life: 8000,
    })
    kpDetailDialog.value = false
    loadKenaikanPangkat()
    if (auth.canManageMaster) loadPerubahanJabatan()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyetujui', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    kpProcessing.value = false
  }
}
async function kpKembalikanAdmin() {
  if (!kpCatatan.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Catatan wajib diisi', detail: 'Isi catatan supaya diketahui apa yang perlu diperbaiki', life: 4000 })
    return
  }
  kpProcessing.value = true
  try {
    const body = new URLSearchParams({ catatan: kpCatatan.value })
    await http.put(`/peta-jabatan/kenaikan-pangkat/${kpDetailRow.value.id}/kembalikan-admin`, body)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan dikembalikan', life: 4000 })
    kpDetailDialog.value = false
    loadKenaikanPangkat()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    kpProcessing.value = false
  }
}
async function previewUkom(row) {
  try {
    const res = await http.get(`/peta-jabatan/kenaikan-pangkat/${row.id}/ukom`, { params: { inline: 1 }, responseType: 'blob' })
    previewType.value = ['jpg', 'jpeg', 'png'].includes(extensionOf(row.ukom_nama_file)) ? 'image' : 'pdf'
    previewTitle.value = 'Bukti Lulus UKOM'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
async function previewCetakKP(row) {
  try {
    const res = await http.get(`/peta-jabatan/kenaikan-pangkat/${row.id}/cetak`, { params: { inline: 1 }, responseType: 'blob' })
    previewType.value = 'pdf'
    previewTitle.value = 'Dokumen Peta Jabatan -- Kenaikan Pangkat'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
async function unduhCetakKP(row) {
  try {
    const res = await http.get(`/peta-jabatan/kenaikan-pangkat/${row.id}/cetak`, { responseType: 'blob' })
    const url = window.URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `peta_jabatan_kenaikan_pangkat_${row.id}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

// ============================================================
// "Perubahan Jabatan" (peringatan wajib upload SK) -- sisi pegawai/atasan
// (hanya milik sendiri, GET .../saya) & sisi administrator/admin (lintas
// sekolah, GET /peta-jabatan/perubahan).
// ============================================================
const perubahanSaya = ref(null)
async function loadPerubahanSaya() {
  try {
    const { data } = await http.get('/peta-jabatan/perubahan/saya')
    perubahanSaya.value = data.data
  } catch {
    perubahanSaya.value = null
  }
}

const uploadPerubahanDialog = ref(false)
const uploadPerubahanFile = ref(null)
const uploadPerubahanFileInput = ref(null)
const uploadPerubahanSubmitting = ref(false)
function pickUploadPerubahanFile() {
  uploadPerubahanFileInput.value?.click()
}
function onUploadPerubahanFileChosen(e) {
  const file = e.target.files?.[0] || null
  if (file) {
    const ext = extensionOf(file.name)
    if (!['pdf', 'jpg', 'jpeg', 'png'].includes(ext)) {
      toast.add({ severity: 'error', summary: 'Format tidak didukung', detail: 'Berkas harus PDF, JPG, atau PNG.', life: 6000 })
      e.target.value = ''
      uploadPerubahanFile.value = null
      return
    }
  }
  uploadPerubahanFile.value = file
}
function openUploadPerubahan() {
  uploadPerubahanFile.value = null
  uploadPerubahanDialog.value = true
}
async function submitUploadPerubahan() {
  if (!uploadPerubahanFile.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Berkas SK wajib diupload', life: 4000 })
    return
  }
  uploadPerubahanSubmitting.value = true
  try {
    const fd = new FormData()
    fd.append('file', uploadPerubahanFile.value)
    await http.post('/peta-jabatan/perubahan/saya/upload', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'SK berhasil diupload, menunggu persetujuan administrator', life: 6000 })
    uploadPerubahanDialog.value = false
    loadPerubahanSaya()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengupload', detail: e.response?.data?.message || e.message, life: 6000 })
  } finally {
    uploadPerubahanSubmitting.value = false
  }
}

const perubahanItems = ref([])
const perubahanLoading = ref(false)
async function loadPerubahanJabatan() {
  perubahanLoading.value = true
  try {
    const { data } = await http.get('/peta-jabatan/perubahan')
    perubahanItems.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    perubahanLoading.value = false
  }
}
function perubahanStatusLabel(s) {
  return (
    { menunggu_upload: 'Menunggu Upload SK', menunggu_admin: 'Menunggu Persetujuan', disetujui: 'Disetujui', ditolak: 'Ditolak' }[s] || s
  )
}
function perubahanStatusSeverity(s) {
  return { menunggu_upload: 'warn', menunggu_admin: 'info', disetujui: 'success', ditolak: 'danger' }[s] || 'secondary'
}

const perubahanDetailDialog = ref(false)
const perubahanDetailRow = ref(null)
const perubahanCatatan = ref('')
const perubahanIdPangkatGolBaru = ref(null)
const perubahanProcessing = ref(false)
function openPerubahanDetail(row) {
  perubahanDetailRow.value = row
  perubahanCatatan.value = ''
  perubahanIdPangkatGolBaru.value = null
  if (refPangkatGol.value.length === 0) loadRefPangkatGol()
  perubahanDetailDialog.value = true
}
function perubahanConfirmApprove() {
  confirm.require({
    message: 'Setujui & terapkan perubahan jabatan ini? Jabatan/sub-jabatan (dan pangkat/golongan, jika dipilih) pegawai akan langsung diperbarui.',
    header: 'Konfirmasi Setujui',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Setujui',
    rejectLabel: 'Batal',
    accept: async () => {
      perubahanProcessing.value = true
      try {
        await http.put(`/peta-jabatan/perubahan/${perubahanDetailRow.value.id}/approve`, {
          id_pangkat_gol_baru: perubahanIdPangkatGolBaru.value,
        })
        toast.add({ severity: 'success', summary: 'Disetujui', detail: 'Perubahan jabatan diterapkan ke data pegawai', life: 6000 })
        perubahanDetailDialog.value = false
        loadPerubahanJabatan()
        loadPeta()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menyetujui', detail: e.response?.data?.message || e.message, life: 5000 })
      } finally {
        perubahanProcessing.value = false
      }
    },
  })
}
async function perubahanReject() {
  if (!perubahanCatatan.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Catatan wajib diisi', detail: 'Isi catatan supaya pegawai tahu apa yang perlu diperbaiki', life: 4000 })
    return
  }
  perubahanProcessing.value = true
  try {
    await http.put(`/peta-jabatan/perubahan/${perubahanDetailRow.value.id}/reject`, { catatan: perubahanCatatan.value })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'SK dikembalikan ke pegawai untuk diupload ulang', life: 5000 })
    perubahanDetailDialog.value = false
    loadPerubahanJabatan()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    perubahanProcessing.value = false
  }
}
async function previewSkPerubahan(row) {
  try {
    const res = await http.get(`/peta-jabatan/perubahan/${row.id}/sk`, { params: { inline: 1 }, responseType: 'blob' })
    previewType.value = ['jpg', 'jpeg', 'png'].includes(extensionOf(row.sk_nama_file)) ? 'image' : 'pdf'
    previewTitle.value = 'Berkas SK Jabatan/Pangkat Baru'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

onMounted(() => {
  if (auth.canManageMaster) {
    loadDaftarSekolah()
  } else {
    loadPeta()
  }
  loadKenaikanPangkat()
  loadRefJabatan()
  if (auth.canManageMaster) {
    loadPerubahanJabatan()
    loadRefPangkatGol()
  }
  if (auth.isPegawai || auth.isAtasan) {
    loadPerubahanSaya()
  }
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Peta Jabatan</div>
    <p class="page-subtitle">
      Bandingkan B (Bezetting, jumlah pegawai saat ini) dengan K (Kebutuhan, formasi yang dibutuhkan) per jabatan, kelola
      pecahan sub-jabatan, dan proses pengajuan kenaikan pangkat pegawai sekolah -- dari lulus UKOM, persetujuan atasan
      &amp; administrator, hingga perubahan jabatan resmi setelah SK baru diupload.
    </p>

    <Message v-if="perubahanSaya" severity="warn" :closable="false" style="margin-bottom: 1rem">
      <div style="display: flex; justify-content: space-between; align-items: center; gap: 0.75rem; flex-wrap: wrap">
        <div>
          <strong>Perubahan Jabatan menunggu ditindaklanjuti.</strong>
          <template v-if="perubahanSaya.status === 'menunggu_upload'">
            Kenaikan pangkat anda sudah disetujui penuh -- upload SK jabatan/pangkat baru anda untuk menyelesaikannya.
            <template v-if="perubahanSaya.catatan_admin">Catatan administrator: "{{ perubahanSaya.catatan_admin }}"</template>
          </template>
          <template v-else>SK yang anda upload sedang menunggu persetujuan administrator.</template>
        </div>
        <Button
          v-if="perubahanSaya.status === 'menunggu_upload'"
          label="Upload SK"
          icon="pi pi-upload"
          size="small"
          @click="openUploadPerubahan"
        />
      </div>
    </Message>

    <Tabs value="sekolah">
      <TabList>
        <Tab value="dinas" disabled>
          Peta Jabatan Dinas
          <Tag value="Segera Hadir" severity="secondary" style="margin-left: 0.4rem" />
        </Tab>
        <Tab value="sekolah"><i class="pi pi-sitemap" style="margin-right: 0.4rem"></i> Peta Jabatan Sekolah</Tab>
      </TabList>
      <TabPanels>
        <TabPanel value="dinas">
          <Message severity="info" :closable="false">
            Peta Jabatan Dinas (untuk pegawai bertempat tugas Dinas/Kantor) akan hadir menyusul setelah Peta Jabatan Sekolah
            selesai.
          </Message>
        </TabPanel>

        <TabPanel value="sekolah">
          <div class="card">
            <div class="tab-toolbar">
              <div v-if="auth.canManageMaster" style="min-width: 240px; flex: 1">
                <Select
                  v-model="selectedUnitKerja"
                  :options="daftarSekolah"
                  optionLabel="unit"
                  optionValue="id"
                  filter
                  placeholder="Pilih sekolah"
                  style="width: 100%; max-width: 420px"
                />
              </div>
              <div v-else-if="petaData?.unit_kerja" style="font-weight: 600; flex: 1">
                <i class="pi pi-building" style="margin-right: 0.4rem"></i>{{ petaData.unit_kerja.unit }}
              </div>
              <Button
                v-if="bolehKelola"
                label="Sinkronkan Data"
                icon="pi pi-sync"
                size="small"
                severity="secondary"
                outlined
                :loading="sinkronkanSubmitting"
                title="Tarik ulang data Jabatan & nama pegawai dari Data Pegawai, perbarui Peta Jabatan sesuai data terkini"
                @click="sinkronkanData"
              />
              <Button
                v-if="bolehKelola"
                label="Cetak Excel"
                icon="pi pi-file-excel"
                size="small"
                severity="success"
                outlined
                :loading="cetakExcelSubmitting"
                title="Unduh tabel Peta Jabatan sebagai Excel (kertas Legal)"
                @click="unduhPetaJabatan('excel')"
              />
              <Button
                v-if="bolehKelola"
                label="Cetak PDF"
                icon="pi pi-file-pdf"
                size="small"
                severity="danger"
                outlined
                :loading="cetakPdfSubmitting"
                title="Unduh tabel Peta Jabatan sebagai PDF (kertas Legal)"
                @click="unduhPetaJabatan('pdf')"
              />
              <Button
                v-if="auth.isPegawai || auth.isAtasan"
                label="Ajukan Kenaikan Pangkat"
                icon="pi pi-arrow-up"
                size="small"
                @click="openAjukanKP"
              />
            </div>

            <div v-if="petaLoading" style="display: flex; justify-content: center; padding: 2rem">
              <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
            </div>
            <Message v-else-if="!petaData" severity="info" :closable="false">
              {{ auth.canManageMaster ? 'Pilih sekolah untuk melihat Peta Jabatan.' : 'Data Peta Jabatan belum tersedia.' }}
            </Message>
            <template v-else>
              <div class="responsive-table-wrap">
                <DataTable
                  :value="petaData.rows"
                  v-model:expandedRows="expandedRows"
                  dataKey="id_jabatan"
                  stripedRows
                  size="small"
                  style="min-width: 640px"
                >
                  <template #empty>
                    <div style="padding: 1.5rem; text-align: center; color: var(--p-text-muted-color)">
                      Belum ada data jabatan/pegawai pada sekolah ini.
                    </div>
                  </template>
                  <Column expander style="width: 3rem" />
                  <Column field="jabatan" header="Jabatan" />
                  <Column header="B" style="width: 80px">
                    <template #body="{ data }">
                      <span class="clickable-count" title="Lihat daftar pegawai" @click="openPegawaiList(data, null)">{{ data.b }}</span>
                    </template>
                  </Column>
                  <Column header="K" style="width: 80px">
                    <template #body="{ data }">
                      <span class="clickable-count" title="Lihat daftar pegawai" @click="openPegawaiList(data, null)">{{ data.k }}</span>
                    </template>
                  </Column>
                  <Column header="+/-" style="width: 90px">
                    <template #body="{ data }">
                      <Tag :value="signed(data.selisih)" :severity="selisihSeverity(data.selisih)" />
                    </template>
                  </Column>
                  <Column v-if="bolehKelola" header="Aksi" style="width: 230px">
                    <template #body="{ data }">
                      <div class="table-actions">
                        <Button label="Sub-Jabatan" icon="pi pi-sitemap" size="small" severity="secondary" outlined @click="openKelolaSub(data)" />
                        <Button
                          v-if="!data.punya_sub_jabatan"
                          label="Atur K"
                          icon="pi pi-pencil"
                          size="small"
                          severity="secondary"
                          outlined
                          @click="openAturK(data, null)"
                        />
                      </div>
                    </template>
                  </Column>
                  <template #expansion="{ data }">
                    <div style="padding: 0.5rem 0.5rem 0.75rem 3rem">
                      <div v-if="!data.sub_jabatan.length" style="color: var(--p-text-muted-color); font-size: 0.85rem">
                        Jabatan ini belum dipecah menjadi sub-jabatan.
                      </div>
                      <table v-else class="sub-jabatan-table">
                        <thead>
                          <tr>
                            <th>Sub-Jabatan</th>
                            <th style="width: 70px">B</th>
                            <th style="width: 70px">K</th>
                            <th style="width: 80px">+/-</th>
                            <th v-if="bolehKelola" style="width: 110px">Aksi</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr v-for="s in data.sub_jabatan" :key="s.id">
                            <td>{{ s.nama }}</td>
                            <td><span class="clickable-count" title="Lihat daftar pegawai" @click="openPegawaiList(data, s)">{{ s.b }}</span></td>
                            <td><span class="clickable-count" title="Lihat daftar pegawai" @click="openPegawaiList(data, s)">{{ s.k }}</span></td>
                            <td><Tag :value="signed(s.selisih)" :severity="selisihSeverity(s.selisih)" /></td>
                            <td v-if="bolehKelola">
                              <Button label="Atur K" icon="pi pi-pencil" size="small" severity="secondary" outlined text @click="openAturK(data, s)" />
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </template>
                </DataTable>
              </div>
            </template>
          </div>

          <div class="card" style="margin-top: 1.25rem">
            <h4 style="margin: 0 0 0.75rem 0">Kenaikan Pangkat</h4>
            <div class="responsive-table-wrap">
              <DataTable :value="kpItems" :loading="kpLoading" dataKey="id" stripedRows size="small" style="min-width: 640px">
                <template #empty>
                  <div style="padding: 1.5rem; text-align: center; color: var(--p-text-muted-color)">Belum ada pengajuan</div>
                </template>
                <Column header="Pegawai">
                  <template #body="{ data }">
                    <div style="font-weight: 600">{{ data.pegawai?.nama || '-' }}</div>
                    <div style="font-size: 0.78rem; color: var(--p-text-muted-color)">{{ data.pegawai?.nip || '-' }} &middot; {{ data.unit_kerja?.unit || '-' }}</div>
                  </template>
                </Column>
                <Column header="Jabatan">
                  <template #body="{ data }">
                    {{ data.jabatan_asal?.jabatan || '-' }}<i class="pi pi-arrow-right" style="margin: 0 0.4rem; font-size: 0.75rem"></i>{{ data.jabatan_tujuan?.jabatan || '-' }}
                  </template>
                </Column>
                <Column header="Tanggal" style="width: 150px">
                  <template #body="{ data }">{{ formatTanggal(data.created_at) }}</template>
                </Column>
                <Column header="Status" style="width: 170px">
                  <template #body="{ data }"><Tag :value="kpStatusLabel(data.status)" :severity="kpStatusSeverity(data.status)" /></template>
                </Column>
                <Column header="Aksi" style="width: 90px">
                  <template #body="{ data }">
                    <Button icon="pi pi-eye" size="small" severity="info" rounded text @click="openKpDetail(data)" />
                  </template>
                </Column>
              </DataTable>
            </div>
          </div>

          <div v-if="auth.canManageMaster" class="card" style="margin-top: 1.25rem">
            <h4 style="margin: 0 0 0.75rem 0">Perubahan Jabatan (Upload SK)</h4>
            <div class="responsive-table-wrap">
              <DataTable :value="perubahanItems" :loading="perubahanLoading" dataKey="id" stripedRows size="small" style="min-width: 640px">
                <template #empty>
                  <div style="padding: 1.5rem; text-align: center; color: var(--p-text-muted-color)">Belum ada data</div>
                </template>
                <Column header="Pegawai">
                  <template #body="{ data }">
                    <div style="font-weight: 600">{{ data.pegawai?.nama || '-' }}</div>
                    <div style="font-size: 0.78rem; color: var(--p-text-muted-color)">{{ data.pegawai?.nip || '-' }} &middot; {{ data.pegawai?.unit_kerja?.unit || '-' }}</div>
                  </template>
                </Column>
                <Column header="Jabatan Baru">
                  <template #body="{ data }">
                    {{ data.jabatan_baru?.jabatan || '-' }}<template v-if="data.sub_jabatan_baru"> ({{ data.sub_jabatan_baru.nama }})</template>
                  </template>
                </Column>
                <Column header="Tanggal" style="width: 150px">
                  <template #body="{ data }">{{ formatTanggal(data.created_at) }}</template>
                </Column>
                <Column header="Status" style="width: 170px">
                  <template #body="{ data }"><Tag :value="perubahanStatusLabel(data.status)" :severity="perubahanStatusSeverity(data.status)" /></template>
                </Column>
                <Column header="Aksi" style="width: 90px">
                  <template #body="{ data }">
                    <Button icon="pi pi-eye" size="small" severity="info" rounded text @click="openPerubahanDetail(data)" />
                  </template>
                </Column>
              </DataTable>
            </div>
          </div>
        </TabPanel>
      </TabPanels>
    </Tabs>

    <!-- dialog "Kelola Sub-Jabatan" -- sekaligus menempatkan pegawai ke
         sub-jabatan yang dibuat (permintaan pengguna, lihat
         loadKelolaSubPegawaiList di atas) -->
    <Dialog v-model:visible="kelolaSubDialog" modal :header="`Kelola Sub-Jabatan -- ${kelolaSubRow?.jabatan || ''}`" style="width: 36rem; max-width: 95vw">
      <div style="display: flex; gap: 0.5rem; margin-bottom: 1rem">
        <InputText v-model="newSubNama" placeholder="Nama sub-jabatan baru, mis. Guru Kelas" style="flex: 1" @keyup.enter="tambahSubJabatan" />
        <Button label="Tambah" icon="pi pi-plus" :loading="subJabatanSubmitting" @click="tambahSubJabatan" />
      </div>
      <div v-if="subJabatanLoading" style="display: flex; justify-content: center; padding: 1rem">
        <ProgressSpinner style="width: 2rem; height: 2rem" />
      </div>
      <Message v-else-if="!subJabatanItems.length" severity="info" :closable="false">Belum ada sub-jabatan untuk jabatan ini.</Message>
      <ul v-else class="sub-jabatan-list">
        <li v-for="item in subJabatanItems" :key="item.id">
          <span>{{ item.nama }}</span>
          <Button icon="pi pi-trash" size="small" severity="danger" text rounded @click="konfirmasiHapusSub(item)" />
        </li>
      </ul>

      <template v-if="subJabatanItems.length">
        <div class="field-label" style="margin-top: 1.25rem">Tempatkan Pegawai ke Sub-Jabatan</div>
        <p style="font-size: 0.78rem; color: var(--p-text-muted-color); margin: 0.2rem 0 0.75rem 0">
          Daftar pegawai berikut ditarik dari data pegawai yang sudah disinkronkan (B) pada jabatan ini. Pilih sub-jabatan
          untuk masing-masing pegawai.
        </p>
        <div v-if="kelolaSubPegawaiLoading" style="display: flex; justify-content: center; padding: 1rem">
          <ProgressSpinner style="width: 2rem; height: 2rem" />
        </div>
        <Message v-else-if="!kelolaSubPegawaiList.length" severity="info" :closable="false">Belum ada pegawai pada jabatan ini.</Message>
        <div v-else class="pegawai-sub-list">
          <div v-for="pg in kelolaSubPegawaiList" :key="pg.id" class="pegawai-sub-item">
            <div style="flex: 1; min-width: 0">
              <div style="font-weight: 600; font-size: 0.85rem">{{ pg.nama }}</div>
              <div style="font-size: 0.75rem; color: var(--p-text-muted-color)">{{ pg.nip }}</div>
            </div>
            <Select
              v-model="pg.id_sub_jabatan"
              :options="subJabatanOptionsKelola"
              optionLabel="label"
              optionValue="value"
              style="width: 210px"
              @update:modelValue="ubahPenempatanSub(pg)"
            />
          </div>
        </div>
      </template>

      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="kelolaSubDialog = false" />
      </template>
    </Dialog>

    <!-- dialog "Atur Kebutuhan (K)" -->
    <Dialog v-model:visible="aturKDialog" modal header="Atur Kebutuhan (K)" style="width: 26rem; max-width: 95vw">
      <div class="field-label-wrap">
        <label class="field-label">{{ aturKTarget.label }}</label>
        <InputNumber v-model="aturKTarget.kebutuhan" :min="0" showButtons style="width: 100%" fluid />
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="aturKDialog = false" />
        <Button label="Simpan" icon="pi pi-save" :loading="aturKSubmitting" @click="simpanAturK" />
      </template>
    </Dialog>

    <!-- modal "Daftar Pegawai" -- klik nilai B/K pada baris Jabatan/Sub-Jabatan -->
    <Dialog v-model:visible="pegawaiListDialog" modal :header="`Daftar Pegawai -- ${pegawaiListTitle}`" style="width: 28rem; max-width: 95vw">
      <div v-if="pegawaiListLoading" style="display: flex; justify-content: center; padding: 1.5rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <Message v-else-if="!pegawaiListItems.length" severity="info" :closable="false">Belum ada pegawai pada baris ini.</Message>
      <ul v-else class="pegawai-list-view">
        <li v-for="pg in pegawaiListItems" :key="pg.id">
          <div style="font-weight: 600; font-size: 0.88rem">{{ pg.nama }}</div>
          <div style="font-size: 0.78rem; color: var(--p-text-muted-color)">{{ pg.nip || '-' }}</div>
        </li>
      </ul>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="pegawaiListDialog = false" />
      </template>
    </Dialog>

    <!-- dialog "Ajukan Kenaikan Pangkat" -->
    <Dialog v-model:visible="ajukanKPDialog" modal header="Ajukan Kenaikan Pangkat" style="width: 30rem; max-width: 95vw">
      <div class="field-label-wrap">
        <label class="field-label">Jabatan Tujuan</label>
        <Select
          v-model="ajukanKPForm.id_jabatan_tujuan"
          :options="jabatanOptions"
          optionLabel="label"
          optionValue="value"
          filter
          placeholder="Pilih jabatan tujuan"
          style="width: 100%"
        />
      </div>
      <div class="field-label-wrap">
        <label class="field-label">Alasan (opsional)</label>
        <Textarea v-model="ajukanKPForm.alasan" rows="3" style="width: 100%" placeholder="Alasan/keterangan tambahan" />
      </div>
      <div class="field-label-wrap">
        <label class="field-label">Bukti Lulus UKOM (PDF, JPG, atau PNG)</label>
        <input ref="ajukanKPFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onAjukanKPFileChosen" />
        <Button :label="ajukanKPFile ? ajukanKPFile.name : 'Pilih Berkas'" icon="pi pi-file" severity="secondary" outlined @click="pickAjukanKPFile" />
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="ajukanKPDialog = false" />
        <Button label="Kirim Pengajuan" icon="pi pi-send" :loading="ajukanKPSubmitting" @click="submitAjukanKP" />
      </template>
    </Dialog>

    <!-- dialog detail Kenaikan Pangkat -->
    <Dialog v-model:visible="kpDetailDialog" modal header="Detail Pengajuan Kenaikan Pangkat" :style="{ width: '42rem', maxWidth: '95vw' }">
      <template v-if="kpDetailRow">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 0.75rem; flex-wrap: wrap; margin-bottom: 1rem">
          <div>
            <div style="font-weight: 600">{{ kpDetailRow.pegawai?.nama }}</div>
            <div style="font-size: 0.85rem; color: var(--p-text-muted-color)">NIP {{ kpDetailRow.pegawai?.nip }} &middot; {{ kpDetailRow.unit_kerja?.unit }}</div>
            <div style="font-size: 0.8rem; color: var(--p-text-muted-color)">Diajukan {{ formatTanggal(kpDetailRow.created_at) }}</div>
          </div>
          <Tag :value="kpStatusLabel(kpDetailRow.status)" :severity="kpStatusSeverity(kpDetailRow.status)" />
        </div>

        <div style="margin-bottom: 1rem">
          <div class="field-label">Jabatan</div>
          <div style="font-size: 0.9rem">
            {{ kpDetailRow.jabatan_asal?.jabatan || '-' }}<template v-if="kpDetailRow.sub_jabatan_asal"> ({{ kpDetailRow.sub_jabatan_asal.nama }})</template>
            <i class="pi pi-arrow-right" style="margin: 0 0.5rem"></i>
            {{ kpDetailRow.jabatan_tujuan?.jabatan || '-' }}<template v-if="kpDetailRow.sub_jabatan_tujuan"> ({{ kpDetailRow.sub_jabatan_tujuan.nama }})</template>
          </div>
        </div>

        <div v-if="kpDetailRow.alasan" style="margin-bottom: 1rem">
          <div class="field-label">Alasan</div>
          <div style="font-size: 0.9rem">{{ kpDetailRow.alasan }}</div>
        </div>

        <div style="margin-bottom: 1rem">
          <div class="field-label">Bukti Lulus UKOM</div>
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <span style="font-size: 0.85rem">{{ kpDetailRow.ukom_nama_file || '-' }}</span>
            <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat" @click="previewUkom(kpDetailRow)" />
          </div>
        </div>

        <Message v-if="kpDetailRow.tgl_atasan_approve" :severity="kpDetailRow.status === 'ditolak_atasan' ? 'danger' : 'success'" :closable="false" style="margin-bottom: 0.75rem">
          Tahap atasan: {{ kpDetailRow.atasan_approve?.nama || '-' }} pada {{ formatTanggal(kpDetailRow.tgl_atasan_approve) }}.
          <template v-if="kpDetailRow.catatan_atasan">Catatan: "{{ kpDetailRow.catatan_atasan }}"</template>
        </Message>
        <Message v-if="kpDetailRow.tgl_admin_approve" :severity="kpDetailRow.status === 'ditolak_admin' ? 'danger' : 'success'" :closable="false" style="margin-bottom: 0.75rem">
          Tahap administrator: {{ kpDetailRow.admin_approve?.nama || kpDetailRow.admin_approve?.username || '-' }} pada {{ formatTanggal(kpDetailRow.tgl_admin_approve) }}.
          <template v-if="kpDetailRow.catatan_admin">Catatan: "{{ kpDetailRow.catatan_admin }}"</template>
        </Message>

        <div v-if="kpDetailRow.status !== 'menunggu_atasan' && kpDetailRow.nomor_surat" style="margin-bottom: 1rem; display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
          <span style="font-size: 0.85rem">Nomor: {{ kpDetailRow.nomor_surat }}</span>
          <Button label="Lihat Dokumen" icon="pi pi-file-pdf" size="small" severity="secondary" outlined @click="previewCetakKP(kpDetailRow)" />
          <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="unduhCetakKP(kpDetailRow)" title="Unduh" />
        </div>

        <template v-if="kpBisaDiprosesAtasan">
          <div class="field-label-wrap" v-if="kpSubTujuanOptions.length">
            <label class="field-label">Sub-Jabatan Tujuan (wajib dipilih)</label>
            <Select
              v-model="kpIdSubTujuan"
              :options="kpSubTujuanOptions"
              optionLabel="nama"
              optionValue="id"
              placeholder="Pilih sub-jabatan tujuan"
              style="width: 100%"
            />
          </div>
          <div class="field-label-wrap">
            <label class="field-label">Catatan (wajib diisi jika mengembalikan)</label>
            <Textarea v-model="kpCatatan" rows="2" style="width: 100%" />
          </div>
        </template>
        <template v-else-if="kpBisaDiprosesAdmin">
          <div class="field-label-wrap">
            <label class="field-label">Catatan (wajib diisi jika mengembalikan)</label>
            <Textarea v-model="kpCatatan" rows="2" style="width: 100%" />
          </div>
        </template>
      </template>

      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="kpDetailDialog = false" />
        <Button v-if="kpBisaDibatalkan" label="Batalkan Pengajuan" icon="pi pi-times" severity="danger" outlined :loading="kpProcessing" @click="kpConfirmBatalkan" />
        <template v-if="kpBisaDiprosesAtasan">
          <Button label="Kembalikan" icon="pi pi-undo" severity="danger" outlined :loading="kpProcessing" @click="kpKembalikanAtasan" />
          <Button label="Setujui" icon="pi pi-check" :loading="kpProcessing" @click="kpSetujuiAtasan" />
        </template>
        <template v-if="kpBisaDiprosesAdmin">
          <Button label="Kembalikan" icon="pi pi-undo" severity="danger" outlined :loading="kpProcessing" @click="kpKembalikanAdmin" />
          <Button label="Setujui" icon="pi pi-check" :loading="kpProcessing" @click="kpSetujuiAdmin" />
        </template>
      </template>
    </Dialog>

    <!-- dialog "Upload SK" (Perubahan Jabatan, sisi pegawai/atasan) -->
    <Dialog v-model:visible="uploadPerubahanDialog" modal header="Upload SK Jabatan/Pangkat Baru" style="width: 28rem; max-width: 95vw">
      <Message severity="info" :closable="false" style="margin-bottom: 1rem">
        Upload SK (surat keputusan) jabatan/pangkat baru anda untuk menyelesaikan proses kenaikan pangkat yang sudah
        disetujui. Jabatan baru diterapkan setelah administrator menyetujui SK ini.
      </Message>
      <div class="field-label-wrap">
        <label class="field-label">Berkas SK (PDF, JPG, atau PNG)</label>
        <input ref="uploadPerubahanFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onUploadPerubahanFileChosen" />
        <Button :label="uploadPerubahanFile ? uploadPerubahanFile.name : 'Pilih Berkas'" icon="pi pi-file" severity="secondary" outlined @click="pickUploadPerubahanFile" />
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="uploadPerubahanDialog = false" />
        <Button label="Upload" icon="pi pi-upload" :loading="uploadPerubahanSubmitting" @click="submitUploadPerubahan" />
      </template>
    </Dialog>

    <!-- dialog detail Perubahan Jabatan (administrator/admin) -->
    <Dialog v-model:visible="perubahanDetailDialog" modal header="Detail Perubahan Jabatan" :style="{ width: '40rem', maxWidth: '95vw' }">
      <template v-if="perubahanDetailRow">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 0.75rem; flex-wrap: wrap; margin-bottom: 1rem">
          <div>
            <div style="font-weight: 600">{{ perubahanDetailRow.pegawai?.nama }}</div>
            <div style="font-size: 0.85rem; color: var(--p-text-muted-color)">NIP {{ perubahanDetailRow.pegawai?.nip }}</div>
          </div>
          <Tag :value="perubahanStatusLabel(perubahanDetailRow.status)" :severity="perubahanStatusSeverity(perubahanDetailRow.status)" />
        </div>

        <div style="margin-bottom: 1rem">
          <div class="field-label">Jabatan Baru</div>
          <div style="font-size: 0.9rem">
            {{ perubahanDetailRow.jabatan_baru?.jabatan || '-' }}<template v-if="perubahanDetailRow.sub_jabatan_baru"> ({{ perubahanDetailRow.sub_jabatan_baru.nama }})</template>
          </div>
        </div>

        <div style="margin-bottom: 1rem">
          <div class="field-label">Berkas SK</div>
          <div v-if="perubahanDetailRow.sk_nama_file" style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <span style="font-size: 0.85rem">{{ perubahanDetailRow.sk_nama_file }}</span>
            <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat" @click="previewSkPerubahan(perubahanDetailRow)" />
          </div>
          <span v-else style="font-size: 0.85rem; color: var(--p-text-muted-color)">Belum diupload</span>
        </div>

        <Message v-if="perubahanDetailRow.status === 'disetujui'" severity="success" :closable="false">
          Disetujui oleh {{ perubahanDetailRow.diputuskan_oleh || '-' }} pada {{ formatTanggal(perubahanDetailRow.tgl_keputusan) }}.
        </Message>
        <template v-else-if="perubahanDetailRow.status === 'menunggu_admin'">
          <div class="field-label-wrap">
            <label class="field-label">Pangkat/Golongan Baru (opsional)</label>
            <Select
              v-model="perubahanIdPangkatGolBaru"
              :options="pangkatGolOptions"
              optionLabel="label"
              optionValue="value"
              filter
              showClear
              placeholder="Tidak ada perubahan pangkat/golongan"
              style="width: 100%"
            />
          </div>
          <div class="field-label-wrap">
            <label class="field-label">Catatan (wajib diisi jika mengembalikan)</label>
            <Textarea v-model="perubahanCatatan" rows="2" style="width: 100%" />
          </div>
        </template>
        <Message v-else-if="perubahanDetailRow.catatan_admin" severity="info" :closable="false">
          Catatan: "{{ perubahanDetailRow.catatan_admin }}"
        </Message>
      </template>

      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="perubahanDetailDialog = false" />
        <template v-if="perubahanDetailRow?.status === 'menunggu_admin'">
          <Button label="Kembalikan" icon="pi pi-undo" severity="danger" outlined :loading="perubahanProcessing" @click="perubahanReject" />
          <Button label="Setujui" icon="pi pi-check" :loading="perubahanProcessing" @click="perubahanConfirmApprove" />
        </template>
      </template>
    </Dialog>

    <!-- preview berkas (UKOM/SK/cetak PDF) -->
    <Dialog v-model:visible="previewDialog" modal :header="previewTitle" :style="{ width: '95vw', maxWidth: '62rem' }" @hide="closePreview">
      <div v-if="previewType === 'pdf'" style="width: 100%; height: 75vh">
        <iframe :src="previewUrl" style="width: 100%; height: 100%; border: none" title="Pratinjau dokumen"></iframe>
      </div>
      <div v-else-if="previewType === 'image'" style="text-align: center">
        <img :src="previewUrl" style="max-width: 100%; max-height: 75vh" alt="Pratinjau dokumen" />
      </div>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="previewDialog = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.tab-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
}

.field-label-wrap {
  margin-bottom: 1.1rem;
}

.sub-jabatan-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
  background: var(--p-content-background, #fff);
  border-radius: 8px;
  overflow: hidden;
}
.sub-jabatan-table th,
.sub-jabatan-table td {
  text-align: left;
  padding: 0.5rem 0.6rem;
  border-bottom: 1px solid #f1f5f9;
}
.sub-jabatan-table th {
  font-weight: 600;
  color: var(--p-text-muted-color);
  font-size: 0.76rem;
}

.sub-jabatan-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  max-height: 16rem;
  overflow-y: auto;
}
.sub-jabatan-list li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--p-content-background, #f8fafc);
  border: 1px solid var(--p-content-border-color, #e2e8f0);
}

.clickable-count {
  cursor: pointer;
  text-decoration: underline;
  text-decoration-style: dotted;
  color: var(--p-primary-color);
  font-weight: 600;
}
.clickable-count:hover {
  opacity: 0.75;
}

.pegawai-sub-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  max-height: 16rem;
  overflow-y: auto;
}
.pegawai-sub-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.4rem 0.1rem;
  border-bottom: 1px solid var(--p-content-border-color, #f1f5f9);
}

.pegawai-list-view {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  max-height: 18rem;
  overflow-y: auto;
}
.pegawai-list-view li {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--p-content-background, #f8fafc);
  border: 1px solid var(--p-content-border-color, #e2e8f0);
}
</style>
