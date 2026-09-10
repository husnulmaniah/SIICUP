<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { useAuthStore } from '../stores/auth'
import http from '../api/http'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
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

const auth = useAuthStore()
const toast = useToast()
const confirm = useConfirm()

const items = ref([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const pageSize = ref(10)
const statusFilter = ref(null)
const search = ref('')

const jenisCutiOptions = ref([])
const polaOptions = ref([])
const pegawaiOptions = ref([])

const statusFilterOptions = [
  { label: 'Semua', value: null },
  { label: 'Menunggu', value: 'pending' },
  { label: 'Disetujui', value: 'disetujui' },
  { label: 'Ditolak', value: 'ditolak' },
]

const isManage = computed(() => auth.isAdministrator || auth.isAdmin)
const isAtasan = computed(() => auth.isAtasan)
const isPegawai = computed(() => auth.isPegawai)

function statusSeverity(status) {
  if (status === 'disetujui') return 'success'
  if (status === 'ditolak') return 'danger'
  return 'warn'
}

function formatDate(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'short', day: 'numeric' })
}

async function loadOptions() {
  const [jc, ph] = await Promise.all([http.get('/ref/jenis-cuti'), http.get('/ref/pola-hari-kerja')])
  jenisCutiOptions.value = jc.data.data || []
  polaOptions.value = ph.data.data || []
  if (isManage.value) {
    const peg = await http.get('/ref/pegawai')
    pegawaiOptions.value = peg.data.data || []
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
  id_pola_hari_kerja: null,
  alasan_cuti: '',
  alamat_selama_cuti: '',
})

function resetForm() {
  form.id = null
  form.id_pegawai = null
  form.id_jenis_cuti = null
  form.tgl_mulai = null
  form.tgl_selesai = null
  form.id_pola_hari_kerja = null
  form.alasan_cuti = ''
  form.alamat_selama_cuti = ''
}

function openCreate() {
  isEditing.value = false
  formErrors.value = ''
  resetForm()
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
  form.id_pola_hari_kerja = row.id_pola_hari_kerja
  form.alasan_cuti = row.alasan_cuti
  form.alamat_selama_cuti = row.alamat_selama_cuti
  formDialog.value = true
}

