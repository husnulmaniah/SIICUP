<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import { toApiDate } from '../utils/date'
import { useBlinkLiveness } from '../composables/useBlinkLiveness'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import DatePicker from 'primevue/datepicker'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

const toast = useToast()

// ============================================================
// pengaturan (aktif/nonaktif & jendela waktu) + riwayat bulanan
// ============================================================

const pengaturan = ref(null)
const loadingPengaturan = ref(true)
const periodDate = ref(new Date())
const riwayat = ref({ absensi: [], tanggal_terlewat: [], tanggal_tercover: [] })
const loadingRiwayat = ref(false)
const dokumenList = ref([])

// jenis dokumen (diinput admin) -> kode singkat yang tampil di riwayat,
// mengikuti pemetaan yang sama dengan models.AbsensiDokumenKode di backend:
// Surat Tugas & Berita Acara sama-sama dibaca "DD" (Dinas Dalam).
const JENIS_KODE = { sks: 'S', surat_tugas: 'DD', berita_acara: 'DD', surat_izin: 'I' }
const JENIS_KODE_LABEL = { S: 'Sakit', DD: 'Dinas Dalam', I: 'Izin' }
function kodeDokumen(jenis) {
  return JENIS_KODE[jenis] || ''
}
function labelKodeDokumen(jenis) {
  return JENIS_KODE_LABEL[kodeDokumen(jenis)] || ''
}

function dateKey(iso) {
  return (iso || '').slice(0, 10)
}
function todayKey() {
  return toApiDate(new Date())
}

// statusHariIni SELALU berisi baris absen hari ini (diambil dari bulan
// berjalan), terpisah dari `riwayat` yang mengikuti bulan yang sedang
// dilihat pegawai lewat pemilih bulan. Dipisah supaya tombol "Absen Masuk"/
// "Absen Pulang" tetap nonaktif begitu pegawai sudah absen, walaupun ia
// sedang melihat riwayat bulan lain.
const statusHariIni = ref(null)

const todayRow = computed(() => statusHariIni.value)
const sudahMasuk = computed(() => !!todayRow.value?.jam_masuk)
const sudahPulang = computed(() => !!todayRow.value?.jam_pulang)

function parseJam(hhmm) {
  if (!hhmm || !hhmm.includes(':')) return null
  const [h, m] = hhmm.split(':').map(Number)
  return h * 60 + m
}
function minutesNow() {
  const now = new Date()
  return now.getHours() * 60 + now.getMinutes()
}

const canMasuk = computed(() => {
  if (!pengaturan.value?.aktif || sudahMasuk.value) return false
  const mulai = parseJam(pengaturan.value?.jam_mulai_pagi)
  return mulai == null || minutesNow() >= mulai
})
const canPulang = computed(() => {
  if (!pengaturan.value?.aktif || sudahPulang.value) return false
  const mulai = parseJam(pengaturan.value?.jam_mulai_pulang)
  return mulai == null || minutesNow() >= mulai
})

async function loadPengaturan() {
  loadingPengaturan.value = true
  try {
    const { data } = await http.get('/absensi/pengaturan')
    pengaturan.value = data.data
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan absen', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingPengaturan.value = false
  }
}

async function loadRiwayat() {
  loadingRiwayat.value = true
  try {
    const bulan = periodDate.value.getMonth() + 1
    const tahun = periodDate.value.getFullYear()
    const { data } = await http.get('/absensi/saya', { params: { bulan, tahun } })
    riwayat.value = data.data
    loadThumbnails(riwayat.value.absensi)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat riwayat absen', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingRiwayat.value = false
  }
}

// loadStatusHariIni mengambil baris absen HARI INI saja (lewat riwayat bulan
// berjalan) -- dipanggil saat halaman dibuka dan setiap kali absen berhasil,
// supaya tombol absen langsung nonaktif tanpa menunggu pegawai refresh.
async function loadStatusHariIni() {
  try {
    const now = new Date()
    const { data } = await http.get('/absensi/saya', {
      params: { bulan: now.getMonth() + 1, tahun: now.getFullYear() },
    })
    const rows = data.data?.absensi || []
    statusHariIni.value = rows.find((a) => dateKey(a.tanggal) === todayKey()) || null
  } catch {
    // tidak kritikal -- tombol absen tetap bisa dipakai, backend juga
    // menolak absen ganda (lihat absenMasuk/absenPulang di backend).
  }
}

