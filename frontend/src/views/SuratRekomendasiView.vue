<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { useBulkDelete } from '../composables/useBulkDelete'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import RadioButton from 'primevue/radiobutton'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'

// SuratRekomendasiView -- menu "Surat Rekomendasi" (khusus administrator &
// admin): mengirim surat rekomendasi perpanjangan kontrak untuk pegawai
// PPPK/PPPK Paruh Waktu, satu-satu ATAU sekaligus (kolektif, difilter dari
// Status Kepegawaian & tahun TMT). Begitu dikirim, surat langsung muncul di
// menu "Arsip Surat" akun pegawai penerimanya (lihat ArsipSuratView.vue) --
// TIDAK ada proses approval terpisah. Backend: handlers/surat_rekomendasi.go.

const toast = useToast()
const confirm = useConfirm()
// Tampilan kartu (bukan DataTable) -- checkbox centang-banyak dikelola
// manual (push/splice ke `selected`, lihat komentar di useBulkDelete.js)
// lewat isItemSelected/toggleItemSelect di bawah, bukan v-model:selection.
const { selected: selectedItems, bulkDeleting, confirmBulkDelete } = useBulkDelete()

function isItemSelected(item) {
  return selectedItems.value.some((i) => i.id === item.id)
}
function toggleItemSelect(item) {
  const idx = selectedItems.value.findIndex((i) => i.id === item.id)
  if (idx === -1) selectedItems.value.push(item)
  else selectedItems.value.splice(idx, 1)
}
function hapusTerpilih() {
  confirmBulkDelete({
    label: 'surat rekomendasi',
    deleteOne: (item) => http.delete(`/surat-rekomendasi/${item.id}`),
    onDone: loadItems,
  })
}

const items = ref([])
const loading = ref(true)
const search = ref('')
const filterTahun = ref(null)
let searchTimer = null

async function loadItems() {
  loading.value = true
  selectedItems.value = []
  try {
    const params = {}
    if (search.value.trim()) params.q = search.value.trim()
    if (filterTahun.value) params.tahun = filterTahun.value
    const { data } = await http.get('/surat-rekomendasi', { params })
    items.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(loadItems, 300)
})
watch(filterTahun, loadItems)

onMounted(loadItems)

function formatTanggal(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'long', day: 'numeric' })
}

// ============================================================
// dialog "Kirim Surat Rekomendasi"
// ============================================================
const kirimDialog = ref(false)
const kirimSubmitting = ref(false)

const judul = ref('')
const tanggalSurat = ref(new Date())
const tahunAktif = computed(() => tanggalSurat.value?.getFullYear() || new Date().getFullYear())

const nomorInfo = ref({ nomor_urut: 1, editable: true })
const nomorUrutAwal = ref(null)
// modeNomor/nomorUrutAwalManual: HANYA relevan saat nomorInfo.editable === false
// (sudah ada surat tahun ini) -- admin boleh pilih "lanjut" (kelanjutan otomatis,
// perilaku lama/default) ATAU "manual" (sengaja mulai ulang dari nomor baru di
// tengah tahun, mis. untuk memperbaiki kesalahan penomoran). Lihat validasi
// bentrok nomor di backend (buatSuratRekomendasi, surat_rekomendasi.go).
const modeNomor = ref('lanjut')
const nomorUrutAwalManual = ref(null)
let nomorInfoTimer = null

async function refreshNomorInfo() {
  try {
    const { data } = await http.get('/surat-rekomendasi/next-nomor', { params: { tahun: tahunAktif.value } })
    nomorInfo.value = data.data
    if (data.data.editable) {
      nomorUrutAwal.value = nomorUrutAwal.value || data.data.nomor_urut
    } else {
      modeNomor.value = 'lanjut'
      nomorUrutAwalManual.value = null
    }
  } catch {
    nomorInfo.value = { nomor_urut: 1, editable: true }
  }
}

watch(tahunAktif, () => {
  clearTimeout(nomorInfoTimer)
  nomorInfoTimer = setTimeout(refreshNomorInfo, 200)
})

