<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { toApiDate, hitungKelayakanKenaikan } from '../utils/date'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Password from 'primevue/password'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'

const props = defineProps({
  config: { type: Object, required: true },
})

const toast = useToast()
const confirm = useConfirm()

// Pilihan "tampilkan N entri" -- dropdown custom di pojok kiri atas tabel
// (dipisah dari paginator bawaan PrimeVue supaya tampilannya sesuai gaya
// tabel baru: dropdown kecil sendiri, paginasi bulat terpisah di bawah).
const entriesOptions = [5, 10, 25, 50, 100]

const items = ref([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const pageSize = ref(10)
const search = ref('')
let searchTimer = null

const dialogVisible = ref(false)
const isEditing = ref(false)
const savingRow = ref(false)
const form = reactive({})
const formErrors = ref('')

const importDialogVisible = ref(false)
const importFile = ref(null)
const importing = ref(false)
const importResult = ref(null)
const fileInputRef = ref(null)
const importMode = ref('append')
const importModeOptions = [
  { label: 'Tambahkan ke data yang ada', value: 'append' },
  { label: 'Hapus semua data lama, lalu import', value: 'replace' },
]

const remoteOptions = reactive({})

// ---- form field type 'coords' (titik koordinat + radius, mis. Unit Kerja)
// -- mengikuti pola "Tempel Koordinat / Link Google Maps" & "Ambil Lokasi
// Saat Ini" yang sama dipakai di Pengaturan Absen (RekapAbsensiView.vue),
// supaya administrator bisa cepat mengisi banyak titik koordinat sekolah
// tanpa harus berada langsung di lokasi tersebut. ----
const coordsPasteText = reactive({})
const locatingCoords = ref(false)

// parseKoordinatMaps mengekstrak (lat, lng) dari teks yang ditempel dari
// Google Maps -- pola dicoba dari yang paling presisi ke paling umum (lihat
// komentar yang sama di RekapAbsensiView.vue).
function parseKoordinatMaps(text) {
  const s = (text || '').trim()
  if (!s) return null
  const cobaPola = (regex) => {
    const m = s.match(regex)
    if (!m) return null
    const lat = Number(m[1])
    const lng = Number(m[2])
    if (Number.isNaN(lat) || Number.isNaN(lng)) return null
    if (lat < -90 || lat > 90 || lng < -180 || lng > 180) return null
    return { lat, lng }
  }
  return (
    cobaPola(/!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)/) ||
    cobaPola(/[?&]q=(-?\d+(?:\.\d+)?),\s*(-?\d+(?:\.\d+)?)/) ||
    cobaPola(/@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)/) ||
    cobaPola(/^(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)$/)
  )
}

function terapkanCoordsPaste(f) {
  const hasil = parseKoordinatMaps(coordsPasteText[f.field])
  if (!hasil) {
    toast.add({
      severity: 'error',
      summary: 'Format tidak dikenali',
      detail: 'Tempel koordinat (contoh: "-1.976688, 121.335284") atau link Google Maps yang mengandung koordinat, lalu klik Terapkan.',
      life: 6000,
    })
    return
  }
  form[f.latField] = Number(hasil.lat.toFixed(6))
  form[f.lngField] = Number(hasil.lng.toFixed(6))
  coordsPasteText[f.field] = ''
}

function gunakanLokasiSaatIni(f) {
  if (!navigator.geolocation) {
    toast.add({ severity: 'warn', summary: 'Tidak didukung', detail: 'Perangkat/browser ini tidak mendukung deteksi lokasi', life: 4000 })
    return
  }
  locatingCoords.value = true
  navigator.geolocation.getCurrentPosition(
    (pos) => {
      form[f.latField] = Number(pos.coords.latitude.toFixed(6))
      form[f.lngField] = Number(pos.coords.longitude.toFixed(6))
      locatingCoords.value = false
      toast.add({ severity: 'success', summary: 'Lokasi ditemukan', detail: `Akurasi ±${Math.round(pos.coords.accuracy)}m -- jangan lupa klik Simpan`, life: 4000 })
    },
    () => {
      locatingCoords.value = false
      toast.add({ severity: 'error', summary: 'Gagal mendeteksi lokasi', detail: 'Izinkan akses lokasi pada browser ini', life: 4000 })
    },
    { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 },
  )
}

function hapusKoordinat(f) {
  form[f.latField] = null
  form[f.lngField] = null
}

// ---- detail dialog (khusus tabel Data Pegawai) ----
const isPegawaiTable = computed(() => props.config.endpoint === '/pegawai')

// ---- generate akun otomatis dari data pegawai (khusus tabel Akun Pengguna) ----
const isUserTable = computed(() => props.config.endpoint === '/user')
const generateDialogVisible = ref(false)
const generating = ref(false)
const generateResult = ref(null)
const detailDialogVisible = ref(false)
const detailLoading = ref(false)
const detailItem = ref(null)
const docFileInputRef = ref(null)
const pendingJenis = ref(null)
const docUploading = ref(false)

const dokumenSlots = [
  { jenis: 'sk-terakhir', field: 'sk_terakhir_nama', label: 'SK Terakhir' },
  { jenis: 'sk-kgb', field: 'sk_kgb_nama', label: 'SK Kenaikan Gaji Berkala' },
  { jenis: 'sk-pangkat', field: 'sk_pangkat_nama', label: 'SK Kenaikan Pangkat' },
  // SK Pensiun SENGAJA selalu ditampilkan (tidak digembok di belakang status
  // "Pensiun" seperti sebelumnya) -- justru upload berkas inilah yang
  // memicu status pegawai berubah otomatis jadi "Pensiun" di backend (lihat
  // uploadDokumenPegawai di handlers/pegawai.go), bukan sebaliknya.
  { jenis: 'sk-pensiun', field: 'sk_pensiun_nama', label: 'SK Pensiun' },
]

function docFilename(jenis) {
  const slot = dokumenSlots.find((d) => d.jenis === jenis)
  return (slot && detailItem.value?.[slot.field]) || ''
}

// ---- riwayat jatah cuti tahunan pegawai ini (diisi otomatis oleh sistem
// setiap kali pengajuan Cuti Tahunan pegawai ini disetujui -- lihat catatan
// di bawah tabel) ----
const detailJatahCuti = ref([])
const detailJatahCutiLoading = ref(false)

function sisaCuti(j) {
  return (j.jumlah_hari ?? 0) - (j.terpakai ?? 0)
}

// ---- usia & kelayakan pensiun (ditampilkan di dialog Detail Pegawai,
// dihitung di frontend murni untuk tampilan -- validasi sesungguhnya tetap
// di backend saat pegawai mengajukan pensiun lewat Profil Saya). ----
const pengaturanPensiun = ref({ usia_pelaksana_struktural: 58, usia_fungsional: 60 })

