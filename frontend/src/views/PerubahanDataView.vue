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
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import InputNumber from 'primevue/inputnumber'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'

const toast = useToast()
const confirm = useConfirm()

const items = ref([])
const loading = ref(false)
const statusFilter = ref('pending')
const statusFilterOptions = [
  { label: 'Menunggu', value: 'pending' },
  { label: 'Disetujui', value: 'disetujui' },
  { label: 'Ditolak', value: 'ditolak' },
  { label: 'Semua', value: null },
]

const refJabatan = ref([])
const refUnitKerja = ref([])
const refPangkatGol = ref([])
const refStatus = ref([])

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
    // gagal memuat referensi -- diff tetap ditampilkan pakai ID mentah
  }
}

async function fetchList() {
  loading.value = true
  try {
    const { data } = await http.get('/perubahan-data', { params: { status: statusFilter.value || undefined } })
    items.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

watch(statusFilter, fetchList)

onMounted(() => {
  loadRefs()
  fetchList()
  loadPengaturanKgb()
})

function jabatanLabel(id) {
  if (!id) return '-'
  return refJabatan.value.find((x) => x.id === id)?.jabatan || `#${id}`
}
function unitKerjaLabel(id) {
  if (!id) return '-'
  return refUnitKerja.value.find((x) => x.id === id)?.unit || `#${id}`
}
function pangkatGolLabel(id) {
  if (!id) return '-'
  const pg = refPangkatGol.value.find((x) => x.id === id)
  if (!pg) return `#${id}`
  return `${pg.pangkat?.pangkat || '-'} / ${pg.gol?.gol || '-'}`
}
function statusPegawaiLabel(id) {
  if (!id) return '-'
  return refStatus.value.find((x) => x.id === id)?.status || `#${id}`
}
function formatDate(v) {
  if (!v) return '-'
  return new Date(v).toLocaleDateString('id-ID', { dateStyle: 'medium' })
}

function parseJson(raw) {
  try {
    return JSON.parse(raw || '{}')
  } catch (e) {
    return {}
  }
}

// baris perbandingan data lama vs data baru untuk dialog detail
function diffRows(row) {
  const lama = parseJson(row.data_lama)
  const baru = parseJson(row.data_baru)
  const fields = [
    { key: 'nama', label: 'Nama' },
    { key: 'id_jabatan', label: 'Jabatan', fmt: jabatanLabel },
    { key: 'id_unit_kerja', label: 'Unit Kerja', fmt: unitKerjaLabel },
    { key: 'id_pangkat_gol', label: 'Pangkat / Golongan', fmt: pangkatGolLabel },
    { key: 'tempat_tgs', label: 'Tempat Tugas' },
    { key: 'tmt', label: 'TMT', fmt: (v) => (v ? formatDate(v) : '-') },
    { key: 'tgl_lahir', label: 'Tanggal Lahir', fmt: (v) => (v ? formatDate(v) : '-') },
    { key: 'tgl_kenaikan_gaji_berkala_terakhir', label: 'Kenaikan Gaji Berkala Terakhir', fmt: (v) => (v ? formatDate(v) : '-') },
    { key: 'tgl_kenaikan_pangkat_terakhir', label: 'Kenaikan Pangkat Terakhir', fmt: (v) => (v ? formatDate(v) : '-') },
    { key: 'no_hp', label: 'No HP' },
    { key: 'id_status', label: 'Status Kepegawaian', fmt: statusPegawaiLabel },
    { key: 'email', label: 'Email' },
  ]
  return fields.map((f) => {
    const rawLama = lama[f.key]
    const rawBaru = baru[f.key]
    return {
      label: f.label,
      lama: f.fmt ? f.fmt(rawLama) : rawLama || '-',
      baru: f.fmt ? f.fmt(rawBaru) : rawBaru || '-',
      changed: JSON.stringify(rawLama || null) !== JSON.stringify(rawBaru || null),
    }
  })
}

function statusSeverity(s) {
  return s === 'disetujui' ? 'success' : s === 'ditolak' ? 'danger' : 'warn'
}
function statusLabel(s) {
  return s === 'disetujui' ? 'Disetujui' : s === 'ditolak' ? 'Ditolak' : 'Menunggu'
}

// ---- detail dialog + aksi setujui/tolak ----
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
    await http.put(`/perubahan-data/${detailRow.value.id}/approve`, { catatan_admin: catatan.value })
    toast.add({ severity: 'success', summary: 'Disetujui', detail: 'Data pegawai & akun pegawai telah diperbarui', life: 4000 })
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
    message: 'Setujui perubahan data ini? Data pegawai & akun pegawai terkait akan langsung diperbarui.',
    header: 'Konfirmasi Setujui',
    icon: 'pi pi-check-circle',
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
    await http.put(`/perubahan-data/${detailRow.value.id}/reject`, { catatan_admin: catatan.value })
    toast.add({ severity: 'success', summary: 'Ditolak', detail: 'Pengajuan perubahan data telah ditolak', life: 4000 })
    detailDialog.value = false
    fetchList()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menolak', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    processing.value = false
  }
}

