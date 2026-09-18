<script setup>
import { ref, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import http from '../api/http'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import ProgressSpinner from 'primevue/progressspinner'

const auth = useAuthStore()
const toast = useToast()
const data = ref(null)
const loading = ref(true)

// ------------------------------------------------------------
// notifikasi "Permintaan SK" (menu Penerima TPP, lihat
// handlers/tpp.go) -- muncul di dashboard pegawai selama
// data.permintaan_sk masih terisi (status "menunggu").
// ------------------------------------------------------------
const skFileInputRef = ref(null)
const uploadingSk = ref(false)

function pickSkFile() {
  skFileInputRef.value?.click()
}

async function onSkFileChosen(e) {
  const file = e.target.files[0]
  e.target.value = ''
  if (!file) return
  uploadingSk.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    await http.post('/tpp/upload-sk-saya', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'SK Terakhir berhasil diupload', life: 3000 })
    await muatDashboard()
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal mengupload', detail: err.response?.data?.message || err.message, life: 5000 })
  } finally {
    uploadingSk.value = false
  }
}

function formatTanggalPanjang(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'long', day: 'numeric' })
}

async function muatDashboard() {
  try {
    const { data: res } = await http.get('/dashboard')
    data.value = res.data
  } finally {
    loading.value = false
  }
}

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

const NAMA_BULAN = [
  '', 'Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni',
  'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember',
]
function namaBulanTahun(statistik) {
  if (!statistik) return ''
  return `${NAMA_BULAN[statistik.bulan] || ''} ${statistik.tahun}`
}

onMounted(() => {
  muatDashboard()
})
</script>

