<script setup>
import { ref, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

// ArsipSuratView -- menu "Arsip Surat" (pegawai/atasan): daftar surat
// rekomendasi perpanjangan kontrak yang DIKIRIM administrator/admin untuk
// akun ini (lihat SuratRekomendasiView.vue di sisi administrator), bisa
// dilihat/diunduh sebagai PDF kapan saja -- TANPA harus mencetak dulu atau
// menunggu proses approval apa pun. Backend GET /api/surat-rekomendasi
// otomatis hanya mengembalikan surat milik pegawai yang login (lihat
// listSuratRekomendasi di handlers/surat_rekomendasi.go).

const toast = useToast()
const items = ref([])
const loading = ref(true)

async function loadItems() {
  loading.value = true
  try {
    const { data } = await http.get('/surat-rekomendasi')
    items.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

onMounted(loadItems)

function formatTanggal(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'long', day: 'numeric' })
}

const previewDialog = ref(false)
const previewTitle = ref('')
const previewPdfUrl = ref('')
let previewObjectUrl = ''

async function lihatSurat(item) {
  previewTitle.value = item.judul
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf`, { params: { inline: 1 }, responseType: 'blob' })
    previewObjectUrl = URL.createObjectURL(res.data)
    previewPdfUrl.value = previewObjectUrl
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal membuka surat', detail: e.response?.data?.message || e.message, life: 5000 })
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

async function unduhSurat(item) {
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `surat_rekomendasi_${item.nip_pegawai || item.id}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Arsip Surat</div>
    <p class="page-subtitle">
      Surat rekomendasi yang dikirim administrator untuk anda. Klik "Lihat" untuk membuka langsung, atau "Unduh"
      untuk menyimpan sebagai PDF.
    </p>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <Message v-else-if="!items.length" severity="info" :closable="false">
      Belum ada surat rekomendasi yang dikirim untuk anda.
    </Message>

    <div v-else class="arsip-grid">
      <div v-for="item in items" :key="item.id" class="arsip-card">
        <div class="arsip-card-icon"><i class="pi pi-file-pdf"></i></div>
        <div class="arsip-card-body">
          <div class="arsip-card-title" :title="item.judul">{{ item.judul }}</div>
          <div class="arsip-card-nomor">{{ item.nomor_surat }}</div>
          <div class="arsip-card-meta">{{ formatTanggal(item.tanggal_surat) }}</div>
        </div>
        <div class="arsip-card-actions">
          <Button label="Lihat" icon="pi pi-eye" size="small" @click="lihatSurat(item)" />
          <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="unduhSurat(item)" title="Unduh" />
        </div>
      </div>
    </div>

    <Dialog v-model:visible="previewDialog" modal :header="previewTitle" style="width: 90vw; max-width: 900px" @hide="tutupPreview">
      <iframe :src="previewPdfUrl" class="preview-frame"></iframe>
    </Dialog>
  </div>
</template>

<style scoped>
.arsip-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
}

.arsip-card {
  background: var(--p-content-background, #fff);
  border-radius: 12px;
  padding: 1.1rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.arsip-card-icon {
  font-size: 2.1rem;
  color: #b91c1c;
}

.arsip-card-title {
  font-weight: 700;
  font-size: 0.98rem;
}

.arsip-card-nomor {
  font-size: 0.8rem;
  font-family: monospace;
  color: #0d9488;
}

.arsip-card-meta {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.78rem;
}

.arsip-card-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: auto;
}

.preview-frame {
  width: 100%;
  height: 75vh;
  border: none;
}
</style>
