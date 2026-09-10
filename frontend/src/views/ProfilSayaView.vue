<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

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
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data profil', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
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

onMounted(() => {
  loadProfile()
  loadRiwayat()
  loadRefs()
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
  no_hp: '',
  id_status: null,
  email: '',
})
const skFile = ref(null)
const skFileInputRef = ref(null)

function openAjukan() {
  if (!profile.value) return
  form.nama = profile.value.nama || ''
  form.id_jabatan = profile.value.id_jabatan || null
  form.id_unit_kerja = profile.value.id_unit_kerja || null
  form.id_pangkat_gol = profile.value.id_pangkat_gol || null
  form.tempat_tgs = profile.value.tempat_tgs || ''
  form.tmt = profile.value.tmt ? new Date(profile.value.tmt) : null
  form.no_hp = profile.value.no_hp || ''
  form.id_status = profile.value.id_status || null
  form.email = profile.value.email || ''
  skFile.value = null
  formErrors.value = ''
  dialogVisible.value = true
}

function pickSkFile() {
  skFileInputRef.value?.click()
}
function onSkFileChosen(e) {
  skFile.value = e.target.files[0] || null
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
  if (!skFile.value) {
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
      no_hp: form.no_hp,
      id_status: form.id_status,
      email: form.email,
    }
    const fd = new FormData()
    fd.append('data', JSON.stringify(payload))
    fd.append('file', skFile.value)
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
        <div class="detail-grid">
          <div><span class="detail-label">NIP</span><div>{{ profile.nip || '-' }}</div></div>
          <div><span class="detail-label">Nama</span><div>{{ profile.nama || '-' }}</div></div>
          <div><span class="detail-label">Jabatan</span><div>{{ profile.jabatan?.jabatan || '-' }}</div></div>
          <div><span class="detail-label">Unit Kerja</span><div>{{ profile.unit_kerja?.unit || '-' }}</div></div>
          <div>
            <span class="detail-label">Pangkat / Golongan</span>
            <div>{{ profile.pangkat_gol?.pangkat?.pangkat || '-' }} / {{ profile.pangkat_gol?.gol?.gol || '-' }}</div>
          </div>
          <div><span class="detail-label">Tempat Tugas</span><div>{{ profile.tempat_tgs || '-' }}</div></div>
          <div><span class="detail-label">TMT</span><div>{{ formatDate(profile.tmt) }}</div></div>
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
          <label class="field-label">No HP</label>
          <InputText v-model="form.no_hp" style="width: 100%" />
        </div>
        <div class="col-12 md:col-6">
          <label class="field-label">Email</label>
          <InputText v-model="form.email" style="width: 100%" />
        </div>
        <div class="col-12">
          <label class="field-label">Berkas SK Terakhir <span style="color: #ef4444">*</span></label>
          <input ref="skFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onSkFileChosen" />
          <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap">
            <Button label="Pilih Berkas" icon="pi pi-upload" outlined @click="pickSkFile" />
            <span style="font-size: 0.85rem">{{ skFile?.name || 'Belum ada berkas dipilih' }}</span>
          </div>
          <small style="color: var(--p-text-muted-color); display: block; margin-top: 0.3rem">Format PDF, JPG, atau PNG. Berkas ini menjadi dasar & bukti perubahan data yang diajukan.</small>
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
