<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'

import Button from 'primevue/button'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import InputText from 'primevue/inputtext'

const toast = useToast()

// ============================================================
// pengaturan (aktif/nonaktif & jendela waktu)
// ============================================================

const pengaturan = reactive({ aktif: true, jam_mulai_pagi: '', jam_batas_pagi: '', jam_mulai_pulang: '' })
const loadingPengaturan = ref(true)
const savingPengaturan = ref(false)

async function loadPengaturan() {
  loadingPengaturan.value = true
  try {
    const { data } = await http.get('/absensi/pengaturan')
    Object.assign(pengaturan, data.data)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan absen', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingPengaturan.value = false
  }
}

const jamPattern = /^([01]\d|2[0-3]):[0-5]\d$/
function jamValid(v) {
  return jamPattern.test(v || '')
}
const pengaturanValid = computed(
  () => jamValid(pengaturan.jam_mulai_pagi) && jamValid(pengaturan.jam_batas_pagi) && jamValid(pengaturan.jam_mulai_pulang),
)

async function savePengaturan() {
  if (!pengaturanValid.value) {
    toast.add({ severity: 'warn', summary: 'Periksa kembali', detail: 'Semua jam wajib berformat HH:MM (contoh: 07:30)', life: 4000 })
    return
  }
  savingPengaturan.value = true
  try {
    const { data } = await http.put('/absensi/pengaturan', { ...pengaturan })
    Object.assign(pengaturan, data.data)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    savingPengaturan.value = false
  }
}

// ============================================================
// rekap semua pegawai
// ============================================================

const periodDate = ref(new Date())
const pegawaiOptions = ref([])
const selectedPegawai = ref(null)
const rekap = ref([])
const loadingRekap = ref(false)

async function loadPegawaiOptions() {
  try {
    const { data } = await http.get('/pegawai', { params: { pageSize: 500 } })
    const list = data.data || []
    pegawaiOptions.value = list.map((p) => ({ label: `${p.nama} (${p.nip})`, value: p.id }))
  } catch {
    pegawaiOptions.value = []
  }
}

async function loadRekap() {
  loadingRekap.value = true
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    if (selectedPegawai.value) params.id_pegawai = selectedPegawai.value
    const { data } = await http.get('/absensi/rekap', { params })
    rekap.value = data.data?.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat rekap', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingRekap.value = false
  }
}

watch([periodDate, selectedPegawai], () => loadRekap())

onMounted(async () => {
  await Promise.all([loadPengaturan(), loadPegawaiOptions()])
  await loadRekap()
})

function jumlahHadir(item) {
  return item.absensi.filter((a) => a.jam_masuk).length
}
function jumlahTerlambat(item) {
  return item.absensi.filter((a) => a.terlambat_menit > 0).length
}

