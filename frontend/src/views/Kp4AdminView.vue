<script setup>
// Kp4AdminView -- menu "KP4" khusus administrator/admin: rekap kelengkapan
// KP4 SEMUA pegawai (lihat GET /api/kp4, handlers/kp4.go) + klik satu baris
// untuk membuka dialog berisi Kp4FormPanel yang sama dipakai pegawai sendiri
// (lihat Kp4View.vue), supaya administrator/admin bisa mengedit langsung
// data KP4 pegawai manapun (mis. saat pegawai kesulitan mengisi sendiri).
import { ref, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import ProgressSpinner from 'primevue/progressspinner'
import Kp4FormPanel from '../components/Kp4FormPanel.vue'

const toast = useToast()
const rows = ref([])
const loading = ref(true)
const search = ref('')
const filterLengkap = ref(null)
const filterOptions = [
  { id: null, label: 'Semua' },
  { id: 'sudah', label: 'Sudah Lengkap' },
  { id: 'belum', label: 'Belum Lengkap' },
]
const meta = ref({ total: 0, total_sudah_lengkap: 0, total_belum_lengkap: 0 })

async function load() {
  loading.value = true
  try {
    const params = {}
    if (search.value) params.q = search.value
    if (filterLengkap.value) params.lengkap = filterLengkap.value
    const { data } = await http.get('/kp4', { params })
    rows.value = data.data || []
    meta.value = data.meta || meta.value
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

let searchTimer = null
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(load, 350)
})
watch(filterLengkap, load)

const dialogVisible = ref(false)
const selectedPegawai = ref(null)

function bukaDetail(row) {
  selectedPegawai.value = row
  dialogVisible.value = true
}

function onSaved() {
  load()
}

onMounted(load)
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">KP4</div>
    <p class="page-subtitle">
      Rekap kelengkapan Surat Keterangan Untuk Mendapatkan Pembayaran Tunjangan Keluarga seluruh pegawai. Klik salah
      satu baris untuk melihat/mengedit data KP4 pegawai tersebut.
    </p>

    <div class="card">
      <div class="toolbar">
        <IconField>
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Cari nama/NIP..." style="width: 220px" />
        </IconField>
        <Select v-model="filterLengkap" :options="filterOptions" optionLabel="label" optionValue="id" style="width: 200px" />
      </div>

      <div class="stat-row">
        <Tag severity="info" :value="`${meta.total ?? 0} Total Pegawai`" />
        <Tag severity="success" :value="`${meta.total_sudah_lengkap ?? 0} Sudah Lengkap`" />
        <Tag severity="warn" :value="`${meta.total_belum_lengkap ?? 0} Belum Lengkap`" />
      </div>

      <div v-if="loading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <DataTable v-else :value="rows" dataKey="id_pegawai" @row-click="(e) => bukaDetail(e.data)" style="cursor: pointer">
        <Column field="nip" header="NIP" />
        <Column field="nama" header="Nama" />
        <Column field="jabatan" header="Jabatan" />
        <Column field="unit_kerja" header="Unit Kerja" />
        <Column header="Status KP4">
          <template #body="{ data }">
            <Tag :severity="data.kp4_belum_lengkap ? 'warn' : 'success'" :value="data.kp4_belum_lengkap ? 'Belum Lengkap' : 'Sudah Lengkap'" />
          </template>
        </Column>
        <Column header="Data Pegawai">
          <template #body="{ data }">
            <Tag v-if="data.data_pegawai_kosong" severity="danger" value="Ada Data Kosong" />
            <Tag v-else severity="success" value="Lengkap" />
          </template>
        </Column>
      </DataTable>
    </div>

    <Dialog v-model:visible="dialogVisible" modal :style="{ width: '900px' }" :breakpoints="{ '960px': '95vw' }" header="Data KP4 Pegawai">
      <div v-if="selectedPegawai" style="margin-bottom: 1rem; font-weight: 600">
        {{ selectedPegawai.nama }} -- NIP. {{ selectedPegawai.nip }}
      </div>
      <Kp4FormPanel v-if="selectedPegawai" :key="selectedPegawai.id_pegawai" :idPegawai="selectedPegawai.id_pegawai" @saved="onSaved" />
    </Dialog>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.stat-row {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
  margin-bottom: 1.25rem;
}
</style>