async function loadDokumen() {
  try {
    const { data } = await http.get('/absensi/dokumen')
    dokumenList.value = data.data || []
  } catch {
    // bagian ini tidak kritikal -- kalau gagal cukup dibiarkan kosong
  }
}

// ============================================================
// thumbnail foto absen (ditampilkan langsung di tabel riwayat)
// ============================================================

// Foto disimpan di database dan hanya bisa diambil dengan token, jadi tidak
// bisa dipasang langsung ke <img src>. Foto diambil sebagai blob lalu
// disimpan object URL-nya di sini, dengan kunci "<id absen>-masuk/pulang".
// Backend mengecilkan foto lewat parameter ?w=96 supaya satu tabel berisi
// puluhan foto tetap ringan (lihat resizeJPEG di handlers/absensi.go).
const thumbUrls = ref({})

function revokeThumbnails() {
  Object.values(thumbUrls.value).forEach((url) => URL.revokeObjectURL(url))
  thumbUrls.value = {}
}

async function fetchThumb(id, jenis) {
  const key = `${id}-${jenis}`
  if (thumbUrls.value[key]) return
  try {
    const res = await http.get(`/absensi/foto/${id}/${jenis}`, { params: { w: 96 }, responseType: 'blob' })
    thumbUrls.value = { ...thumbUrls.value, [key]: URL.createObjectURL(res.data) }
  } catch {
    // foto tidak ada/gagal dimuat -- kolom foto cukup menampilkan "-"
  }
}

// loadThumbnails mengambil thumbnail baris demi baris (maksimal 4 permintaan
// berjalan bersamaan) supaya tidak membanjiri koneksi HP saat satu bulan
// penuh berisi foto.
async function loadThumbnails(rows) {
  revokeThumbnails()
  const jobs = []
  for (const row of rows || []) {
    if (row.jam_masuk) jobs.push([row.id, 'masuk'])
    if (row.jam_pulang) jobs.push([row.id, 'pulang'])
  }
  let idx = 0
  const worker = async () => {
    while (idx < jobs.length) {
      const [id, jenis] = jobs[idx++]
      await fetchThumb(id, jenis)
    }
  }
  await Promise.all(Array.from({ length: Math.min(4, jobs.length) }, worker))
}

function thumbUrl(row, jenis) {
  return thumbUrls.value[`${row.id}-${jenis}`] || ''
}

watch(periodDate, () => loadRiwayat())

onMounted(async () => {
  await loadPengaturan()
  await Promise.all([loadRiwayat(), loadDokumen(), loadStatusHariIni()])
})

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

// ============================================================
// titik koordinat -> tautan Google Maps
// ============================================================

// Format "?api=1&query=lat,lng" adalah format resmi Google Maps: sekali klik
// langsung terbuka dengan penanda pada titik tersebut, di browser maupun di
// aplikasi Google Maps pada HP -- jadi koordinat tidak perlu disalin manual.
function mapsUrl(lat, lng) {
  return `https://www.google.com/maps/search/?api=1&query=${Number(lat).toFixed(6)},${Number(lng).toFixed(6)}`
}

function punyaKoordinat(row, jenis) {
  const lat = jenis === 'masuk' ? row.lat_masuk : row.lat_pulang
  const lng = jenis === 'masuk' ? row.lng_masuk : row.lng_pulang
  return lat != null && lng != null
}

function mapsUrlBaris(row, jenis) {
  const lat = jenis === 'masuk' ? row.lat_masuk : row.lat_pulang
  const lng = jenis === 'masuk' ? row.lng_masuk : row.lng_pulang
  return lat == null || lng == null ? '' : mapsUrl(lat, lng)
}

// ============================================================
// kamera + verifikasi kedipan mata + capture otomatis
// ============================================================

const cameraDialog = ref(false)
const cameraMode = ref('masuk')
const videoEl = ref(null)
const canvasEl = ref(null)
const cameraError = ref('')
const capturedUrl = ref('')
const capturedBlob = ref(null)
const submitting = ref(false)
const noBlinkWarning = ref(false)
const coords = ref({ lat: null, lng: null, accuracy: null })
const geoStatus = ref('')
const locationChecking = ref(false)
const locationBlocked = ref(false)
const locationBlockedMsg = ref('')

