<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
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
const router = useRouter()
const data = ref(null)
const loading = ref(true)

// kartu "Kelengkapan Data Pegawai" (lihat kelengkapan_data pada respons
// dashboard) -- rinciannya cuma tampil kalau kartu ini diklik, lihat
// kelengkapanExpanded di template.
const kelengkapanExpanded = ref(false)

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

// backend (handlers/pegawai.go, unduhDokumenPegawai) selalu mengirim
// Content-Type: application/octet-stream untuk endpoint download dokumen --
// itu cocok untuk mengunduh (nama file & isinya tetap benar), tapi kalau
// blob-nya dibuka langsung di tab baru tanpa MIME type yang benar, browser
// menampilkannya sebagai teks/kode mentah bukan me-render-nya sebagai
// PDF/gambar. Jadi MIME type ditebak sendiri di sini dari ekstensi nama
// filenya supaya "klik untuk melihat" benar-benar menampilkan dokumennya.
function mimeTypeDariNamaFile(nama) {
  const ext = (nama || '').split('.').pop()?.toLowerCase()
  if (ext === 'pdf') return 'application/pdf'
  if (ext === 'jpg' || ext === 'jpeg') return 'image/jpeg'
  if (ext === 'png') return 'image/png'
  return 'application/octet-stream'
}

