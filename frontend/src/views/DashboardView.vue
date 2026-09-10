<script setup>
import { ref, onMounted } from 'vue'
import { useAuthStore } from '../stores/auth'
import http from '../api/http'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'

const auth = useAuthStore()
const data = ref(null)
const loading = ref(true)

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

onMounted(async () => {
  try {
    const { data: res } = await http.get('/dashboard')
    data.value = res.data
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="page-wrap">
    <div class="dashboard-header">
      <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="dashboard-logo" />
      <div>
        <div class="page-title">Selamat datang, {{ auth.user?.nama }}</div>
        <p class="page-subtitle">Ringkasan data cuti pegawai &mdash; Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara</p>
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
        <div class="stat-grid">
          <div class="stat-card"><div class="stat-value">{{ data.jatah_tahun_ini }}</div><div class="stat-label">Jatah Cuti Tahun Ini</div></div>
          <div class="stat-card" style="border-color: #f59e0b"><div class="stat-value">{{ data.terpakai }}</div><div class="stat-label">Terpakai</div></div>
          <div class="stat-card" style="border-color: #22c55e"><div class="stat-value">{{ data.sisa }}</div><div class="stat-label">Sisa Cuti</div></div>
          <div class="stat-card" style="border-color: #6366f1"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Menunggu Persetujuan</div></div>
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
</style>