let mediaStream = null
let noBlinkTimer = null
const blink = useBlinkLiveness()

function labelMode(mode) {
  return mode === 'masuk' ? 'Absen Masuk' : 'Absen Pulang'
}

// haversineMeter menghitung jarak (meter) dua titik koordinat bumi --
// dipakai untuk memvalidasi radius kantor di sisi browser sebelum kamera
// dibuka (validasi yang sesungguhnya tetap dilakukan lagi di backend saat
// submit, lihat absensiCekRadius di handlers/absensi.go).
function haversineMeter(lat1, lng1, lat2, lng2) {
  const R = 6371000
  const toRad = (d) => (d * Math.PI) / 180
  const dLat = toRad(lat2 - lat1)
  const dLng = toRad(lng2 - lng1)
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLng / 2) ** 2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

function getLocationOnce() {
  return new Promise((resolve) => {
    if (!navigator.geolocation) {
      resolve(null)
      return
    }
    navigator.geolocation.getCurrentPosition(
      (pos) => resolve({ lat: pos.coords.latitude, lng: pos.coords.longitude, accuracy: pos.coords.accuracy }),
      () => resolve(null),
      { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 },
    )
  })
}

// cekLokasiKantor mencari titik koordinat pegawai lalu memeriksanya terhadap
// titik koordinat kantor (kalau sudah diatur administrator). Kamera hanya
// dibuka kalau lolos -- kalau di luar radius atau lokasi tidak terdeteksi
// (padahal geofence aktif), kamera TIDAK dibuka dan peringatan ditampilkan
// (lihat locationBlocked di template).
async function cekLokasiKantor() {
  locationChecking.value = true
  geoStatus.value = 'mencari titik koordinat...'
  const pos = await getLocationOnce()
  locationChecking.value = false

  const kantorLat = pengaturan.value?.kantor_lat
  const kantorLng = pengaturan.value?.kantor_lng
  const radius = pengaturan.value?.radius_meter || 20
  const geofenceAktif = kantorLat != null && kantorLng != null

  if (!pos) {
    geoStatus.value = 'lokasi tidak diizinkan/tidak ditemukan'
    if (geofenceAktif) {
      locationBlocked.value = true
      locationBlockedMsg.value =
        'Lokasi GPS tidak terdeteksi. Aktifkan layanan lokasi dan izinkan akses lokasi pada browser ini, lalu coba lagi.'
      return false
    }
    return true
  }

  coords.value = { lat: pos.lat, lng: pos.lng, accuracy: pos.accuracy ?? null }
  geoStatus.value = `titik koordinat ditemukan (akurasi ±${Math.round(pos.accuracy)}m)`

  if (!geofenceAktif) return true

  const jarak = haversineMeter(pos.lat, pos.lng, kantorLat, kantorLng)
  // GPS ponsel (apalagi di dalam gedung) sering meleset 50-150m walaupun
  // pegawai tidak bergerak. Supaya tidak berulang kali ditolak hanya karena
  // noise GPS, jarak dibandingkan setelah dikurangi toleransi akurasi
  // (dibatasi maks. 100m) -- pengecekan akhir & mengikat tetap dilakukan
  // ulang di server (absensiCekRadius di backend) dengan aturan yang sama.
  const toleransi = pos.accuracy > 0 ? Math.min(pos.accuracy, 100) : 0
  const jarakEfektif = Math.max(jarak - toleransi, 0)
  if (jarakEfektif > radius) {
    locationBlocked.value = true
    const infoAkurasi = pos.accuracy > 0 ? ` (akurasi GPS perangkat Anda saat ini sekitar ${Math.round(pos.accuracy)} meter)` : ''
    locationBlockedMsg.value = `Anda berada di luar radius kantor (jarak sekitar ${Math.round(jarak)} meter, maksimal ${radius} meter dari titik kantor)${infoAkurasi}. Absen tidak dapat dilakukan dari lokasi ini.`
    return false
  }
  return true
}