// melihat (preview) SK Terakhir yang SAAT INI sudah terupload -- dipakai
// notifikasi "Permintaan SK" ketika pegawai ini sudah punya SK Terakhir
// (administrator minta diperiksa ulang/diganti kalau kurang sesuai). Dibuka
// di tab baru (bukan didownload otomatis) supaya benar-benar "klik untuk
// melihat" dulu sebelum memutuskan perlu upload ulang atau tidak.
const melihatSk = ref(false)
async function lihatSkSaatIni() {
  const idPegawai = data.value?.permintaan_sk?.id_pegawai
  const namaFile = data.value?.permintaan_sk?.pegawai?.sk_terakhir_nama
  if (!idPegawai) return
  melihatSk.value = true
  try {
    const res = await http.get(`/pegawai/${idPegawai}/dokumen/sk-terakhir`, { responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data], { type: mimeTypeDariNamaFile(namaFile) }))
    window.open(url, '_blank')
    setTimeout(() => window.URL.revokeObjectURL(url), 60000)
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal membuka', detail: err.response?.data?.message || err.message, life: 4000 })
  } finally {
    melihatSk.value = false
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

// ------------------------------------------------------------
// Dashboard administrator/admin -- ringkasan SEMUA menu penting
// (permintaan pengguna: "buat seinformatif mungkin"). Setiap kartu domain
// di bawah (Data Pegawai, Pengajuan Cuti, Rekap Absensi Hari Ini, Pengajuan
// Pensiun, Peta Jabatan, KP4, Menu Lainnya) punya judul yang bisa diklik
// untuk langsung membuka menu terkait -- lihat ringkasanLainnyaItems untuk
// daftar menu "lainnya" yang belum punya kartu statistik sendiri.
// ------------------------------------------------------------
function pjSelisihSeverity(selisih) {
  if (selisih > 0) return 'success'
  if (selisih < 0) return 'danger'
  return 'info'
}
function pjSelisihText(selisih) {
  if (selisih > 0) return `+${selisih} (kelebihan)`
  if (selisih < 0) return `${selisih} (kekurangan)`
  return '0 (pas)'
}

// cincin persentase generik (dipakai kartu KP4 di bawah) -- pola warna SAMA
// dengan kelengkapanRingClass di atas (hijau/kuning/merah).
function ringClass(persen) {
  if (persen >= 100) return 'kelengkapan-ring-hijau'
  if (persen >= 60) return 'kelengkapan-ring-kuning'
  return 'kelengkapan-ring-merah'
}

// ringkasanLainnyaItems: daftar baris kartu "Menu Lainnya" -- gabungan
// beberapa menu yang masing-masing SUDAH punya alur persetujuan sendiri
// tapi belum punya kartu statistik besar sendiri di dashboard (data
// mentahnya dari ringkasan_lainnya, lihat handlers/dashboard.go).
const ringkasanLainnyaItems = computed(() => {
  const l = data.value?.ringkasan_lainnya
  if (!l) return []
  return [
    { label: 'Kenaikan Pangkat Menunggu Admin', count: l.kenaikan_pangkat_pending, icon: 'pi-arrow-up', aksi: '/peta-jabatan' },
    { label: 'Perubahan Data Menunggu Persetujuan', count: l.perubahan_data_pending, icon: 'pi-user-edit', aksi: '/perubahan-data' },
    { label: 'Berita Acara Menunggu Admin', count: l.berita_acara_pending, icon: 'pi-file-edit', aksi: '/berita-acara' },
    { label: 'Surat Kolektif Menunggu Verifikasi', count: l.surat_kolektif_pending, icon: 'pi-file-check', aksi: '/rekap-absen' },
    { label: 'Permintaan SK TPP Menunggu Pegawai', count: l.permintaan_sk_pending, icon: 'pi-upload', aksi: '/penerima-tpp' },
  ]
})

// warna cincin persentase pada kartu "Kelengkapan Data Pegawai" -- hijau
// kalau sudah lengkap/mendekati, kuning di tengah, merah kalau masih jauh.
const kelengkapanRingClass = computed(() => {
  const p = data.value?.kelengkapan_data?.persentase ?? 0
  if (p >= 100) return 'kelengkapan-ring-hijau'
  if (p >= 60) return 'kelengkapan-ring-kuning'
  return 'kelengkapan-ring-merah'
})

onMounted(() => {
  muatDashboard()
})
</script>

<template>
  <div class="page-wrap">
    <div class="dashboard-banner">
      <div class="dashboard-header">
        <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="dashboard-logo" />
        <div>
          <div class="page-title">Selamat datang, {{ auth.user?.nama }}</div>
          <p class="page-subtitle" style="margin-bottom: 0">Ringkasan manajemen administrasi dinas utama &mdash; Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara</p>
        </div>
      </div>
    </div>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <template v-else-if="data">
      <!-- administrator / admin -->
      <template v-if="['administrator', 'admin'].includes(data.role)">
        <div class="dashboard-admin-grid">
          <!-- Data Pegawai -->
          <div class="card domain-card">
            <div class="domain-card-title" @click="router.push('/master/pegawai')">
              <i class="pi pi-users"></i><span>Data Pegawai</span><i class="pi pi-arrow-up-right domain-card-link-icon"></i>
            </div>
            <div class="stat-grid stat-grid-compact">
              <div class="stat-card tile-teal">
                <div class="stat-icon-badge"><i class="pi pi-users"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pegawai?.total ?? data.total_pegawai }}</div><div class="stat-label">Total Pegawai</div></div>
              </div>
              <div class="stat-card tile-green">
                <div class="stat-icon-badge"><i class="pi pi-check-circle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pegawai?.aktif }}</div><div class="stat-label">Aktif</div></div>
              </div>
              <div class="stat-card tile-sky">
                <div class="stat-icon-badge"><i class="pi pi-building-columns"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pegawai?.dinas }}</div><div class="stat-label">Dinas/Kantor</div></div>
              </div>
              <div class="stat-card tile-purple">
                <div class="stat-icon-badge"><i class="pi pi-graduation-cap"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pegawai?.sekolah }}</div><div class="stat-label">Sekolah</div></div>
              </div>
              <div class="stat-card tile-slate">
                <div class="stat-icon-badge"><i class="pi pi-user-minus"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pegawai?.pensiun }}</div><div class="stat-label">Pensiun</div></div>
              </div>
              <div class="stat-card tile-orange">
                <div class="stat-icon-badge"><i class="pi pi-arrow-right-arrow-left"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pegawai?.mutasi }}</div><div class="stat-label">Mutasi</div></div>
              </div>
            </div>
          </div>

          <!-- Pengajuan Cuti -->
          <div class="card domain-card">
            <div class="domain-card-title" @click="router.push('/pengajuan-cuti')">
              <i class="pi pi-calendar"></i><span>Pengajuan Cuti</span><i class="pi pi-arrow-up-right domain-card-link-icon"></i>
            </div>
            <div class="stat-grid stat-grid-compact">
              <div class="stat-card tile-amber">
                <div class="stat-icon-badge"><i class="pi pi-clock"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_cuti?.pending ?? data.total_pending }}</div><div class="stat-label">Menunggu</div></div>
              </div>
              <div class="stat-card tile-green">
                <div class="stat-icon-badge"><i class="pi pi-check-circle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_cuti?.disetujui ?? data.total_disetujui }}</div><div class="stat-label">Disetujui</div></div>
              </div>
              <div class="stat-card tile-red">
                <div class="stat-icon-badge"><i class="pi pi-times-circle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_cuti?.ditolak ?? data.total_ditolak }}</div><div class="stat-label">Ditolak</div></div>
              </div>
              <div class="stat-card tile-sky">
                <div class="stat-icon-badge"><i class="pi pi-calendar-plus"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_cuti?.diajukan_bulan_ini }}</div><div class="stat-label">Diajukan Bulan Ini</div></div>
              </div>
            </div>
          </div>

          <!-- Rekap Absensi Hari Ini -->
          <div class="card domain-card">
            <div class="domain-card-title" @click="router.push('/rekap-absen')">
              <i class="pi pi-id-card"></i><span>Rekap Absensi Hari Ini</span><i class="pi pi-arrow-up-right domain-card-link-icon"></i>
            </div>
            <p v-if="data.rekap_absensi_hari_ini?.wajib_hadir === 0" class="page-subtitle" style="margin-top: -0.4rem">
              Hari ini libur untuk seluruh pegawai (akhir pekan/tanggal merah).
            </p>
            <div class="stat-grid stat-grid-compact">
              <div class="stat-card tile-slate">
                <div class="stat-icon-badge"><i class="pi pi-users"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.wajib_hadir }}</div><div class="stat-label">Wajib Hadir</div></div>
              </div>
              <div class="stat-card tile-green">
                <div class="stat-icon-badge"><i class="pi pi-check"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.hadir }}</div><div class="stat-label">Hadir</div></div>
              </div>
              <div class="stat-card tile-orange">
                <div class="stat-icon-badge"><i class="pi pi-stopwatch"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.terlambat }}</div><div class="stat-label">Terlambat</div></div>
              </div>
              <div class="stat-card tile-amber">
                <div class="stat-icon-badge"><i class="pi pi-heart"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.sakit }}</div><div class="stat-label">Sakit</div></div>
              </div>
              <div class="stat-card tile-sky">
                <div class="stat-icon-badge"><i class="pi pi-info-circle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.izin }}</div><div class="stat-label">Izin</div></div>
              </div>
              <div class="stat-card tile-purple">
                <div class="stat-icon-badge"><i class="pi pi-calendar"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.cuti }}</div><div class="stat-label">Cuti</div></div>
              </div>
              <div class="stat-card tile-red">
                <div class="stat-icon-badge"><i class="pi pi-ban"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.rekap_absensi_hari_ini?.belum_absen }}</div><div class="stat-label">Belum Absen</div></div>
              </div>
            </div>
          </div>

          <!-- Pengajuan Pensiun -->
          <div class="card domain-card">
            <div class="domain-card-title" @click="router.push('/pengajuan-pensiun')">
              <i class="pi pi-user-minus"></i><span>Pengajuan Pensiun</span><i class="pi pi-arrow-up-right domain-card-link-icon"></i>
            </div>
            <div class="stat-grid stat-grid-compact">
              <div class="stat-card tile-amber">
                <div class="stat-icon-badge"><i class="pi pi-clock"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pensiun?.pending }}</div><div class="stat-label">Menunggu</div></div>
              </div>
              <div class="stat-card tile-green">
                <div class="stat-icon-badge"><i class="pi pi-check-circle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pensiun?.disetujui_tahun_ini }}</div><div class="stat-label">Disetujui Tahun Ini</div></div>
              </div>
              <div class="stat-card tile-red">
                <div class="stat-icon-badge"><i class="pi pi-times-circle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pensiun?.ditolak }}</div><div class="stat-label">Ditolak</div></div>
              </div>
              <div class="stat-card tile-rose">
                <div class="stat-icon-badge"><i class="pi pi-exclamation-triangle"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_pensiun?.akan_pensiun_1_tahun }}</div><div class="stat-label">Akan Pensiun &lt; 1 Tahun</div></div>
              </div>
            </div>
          </div>

          <!-- Peta Jabatan -->
          <div class="card domain-card">
            <div class="domain-card-title" @click="router.push('/peta-jabatan')">
              <i class="pi pi-sitemap"></i><span>Peta Jabatan</span><i class="pi pi-arrow-up-right domain-card-link-icon"></i>
            </div>
            <p class="page-subtitle" style="margin-top: -0.4rem">Ringkasan seluruh Sekolah &amp; Dinas/Kantor</p>
            <div class="stat-grid stat-grid-compact">
              <div class="stat-card tile-sky">
                <div class="stat-icon-badge"><i class="pi pi-briefcase"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_peta_jabatan?.total_kebutuhan }}</div><div class="stat-label">Total Kebutuhan</div></div>
              </div>
              <div class="stat-card tile-teal">
                <div class="stat-icon-badge"><i class="pi pi-users"></i></div>
                <div class="stat-body"><div class="stat-value">{{ data.ringkasan_peta_jabatan?.total_terisi }}</div><div class="stat-label">Total Terisi</div></div>
              </div>
              <div class="stat-card" :class="(data.ringkasan_peta_jabatan?.selisih ?? 0) < 0 ? 'tile-red' : 'tile-green'">
                <div class="stat-icon-badge"><i class="pi pi-equals"></i></div>
                <div class="stat-body">
                  <div class="stat-value" style="font-size: 1.2rem">{{ pjSelisihText(data.ringkasan_peta_jabatan?.selisih ?? 0) }}</div>
                  <div class="stat-label">Selisih Kebutuhan vs Terisi</div>
                </div>
              </div>
            </div>
          </div>

          <!-- KP4 -->
          <div class="card domain-card">
            <div class="domain-card-title" @click="router.push('/kp4-admin')">
              <i class="pi pi-id-card"></i><span>KP4</span><i class="pi pi-arrow-up-right domain-card-link-icon"></i>
            </div>
            <div style="display: flex; align-items: center; gap: 1rem; margin-top: 0.5rem">
              <div class="kelengkapan-ring" :class="ringClass(data.ringkasan_kp4?.persentase ?? 0)">{{ data.ringkasan_kp4?.persentase ?? 0 }}%</div>
              <div style="flex: 1; min-width: 0">
                <div><strong>{{ data.ringkasan_kp4?.sudah_isi }}</strong> pegawai sudah mengisi KP4</div>
                <div class="page-subtitle" style="margin: 0.15rem 0 0 0"><strong>{{ data.ringkasan_kp4?.belum_isi }}</strong> pegawai belum mengisi KP4</div>
              </div>
            </div>
          </div>

          <!-- Menu Lainnya -->
          <div class="card domain-card">
            <div class="domain-card-title" style="cursor: default">
              <i class="pi pi-th-large"></i><span>Menu Lainnya Menunggu Tindakan</span>
            </div>
            <div style="display: flex; flex-direction: column; gap: 0.5rem; margin-top: 0.5rem">
              <div v-for="(item, idx) in ringkasanLainnyaItems" :key="idx" class="kelengkapan-kategori-row" style="cursor: pointer" @click="router.push(item.aksi)">
                <div style="display: flex; align-items: center; gap: 0.6rem; min-width: 0">
                  <i :class="`pi ${item.icon}`" style="color: var(--p-text-muted-color)"></i>
                  <strong>{{ item.label }}</strong>
                </div>
                <Tag :value="String(item.count)" :severity="item.count > 0 ? 'warn' : 'secondary'" />
              </div>
            </div>
          </div>
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
          <div class="stat-card tile-teal">
            <div class="stat-icon-badge"><i class="pi pi-sitemap"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_bawahan }}</div><div class="stat-label">Jumlah Bawahan</div></div>
          </div>
          <div class="stat-card tile-amber">
            <div class="stat-icon-badge"><i class="pi pi-clock"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Menunggu Persetujuan</div></div>
          </div>
          <div class="stat-card tile-green">
            <div class="stat-icon-badge"><i class="pi pi-check-circle"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_disetujui }}</div><div class="stat-label">Disetujui</div></div>
          </div>
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
        <Message v-if="data.permintaan_sk && !data.permintaan_sk.pegawai?.sk_terakhir_nama" severity="warn" :closable="false" style="margin-bottom: 1rem">
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

        <!-- pegawai ini SUDAH punya SK Terakhir, tapi administrator minta
        diperiksa ulang (lihat menu Penerima TPP -> Kirim Permintaan SK) --
        tampilkan SK yang saat ini terupload sebagai tautan yang bisa
        diklik untuk dilihat, plus opsi upload ulang kalau ternyata kurang
        sesuai. -->
        <Message v-if="data.permintaan_sk && data.permintaan_sk.pegawai?.sk_terakhir_nama" severity="warn" :closable="false" style="margin-bottom: 1rem">
          <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; justify-content: space-between">
            <div>
              <strong>Periksa Kembali SK Terakhir untuk Penerima TPP</strong>
              <div style="margin-top: 0.25rem">
                Administrator meminta Anda memeriksa kembali dokumen SK Terakhir yang saat ini terupload paling lambat
                tanggal <strong>{{ formatTanggalPanjang(data.permintaan_sk.batas_tanggal) }}</strong>. SK Anda saat ini:
                <a href="#" @click.prevent="lihatSkSaatIni" style="font-weight: 600">{{ data.permintaan_sk.pegawai.sk_terakhir_nama }}</a>
                &mdash; klik untuk melihat. Jika SK tersebut tidak sesuai/sudah tidak berlaku, silakan upload ulang
                dengan SK yang benar (format PDF, JPG, atau PNG).
              </div>
            </div>
            <Button
              label="Upload Ulang SK Terakhir"
              icon="pi pi-upload"
              size="small"
              severity="secondary"
              :loading="uploadingSk"
              @click="pickSkFile"
            />
            <input ref="skFileInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onSkFileChosen" />
          </div>
        </Message>

        <!-- kartu "Kelengkapan Data Pegawai" (lihat kelengkapan_data pada
        respons dashboard, handlers/dashboard.go) -- SATU kartu ringkas
        persentase menggantikan 3 kartu peringatan terpisah (KP4/Data
        Pegawai/Tanda Tangan Digital) yang dulu SELALU tampil sekaligus --
        permintaan pengguna: "agar tampilan dashboard pegawai lebih bersih".
        Rinciannya baru muncul (dropdown) kalau kartu ini diklik. -->
        <div v-if="data.kelengkapan_data" class="card kelengkapan-card" style="margin-bottom: 1rem" @click="kelengkapanExpanded = !kelengkapanExpanded">
          <div style="display: flex; align-items: center; gap: 1rem">
            <div class="kelengkapan-ring" :class="kelengkapanRingClass">{{ data.kelengkapan_data.persentase }}%</div>
            <div style="flex: 1; min-width: 0">
              <strong>Kelengkapan Data Pegawai</strong>
              <div class="page-subtitle" style="margin: 0.15rem 0 0 0">
                {{ data.kelengkapan_data.lengkap ? 'Data Anda sudah lengkap.' : 'Klik untuk melihat data yang belum lengkap.' }}
              </div>
            </div>
            <i :class="kelengkapanExpanded ? 'pi pi-chevron-up' : 'pi pi-chevron-down'" style="color: var(--p-text-muted-color)"></i>
          </div>
          <div v-if="kelengkapanExpanded && data.kelengkapan_data.kategori?.length" style="margin-top: 1rem; display: flex; flex-direction: column; gap: 0.6rem" @click.stop>
            <div v-for="(kat, idx) in data.kelengkapan_data.kategori" :key="idx" class="kelengkapan-kategori-row">
              <div style="min-width: 0">
                <strong>{{ kat.label }}</strong>
                <div v-if="kat.items?.length" style="font-size: 0.85rem; color: var(--p-text-muted-color); margin-top: 0.15rem">
                  {{ kat.items.join(', ') }}
                </div>
              </div>
              <Button :label="kat.aksi_label" size="small" outlined @click="router.push(kat.aksi)" />
            </div>
          </div>
        </div>

        <div class="stat-grid">
          <div class="stat-card tile-teal">
            <div class="stat-icon-badge"><i class="pi pi-calendar"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.jatah_tahun_ini }}</div><div class="stat-label">Jatah Cuti Tahun Ini</div></div>
          </div>
          <div class="stat-card tile-amber">
            <div class="stat-icon-badge"><i class="pi pi-calendar-minus"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.terpakai }}</div><div class="stat-label">Terpakai</div></div>
          </div>
          <div class="stat-card tile-green">
            <div class="stat-icon-badge"><i class="pi pi-calendar-plus"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.sisa }}</div><div class="stat-label">Sisa Cuti</div></div>
          </div>
          <div class="stat-card tile-sky">
            <div class="stat-icon-badge"><i class="pi pi-clock"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Menunggu Persetujuan</div></div>
          </div>
        </div>

        <!-- Statistik Kenaikan Pangkat (lihat ringkasan_kenaikan_pangkat,
        handlers/dashboard.go) -- SELALU tampil untuk pegawai (permintaan
        pengguna: jabatan Guru/fungsional melihat statistik cuti & kenaikan
        pangkat saja, tapi pegawai lain tetap boleh melihatnya juga). -->
        <div v-if="data.ringkasan_kenaikan_pangkat" class="card">
          <h3 style="margin-top: 0">Pengajuan Kenaikan Pangkat</h3>
          <div class="stat-grid">
            <div class="stat-card tile-amber">
              <div class="stat-icon-badge"><i class="pi pi-clock"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.ringkasan_kenaikan_pangkat.menunggu_atasan }}</div><div class="stat-label">Menunggu Atasan</div></div>
            </div>
            <div class="stat-card tile-sky">
              <div class="stat-icon-badge"><i class="pi pi-file-export"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.ringkasan_kenaikan_pangkat.menunggu_upload_sk }}</div><div class="stat-label">Menunggu Cetak &amp; Upload SK</div></div>
            </div>
            <div class="stat-card tile-purple">
              <div class="stat-icon-badge"><i class="pi pi-hourglass"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.ringkasan_kenaikan_pangkat.menunggu_admin }}</div><div class="stat-label">Menunggu Administrator</div></div>
            </div>
            <div class="stat-card tile-green">
              <div class="stat-icon-badge"><i class="pi pi-check-circle"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.ringkasan_kenaikan_pangkat.selesai }}</div><div class="stat-label">Selesai</div></div>
            </div>
            <div class="stat-card tile-red">
              <div class="stat-icon-badge"><i class="pi pi-times-circle"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.ringkasan_kenaikan_pangkat.ditolak }}</div><div class="stat-label">Ditolak</div></div>
            </div>
          </div>
        </div>

        <!-- Statistik Absensi disembunyikan untuk jabatan fungsional (Guru
        Ahli Pertama/Muda/Madya, dst, lihat jenis_jabatan) -- permintaan
        pengguna: dashboard Guru cukup menampilkan statistik Pengajuan Cuti &
        Kenaikan Pangkat, bukan statistik absensi. -->
        <div v-if="data.statistik_absensi && data.jenis_jabatan !== 'fungsional'" class="card">
          <h3 style="margin-top: 0">Statistik Absensi &mdash; {{ namaBulanTahun(data.statistik_absensi) }}</h3>
          <p class="page-subtitle" style="margin-top: -0.5rem">
            Dari {{ data.statistik_absensi.total_hari_kerja }} hari kerja bulan ini (Sabtu-Minggu untuk pegawai dinas, atau hanya Minggu untuk pegawai sekolah, & tanggal merah tidak dihitung)
          </p>
          <div class="stat-grid stat-grid-absensi">
            <div class="stat-card tile-green">
              <div class="stat-icon-badge"><i class="pi pi-check"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.hadir }}</div><div class="stat-label">Hadir</div></div>
            </div>
            <div class="stat-card tile-orange">
              <div class="stat-icon-badge"><i class="pi pi-heart"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.sakit }}</div><div class="stat-label">Sakit</div></div>
            </div>
            <div class="stat-card tile-amber">
              <div class="stat-icon-badge"><i class="pi pi-info-circle"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.izin }}</div><div class="stat-label">Izin</div></div>
            </div>
            <div class="stat-card tile-sky">
              <div class="stat-icon-badge"><i class="pi pi-calendar"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.cuti }}</div><div class="stat-label">Cuti</div></div>
            </div>
            <div class="stat-card tile-purple">
              <div class="stat-icon-badge"><i class="pi pi-heart-fill"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.cuti_melahirkan }}</div><div class="stat-label">Cuti Melahirkan</div></div>
            </div>
            <div class="stat-card tile-slate">
              <div class="stat-icon-badge"><i class="pi pi-exclamation-triangle"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.tidak_absen_pulang }}</div><div class="stat-label">Hadir tapi Tidak Absen Pulang</div></div>
            </div>
            <div class="stat-card tile-red">
              <div class="stat-icon-badge"><i class="pi pi-ban"></i></div>
              <div class="stat-body"><div class="stat-value">{{ data.statistik_absensi.tidak_melakukan_absensi }}</div><div class="stat-label">Tidak Melakukan Absensi</div></div>
            </div>
            <div
              v-for="(jumlah, kode) in data.statistik_absensi.lainnya || {}"
              :key="kode"
              class="stat-card tile-slate"
            >
              <div class="stat-icon-badge"><i class="pi pi-tag"></i></div>
              <div class="stat-body"><div class="stat-value">{{ jumlah }}</div><div class="stat-label">{{ kode }}</div></div>
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
/* ============================================================
   Tampilan dashboard "lebih soft, elegan & menarik" (permintaan pengguna):
   kartu statistik diganti dari aksen garis kiri warna tegas di atas latar
   putih polos, jadi latar pastel lembut senada + lencana ikon bundar +
   angka besar dalam warna aksen yang lebih kalem -- pola "soft stat tile"
   yang umum dipakai dashboard modern, sekaligus tetap mempertahankan kode
   warna semantik yang sama (hijau = disetujui/hadir, kuning = menunggu,
   merah = ditolak/kritis, dst.) supaya artinya tidak berubah, cuma lebih
   enak dilihat. SEMUA perubahan di-scoped ke komponen ini saja (lihat
   atribut data-v- yang disisipkan Vue) -- halaman lain yang juga memakai
   class global .stat-card/.card (lihat style.css) TIDAK ikut terdampak.
   ============================================================ */