// --- filter & daftar calon pegawai ---
const refStatus = ref([])
const statusOptions = computed(() =>
  refStatus.value
    .filter((s) => /pppk/i.test(s.status))
    .map((s) => ({ label: s.status, value: s.id }))
)
const refTahunTmt = ref([])

// filterIdStatus: array id status -- MultiSelect supaya admin bisa memilih
// PPPK & PPPK Paruh Waktu SEKALIGUS (dikirim kolektif dalam satu batch),
// atau tetap bisa memilih satu status saja (kirim satu-satu/per status).
const filterIdStatus = ref([])
const filterTahunTmt = ref(null)
const filterCariNama = ref('')
const calonLoading = ref(false)
const calonList = ref([])
const selectedIds = ref(new Set())

async function loadRefStatus() {
  try {
    const { data } = await http.get('/ref/status')
    refStatus.value = data.data || []
  } catch {
    refStatus.value = []
  }
}

async function loadRefTahunTmt() {
  try {
    // dipakai bersama menu Penerima TPP -- daftar tahun TMT yang BENAR-BENAR
    // ADA pada data pegawai, bukan rentang tahun bebas (lihat
    // listTahunTmtPegawai di handlers/tpp.go).
    const { data } = await http.get('/tpp/tahun-tmt')
    refTahunTmt.value = data.data || []
  } catch {
    refTahunTmt.value = []
  }
}

async function cariCalonPegawai() {
  calonLoading.value = true
  try {
    const params = {}
    if (filterIdStatus.value?.length) params.id_status = filterIdStatus.value.join(',')
    if (filterTahunTmt.value) params.tahun_tmt = filterTahunTmt.value
    if (filterCariNama.value.trim()) params.q = filterCariNama.value.trim()
    const { data } = await http.get('/surat-rekomendasi/calon-pegawai', { params })
    calonList.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mencari pegawai', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    calonLoading.value = false
  }
}

function toggleSelect(row) {
  const s = new Set(selectedIds.value)
  if (s.has(row.id)) s.delete(row.id)
  else s.add(row.id)
  selectedIds.value = s
}

const semuaTerpilih = computed(() => calonList.value.length > 0 && calonList.value.every((r) => selectedIds.value.has(r.id)))
function toggleSemua() {
  const s = new Set(selectedIds.value)
  if (semuaTerpilih.value) {
    for (const r of calonList.value) s.delete(r.id)
  } else {
    for (const r of calonList.value) s.add(r.id)
  }
  selectedIds.value = s
}

const jumlahTerpilih = computed(() => selectedIds.value.size)

function statusPegawai(row) {
  return row.status?.status || '-'
}

function bukaKirimDialog() {
  judul.value = `Surat Rekomendasi Tahun ${new Date().getFullYear()}`
  tanggalSurat.value = new Date()
  nomorUrutAwal.value = null
  modeNomor.value = 'lanjut'
  nomorUrutAwalManual.value = null
  filterIdStatus.value = []
  filterTahunTmt.value = null
  filterCariNama.value = ''
  calonList.value = []
  selectedIds.value = new Set()
  if (refStatus.value.length === 0) loadRefStatus()
  if (refTahunTmt.value.length === 0) loadRefTahunTmt()
  refreshNomorInfo()
  kirimDialog.value = true
}

function toDateStr(d) {
  if (!d) return ''
  const dt = new Date(d)
  const pad = (n) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
}

async function kirimSurat() {
  if (!judul.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Judul surat wajib diisi', life: 4000 })
    return
  }
  if (!tanggalSurat.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Tanggal surat wajib diisi', life: 4000 })
    return
  }
  if (jumlahTerpilih.value === 0) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih minimal satu pegawai penerima', life: 4000 })
    return
  }
  if (nomorInfo.value.editable && (!nomorUrutAwal.value || nomorUrutAwal.value < 1)) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: `Ini surat pertama untuk tahun ${tahunAktif.value} -- isi nomor urut awal`, life: 5000 })
    return
  }
  if (!nomorInfo.value.editable && modeNomor.value === 'manual' && (!nomorUrutAwalManual.value || nomorUrutAwalManual.value < 1)) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Isi nomor urut awal yang baru', life: 4000 })
    return
  }
  kirimSubmitting.value = true
  try {
    const payload = {
      judul: judul.value.trim(),
      tanggal_surat: toDateStr(tanggalSurat.value),
      tahun: tahunAktif.value,
      id_pegawai: Array.from(selectedIds.value),
    }
    if (nomorInfo.value.editable) payload.nomor_urut_awal = nomorUrutAwal.value
    else if (modeNomor.value === 'manual') payload.nomor_urut_awal = nomorUrutAwalManual.value
    const { data } = await http.post('/surat-rekomendasi', payload)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
    kirimDialog.value = false
    await loadItems()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengirim', detail: e.response?.data?.message || e.message, life: 7000 })
  } finally {
    kirimSubmitting.value = false
  }
}

