<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { toApiDate } from '../utils/date'
import { useBlinkLiveness } from '../composables/useBlinkLiveness'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Textarea from 'primevue/textarea'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

const toast = useToast()
const confirm = useConfirm()

// ============================================================
// pengaturan (aktif/nonaktif & jendela waktu) + riwayat bulanan
// ============================================================

const pengaturan = ref(null)
const loadingPengaturan = ref(true)
const periodDate = ref(new Date())
const riwayat = ref({ absensi: [], tanggal_terlewat: [] })
const loadingRiwayat = ref(false)
const dokumenList = ref([])

function dateKey(iso) {
  return (iso || '').slice(0, 10)
}
function todayKey() {
  return toApiDate(new Date())
}

const todayRow = computed(() => riwayat.value.absensi.find((a) => dateKey(a.tanggal) === todayKey()) || null)
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
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat riwayat absen', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingRiwayat.value = false
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

function dokumenFor(tanggal) {
  return dokumenList.value.find((d) => dateKey(d.tanggal) === tanggal)
}

watch(periodDate, () => loadRiwayat())

onMounted(async () => {
  await loadPengaturan()
  await Promise.all([loadRiwayat(), loadDokumen()])
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
const coords = ref({ lat: null, lng: null })
const geoStatus = ref('')

let mediaStream = null
let noBlinkTimer = null
const blink = useBlinkLiveness()

function labelMode(mode) {
  return mode === 'masuk' ? 'Absen Masuk' : 'Absen Pulang'
}

function fetchLocation() {
  if (!navigator.geolocation) {
    geoStatus.value = 'perangkat/browser tidak mendukung deteksi lokasi'
    return
  }
  geoStatus.value = 'mencari titik koordinat...'
  navigator.geolocation.getCurrentPosition(
    (pos) => {
      coords.value = { lat: pos.coords.latitude, lng: pos.coords.longitude }
      geoStatus.value = `titik koordinat ditemukan (akurasi ±${Math.round(pos.coords.accuracy)}m)`
    },
    () => {
      geoStatus.value = 'lokasi tidak diizinkan/tidak ditemukan -- absen tetap bisa dilanjutkan tanpa koordinat'
    },
    { enableHighAccuracy: true, timeout: 10000, maximumAge: 30000 },
  )
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
  cameraMode.value = mode
  capturedBlob.value = null
  if (capturedUrl.value) URL.revokeObjectURL(capturedUrl.value)
  capturedUrl.value = ''
  noBlinkWarning.value = false
  coords.value = { lat: null, lng: null }
  cameraDialog.value = true

  fetchLocation()
  try {
    await blink.init()
  } catch {
    // blink.modelsError sudah terisi, ditampilkan lewat Message di template;
    // kamera tetap dibuka supaya pegawai masih bisa ambil foto manual.
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
}

async function submitAbsen() {
  if (!capturedBlob.value) return
  submitting.value = true
  try {
    const fd = new FormData()
    fd.append('foto', capturedBlob.value, 'absen.jpg')
    if (coords.value.lat != null) fd.append('lat', String(coords.value.lat))
    if (coords.value.lng != null) fd.append('lng', String(coords.value.lng))
    fd.append('kedipan_ok', blink.blinkDetected.value ? 'true' : 'false')
    const url = cameraMode.value === 'masuk' ? '/absensi/masuk' : '/absensi/pulang'
    const { data } = await http.post(url, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
    closeCameraDialog()
    loadRiwayat()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    submitting.value = false
  }
}

onBeforeUnmount(() => stopCamera())

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
// upload surat pengganti untuk tanggal terlewat
// ============================================================

const uploadDialog = ref(false)
const uploadTanggal = ref('')
const uploadForm = reactive({ jenis: null, keterangan: '' })
const uploadFile = ref(null)
const uploadFileInput = ref(null)
const uploading = ref(false)

const jenisSuratOptions = [
  { label: 'SKS (Surat Keterangan Sakit)', value: 'sks' },
  { label: 'Surat Tugas', value: 'surat_tugas' },
  { label: 'Berita Acara', value: 'berita_acara' },
  { label: 'Surat Izin', value: 'surat_izin' },
]

function openUpload(tanggal) {
  uploadTanggal.value = tanggal
  uploadForm.jenis = null
  uploadForm.keterangan = ''
  uploadFile.value = null
  uploadDialog.value = true
}
function pickUploadFile() {
  uploadFileInput.value?.click()
}
function onUploadFileChosen(e) {
  uploadFile.value = e.target.files[0] || null
}

async function submitUpload() {
  if (!uploadForm.jenis || !uploadFile.value) return
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('tanggal', uploadTanggal.value)
    fd.append('jenis', uploadForm.jenis)
    fd.append('keterangan', uploadForm.keterangan || '')
    fd.append('file', uploadFile.value)
    const { data } = await http.post('/absensi/dokumen', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 4000 })
    uploadDialog.value = false
    await Promise.all([loadRiwayat(), loadDokumen()])
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    uploading.value = false
  }
}

function confirmHapusDokumen(item) {
  confirm.require({
    message: `Hapus surat pengganti untuk tanggal ${formatTanggal(dateKey(item.tanggal))}?`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    accept: async () => {
      try {
        await http.delete(`/absensi/dokumen/${item.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Surat pengganti dihapus', life: 3000 })
        await Promise.all([loadRiwayat(), loadDokumen()])
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

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
          <Button label="Absen Masuk" icon="pi pi-camera" :disabled="!canMasuk" @click="openCamera('masuk')" />
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
          <Button label="Absen Pulang" icon="pi pi-camera" severity="danger" :disabled="!canPulang" @click="openCamera('pulang')" />
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
          <Column header="Titik Koordinat">
            <template #body="{ data }">{{ formatKoordinat(data, data.jam_pulang ? 'pulang' : 'masuk') }}</template>
          </Column>
          <template #empty>Belum ada riwayat absen pada bulan ini.</template>
        </DataTable>
      </div>

      <div v-if="riwayat.tanggal_terlewat?.length" class="section">
        <h3>Tanggal Terlewat</h3>
        <p class="text-muted">Hari kerja berikut belum ada absennya. Upload surat pendukung (SKS/Surat Tugas/Berita Acara/Surat Izin) untuk melengkapi.</p>
        <div class="terlewat-list">
          <div v-for="tgl in riwayat.tanggal_terlewat" :key="tgl" class="terlewat-item">
            <span>{{ formatTanggal(tgl) }}</span>
            <Button label="Upload Surat" icon="pi pi-upload" size="small" outlined @click="openUpload(tgl)" />
          </div>
        </div>
      </div>

      <div v-if="dokumenList.length" class="section">
        <h3>Surat Pengganti yang Diupload</h3>
        <DataTable :value="dokumenList" size="small" stripedRows responsiveLayout="scroll">
          <Column header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
          </Column>
          <Column field="label" header="Jenis Surat" />
          <Column field="keterangan" header="Keterangan" />
          <Column header="Aksi">
            <template #body="{ data }">
              <Button icon="pi pi-download" size="small" text rounded title="Unduh" @click="downloadDokumen(data)" />
              <Button icon="pi pi-trash" size="small" text rounded severity="danger" title="Hapus" @click="confirmHapusDokumen(data)" />
            </template>
          </Column>
        </DataTable>
      </div>
    </template>

    <!-- ================= dialog kamera + kedipan ================= -->
    <Dialog v-model:visible="cameraDialog" modal :header="labelMode(cameraMode)" :style="{ width: '440px' }" @hide="stopCamera">
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

      <template #footer>
        <template v-if="!capturedUrl">
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
    <Dialog v-model:visible="fotoDialog" modal :header="fotoDialogTitle" :style="{ width: '420px' }" @hide="closeFotoDialog">
      <img v-if="fotoDialogUrl" :src="fotoDialogUrl" style="width: 100%; border-radius: 8px" alt="Foto absen" />
    </Dialog>

    <!-- ================= dialog upload surat pengganti ================= -->
    <Dialog v-model:visible="uploadDialog" modal header="Upload Surat Pengganti" :style="{ width: '420px' }">
      <div class="field">
        <label>Tanggal</label>
        <div>{{ formatTanggal(uploadTanggal) }}</div>
      </div>
      <div class="field">
        <label>Jenis Surat</label>
        <Select v-model="uploadForm.jenis" :options="jenisSuratOptions" optionLabel="label" optionValue="value" placeholder="Pilih jenis surat" style="width: 100%" />
      </div>
      <div class="field">
        <label>Keterangan (opsional)</label>
        <Textarea v-model="uploadForm.keterangan" rows="2" style="width: 100%" />
      </div>
      <div class="field">
        <label>Berkas (PDF/JPG/PNG)</label>
        <input ref="uploadFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onUploadFileChosen" />
        <Button :label="uploadFile ? uploadFile.name : 'Pilih Berkas'" icon="pi pi-file" severity="secondary" outlined @click="pickUploadFile" />
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" text @click="uploadDialog = false" />
        <Button label="Upload" icon="pi pi-upload" :disabled="!uploadForm.jenis || !uploadFile" :loading="uploading" @click="submitUpload" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.absensi-page {
  max-width: 960px;
}
.page-header h2 {
  margin: 0 0 0.15rem;
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
  width: 320px;
  height: 320px;
}
.camera-video {
  width: 320px;
  height: 320px;
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
</style>