<template>
  <div class="page-wrap">
    <div class="dashboard-header">
      <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="dashboard-logo" />
      <div>
        <div class="page-title">Selamat datang, {{ auth.user?.nama }}</div>
        <p class="page-subtitle">Ringkasan manajemen administrasi dinas utama &mdash; Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara</p>
      </div>
    </div>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <template v-else-if="data">
      <!-- administrator / admin -->
      <template v-if="['administrator', 'admin'].includes(data.role)">
        <div class="stat-grid">
          <div class="stat-card"><div class="stat-value">{{ data.total_pegawai }}</div><div class="stat-label">Total Pegawai</div></div>
          <div class="stat-card" style="border-color: #f59e0b"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Pengajuan Menunggu</div></div>
          <div class="stat-card" style="border-color: #22c55e"><div class="stat-value">{{ data.total_disetujui }}</div><div class="stat-label">Disetujui</div></div>
          <div class="stat-card" style="border-color: #ef4444"><div class="stat-value">{{ data.total_ditolak }}</div><div class="stat-label">Ditolak</div></div>
        </div>
        <div class="card">
          <h3 style="margin-top: 0">Pengajuan Cuti Terbaru</h3>
          <div class="responsive-table-wrap">
            <DataTable :value="data.pengajuan_terbaru" size="small" style="min-width: 600px">
              <Column field="pegawai.nama" header="Pegawai" />
              <Column field="jenis_cuti.jenis" header="Jenis Cuti" />
              <Column header="Tanggal">
                <template #body="{ data: row }">{{ formatDate(row.tgl_mulai) }} - {{ formatDate(row.tgl_selesai) }}</template>
              </Column>
              <Column header="Status">
                <template #body="{ data: row }"><Tag :value="row.status" :severity="statusSeverity(row.status)" /></template>
              </Column>
            </DataTable>
          </div>
        </div>
      </template>

      <!-- atasan -->
      <template v-else-if="data.role === 'atasan'">
        <div class="stat-grid">
          <div class="stat-card"><div class="stat-value">{{ data.total_bawahan }}</div><div class="stat-label">Jumlah Bawahan</div></div>
          <div class="stat-card" style="border-color: #f59e0b"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Menunggu Persetujuan</div></div>
          <div class="stat-card" style="border-color: #22c55e"><div class="stat-value">{{ data.total_disetujui }}</div><div class="stat-label">Disetujui</div></div>
        </div>
        <div class="card">
          <h3 style="margin-top: 0">Menunggu Persetujuan Anda</h3>
          <p class="page-subtitle" style="margin-top: -0.5rem">Proses persetujuan lengkap ada di menu Pengajuan Cuti</p>
          <div class="responsive-table-wrap">
            <DataTable :value="data.menunggu_persetujuan" size="small" style="min-width: 600px">
              <Column field="pegawai.nama" header="Pegawai" />
              <Column field="jenis_cuti.jenis" header="Jenis Cuti" />
              <Column header="Tanggal">
                <template #body="{ data: row }">{{ formatDate(row.tgl_mulai) }} - {{ formatDate(row.tgl_selesai) }}</template>
              </Column>
              <Column field="jumlah_hari" header="Jumlah Hari" />
            </DataTable>
          </div>
        </div>
      </template>

      <!-- pegawai -->
      <template v-else-if="data.role === 'pegawai'">
        <Message v-if="data.permintaan_sk" severity="warn" :closable="false" style="margin-bottom: 1rem">
          <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; justify-content: space-between">
            <div>
              <strong>Upload SK Terakhir untuk Penerima TPP</strong>
              <div style="margin-top: 0.25rem">
                Anda diminta mengupload dokumen SK Terakhir paling lambat tanggal
                <strong>{{ formatTanggalPanjang(data.permintaan_sk.batas_tanggal) }}</strong> agar data Penerima TPP Anda
                lengkap. Format PDF, JPG, atau PNG.
              </div>
            </div>
            <Button label="Upload SK Terakhir" icon="pi pi-upload" size="small" :loading="uploadingSk" @click="pickSkFile" />
            <input ref="skFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onSkFileChosen" />
          </div>
        </Message>

        <div class="stat-grid">
          <div class="stat-card"><div class="stat-value">{{ data.jatah_tahun_ini }}</div><div class="stat-label">Jatah Cuti Tahun Ini</div></div>
          <div class="stat-card" style="border-color: #f59e0b"><div class="stat-value">{{ data.terpakai }}</div><div class="stat-label">Terpakai</div></div>
          <div class="stat-card" style="border-color: #22c55e"><div class="stat-value">{{ data.sisa }}</div><div class="stat-label">Sisa Cuti</div></div>
          <div class="stat-card" style="border-color: #0d9488"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Menunggu Persetujuan</div></div>
        </div>

        <div v-if="data.statistik_absensi" class="card">
          <h3 style="margin-top: 0">Statistik Absensi &mdash; {{ namaBulanTahun(data.statistik_absensi) }}</h3>
          <p class="page-subtitle" style="margin-top: -0.5rem">
            Dari {{ data.statistik_absensi.total_hari_kerja }} hari kerja bulan ini (Sabtu-Minggu untuk pegawai dinas, atau hanya Minggu untuk pegawai sekolah, & tanggal merah tidak dihitung)
          </p>
          <div class="stat-grid stat-grid-absensi">
            <div class="stat-card" style="border-color: #22c55e"><div class="stat-value">{{ data.statistik_absensi.hadir }}</div><div class="stat-label">Hadir</div></div>
            <div class="stat-card" style="border-color: #f97316"><div class="stat-value">{{ data.statistik_absensi.sakit }}</div><div class="stat-label">Sakit</div></div>
            <div class="stat-card" style="border-color: #f59e0b"><div class="stat-value">{{ data.statistik_absensi.izin }}</div><div class="stat-label">Izin</div></div>
            <div class="stat-card" style="border-color: #0ea5e9"><div class="stat-value">{{ data.statistik_absensi.cuti }}</div><div class="stat-label">Cuti</div></div>
            <div class="stat-card" style="border-color: #a855f7"><div class="stat-value">{{ data.statistik_absensi.cuti_melahirkan }}</div><div class="stat-label">Cuti Melahirkan</div></div>
            <div class="stat-card" style="border-color: #d97706"><div class="stat-value">{{ data.statistik_absensi.tidak_absen_pulang }}</div><div class="stat-label">Hadir tapi Tidak Absen Pulang</div></div>
            <div class="stat-card" style="border-color: #ef4444"><div class="stat-value">{{ data.statistik_absensi.tidak_melakukan_absensi }}</div><div class="stat-label">Tidak Melakukan Absensi</div></div>
            <div
              v-for="(jumlah, kode) in data.statistik_absensi.lainnya || {}"
              :key="kode"
              class="stat-card"
              style="border-color: #64748b"
            >
              <div class="stat-value">{{ jumlah }}</div>
              <div class="stat-label">{{ kode }}</div>
            </div>
          </div>
        </div>

        <div class="card">
          <h3 style="margin-top: 0">Riwayat Pengajuan Cuti Saya</h3>
          <div class="responsive-table-wrap">
            <DataTable :value="data.riwayat_cuti" size="small" style="min-width: 600px">
              <Column field="jenis_cuti.jenis" header="Jenis Cuti" />
              <Column header="Tanggal">
                <template #body="{ data: row }">{{ formatDate(row.tgl_mulai) }} - {{ formatDate(row.tgl_selesai) }}</template>
              </Column>
              <Column field="jumlah_hari" header="Jumlah Hari" />
              <Column header="Status">
                <template #body="{ data: row }"><Tag :value="row.status" :severity="statusSeverity(row.status)" /></template>
              </Column>
            </DataTable>
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.dashboard-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 0.5rem;
}

.dashboard-logo {
  height: 56px;
  width: auto;
  flex-shrink: 0;
}

@media (max-width: 480px) {
  .dashboard-logo {
    height: 42px;
  }
}

.stat-grid-absensi {
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  margin-bottom: 0;
}
.stat-grid-absensi .stat-card {
  padding: 0.85rem 1rem;
}
.stat-grid-absensi .stat-value {
  font-size: 1.4rem;
}
</style>