.dashboard-banner {
  background: linear-gradient(135deg, #f0fdfa 0%, #ffffff 65%);
  border-radius: 20px;
  padding: 1.25rem 1.5rem;
  margin-bottom: 1.5rem;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.03), 0 10px 24px -14px rgba(15, 23, 42, 0.12);
}

.dashboard-header {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.dashboard-logo {
  height: 56px;
  width: auto;
  flex-shrink: 0;
}

/* Reset kartu .page-title/.page-subtitle global (lihat style.css) KHUSUS
   di dalam .dashboard-header -- .dashboard-banner di sini sudah punya
   kartu gradien soft-nya sendiri, jadi judul & subjudul "Selamat
   datang..." di dalamnya dikembalikan transparan tanpa padding/radius
   sendiri, supaya tidak muncul "kartu di dalam kartu". Scoped (data-v-)
   otomatis lebih spesifik daripada aturan global di style.css tanpa perlu
   !important -- pola yang sama dipakai untuk override .stat-card di atas. */
.dashboard-header .page-title,
.dashboard-header .page-title:has(+ .page-subtitle),
.dashboard-header .page-title + .page-subtitle {
  background: none;
  padding: 0;
  border-radius: 0;
  color: inherit;
  opacity: 1;
}

.dashboard-header .page-title {
  color: #0f172a;
}

@media (max-width: 480px) {
  .dashboard-banner {
    padding: 1rem;
  }
  .dashboard-logo {
    height: 42px;
  }
}

/* Kartu statistik: latar pastel (var --tile-bg dari salah satu modifier
   .tile-* di bawah) menggantikan latar putih + garis kiri tegas bawaan
   .stat-card (style.css) -- override di sini otomatis lebih spesifik
   karena Vue menambahkan atribut data-v- pada selector scoped, tanpa perlu
   !important. */
.stat-card {
  border-left: none;
  background: var(--tile-bg, var(--p-content-background, #fff));
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.03), 0 4px 14px -8px rgba(15, 23, 42, 0.08);
  display: flex;
  align-items: flex-start;
  gap: 0.85rem;
}

.stat-icon-badge {
  width: 2.4rem;
  height: 2.4rem;
  min-width: 2.4rem;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--tile-icon-bg, rgba(13, 148, 136, 0.12));
  color: var(--tile-accent, #0f766e);
  font-size: 1.1rem;
}

.stat-body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.stat-card .stat-value {
  color: var(--tile-accent, inherit);
}

/* palet pastel lembut per makna semantik (dipakai via class tile-* pada
   markup) -- latar (--tile-bg) & lencana ikon (--tile-icon-bg) sengaja
   dibuat jauh lebih muda/lembut daripada warna aksen teks (--tile-accent)
   supaya kontras angka & label tetap enak dibaca, bukan sekadar warna
   terang di atas warna terang. */
.tile-teal {
  --tile-bg: #f0fdfa;
  --tile-accent: #0f766e;
  --tile-icon-bg: rgba(15, 118, 110, 0.12);
}
.tile-amber {
  --tile-bg: #fffbeb;
  --tile-accent: #b45309;
  --tile-icon-bg: rgba(180, 83, 9, 0.12);
}
.tile-green {
  --tile-bg: #ecfdf5;
  --tile-accent: #15803d;
  --tile-icon-bg: rgba(21, 128, 61, 0.12);
}
.tile-red {
  --tile-bg: #fef2f2;
  --tile-accent: #b91c1c;
  --tile-icon-bg: rgba(185, 28, 28, 0.12);
}
.tile-sky {
  --tile-bg: #eff6ff;
  --tile-accent: #0369a1;
  --tile-icon-bg: rgba(3, 105, 161, 0.12);
}
.tile-purple {
  --tile-bg: #faf5ff;
  --tile-accent: #7e22ce;
  --tile-icon-bg: rgba(126, 34, 206, 0.12);
}
.tile-orange {
  --tile-bg: #fff7ed;
  --tile-accent: #c2410c;
  --tile-icon-bg: rgba(194, 65, 12, 0.12);
}
.tile-slate {
  --tile-bg: #f8fafc;
  --tile-accent: #475569;
  --tile-icon-bg: rgba(71, 85, 105, 0.12);
}
.tile-rose {
  --tile-bg: #fff1f2;
  --tile-accent: #be123c;
  --tile-icon-bg: rgba(190, 18, 60, 0.12);
}

/* ============================================================
   Dashboard administrator/admin -- kartu per MENU (Data Pegawai,
   Pengajuan Cuti, Rekap Absensi Hari Ini, Pengajuan Pensiun, Peta Jabatan,
   KP4, Menu Lainnya), permintaan pengguna: "dashboard menampilkan statistik
   dari semua menu penting ... buat seinformatif mungkin". Dua kolom di
   layar lebar, satu kolom di HP (auto-fit) -- setiap kartu "domain-card"
   punya judul yang bisa diklik untuk membuka menu terkait langsung dari
   dashboard, dan isi stat-grid-compact-nya sendiri (ukuran lebih kecil dari
   stat-grid biasa supaya banyak kartu domain tetap muat tanpa membuat
   halaman kepanjangan).
   ============================================================ */
.dashboard-admin-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
  align-items: start;
}

.domain-card-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 700;
  font-size: 1rem;
  margin-bottom: 0.5rem;
  cursor: pointer;
  color: #0f172a;
}

