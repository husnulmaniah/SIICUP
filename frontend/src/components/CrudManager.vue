<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Password from 'primevue/password'
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

// ---- detail dialog (khusus tabel Data Pegawai) ----
const isPegawaiTable = computed(() => props.config.endpoint === '/pegawai')
const detailDialogVisible = ref(false)
const detailLoading = ref(false)
const detailItem = ref(null)
const docFileInputRef = ref(null)
const pendingJenis = ref(null)
const docUploading = ref(false)

const dokumenSlots = [
  { jenis: 'sk-terakhir', field: 'sk_terakhir_nama', label: 'SK Terakhir' },
  { jenis: 'sk-kgb', field: 'sk_kgb_nama', label: 'SK Kenaikan Gaji Berkala' },
  { jenis: 'sk-pensiun', field: 'sk_pensiun_nama', label: 'SK Pensiun' },
]

function isPensiun(item) {
  const s = item?.status?.status || ''
  return s.toLowerCase().includes('pensiun')
}

function visibleDokumenSlots(item) {
  return dokumenSlots.filter((d) => d.jenis !== 'sk-pensiun' || isPensiun(item))
}

function docFilename(jenis) {
  const slot = dokumenSlots.find((d) => d.jenis === jenis)
  return (slot && detailItem.value?.[slot.field]) || ''
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
    form[f.field] = f.type === 'number' ? null : ''
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
    let v = row[f.field]
    if (f.type === 'date' && v) v = new Date(v)
    form[f.field] = v ?? (f.type === 'number' ? null : '')
  }
  form.__id = row.id
  dialogVisible.value = true
}

function serializeForm() {
  const payload = {}
  for (const f of props.config.formFields) {
    let v = form[f.field]
    if (f.type === 'date' && v instanceof Date) {
      v = v.toISOString().slice(0, 10)
    }
    if (f.type === 'password' && !v) {
      continue // don't send empty password on edit
    }
    payload[f.field] = v
  }
  return payload
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
    confirm.require({
      message: 'Semua data yang sudah ada di tabel ini akan DIHAPUS sebelum data dari file excel dimasukkan. Aksi ini tidak bisa dibatalkan. Lanjutkan?',
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

onMounted(() => {
  fetchList()
  loadRemoteOptions()
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
      <div class="toolbar-actions" style="justify-content: space-between; margin-bottom: 1rem; align-items: center">
        <IconField v-if="config.searchPlaceholder !== false" style="min-width: 220px; max-width: 320px; flex: 1">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" :placeholder="config.searchPlaceholder || 'Cari...'" style="width: 100%" />
        </IconField>
        <div class="toolbar-actions">
          <Button icon="pi pi-plus" label="Tambah" @click="openCreate" />
          <Button icon="pi pi-download" label="Template" severity="secondary" outlined @click="downloadTemplate" />
          <Button icon="pi pi-upload" label="Import" severity="secondary" outlined @click="openImportDialog" />
          <Button icon="pi pi-file-export" label="Export" severity="secondary" outlined @click="exportData" />
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
          :rowsPerPageOptions="[10, 25, 50, 100]"
          dataKey="id"
          stripedRows
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
              <template v-else>{{ fieldValue(data, col.field) ?? '-' }}</template>
            </template>
          </Column>
          <Column header="Aksi" :style="{ width: isPegawaiTable ? '170px' : '130px' }">
            <template #body="{ data }">
              <div style="display: flex; gap: 0.35rem">
                <Button v-if="isPegawaiTable" icon="pi pi-eye" size="small" severity="info" rounded text @click="openDetail(data)" />
                <Button icon="pi pi-pencil" size="small" severity="secondary" rounded text @click="openEdit(data)" />
                <Button icon="pi pi-trash" size="small" severity="danger" rounded text @click="confirmDelete(data)" />
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
          <label style="display: block; font-size: 0.85rem; font-weight: 600; margin-bottom: 0.35rem">
            {{ f.label }} <span v-if="f.required || (f.requiredOnCreate && !isEditing)" style="color: #ef4444">*</span>
          </label>
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
          <small v-if="f.hint" style="color: var(--p-text-muted-color)">{{ f.hint }}</small>
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
        <small v-if="importMode === 'replace'" style="color: #ef4444; display: block; margin-top: 0.4rem">
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

    <!-- Detail Pegawai + Dokumen dialog -->
    <input ref="docFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onDocFileChosen" />
    <Dialog v-model:visible="detailDialogVisible" modal header="Detail Pegawai" :style="{ width: '40rem', maxWidth: '95vw' }">
      <div v-if="detailLoading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 38px; height: 38px" />
      </div>
      <template v-else-if="detailItem">
        <div class="detail-grid">
          <div><span class="detail-label">NIP</span><div>{{ detailItem.nip || '-' }}</div></div>
          <div><span class="detail-label">Nama</span><div>{{ detailItem.nama || '-' }}</div></div>
          <div><span class="detail-label">Jabatan</span><div>{{ detailItem.jabatan?.jabatan || '-' }}</div></div>
          <div><span class="detail-label">Unit Kerja</span><div>{{ detailItem.unit_kerja?.unit || '-' }}</div></div>
          <div><span class="detail-label">Pangkat / Golongan</span><div>{{ detailItem.pangkat_gol?.pangkat?.pangkat || '-' }} / {{ detailItem.pangkat_gol?.gol?.gol || '-' }}</div></div>
          <div><span class="detail-label">Tempat Tugas</span><div>{{ detailItem.tempat_tgs || '-' }}</div></div>
          <div><span class="detail-label">TMT</span><div>{{ formatDate(detailItem.tmt) }}</div></div>
          <div><span class="detail-label">No HP</span><div>{{ detailItem.no_hp || '-' }}</div></div>
          <div><span class="detail-label">Email</span><div>{{ detailItem.email || '-' }}</div></div>
          <div><span class="detail-label">Status</span><div><Tag :value="detailItem.status?.status || '-'" severity="info" /></div></div>
          <div><span class="detail-label">Atasan Langsung</span><div>{{ detailItem.atasan?.nama || '-' }}</div></div>
        </div>

        <h4 style="margin: 1.5rem 0 0.75rem 0">Dokumen</h4>
        <div v-for="slot in visibleDokumenSlots(detailItem)" :key="slot.jenis" class="doc-row">
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
      </template>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="detailDialogVisible = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
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
