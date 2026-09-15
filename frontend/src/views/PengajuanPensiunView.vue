<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Textarea from 'primevue/textarea'
import InputNumber from 'primevue/inputnumber'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'

const toast = useToast()
const confirm = useConfirm()

// ============================================================
// Tab "Pengajuan Pensiun" -- daftar pengajuan pensiun dari pegawai (lewat
// Profil Saya), disetujui/ditolak di sini. Kalau disetujui, status pegawai
// otomatis berubah jadi "Pensiun" & akun login pegawai itu otomatis
// dinonaktifkan (lihat backend/handlers/pengajuan_pensiun.go). Persetujuan
// yang sudah diberikan bisa dibatalkan lagi kapan saja lewat tombol
// "Batalkan Persetujuan" -- ini akan mengembalikan status pegawai & akun
// login-nya seperti semula.
// ============================================================
const items = ref([])
const loading = ref(false)
const statusFilter = ref('pending')
const statusFilterOptions = [
  { label: 'Menunggu', value: 'pending' },
  { label: 'Disetujui', value: 'disetujui' },
  { label: 'Ditolak', value: 'ditolak' },
  { label: 'Semua', value: null },
]

async function fetchList() {
  loading.value = true
  try {
    const { data } = await http.get('/pengajuan-pensiun', { params: { status: statusFilter.value || undefined } })
    items.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

watch(statusFilter, fetchList)

function statusSeverity(s) {
  return s === 'disetujui' ? 'success' : s === 'ditolak' ? 'danger' : 'warn'
}
function statusLabel(s) {
  return s === 'disetujui' ? 'Disetujui' : s === 'ditolak' ? 'Ditolak' : 'Menunggu'
}
function formatDateTime(v) {
  if (!v) return '-'
  return new Date(v).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
}

// ---- detail dialog + aksi setujui/tolak/batalkan persetujuan ----
const detailDialog = ref(false)
const detailRow = ref(null)
const catatan = ref('')
const processing = ref(false)

function openDetail(row) {
  detailRow.value = row
  catatan.value = ''
  detailDialog.value = true
}

async function doApprove() {
  processing.value = true
  try {
    await http.put(`/pengajuan-pensiun/${detailRow.value.id}/approve`, { catatan_admin: catatan.value })
    toast.add({
      severity: 'success',
      summary: 'Disetujui',
      detail: 'Status pegawai diubah jadi Pensiun & akun login pegawai dinonaktifkan',
      life: 5000,
    })
    detailDialog.value = false
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyetujui', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    processing.value = false
  }
}

function confirmApprove() {
  confirm.require({
    message: 'Setujui pengajuan pensiun ini? Status pegawai akan langsung berubah jadi "Pensiun" dan akun login pegawai tersebut akan dinonaktifkan (tidak bisa login lagi).',
    header: 'Konfirmasi Setujui',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Setujui',
    rejectLabel: 'Batal',
    accept: doApprove,
  })
}

async function doReject() {
  if (!catatan.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Alasan wajib diisi', detail: 'Isi catatan/alasan penolakan terlebih dahulu', life: 3500 })
    return
  }
  processing.value = true
  try {
    await http.put(`/pengajuan-pensiun/${detailRow.value.id}/reject`, { catatan_admin: catatan.value })
    toast.add({ severity: 'success', summary: 'Ditolak', detail: 'Pengajuan pensiun telah ditolak', life: 4000 })
    detailDialog.value = false
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menolak', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    processing.value = false
  }
}

function confirmBatalkanPersetujuan(row) {
  confirm.require({
    message: `Batalkan persetujuan pensiun untuk ${row.pegawai?.nama || 'pegawai ini'}? Status pegawai akan dikembalikan seperti sebelum disetujui & akun login pegawai akan diaktifkan kembali.`,
    header: 'Konfirmasi Batalkan Persetujuan',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Batalkan',
    rejectLabel: 'Tidak',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await http.put(`/pengajuan-pensiun/${row.id}/batalkan-persetujuan`)
        toast.add({
          severity: 'success',
          summary: 'Berhasil',
          detail: 'Persetujuan dibatalkan -- status pegawai & akun login pegawai dikembalikan seperti semula',
          life: 5000,
        })
        fetchList()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

// ---- lihat / unduh berkas SK / usulan pensiun ----
const previewDialog = ref(false)
const previewUrl = ref('')
const previewType = ref('pdf')

async function previewSk(row) {
  try {
    const res = await http.get(`/pengajuan-pensiun/${row.id}/dokumen`, { params: { inline: 1 }, responseType: 'blob' })
    const ext = (row.sk_nama_file || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
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

// ============================================================
// Tab "Pengaturan Usia Pensiun" -- usia pensiun standar per jenis jabatan,
// bisa diubah administrator/admin. Dipakai backend untuk memvalidasi
// pengajuan pensiun (non-"pensiun dini") pegawai lewat Profil Saya.
// ============================================================
const pengaturan = reactive({ usia_pelaksana_struktural: 58, usia_fungsional: 60 })
const loadingPengaturan = ref(false)
const savingPengaturan = ref(false)

async function loadPengaturan() {
  loadingPengaturan.value = true
  try {
    const { data } = await http.get('/pengaturan-pensiun')
    pengaturan.usia_pelaksana_struktural = data.data.usia_pelaksana_struktural
    pengaturan.usia_fungsional = data.data.usia_fungsional
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingPengaturan.value = false
  }
}

async function simpanPengaturan() {
  savingPengaturan.value = true
  try {
    await http.put('/pengaturan-pensiun', {
      usia_pelaksana_struktural: pengaturan.usia_pelaksana_struktural,
      usia_fungsional: pengaturan.usia_fungsional,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengaturan usia pensiun disimpan', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    savingPengaturan.value = false
  }
}

onMounted(() => {
  fetchList()
  loadPengaturan()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Pengajuan Pensiun</div>
    <p class="page-subtitle">
      Tinjau & setujui/tolak pengajuan pensiun yang dikirim pegawai. Jika disetujui, status pegawai otomatis berubah jadi
      "Pensiun" dan akun login pegawai tersebut otomatis dinonaktifkan.
    </p>

    <Tabs value="pengajuan">
      <TabList>
        <Tab value="pengajuan"><i class="pi pi-briefcase" style="margin-right: 0.4rem"></i> Pengajuan Pensiun</Tab>
        <Tab value="pengaturan"><i class="pi pi-cog" style="margin-right: 0.4rem"></i> Pengaturan Usia Pensiun</Tab>
      </TabList>
      <TabPanels>
        <TabPanel value="pengajuan">
          <div class="card">
            <SelectButton
              v-model="statusFilter"
              :options="statusFilterOptions"
              optionLabel="label"
              optionValue="value"
              class="tab-filter"
              style="width: 100%; flex-wrap: wrap"
            />

            <div class="responsive-table-wrap">
              <DataTable :value="items" :loading="loading" dataKey="id" stripedRows size="small" style="min-width: 640px">
                <template #empty>
                  <div style="padding: 1.5rem; text-align: center; color: var(--p-text-muted-color)">Tidak ada data</div>
                </template>
                <Column field="pegawai.nama" header="Nama Pegawai">
                  <template #body="{ data }">{{ data.pegawai?.nama || '-' }}</template>
                </Column>
                <Column field="pegawai.nip" header="NIP" style="width: 170px">
                  <template #body="{ data }">{{ data.pegawai?.nip || '-' }}</template>
                </Column>
                <Column header="Jenis" style="width: 130px">
                  <template #body="{ data }">
                    <Tag v-if="data.is_pensiun_dini" value="Pensiun Dini" severity="help" />
                    <Tag v-else value="Reguler" severity="secondary" />
                  </template>
                </Column>
                <Column header="Tanggal Pengajuan" style="width: 170px">
                  <template #body="{ data }">{{ formatDateTime(data.created_at) }}</template>
                </Column>
                <Column header="Status" style="width: 140px">
                  <template #body="{ data }"><Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" /></template>
                </Column>
                <Column header="Aksi" style="width: 150px">
                  <template #body="{ data }">
                    <div class="table-actions">
                      <Button icon="pi pi-eye" size="small" severity="info" rounded text @click="openDetail(data)" />
                      <Button
                        v-if="data.status === 'disetujui'"
                        icon="pi pi-undo"
                        size="small"
                        severity="danger"
                        rounded
                        text
                        title="Batalkan Persetujuan"
                        @click="confirmBatalkanPersetujuan(data)"
                      />
                    </div>
                  </template>
                </Column>
              </DataTable>
            </div>
          </div>
        </TabPanel>

        <TabPanel value="pengaturan">
          <div class="card">
            <div v-if="loadingPengaturan" style="display: flex; justify-content: center; padding: 1.5rem">
              <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
            </div>
            <template v-else>
              <Message severity="info" :closable="false" style="margin-bottom: 1.25rem">
                Usia pensiun standar ini dipakai untuk memvalidasi pengajuan pensiun pegawai (di luar opsi "Pensiun Dini", yang
                melewati batas usia ini). Jenis Jabatan tiap pegawai diatur di menu Master Data &rarr; Jabatan.
              </Message>
              <div class="pengaturan-grid">
                <div>
                  <label class="field-label">Usia Pensiun Pelaksana &amp; Struktural (tahun)</label>
                  <InputNumber v-model="pengaturan.usia_pelaksana_struktural" :min="40" :max="75" showButtons style="width: 100%" fluid />
                </div>
                <div>
                  <label class="field-label">Usia Pensiun Fungsional (tahun)</label>
                  <InputNumber v-model="pengaturan.usia_fungsional" :min="40" :max="75" showButtons style="width: 100%" fluid />
                </div>
              </div>
              <Button label="Simpan Pengaturan" icon="pi pi-save" :loading="savingPengaturan" style="margin-top: 1.25rem" @click="simpanPengaturan" />
            </template>
          </div>
        </TabPanel>
      </TabPanels>
    </Tabs>

    <!-- Detail + review dialog -->
    <Dialog v-model:visible="detailDialog" modal header="Detail Pengajuan Pensiun" :style="{ width: '40rem', maxWidth: '95vw' }">
      <template v-if="detailRow">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 0.75rem; flex-wrap: wrap; margin-bottom: 1rem">
          <div>
            <div style="font-weight: 600">{{ detailRow.pegawai?.nama }}</div>
            <div style="font-size: 0.85rem; color: var(--p-text-muted-color)">NIP {{ detailRow.pegawai?.nip }}</div>
            <div style="font-size: 0.8rem; color: var(--p-text-muted-color)">Diajukan {{ formatDateTime(detailRow.created_at) }}</div>
          </div>
          <Tag :value="statusLabel(detailRow.status)" :severity="statusSeverity(detailRow.status)" />
        </div>

        <Tag v-if="detailRow.is_pensiun_dini" value="Pengajuan Pensiun Dini" severity="help" style="margin-bottom: 0.75rem" />

        <div v-if="detailRow.alasan" style="margin-bottom: 1rem">
          <div class="field-label">Alasan</div>
          <div style="font-size: 0.9rem">{{ detailRow.alasan }}</div>
        </div>

        <div style="margin-top: 0.5rem">
          <div class="field-label">Berkas SK / Usulan Pensiun</div>
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <span style="font-size: 0.85rem">{{ detailRow.sk_nama_file || '-' }}</span>
            <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat" @click="previewSk(detailRow)" />
            <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadSk(detailRow)" />
          </div>
        </div>

        <template v-if="detailRow.status === 'pending'">
          <div style="margin-top: 1.25rem">
            <label class="field-label">Catatan (wajib diisi jika menolak)</label>
            <Textarea v-model="catatan" rows="3" style="width: 100%" placeholder="Catatan untuk pegawai, mis. alasan penolakan atau catatan persetujuan" />
          </div>
        </template>
        <template v-else>
          <Message :severity="statusSeverity(detailRow.status)" :closable="false" style="margin-top: 1.25rem">
            Diputuskan oleh {{ detailRow.diputuskan_oleh || '-' }} pada {{ formatDateTime(detailRow.tgl_keputusan) }}.
            <template v-if="detailRow.catatan_admin">Catatan: "{{ detailRow.catatan_admin }}"</template>
          </Message>
        </template>
      </template>

      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="detailDialog = false" />
        <template v-if="detailRow?.status === 'pending'">
          <Button label="Tolak" icon="pi pi-times" severity="danger" outlined :loading="processing" @click="doReject" />
          <Button label="Setujui" icon="pi pi-check" :loading="processing" @click="confirmApprove" />
        </template>
      </template>
    </Dialog>

    <!-- Preview dokumen SK -->
    <Dialog v-model:visible="previewDialog" modal header="Berkas SK / Usulan Pensiun" :style="{ width: '95vw', maxWidth: '62rem' }" @hide="closePreview">
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

.pengaturan-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem;
}

@media (max-width: 480px) {
  .pengaturan-grid {
    grid-template-columns: 1fr;
  }
}
</style>