async function loadPengaturanPensiun() {
  try {
    const { data } = await http.get('/pengaturan-pensiun')
    pengaturanPensiun.value = data.data
  } catch (e) {
    // biarkan pakai default kalau gagal memuat -- hanya tampilan, tidak fatal
  }
}

function hitungUsiaDari(tglLahir) {
  if (!tglLahir) return null
  const lahir = new Date(tglLahir)
  const now = new Date()
  let usia = now.getFullYear() - lahir.getFullYear()
  if (now.getMonth() < lahir.getMonth() || (now.getMonth() === lahir.getMonth() && now.getDate() < lahir.getDate())) {
    usia--
  }
  return usia < 0 ? 0 : usia
}

function usiaPensiunUntuk(jenisJabatan) {
  return jenisJabatan === 'fungsional' ? pengaturanPensiun.value.usia_fungsional : pengaturanPensiun.value.usia_pelaksana_struktural
}

const detailUsia = computed(() => hitungUsiaDari(detailItem.value?.tgl_lahir))
const detailUsiaPensiun = computed(() => usiaPensiunUntuk(detailItem.value?.jabatan?.jenis_jabatan))
const detailSudahMemenuhi = computed(() => detailUsia.value != null && detailUsia.value >= detailUsiaPensiun.value)

// ---- kelayakan kenaikan gaji berkala & kenaikan pangkat (ditampilkan di
// dialog Detail Pegawai, dihitung di frontend murni untuk tampilan -- lihat
// models.PengaturanKenaikanGajiBerkala di backend) ----
const pengaturanKgb = ref({
  gaji_berkala_fungsional_tahun: 1,
  gaji_berkala_pelaksana_struktural_tahun: 2,
  pangkat_fungsional_tahun: 2,
  pangkat_pelaksana_struktural_tahun: 4,
})

async function loadPengaturanKgb() {
  try {
    const { data } = await http.get('/pengaturan-kenaikan-gaji-berkala')
    pengaturanKgb.value = data.data
  } catch (e) {
    // biarkan pakai default kalau gagal memuat -- hanya tampilan, tidak fatal
  }
}

function intervalGajiBerkalaUntuk(jenisJabatan) {
  return jenisJabatan === 'fungsional' ? pengaturanKgb.value.gaji_berkala_fungsional_tahun : pengaturanKgb.value.gaji_berkala_pelaksana_struktural_tahun
}
function intervalPangkatUntuk(jenisJabatan) {
  return jenisJabatan === 'fungsional' ? pengaturanKgb.value.pangkat_fungsional_tahun : pengaturanKgb.value.pangkat_pelaksana_struktural_tahun
}
const detailIntervalGajiBerkala = computed(() => intervalGajiBerkalaUntuk(detailItem.value?.jabatan?.jenis_jabatan))
const detailIntervalPangkat = computed(() => intervalPangkatUntuk(detailItem.value?.jabatan?.jenis_jabatan))
const detailKelayakanGajiBerkala = computed(() => hitungKelayakanKenaikan(detailItem.value?.tgl_kenaikan_gaji_berkala_terakhir, detailIntervalGajiBerkala.value))
const detailKelayakanPangkat = computed(() => hitungKelayakanKenaikan(detailItem.value?.tgl_kenaikan_pangkat_terakhir, detailIntervalPangkat.value))

async function loadDetailJatahCuti(pegawaiId) {
  detailJatahCutiLoading.value = true
  try {
    const { data } = await http.get('/jatah-cuti', { params: { id_pegawai: pegawaiId, pageSize: 50 } })
    detailJatahCuti.value = data.data || []
  } catch (e) {
    detailJatahCuti.value = []
  } finally {
    detailJatahCutiLoading.value = false
  }
}

async function openDetail(row) {
  detailItem.value = row
  detailDialogVisible.value = true
  detailLoading.value = true
  try {
    const { data } = await http.get(`/pegawai/${row.id}`)
    detailItem.value = data.data
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat detail', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    detailLoading.value = false
  }
  loadDetailJatahCuti(row.id)
  loadDetailFoto(detailItem.value)
}

// ---- foto profil pegawai (di dialog Detail Pegawai) ----
// Administrator/admin boleh mengelola foto profil pegawai APA SAJA dari
// sini (mis. menyiapkan foto pegawai baru) -- endpoint backend yang sama
// (/pegawai/{id}/foto) juga dipakai pegawai sendiri lewat ProfilSayaView.vue
// untuk mengganti foto profilnya sendiri kapan saja tanpa persetujuan.
const detailFotoUrl = ref('')
const fotoUploading = ref(false)
const fotoFileInputRef = ref(null)

function closeDetailFoto() {
  if (detailFotoUrl.value) {
    window.URL.revokeObjectURL(detailFotoUrl.value)
    detailFotoUrl.value = ''
  }
}

async function loadDetailFoto(item) {
  closeDetailFoto()
  if (!item?.foto_profil_nama) return
  try {
    const res = await http.get(`/pegawai/${item.id}/foto`, { responseType: 'blob' })
    detailFotoUrl.value = window.URL.createObjectURL(res.data)
  } catch (e) {
    detailFotoUrl.value = ''
  }
}

function pickFotoFile() {
  fotoFileInputRef.value?.click()
}

async function onFotoFileChosen(e) {
  const file = e.target.files[0]
  e.target.value = ''
  if (!file || !detailItem.value) return
  fotoUploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    await http.post(`/pegawai/${detailItem.value.id}/foto`, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    detailItem.value.foto_profil_nama = file.name
    await loadDetailFoto(detailItem.value)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Foto profil berhasil diperbarui', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal upload foto', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    fotoUploading.value = false
  }
}

