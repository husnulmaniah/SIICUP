<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { useProfilePhoto } from '../composables/useProfilePhoto'
import { jenisJabatanLabels } from '../config/tables'
import { hitungKelayakanKenaikan } from '../utils/date'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Checkbox from 'primevue/checkbox'
import Textarea from 'primevue/textarea'

const toast = useToast()
const confirm = useConfirm()

const loading = ref(true)
const profile = ref(null)

const refJabatan = ref([])
const refUnitKerja = ref([])
const refPangkatGol = ref([])
const refStatus = ref([])

const riwayat = ref([])
const riwayatLoading = ref(false)

const pendingRequest = computed(() => riwayat.value.find((r) => r.status === 'pending'))
const riwayatSelesai = computed(() => riwayat.value.filter((r) => r.status !== 'pending'))

async function loadProfile() {
  loading.value = true
  try {
    const { data } = await http.get('/pegawai/me')
    profile.value = data.data
    refreshFoto(profile.value?.id)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data profil', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

// ---- foto profil: pegawai boleh mengganti/menghapus SENDIRI kapan saja,
// TIDAK lewat alur pengajuan Perubahan Data Pegawai (yang butuh persetujuan
// administrator/admin di bawah) -- berlaku langsung begitu diupload. State
// foto dibagikan dengan avatar topbar lewat composable ini (lihat
// composables/useProfilePhoto.js), supaya topbar ikut berubah seketika.
const { url: fotoUrl, refresh: refreshFoto } = useProfilePhoto()
const fotoFileInputRef = ref(null)
const fotoUploading = ref(false)

async function onFotoFileChosen(e) {
  const file = e.target.files[0]
  e.target.value = ''
  if (!file || !profile.value) return
  fotoUploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    await http.post(`/pegawai/${profile.value.id}/foto`, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    profile.value.foto_profil_nama = file.name
    await refreshFoto(profile.value.id)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Foto profil berhasil diperbarui', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal upload foto', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    fotoUploading.value = false
  }
}

function confirmHapusFoto() {
  confirm.require({
    message: 'Hapus foto profil anda?',
    header: 'Konfirmasi Hapus Foto',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/pegawai/${profile.value.id}/foto`)
        profile.value.foto_profil_nama = ''
        await refreshFoto(profile.value.id)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Foto profil berhasil dihapus', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

async function loadRiwayat() {
  riwayatLoading.value = true
  try {
    const { data } = await http.get('/perubahan-data')
    riwayat.value = data.data || []
  } catch (e) {
    riwayat.value = []
  } finally {
    riwayatLoading.value = false
  }
}

async function loadRefs() {
  try {
    const [j, u, pg, s] = await Promise.all([
      http.get('/ref/jabatan'),
      http.get('/ref/unit-kerja'),
      http.get('/ref/pangkat-gol'),
      http.get('/ref/status'),
    ])
    refJabatan.value = j.data.data || []
    refUnitKerja.value = u.data.data || []
    refPangkatGol.value = pg.data.data || []
    refStatus.value = s.data.data || []
  } catch (e) {
    // dropdown referensi gagal dimuat -- form tetap bisa dipakai untuk field teks
  }
}

// ============================================================
// Ajukan Pensiun -- pegawai bisa mengajukan sendiri pensiun (berbeda dari
// upload SK Pensiun langsung oleh admin di menu Data Pegawai, yang instan
// tanpa persetujuan). Pengajuan di sini WAJIB disetujui administrator/admin
// dulu sebelum status pegawai berubah jadi "Pensiun" & akun login
// dinonaktifkan (lihat backend/handlers/pengajuan_pensiun.go). Kalau usia
// pegawai (dihitung dari Tanggal Lahir) belum mencapai usia pensiun standar
// jabatannya, harus mencentang "Pensiun Dini" & mengisi alasan.
// ============================================================
const riwayatPensiun = ref([])
const riwayatPensiunLoading = ref(false)
const pendingPensiunRequest = computed(() => riwayatPensiun.value.find((r) => r.status === 'pending'))
const pengaturanPensiun = ref({ usia_pelaksana_struktural: 58, usia_fungsional: 60 })

async function loadRiwayatPensiun() {
  riwayatPensiunLoading.value = true
  try {
    const { data } = await http.get('/pengajuan-pensiun')
    riwayatPensiun.value = data.data || []
  } catch (e) {
    riwayatPensiun.value = []
  } finally {
    riwayatPensiunLoading.value = false
  }
}

async function loadPengaturanPensiun() {
  try {
    const { data } = await http.get('/pengaturan-pensiun')
    pengaturanPensiun.value = data.data
  } catch (e) {
    // biarkan pakai default kalau gagal memuat -- hanya tampilan perkiraan
  }
}

const usiaSaya = computed(() => {
  if (!profile.value?.tgl_lahir) return null
  const lahir = new Date(profile.value.tgl_lahir)
  const now = new Date()
  let usia = now.getFullYear() - lahir.getFullYear()
  if (now.getMonth() < lahir.getMonth() || (now.getMonth() === lahir.getMonth() && now.getDate() < lahir.getDate())) usia--
  return usia < 0 ? 0 : usia
})
const usiaPensiunSaya = computed(() =>
  profile.value?.jabatan?.jenis_jabatan === 'fungsional' ? pengaturanPensiun.value.usia_fungsional : pengaturanPensiun.value.usia_pelaksana_struktural,
)
const sudahMemenuhiUsiaPensiun = computed(() => usiaSaya.value != null && usiaSaya.value >= usiaPensiunSaya.value)

// ============================================================
// Kelayakan Kenaikan Gaji Berkala & Kenaikan Pangkat -- dihitung murni untuk
// TAMPILAN dari Pegawai.TglKenaikanGajiBerkalaTerakhir/
// TglKenaikanPangkatTerakhir (opsional, diisi lewat Ajukan Perubahan Data di
// bawah atau langsung oleh administrator) + interval standar per jenis
// jabatan (lihat models.PengaturanKenaikanGajiBerkala di backend). Tidak ada
// alur pengajuan/persetujuan tersendiri seperti Pensiun -- hanya perkiraan.
// ============================================================
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
    // biarkan pakai default kalau gagal memuat -- hanya tampilan perkiraan
  }
}

const isFungsional = computed(() => profile.value?.jabatan?.jenis_jabatan === 'fungsional')
const intervalGajiBerkalaSaya = computed(() =>
  isFungsional.value ? pengaturanKgb.value.gaji_berkala_fungsional_tahun : pengaturanKgb.value.gaji_berkala_pelaksana_struktural_tahun,
)
const intervalPangkatSaya = computed(() =>
  isFungsional.value ? pengaturanKgb.value.pangkat_fungsional_tahun : pengaturanKgb.value.pangkat_pelaksana_struktural_tahun,
)
const kelayakanGajiBerkalaSaya = computed(() => hitungKelayakanKenaikan(profile.value?.tgl_kenaikan_gaji_berkala_terakhir, intervalGajiBerkalaSaya.value))
const kelayakanPangkatSaya = computed(() => hitungKelayakanKenaikan(profile.value?.tgl_kenaikan_pangkat_terakhir, intervalPangkatSaya.value))

const pensiunDialogVisible = ref(false)
const pensiunSaving = ref(false)
const pensiunFormErrors = ref('')
const pensiunIsDini = ref(false)
const pensiunAlasan = ref('')
const pensiunFile = ref(null)
const pensiunFileInputRef = ref(null)

function openAjukanPensiun() {
  pensiunIsDini.value = !sudahMemenuhiUsiaPensiun.value
  pensiunAlasan.value = ''
  pensiunFile.value = null
  pensiunFormErrors.value = ''
  pensiunDialogVisible.value = true
}

function pickPensiunFile() {
  pensiunFileInputRef.value?.click()
}
function onPensiunFileChosen(e) {
  pensiunFile.value = e.target.files[0] || null
}

async function submitAjukanPensiun() {
  pensiunFormErrors.value = ''
  if (pensiunIsDini.value && !pensiunAlasan.value.trim()) {
    pensiunFormErrors.value = 'Alasan wajib diisi untuk pengajuan pensiun dini'
    return
  }
  if (!pensiunFile.value) {
    pensiunFormErrors.value = 'Berkas SK/usulan pensiun wajib diupload'
    return
  }
  pensiunSaving.value = true
  try {
    const fd = new FormData()
    fd.append('is_pensiun_dini', pensiunIsDini.value ? 'true' : 'false')
    fd.append('alasan', pensiunAlasan.value)
    fd.append('file', pensiunFile.value)
    await http.post('/pengajuan-pensiun', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({
      severity: 'success',
      summary: 'Berhasil dikirim',
      detail: 'Pengajuan pensiun berhasil dikirim, menunggu persetujuan administrator/admin',
      life: 5000,
    })
    pensiunDialogVisible.value = false
    loadRiwayatPensiun()
  } catch (e) {
    pensiunFormErrors.value = e.response?.data?.message || 'Gagal mengirim pengajuan'
  } finally {
    pensiunSaving.value = false
  }
}

function confirmBatalPensiun(row) {
  confirm.require({
    message: 'Batalkan pengajuan pensiun ini? Anda bisa mengajukan lagi kapan saja setelah dibatalkan.',
    header: 'Konfirmasi Batal',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Batalkan',
    rejectLabel: 'Tidak',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/pengajuan-pensiun/${row.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan dibatalkan', life: 3000 })
        loadRiwayatPensiun()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

async function previewSkPensiun(row) {
  try {
    const res = await http.get(`/pengajuan-pensiun/${row.id}/dokumen`, { params: { inline: 1 }, responseType: 'blob' })
    const ext = (row.sk_nama_file || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewTitle.value = 'Berkas SK / Usulan Pensiun'
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
async function downloadSkPensiun(row) {
  try {
    const res = await http.get(`/pengajuan-pensiun/${row.id}/dokumen`, { responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.download = row.sk_nama_file || 'sk-pensiun'
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh berkas', detail: e.message, life: 4000 })
  }
}

onMounted(() => {
  loadProfile()
  loadRiwayat()
  loadRefs()
  loadRiwayatPensiun()
  loadPengaturanPensiun()
  loadPengaturanKgb()
})

function jabatanLabel(id) {
  return refJabatan.value.find((x) => x.id === id)?.jabatan || '-'
}
function unitKerjaLabel(id) {
  return refUnitKerja.value.find((x) => x.id === id)?.unit || '-'
}
function pangkatGolLabel(id) {
  const pg = refPangkatGol.value.find((x) => x.id === id)
  if (!pg) return '-'
  return `${pg.pangkat?.pangkat || '-'} / ${pg.gol?.gol || '-'}`
}
function statusPegawaiLabel(id) {
  return refStatus.value.find((x) => x.id === id)?.status || '-'
}

// ---- form pengajuan perubahan data ----
const dialogVisible = ref(false)
const saving = ref(false)
const formErrors = ref('')
const form = reactive({
  nama: '',
  id_jabatan: null,
  id_unit_kerja: null,
  id_pangkat_gol: null,
  tempat_tgs: '',
  tmt: null,
  tgl_lahir: null,
  tgl_kenaikan_gaji_berkala_terakhir: null,
  tgl_kenaikan_pangkat_terakhir: null,
  no_hp: '',
  id_status: null,
  email: '',
})
const skFile = ref(null)
const skFileInputRef = ref(null)
const skKgbFile = ref(null)
const skKgbFileInputRef = ref(null)

const hasSkTerakhir = computed(() => !!profile.value?.sk_terakhir_nama)
const hasSkKgb = computed(() => !!profile.value?.sk_kgb_nama)

function openAjukan() {
  if (!profile.value) return
  form.nama = profile.value.nama || ''
  form.id_jabatan = profile.value.id_jabatan || null
  form.id_unit_kerja = profile.value.id_unit_kerja || null
  form.id_pangkat_gol = profile.value.id_pangkat_gol || null
  form.tempat_tgs = profile.value.tempat_tgs || ''
  form.tmt = profile.value.tmt ? new Date(profile.value.tmt) : null
  form.tgl_lahir = profile.value.tgl_lahir ? new Date(profile.value.tgl_lahir) : null
  form.tgl_kenaikan_gaji_berkala_terakhir = profile.value.tgl_kenaikan_gaji_berkala_terakhir ? new Date(profile.value.tgl_kenaikan_gaji_berkala_terakhir) : null
  form.tgl_kenaikan_pangkat_terakhir = profile.value.tgl_kenaikan_pangkat_terakhir ? new Date(profile.value.tgl_kenaikan_pangkat_terakhir) : null
  form.no_hp = profile.value.no_hp || ''
  form.id_status = profile.value.id_status || null
  form.email = profile.value.email || ''
  skFile.value = null
  skKgbFile.value = null
  formErrors.value = ''
  dialogVisible.value = true
}

async function previewSkKgbSaatIni() {
  if (!profile.value?.id) return
  try {
    const res = await http.get(`/pegawai/${profile.value.id}/dokumen/sk-kgb`, { responseType: 'blob' })
    const ext = (profile.value.sk_kgb_nama || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewTitle.value = 'Berkas SK Kenaikan Gaji Berkala Saat Ini'
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

async function previewSkTerakhirSaatIni() {
  if (!profile.value?.id) return
  try {
    const res = await http.get(`/pegawai/${profile.value.id}/dokumen/sk-terakhir`, { responseType: 'blob' })
    const ext = (profile.value.sk_terakhir_nama || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewTitle.value = 'Berkas SK Terakhir Saat Ini'
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

function pickSkFile() {
  skFileInputRef.value?.click()
}
function onSkFileChosen(e) {
  skFile.value = e.target.files[0] || null
}

function pickSkKgbFile() {
  skKgbFileInputRef.value?.click()
}
function onSkKgbFileChosen(e) {
  skKgbFile.value = e.target.files[0] || null
}

function toDateStr(d) {
  if (!d) return ''
  const dt = new Date(d)
  const pad = (n) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
}

async function submitAjukan() {
  formErrors.value = ''
  if (!form.nama?.trim()) {
    formErrors.value = 'Nama wajib diisi'
    return
  }
  if (!skFile.value && !hasSkTerakhir.value) {
    formErrors.value = 'Berkas SK Terakhir wajib diupload sebagai dasar perubahan data'
    return
  }
  saving.value = true
  try {
    const payload = {
      nama: form.nama,
      id_jabatan: form.id_jabatan,
      id_unit_kerja: form.id_unit_kerja,
      id_pangkat_gol: form.id_pangkat_gol,
      tempat_tgs: form.tempat_tgs,
      tmt: toDateStr(form.tmt),
      tgl_lahir: toDateStr(form.tgl_lahir),
      tgl_kenaikan_gaji_berkala_terakhir: toDateStr(form.tgl_kenaikan_gaji_berkala_terakhir),
      tgl_kenaikan_pangkat_terakhir: toDateStr(form.tgl_kenaikan_pangkat_terakhir),
      no_hp: form.no_hp,
      id_status: form.id_status,
      email: form.email,
    }
    const fd = new FormData()
    fd.append('data', JSON.stringify(payload))
    if (skFile.value) fd.append('file', skFile.value)
    if (skKgbFile.value) fd.append('file_kgb', skKgbFile.value)
    await http.post('/perubahan-data', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({
      severity: 'success',
      summary: 'Berhasil dikirim',
      detail: 'Pengajuan perubahan data berhasil dikirim, menunggu persetujuan administrator/admin',
      life: 5000,
    })
    dialogVisible.value = false
    loadRiwayat()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengirim pengajuan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    saving.value = false
  }
}

function confirmBatal(row) {
  confirm.require({
    message: 'Batalkan pengajuan perubahan data ini? Anda bisa mengajukan lagi kapan saja setelah dibatalkan.',
    header: 'Konfirmasi Batal',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Batalkan',
    rejectLabel: 'Tidak',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/perubahan-data/${row.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan dibatalkan', life: 3000 })
        loadRiwayat()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

// ---- lihat / unduh berkas SK yang pernah diupload ----
const previewDialog = ref(false)
const previewUrl = ref('')
const previewType = ref('pdf')
const previewTitle = ref('')

async function previewSk(row) {
  try {
    const res = await http.get(`/perubahan-data/${row.id}/dokumen`, { params: { inline: 1 }, responseType: 'blob' })
    const ext = (row.sk_nama_file || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewTitle.value = 'Berkas SK Terakhir'
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
function closePreview() {
  if (previewUrl.value) window.URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
}
async function downloadSk(row) {
  try {
    const res = await http.get(`/perubahan-data/${row.id}/dokumen`, { responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.download = row.sk_nama_file || 'sk-terakhir'
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh berkas', detail: e.message, life: 4000 })
  }
}

function statusSeverity(s) {
  return s === 'disetujui' ? 'success' : s === 'ditolak' ? 'danger' : 'warn'
}
function statusLabel(s) {
  return s === 'disetujui' ? 'Disetujui' : s === 'ditolak' ? 'Ditolak' : 'Menunggu Persetujuan'
}
function formatDateTime(v) {
  if (!v) return '-'
  return new Date(v).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}
function formatDate(v) {
  if (!v) return '-'
  return new Date(v).toLocaleDateString('id-ID', { dateStyle: 'medium' })
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Profil Saya</div>
    <p class="page-subtitle">Lihat data diri anda dan ajukan perubahan data jika ada yang perlu diperbarui (mis. jabatan, pangkat/golongan, atau unit kerja baru).</p>

    <div class="card">
      <div v-if="loading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <template v-else-if="profile">
        <div class="foto-profil-row">
          <div class="foto-profil-avatar">
            <img v-if="fotoUrl" :src="fotoUrl" alt="Foto profil" />
            <i v-else class="pi pi-user"></i>
          </div>
          <div>
            <div style="font-weight: 600; margin-bottom: 0.4rem">Foto Profil</div>
            <input ref="fotoFileInputRef" type="file" accept=".jpg,.jpeg,.png" style="display: none" @change="onFotoFileChosen" />
            <div style="display: flex; gap: 0.5rem; flex-wrap: wrap">
              <Button
                :label="profile.foto_profil_nama ? 'Ganti Foto' : 'Upload Foto'"
                icon="pi pi-upload"
                size="small"
                outlined
                :loading="fotoUploading"
                @click="fotoFileInputRef?.click()"
              />
              <Button v-if="profile.foto_profil_nama" label="Hapus Foto" icon="pi pi-trash" size="small" severity="danger" text @click="confirmHapusFoto" />
            </div>
            <small style="display: block; margin-top: 0.4rem; color: var(--p-text-muted-color)">
              Foto profil bisa anda ganti atau hapus sendiri kapan saja, langsung berlaku tanpa perlu persetujuan administrator/admin. Format JPG atau PNG.
            </small>
          </div>
        </div>

        <div class="detail-grid">
          <div><span class="detail-label">NIP</span><div>{{ profile.nip || '-' }}</div></div>
          <div><span class="detail-label">Nama</span><div>{{ profile.nama || '-' }}</div></div>
          <div><span class="detail-label">Jabatan</span><div>{{ profile.jabatan?.jabatan || '-' }}</div></div>
          <div>
            <span class="detail-label">Jenis Jabatan</span>
            <div>{{ jenisJabatanLabels[profile.jabatan?.jenis_jabatan] || '-' }}</div>
          </div>
          <div><span class="detail-label">Unit Kerja</span><div>{{ profile.unit_kerja?.unit || '-' }}</div></div>
          <div>
            <span class="detail-label">Pangkat / Golongan</span>
            <div>{{ profile.pangkat_gol?.pangkat?.pangkat || '-' }} / {{ profile.pangkat_gol?.gol?.gol || '-' }}</div>
          </div>
          <div><span class="detail-label">Tempat Tugas</span><div>{{ profile.tempat_tgs || '-' }}</div></div>
          <div><span class="detail-label">TMT</span><div>{{ formatDate(profile.tmt) }}</div></div>
          <div>
            <span class="detail-label">Tanggal Lahir</span>
            <div>
              {{ formatDate(profile.tgl_lahir) }}
              <span v-if="usiaSaya != null" style="color: var(--p-text-muted-color)">({{ usiaSaya }} tahun)</span>
            </div>
          </div>
          <div>
            <span class="detail-label">Kelayakan Pensiun</span>
            <div>
              <Tag
                v-if="usiaSaya != null"
                :value="sudahMemenuhiUsiaPensiun ? `Sudah memenuhi usia pensiun (${usiaPensiunSaya})` : `Belum (usia pensiun ${usiaPensiunSaya})`"
                :severity="sudahMemenuhiUsiaPensiun ? 'warn' : 'success'"
              />
              <span v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Isi Tanggal Lahir untuk menghitung</span>
            </div>
          </div>
          <div>
            <span class="detail-label">Kenaikan Gaji Berkala Terakhir</span>
            <div>
              {{ formatDate(profile.tgl_kenaikan_gaji_berkala_terakhir) }}
              <Button v-if="hasSkKgb" label="Lihat SK" text size="small" style="padding: 0 0.3rem" @click="previewSkKgbSaatIni" />
            </div>
          </div>
          <div>
            <span class="detail-label">Kelayakan Kenaikan Gaji Berkala</span>
            <div>
              <Tag
                v-if="kelayakanGajiBerkalaSaya.jatuhTempo"
                :value="kelayakanGajiBerkalaSaya.sudahWaktunya ? `Sudah waktunya (interval ${intervalGajiBerkalaSaya} tahun)` : `Jatuh tempo ${formatDate(kelayakanGajiBerkalaSaya.jatuhTempo)}`"
                :severity="kelayakanGajiBerkalaSaya.sudahWaktunya ? 'warn' : 'success'"
              />
              <span v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Isi tanggal kenaikan terakhir untuk menghitung</span>
            </div>
          </div>
          <div>
            <span class="detail-label">Kenaikan Pangkat Terakhir</span>
            <div>{{ formatDate(profile.tgl_kenaikan_pangkat_terakhir) }}</div>
          </div>
          <div>
            <span class="detail-label">Kelayakan Kenaikan Pangkat</span>
            <div>
              <Tag
                v-if="kelayakanPangkatSaya.jatuhTempo"
                :value="kelayakanPangkatSaya.sudahWaktunya ? `Sudah waktunya (interval ${intervalPangkatSaya} tahun)` : `Jatuh tempo ${formatDate(kelayakanPangkatSaya.jatuhTempo)}`"
                :severity="kelayakanPangkatSaya.sudahWaktunya ? 'warn' : 'success'"
              />
              <span v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Isi tanggal kenaikan terakhir untuk menghitung</span>
            </div>
          </div>
          <div><span class="detail-label">No HP</span><div>{{ profile.no_hp || '-' }}</div></div>
          <div><span class="detail-label">Email</span><div>{{ profile.email || '-' }}</div></div>
          <div><span class="detail-label">Status</span><div><Tag :value="profile.status?.status || '-'" severity="info" /></div></div>
          <div><span class="detail-label">Atasan Langsung</span><div>{{ profile.atasan?.nama || '-' }}</div></div>
        </div>

        <Message v-if="pendingRequest" severity="warn" :closable="false" style="margin-top: 1.25rem">
          Anda memiliki pengajuan perubahan data yang sedang menunggu persetujuan administrator/admin (diajukan {{ formatDateTime(pendingRequest.created_at) }}).
          Belum bisa mengajukan perubahan baru sampai pengajuan ini diproses, atau anda bisa membatalkannya di riwayat di bawah.
        </Message>

        <div style="margin-top: 1.25rem">
          <Button
            label="Ajukan Perubahan Data"
            icon="pi pi-pencil"
            :disabled="!!pendingRequest"
            @click="openAjukan"
          />
          <small style="display: block; margin-top: 0.5rem; color: var(--p-text-muted-color)">
            Setiap pengajuan perubahan data wajib disertai upload SK Terakhir sebagai dasar perubahan, dan baru berlaku setelah disetujui administrator/admin.
          </small>
        </div>
      </template>
    </div>

    <div class="card" style="margin-top: 1.25rem">
      <h4 style="margin-top: 0">Riwayat Pengajuan Perubahan Data</h4>
      <div v-if="riwayatLoading" style="display: flex; justify-content: center; padding: 1.5rem">
        <ProgressSpinner style="width: 2rem; height: 2rem" />
      </div>
      <div v-else-if="!riwayat.length" style="color: var(--p-text-muted-color); font-size: 0.85rem; padding: 0.5rem 0">
        Belum ada pengajuan perubahan data.
      </div>
      <template v-else>
        <div v-for="row in riwayat" :key="row.id" class="riwayat-row">
          <div class="riwayat-info">
            <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap">
              <Tag :value="statusLabel(row.status)" :severity="statusSeverity(row.status)" />
              <span style="font-size: 0.82rem; color: var(--p-text-muted-color)">Diajukan {{ formatDateTime(row.created_at) }}</span>
            </div>
            <div v-if="row.status !== 'pending'" style="font-size: 0.82rem; color: var(--p-text-muted-color); margin-top: 0.25rem">
              Diputuskan {{ formatDateTime(row.tgl_keputusan) }} oleh {{ row.diputuskan_oleh || '-' }}
              <template v-if="row.catatan_admin"> &mdash; "{{ row.catatan_admin }}"</template>
            </div>
          </div>
          <div class="riwayat-actions">
            <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat SK" @click="previewSk(row)" />
            <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadSk(row)" />
            <Button v-if="row.status === 'pending'" icon="pi pi-times" size="small" severity="danger" outlined label="Batalkan" @click="confirmBatal(row)" />
          </div>
        </div>
      </template>
    </div>

    <div class="card" style="margin-top: 1.25rem">
      <h4 style="margin-top: 0">Pensiun</h4>
      <template v-if="profile">
        <div class="detail-grid" style="margin-bottom: 1rem">
          <div>
            <span class="detail-label">Usia Saat Ini</span>
            <div>{{ usiaSaya != null ? `${usiaSaya} tahun` : 'Tanggal lahir belum diisi administrator' }}</div>
          </div>
          <div>
            <span class="detail-label">Usia Pensiun Standar Jabatan Anda</span>
            <div>{{ usiaPensiunSaya }} tahun</div>
          </div>
        </div>
        <Message v-if="usiaSaya != null" :severity="sudahMemenuhiUsiaPensiun ? 'warn' : 'info'" :closable="false" style="margin-bottom: 1rem">
          <template v-if="sudahMemenuhiUsiaPensiun">Usia anda sudah memenuhi usia pensiun standar. Anda bisa mengajukan pensiun kapan saja.</template>
          <template v-else>Usia anda belum memenuhi usia pensiun standar. Anda masih bisa mengajukan lewat opsi "Pensiun Dini" jika memang dikehendaki.</template>
        </Message>

        <Message v-if="pendingPensiunRequest" severity="warn" :closable="false" style="margin-bottom: 1.25rem">
          Anda memiliki pengajuan pensiun yang sedang menunggu persetujuan administrator/admin (diajukan {{ formatDateTime(pendingPensiunRequest.created_at) }}).
          Belum bisa mengajukan pensiun baru sampai pengajuan ini diproses, atau anda bisa membatalkannya di riwayat di bawah.
        </Message>

        <Button label="Ajukan Pensiun" icon="pi pi-briefcase" severity="warn" outlined :disabled="!!pendingPensiunRequest" @click="openAjukanPensiun" />
        <small style="display: block; margin-top: 0.5rem; color: var(--p-text-muted-color)">
          Jika disetujui administrator/admin, status anda otomatis berubah jadi "Pensiun" dan akun login anda otomatis dinonaktifkan (tidak bisa diakses lagi).
        </small>
      </template>

      <div v-if="riwayatPensiun.length" style="margin-top: 1.5rem">
        <h4 style="margin-bottom: 0.75rem">Riwayat Pengajuan Pensiun</h4>
        <div v-if="riwayatPensiunLoading" style="display: flex; justify-content: center; padding: 1.5rem">
          <ProgressSpinner style="width: 2rem; height: 2rem" />
        </div>
        <template v-else>
          <div v-for="row in riwayatPensiun" :key="row.id" class="riwayat-row">
            <div class="riwayat-info">
              <div style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap">
                <Tag :value="statusLabel(row.status)" :severity="statusSeverity(row.status)" />
                <Tag v-if="row.is_pensiun_dini" value="Pensiun Dini" severity="help" />
                <span style="font-size: 0.82rem; color: var(--p-text-muted-color)">Diajukan {{ formatDateTime(row.created_at) }}</span>
              </div>
              <div v-if="row.status !== 'pending'" style="font-size: 0.82rem; color: var(--p-text-muted-color); margin-top: 0.25rem">
                Diputuskan {{ formatDateTime(row.tgl_keputusan) }} oleh {{ row.diputuskan_oleh || '-' }}
                <template v-if="row.catatan_admin"> &mdash; "{{ row.catatan_admin }}"</template>
              </div>
            </div>
            <div class="riwayat-actions">
              <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat" @click="previewSkPensiun(row)" />
              <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadSkPensiun(row)" />
              <Button v-if="row.status === 'pending'" icon="pi pi-times" size="small" severity="danger" outlined label="Batalkan" @click="confirmBatalPensiun(row)" />
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Dialog Ajukan Pensiun -->
    <Dialog v-model:visible="pensiunDialogVisible" modal header="Ajukan Pensiun" :style="{ width: '32rem', maxWidth: '95vw' }">
      <Message v-if="pensiunFormErrors" severity="error" :closable="false" style="margin-bottom: 1rem">{{ pensiunFormErrors }}</Message>
      <Message severity="info" :closable="false" style="margin-bottom: 1rem">
        Pengajuan ini baru berlaku (status jadi Pensiun & akun dinonaktifkan) setelah disetujui administrator/admin.
      </Message>
      <div style="display: flex; flex-direction: column; gap: 1rem">
        <div style="display: flex; align-items: center; gap: 0.5rem">
          <Checkbox v-model="pensiunIsDini" inputId="chk-pensiun-dini" binary />
          <label for="chk-pensiun-dini" style="font-size: 0.9rem; font-weight: 600; cursor: pointer">Pensiun Dini</label>
        </div>
        <small v-if="!sudahMemenuhiUsiaPensiun && !pensiunIsDini" style="color: #ef4444">
          Usia anda belum memenuhi usia pensiun standar ({{ usiaPensiunSaya }} tahun) -- centang "Pensiun Dini" untuk melanjutkan.
        </small>
        <div v-if="pensiunIsDini">
          <label class="field-label">Alasan Pensiun Dini <span style="color: #ef4444">*</span></label>
          <Textarea v-model="pensiunAlasan" rows="3" style="width: 100%" placeholder="Jelaskan alasan pengajuan pensiun dini" />
        </div>
        <div>
          <label class="field-label">Berkas SK / Usulan Pensiun <span style="color: #ef4444">*</span></label>
          <input ref="pensiunFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onPensiunFileChosen" />
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <Button label="Pilih Berkas" icon="pi pi-upload" outlined @click="pickPensiunFile" />
            <span style="font-size: 0.85rem">{{ pensiunFile?.name || 'Belum ada berkas dipilih' }}</span>
          </div>
          <small style="color: var(--p-text-muted-color); display: block; margin-top: 0.3rem">Format PDF, JPG, atau PNG.</small>
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="pensiunDialogVisible = false" />
        <Button label="Kirim Pengajuan" icon="pi pi-send" :loading="pensiunSaving" @click="submitAjukanPensiun" />
      </template>
    </Dialog>

    <!-- Dialog Ajukan Perubahan Data -->
    <Dialog v-model:visible="dialogVisible" modal header="Ajukan Perubahan Data" :style="{ width: '38rem', maxWidth: '95vw' }">
      <Message v-if="formErrors" severity="error" :closable="false" style="margin-bottom: 1rem">{{ formErrors }}</Message>
      <Message severity="info" :closable="false" style="margin-bottom: 1rem">
        NIP tidak bisa diubah lewat form ini. Perubahan yang anda ajukan baru berlaku setelah disetujui administrator/admin.
      </Message>
      <div class="grid formgrid">
        <div class="col-12">
          <label class="field-label">Nama <span style="color: #ef4444">*</span></label>
          <InputText v-model="form.nama" style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Jabatan</label>
          <Select v-model="form.id_jabatan" :options="refJabatan" optionLabel="jabatan" optionValue="id" filter showClear style="width: 100%" placeholder="Pilih..." />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Unit Kerja</label>
          <Select v-model="form.id_unit_kerja" :options="refUnitKerja" optionLabel="unit" optionValue="id" filter showClear style="width: 100%" placeholder="Pilih..." />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Pangkat / Golongan</label>
          <Select
            v-model="form.id_pangkat_gol"
            :options="refPangkatGol"
            :optionLabel="(o) => `${o.pangkat?.pangkat || '-'} / ${o.gol?.gol || '-'}`"
            optionValue="id"
            filter
            showClear
            style="width: 100%"
            placeholder="Pilih..."
          />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Status Kepegawaian</label>
          <Select v-model="form.id_status" :options="refStatus" optionLabel="status" optionValue="id" filter showClear style="width: 100%" placeholder="Pilih..." />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Tempat Tugas</label>
          <InputText v-model="form.tempat_tgs" style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">TMT</label>
          <DatePicker v-model="form.tmt" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Tanggal Lahir</label>
          <DatePicker v-model="form.tgl_lahir" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
          <small style="color: var(--p-text-muted-color)">Dipakai untuk menghitung usia & kelayakan pensiun anda.</small>
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">No HP</label>
          <InputText v-model="form.no_hp" style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Email</label>
          <InputText v-model="form.email" style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">
            Kenaikan Gaji Berkala Terakhir
            <span style="font-weight: 400; color: var(--p-text-muted-color)">(opsional)</span>
          </label>
          <DatePicker v-model="form.tgl_kenaikan_gaji_berkala_terakhir" dateFormat="dd-mm-yy" showIcon showButtonBar style="width: 100%" />
          <small style="color: var(--p-text-muted-color)">Dipakai menghitung kapan kenaikan gaji berkala berikutnya jatuh tempo.</small>
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">
            Kenaikan Pangkat Terakhir
            <span style="font-weight: 400; color: var(--p-text-muted-color)">(opsional)</span>
          </label>
          <DatePicker v-model="form.tgl_kenaikan_pangkat_terakhir" dateFormat="dd-mm-yy" showIcon showButtonBar style="width: 100%" />
          <small style="color: var(--p-text-muted-color)">Dipakai menghitung kapan kenaikan pangkat berikutnya jatuh tempo.</small>
        </div>
        <div class="col-12">
          <label class="field-label">
            Berkas SK Kenaikan Gaji Berkala
            <span style="font-weight: 400; color: var(--p-text-muted-color)">(opsional)</span>
          </label>
          <div v-if="hasSkKgb" style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; margin-bottom: 0.5rem">
            <Tag severity="info" value="SK Tersimpan" />
            <span style="font-size: 0.85rem">{{ profile.sk_kgb_nama }}</span>
            <Button label="Lihat SK Saat Ini" icon="pi pi-eye" text size="small" @click="previewSkKgbSaatIni" />
          </div>
          <input ref="skKgbFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onSkKgbFileChosen" />
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <Button :label="hasSkKgb ? 'Ganti Berkas' : 'Pilih Berkas'" icon="pi pi-upload" outlined @click="pickSkKgbFile" />
            <span style="font-size: 0.85rem">{{ skKgbFile?.name || 'Belum ada berkas dipilih' }}</span>
          </div>
          <small style="color: var(--p-text-muted-color); display: block; margin-top: 0.3rem">
            Tidak wajib. Isi hanya jika ingin memperbarui tanggal &amp; bukti kenaikan gaji berkala terakhir anda.
          </small>
        </div>
        <div class="col-12">
          <label class="field-label">
            Berkas SK Terakhir
            <span v-if="!hasSkTerakhir" style="color: #ef4444">*</span>
            <span v-else style="font-weight: 400; color: var(--p-text-muted-color)">(opsional jika tidak diganti)</span>
          </label>
          <div v-if="hasSkTerakhir" style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; margin-bottom: 0.5rem">
            <Tag severity="info" value="SK Tersimpan" />
            <span style="font-size: 0.85rem">{{ profile.sk_terakhir_nama }}</span>
            <Button label="Lihat SK Saat Ini" icon="pi pi-eye" text size="small" @click="previewSkTerakhirSaatIni" />
          </div>
          <input ref="skFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onSkFileChosen" />
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <Button :label="hasSkTerakhir ? 'Ganti Berkas' : 'Pilih Berkas'" icon="pi pi-upload" outlined @click="pickSkFile" />
            <span style="font-size: 0.85rem">{{ skFile?.name || 'Belum ada berkas dipilih' }}</span>
          </div>
          <small style="color: var(--p-text-muted-color); display: block; margin-top: 0.3rem">
            <template v-if="hasSkTerakhir">Jika SK terakhir sudah sesuai dan tidak ingin diganti, berkas ini tidak perlu diupload ulang. Upload berkas baru hanya jika ingin mengganti SK.</template>
            <template v-else>Format PDF, JPG, atau PNG. Berkas ini menjadi dasar & bukti perubahan data yang diajukan.</template>
          </small>
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="dialogVisible = false" />
        <Button label="Kirim Pengajuan" icon="pi pi-send" :loading="saving" @click="submitAjukan" />
      </template>
    </Dialog>

    <!-- Preview dokumen SK -->
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
.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
}

.foto-profil-row {
  display: flex;
  align-items: center;
  gap: 1.1rem;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}

.foto-profil-avatar {
  width: 84px;
  height: 84px;
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
  font-size: 2.1rem;
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

.riwayat-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.75rem 0;
  border-bottom: 1px solid #f1f5f9;
  flex-wrap: wrap;
}

.riwayat-row:last-child {
  border-bottom: none;
}

.riwayat-actions {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}
</style>
