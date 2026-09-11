<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { useAuthStore } from '../stores/auth'
import http from '../api/http'
import { toApiDate } from '../utils/date'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import Dialog from 'primevue/dialog'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Textarea from 'primevue/textarea'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Checkbox from 'primevue/checkbox'

const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const items = ref([])
const total = ref(0)
const loading = ref(false)
// Pilihan "tampilkan N entri" -- dropdown custom di pojok kiri atas tabel,
// dipisah dari paginator bawaan PrimeVue (lihat CrudManager.vue untuk pola
// yang sama, dipakai konsisten di semua tabel data pada aplikasi ini).
const entriesOptions = [5, 10, 25, 50, 100]

const page = ref(1)
const pageSize = ref(10)
const statusFilter = ref(null)
const search = ref('')

const jenisCutiOptions = ref([])
const pegawaiOptions = ref([])

// Checklist kelengkapan berkas per jenis cuti (cermin dari aturan backend --
// lihat dokumenRequirementsForJenis di backend/handlers/pengajuan_cuti.go).
// Urutan pengecekan penting: kata kunci yang lebih spesifik (melahirkan/
// umroh/sakit/alasan penting) dicek sebelum kata kunci umum "tahunan",
// karena "Cuti Tahunan Umroh" mengandung kedua kata "tahunan" dan "umroh".
//
// "Surat Rekomendasi Kepala Sekolah" hanya berlaku untuk pegawai yang tempat
// tugasnya di sekolah (yang punya Kepala Sekolah untuk menandatanganinya) --
// pegawai yang tempat tugasnya di Dinas tidak pernah diminta berkas ini.
function dokumenRequirementsForJenis(jenisNama, isSekolah) {
  const rekomendasiKepsek = { key: 'rekomendasi_kepsek', label: 'Surat Rekomendasi Kepala Sekolah', required: true }
  const withRekomendasiKepsek = (...reqs) => (isSekolah ? [rekomendasiKepsek, ...reqs] : reqs)

  const j = (jenisNama || '').toLowerCase()
  if (j.includes('melahirkan')) {
    return withRekomendasiKepsek(
      { key: 'sk_terakhir', label: 'SK Terakhir', required: true },
      { key: 'keterangan_hpl', label: 'Surat Keterangan HPL (Rumah Sakit/Puskesmas)', required: true },
      { key: 'buku_kia', label: 'Buku KIA', required: true },
      { key: 'hasil_usg', label: 'Hasil USG', required: false },
    )
  }
  if (j.includes('umroh')) {
    return withRekomendasiKepsek(
      { key: 'sk_terakhir', label: 'SK Terakhir', required: true },
      { key: 'keterangan_travel', label: 'Surat Keterangan dari Travel Pemberangkatan', required: true },
    )
  }
  if (j.includes('sakit')) {
    return [
      { key: 'sk_terakhir', label: 'SK Terakhir', required: true },
      { key: 'surat_rujukan', label: 'Surat Rujukan', required: true },
      { key: 'keterangan_rawat_inap', label: 'Surat Keterangan Rawat Inap', required: true },
    ]
  }
  if (j.includes('alasan penting')) {
    return withRekomendasiKepsek(
      { key: 'sk_terakhir', label: 'SK Terakhir', required: true },
      { key: 'dokumen_pendukung', label: 'Dokumen Pendukung (rawat inap keluarga / kematian / KUA / istri melahirkan)', required: true },
    )
  }
  if (j.includes('tahunan')) {
    return withRekomendasiKepsek({ key: 'sk_terakhir', label: 'SK Terakhir', required: true })
  }
  return []
}

const statusFilterOptions = [
  { label: 'Semua', value: null },
  { label: 'Menunggu', value: 'pending' },
  { label: 'Disetujui', value: 'disetujui' },
  { label: 'Ditolak', value: 'ditolak' },
  { label: 'Dikembalikan', value: 'dikembalikan' },
]

const isManage = computed(() => auth.isAdministrator || auth.isAdmin)
const isAtasan = computed(() => auth.isAtasan)
const isPegawai = computed(() => auth.isPegawai)

function statusSeverity(status) {
  if (status === 'disetujui') return 'success'
  if (status === 'ditolak') return 'danger'
  if (status === 'dikembalikan') return 'contrast'
  return 'warn'
}

function formatDate(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'short', day: 'numeric' })
}

// Tempat tugas pegawai yang sedang login (dipakai saat pegawai/atasan
// mengajukan cuti untuk diri sendiri) -- dipakai untuk menentukan apakah
// "Surat Rekomendasi Kepala Sekolah" wajib diupload (lihat isSekolah di bawah).
const myTempatTgs = ref('')

async function loadOptions() {
  const jc = await http.get('/ref/jenis-cuti')
  jenisCutiOptions.value = jc.data.data || []
  if (isManage.value) {
    const peg = await http.get('/ref/pegawai')
    pegawaiOptions.value = peg.data.data || []
  } else {
    try {
      const me = await http.get('/pegawai/me')
      myTempatTgs.value = me.data.data?.tempat_tgs || ''
    } catch (e) {
      myTempatTgs.value = ''
    }
  }
}