function confirmRemoveFoto() {
  confirm.require({
    message: 'Hapus foto profil pegawai ini?',
    header: 'Konfirmasi Hapus Foto',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/pegawai/${detailItem.value.id}/foto`)
        detailItem.value.foto_profil_nama = ''
        await loadDetailFoto(detailItem.value)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Foto profil berhasil dihapus', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

function pickDocFile(jenis) {
  pendingJenis.value = jenis
  docFileInputRef.value?.click()
}

async function onDocFileChosen(e) {
  const file = e.target.files[0]
  e.target.value = ''
  const jenis = pendingJenis.value
  if (!file || !jenis || !detailItem.value) return
  docUploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    const { data } = await http.post(`/pegawai/${detailItem.value.id}/dokumen/${jenis}`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    const slot = dokumenSlots.find((d) => d.jenis === jenis)
    if (slot) detailItem.value[slot.field] = data.data.nama_file
    if (jenis === 'sk-pensiun') {
      // Upload SK Pensiun otomatis mengubah status pegawai jadi "Pensiun" di
      // backend (lihat uploadDokumenPegawai) -- muat ulang detail & tabel
      // supaya tag Status di dialog ini dan baris tabel di belakangnya ikut
      // ter-update tanpa perlu tutup-buka dialog / reload halaman.
      try {
        const refreshed = await http.get(`/pegawai/${detailItem.value.id}`)
        detailItem.value = refreshed.data.data
      } catch {
        // biarkan detailItem apa adanya kalau gagal muat ulang -- upload
        // dokumennya sendiri sudah berhasil (toast sukses tetap tampil)
      }
      fetchList()
    }
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Dokumen berhasil diupload', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal upload', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    docUploading.value = false
    pendingJenis.value = null
  }
}

async function downloadDokumen(jenis) {
  try {
    const res = await http.get(`/pegawai/${detailItem.value.id}/dokumen/${jenis}`, { responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.download = docFilename(jenis) || 'dokumen'
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal download', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

function confirmRemoveDokumen(jenis, label) {
  confirm.require({
    message: `Hapus dokumen ${label}? Aksi ini tidak bisa dibatalkan.`,
    header: 'Konfirmasi Hapus Dokumen',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/pegawai/${detailItem.value.id}/dokumen/${jenis}`)
        const slot = dokumenSlots.find((d) => d.jenis === jenis)
        if (slot) detailItem.value[slot.field] = ''
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Dokumen berhasil dihapus', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

function fieldValue(row, path) {
  return path.split('.').reduce((acc, key) => (acc == null ? acc : acc[key]), row)
}

function formatDate(value) {
  if (!value) return '-'
  const d = new Date(value)
  if (isNaN(d.getTime())) return value
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'short', day: 'numeric' })
}

function optionLabelOf(field, option) {
  if (typeof field.optionLabel === 'function') return field.optionLabel(option)
  return option?.[field.optionLabel]
}

async function loadRemoteOptions() {
  const refs = [...new Set((props.config.formFields || []).filter((f) => f.ref).map((f) => f.ref))]
  for (const ref of refs) {
    if (remoteOptions[ref]) continue
    try {
      const { data } = await http.get(`/ref/${ref}`)
      remoteOptions[ref] = data.data || []
    } catch (e) {
      remoteOptions[ref] = []
    }
  }
}

function optionsFor(field) {
  // field.staticOptions: dropdown dengan pilihan tetap (bukan lookup ke
  // tabel referensi lain lewat /ref/*), mis. Jenis Jabatan (Pelaksana/
  // Struktural/Fungsional). Formatnya tetap [{id, label}, ...] agar cocok
  // dengan binding optionValue="id" yang sama dipakai untuk field.ref di
  // bawah, tanpa perlu template Select terpisah.
  if (field.staticOptions) return field.staticOptions
  return remoteOptions[field.ref] || []
}

async function fetchList() {
  loading.value = true
  try {
    const { data } = await http.get(props.config.endpoint, {
      params: { page: page.value, pageSize: pageSize.value, q: search.value || undefined },
    })
    items.value = data.data || []
    total.value = data.meta?.total ?? items.value.length
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

function onPage(event) {
  page.value = Math.floor(event.first / event.rows) + 1
  pageSize.value = event.rows
  fetchList()
}

function onEntriesChange() {
  page.value = 1
  fetchList()
}

watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    fetchList()
  }, 350)
})

function resetForm() {
  Object.keys(form).forEach((k) => delete form[k])
  for (const f of props.config.formFields) {
    if (f.type === 'coords') {
      form[f.latField] = null
      form[f.lngField] = null
      if (f.radiusField) form[f.radiusField] = null
      continue
    }
    if (f.type === 'jamKerja') {
      for (const key of Object.values(f.jamFields)) form[key] = ''
      continue
    }
    form[f.field] = f.type === 'number' ? null : f.type === 'checkbox' ? false : ''
  }
}

function openCreate() {
  isEditing.value = false
  formErrors.value = ''
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  isEditing.value = true
  formErrors.value = ''
  resetForm()
  for (const f of props.config.formFields) {
    if (f.type === 'coords') {
      form[f.latField] = row[f.latField] ?? null
      form[f.lngField] = row[f.lngField] ?? null
      if (f.radiusField) form[f.radiusField] = row[f.radiusField] ?? null
      continue
    }
    if (f.type === 'jamKerja') {
      for (const key of Object.values(f.jamFields)) form[key] = row[key] ?? ''
      continue
    }
    let v = row[f.field]
    if (f.type === 'date' && v) v = new Date(v)
    if (f.type === 'checkbox') {
      form[f.field] = !!v
      continue
    }
    form[f.field] = v ?? (f.type === 'number' ? null : '')
  }
  form.__id = row.id
  dialogVisible.value = true
}

function serializeForm() {
  const payload = {}
  for (const f of props.config.formFields) {
    if (f.type === 'coords') {
      payload[f.latField] = form[f.latField] ?? null
      payload[f.lngField] = form[f.lngField] ?? null
      if (f.radiusField) payload[f.radiusField] = form[f.radiusField] ?? null
      continue
    }
    if (f.type === 'jamKerja') {
      for (const key of Object.values(f.jamFields)) {
        const v = (form[key] || '').trim()
        payload[key] = v || null
      }
      continue
    }
    let v = form[f.field]
    if (f.type === 'date' && v instanceof Date) {
      v = toApiDate(v)
    }
    if (f.type === 'password' && !v) {
      continue // don't send empty password on edit
    }
    if (f.type === 'select' && v === '') {
      v = null // field select opsional yang dikosongkan (mis. "Pilih...") harus dikirim null, bukan string kosong
    }
    payload[f.field] = v
  }
  return payload
}

const jamKerjaPattern = /^([01]\d|2[0-3]):[0-5]\d$/
// validasiJamKerja memeriksa satu grup field 'jamKerja': kelima jam harus
// diisi SEKALIGUS (atau dikosongkan semua) supaya konsisten dengan
// jamAbsenUntukPegawai di backend (yang hanya memakai jam khusus unit kerja
// kalau KELIMA field terisi) -- kembalikan pesan error, atau string kosong
// kalau valid.
function validasiJamKerja(f) {
  const keys = Object.values(f.jamFields)
  const vals = keys.map((k) => (form[k] || '').trim())
  const filled = vals.filter((v) => v !== '')
  if (filled.length === 0) return ''
  if (filled.length < keys.length) {
    return `${f.label}: isi kelima jam sekaligus, atau kosongkan semuanya untuk memakai jam default sekolah/dinas.`
  }
  if (!vals.every((v) => jamKerjaPattern.test(v))) {
    return `${f.label}: format jam tidak valid, gunakan HH:MM (contoh 06:30).`
  }
  return ''
}