async function startCameraStream() {
  cameraError.value = ''
  try {
    mediaStream = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: 'user', width: { ideal: 480 }, height: { ideal: 480 } },
      audio: false,
    })
  } catch {
    cameraError.value = 'Tidak bisa mengakses kamera. Pastikan izin kamera diberikan pada browser ini.'
    return
  }
  await new Promise((resolve) => setTimeout(resolve, 60))
  if (videoEl.value) {
    videoEl.value.srcObject = mediaStream
    try {
      await videoEl.value.play()
    } catch {
      // beberapa browser menolak play() otomatis sebelum interaksi user --
      // video tetap akan tampil begitu elemen <video> menerima srcObject.
    }
  }
  blink.reset()
  blink.start(videoEl.value)
  if (noBlinkTimer) clearTimeout(noBlinkTimer)
  noBlinkTimer = setTimeout(() => {
    noBlinkWarning.value = true
  }, 12000)
}

async function openCamera(mode) {
  // pengaman tambahan selain tombol yang sudah di-disable: kalau absen hari
  // ini sudah tercatat, kamera tidak usah dibuka sama sekali.
  if (mode === 'masuk' && sudahMasuk.value) {
    toast.add({ severity: 'info', summary: 'Sudah absen', detail: 'Anda sudah absen masuk hari ini', life: 3000 })
    return
  }
  if (mode === 'pulang' && sudahPulang.value) {
    toast.add({ severity: 'info', summary: 'Sudah absen', detail: 'Anda sudah absen pulang hari ini', life: 3000 })
    return
  }

  cameraMode.value = mode
  capturedBlob.value = null
  if (capturedUrl.value) URL.revokeObjectURL(capturedUrl.value)
  capturedUrl.value = ''
  noBlinkWarning.value = false
  coords.value = { lat: null, lng: null, accuracy: null }
  locationBlocked.value = false
  locationBlockedMsg.value = ''
  cameraDialog.value = true

  const lolos = await cekLokasiKantor()
  if (!lolos) return

  try {
    await blink.init()
  } catch {
    // blink.modelsError sudah terisi, ditampilkan lewat Message di template;
    // kamera tetap dibuka supaya pegawai masih bisa ambil foto manual.
  }
  await startCameraStream()
}

// retryLocation dipanggil dari tombol "Coba Lagi" saat lokasi di luar
// radius/tidak terdeteksi -- mengulang pengecekan lokasi tanpa menutup
// dialog kamera.
async function retryLocation() {
  locationBlocked.value = false
  locationBlockedMsg.value = ''
  const lolos = await cekLokasiKantor()
  if (!lolos) return
  try {
    await blink.init()
  } catch {
    // lihat komentar di openCamera
  }
  await startCameraStream()
}

function stopCamera() {
  blink.stop()
  if (noBlinkTimer) {
    clearTimeout(noBlinkTimer)
    noBlinkTimer = null
  }
  if (mediaStream) {
    mediaStream.getTracks().forEach((t) => t.stop())
    mediaStream = null
  }
}

function capturePhoto() {
  if (!videoEl.value || !canvasEl.value || !videoEl.value.videoWidth) return
  const video = videoEl.value
  const canvas = canvasEl.value
  const size = Math.min(video.videoWidth, video.videoHeight) || 480
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')
  const sx = (video.videoWidth - size) / 2
  const sy = (video.videoHeight - size) / 2
  ctx.drawImage(video, sx, sy, size, size, 0, 0, size, size)
  canvas.toBlob(
    (blob) => {
      capturedBlob.value = blob
      capturedUrl.value = URL.createObjectURL(blob)
      stopCamera()
    },
    'image/jpeg',
    0.85,
  )
}

watch(
  () => blink.blinkDetected.value,
  (val) => {
    if (val && !capturedBlob.value) capturePhoto()
  },
)

function ambilFotoManual() {
  capturePhoto()
}

async function retake() {
  capturedBlob.value = null
  if (capturedUrl.value) URL.revokeObjectURL(capturedUrl.value)
  capturedUrl.value = ''
  noBlinkWarning.value = false
  await startCameraStream()
}

function closeCameraDialog() {
  stopCamera()
  cameraDialog.value = false
  if (capturedUrl.value) URL.revokeObjectURL(capturedUrl.value)
  capturedUrl.value = ''
  capturedBlob.value = null
  locationBlocked.value = false
  locationBlockedMsg.value = ''
}