async function fetchList() {
  loading.value = true
  try {
    const { data } = await http.get('/pengajuan-cuti', {
      params: { page: page.value, pageSize: pageSize.value, status: statusFilter.value || undefined },
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

watch(statusFilter, () => {
  page.value = 1
  fetchList()
})

// ---- create / edit (pegawai ajukan sendiri, admin bisa untuk siapa saja) ----
const formDialog = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const formErrors = ref('')
const form = reactive({
  id: null,
  id_pegawai: null,
  id_jenis_cuti: null,
  tgl_mulai: null,
  tgl_selesai: null,
  alasan_cuti: '',
  alamat_selama_cuti: '',
})

// Berkas kelengkapan yang dipilih user, keyed by jenis/key dokumen (mis.
// "sk_terakhir" -> File). Dipakai untuk berkas pengajuan baru (create) MAUPUN
// untuk mengupload ulang berkas yang ditandai "perlu diperbaiki" saat edit
// pengajuan yang statusnya dikembalikan.
const docFiles = reactive({})
const docFileInputs = {}

// Berkas yang ditandai atasan/admin sebagai "perlu diperbaiki" saat pengajuan
// ini dikembalikan (diisi saat openEdit, dari row.dokumen yang sudah
// dipreload). Kosong jika pengajuan tidak pernah dikembalikan / semua berkas
// sudah oke.
const flaggedEditDocs = ref([])
const editReturnNote = ref('')

const selectedJenisNama = computed(() => jenisCutiOptions.value.find((j) => j.id === form.id_jenis_cuti)?.jenis || '')
// Tempat tugas dari pegawai yang bersangkutan -- pegawai yang dipilih admin
// (form.id_pegawai) untuk isManage, atau profil pegawai/atasan sendiri.
const selectedTempatTgs = computed(() => {
  if (isManage.value) {
    return pegawaiOptions.value.find((p) => p.id === form.id_pegawai)?.tempat_tgs || ''
  }
  return myTempatTgs.value
})
const isSekolah = computed(() => selectedTempatTgs.value.toLowerCase().includes('sekolah'))
const docRequirements = computed(() => dokumenRequirementsForJenis(selectedJenisNama.value, isSekolah.value))
// admin/administrator boleh membuat pengajuan tanpa berkas; pegawai/atasan
// yang mengajukan untuk diri sendiri wajib melengkapi berkas.
const docsRequired = computed(() => !isManage.value)

function resetForm() {
  form.id = null
  form.id_pegawai = null
  form.id_jenis_cuti = null
  form.tgl_mulai = null
  form.tgl_selesai = null
  form.alasan_cuti = ''
  form.alamat_selama_cuti = ''
  Object.keys(docFiles).forEach((k) => delete docFiles[k])
}

function openCreate() {
  isEditing.value = false
  formErrors.value = ''
  resetForm()
  flaggedEditDocs.value = []
  editReturnNote.value = ''
  formDialog.value = true
}

function openEdit(row) {
  isEditing.value = true
  formErrors.value = ''
  form.id = row.id
  form.id_pegawai = row.id_pegawai
  form.id_jenis_cuti = row.id_jenis_cuti
  form.tgl_mulai = new Date(row.tgl_mulai)
  form.tgl_selesai = new Date(row.tgl_selesai)
  form.alasan_cuti = row.alasan_cuti
  form.alamat_selama_cuti = row.alamat_selama_cuti
  Object.keys(docFiles).forEach((k) => delete docFiles[k])
  flaggedEditDocs.value = row.status === 'dikembalikan' ? (row.dokumen || []).filter((d) => d.perlu_perbaikan) : []
  editReturnNote.value = row.status === 'dikembalikan' ? row.catatan_approval || '' : ''
  formDialog.value = true
}

function pickDocFile(key) {
  docFileInputs[key]?.click()
}
function onDocFileChosen(key, e) {
  docFiles[key] = e.target.files[0] || null
}
function missingDocs() {
  if (isEditing.value || !docsRequired.value) return []
  return docRequirements.value.filter((r) => r.required && !docFiles[r.key]).map((r) => r.label)
}
function missingFlaggedDocs() {
  if (!isEditing.value) return []
  return flaggedEditDocs.value.filter((d) => !docFiles[d.jenis]).map((d) => d.label || d.jenis)
}

async function saveForm() {
  if (!form.id_jenis_cuti || !form.tgl_mulai || !form.tgl_selesai || (isManage.value && !form.id_pegawai)) {
    formErrors.value = 'Lengkapi semua data wajib (pegawai, jenis cuti, tanggal mulai & selesai)'
    return
  }
  const missing = missingDocs()
  if (missing.length) {
    formErrors.value = 'Berkas wajib belum diupload: ' + missing.join(', ')
    return
  }
  const missingFixed = missingFlaggedDocs()
  if (missingFixed.length) {
    formErrors.value = 'Berkas yang perlu diperbaiki belum diupload ulang: ' + missingFixed.join(', ')
    return
  }
  formErrors.value = ''
  saving.value = true
  try {
    if (isEditing.value) {
      if (flaggedEditDocs.value.length) {
        // ada berkas yang ditandai perlu diperbaiki -> kirim multipart supaya
        // berkas penggantinya ikut terupload dalam permintaan yang sama.
        const formData = new FormData()
        if (form.id_pegawai) formData.append('id_pegawai', form.id_pegawai)
        formData.append('id_jenis_cuti', form.id_jenis_cuti)
        formData.append('tgl_mulai', toApiDate(form.tgl_mulai))
        formData.append('tgl_selesai', toApiDate(form.tgl_selesai))
        formData.append('alasan_cuti', form.alasan_cuti || '')
        formData.append('alamat_selama_cuti', form.alamat_selama_cuti || '')
        for (const d of flaggedEditDocs.value) {
          if (docFiles[d.jenis]) formData.append('dokumen_' + d.jenis, docFiles[d.jenis])
        }
        await http.put(`/pengajuan-cuti/${form.id}`, formData, { headers: { 'Content-Type': 'multipart/form-data' } })
      } else {
        const payload = {
          id_pegawai: form.id_pegawai,
          id_jenis_cuti: form.id_jenis_cuti,
          tgl_mulai: toApiDate(form.tgl_mulai),
          tgl_selesai: toApiDate(form.tgl_selesai),
          alasan_cuti: form.alasan_cuti,
          alamat_selama_cuti: form.alamat_selama_cuti,
        }
        await http.put(`/pengajuan-cuti/${form.id}`, payload)
      }
      toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan cuti berhasil diperbarui', life: 3000 })
    } else {
      const formData = new FormData()
      if (form.id_pegawai) formData.append('id_pegawai', form.id_pegawai)
      formData.append('id_jenis_cuti', form.id_jenis_cuti)
      formData.append('tgl_mulai', toApiDate(form.tgl_mulai))
      formData.append('tgl_selesai', toApiDate(form.tgl_selesai))
      formData.append('alasan_cuti', form.alasan_cuti || '')
      formData.append('alamat_selama_cuti', form.alamat_selama_cuti || '')
      for (const req of docRequirements.value) {
        if (docFiles[req.key]) formData.append('dokumen_' + req.key, docFiles[req.key])
      }
      await http.post('/pengajuan-cuti', formData, { headers: { 'Content-Type': 'multipart/form-data' } })
      toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan cuti berhasil diajukan', life: 3000 })
    }
    formDialog.value = false
    fetchList()
  } catch (e) {
    formErrors.value = e.response?.data?.message || 'Gagal menyimpan pengajuan cuti'
  } finally {
    saving.value = false
  }
}

function confirmDelete(row) {
  confirm.require({
    message: 'Hapus pengajuan cuti ini?',
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/pengajuan-cuti/${row.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Data berhasil dihapus', life: 3000 })
        fetchList()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

// ---- detail (semua role bisa lihat riwayat pengajuan miliknya sendiri) ----
const detailDialog = ref(false)
const detailRow = ref(null)
function openDetail(row) {
  detailRow.value = row
  detailDialog.value = true
}
function downloadDoc(pengajuanId, doc) {
  downloadFile(`/pengajuan-cuti/${pengajuanId}/dokumen/${doc.jenis}`, doc.nama_file)
}

// ---- lihat dokumen langsung (tanpa download) ----
const previewDialog = ref(false)
const previewUrl = ref('')
const previewType = ref('pdf') // 'pdf' | 'image' | 'other'
const previewTitle = ref('')
const previewKind = ref('') // 'rekomendasi' | 'cuti' | '' -- dipakai untuk tahu kapan menampilkan input Nomor Surat
const previewPengajuanId = ref(null)

// ---- nomor surat pada Surat Rekomendasi Izin Cuti -- opsional, hanya
// administrator/admin yang boleh mengisi (lihat isManage). Diedit langsung
// dari dialog pratinjau surat rekomendasi, lalu suratnya di-generate ulang
// supaya nomor barunya langsung terlihat tanpa perlu tutup-buka dialog.
const nomorSuratInput = ref('')
const savingNomorSurat = ref(false)

async function saveNomorSurat() {
  if (!previewPengajuanId.value) return
  savingNomorSurat.value = true
  try {
    await http.put(`/pengajuan-cuti/${previewPengajuanId.value}/nomor-surat`, { nomor_surat: nomorSuratInput.value })
    if (previewUrl.value) window.URL.revokeObjectURL(previewUrl.value)
    const res = await http.get(`/pengajuan-cuti/${previewPengajuanId.value}/form/rekomendasi`, {
      params: { inline: 1 },
      responseType: 'blob',
    })
    previewUrl.value = window.URL.createObjectURL(res.data)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Nomor surat berhasil disimpan', life: 3000 })
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    savingNomorSurat.value = false
  }
}

async function previewDoc(pengajuanId, doc) {
  try {
    const res = await http.get(`/pengajuan-cuti/${pengajuanId}/dokumen/${doc.jenis}`, {
      params: { inline: 1 },
      responseType: 'blob',
    })
    const ext = (doc.nama_file || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewTitle.value = doc.label || doc.jenis
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

// ---- lihat & cetak formulir (Surat Rekomendasi / Formulir Cuti) langsung,
// tanpa memaksa download -- pakai dialog+iframe yang sama seperti previewDoc.
// Karena backend mengirim PDF dengan Content-Disposition: inline, browser
// menampilkan viewer PDF bawaan yang sudah punya tombol print/download sendiri.
async function previewForm(pengajuanId, kind, title, nomorSurat) {
  try {
    const url = kind === 'rekomendasi' ? `/pengajuan-cuti/${pengajuanId}/form/rekomendasi` : `/pengajuan-cuti/${pengajuanId}/form/cuti`
    const res = await http.get(url, { params: { inline: 1 }, responseType: 'blob' })
    previewType.value = 'pdf'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewTitle.value = title
    previewKind.value = kind
    previewPengajuanId.value = pengajuanId
    nomorSuratInput.value = nomorSurat || ''
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat formulir', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

function closePreview() {
  if (previewUrl.value) window.URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
  previewKind.value = ''
  previewPengajuanId.value = null
}

// ---- upload/lihat/hapus scan formulir yang sudah ditandatangani basah oleh
// Kepala Dinas (admin/administrator saja yang bisa upload/hapus; semua role,
// termasuk pegawai pemilik pengajuan, bisa melihat & mengunduhnya). Cukup
// SATU file per pengajuan (Surat Rekomendasi + Formulir Cuti yang sudah
// ditandatangani biasanya sudah discan jadi satu berkas), disimpan sebagai
// PengajuanDokumen biasa dengan jenis "ttd_formulir".
const TTD_JENIS = 'ttd_formulir'
const TTD_LABEL = 'Berkas Pengajuan Cuti (Sudah TTD Kepala Dinas)'
const ttdFileInput = ref(null)
const ttdUploading = ref(false)

function ttdDoc() {
  return (detailRow.value?.dokumen || []).find((d) => d.jenis === TTD_JENIS)
}
// sama seperti ttdDoc, tapi menerima row apapun (dipakai di kolom Aksi tabel,
// bukan hanya detailRow) -- listPengajuan sudah preload relasi Dokumen
// (tanpa isi file-nya) jadi ini tidak perlu request tambahan.
function ttdDocOf(row) {
  return (row?.dokumen || []).find((d) => d.jenis === TTD_JENIS)
}
// berkas kelengkapan pengajuan (yang diupload pegawai saat mengajukan),
// dipisahkan dari berkas formulir bertanda tangan (ttd_formulir) yang punya
// section sendiri di detail dialog.
const kelengkapanDocs = computed(() => (detailRow.value?.dokumen || []).filter((d) => d.jenis !== TTD_JENIS))
async function refreshDetailRow() {
  if (!detailRow.value) return
  try {
    const { data } = await http.get(`/pengajuan-cuti/${detailRow.value.id}`)
    detailRow.value = data.data
  } catch (e) {
    // biarkan; dialog tetap menampilkan data sebelumnya
  }
}
function pickTtdFile() {
  ttdFileInput.value?.click()
}
async function onTtdFileChosen(e) {
  const file = e.target.files[0]
  e.target.value = ''
  if (!file || !detailRow.value) return
  ttdUploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    await http.post(`/pengajuan-cuti/${detailRow.value.id}/form/ttd`, formData, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: `${TTD_LABEL} berhasil diupload`, life: 3000 })
    await refreshDetailRow()
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal upload', detail: err.response?.data?.message || err.message, life: 4000 })
  } finally {
    ttdUploading.value = false
  }
}
function confirmDeleteTtd() {
  confirm.require({
    message: `Hapus ${TTD_LABEL} yang sudah diupload?`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.delete(`/pengajuan-cuti/${detailRow.value.id}/form/ttd`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Berkas dihapus', life: 3000 })
        await refreshDetailRow()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

// ---- approval (atasan + admin/administrator) ----
const approvalDialog = ref(false)
const approvalRow = ref(null)
const approvalNote = ref('')
const approvalLoading = ref(false)
// id dokumen yang dicentang sebagai "tidak sesuai/bermasalah" -- hanya dipakai
// saat aksi "Kembalikan", supaya pegawai tahu berkas mana yang wajib diupload
// ulang (lihat flaggedEditDocs di form edit).
const flaggedDocIds = ref([])

function openApproval(row) {
  approvalRow.value = row
  approvalNote.value = ''
  flaggedDocIds.value = []
  approvalDialog.value = true
}

async function processApproval(action) {
  if (action === 'kembalikan' && !approvalNote.value.trim()) {
    toast.add({
      severity: 'warn',
      summary: 'Alasan wajib diisi',
      detail: 'Isi Catatan dengan alasan pengembalian (misal: ada berkas yang tidak sesuai)',
      life: 4000,
    })
    return
  }
  approvalLoading.value = true
  try {
    const payload = { catatan_approval: approvalNote.value }
    if (action === 'kembalikan') payload.dokumen_ids = flaggedDocIds.value
    await http.put(`/pengajuan-cuti/${approvalRow.value.id}/${action}`, payload)
    const messages = {
      approve: 'Pengajuan cuti disetujui',
      reject: 'Pengajuan cuti ditolak',
      kembalikan: 'Pengajuan cuti dikembalikan ke pegawai untuk diperbaiki',
    }
    toast.add({
      severity: action === 'approve' ? 'success' : 'warn',
      summary: 'Berhasil',
      detail: messages[action] || 'Berhasil diproses',
      life: 3000,
    })
    approvalDialog.value = false
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    approvalLoading.value = false
  }
}

// ---- kembalikan ke status menunggu (atasan + admin/administrator) ----
function confirmReturn(row) {
  confirm.require({
    message: `Kembalikan pengajuan cuti ini ke status "menunggu"? ${row.status === 'disetujui' ? 'Kuota cuti tahunan yang sudah terpotong akan dikembalikan.' : ''}`,
    header: 'Konfirmasi Kembalikan Pengajuan',
    icon: 'pi pi-undo',
    acceptLabel: 'Ya, Kembalikan',
    rejectLabel: 'Batal',
    accept: async () => {
      try {
        await http.put(`/pengajuan-cuti/${row.id}/return`, {})
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan cuti dikembalikan ke status menunggu', life: 3000 })
        fetchList()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

// ---- excel import/export (admin/administrator only) ----
async function downloadFile(url, filename) {
  try {
    const res = await http.get(url, { responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.download = filename
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh file', detail: e.message, life: 4000 })
  }
}

const importDialog = ref(false)
const importFile = ref(null)
const importing = ref(false)
const importResult = ref(null)
const fileInputRef = ref(null)
const importMode = ref('append')
const importModeOptions = [
  { label: 'Tambahkan ke data yang ada', value: 'append' },
  { label: 'Hapus semua data lama, lalu import', value: 'replace' },
]

function openImport() {
  importFile.value = null
  importResult.value = null
  importMode.value = 'append'
  importDialog.value = true
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
    confirm.require({
      message: 'Semua data pengajuan cuti yang sudah ada akan DIHAPUS sebelum data dari file excel dimasukkan. Aksi ini tidak bisa dibatalkan. Lanjutkan?',
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
  try {
    const formData = new FormData()
    formData.append('file', importFile.value)
    formData.append('mode', importMode.value)
    const { data } = await http.post('/pengajuan-cuti/import', formData, { headers: { 'Content-Type': 'multipart/form-data' } })
    importResult.value = data.data
    toast.add({ severity: data.success ? 'success' : 'warn', summary: 'Import selesai', detail: data.message, life: 4000 })
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal import', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    importing.value = false
  }
}

// ---- overlay menu: gabungkan Template/Import/Export (khusus isManage)
// menjadi satu tombol trigger + dropdown, mengikuti pola "overlay menu".
// Untuk role Pegawai biasa yang hanya punya aksi "Ajukan Cuti" (satu
// aksi saja, tidak ada Template/Import/Export), tombol tetap tunggal
// seperti semula -- tidak perlu dibungkus dropdown.
const actionsMenuRef = ref(null)

function toggleActionsMenu(event) {
  actionsMenuRef.value?.toggle(event)
}

const actionsMenuItems = computed(() => [
  { label: 'Ajukan Cuti', icon: 'pi pi-plus', command: () => openCreate() },
  { label: 'Template', icon: 'pi pi-download', command: () => downloadFile('/pengajuan-cuti/template', 'template_pengajuan_cuti.xlsx') },
  { label: 'Import', icon: 'pi pi-upload', command: () => openImport() },
  { label: 'Export', icon: 'pi pi-file-export', command: () => downloadFile('/pengajuan-cuti/export', 'data_pengajuan_cuti.xlsx') },
])

onMounted(() => {
  loadOptions()
  fetchList()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Pengajuan Cuti</div>
    <p class="page-subtitle">
      <span v-if="isPegawai">Ajukan cuti dan pantau status persetujuannya di sini.</span>
      <span v-else-if="isAtasan">Tinjau dan proses pengajuan cuti bawahan anda.</span>
      <span v-else>Pantau dan kelola seluruh pengajuan cuti pegawai.</span>
    </p>

    <div class="card">
      <SelectButton
        v-model="statusFilter"
        :options="statusFilterOptions"
        optionLabel="label"
        optionValue="value"
        class="tab-filter"
        style="width: 100%; flex-wrap: wrap"
      />

      <div class="toolbar-actions" style="justify-content: space-between; margin-bottom: 1rem; align-items: flex-end; flex-wrap: wrap; gap: .75rem">
        <div class="entries-picker">
          <span class="entries-picker-label">Tampilkan</span>
          <Select v-model="pageSize" :options="entriesOptions" @change="onEntriesChange" />
        </div>
        <div class="toolbar-actions" style="align-items: center; flex-wrap: wrap">
          <template v-if="isManage">
            <Button icon="pi pi-chevron-down" iconPos="right" label="Menu" class="overlay-menu-trigger" @click="toggleActionsMenu" aria-haspopup="true" />
            <Menu ref="actionsMenuRef" :model="actionsMenuItems" popup class="overlay-actions-menu" />
          </template>
          <Button v-else-if="isPegawai" icon="pi pi-plus" label="Ajukan Cuti" @click="openCreate" />
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
          style="min-width: 760px"
        >
          <template #empty><div style="padding: 1.5rem; text-align: center; color: var(--p-text-muted-color)">Tidak ada data</div></template>
          <Column v-if="!isPegawai" field="pegawai.nama" header="Pegawai" />
          <Column field="jenis_cuti.jenis" header="Jenis Cuti" />
          <Column header="Tanggal">
            <template #body="{ data }">{{ formatDate(data.tgl_mulai) }} &ndash; {{ formatDate(data.tgl_selesai) }}</template>
          </Column>
          <Column field="jumlah_hari" header="Jml Hari" style="width: 100px" />
          <Column header="Status" style="width: 130px">
            <template #body="{ data }"><Tag :value="data.status" :severity="statusSeverity(data.status)" /></template>
          </Column>
          <Column header="Aksi" style="width: 300px">
            <template #body="{ data }">
              <div style="display: flex; gap: 0.35rem; flex-wrap: wrap">
                <Button icon="pi pi-eye" size="small" severity="secondary" rounded text @click="openDetail(data)" />
                <template v-if="data.status === 'disetujui'">
                  <!-- admin/administrator/atasan: cetak formulir mentah (belum ttd) untuk diajukan ttd ke Kepala Dinas -->
                  <template v-if="!isPegawai">
                    <Button
                      icon="pi pi-file-pdf"
                      size="small"
                      severity="info"
                      rounded
                      text
                      title="Lihat & Cetak Surat Rekomendasi"
                      @click="previewForm(data.id, 'rekomendasi', 'Surat Rekomendasi Izin Cuti', data.nomor_surat)"
                    />
                    <Button
                      icon="pi pi-file-pdf"
                      size="small"
                      severity="help"
                      rounded
                      text
                      title="Lihat & Cetak Formulir Cuti"
                      @click="previewForm(data.id, 'cuti', 'Formulir Cuti')"
                    />
                    <Button
                      v-if="isManage"
                      icon="pi pi-upload"
                      size="small"
                      severity="contrast"
                      rounded
                      text
                      title="Upload Formulir yang Sudah di-TTD Kepala Dinas"
                      @click="openDetail(data)"
                    />
                  </template>
                  <!-- pegawai: hanya lihat berkas yang SUDAH di-ttd Kepala Dinas & sudah diupload admin.
                       Sediakan tombol Lihat (preview inline) DAN Unduh (download langsung) --
                       preview inline lewat iframe kadang tidak didukung di browser HP (mis. Chrome
                       Android hanya menampilkan placeholder "Buka" tanpa render PDF-nya), jadi tombol
                       download jadi jalan pintas yang selalu berhasil di semua perangkat. -->
                  <template v-else>
                    <template v-if="ttdDocOf(data)">
                      <Button
                        icon="pi pi-eye"
                        size="small"
                        severity="success"
                        rounded
                        text
                        title="Lihat Berkas Pengajuan Cuti (sudah TTD Kepala Dinas)"
                        @click="previewDoc(data.id, ttdDocOf(data))"
                      />
                      <Button
                        icon="pi pi-download"
                        size="small"
                        severity="success"
                        rounded
                        text
                        title="Unduh Berkas Pengajuan Cuti (sudah TTD Kepala Dinas)"
                        @click="downloadDoc(data.id, ttdDocOf(data))"
                      />
                    </template>
                    <Tag
                      v-else
                      value="Proses TTD Kepala Dinas"
                      severity="warn"
                      title="Pengajuan sudah disetujui, berkas sedang diproses tanda tangan Kepala Dinas"
                    />
                  </template>
                </template>
                <Button
                  v-if="(isAtasan || isManage) && data.status === 'pending'"
                  icon="pi pi-check-square"
                  size="small"
                  label="Proses"
                  @click="openApproval(data)"
                />
                <Button
                  v-if="(isAtasan || isManage) && data.status !== 'pending'"
                  icon="pi pi-undo"
                  size="small"
                  severity="warn"
                  outlined
                  label="Kembalikan"
                  @click="confirmReturn(data)"
                />
                <template v-if="(isPegawai && (data.status === 'pending' || data.status === 'dikembalikan')) || isManage">
                  <Button icon="pi pi-pencil" size="small" severity="secondary" rounded text @click="openEdit(data)" />
                  <Button icon="pi pi-trash" size="small" severity="danger" rounded text @click="confirmDelete(data)" />
                </template>
              </div>
            </template>
          </Column>
        </DataTable>
      </div>
    </div>

    <!-- create/edit dialog -->
    <Dialog v-model:visible="formDialog" modal :header="isEditing ? 'Edit Pengajuan Cuti' : 'Ajukan Cuti'" :style="{ width: '38rem', maxWidth: '95vw' }">
      <Message v-if="formErrors" severity="error" :closable="false" style="margin-bottom: 1rem">{{ formErrors }}</Message>
      <Message v-if="flaggedEditDocs.length" severity="warn" :closable="false" style="margin-bottom: 1rem">
        Pengajuan ini dikembalikan{{ editReturnNote ? ': ' + editReturnNote : '' }}. Silakan upload ulang berkas yang ditandai di bawah ini.
      </Message>
      <!-- grid responsive (PrimeFlex): 1 kolom di HP, otomatis jadi 2 kolom di tablet/desktop (>=768px) -->
      <div class="grid formgrid">
        <div v-if="isManage" class="col-12">
          <label class="field-label">Pegawai *</label>
          <Select v-model="form.id_pegawai" :options="pegawaiOptions" :optionLabel="(o) => `${o.nama} (${o.nip})`" optionValue="id" filter style="width: 100%" placeholder="Pilih pegawai" />
        </div>
        <div class="col-12">
          <label class="field-label">Jenis Cuti *</label>
          <Select v-model="form.id_jenis_cuti" :options="jenisCutiOptions" optionLabel="jenis" optionValue="id" style="width: 100%" placeholder="Pilih jenis cuti" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Tanggal Mulai *</label>
          <DatePicker v-model="form.tgl_mulai" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Tanggal Selesai *</label>
          <DatePicker v-model="form.tgl_selesai" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
        </div>
        <div class="col-12">
          <Message severity="info" :closable="false" style="font-size: 0.8rem">
            Pola hari kerja dihitung otomatis dari tempat tugas pegawai: tempat tugas "Sekolah" &rarr; 6 hari kerja (Senin&ndash;Sabtu), tempat tugas lainnya (Dinas/Kantor) &rarr; 5 hari kerja (Senin&ndash;Jumat).
          </Message>
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Alasan Cuti</label>
          <Textarea v-model="form.alasan_cuti" rows="2" style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Alamat Selama Cuti</label>
          <Textarea v-model="form.alamat_selama_cuti" rows="2" style="width: 100%" />
        </div>
      </div>
      <div style="display: flex; flex-direction: column; gap: 1rem; margin-top: 1rem">
        <div v-if="!isEditing && docRequirements.length">
          <label class="field-label">Berkas Kelengkapan {{ docsRequired ? '(wajib)' : '(opsional, karena diajukan oleh admin)' }}</label>
          <div v-for="req in docRequirements" :key="req.key" class="doc-upload-row">
            <div class="doc-upload-label">
              {{ req.label }}
              <span v-if="req.required && docsRequired" style="color: #ef4444">*</span>
              <span v-else-if="!req.required" style="color: var(--p-text-muted-color); font-size: 0.75rem">(opsional)</span>
            </div>
            <div class="doc-upload-actions">
              <input :ref="(el) => (docFileInputs[req.key] = el)" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="(e) => onDocFileChosen(req.key, e)" />
              <Button size="small" outlined :label="docFiles[req.key] ? 'Ganti File' : 'Pilih File'" icon="pi pi-upload" @click="pickDocFile(req.key)" />
              <span v-if="docFiles[req.key]" class="doc-upload-filename">{{ docFiles[req.key].name }}</span>
            </div>
          </div>
        </div>
        <div v-if="isEditing && flaggedEditDocs.length">
          <label class="field-label">Berkas yang Perlu Diperbaiki (wajib diupload ulang)</label>
          <div v-for="doc in flaggedEditDocs" :key="doc.id" class="doc-upload-row">
            <div class="doc-upload-label">
              {{ doc.label || doc.jenis }} <small style="color: var(--p-text-muted-color)">(berkas lama: {{ doc.nama_file }})</small>
              <span style="color: #ef4444">*</span>
            </div>
            <div class="doc-upload-actions">
              <input :ref="(el) => (docFileInputs[doc.jenis] = el)" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="(e) => onDocFileChosen(doc.jenis, e)" />
              <Button size="small" outlined :label="docFiles[doc.jenis] ? 'Ganti File' : 'Pilih File Baru'" icon="pi pi-upload" @click="pickDocFile(doc.jenis)" />
              <span v-if="docFiles[doc.jenis]" class="doc-upload-filename">{{ docFiles[doc.jenis].name }}</span>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="formDialog = false" />
        <Button label="Simpan" :loading="saving" @click="saveForm" />
      </template>
    </Dialog>

    <!-- detail dialog (semua role) -->
    <Dialog v-model:visible="detailDialog" modal header="Detail Pengajuan Cuti" :style="{ width: '32rem', maxWidth: '95vw' }">
      <div v-if="detailRow" style="display: flex; flex-direction: column; gap: 0.5rem; font-size: 0.9rem">
        <div v-if="!isPegawai"><strong>Pegawai:</strong> {{ detailRow.pegawai?.nama }}</div>
        <div><strong>Jenis Cuti:</strong> {{ detailRow.jenis_cuti?.jenis }}</div>
        <div><strong>Tanggal:</strong> {{ formatDate(detailRow.tgl_mulai) }} &ndash; {{ formatDate(detailRow.tgl_selesai) }} ({{ detailRow.jumlah_hari }} hari)</div>
        <div><strong>Status:</strong> <Tag :value="detailRow.status" :severity="statusSeverity(detailRow.status)" /></div>
        <div><strong>Alasan:</strong> {{ detailRow.alasan_cuti || '-' }}</div>
        <div><strong>Alamat Selama Cuti:</strong> {{ detailRow.alamat_selama_cuti || '-' }}</div>
        <div v-if="detailRow.catatan_approval"><strong>Catatan Atasan:</strong> {{ detailRow.catatan_approval }}</div>
        <div style="margin-top: 0.5rem; font-weight: 600">Berkas Kelengkapan</div>
        <div v-if="kelengkapanDocs.length">
          <div v-for="doc in kelengkapanDocs" :key="doc.id" class="doc-row">
            <span>
              {{ doc.label || doc.jenis }} <small style="color: var(--p-text-muted-color)">({{ doc.nama_file }})</small>
              <Tag v-if="doc.perlu_perbaikan" value="Perlu Diperbaiki" severity="warn" style="margin-left: 0.35rem" />
            </span>
            <div style="display: flex; gap: 0.15rem">
              <Button icon="pi pi-eye" size="small" text label="Lihat" @click="previewDoc(detailRow.id, doc)" />
              <Button icon="pi pi-download" size="small" text @click="downloadDoc(detailRow.id, doc)" />
            </div>
          </div>
        </div>
        <div v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Tidak ada berkas yang diupload.</div>
        <!-- pegawai tidak perlu melihat/mencetak draf formulir yang belum di-ttd --
             cukup admin/administrator/atasan yang mencetaknya untuk diajukan ttd. -->
        <div v-if="detailRow.status === 'disetujui' && !isPegawai" style="margin-top: 0.75rem; border-top: 1px solid var(--p-content-border-color); padding-top: 0.75rem">
          <div style="font-weight: 600; margin-bottom: 0.5rem">Formulir Cetak (dibuat otomatis oleh sistem, belum TTD)</div>
          <div style="display: flex; gap: 0.5rem; flex-wrap: wrap">
            <Button
              icon="pi pi-eye"
              size="small"
              outlined
              label="Surat Rekomendasi"
              @click="previewForm(detailRow.id, 'rekomendasi', 'Surat Rekomendasi Izin Cuti', detailRow.nomor_surat)"
            />
            <Button
              icon="pi pi-eye"
              size="small"
              outlined
              label="Formulir Cuti"
              @click="previewForm(detailRow.id, 'cuti', 'Formulir Cuti')"
            />
          </div>
          <small style="display: block; margin-top: 0.4rem; color: var(--p-text-muted-color)">Klik untuk melihat &amp; langsung mencetak (tanpa perlu download dulu).</small>
        </div>
        <div v-if="detailRow.status === 'disetujui'" style="margin-top: 0.75rem; border-top: 1px solid var(--p-content-border-color); padding-top: 0.75rem">
          <div style="font-weight: 600; margin-bottom: 0.5rem">Formulir Bertanda Tangan Kepala Dinas</div>
          <div class="doc-row">
            <span>
              Berkas Pengajuan Cuti
              <Tag v-if="ttdDoc()" value="Sudah diupload" severity="success" style="margin-left: 0.35rem" />
              <Tag v-else value="Menunggu TTD Kepala Dinas" severity="warn" style="margin-left: 0.35rem" />
            </span>
            <div style="display: flex; gap: 0.15rem; align-items: center">
              <template v-if="ttdDoc()">
                <Button icon="pi pi-eye" size="small" text label="Lihat" @click="previewDoc(detailRow.id, ttdDoc())" />
                <Button icon="pi pi-download" size="small" text @click="downloadDoc(detailRow.id, ttdDoc())" />
              </template>
              <template v-if="isManage">
                <input ref="ttdFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onTtdFileChosen" />
                <Button size="small" outlined :loading="ttdUploading" :label="ttdDoc() ? 'Ganti File' : 'Upload'" icon="pi pi-upload" @click="pickTtdFile" />
                <Button v-if="ttdDoc()" icon="pi pi-trash" size="small" severity="danger" text @click="confirmDeleteTtd" />
              </template>
            </div>
          </div>
          <small style="display: block; margin-top: 0.3rem; color: var(--p-text-muted-color)">
            Satu berkas gabungan (Surat Rekomendasi + Formulir Cuti yang sudah ditandatangani Kepala Dinas) sudah cukup -- tidak perlu diupload terpisah.
          </small>
          <small v-if="!isManage && !ttdDoc()" style="display: block; margin-top: 0.2rem; color: var(--p-text-muted-color)">
            Pengajuan cuti ini sudah <strong>disetujui</strong>, berkas resminya masih dalam proses tanda tangan Kepala Dinas. Berkas yang sudah di-ttd akan tampil di sini begitu diupload oleh Administrator.
          </small>
        </div>
      </div>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="detailDialog = false" />
      </template>
    </Dialog>

    <!-- approval dialog (atasan) -->
    <Dialog v-model:visible="approvalDialog" modal header="Proses Pengajuan Cuti" :style="{ width: '30rem', maxWidth: '95vw' }">
      <div v-if="approvalRow" style="display: flex; flex-direction: column; gap: 0.5rem; margin-bottom: 1rem; font-size: 0.9rem">
        <div><strong>Pegawai:</strong> {{ approvalRow.pegawai?.nama }}</div>
        <div><strong>Jenis Cuti:</strong> {{ approvalRow.jenis_cuti?.jenis }}</div>
        <div><strong>Tanggal:</strong> {{ formatDate(approvalRow.tgl_mulai) }} &ndash; {{ formatDate(approvalRow.tgl_selesai) }} ({{ approvalRow.jumlah_hari }} hari)</div>
        <div><strong>Alasan:</strong> {{ approvalRow.alasan_cuti || '-' }}</div>
        <div><strong>Alamat Selama Cuti:</strong> {{ approvalRow.alamat_selama_cuti || '-' }}</div>
        <div style="margin-top: 0.25rem; font-weight: 600">Berkas Kelengkapan</div>
        <Message v-if="approvalRow.dokumen?.length" severity="info" :closable="false" style="font-size: 0.78rem; padding: 0.5rem 0.75rem">
          Centang berkas yang tidak sesuai sebelum klik "Kembalikan", agar pegawai tahu berkas mana yang wajib diupload ulang.
        </Message>
        <div v-if="approvalRow.dokumen?.length">
          <div v-for="doc in approvalRow.dokumen" :key="doc.id" class="doc-row">
            <span style="display: flex; align-items: center; gap: 0.5rem">
              <Checkbox v-model="flaggedDocIds" :inputId="'doc-flag-' + doc.id" :value="doc.id" />
              <label :for="'doc-flag-' + doc.id" style="cursor: pointer">{{ doc.label || doc.jenis }} <small style="color: var(--p-text-muted-color)">({{ doc.nama_file }})</small></label>
            </span>
            <div style="display: flex; gap: 0.15rem">
              <Button icon="pi pi-eye" size="small" text label="Lihat" @click="previewDoc(approvalRow.id, doc)" />
              <Button icon="pi pi-download" size="small" text @click="downloadDoc(approvalRow.id, doc)" />
            </div>
          </div>
        </div>
        <div v-else style="color: var(--p-text-muted-color); font-size: 0.85rem">Tidak ada berkas yang diupload.</div>
      </div>
      <label class="field-label">Catatan (wajib diisi untuk Tolak/Kembalikan)</label>
      <Textarea v-model="approvalNote" rows="2" style="width: 100%" placeholder="Catatan untuk pegawai... (mis. ada berkas yang tidak sesuai)" />
      <template #footer>
        <Button label="Kembalikan" severity="warn" outlined :loading="approvalLoading" @click="processApproval('kembalikan')" />
        <Button label="Tolak" severity="danger" outlined :loading="approvalLoading" @click="processApproval('reject')" />
        <Button label="Setujui" severity="success" :loading="approvalLoading" @click="processApproval('approve')" />
      </template>
    </Dialog>

    <!-- lihat dokumen (tanpa download) -->
    <Dialog v-model:visible="previewDialog" modal :header="previewTitle" :style="{ width: '95vw', maxWidth: '62rem' }" @hide="closePreview">
      <!-- Nomor Surat pada Surat Rekomendasi Izin Cuti -- opsional (boleh
           dikosongkan), hanya administrator/admin yang bisa mengisi/mengubah.
           Setelah disimpan, pratinjau PDF di bawah otomatis dimuat ulang
           supaya nomor barunya langsung terlihat. -->
      <div v-if="previewKind === 'rekomendasi' && isManage" style="display: flex; gap: 0.5rem; align-items: flex-end; flex-wrap: wrap; margin-bottom: 0.75rem">
        <div style="flex: 1; min-width: 220px">
          <label class="field-label">Nomor Surat (opsional)</label>
          <InputText v-model="nomorSuratInput" placeholder="mis. 123" style="width: 100%" />
        </div>
        <Button label="Simpan Nomor Surat" icon="pi pi-save" size="small" :loading="savingNomorSurat" @click="saveNomorSurat" />
      </div>
      <div v-if="previewType === 'pdf'" style="width: 100%; height: 75vh">
        <iframe :src="previewUrl" style="width: 100%; height: 100%; border: none" title="Pratinjau dokumen"></iframe>
      </div>
      <div v-else-if="previewType === 'image'" style="text-align: center">
        <img :src="previewUrl" style="max-width: 100%; max-height: 75vh" alt="Pratinjau dokumen" />
      </div>
      <div v-else style="padding: 2rem; text-align: center; color: var(--p-text-muted-color)">
        Format berkas ini tidak bisa dipratinjau langsung, silakan unduh untuk melihatnya.
      </div>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="previewDialog = false" />
      </template>
    </Dialog>

    <!-- import dialog (admin/administrator) -->
    <Dialog v-model:visible="importDialog" modal header="Import Data dari Excel" :style="{ width: '34rem', maxWidth: '95vw' }">
      <p style="margin-top: 0; color: var(--p-text-muted-color); font-size: 0.9rem">
        Unduh template terlebih dahulu, isi data sesuai format (tanggal: DD-MM-YYYY), lalu upload file excel (.xlsx) di bawah ini.
      </p>
      <Button label="Download Template" icon="pi pi-download" severity="secondary" outlined @click="downloadFile('/pengajuan-cuti/template', 'template_pengajuan_cuti.xlsx')" style="margin-bottom: 1rem" />
      <input ref="fileInputRef" type="file" accept=".xlsx" style="display: none" @change="onFileChosen" />
      <div style="display: flex; gap: 0.5rem; align-items: center; margin-bottom: 1rem">
        <Button label="Pilih File Excel" icon="pi pi-file-excel" outlined @click="pickFile" />
        <span style="font-size: 0.85rem">{{ importFile?.name || 'Belum ada file dipilih' }}</span>
      </div>
      <div style="margin-bottom: 1rem">
        <label style="display: block; font-size: 0.85rem; font-weight: 600; margin-bottom: 0.5rem">Jika ada data sebelumnya</label>
        <SelectButton v-model="importMode" :options="importModeOptions" optionLabel="label" optionValue="value" :allowEmpty="false" style="display: flex; flex-wrap: wrap" />
        <small v-if="importMode === 'replace'" style="color: #ef4444; display: block; margin-top: 0.4rem">
          Semua pengajuan cuti yang sudah ada akan dihapus permanen sebelum data baru dari file dimasukkan.
        </small>
      </div>
      <div v-if="importResult" style="margin-top: 1rem">
        <Message :severity="importResult.failed_rows?.length ? 'warn' : 'success'" :closable="false">
          {{ importResult.success_count }} baris berhasil, {{ importResult.failed_rows?.length || 0 }} baris gagal.
        </Message>
      </div>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="importDialog = false" />
        <Button label="Upload & Import" icon="pi pi-upload" :loading="importing" :disabled="!importFile" @click="submitImport" />
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
.doc-upload-row {
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  padding: 0.5rem 0.65rem;
  margin-bottom: 0.5rem;
}
.doc-upload-label {
  font-size: 0.85rem;
  margin-bottom: 0.4rem;
}
.doc-upload-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.doc-upload-filename {
  font-size: 0.8rem;
  color: var(--p-text-muted-color);
}
.doc-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.35rem 0;
  border-bottom: 1px solid var(--p-content-border-color);
  font-size: 0.85rem;
}
</style>