function hapusJamKerja(f) {
  for (const key of Object.values(f.jamFields)) form[key] = ''
}

async function saveForm() {
  const missing = (props.config.formFields || []).filter((f) => {
    if (!f.required && !(f.requiredOnCreate && !isEditing.value)) return false
    const v = form[f.field]
    return v === '' || v === null || v === undefined
  })
  if (missing.length) {
    formErrors.value = `Wajib diisi: ${missing.map((f) => f.label).join(', ')}`
    return
  }
  for (const f of props.config.formFields) {
    if (f.type !== 'jamKerja') continue
    const err = validasiJamKerja(f)
    if (err) {
      formErrors.value = err
      return
    }
  }
  formErrors.value = ''
  savingRow.value = true
  try {
    const payload = serializeForm()
    if (isEditing.value) {
      await http.put(`${props.config.endpoint}/${form.__id}`, payload)
      toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Data berhasil diperbarui', life: 3000 })
    } else {
      await http.post(props.config.endpoint, payload)
      toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Data berhasil ditambahkan', life: 3000 })
    }
    dialogVisible.value = false
    fetchList()
  } catch (e) {
    formErrors.value = e.response?.data?.message || 'Gagal menyimpan data'
  } finally {
    savingRow.value = false
  }
}

function confirmDelete(row) {
  confirm.require({
    message: 'Apakah anda yakin ingin menghapus data ini?',
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`${props.config.endpoint}/${row.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Data berhasil dihapus', life: 3000 })
        fetchList()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

async function downloadTemplate() {
  await downloadFile(`${props.config.endpoint}/template`, `template_${props.config.endpoint.replace('/', '')}.xlsx`)
}

async function exportData() {
  await downloadFile(`${props.config.endpoint}/export`, `data_${props.config.endpoint.replace('/', '')}.xlsx`)
}

// ---- export dengan filter (khusus tabel Data Pegawai) ----
const exportFilterDialogVisible = ref(false)
const exportFilterTempatTugas = ref('')
const exportFilterStatus = ref(null)
const exportFilterTempatTugasOptions = [
  { label: 'Semua Tempat Tugas', value: '' },
  { label: 'Dinas / Kantor', value: 'dinas' },
  { label: 'Sekolah', value: 'sekolah' },
]

function openExportFilterDialog() {
  exportFilterTempatTugas.value = ''
  exportFilterStatus.value = null
  exportFilterDialogVisible.value = true
}

async function exportPegawaiFiltered() {
  const params = {}
  if (exportFilterTempatTugas.value) params.tempat_tugas = exportFilterTempatTugas.value
  if (exportFilterStatus.value) params.id_status = exportFilterStatus.value
  try {
    const res = await http.get(`${props.config.endpoint}/export`, { params, responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.setAttribute('download', `data_pegawai.xlsx`)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
    exportFilterDialogVisible.value = false
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh file', detail: e.message, life: 4000 })
  }
}

async function downloadFile(url, filename) {
  try {
    const res = await http.get(url, { responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.setAttribute('download', filename)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh file', detail: e.message, life: 4000 })
  }
}

function openImportDialog() {
  importFile.value = null
  importResult.value = null
  importMode.value = 'append'
  importDialogVisible.value = true
}

function pickFile() {
  fileInputRef.value?.click()
}

function onFileChosen(e) {
  importFile.value = e.target.files[0] || null
}

function submitImport() {
  if (!importFile.value) return
  if (importMode.value === 'replace') {
    const message = isPegawaiTable.value
      ? 'Semua data pegawai akan DIHAPUS, termasuk riwayat pengajuan cuti & jatah cuti tahunan yang terhubung (akun user hanya akan terlepas, tidak terhapus), sebelum data dari file excel dimasukkan. Aksi ini tidak bisa dibatalkan. Lanjutkan?'
      : 'Semua data yang sudah ada di tabel ini akan DIHAPUS sebelum data dari file excel dimasukkan. Aksi ini tidak bisa dibatalkan. Lanjutkan?'
    confirm.require({
      message,
      header: 'Konfirmasi Hapus & Import Ulang',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: 'Ya, Hapus & Import',
      rejectLabel: 'Batal',
      acceptClass: 'p-button-danger',
      accept: () => doImport(),
    })
  } else {
    doImport()
  }
}

async function doImport() {
  importing.value = true
  importResult.value = null
  try {
    const formData = new FormData()
    formData.append('file', importFile.value)
    formData.append('mode', importMode.value)
    const { data } = await http.post(`${props.config.endpoint}/import`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    importResult.value = data.data
    toast.add({ severity: data.success ? 'success' : 'warn', summary: 'Import selesai', detail: data.message, life: 4000 })
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal import', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    importing.value = false
  }
}

function openGenerateDialog() {
  generateResult.value = null
  generateDialogVisible.value = true
}

async function submitGenerate() {
  generating.value = true
  generateResult.value = null
  try {
    const { data } = await http.post('/user/generate-from-pegawai')
    generateResult.value = data.data
    toast.add({ severity: 'success', summary: 'Selesai', detail: data.message, life: 4000 })
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal generate akun', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    generating.value = false
  }
}

// ---- overlay menu: gabungkan Tambah/Template/Import/Export (dan Generate
// Akun, khusus tabel Akun Pengguna) menjadi satu tombol trigger + dropdown,
// mengikuti pola "overlay menu" (tombol + panel berisi aksi ikon+label). ----
const actionsMenuRef = ref(null)

function toggleActionsMenu(event) {
  actionsMenuRef.value?.toggle(event)
}

const actionsMenuItems = computed(() => {
  const items = [
    { label: 'Tambah', icon: 'pi pi-plus', command: () => openCreate() },
    { label: 'Template', icon: 'pi pi-download', command: () => downloadTemplate() },
    { label: 'Import', icon: 'pi pi-upload', command: () => openImportDialog() },
    { label: 'Export', icon: 'pi pi-file-export', command: () => (isPegawaiTable.value ? openExportFilterDialog() : exportData()) },
  ]
  if (isUserTable.value) {
    items.push({ separator: true })
    items.push({ label: 'Generate Akun dari Pegawai', icon: 'pi pi-users', command: () => openGenerateDialog() })
  }
  return items
})

onMounted(() => {
  fetchList()
  loadRemoteOptions()
  if (isPegawaiTable.value) {
    loadPengaturanPensiun()
    loadPengaturanKgb()
  }
})

watch(
  () => props.config,
  () => {
    page.value = 1
    search.value = ''
    fetchList()
    loadRemoteOptions()
  },
)

const canManage = computed(() => true) // route guard already restricts page access per role
</script>

<template>
  <div>
    <div class="page-title">{{ config.title }}</div>
    <p class="page-subtitle">{{ config.subtitle }}</p>

    <div class="card">
      <div class="toolbar-actions" style="justify-content: space-between; margin-bottom: 1rem; align-items: flex-end; flex-wrap: wrap; gap: .75rem">
        <div class="entries-picker">
          <span class="entries-picker-label">Tampilkan</span>
          <Select v-model="pageSize" :options="entriesOptions" @change="onEntriesChange" />
        </div>
        <div class="toolbar-actions" style="align-items: center; flex: 1; justify-content: flex-end">
          <IconField v-if="config.searchPlaceholder !== false" class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
            <InputText v-model="search" :placeholder="config.searchPlaceholder || 'Cari...'" style="width: 100%" />
            <InputIcon class="pi pi-search" />
          </IconField>
          <Button icon="pi pi-chevron-down" iconPos="right" label="Menu" class="overlay-menu-trigger" @click="toggleActionsMenu" aria-haspopup="true" />
          <Menu ref="actionsMenuRef" :model="actionsMenuItems" popup class="overlay-actions-menu" />
        </div>
      </div>

      <div class="responsive-table-wrap">
        <DataTable
          :value="items"
          :loading="loading"
          lazy
          paginator
          :rows="pageSize"
          :totalRecords="total"
          :first="(page - 1) * pageSize"
          @page="onPage"
          paginatorTemplate="CurrentPageReport FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
          currentPageReportTemplate="Showing {first} to {last} of {totalRecords} entries"
          dataKey="id"
          size="small"
          style="min-width: 640px"
        >
          <template #empty>
            <div style="padding: 1.5rem; text-align: center; color: var(--p-text-muted-color)">Tidak ada data</div>
          </template>
          <Column v-for="col in config.columns" :key="col.field" :field="col.field" :header="col.header" :style="col.width ? { width: col.width } : {}">
            <template #body="{ data }">
              <template v-if="col.type === 'date'">{{ formatDate(fieldValue(data, col.field)) }}</template>
              <template v-else-if="col.type === 'badge'">
                <Tag :value="fieldValue(data, col.field) || '-'" severity="info" />
              </template>
              <template v-else-if="col.type === 'boolean'">
                <Tag :value="fieldValue(data, col.field) ? 'Ya' : 'Tidak'" :severity="fieldValue(data, col.field) ? 'success' : 'secondary'" />
              </template>
              <template v-else-if="col.type === 'lookup'">{{ (col.map || {})[fieldValue(data, col.field)] || fieldValue(data, col.field) || '-' }}</template>
              <template v-else-if="col.type === 'coords'">
                <Tag
                  v-if="fieldValue(data, col.latField) != null && fieldValue(data, col.lngField) != null"
                  value="Sudah diatur"
                  severity="success"
                />
                <Tag v-else value="Belum diatur" severity="secondary" />
              </template>
              <template v-else-if="col.type === 'jamKerja'">
                <Tag
                  v-if="Object.values(col.jamFields).every((k) => fieldValue(data, k))"
                  value="Sudah diatur"
                  severity="success"
                />
                <Tag v-else value="Default sekolah/dinas" severity="secondary" />
              </template>
              <template v-else>{{ fieldValue(data, col.field) ?? '-' }}</template>
            </template>
          </Column>
          <Column header="Aksi" :style="{ width: isPegawaiTable ? '170px' : '130px' }">
            <template #body="{ data }">
              <div class="table-actions">
                <Button v-if="isPegawaiTable" icon="pi pi-user" size="small" severity="info" rounded class="table-action-btn" @click="openDetail(data)" />
                <Button icon="pi pi-pencil" size="small" severity="success" rounded class="table-action-btn" @click="openEdit(data)" />
                <Button icon="pi pi-times" size="small" severity="warn" rounded class="table-action-btn" @click="confirmDelete(data)" />
              </div>
            </template>
          </Column>
        </DataTable>
      </div>
    </div>

    <!-- Add / Edit dialog -->
    <Dialog v-model:visible="dialogVisible" modal :header="isEditing ? `Edit ${config.title}` : `Tambah ${config.title}`" :style="{ width: '32rem', maxWidth: '95vw' }">
      <Message v-if="formErrors" severity="error" :closable="false" style="margin-bottom: 1rem">{{ formErrors }}</Message>
      <div style="display: flex; flex-direction: column; gap: 1rem">
        <div v-for="f in config.formFields" :key="f.field">
          <label v-if="f.type !== 'checkbox'" style="display: block; font-size: 0.85rem; font-weight: 600; margin-bottom: 0.35rem">
            {{ f.label }} <span v-if="f.required || (f.requiredOnCreate && !isEditing)" style="color: #ef4444">*</span>
          </label>
          <div v-if="f.type === 'checkbox'" style="display: flex; align-items: center; gap: 0.5rem">
            <Checkbox v-model="form[f.field]" :inputId="'chk-' + f.field" binary />
            <label :for="'chk-' + f.field" style="font-size: 0.85rem; font-weight: 600; cursor: pointer">{{ f.label }}</label>
          </div>
          <InputText v-if="f.type === 'text'" v-model="form[f.field]" :placeholder="f.placeholder" style="width: 100%" />
          <InputNumber v-else-if="f.type === 'number'" v-model="form[f.field]" style="width: 100%" fluid />
          <Textarea v-else-if="f.type === 'textarea'" v-model="form[f.field]" rows="3" style="width: 100%" />
          <DatePicker v-else-if="f.type === 'date'" v-model="form[f.field]" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
          <Password v-else-if="f.type === 'password'" v-model="form[f.field]" :feedback="false" toggleMask style="width: 100%" inputStyle="width: 100%" />
          <Select
            v-else-if="f.type === 'select'"
            v-model="form[f.field]"
            :options="optionsFor(f)"
            :optionLabel="(o) => optionLabelOf(f, o)"
            optionValue="id"
            filter
            showClear
            style="width: 100%"
            placeholder="Pilih..."
          />
          <div v-else-if="f.type === 'coords'">
            <div style="display: flex; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 0.5rem">
              <InputText
                v-model="coordsPasteText[f.field]"
                placeholder='Tempel koordinat (mis. -1.976688, 121.335284) atau link Google Maps'
                style="flex: 1 1 220px"
                @keyup.enter="terapkanCoordsPaste(f)"
              />
              <Button label="Terapkan" icon="pi pi-map" size="small" @click="terapkanCoordsPaste(f)" />
            </div>
            <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 0.5rem">
              <InputNumber v-model="form[f.latField]" placeholder="Lintang (lat)" :minFractionDigits="6" :maxFractionDigits="6" style="width: 100%" />
              <InputNumber v-model="form[f.lngField]" placeholder="Bujur (lng)" :minFractionDigits="6" :maxFractionDigits="6" style="width: 100%" />
              <InputNumber v-if="f.radiusField" v-model="form[f.radiusField]" placeholder="Radius (meter)" suffix=" m" :min="1" style="width: 100%" />
            </div>
            <div style="display: flex; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.5rem">
              <Button label="Gunakan Lokasi Saat Ini" icon="pi pi-map-marker" size="small" outlined :loading="locatingCoords" @click="gunakanLokasiSaatIni(f)" />
              <Button v-if="form[f.latField] != null" label="Hapus Titik Koordinat" icon="pi pi-times" size="small" text severity="danger" @click="hapusKoordinat(f)" />
            </div>
          </div>
          <div v-else-if="f.type === 'jamKerja'">
            <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 0.5rem">
              <div>
                <label style="display: block; font-size: 0.78rem; margin-bottom: 0.2rem">Jam Mulai Absen Pagi</label>
                <InputText v-model="form[f.jamFields.mulaiPagi]" placeholder="06:30" style="width: 100%" />
              </div>
              <div>
                <label style="display: block; font-size: 0.78rem; margin-bottom: 0.2rem">Jam Batas Absen Pagi (terlambat)</label>
                <InputText v-model="form[f.jamFields.batasPagi]" placeholder="07:00" style="width: 100%" />
              </div>
              <div>
                <label style="display: block; font-size: 0.78rem; margin-bottom: 0.2rem">Jam Tutup Absen Masuk</label>
                <InputText v-model="form[f.jamFields.tutupPagi]" placeholder="08:00" style="width: 100%" />
              </div>
              <div>
                <label style="display: block; font-size: 0.78rem; margin-bottom: 0.2rem">Jam Mulai Absen Pulang</label>
                <InputText v-model="form[f.jamFields.mulaiPulang]" placeholder="12:30" style="width: 100%" />
              </div>
              <div>
                <label style="display: block; font-size: 0.78rem; margin-bottom: 0.2rem">Jam Tutup Absen Pulang</label>
                <InputText v-model="form[f.jamFields.tutupPulang]" placeholder="15:00" style="width: 100%" />
              </div>
            </div>
            <div style="margin-top: 0.5rem">
              <Button
                v-if="Object.values(f.jamFields).some((k) => form[k])"
                label="Hapus Jam Khusus"
                icon="pi pi-times"
                size="small"
                text
                severity="danger"
                @click="hapusJamKerja(f)"
              />
            </div>
          </div>
          <small v-if="f.hint" style="color: var(--p-text-muted-color); display: block; margin-top: 0.35rem">{{ f.hint }}</small>
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="dialogVisible = false" />
        <Button label="Simpan" :loading="savingRow" @click="saveForm" />
      </template>
    </Dialog>

    <!-- Import dialog -->
    <Dialog v-model:visible="importDialogVisible" modal header="Import Data dari Excel" :style="{ width: '34rem', maxWidth: '95vw' }">
      <p style="margin-top: 0; color: var(--p-text-muted-color); font-size: 0.9rem">
        Unduh template terlebih dahulu, isi data sesuai format (tanggal: DD-MM-YYYY), lalu upload file excel (.xlsx) di bawah ini.
      </p>
      <Button label="Download Template" icon="pi pi-download" severity="secondary" outlined @click="downloadTemplate" style="margin-bottom: 1rem" />

      <input ref="fileInputRef" type="file" accept=".xlsx" style="display: none" @change="onFileChosen" />
      <div style="display: flex; gap: 0.5rem; align-items: center; margin-bottom: 1rem">
        <Button label="Pilih File Excel" icon="pi pi-file-excel" @click="pickFile" outlined />
        <span style="font-size: 0.85rem">{{ importFile?.name || 'Belum ada file dipilih' }}</span>
      </div>

      <div style="margin-bottom: 1rem">
        <label style="display: block; font-size: 0.85rem; font-weight: 600; margin-bottom: 0.5rem">Jika ada data sebelumnya</label>
        <SelectButton v-model="importMode" :options="importModeOptions" optionLabel="label" optionValue="value" :allowEmpty="false" style="display: flex; flex-wrap: wrap" />
        <small v-if="importMode === 'replace' && isPegawaiTable" style="color: #ef4444; display: block; margin-top: 0.4rem">
          Semua data pegawai akan dihapus permanen, termasuk riwayat pengajuan cuti & jatah cuti tahunan yang terhubung. Akun user yang terhubung hanya akan terlepas (tidak terhapus).
        </small>
        <small v-else-if="importMode === 'replace'" style="color: #ef4444; display: block; margin-top: 0.4rem">
          Semua data lama di tabel ini akan dihapus permanen sebelum data baru dari file dimasukkan.
        </small>
      </div>

      <div v-if="importResult" style="margin-top: 1rem">
        <Message :severity="importResult.failed_rows?.length ? 'warn' : 'success'" :closable="false">
          {{ importResult.success_count }} baris berhasil diimport, {{ importResult.failed_rows?.length || 0 }} baris gagal.
        </Message>
        <div v-if="importResult.failed_rows?.length" class="responsive-table-wrap" style="margin-top: 0.75rem; max-height: 220px; overflow-y: auto">
          <table style="width: 100%; border-collapse: collapse; font-size: 0.82rem">
            <thead>
              <tr>
                <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">Baris</th>
                <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">Error</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="fr in importResult.failed_rows" :key="fr.row">
                <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ fr.row }}</td>
                <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ fr.errors.join('; ') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="importDialogVisible = false" />
        <Button label="Upload & Import" icon="pi pi-upload" :loading="importing" :disabled="!importFile" @click="submitImport" />
      </template>
    </Dialog>

    <!-- Generate akun otomatis dari Data Pegawai (khusus tabel Akun Pengguna) -->
    <Dialog v-model:visible="generateDialogVisible" modal header="Generate Akun dari Data Pegawai" :style="{ width: '34rem', maxWidth: '95vw' }">
      <Message severity="info" :closable="false" style="margin-bottom: 1rem">
        Sistem akan membuat akun untuk setiap pegawai di Data Pegawai yang belum terhubung ke akun manapun, dengan aturan:
        username = NIP pegawai, nama = nama pegawai, role = <strong>pegawai</strong>, password = <strong>123456</strong>,
        dan otomatis terhubung ke data pegawai masing-masing. Pegawai yang sudah punya akun tidak akan dibuatkan akun baru.
      </Message>

      <div v-if="generateResult" style="margin-top: 1rem">
        <Message :severity="generateResult.skipped_count ? 'warn' : 'success'" :closable="false">
          {{ generateResult.created_count }} akun berhasil dibuat, {{ generateResult.skipped_count }} pegawai dilewati.
        </Message>

        <div v-if="generateResult.created?.length" style="margin-top: 0.75rem">
          <div style="font-size: 0.85rem; font-weight: 600; margin-bottom: 0.35rem">Akun yang dibuat</div>
          <div class="responsive-table-wrap" style="max-height: 200px; overflow-y: auto">
            <table style="width: 100%; border-collapse: collapse; font-size: 0.82rem">
              <thead>
                <tr>
                  <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">Nama</th>
                  <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">Username (NIP)</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(c, idx) in generateResult.created" :key="'c' + idx">
                  <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ c.nama }}</td>
                  <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ c.username }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div v-if="generateResult.skipped?.length" style="margin-top: 0.75rem">
          <div style="font-size: 0.85rem; font-weight: 600; margin-bottom: 0.35rem">Pegawai yang dilewati</div>
          <div class="responsive-table-wrap" style="max-height: 200px; overflow-y: auto">
            <table style="width: 100%; border-collapse: collapse; font-size: 0.82rem">
              <thead>
                <tr>
                  <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">Nama</th>
                  <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">NIP</th>
                  <th style="text-align: left; padding: 0.4rem; border-bottom: 1px solid #e2e8f0">Alasan</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(s, idx) in generateResult.skipped" :key="'s' + idx">
                  <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ s.nama }}</td>
                  <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ s.nip || '-' }}</td>
                  <td style="padding: 0.4rem; border-bottom: 1px solid #f1f5f9">{{ s.reason }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="generateDialogVisible = false" />
        <Button label="Generate Akun" icon="pi pi-users" :loading="generating" @click="submitGenerate" />
      </template>
    </Dialog>

    <!-- Export filter dialog (khusus Data Pegawai) -->
    <Dialog v-model:visible="exportFilterDialogVisible" modal header="Export Data Pegawai" :style="{ width: '28rem', maxWidth: '95vw' }">
      <p style="margin-top: 0; color: var(--p-text-muted-color); font-size: 0.9rem">
        Pilih filter (opsional) sebelum mengunduh data pegawai ke excel. Kosongkan / pilih "Semua" untuk mengekspor seluruh data.
      </p>
      <div style="display: flex; flex-direction: column; gap: 1rem">
        <div>
          <label style="display: block; font-size: 0.85rem; font-weight: 600; margin-bottom: 0.4rem">Tempat Tugas</label>
          <Select v-model="exportFilterTempatTugas" :options="exportFilterTempatTugasOptions" optionLabel="label" optionValue="value" style="width: 100%" />
        </div>
        <div>
          <label style="display: block; font-size: 0.85rem; font-weight: 600; margin-bottom: 0.4rem">Status Kepegawaian</label>
          <Select
            v-model="exportFilterStatus"
            :options="remoteOptions.status || []"
            optionLabel="status"
            optionValue="id"
            showClear
            filter
            placeholder="Semua Status"
            style="width: 100%"
          />
          <small style="color: var(--p-text-muted-color)">Tambah/kelola pilihan status (mis. PNS, PPPK, PPPK Paruh Waktu) lewat menu Status Pegawai.</small>
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="exportFilterDialogVisible = false" />
        <Button label="Export" icon="pi pi-file-export" @click="exportPegawaiFiltered" />
      </template>
    </Dialog>

    <!-- Detail Pegawai + Dokumen dialog -->
    <input ref="docFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onDocFileChosen" />
    <input ref="fotoFileInputRef" type="file" accept=".jpg,.jpeg,.png" style="display: none" @change="onFotoFileChosen" />
    <Dialog
      v-model:visible="detailDialogVisible"
      modal
      header="Detail Pegawai"
      :style="{ width: '40rem', maxWidth: '95vw' }"
      @hide="closeDetailFoto"
    >
      <div v-if="detailLoading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 38px; height: 38px" />
      </div>
      <template v-else-if="detailItem">
        <div class="foto-profil-row">
          <div class="foto-profil-avatar">
            <img v-if="detailFotoUrl" :src="detailFotoUrl" alt="Foto profil" />
            <i v-else class="pi pi-user"></i>
          </div>
          <div>
            <div style="font-weight: 600; margin-bottom: 0.4rem">Foto Profil</div>
            <div style="display: flex; gap: 0.5rem; flex-wrap: wrap">
              <Button
                :label="detailItem.foto_profil_nama ? 'Ganti Foto' : 'Upload Foto'"
                icon="pi pi-upload"
                size="small"
                outlined
                :loading="fotoUploading"
                @click="pickFotoFile"
              />
              <Button v-if="detailItem.foto_profil_nama" label="Hapus" icon="pi pi-trash" size="small" severity="danger" text @click="confirmRemoveFoto" />
            </div>
            <small style="display: block; margin-top: 0.35rem; color: var(--p-text-muted-color)">
              Foto ini juga akan muncul di akun pegawai ybs. Pegawai sendiri bisa menggantinya kapan saja lewat Profil Saya, tanpa persetujuan.
            </small>
          </div>
        </div>

        <div class="detail-grid">
          <div><span class="detail-label">NIP</span><div>{{ detailItem.nip || '-' }}</div></div>
          <div><span class="detail-label">Nama</span><div>{{ detailItem.nama || '-' }}</div></div>
          <div><span class="detail-label">Jabatan</span><div>{{ detailItem.jabatan?.jabatan || '-' }}</div></div>
          <div><span class="detail-label">Unit Kerja</span><div>{{ detailItem.unit_kerja?.unit || '-' }}</div></div>
          <div><span class="detail-label">Pangkat / Golongan</span><div>{{ detailItem.pangkat_gol?.pangkat?.pangkat || '-' }} / {{ detailItem.pangkat_gol?.gol?.gol || '-' }}</div></div>
          <div><span class="detail-label">Tempat Tugas</span><div>{{ detailItem.tempat_tgs || '-' }}</div></div>
          <div><span class="detail-label">TMT</span><div>{{ formatDate(detailItem.tmt) }}</div></div>
          <div>
            <span class="detail-label">Tanggal Lahir</span>
            <div>
              {{ formatDate(detailItem.tgl_lahir) }}
              <span v-if="detailUsia != null" style="color: var(--p-text-muted-color)">({{ detailUsia }} tahun)</span>
            </div>
          </div>
          <div>
            <span class="detail-label">Kelayakan Pensiun</span>
            <div>
              <Tag
                v-if="detailUsia != null"
                :value="detailSudahMemenuhi ? `Sudah memenuhi usia pensiun (${detailUsiaPensiun})` : `Belum (usia pensiun ${detailUsiaPensiun})`"
                :severity="detailSudahMemenuhi ? 'warn' : 'success'"
              />
              <span v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Isi Tanggal Lahir untuk menghitung</span>
            </div>
          </div>
          <div>
            <span class="detail-label">Kenaikan Gaji Berkala Terakhir</span>
            <div>{{ formatDate(detailItem.tgl_kenaikan_gaji_berkala_terakhir) }}</div>
          </div>
          <div>
            <span class="detail-label">Kelayakan Kenaikan Gaji Berkala</span>
            <div>
              <Tag
                v-if="detailKelayakanGajiBerkala.jatuhTempo"
                :value="detailKelayakanGajiBerkala.sudahWaktunya ? `Sudah waktunya (interval ${detailIntervalGajiBerkala} th)` : `Jatuh tempo ${formatDate(detailKelayakanGajiBerkala.jatuhTempo)}`"
                :severity="detailKelayakanGajiBerkala.sudahWaktunya ? 'warn' : 'success'"
              />
              <span v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Belum diisi</span>
            </div>
          </div>
          <div>
            <span class="detail-label">Kenaikan Pangkat Terakhir</span>
            <div>{{ formatDate(detailItem.tgl_kenaikan_pangkat_terakhir) }}</div>
          </div>
          <div>
            <span class="detail-label">Kelayakan Kenaikan Pangkat</span>
            <div>
              <Tag
                v-if="detailKelayakanPangkat.jatuhTempo"
                :value="detailKelayakanPangkat.sudahWaktunya ? `Sudah waktunya (interval ${detailIntervalPangkat} th)` : `Jatuh tempo ${formatDate(detailKelayakanPangkat.jatuhTempo)}`"
                :severity="detailKelayakanPangkat.sudahWaktunya ? 'warn' : 'success'"
              />
              <span v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Belum diisi</span>
            </div>
          </div>
          <div><span class="detail-label">No HP</span><div>{{ detailItem.no_hp || '-' }}</div></div>
          <div><span class="detail-label">Email</span><div>{{ detailItem.email || '-' }}</div></div>
          <div><span class="detail-label">Status</span><div><Tag :value="detailItem.status?.status || '-'" severity="info" /></div></div>
          <div><span class="detail-label">Atasan Langsung</span><div>{{ detailItem.atasan?.nama || '-' }}</div></div>
        </div>

        <h4 style="margin: 1.5rem 0 0.75rem 0">Dokumen</h4>
        <div v-for="slot in dokumenSlots" :key="slot.jenis" class="doc-row">
          <div class="doc-info">
            <div class="doc-label">{{ slot.label }}</div>
            <div class="doc-filename">{{ docFilename(slot.jenis) || 'Belum ada dokumen' }}</div>
          </div>
          <div class="doc-actions">
            <Button v-if="docFilename(slot.jenis)" icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadDokumen(slot.jenis)" />
            <Button
              icon="pi pi-upload"
              size="small"
              outlined
              :label="docFilename(slot.jenis) ? 'Ganti' : 'Upload'"
              :loading="docUploading && pendingJenis === slot.jenis"
              @click="pickDocFile(slot.jenis)"
            />
            <Button v-if="docFilename(slot.jenis)" icon="pi pi-trash" size="small" severity="danger" text @click="confirmRemoveDokumen(slot.jenis, slot.label)" />
          </div>
        </div>
        <small style="display: block; margin-top: 0.4rem; color: var(--p-text-muted-color)">
          Meng-upload SK Pensiun akan otomatis mengubah Status pegawai ini menjadi "Pensiun".
        </small>

        <h4 style="margin: 1.5rem 0 0.75rem 0">Jatah Cuti Tahunan</h4>
        <div v-if="detailJatahCutiLoading" style="display: flex; justify-content: center; padding: 1rem">
          <ProgressSpinner style="width: 28px; height: 28px" />
        </div>
        <template v-else-if="detailJatahCuti.length">
          <div class="responsive-table-wrap">
            <table class="jatah-table">
              <thead>
                <tr>
                  <th>Tahun</th>
                  <th>Jumlah Hari</th>
                  <th>Terpakai</th>
                  <th>Sisa</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="j in detailJatahCuti" :key="j.id">
                  <td>{{ j.tahun }}</td>
                  <td>{{ j.jumlah_hari }}</td>
                  <td>{{ j.terpakai }}</td>
                  <td><Tag :value="String(sisaCuti(j))" :severity="sisaCuti(j) > 0 ? 'success' : 'danger'" /></td>
                </tr>
              </tbody>
            </table>
          </div>
          <small style="display: block; margin-top: 0.4rem; color: var(--p-text-muted-color)">
            Kolom "Terpakai" terisi &amp; bertambah otomatis setiap kali pengajuan Cuti Tahunan pegawai ini disetujui.
          </small>
        </template>
        <div v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">
          Belum ada data jatah cuti tahunan. Baris akan muncul otomatis di sini begitu pengajuan Cuti Tahunan pegawai ini pertama kali disetujui (atau bisa ditambahkan manual lewat menu Jatah Cuti Tahunan).
        </div>
      </template>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="detailDialogVisible = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.foto-profil-row {
  display: flex;
  align-items: center;
  gap: 1.1rem;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}

.foto-profil-avatar {
  width: 76px;
  height: 76px;
  border-radius: 50%;
  background: var(--p-surface-100, #f1f5f9);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  flex-shrink: 0;
}

.foto-profil-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.foto-profil-avatar i {
  font-size: 1.9rem;
  color: var(--p-text-muted-color, #94a3b8);
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.9rem 1.25rem;
}

@media (max-width: 480px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }
}

.detail-label {
  display: block;
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--p-text-muted-color);
  text-transform: uppercase;
  letter-spacing: 0.02em;
  margin-bottom: 0.15rem;
}

.doc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.65rem 0;
  border-bottom: 1px solid #f1f5f9;
  flex-wrap: wrap;
}

.doc-row:last-child {
  border-bottom: none;
}

.doc-label {
  font-size: 0.85rem;
  font-weight: 600;
}

.jatah-table {
  width: 100%;
  min-width: 320px;
  border-collapse: collapse;
  font-size: 0.85rem;
}

.jatah-table th,
.jatah-table td {
  text-align: left;
  padding: 0.5rem 0.6rem;
  border-bottom: 1px solid #f1f5f9;
}

.jatah-table th {
  font-weight: 600;
  color: var(--p-text-muted-color);
  font-size: 0.78rem;
}

.doc-filename {
  font-size: 0.8rem;
  color: var(--p-text-muted-color);
}

.doc-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}
</style>
