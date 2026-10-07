<script setup>
import { ref, onMounted } from 'vue'
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
        <div class="stat-grid">
          <div class="stat-card tile-teal">
            <div class="stat-icon-badge"><i class="pi pi-users"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_pegawai }}</div><div class="stat-label">Total Pegawai</div></div>
          </div>
          <div class="stat-card tile-amber">
            <div class="stat-icon-badge"><i class="pi pi-clock"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_pending }}</div><div class="stat-label">Pengajuan Menunggu</div></div>
          </div>
          <div class="stat-card tile-green">
            <div class="stat-icon-badge"><i class="pi pi-check-circle"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_disetujui }}</div><div class="stat-label">Disetujui</div></div>
          </div>
          <div class="stat-card tile-red">
            <div class="stat-icon-badge"><i class="pi pi-times-circle"></i></div>
            <div class="stat-body"><div class="stat-value">{{ data.total_ditolak }}</div><div class="stat-label">Ditolak</div></div>
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

        <!-- notifikasi KP4 (lihat handlers/kp4.go, kp4_kelengkapan pada
        respons dashboard) -- dua pesan berbeda: (1) data KP4-nya sendiri
        belum lengkap -> arahkan ke menu KP4, (2) field dasar Data Pegawai
        yang dipakai KP4 belum lengkap -> arahkan ke Profil Saya. Keduanya
        bisa muncul bersamaan. -->
        <Message v-if="data.kp4_kelengkapan?.kp4_belum_lengkap" severity="warn" :closable="false" style="margin-bottom: 1rem">
          <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; justify-content: space-between">
            <div>
              <strong>KP4 Belum Terisi</strong>
              <div style="margin-top: 0.25rem">
                Data KP4 Anda belum lengkap ({{ data.kp4_kelengkapan.kp4_field_kosong?.join(', ') }}). Silakan
                dilengkapi.
              </div>
            </div>
            <Button label="Lengkapi KP4" icon="pi pi-id-card" size="small" @click="router.push('/kp4')" />
          </div>
        </Message>
        <Message v-if="data.kp4_kelengkapan?.data_pegawai_kosong?.length" severity="warn" :closable="false" style="margin-bottom: 1rem">
          <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; justify-content: space-between">
            <div>
              <strong>Data Pegawai Belum Lengkap</strong>
              <div style="margin-top: 0.25rem">
                Data berikut masih kosong: {{ data.kp4_kelengkapan.data_pegawai_kosong.join(', ') }}. Silakan
                dilengkapi di menu Profil Saya (Ajukan Perubahan Data).
              </div>
            </div>
            <Button label="Ajukan Perubahan Data" icon="pi pi-user-edit" size="small" @click="router.push('/profil-saya')" />
          </div>
        </Message>

        <!-- notifikasi tanda tangan digital (lihat ttd_kosong pada respons
        dashboard, handlers/dashboard.go) -- mengarahkan ke Profil Saya
        supaya Formulir Cuti berikutnya bisa langsung tertempel TTD tanpa
        perlu cetak-tanda tangan basah-scan lagi. -->
        <Message v-if="data.ttd_kosong" severity="warn" :closable="false" style="margin-bottom: 1rem">
          <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; justify-content: space-between">
            <div>
              <strong>Tanda Tangan Digital Belum Dibuat</strong>
              <div style="margin-top: 0.25rem">
                Isi tanda tangan digital anda terlebih dahulu agar otomatis tertempel di atas nama anda pada Formulir Cuti, tanpa perlu mencetak dan tanda tangan basah lebih dulu.
              </div>
            </div>
            <Button label="Buat Tanda Tangan" icon="pi pi-pencil" size="small" @click="router.push('/profil-saya')" />
          </div>
        </Message>

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

        <div v-if="data.statistik_absensi" class="card">
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
</style>