async function submitAbsen() {
  if (!capturedBlob.value) return
  submitting.value = true
  try {
    const fd = new FormData()
    fd.append('foto', capturedBlob.value, 'absen.jpg')
    if (coords.value.lat != null) fd.append('lat', String(coords.value.lat))
    if (coords.value.lng != null) fd.append('lng', String(coords.value.lng))
    if (coords.value.accuracy != null) fd.append('accuracy', String(coords.value.accuracy))
    fd.append('kedipan_ok', blink.blinkDetected.value ? 'true' : 'false')
    const url = cameraMode.value === 'masuk' ? '/absensi/masuk' : '/absensi/pulang'
    const { data } = await http.post(url, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
    // pakai baris absen dari response supaya tombol langsung nonaktif
    // walaupun pemuatan ulang riwayat masih berjalan.
    if (data.data) statusHariIni.value = data.data
    closeCameraDialog()
    loadRiwayat()
    loadStatusHariIni()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    submitting.value = false
  }
}

onBeforeUnmount(() => {
  stopCamera()
  revokeThumbnails()
})

// ============================================================
// lihat foto hasil absen
// ============================================================

const fotoDialog = ref(false)
const fotoDialogUrl = ref('')
const fotoDialogTitle = ref('')

async function lihatFoto(row, jenis) {
  try {
    const res = await http.get(`/absensi/foto/${row.id}/${jenis}`, { responseType: 'blob' })
    fotoDialogUrl.value = URL.createObjectURL(res.data)
    fotoDialogTitle.value = `Foto ${jenis === 'masuk' ? 'Absen Masuk' : 'Absen Pulang'} -- ${formatTanggal(dateKey(row.tanggal))}`
    fotoDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat foto', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
function closeFotoDialog() {
  fotoDialog.value = false
  if (fotoDialogUrl.value) URL.revokeObjectURL(fotoDialogUrl.value)
  fotoDialogUrl.value = ''
}

// ============================================================
// surat pendukung yang diinput admin (pegawai hanya bisa melihat/unduh --
// input & hapus sekarang khusus administrator/admin lewat halaman Rekap
// Absen, lihat RekapAbsensiView.vue)
// ============================================================

async function downloadDokumen(item) {
  try {
    const res = await http.get(`/absensi/dokumen/${item.id}/file`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = item.nama_file || 'surat'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}
</script>

<template>
  <div class="absensi-page">
    <div class="page-header">
      <h2>Absen</h2>
      <p class="text-muted">Absen masuk &amp; pulang lewat kamera dengan verifikasi kedipan mata.</p>
    </div>

    <div v-if="loadingPengaturan" class="loading-box"><ProgressSpinner style="width: 40px; height: 40px" /></div>

    <template v-else-if="pengaturan && !pengaturan.aktif">
      <Message severity="warn" :closable="false">
        Menu Absen sedang dinonaktifkan oleh administrator. Hubungi administrator/admin jika ini tidak sesuai.
      </Message>
    </template>

    <template v-else-if="pengaturan && pengaturan.eligible === false">
      <Message severity="warn" :closable="false">
        Menu ini bukan untuk Anda. Hubungi administrator/admin jika menurut Anda ini tidak sesuai.
      </Message>
    </template>

    <template v-else>
      <div class="jam-info">
        <Message severity="info" :closable="false">
          Absen masuk dibuka mulai jam <b>{{ pengaturan?.jam_mulai_pagi }}</b> (dianggap terlambat setelah jam
          <b>{{ pengaturan?.jam_batas_pagi }}</b>). Absen pulang dibuka mulai jam <b>{{ pengaturan?.jam_mulai_pulang }}</b>.
        </Message>
      </div>

      <div class="absen-actions">
        <div class="absen-card">
          <i class="pi pi-sign-in" style="font-size: 1.8rem; color: #16a34a"></i>
          <div class="absen-card-label">Absen Masuk</div>
          <div class="absen-card-status">
            <Tag v-if="sudahMasuk" severity="success" value="Sudah absen masuk" />
            <span v-else-if="!canMasuk" class="text-muted">belum dibuka / tidak aktif</span>
          </div>
          <Button
            :label="sudahMasuk ? 'Sudah Absen Masuk' : 'Absen Masuk'"
            :icon="sudahMasuk ? 'pi pi-check' : 'pi pi-camera'"
            :disabled="!canMasuk"
            @click="openCamera('masuk')"
          />
          <div v-if="todayRow?.jam_masuk" class="absen-card-detail">
            Jam masuk: {{ formatJam(todayRow.jam_masuk) }}
            <span v-if="todayRow.terlambat_menit > 0" class="text-danger"> (terlambat {{ todayRow.terlambat_menit }} menit)</span>
          </div>
        </div>
        <div class="absen-card">
          <i class="pi pi-sign-out" style="font-size: 1.8rem; color: #dc2626"></i>
          <div class="absen-card-label">Absen Pulang</div>
          <div class="absen-card-status">
            <Tag v-if="sudahPulang" severity="success" value="Sudah absen pulang" />
            <span v-else-if="!canPulang" class="text-muted">belum dibuka / tidak aktif</span>
          </div>
          <Button
            :label="sudahPulang ? 'Sudah Absen Pulang' : 'Absen Pulang'"
            :icon="sudahPulang ? 'pi pi-check' : 'pi pi-camera'"
            severity="danger"
            :disabled="!canPulang"
            @click="openCamera('pulang')"
          />
          <div v-if="todayRow?.jam_pulang" class="absen-card-detail">Jam pulang: {{ formatJam(todayRow.jam_pulang) }}</div>
        </div>
      </div>

      <div class="section">
        <div class="section-header">
          <h3>Riwayat Absen</h3>
          <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        </div>
        <DataTable :value="riwayat.absensi" :loading="loadingRiwayat" size="small" stripedRows responsiveLayout="scroll">
          <Column field="tanggal" header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
          </Column>
          <Column header="Jam Masuk">
            <template #body="{ data }">
              <a v-if="data.jam_masuk" href="#" @click.prevent="lihatFoto(data, 'masuk')">{{ formatJam(data.jam_masuk) }}</a>
              <span v-else>-</span>
              <i v-if="data.jam_masuk && !data.kedipan_masuk_ok" class="pi pi-exclamation-triangle" style="color: #d97706; margin-left: 4px" title="Kedipan mata tidak terdeteksi pada foto ini" />
            </template>
          </Column>
          <Column header="Terlambat">
            <template #body="{ data }">
              <Tag v-if="data.terlambat_menit > 0" severity="danger" :value="`${data.terlambat_menit} menit`" />
              <span v-else-if="data.jam_masuk">Tepat waktu</span>
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Jam Pulang">
            <template #body="{ data }">
              <a v-if="data.jam_pulang" href="#" @click.prevent="lihatFoto(data, 'pulang')">{{ formatJam(data.jam_pulang) }}</a>
              <span v-else>-</span>
              <i v-if="data.jam_pulang && !data.kedipan_pulang_ok" class="pi pi-exclamation-triangle" style="color: #d97706; margin-left: 4px" title="Kedipan mata tidak terdeteksi pada foto ini" />
            </template>
          </Column>
          <Column header="Foto Masuk">
            <template #body="{ data }">
              <img
                v-if="thumbUrl(data, 'masuk')"
                :src="thumbUrl(data, 'masuk')"
                class="foto-thumb"
                alt="Foto absen masuk"
                title="Klik untuk memperbesar"
                @click="lihatFoto(data, 'masuk')"
              />
              <span v-else-if="data.jam_masuk" class="text-muted">memuat...</span>
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Foto Pulang">
            <template #body="{ data }">
              <img
                v-if="thumbUrl(data, 'pulang')"
                :src="thumbUrl(data, 'pulang')"
                class="foto-thumb"
                alt="Foto absen pulang"
                title="Klik untuk memperbesar"
                @click="lihatFoto(data, 'pulang')"
              />
              <span v-else-if="data.jam_pulang" class="text-muted">memuat...</span>
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Titik Koordinat">
            <template #body="{ data }">
              <div class="koordinat-cell">
                <a
                  v-if="punyaKoordinat(data, 'masuk')"
                  class="koordinat-link"
                  :href="mapsUrlBaris(data, 'masuk')"
                  target="_blank"
                  rel="noopener"
                  title="Buka lokasi absen masuk di Google Maps"
                >
                  <i class="pi pi-map-marker"></i> Masuk: {{ formatKoordinat(data, 'masuk') }}
                </a>
                <a
                  v-if="punyaKoordinat(data, 'pulang')"
                  class="koordinat-link"
                  :href="mapsUrlBaris(data, 'pulang')"
                  target="_blank"
                  rel="noopener"
                  title="Buka lokasi absen pulang di Google Maps"
                >
                  <i class="pi pi-map-marker"></i> Pulang: {{ formatKoordinat(data, 'pulang') }}
                </a>
                <span v-if="!punyaKoordinat(data, 'masuk') && !punyaKoordinat(data, 'pulang')">-</span>
              </div>
            </template>
          </Column>
          <template #empty>Belum ada riwayat absen pada bulan ini.</template>
        </DataTable>
      </div>

      <div v-if="riwayat.tanggal_terlewat?.length" class="section">
        <h3>Tanggal Terlewat</h3>
        <Message severity="warn" :closable="false">
          Hari kerja berikut belum ada absennya dan belum ada surat pendukung (BA/Surat Tugas/Surat Izin/SKS) yang
          diinput administrator. Hubungi administrator/admin bila Anda memang bertugas/izin/sakit pada tanggal
          tersebut, supaya suratnya dapat diinput.
        </Message>
        <div class="terlewat-list">
          <div v-for="tgl in riwayat.tanggal_terlewat" :key="tgl" class="terlewat-item">
            <span>{{ formatTanggal(tgl) }}</span>
            <Tag severity="warn" value="Tidak melakukan absensi" />
          </div>
        </div>
      </div>

      <div v-if="dokumenList.length" class="section">
        <h3>Surat Pendukung (Diinput Administrator)</h3>
        <p class="text-muted">Surat berikut diinput oleh administrator/admin untuk melengkapi tanggal absen Anda.</p>
        <DataTable :value="dokumenList" size="small" stripedRows responsiveLayout="scroll">
          <Column header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
          </Column>
          <Column field="label" header="Jenis Surat" />
          <Column header="Kode">
            <template #body="{ data }">
              <Tag :value="kodeDokumen(data.jenis)" :title="labelKodeDokumen(data.jenis)" />
            </template>
          </Column>
          <Column field="keterangan" header="Keterangan" />
          <Column header="Aksi">
            <template #body="{ data }">
              <Button icon="pi pi-download" size="small" text rounded title="Unduh" @click="downloadDokumen(data)" />
            </template>
          </Column>
        </DataTable>
      </div>
    </template>

    <!-- ================= dialog kamera + kedipan ================= -->
    <Dialog
      v-model:visible="cameraDialog"
      modal
      :header="labelMode(cameraMode)"
      :style="{ width: '440px' }"
      :breakpoints="{ '640px': '94vw' }"
      @hide="stopCamera"
    >
      <div v-if="locationChecking" class="loading-box">
        <ProgressSpinner style="width: 40px; height: 40px" />
        <p class="text-muted" style="margin-top: 0.5rem">Memeriksa titik koordinat Anda...</p>
      </div>

      <Message v-else-if="locationBlocked" severity="warn" :closable="false">{{ locationBlockedMsg }}</Message>

      <template v-else>
        <Message v-if="cameraError" severity="error" :closable="false">{{ cameraError }}</Message>
        <Message v-else-if="blink.modelsError.value" severity="warn" :closable="false">
          {{ blink.modelsError.value }} -- deteksi kedipan otomatis tidak tersedia, gunakan tombol "Ambil Foto" secara manual.
        </Message>

        <div class="camera-box">
          <div v-if="!capturedUrl" class="video-wrap">
            <video ref="videoEl" autoplay playsinline muted class="camera-video"></video>
            <div class="camera-overlay">
              <span v-if="!blink.faceDetected.value">Arahkan wajah ke kamera...</span>
              <span v-else-if="!blink.blinkDetected.value">Wajah terdeteksi -- berkedip untuk mengambil foto otomatis</span>
            </div>
          </div>
          <img v-else :src="capturedUrl" class="camera-video" alt="Foto absen" />
          <canvas ref="canvasEl" style="display: none"></canvas>
        </div>

        <Message v-if="noBlinkWarning && !capturedUrl" severity="warn" :closable="false">
          Belum terdeteksi kedipan mata. Pastikan wajah terlihat jelas oleh kamera, atau ambil foto secara manual di bawah.
        </Message>
        <p class="text-muted geo-status">{{ geoStatus }}</p>
      </template>

      <template #footer>
        <template v-if="locationChecking">
          <Button label="Batal" severity="secondary" text @click="closeCameraDialog" />
        </template>
        <template v-else-if="locationBlocked">
          <Button label="Coba Lagi" icon="pi pi-refresh" @click="retryLocation" />
          <Button label="Batal" severity="secondary" text @click="closeCameraDialog" />
        </template>
        <template v-else-if="!capturedUrl">
          <Button label="Ambil Foto Manual" icon="pi pi-camera" severity="secondary" outlined :disabled="!!cameraError" @click="ambilFotoManual" />
          <Button label="Batal" severity="secondary" text @click="closeCameraDialog" />
        </template>
        <template v-else>
          <Button label="Ulangi" icon="pi pi-refresh" severity="secondary" outlined @click="retake" :disabled="submitting" />
          <Button label="Kirim" icon="pi pi-check" @click="submitAbsen" :loading="submitting" />
        </template>
      </template>
    </Dialog>

    <!-- ================= dialog lihat foto ================= -->
    <Dialog
      v-model:visible="fotoDialog"
      modal
      :header="fotoDialogTitle"
      :style="{ width: '420px' }"
      :breakpoints="{ '640px': '94vw' }"
      @hide="closeFotoDialog"
    >
      <img v-if="fotoDialogUrl" :src="fotoDialogUrl" style="width: 100%; border-radius: 8px" alt="Foto absen" />
    </Dialog>
  </div>
</template>

<style scoped>
/* padding disamakan dengan .page-wrap (style.css) supaya isi halaman tidak
   menempel ke tepi layar -- terasa terutama di HP. */
.absensi-page {
  max-width: 1280px;
  padding: 1rem;
}
@media (min-width: 768px) {
  .absensi-page {
    padding: 1.5rem 2rem;
  }
}
.page-header h2 {
  margin: 0 0 0.15rem;
}
.foto-thumb {
  width: 48px;
  height: 48px;
  object-fit: cover;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  cursor: pointer;
  display: block;
}
.foto-thumb:hover {
  border-color: #6366f1;
}
.koordinat-cell {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.koordinat-link {
  color: #4f46e5;
  text-decoration: none;
  white-space: nowrap;
  font-size: 0.82rem;
}
.koordinat-link:hover {
  text-decoration: underline;
}
.koordinat-link i {
  font-size: 0.72rem;
}
.text-muted {
  color: #6b7280;
  font-size: 0.85rem;
}
.text-danger {
  color: #dc2626;
}
.loading-box {
  display: flex;
  justify-content: center;
  padding: 2rem;
}
.jam-info {
  margin: 1rem 0;
}
.absen-actions {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
  margin-bottom: 1.5rem;
}
.absen-card {
  flex: 1;
  min-width: 220px;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  text-align: center;
}
.absen-card-label {
  font-weight: 600;
}
.absen-card-status {
  min-height: 1.5rem;
}
.absen-card-detail {
  font-size: 0.85rem;
  color: #374151;
}
.section {
  margin: 1.75rem 0;
}
.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}
.section-header h3,
.section > h3 {
  margin: 0 0 0.5rem;
}
.terlewat-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.terlewat-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: 1px solid #fde68a;
  background: #fffbeb;
  border-radius: 8px;
  padding: 0.5rem 0.9rem;
}
.camera-box {
  display: flex;
  justify-content: center;
  margin: 0.5rem 0;
}
.video-wrap {
  position: relative;
  width: 100%;
  max-width: 320px;
  aspect-ratio: 1 / 1;
}
.camera-video {
  width: 100%;
  max-width: 320px;
  aspect-ratio: 1 / 1;
  object-fit: cover;
  border-radius: 12px;
  background: #111827;
  transform: scaleX(-1);
}
.camera-overlay {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 8px;
  text-align: center;
  color: #fff;
  font-size: 0.8rem;
  background: rgba(0, 0, 0, 0.45);
  border-radius: 6px;
  padding: 4px 8px;
  margin: 0 8px;
}
.geo-status {
  text-align: center;
  margin-top: 0.5rem;
}
.field {
  margin-bottom: 1rem;
}
.field label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.3rem;
  color: #374151;
}

/* ---------- tampilan HP ---------- */
@media (max-width: 640px) {
  .absen-card {
    min-width: 100%;
  }
  /* tombol absen dibuat selebar kartu supaya gampang ditekan dengan jempol */
  .absen-card :deep(.p-button) {
    width: 100%;
    justify-content: center;
  }
  .section-header {
    align-items: stretch;
  }
  .section-header :deep(.p-datepicker) {
    width: 100% !important;
  }
  .terlewat-item {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
  }
}
</style>