// ============================================================
// preview / unduh / hapus
// ============================================================
const previewDialog = ref(false)
const previewTitle = ref('')
const previewPdfUrl = ref('')
let previewObjectUrl = ''

async function lihatSurat(item) {
  previewTitle.value = `${item.judul} -- ${item.nama_pegawai}`
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf`, { params: { inline: 1 }, responseType: 'blob' })
    previewObjectUrl = URL.createObjectURL(res.data)
    previewPdfUrl.value = previewObjectUrl
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal membuka surat', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

function tutupPreview() {
  previewDialog.value = false
  if (previewObjectUrl) {
    URL.revokeObjectURL(previewObjectUrl)
    previewObjectUrl = ''
  }
  previewPdfUrl.value = ''
}

async function unduhSurat(item) {
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `surat_rekomendasi_${item.nip_pegawai}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

// ------------------------------------------------------------
// Lampiran 1 (surat permohonan perpanjangan kontrak yang DIAJUKAN PEGAWAI
// SENDIRI ke Bupati) -- berkas KEDUA dari baris surat rekomendasi yang
// sama, supaya pegawai (lihat ArsipSuratView.vue) bisa mengunduh dua
// berkas sekaligus; disediakan juga di sisi admin ini supaya admin bisa
// mengecek isinya. Tidak perlu nomor surat/QR, lihat
// pdfPermohonanSuratRekomendasi di handlers/surat_rekomendasi.go.
// ------------------------------------------------------------

async function lihatLampiran1(item) {
  previewTitle.value = `${item.judul} -- ${item.nama_pegawai} -- Lampiran 1 (Permohonan)`
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf-permohonan`, { params: { inline: 1 }, responseType: 'blob' })
    previewObjectUrl = URL.createObjectURL(res.data)
    previewPdfUrl.value = previewObjectUrl
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal membuka surat', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

async function unduhLampiran1(item) {
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf-permohonan`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `permohonan_perpanjangan_kontrak_${item.nip_pegawai}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

function konfirmasiHapus(item) {
  confirm.require({
    message: `Hapus surat rekomendasi "${item.nomor_surat}" milik ${item.nama_pegawai}? Nomor ini TIDAK akan dipakai ulang.`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    accept: () => doHapus(item),
  })
}

async function doHapus(item) {
  try {
    const { data } = await http.delete(`/surat-rekomendasi/${item.id}`)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 4000 })
    await loadItems()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

// ------------------------------------------------------------
// toggleTampil: KHUSUS surat sekolah ("Lampiran 3", item.is_sekolah) --
// administrator bisa menampilkan/menyembunyikan surat ini dari menu Arsip
// Surat akun pegawai penerimanya (mis. kalau surat masih belum final atau
// administrasinya masih ditangani sekolah sendiri). Lihat
// updateTampilSuratRekomendasi di handlers/surat_rekomendasi.go --
// perubahan berlaku langsung, tidak mempengaruhi akses akun atasan.
// ------------------------------------------------------------
const tampilSaving = ref(new Set())

async function toggleTampil(item, nilaiBaru) {
  const saving = new Set(tampilSaving.value)
  saving.add(item.id)
  tampilSaving.value = saving
  try {
    const { data } = await http.put(`/surat-rekomendasi/${item.id}/tampil`, { tampil: nilaiBaru })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 4000 })
    item.tampil_ke_pegawai = nilaiBaru
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengubah', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    const s = new Set(tampilSaving.value)
    s.delete(item.id)
    tampilSaving.value = s
  }
}

// ------------------------------------------------------------
// sembunyikanSemuaLampiran3 -- tombol "Sembunyikan Semua Lampiran 3 dari
// Akun Pegawai": menyembunyikan SEKALIGUS SEMUA surat sekolah/puskesmas
// dari SEMUA tahun dalam satu klik, pengganti toggle satu-satu per kartu
// kalau administrator ingin menyembunyikan semuanya sekaligus. Lihat
// sembunyikanSemuaLampiran3 di handlers/surat_rekomendasi.go.
// ------------------------------------------------------------
const sembunyikanSemuaLoading = ref(false)

function konfirmasiSembunyikanSemuaLampiran3() {
  confirm.require({
    message: 'Sembunyikan SEMUA surat sekolah/puskesmas (Lampiran 3) dari SEMUA tahun dari menu Arsip Surat akun pegawai? Surat Dinas/Kantor (Lampiran 2) tidak terpengaruh, dan akun atasan tetap bisa melihat & menyetujui bawahannya seperti biasa. Bisa ditampilkan lagi satu-satu lewat toggle di tiap kartu.',
    header: 'Sembunyikan Semua Lampiran 3?',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Sembunyikan Semua',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'warn' },
    accept: doSembunyikanSemuaLampiran3,
  })
}

async function doSembunyikanSemuaLampiran3() {
  sembunyikanSemuaLoading.value = true
  try {
    const { data } = await http.put('/surat-rekomendasi/sembunyikan-semua-lampiran3')
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
    await loadItems()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyembunyikan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    sembunyikanSemuaLoading.value = false
  }
}

// ============================================================
// unduh Excel (Nomor Surat -> Nama Pegawai) -- difilter dari TANGGAL
// SURAT (tanggal dikirimnya surat rekomendasi, bukan tanggal dibuatnya
// baris), lihat exportSuratRekomendasi di handlers/surat_rekomendasi.go.
// Kedua tanggal opsional & boleh dipakai sendiri-sendiri.
// ============================================================
const excelDialog = ref(false)
const excelDari = ref(null)
const excelSampai = ref(null)
const excelLoading = ref(false)

function bukaExcelDialog() {
  excelDari.value = null
  excelSampai.value = null
  excelDialog.value = true
}

function toDateStrExcel(d) {
  if (!d) return ''
  const dt = new Date(d)
  const pad = (n) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
}

async function unduhExcel() {
  if (excelDari.value && excelSampai.value && excelDari.value > excelSampai.value) {
    toast.add({ severity: 'warn', summary: 'Rentang tanggal tidak valid', detail: '"Dari Tanggal" harus sebelum atau sama dengan "Sampai Tanggal"', life: 5000 })
    return
  }
  excelLoading.value = true
  try {
    const params = {}
    if (excelDari.value) params.tanggal_mulai = toDateStrExcel(excelDari.value)
    if (excelSampai.value) params.tanggal_selesai = toDateStrExcel(excelSampai.value)
    const res = await http.get('/surat-rekomendasi/export', { params, responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = 'surat_rekomendasi.xlsx'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    excelDialog.value = false
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh Excel', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    excelLoading.value = false
  }
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-header-row">
      <div>
        <div class="page-title">Surat Rekomendasi</div>
        <p class="page-subtitle">
          Kirim surat rekomendasi perpanjangan kontrak untuk pegawai PPPK/PPPK Paruh Waktu -- satu-satu atau
          sekaligus (kolektif, difilter dari Status Kepegawaian &amp; tahun TMT). Surat langsung muncul di menu
          Arsip Surat akun pegawai penerimanya.
        </p>
      </div>
      <div class="page-header-actions">
        <Button
          v-if="selectedItems.length"
          :label="`Hapus Terpilih (${selectedItems.length})`"
          icon="pi pi-trash"
          severity="danger"
          outlined
          :loading="bulkDeleting"
          @click="hapusTerpilih"
        />
        <Button
          label="Sembunyikan Semua Lampiran 3"
          icon="pi pi-eye-slash"
          severity="warn"
          outlined
          :loading="sembunyikanSemuaLoading"
          @click="konfirmasiSembunyikanSemuaLampiran3"
        />
        <Button label="Unduh Excel" icon="pi pi-file-excel" severity="secondary" outlined @click="bukaExcelDialog" />
        <Button label="Kirim Surat Rekomendasi" icon="pi pi-send" @click="bukaKirimDialog" />
      </div>
    </div>

    <div class="toolbar-row">
      <IconField class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
        <InputText v-model="search" placeholder="Cari nama atau NIP..." style="width: 100%" />
        <InputIcon class="pi pi-search" />
      </IconField>
      <InputNumber v-model="filterTahun" placeholder="Filter tahun" :useGrouping="false" style="width: 140px" showClear />
    </div>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <Message v-else-if="!items.length" severity="info" :closable="false">
      Belum ada surat rekomendasi yang dikirim. Klik "Kirim Surat Rekomendasi" untuk mulai.
    </Message>

    <div v-else class="rekom-grid">
      <div v-for="item in items" :key="item.id" class="rekom-card">
        <div class="rekom-card-select">
          <Checkbox :modelValue="isItemSelected(item)" binary @update:modelValue="toggleItemSelect(item)" />
        </div>
        <div class="rekom-card-icon"><i class="pi pi-file-pdf"></i></div>
        <div class="rekom-card-body">
          <div class="rekom-card-title" :title="item.judul">{{ item.judul }}</div>
          <div class="rekom-card-nama">{{ item.nama_pegawai }}</div>
          <div class="rekom-card-meta">{{ item.nip_pegawai }} &middot; {{ item.jabatan || '-' }}</div>
          <div class="rekom-card-meta">{{ item.status_kepegawaian || '-' }} &middot; {{ item.unit_kerja || '-' }}</div>
          <div class="rekom-card-nomor">{{ item.nomor_surat }}</div>
          <div class="rekom-card-meta">{{ formatTanggal(item.tanggal_surat) }}</div>
          <Tag
            v-if="item.is_sekolah"
            :value="item.status_approval === 'pending' ? 'Menunggu Persetujuan Atasan' : 'Disetujui Atasan'"
            :severity="item.status_approval === 'pending' ? 'warn' : 'success'"
            class="rekom-card-status"
          />
          <div v-if="item.is_sekolah" class="rekom-card-tampil">
            <ToggleSwitch
              :modelValue="item.tampil_ke_pegawai"
              :disabled="tampilSaving.has(item.id)"
              @update:modelValue="(v) => toggleTampil(item, v)"
            />
            <span>{{ item.tampil_ke_pegawai ? 'Tampil di akun pegawai' : 'Disembunyikan dari akun pegawai' }}</span>
          </div>
        </div>
        <div class="rekom-card-actions">
          <Button label="Lihat" icon="pi pi-eye" size="small" @click="lihatSurat(item)" />
          <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="unduhSurat(item)" title="Unduh Surat Rekomendasi" />
          <Button icon="pi pi-trash" size="small" severity="danger" outlined @click="konfirmasiHapus(item)" title="Hapus" />
        </div>
        <div class="rekom-card-lampiran">
          <span class="rekom-card-lampiran-label">Lampiran 1 (Permohonan):</span>
          <Button icon="pi pi-eye" size="small" severity="secondary" text @click="lihatLampiran1(item)" title="Lihat Lampiran 1" />
          <Button icon="pi pi-download" size="small" severity="secondary" text @click="unduhLampiran1(item)" title="Unduh Lampiran 1" />
        </div>
      </div>
    </div>

    <!-- dialog kirim -->
    <Dialog v-model:visible="kirimDialog" modal header="Kirim Surat Rekomendasi" style="width: 46rem; max-width: 96vw">
      <div class="form-grid-2">
        <div class="form-field">
          <label>Judul Surat</label>
          <InputText v-model="judul" style="width: 100%" />
        </div>
        <div class="form-field">
          <label>Tanggal Surat</label>
          <DatePicker v-model="tanggalSurat" dateFormat="dd/mm/yy" showIcon style="width: 100%" />
        </div>
      </div>

      <Message v-if="nomorInfo.editable" severity="warn" :closable="false" style="margin-bottom: 1rem">
        Ini surat rekomendasi PERTAMA untuk tahun {{ tahunAktif }} -- isi nomor urut awal (kode klasifikasi 800.1.11
        &amp; bulan/tahun otomatis mengikuti). Surat berikutnya di tahun {{ tahunAktif }} akan melanjutkan nomor ini
        secara otomatis.
      </Message>
      <div v-if="nomorInfo.editable" class="form-field">
        <label>Nomor Urut Awal</label>
        <InputNumber v-model="nomorUrutAwal" :min="1" :useGrouping="false" showButtons style="width: 12rem" />
      </div>

      <template v-else>
        <div class="form-field">
          <label>Nomor Urut Surat Baru</label>
          <div style="display: flex; flex-direction: column; gap: 0.5rem; margin-top: 0.4rem">
            <label style="display: flex; align-items: center; gap: 0.5rem; font-weight: 400; cursor: pointer">
              <RadioButton v-model="modeNomor" value="lanjut" />
              Lanjutkan otomatis dari surat sebelumnya (nomor <b>{{ nomorInfo.nomor_urut }}</b>)
            </label>
            <label style="display: flex; align-items: center; gap: 0.5rem; font-weight: 400; cursor: pointer">
              <RadioButton v-model="modeNomor" value="manual" />
              Mulai dari nomor baru
            </label>
          </div>
        </div>
        <div v-if="modeNomor === 'manual'" class="form-field">
          <label>Nomor Urut Awal (Baru)</label>
          <InputNumber v-model="nomorUrutAwalManual" :min="1" :useGrouping="false" showButtons style="width: 12rem" />
          <small style="display: block; margin-top: 0.35rem; color: var(--p-text-muted-color, #64748b)">
            Hanya berlaku untuk pegawai BARU di pengiriman ini -- pegawai yang sudah pernah dikirimi surat tahun
            {{ tahunAktif }} tetap mempertahankan nomor &amp; tanggal surat lamanya.
          </small>
        </div>
      </template>

      <div class="divider-label">Pilih Pegawai Penerima</div>
      <div class="filter-row">
        <MultiSelect
          v-model="filterIdStatus"
          :options="statusOptions"
          optionLabel="label"
          optionValue="value"
          placeholder="Status Kepegawaian"
          display="chip"
          showClear
          style="min-width: 220px"
        />
        <Select
          v-model="filterTahunTmt"
          :options="refTahunTmt.map((t) => ({ label: String(t), value: t }))"
          optionLabel="label"
          optionValue="value"
          placeholder="Tahun TMT"
          showClear
          style="min-width: 150px"
        />
        <InputText v-model="filterCariNama" placeholder="Cari nama/NIP..." style="min-width: 180px; flex: 1" />
        <Button label="Cari" icon="pi pi-search" @click="cariCalonPegawai" :loading="calonLoading" />
      </div>

      <div v-if="calonList.length" class="responsive-table-wrap" style="margin-top: 0.75rem">
        <DataTable :value="calonList" size="small" scrollable scrollHeight="280px" style="min-width: 500px">
          <Column style="width: 3rem">
            <template #header>
              <Checkbox :modelValue="semuaTerpilih" binary @update:modelValue="toggleSemua" />
            </template>
            <template #body="{ data: row }">
              <Checkbox :modelValue="selectedIds.has(row.id)" binary @update:modelValue="toggleSelect(row)" />
            </template>
          </Column>
          <Column header="Nama">
            <template #body="{ data: row }">
              <div>{{ row.nama }}</div>
              <div style="font-size: 0.78rem; color: var(--p-text-muted-color, #64748b)">{{ row.nip }}</div>
            </template>
          </Column>
          <Column header="Status">
            <template #body="{ data: row }">{{ statusPegawai(row) }}</template>
          </Column>
          <Column header="TMT">
            <template #body="{ data: row }">{{ row.tmt ? formatTanggal(row.tmt) : '-' }}</template>
          </Column>
        </DataTable>
      </div>
      <Message v-else severity="secondary" :closable="false" style="margin-top: 0.75rem">
        Belum ada hasil -- atur filter Status Kepegawaian/Tahun TMT atau ketik nama/NIP, lalu klik "Cari".
      </Message>

      <div v-if="jumlahTerpilih" class="terpilih-info">{{ jumlahTerpilih }} pegawai terpilih</div>

      <template #footer>
        <Button label="Batal" severity="secondary" text @click="kirimDialog = false" />
        <Button :label="`Kirim${jumlahTerpilih ? ' (' + jumlahTerpilih + ')' : ''}`" icon="pi pi-send" :loading="kirimSubmitting" :disabled="jumlahTerpilih === 0" @click="kirimSurat" />
      </template>
    </Dialog>

    <!-- dialog preview -->
    <Dialog v-model:visible="previewDialog" modal :header="previewTitle" style="width: 90vw; max-width: 900px" @hide="tutupPreview">
      <iframe :src="previewPdfUrl" class="preview-frame"></iframe>
    </Dialog>

    <!-- dialog unduh Excel -->
    <Dialog v-model:visible="excelDialog" modal header="Unduh Excel Surat Rekomendasi" style="width: 30rem; max-width: 96vw">
      <p class="page-subtitle" style="margin-top: 0">
        Excel berisi Nomor Surat &amp; Nama Pegawai (beserta NIP, Jabatan, Unit Kerja, Status Kepegawaian, Judul), difilter
        berdasarkan tanggal surat dikirim. Kosongkan salah satu atau kedua tanggal untuk mengunduh semua surat.
      </p>
      <div class="form-grid-2">
        <div class="form-field">
          <label>Dari Tanggal Dikirim</label>
          <DatePicker v-model="excelDari" dateFormat="dd/mm/yy" showIcon showButtonBar style="width: 100%" />
        </div>
        <div class="form-field">
          <label>Sampai Tanggal Dikirim</label>
          <DatePicker v-model="excelSampai" dateFormat="dd/mm/yy" showIcon showButtonBar style="width: 100%" />
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" text @click="excelDialog = false" />
        <Button label="Unduh Excel" icon="pi pi-file-excel" :loading="excelLoading" @click="unduhExcel" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page-header-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.page-header-actions {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.toolbar-row {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.rekom-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 1rem;
}

.rekom-card {
  position: relative;
  background: var(--p-content-background, #fff);
  border-radius: 12px;
  padding: 1.1rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.rekom-card-select {
  position: absolute;
  top: 0.75rem;
  right: 0.75rem;
}

.rekom-card-icon {
  font-size: 2rem;
  color: #b91c1c;
}

.rekom-card-title {
  font-weight: 700;
  font-size: 0.95rem;
}

.rekom-card-nama {
  font-weight: 600;
  font-size: 0.9rem;
}

.rekom-card-meta {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.78rem;
}

.rekom-card-nomor {
  font-size: 0.8rem;
  font-family: monospace;
  color: #0d9488;
}

.rekom-card-status {
  align-self: flex-start;
  margin-top: 0.15rem;
}

.rekom-card-tampil {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.3rem;
  font-size: 0.76rem;
  color: var(--p-text-muted-color, #64748b);
}

.rekom-card-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: auto;
  padding-top: 0.4rem;
}

.rekom-card-lampiran {
  display: flex;
  align-items: center;
  gap: 0.15rem;
  padding-top: 0.4rem;
  border-top: 1px dashed var(--p-content-border-color, #e2e8f0);
}

.rekom-card-lampiran-label {
  font-size: 0.72rem;
  color: var(--p-text-muted-color, #64748b);
  margin-right: auto;
}

.form-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.form-field {
  margin-bottom: 1rem;
}

.form-field label {
  display: block;
  font-size: 0.82rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
  color: var(--p-text-muted-color, #64748b);
}

.divider-label {
  font-weight: 700;
  font-size: 0.9rem;
  margin: 1rem 0 0.5rem;
  border-top: 1px solid var(--p-content-border-color, #e2e8f0);
  padding-top: 1rem;
}

.filter-row {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.terpilih-info {
  margin-top: 0.6rem;
  font-size: 0.85rem;
  font-weight: 600;
  color: #0d9488;
}

.preview-frame {
  width: 100%;
  height: 75vh;
  border: none;
}
</style>