async function exportExcel() {
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    if (selectedPegawai.value) params.id_pegawai = selectedPegawai.value
    const res = await http.get('/absensi/rekap/export', { params, responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = url
    const bulanStr = String(params.bulan).padStart(2, '0')
    link.download = `rekap_absensi_${params.tahun}-${bulanStr}.xlsx`
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

// ============================================================
// detail per pegawai (dialog)
// ============================================================

const detailDialog = ref(false)
const detailItem = ref(null)

function openDetail(item) {
  detailItem.value = item
  detailDialog.value = true
}
function dateKey(iso) {
  return (iso || '').slice(0, 10)
}
function formatTanggal(key) {
  const [y, m, d] = key.split('-')
  return `${d}-${m}-${y}`
}
function formatJam(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' })
}
function formatKoordinat(row, jenis) {
  const lat = jenis === 'masuk' ? row.lat_masuk : row.lat_pulang
  const lng = jenis === 'masuk' ? row.lng_masuk : row.lng_pulang
  if (lat == null || lng == null) return '-'
  return `${Number(lat).toFixed(5)}, ${Number(lng).toFixed(5)}`
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Rekap Absen</div>
    <p class="page-subtitle">Rekap kehadiran seluruh pegawai, pengaturan jendela waktu absen, dan aktif/nonaktifkan menu Absen.</p>

    <div class="card" style="max-width: 40rem; margin-bottom: 1.5rem">
      <div v-if="loadingPengaturan" style="display: flex; justify-content: center; padding: 1.5rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <div v-else style="display: flex; flex-direction: column; gap: 1rem">
        <div style="display: flex; align-items: center; gap: 0.75rem">
          <ToggleSwitch v-model="pengaturan.aktif" />
          <span>Menu Absen {{ pengaturan.aktif ? 'aktif' : 'nonaktif' }} untuk pegawai</span>
        </div>
        <Message v-if="!pengaturan.aktif" severity="warn" :closable="false">
          Pegawai tidak akan bisa absen selama menu ini nonaktif.
        </Message>
        <div class="jam-grid">
          <div>
            <label class="field-label">Jam Mulai Absen Pagi</label>
            <InputText v-model="pengaturan.jam_mulai_pagi" placeholder="06:00" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Jam Batas Absen Pagi (setelah ini terlambat)</label>
            <InputText v-model="pengaturan.jam_batas_pagi" placeholder="07:30" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Jam Mulai Absen Pulang</label>
            <InputText v-model="pengaturan.jam_mulai_pulang" placeholder="15:00" style="width: 100%" />
          </div>
        </div>
        <div>
          <Button label="Simpan Pengaturan" icon="pi pi-save" :loading="savingPengaturan" @click="savePengaturan" />
        </div>
      </div>
    </div>

    <div class="card">
      <div class="rekap-toolbar">
        <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        <Select v-model="selectedPegawai" :options="pegawaiOptions" optionLabel="label" optionValue="value" filter showClear placeholder="Semua pegawai" style="min-width: 220px" />
        <Button label="Export Excel" icon="pi pi-file-excel" severity="success" outlined @click="exportExcel" />
      </div>

      <DataTable :value="rekap" :loading="loadingRekap" size="small" stripedRows responsiveLayout="scroll">
        <Column header="Nama">
          <template #body="{ data }">{{ data.pegawai?.nama }}</template>
        </Column>
        <Column header="NIP">
          <template #body="{ data }">{{ data.pegawai?.nip }}</template>
        </Column>
        <Column header="Jumlah Hadir">
          <template #body="{ data }">{{ jumlahHadir(data) }}</template>
        </Column>
        <Column header="Jumlah Terlambat">
          <template #body="{ data }">
            <Tag v-if="jumlahTerlambat(data) > 0" severity="danger" :value="jumlahTerlambat(data)" />
            <span v-else>0</span>
          </template>
        </Column>
        <Column header="Tanggal Terlewat">
          <template #body="{ data }">
            <Tag v-if="data.tanggal_terlewat?.length" severity="warn" :value="data.tanggal_terlewat.length" />
            <span v-else>0</span>
          </template>
        </Column>
        <Column header="Aksi">
          <template #body="{ data }">
            <Button icon="pi pi-eye" size="small" text rounded title="Lihat Detail" @click="openDetail(data)" />
          </template>
        </Column>
        <template #empty>Tidak ada data pegawai.</template>
      </DataTable>
    </div>

    <Dialog v-model:visible="detailDialog" modal :header="`Detail Absen -- ${detailItem?.pegawai?.nama || ''}`" :style="{ width: '640px' }">
      <template v-if="detailItem">
        <h4>Riwayat Absen</h4>
        <DataTable :value="detailItem.absensi" size="small" stripedRows responsiveLayout="scroll">
          <Column header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
          </Column>
          <Column header="Jam Masuk">
            <template #body="{ data }">{{ formatJam(data.jam_masuk) }}</template>
          </Column>
          <Column header="Terlambat">
            <template #body="{ data }">{{ data.terlambat_menit > 0 ? data.terlambat_menit + ' menit' : '-' }}</template>
          </Column>
          <Column header="Jam Pulang">
            <template #body="{ data }">{{ formatJam(data.jam_pulang) }}</template>
          </Column>
          <Column header="Koordinat">
            <template #body="{ data }">{{ formatKoordinat(data, data.jam_pulang ? 'pulang' : 'masuk') }}</template>
          </Column>
          <template #empty>Belum ada absen pada bulan ini.</template>
        </DataTable>

        <template v-if="detailItem.tanggal_terlewat?.length">
          <h4 style="margin-top: 1.25rem">Tanggal Terlewat</h4>
          <ul>
            <li v-for="tgl in detailItem.tanggal_terlewat" :key="tgl">{{ formatTanggal(tgl) }}</li>
          </ul>
        </template>
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
.jam-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 1rem;
}
.rekap-toolbar {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}
</style>
