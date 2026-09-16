<script setup>
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { useAuthStore } from '../stores/auth'
import http from '../api/http'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'

const toast = useToast()
const confirm = useConfirm()
const auth = useAuthStore()
const canManage = computed(() => auth.canManageMaster)

const templates = ref([])
const loading = ref(true)

async function loadTemplates() {
  loading.value = true
  try {
    const { data } = await http.get('/template-surat')
    templates.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

onMounted(loadTemplates)

function formatTanggal(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'short', day: 'numeric' })
}

function ekstensi(namaFile) {
  if (!namaFile) return ''
  const idx = namaFile.lastIndexOf('.')
  return idx === -1 ? '' : namaFile.slice(idx + 1).toLowerCase()
}

function ikonFile(namaFile) {
  const ext = ekstensi(namaFile)
  if (ext === 'pdf') return 'pi pi-file-pdf'
  if (ext === 'doc' || ext === 'docx') return 'pi pi-file-word'
  return 'pi pi-file'
}

// ============================================================
// tambah / ubah (khusus administrator/admin)
// ============================================================
const dialogVisible = ref(false)
const dialogMode = ref('tambah') // 'tambah' | 'ubah'
const form = ref({ id: null, judul: '' })
const fileInput = ref(null)
const chosenFile = ref(null)
const submitting = ref(false)

function bukaTambah() {
  dialogMode.value = 'tambah'
  form.value = { id: null, judul: '' }
  chosenFile.value = null
  dialogVisible.value = true
}

function bukaUbah(item) {
  dialogMode.value = 'ubah'
  form.value = { id: item.id, judul: item.judul }
  chosenFile.value = null
  dialogVisible.value = true
}

function tutupDialog() {
  dialogVisible.value = false
}

function pilihFile() {
  fileInput.value?.click()
}

function onFileChosen(e) {
  const f = e.target.files?.[0]
  if (!f) return
  const ext = f.name.slice(f.name.lastIndexOf('.')).toLowerCase()
  if (!['.pdf', '.doc', '.docx'].includes(ext)) {
    toast.add({ severity: 'warn', summary: 'Format tidak didukung', detail: 'Berkas harus PDF, DOC, atau DOCX', life: 4000 })
    e.target.value = ''
    return
  }
  chosenFile.value = f
}

async function simpanTemplate() {
  const judul = form.value.judul.trim()
  if (!judul) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Judul surat wajib diisi', life: 4000 })
    return
  }
  if (dialogMode.value === 'tambah' && !chosenFile.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih berkas PDF/Word', life: 4000 })
    return
  }
  submitting.value = true
  try {
    const fd = new FormData()
    fd.append('judul', judul)
    if (chosenFile.value) fd.append('file', chosenFile.value)
    let data
    if (dialogMode.value === 'tambah') {
      ;({ data } = await http.post('/template-surat', fd, { headers: { 'Content-Type': 'multipart/form-data' } }))
    } else {
      ;({ data } = await http.put(`/template-surat/${form.value.id}`, fd, { headers: { 'Content-Type': 'multipart/form-data' } }))
    }
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 5000 })
    tutupDialog()
    await loadTemplates()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 7000 })
  } finally {
    submitting.value = false
  }
}