// ---- lihat / unduh berkas SK ----
const previewDialog = ref(false)
const previewUrl = ref('')
const previewType = ref('pdf')

async function previewSk(row) {
  try {
    const res = await http.get(`/perubahan-data/${row.id}/dokumen`, { params: { inline: 1 }, responseType: 'blob' })
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

// ---- lihat / unduh berkas SK Kenaikan Gaji Berkala (opsional, kalau
// diupload pegawai saat mengajukan -- lihat backend/handlers/perubahan_data.go) ----
async function previewSkKgb(row) {
  try {
    const res = await http.get(`/perubahan-data/${row.id}/dokumen-kgb`, { params: { inline: 1 }, responseType: 'blob' })
    const ext = (row.sk_kgb_nama || '').split('.').pop().toLowerCase()
    previewType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewUrl.value = window.URL.createObjectURL(res.data)
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
async function downloadSkKgb(row) {
  try {
    const res = await http.get(`/perubahan-data/${row.id}/dokumen-kgb`, { responseType: 'blob' })
    const blobUrl = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = blobUrl
    link.download = row.sk_kgb_nama || 'sk-kgb'
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(blobUrl)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh berkas', detail: e.message, life: 4000 })
  }
}

// ============================================================
// Tab "Pengaturan Kenaikan Gaji Berkala" -- interval standar (tahun) untuk
// kenaikan gaji berkala & kenaikan pangkat per jenis jabatan, bisa diubah
// administrator/admin. Dipakai frontend (Profil Saya & Data Pegawai) untuk
// MENGHITUNG & MENAMPILKAN kapan kenaikan berikutnya jatuh tempo -- lihat
// backend/handlers/kenaikan_gaji_berkala.go. TIDAK ada alur
// pengajuan/persetujuan tersendiri seperti Pensiun; tanggal kenaikan
// terakhir & berkas SK-nya diajukan lewat tab "Perubahan Data" di atas
// (pegawai lewat Profil Saya) atau diisi langsung di menu Data Pegawai.
// ============================================================
const pengaturanKgb = reactive({
  gaji_berkala_fungsional_tahun: 1,
  gaji_berkala_pelaksana_struktural_tahun: 2,
  pangkat_fungsional_tahun: 2,
  pangkat_pelaksana_struktural_tahun: 4,
})
const loadingPengaturanKgb = ref(false)
const savingPengaturanKgb = ref(false)

async function loadPengaturanKgb() {
  loadingPengaturanKgb.value = true
  try {
    const { data } = await http.get('/pengaturan-kenaikan-gaji-berkala')
    pengaturanKgb.gaji_berkala_fungsional_tahun = data.data.gaji_berkala_fungsional_tahun
    pengaturanKgb.gaji_berkala_pelaksana_struktural_tahun = data.data.gaji_berkala_pelaksana_struktural_tahun
    pengaturanKgb.pangkat_fungsional_tahun = data.data.pangkat_fungsional_tahun
    pengaturanKgb.pangkat_pelaksana_struktural_tahun = data.data.pangkat_pelaksana_struktural_tahun
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingPengaturanKgb.value = false
  }
}

async function simpanPengaturanKgb() {
  savingPengaturanKgb.value = true
  try {
    await http.put('/pengaturan-kenaikan-gaji-berkala', {
      gaji_berkala_fungsional_tahun: pengaturanKgb.gaji_berkala_fungsional_tahun,
      gaji_berkala_pelaksana_struktural_tahun: pengaturanKgb.gaji_berkala_pelaksana_struktural_tahun,
      pangkat_fungsional_tahun: pengaturanKgb.pangkat_fungsional_tahun,
      pangkat_pelaksana_struktural_tahun: pengaturanKgb.pangkat_pelaksana_struktural_tahun,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengaturan kenaikan gaji berkala disimpan', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    savingPengaturanKgb.value = false
  }
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Perubahan Data Pegawai</div>
    <p class="page-subtitle">Tinjau & setujui/tolak pengajuan perubahan data diri yang dikirim pegawai. Jika disetujui, data pegawai dan akun pegawai terkait diperbarui otomatis.</p>

    <Tabs value="perubahan">
      <TabList>
        <Tab value="perubahan"><i class="pi pi-user-edit" style="margin-right: 0.4rem"></i> Perubahan Data</Tab>
        <Tab value="pengaturan"><i class="pi pi-cog" style="margin-right: 0.4rem"></i> Pengaturan Kenaikan Gaji Berkala</Tab>
      </TabList>
      <TabPanels>
        <TabPanel value="perubahan">
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
                <Column header="Tanggal Pengajuan" style="width: 170px">
                  <template #body="{ data }">{{ formatDate(data.created_at) }}</template>
                </Column>
                <Column header="Status" style="width: 140px">
                  <template #body="{ data }"><Tag :value="statusLabel(data.status)" :severity="statusSeverity(data.status)" /></template>
                </Column>
                <Column header="Aksi" style="width: 110px">
                  <template #body="{ data }">
                    <Button icon="pi pi-eye" size="small" severity="info" rounded text @click="openDetail(data)" />
                  </template>
                </Column>
              </DataTable>
            </div>
          </div>
        </TabPanel>

        <TabPanel value="pengaturan">
          <div class="card">
            <div v-if="loadingPengaturanKgb" style="display: flex; justify-content: center; padding: 1.5rem">
              <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
            </div>
            <template v-else>
              <Message severity="info" :closable="false" style="margin-bottom: 1.25rem">
                Interval standar ini dipakai untuk menghitung & menampilkan kapan kenaikan gaji berkala/pangkat berikutnya jatuh
                tempo, berdasarkan tanggal kenaikan terakhir tiap pegawai (diisi lewat tab "Perubahan Data" di atas oleh pegawai
                sendiri, atau langsung oleh administrator di menu Data Pegawai). Jenis Jabatan tiap pegawai diatur di menu Master
                Data &rarr; Jabatan.
              </Message>
              <h4 style="margin: 0 0 0.75rem 0">Kenaikan Gaji Berkala</h4>
              <div class="pengaturan-grid">
                <div>
                  <label class="field-label">Pelaksana &amp; Struktural (tahun sekali)</label>
                  <InputNumber v-model="pengaturanKgb.gaji_berkala_pelaksana_struktural_tahun" :min="1" :max="10" showButtons style="width: 100%" fluid />
                </div>
                <div>
                  <label class="field-label">Fungsional (tahun sekali)</label>
                  <InputNumber v-model="pengaturanKgb.gaji_berkala_fungsional_tahun" :min="1" :max="10" showButtons style="width: 100%" fluid />
                </div>
              </div>
              <h4 style="margin: 1.5rem 0 0.75rem 0">Kenaikan Pangkat</h4>
              <div class="pengaturan-grid">
                <div>
                  <label class="field-label">Pelaksana &amp; Struktural (tahun sekali)</label>
                  <InputNumber v-model="pengaturanKgb.pangkat_pelaksana_struktural_tahun" :min="1" :max="10" showButtons style="width: 100%" fluid />
                </div>
                <div>
                  <label class="field-label">Fungsional (tahun sekali)</label>
                  <InputNumber v-model="pengaturanKgb.pangkat_fungsional_tahun" :min="1" :max="10" showButtons style="width: 100%" fluid />
                </div>
              </div>
              <Button label="Simpan Pengaturan" icon="pi pi-save" :loading="savingPengaturanKgb" style="margin-top: 1.25rem" @click="simpanPengaturanKgb" />
            </template>
          </div>
        </TabPanel>
      </TabPanels>
    </Tabs>

    <!-- Detail + review dialog -->
    <Dialog v-model:visible="detailDialog" modal header="Detail Pengajuan Perubahan Data" :style="{ width: '44rem', maxWidth: '95vw' }">
      <template v-if="detailRow">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 0.75rem; flex-wrap: wrap; margin-bottom: 1rem">
          <div>
            <div style="font-weight: 600">{{ detailRow.pegawai?.nama }}</div>
            <div style="font-size: 0.85rem; color: var(--p-text-muted-color)">NIP {{ detailRow.pegawai?.nip }}</div>
            <div style="font-size: 0.8rem; color: var(--p-text-muted-color)">Diajukan {{ formatDate(detailRow.created_at) }}</div>
          </div>
          <Tag :value="statusLabel(detailRow.status)" :severity="statusSeverity(detailRow.status)" />
        </div>

        <div class="responsive-table-wrap">
          <table class="diff-table">
            <thead>
              <tr>
                <th>Field</th>
                <th>Data Lama</th>
                <th>Data Baru (Diajukan)</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in diffRows(detailRow)" :key="d.label" :class="{ changed: d.changed }">
                <td>{{ d.label }}</td>
                <td>{{ d.lama }}</td>
                <td>{{ d.baru }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div style="margin-top: 1rem">
          <div style="font-size: 0.85rem; font-weight: 600; margin-bottom: 0.4rem">Berkas SK Terakhir (dasar perubahan)</div>
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <span style="font-size: 0.85rem">{{ detailRow.sk_nama_file || '-' }}</span>
            <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat" @click="previewSk(detailRow)" />
            <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadSk(detailRow)" />
          </div>
        </div>

        <div v-if="detailRow.sk_kgb_nama" style="margin-top: 1rem">
          <div style="font-size: 0.85rem; font-weight: 600; margin-bottom: 0.4rem">Berkas SK Kenaikan Gaji Berkala (opsional)</div>
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <span style="font-size: 0.85rem">{{ detailRow.sk_kgb_nama }}</span>
            <Button icon="pi pi-eye" size="small" severity="secondary" outlined label="Lihat" @click="previewSkKgb(detailRow)" />
            <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="downloadSkKgb(detailRow)" />
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
            Diputuskan oleh {{ detailRow.diputuskan_oleh || '-' }} pada {{ formatDate(detailRow.tgl_keputusan) }}.
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
    <Dialog v-model:visible="previewDialog" modal header="Berkas SK Terakhir" :style="{ width: '95vw', maxWidth: '62rem' }" @hide="closePreview">
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

.diff-table {
  width: 100%;
  min-width: 480px;
  border-collapse: collapse;
  font-size: 0.85rem;
}

.diff-table th,
.diff-table td {
  text-align: left;
  padding: 0.5rem 0.6rem;
  border-bottom: 1px solid #f1f5f9;
}

.diff-table th {
  font-weight: 600;
  color: var(--p-text-muted-color);
  font-size: 0.78rem;
}

.diff-table tr.changed td:nth-child(3) {
  color: #16a34a;
  font-weight: 600;
}

.diff-table tr.changed td:nth-child(2) {
  color: #9ca3af;
  text-decoration: line-through;
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
