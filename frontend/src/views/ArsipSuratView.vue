<script setup>
import { ref, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { useAuthStore } from '../stores/auth'
import http from '../api/http'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'

// ArsipSuratView -- menu "Arsip Surat" (pegawai/atasan): daftar surat
// rekomendasi perpanjangan kontrak, bisa dilihat/diunduh sebagai PDF kapan
// saja. Backend GET /api/surat-rekomendasi mengembalikan isi yang BEDA
// menurut role (lihat listSuratRekomendasi di handlers/surat_rekomendasi.go):
//   - pegawai: surat MILIKNYA SENDIRI yang dikirim administrator/admin --
//     TANPA proses approval apa pun (sama seperti sebelumnya).
//   - atasan (mis. Kepala Sekolah/Kepala Puskesmas): surat BAWAHAN
//     LANGSUNGNYA -- KHUSUS surat "Lampiran 3" (pegawai bertempat tugas
//     Sekolah/Puskesmas, item.is_sekolah true) yang masih berstatus
//     "pending", atasan WAJIB klik "Setujui" dulu (lihat doApprove) sebelum
//     QR tanda tangannya muncul di surat bawahannya itu -- sebelum disetujui
//     suratnya TETAP bisa dilihat/diunduh, hanya tanda tangannya kosong.
const toast = useToast()
const confirm = useConfirm()
const authStore = useAuthStore()
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

// ------------------------------------------------------------
// Lampiran 1 (surat permohonan perpanjangan kontrak YANG DIAJUKAN PEGAWAI
// SENDIRI ke Bupati) -- berkas KEDUA yang dibuat dari baris surat
// rekomendasi yang SAMA, lihat pdfPermohonanSuratRekomendasi di
// handlers/surat_rekomendasi.go. Tidak perlu nomor surat/tanda tangan QR
// karena pegawai menandatanganinya sendiri secara fisik.
// ------------------------------------------------------------

async function lihatLampiran1(item) {
  previewTitle.value = `${item.judul} -- Lampiran 1 (Permohonan)`
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf-permohonan`, { params: { inline: 1 }, responseType: 'blob' })
    previewObjectUrl = URL.createObjectURL(res.data)
    previewPdfUrl.value = previewObjectUrl
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal membuka surat', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

async function unduhLampiran1(item) {
  try {
    const res = await http.get(`/surat-rekomendasi/${item.id}/pdf-permohonan`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `permohonan_perpanjangan_kontrak_${item.nip_pegawai || item.id}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

// ------------------------------------------------------------
// Persetujuan atasan (KHUSUS surat "Lampiran 3" pegawai Sekolah/Puskesmas,
// item.is_sekolah true, status_approval masih "pending") -- lihat
// approveSuratRekomendasi di handlers/surat_rekomendasi.go.
// ------------------------------------------------------------

const approving = ref(null)

function confirmApprove(item) {
  confirm.require({
    message: `Setujui surat rekomendasi "${item.nomor_surat}" untuk ${item.nama_pegawai}? Tanda tangan QR otomatis akan langsung muncul di surat ini setelah disetujui.`,
    header: 'Konfirmasi Persetujuan',
    icon: 'pi pi-question-circle',
    acceptLabel: 'Ya, Setujui',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'success' },
    accept: () => doApprove(item),
  })
}

async function doApprove(item) {
  approving.value = item.id
  try {
    await http.put(`/surat-rekomendasi/${item.id}/approve`)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Surat rekomendasi disetujui, tanda tangan QR otomatis sudah muncul', life: 4000 })
    await loadItems()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyetujui', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    approving.value = null
  }
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Arsip Surat</div>
    <p class="page-subtitle" v-if="authStore.isAtasan">
      Surat rekomendasi bawahan langsung anda. Khusus surat pegawai Sekolah/Puskesmas yang masih "Menunggu
      Persetujuan", klik "Setujui" supaya tanda tangan QR otomatis anda muncul di surat itu -- sebelum disetujui,
      surat tetap bisa dilihat/diunduh pegawai, hanya tanda tangannya masih kosong.
    </p>
    <p class="page-subtitle" v-else>
      Surat rekomendasi yang dikirim administrator untuk anda, beserta Lampiran 1 (surat permohonan perpanjangan
      kontrak yang anda ajukan sendiri ke Bupati) -- dua berkas sekaligus untuk setiap pengiriman. Klik "Lihat" untuk
      membuka langsung, atau ikon unduh untuk menyimpan sebagai PDF.
    </p>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <Message v-else-if="!items.length" severity="info" :closable="false">
      {{ authStore.isAtasan ? 'Belum ada surat rekomendasi bawahan anda.' : 'Belum ada surat rekomendasi yang dikirim untuk anda.' }}
    </Message>

    <div v-else class="arsip-grid">
      <div v-for="item in items" :key="item.id" class="arsip-card">
        <div class="arsip-card-icon"><i class="pi pi-file-pdf"></i></div>
        <div class="arsip-card-body">
          <div class="arsip-card-title" :title="item.judul">{{ item.judul }}</div>
          <div v-if="authStore.isAtasan" class="arsip-card-pegawai">{{ item.nama_pegawai }}</div>
          <div class="arsip-card-nomor">{{ item.nomor_surat }}</div>
          <div class="arsip-card-meta">{{ formatTanggal(item.tanggal_surat) }}</div>
          <Tag
            v-if="item.is_sekolah"
            :value="item.status_approval === 'pending' ? (authStore.isAtasan ? 'Menunggu Persetujuan' : 'Menunggu Persetujuan Kepala Sekolah') : 'Disetujui'"
            :severity="item.status_approval === 'pending' ? 'warn' : 'success'"
            class="arsip-card-status"
          />
        </div>
        <div class="arsip-card-actions">
          <Button label="Lihat" icon="pi pi-eye" size="small" @click="lihatSurat(item)" />
          <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="unduhSurat(item)" title="Unduh Surat Rekomendasi" />
        </div>
        <div v-if="authStore.isAtasan && item.is_sekolah && item.status_approval === 'pending'" class="arsip-card-approve">
          <Button
            label="Setujui"
            icon="pi pi-check"
            size="small"
            severity="success"
            :loading="approving === item.id"
            @click="confirmApprove(item)"
          />
        </div>
        <div v-if="!authStore.isAtasan" class="arsip-card-lampiran">
          <span class="arsip-card-lampiran-label">Lampiran 1 (Permohonan):</span>
          <Button icon="pi pi-eye" size="small" severity="secondary" text @click="lihatLampiran1(item)" title="Lihat Lampiran 1" />
          <Button icon="pi pi-download" size="small" severity="secondary" text @click="unduhLampiran1(item)" title="Unduh Lampiran 1" />
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

.arsip-card-pegawai {
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--p-text-color, #1e293b);
}

.arsip-card-status {
  align-self: flex-start;
  margin-top: 0.15rem;
}

.arsip-card-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: auto;
}

.arsip-card-approve {
  padding-top: 0.4rem;
  border-top: 1px dashed var(--p-content-border-color, #e2e8f0);
}

.arsip-card-approve :deep(.p-button) {
  width: 100%;
}

.arsip-card-lampiran {
  display: flex;
  align-items: center;
  gap: 0.15rem;
  padding-top: 0.4rem;
  border-top: 1px dashed var(--p-content-border-color, #e2e8f0);
}

.arsip-card-lampiran-label {
  font-size: 0.72rem;
  color: var(--p-text-muted-color, #64748b);
  margin-right: auto;
}

.preview-frame {
  width: 100%;
  height: 75vh;
  border: none;
}
</style>