.domain-card-title:hover {
  color: var(--p-primary-color, #0d9488);
}

.domain-card-link-icon {
  font-size: 0.75rem;
  color: var(--p-text-muted-color);
  margin-left: auto;
}

.stat-grid-compact {
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 0.6rem;
  margin-bottom: 0;
}
.stat-grid-compact .stat-card {
  padding: 0.75rem 0.85rem;
  gap: 0.6rem;
}
.stat-grid-compact .stat-icon-badge {
  width: 2rem;
  height: 2rem;
  min-width: 2rem;
  font-size: 0.95rem;
  border-radius: 10px;
}
.stat-grid-compact .stat-value {
  font-size: 1.35rem;
}

.stat-grid-absensi {
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  margin-bottom: 0;
}
.stat-grid-absensi .stat-card {
  padding: 0.85rem 1rem;
}
.stat-grid-absensi .stat-icon-badge {
  width: 2rem;
  height: 2rem;
  min-width: 2rem;
  font-size: 0.95rem;
  border-radius: 10px;
}
.stat-grid-absensi .stat-value {
  font-size: 1.4rem;
}

/* kartu "Kelengkapan Data Pegawai" -- satu kartu klik-untuk-expand yang
   menggantikan 3 kartu peringatan terpisah (lihat komentar di template). */
.kelengkapan-card {
  cursor: pointer;
  user-select: none;
}

.kelengkapan-ring {
  width: 3.4rem;
  height: 3.4rem;
  min-width: 3.4rem;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 0.95rem;
  color: #fff;
}

.kelengkapan-ring-hijau {
  background: #15803d;
}
.kelengkapan-ring-kuning {
  background: #b45309;
}
.kelengkapan-ring-merah {
  background: #b91c1c;
}

.kelengkapan-kategori-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.6rem 0.75rem;
  background: var(--p-content-background, #f8fafc);
  border: 1px solid rgba(15, 23, 42, 0.06);
  border-radius: 10px;
}
</style>