async function saveForm() {
  if (!form.id_jenis_cuti || !form.tgl_mulai || !form.tgl_selesai || (isManage.value && !form.id_pegawai)) {
    formErrors.value = 'Lengkapi semua data wajib (pegawai, jenis cuti, tanggal mulai & selesai)'
    return
  }
  formErrors.value = ''
  saving.value = true
  try {
    const payload = {
      id_pegawai: form.id_pegawai,
      id_jenis_cuti: form.id_jenis_cuti,
      tgl_mulai: form.tgl_mulai.toISOString().slice(0, 10),
      tgl_selesai: form.tgl_selesai.toISOString().slice(0, 10),
      id_pola_hari_kerja: form.id_pola_hari_kerja,
      alasan_cuti: form.alasan_cuti,
      alamat_selama_cuti: form.alamat_selama_cuti,
    }
    if (isEditing.value) {
      await http.put(`/pengajuan-cuti/${form.id}`, payload)
      toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengajuan cuti berhasil diperbarui', life: 3000 })
    } else {
      await http.post('/pengajuan-cuti', payload)
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

// ---- approval (atasan) ----
const approvalDialog = ref(false)
const approvalRow = ref(null)
const approvalNote = ref('')
const approvalLoading = ref(false)

function openApproval(row) {
  approvalRow.value = row
  approvalNote.value = ''
  approvalDialog.value = true
}

async function processApproval(action) {
  approvalLoading.value = true
  try {
    await http.put(`/pengajuan-cuti/${approvalRow.value.id}/${action}`, { catatan_approval: approvalNote.value })
    toast.add({
      severity: action === 'approve' ? 'success' : 'warn',
      summary: 'Berhasil',
      detail: action === 'approve' ? 'Pengajuan cuti disetujui' : 'Pengajuan cuti ditolak',
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

function openImport() {
  importFile.value = null
  importResult.value = null
  importDialog.value = true
}
function pickFile() {
  fileInputRef.value?.click()
}
function onFileChosen(e) {
  importFile.value = e.target.files[0] || null
}
async function submitImport() {
  if (!importFile.value) return
  importing.value = true
  try {
    const formData = new FormData()
    formData.append('file', importFile.value)
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
      <div class="toolbar-actions" style="justify-content: space-between; margin-bottom: 1rem; align-items: center; flex-wrap: wrap">
        <SelectButton v-model="statusFilter" :options="statusFilterOptions" optionLabel="label" optionValue="value" />
        <div class="toolbar-actions">
          <Button v-if="isPegawai || isManage" icon="pi pi-plus" label="Ajukan Cuti" @click="openCreate" />
          <template v-if="isManage">
            <Button icon="pi pi-download" label="Template" severity="secondary" outlined @click="downloadFile('/pengajuan-cuti/template', 'template_pengajuan_cuti.xlsx')" />
            <Button icon="pi pi-upload" label="Import" severity="secondary" outlined @click="openImport" />
            <Button icon="pi pi-file-export" label="Export" severity="secondary" outlined @click="downloadFile('/pengajuan-cuti/export', 'data_pengajuan_cuti.xlsx')" />
          </template>
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
          :rowsPerPageOptions="[10, 25, 50]"
          dataKey="id"
          stripedRows
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
          <Column header="Aksi" style="width: 150px">
            <template #body="{ data }">
              <div style="display: flex; gap: 0.35rem">
                <Button
                  v-if="isAtasan && data.status === 'pending'"
                  icon="pi pi-check-square"
                  size="small"
                  label="Proses"
                  @click="openApproval(data)"
                />
                <template v-if="(isPegawai && data.status === 'pending') || isManage">
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
    <Dialog v-model:visible="formDialog" modal :header="isEditing ? 'Edit Pengajuan Cuti' : 'Ajukan Cuti'" :style="{ width: '30rem', maxWidth: '95vw' }">
      <Message v-if="formErrors" severity="error" :closable="false" style="margin-bottom: 1rem">{{ formErrors }}</Message>
      <div style="display: flex; flex-direction: column; gap: 1rem">
        <div v-if="isManage">
          <label class="field-label">Pegawai *</label>
          <Select v-model="form.id_pegawai" :options="pegawaiOptions" :optionLabel="(o) => `${o.nama} (${o.nip})`" optionValue="id" filter style="width: 100%" placeholder="Pilih pegawai" />
        </div>
        <div>
          <label class="field-label">Jenis Cuti *</label>
          <Select v-model="form.id_jenis_cuti" :options="jenisCutiOptions" optionLabel="jenis" optionValue="id" style="width: 100%" placeholder="Pilih jenis cuti" />
        </div>
        <div style="display: flex; gap: 0.75rem">
          <div style="flex: 1">
            <label class="field-label">Tanggal Mulai *</label>
            <DatePicker v-model="form.tgl_mulai" dateFormat="yy-mm-dd" showIcon style="width: 100%" />
          </div>
          <div style="flex: 1">
            <label class="field-label">Tanggal Selesai *</label>
            <DatePicker v-model="form.tgl_selesai" dateFormat="yy-mm-dd" showIcon style="width: 100%" />
          </div>
        </div>
        <div>
          <label class="field-label">Pola Hari Kerja</label>
          <Select v-model="form.id_pola_hari_kerja" :options="polaOptions" optionLabel="pola" optionValue="id" showClear style="width: 100%" placeholder="Default: hitung Senin-Jumat" />
        </div>
        <div>
          <label class="field-label">Alasan Cuti</label>
          <Textarea v-model="form.alasan_cuti" rows="2" style="width: 100%" />
        </div>
        <div>
          <label class="field-label">Alamat Selama Cuti</label>
          <Textarea v-model="form.alamat_selama_cuti" rows="2" style="width: 100%" />
        </div>
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="formDialog = false" />
        <Button label="Simpan" :loading="saving" @click="saveForm" />
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
      </div>
      <label class="field-label">Catatan (opsional)</label>
      <Textarea v-model="approvalNote" rows="2" style="width: 100%" placeholder="Catatan untuk pegawai..." />
      <template #footer>
        <Button label="Tolak" severity="danger" outlined :loading="approvalLoading" @click="processApproval('reject')" />
        <Button label="Setujui" severity="success" :loading="approvalLoading" @click="processApproval('approve')" />
      </template>
    </Dialog>

    <!-- import dialog (admin/administrator) -->
    <Dialog v-model:visible="importDialog" modal header="Import Data dari Excel" :style="{ width: '34rem', maxWidth: '95vw' }">
      <p style="margin-top: 0; color: var(--p-text-muted-color); font-size: 0.9rem">
        Unduh template, isi data, lalu upload file excel (.xlsx). Import akan menambahkan pengajuan cuti baru.
      </p>
      <Button label="Download Template" icon="pi pi-download" severity="secondary" outlined @click="downloadFile('/pengajuan-cuti/template', 'template_pengajuan_cuti.xlsx')" style="margin-bottom: 1rem" />
      <input ref="fileInputRef" type="file" accept=".xlsx" style="display: none" @change="onFileChosen" />
      <div style="display: flex; gap: 0.5rem; align-items: center; margin-bottom: 1rem">
        <Button label="Pilih File Excel" icon="pi pi-file-excel" outlined @click="pickFile" />
        <span style="font-size: 0.85rem">{{ importFile?.name || 'Belum ada file dipilih' }}</span>
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
</style>