function konfirmasiHapus(item) {
  confirm.require({
    message: `Hapus template surat "${item.judul}"? Tindakan ini tidak bisa dibatalkan.`,
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
    const { data } = await http.delete(`/template-surat/${item.id}`)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 4000 })
    await loadTemplates()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

async function unduhTemplate(item) {
  try {
    const res = await http.get(`/template-surat/${item.id}/file`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = item.nama_file || item.judul
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

// ============================================================
// preview -- tanpa harus mendownload dulu. PDF ditampilkan lewat
// <iframe> memakai blob URL terautentikasi (sama pola dengan lihatFoto di
// AbsensiView.vue); DOCX dirender langsung jadi HTML lewat docx-preview;
// DOC (format biner lama) tidak didukung docx-preview -- fallback hanya
// menawarkan unduh dengan penjelasan singkat.
// ============================================================
const previewDialog = ref(false)
const previewTitle = ref('')
const previewLoading = ref(false)
const previewError = ref('')
const previewKind = ref('') // 'pdf' | 'docx' | 'unsupported'
const previewPdfUrl = ref('')
const previewDocxContainer = ref(null)
let previewObjectUrl = ''

async function bukaPreview(item) {
  previewTitle.value = item.judul
  previewDialog.value = true
  previewLoading.value = true
  previewError.value = ''
  previewKind.value = ''
  previewPdfUrl.value = ''

  const ext = ekstensi(item.nama_file)
  try {
    if (ext === 'pdf') {
      const res = await http.get(`/template-surat/${item.id}/file`, { params: { inline: 1 }, responseType: 'blob' })
      previewObjectUrl = URL.createObjectURL(res.data)
      previewPdfUrl.value = previewObjectUrl
      previewKind.value = 'pdf'
    } else if (ext === 'docx') {
      const res = await http.get(`/template-surat/${item.id}/file`, { params: { inline: 1 }, responseType: 'blob' })
      previewKind.value = 'docx'
      // previewLoading harus dimatikan SEBELUM docx-preview merender --
      // <div ref="previewDocxContainer"> baru muncul di DOM setelah
      // v-if="previewLoading" berubah jadi false (lihat template di bawah),
      // jadi kalau ini di dalam finally (dijalankan SETELAH percobaan
      // render), containernya belum ter-mount sama sekali & docx-preview
      // tidak punya tempat untuk merender apa pun -- dialog akhirnya
      // tampil kosong tanpa error apa pun. nextTick() menunggu DOM benar-
      // benar diperbarui dulu sebelum docx-preview dipanggil.
      previewLoading.value = false
      await nextTick()
      try {
        const { renderAsync } = await import('docx-preview')
        if (previewDocxContainer.value) {
          previewDocxContainer.value.innerHTML = ''
          await renderAsync(res.data, previewDocxContainer.value, previewDocxContainer.value, {
            inWrapper: true,
            ignoreLastRenderedPageBreak: true,
          })
        }
      } catch (err) {
        previewKind.value = 'unsupported'
        previewError.value = 'Gagal menampilkan isi berkas Word ini. Coba unduh langsung.'
      }
      return
    } else {
      // .doc lama (format biner, bukan OOXML) tidak bisa dipratinjau di
      // browser tanpa konversi server -- tawarkan unduh saja.
      previewKind.value = 'unsupported'
    }
  } catch (e) {
    previewError.value = e.response?.data?.message || e.message
    previewKind.value = 'unsupported'
  } finally {
    if (previewKind.value !== 'docx') previewLoading.value = false
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

onUnmounted(() => {
  if (previewObjectUrl) URL.revokeObjectURL(previewObjectUrl)
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-header-row">
      <div>
        <div class="page-title">Template Surat</div>
        <p class="page-subtitle">
          {{ canManage
            ? 'Kelola template surat (PDF/Word) yang bisa dilihat langsung oleh seluruh akun sekolah.'
            : 'Template surat yang disediakan administrator -- klik "Lihat" untuk membuka tanpa perlu mengunduh.' }}
        </p>
      </div>
      <Button v-if="canManage" label="Tambah Template" icon="pi pi-plus" @click="bukaTambah" />
    </div>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <Message v-else-if="!templates.length" severity="info" :closable="false">
      Belum ada template surat{{ canManage ? '' : ' yang ditambahkan administrator' }}.
    </Message>

    <div v-else class="template-grid">
      <div v-for="item in templates" :key="item.id" class="template-card">
        <div class="template-card-icon">
          <i :class="ikonFile(item.nama_file)"></i>
        </div>
        <div class="template-card-body">
          <div class="template-card-title" :title="item.judul">{{ item.judul }}</div>
          <div class="template-card-meta">{{ ekstensi(item.nama_file).toUpperCase() }} &middot; {{ formatTanggal(item.created_at) }}</div>
        </div>
        <div class="template-card-actions">
          <Button label="Lihat" icon="pi pi-eye" size="small" @click="bukaPreview(item)" />
          <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="unduhTemplate(item)" title="Unduh" />
          <template v-if="canManage">
            <Button icon="pi pi-pencil" size="small" severity="warn" outlined @click="bukaUbah(item)" title="Ubah" />
            <Button icon="pi pi-trash" size="small" severity="danger" outlined @click="konfirmasiHapus(item)" title="Hapus" />
          </template>
        </div>
      </div>
    </div>

    <!-- dialog tambah/ubah -->
    <Dialog v-model:visible="dialogVisible" modal :header="dialogMode === 'tambah' ? 'Tambah Template Surat' : 'Ubah Template Surat'" style="width: 32rem">
      <div class="form-field">
        <label>Judul Surat</label>
        <InputText v-model="form.judul" placeholder="mis. Surat Keterangan Sakit" style="width: 100%" />
      </div>
      <div class="form-field">
        <label>Berkas (PDF/DOC/DOCX){{ dialogMode === 'ubah' ? ' -- opsional, biarkan kosong kalau tidak diganti' : '' }}</label>
        <input ref="fileInput" type="file" accept=".pdf,.doc,.docx" style="display: none" @change="onFileChosen" />
        <div class="file-picker-row">
          <Button type="button" :label="chosenFile ? chosenFile.name : 'Pilih Berkas'" icon="pi pi-upload" severity="secondary" outlined @click="pilihFile" />
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" text @click="tutupDialog" />
        <Button label="Simpan" icon="pi pi-check" :loading="submitting" @click="simpanTemplate" />
      </template>
    </Dialog>

    <!-- dialog preview -->
    <Dialog v-model:visible="previewDialog" modal :header="previewTitle" style="width: 90vw; max-width: 900px" @hide="tutupPreview">
      <div v-if="previewLoading" style="display: flex; justify-content: center; padding: 3rem">
        <ProgressSpinner style="width: 42px; height: 42px" />
      </div>
      <template v-else>
        <iframe v-if="previewKind === 'pdf'" :src="previewPdfUrl" class="preview-frame"></iframe>
        <div v-else-if="previewKind === 'docx'" ref="previewDocxContainer" class="preview-docx"></div>
        <Message v-else severity="warn" :closable="false">
          {{ previewError || 'Berkas ini (format DOC lama) tidak bisa dipratinjau langsung di sini -- silakan unduh untuk membukanya.' }}
        </Message>
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

.template-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
}

.template-card {
  background: var(--p-content-background, #fff);
  border-radius: 12px;
  padding: 1.1rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}

.template-card-icon {
  font-size: 2.1rem;
  color: #0d9488;
}

.template-card-title {
  font-weight: 700;
  font-size: 0.98rem;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.template-card-meta {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.78rem;
}

.template-card-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: auto;
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

.file-picker-row {
  display: flex;
}

.preview-frame {
  width: 100%;
  height: 75vh;
  border: none;
}

.preview-docx {
  max-height: 75vh;
  overflow-y: auto;
  background: #f1f5f9;
  padding: 1rem;
}
</style>
