<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import { toApiDate } from '../utils/date'
import { useBlinkLiveness } from '../composables/useBlinkLiveness'
import { KETERANGAN_SURAT_DROPDOWN, KETERANGAN_LAINNYA, pisahkanKeterangan, gabungkanKeterangan } from '../composables/keteranganSurat'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import DatePicker from 'primevue/datepicker'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Checkbox from 'primevue/checkbox'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'

const toast = useToast()

// isSekolahSaya: dikirim langsung oleh backend lewat riwayat.is_sekolah
// (hasil isSekolahPegawai yang sudah memperhitungkan UnitKerja.TempatKerja
// sebagai prioritas utama -- lihat riwayatAbsenResponse.IsSekolah di
// handlers/absensi.go) -- dipakai untuk menampilkan/menyembunyikan kartu
// "Ajukan Surat Kolektif" di bawah, pegawai bertugas DINAS/KANTOR tidak
// boleh mengajukan sendiri (poin 6 permintaan pengguna).
const isSekolahSaya = computed(() => !!riwayat.value?.is_sekolah)

// ============================================================
// pengaturan (aktif/nonaktif & jendela waktu) + riwayat bulanan
// ============================================================

const pengaturan = ref(null)
const loadingPengaturan = ref(true)
const periodDate = ref(new Date())
const riwayat = ref({ absensi: [], tanggal_terlewat: [], tanggal_tercover: [] })
const loadingRiwayat = ref(false)
const dokumenList = ref([])

// Kode & Label kolom "Surat Pendukung" (data.kode/data.label) SEKARANG
// dikirim langsung oleh backend (lihat absensiDokumenSayaOut di
// listAbsensiDokumenSaya, handlers/absensi_dokumen.go), dihitung ulang LIVE
// dari master Jenis Surat (menu Master Data -> Jenis Surat) -- BUKAN lagi
// ditebak di sini lewat peta hardcode 4 slug bawaan (sks/surat_tugas/
// berita_acara/surat_izin). Peta hardcode itu SEBELUMNYA dipakai di sini
// (kodeDokumen/labelKodeDokumen) dan gagal mengenali Jenis Surat baru yang
// dibuat administrator sendiri (mis. slug "dinas_dalam") -- Kode-nya selalu
// tampil kosong walau Jenis Surat/labelnya sendiri tampil benar. Dihapus
// supaya tidak ada lagi dua sumber kebenaran (backend vs peta lokal di sini)
// yang bisa saling tidak sinkron.

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

// masukTertutup: sudah lewat jam tutup absen masuk (lihat JamTutupPagi di
// backend) dan pegawai belum absen masuk sama sekali hari ini -- begitu
// tertutup, absen masuk TIDAK bisa lagi (bukan cuma dianggap terlambat),
// dan otomatis absen pulang juga ikut tidak tersedia (mensyaratkan absen
// masuk).
const masukTertutup = computed(() => {
  if (sudahMasuk.value) return false
  const tutup = parseJam(pengaturan.value?.jam_tutup_pagi)
  return tutup != null && minutesNow() > tutup
})
const canMasuk = computed(() => {
  if (!pengaturan.value?.aktif || sudahMasuk.value || masukTertutup.value) return false
  const mulai = parseJam(pengaturan.value?.jam_mulai_pagi)
  return mulai == null || minutesNow() >= mulai
})
// pulangTertutup: sudah lewat jam tutup absen pulang (lihat JamTutupPulang
// di backend) dan pegawai belum absen pulang -- begitu tertutup, absen
// pulang TIDAK bisa lagi walaupun pegawai sudah absen masuk dan belum
// sempat absen pulang hari itu.
const pulangTertutup = computed(() => {
  if (sudahPulang.value) return false
  const tutup = parseJam(pengaturan.value?.jam_tutup_pulang)
  return tutup != null && minutesNow() > tutup
})
const canPulang = computed(() => {
  // absen pulang cuma tersedia kalau sudah absen masuk hari ini -- tidak
  // boleh lagi merekam kepulangan tanpa jam masuk sama sekali (lihat
  // pengecekan yang sama di backend, absenPulang di absensi.go).
  if (!pengaturan.value?.aktif || sudahPulang.value || !sudahMasuk.value || pulangTertutup.value) return false
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
// Pengajuan Surat Kolektif mandiri (khusus pegawai bertugas di sekolah --
// lihat isSekolahSaya di atas) untuk tanggal terlewat, lewat
// handlers/pengajuan_surat_kolektif.go. Menunggu verifikasi administrator/
// akun Admin Verifikasi sebelum absen benar-benar "berubah" jadi bersurat.
// ============================================================

const jenisSuratOptions = ref([])
async function loadJenisSuratOptions() {
  try {
    const { data } = await http.get('/ref/jenis-surat')
    jenisSuratOptions.value = (data.data || []).map((it) => ({ label: `${it.nama} (${it.kode})`, value: it.slug }))
  } catch {
    jenisSuratOptions.value = []
  }
}

const pengajuanSayaList = ref([])
async function loadPengajuanSaya() {
  try {
    const { data } = await http.get('/pengajuan-surat-kolektif/saya')
    pengajuanSayaList.value = data.data || []
  } catch {
    // tidak kritikal -- daftar cukup dibiarkan kosong kalau gagal
  }
}

// opsi tanggal untuk MultiSelect form pengajuan -- daftar tanggal_terlewat
// bulan yang sedang dilihat, DITAMBAH tanggal milik pengajuan yang sedang
// diedit (kalau ada, supaya tetap terlihat & bisa dipilih ulang walau
// pengajuan itu dibuat/berasal dari bulan lain).
function opsiTanggalUntuk(extraDates) {
  const set = new Set(riwayat.value.tanggal_terlewat || [])
  for (const t of extraDates || []) set.add(t)
  return Array.from(set)
    .sort()
    .map((t) => ({ label: formatTanggal(t), value: t }))
}
const opsiTanggalKolektif = computed(() => opsiTanggalUntuk(editPengajuanItem.value ? editPengajuanItem.value.tanggal_list : []))

const kolektifSelfForm = ref({ tanggal: [], jenis: null, keterangan: '' })
const kolektifSelfFile = ref(null)
const kolektifSelfFileInput = ref(null)
const submittingKolektifSelf = ref(false)
function pickKolektifSelfFile() {
  kolektifSelfFileInput.value?.click()
}
function onKolektifSelfFileChosen(e) {
  kolektifSelfFile.value = e.target.files?.[0] || null
}

// Keterangan sekarang dropdown (lihat composables/keteranganSurat.js) --
// kolektifSelfForm.keterangan tetap dipakai (ini yang benar-benar dikirim
// ke server), tapi nilainya diturunkan dari 2 ref terpisah ini: pilihan
// dropdown, dan teks bebas kalau pilihan = "Lainnya".
const kolektifSelfKeteranganPilihan = ref(null)
const kolektifSelfKeteranganLainnya = ref('')
watch([kolektifSelfKeteranganPilihan, kolektifSelfKeteranganLainnya], () => {
  kolektifSelfForm.value.keterangan = gabungkanKeterangan(kolektifSelfKeteranganPilihan.value, kolektifSelfKeteranganLainnya.value)
})

async function submitKolektifSelf() {
  if (!kolektifSelfForm.value.tanggal.length) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih minimal satu tanggal terlewat', life: 4000 })
    return
  }
  if (!kolektifSelfForm.value.jenis) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih jenis surat', life: 4000 })
    return
  }
  if (!kolektifSelfFile.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih berkas surat', life: 4000 })
    return
  }
  submittingKolektifSelf.value = true
  try {
    const fd = new FormData()
    for (const t of kolektifSelfForm.value.tanggal) fd.append('tanggal', t)
    fd.append('jenis', kolektifSelfForm.value.jenis)
    fd.append('keterangan', kolektifSelfForm.value.keterangan || '')
    fd.append('file', kolektifSelfFile.value)
    const { data } = await http.post('/pengajuan-surat-kolektif', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 5000 })
    kolektifSelfForm.value = { tanggal: [], jenis: null, keterangan: '' }
    kolektifSelfKeteranganPilihan.value = null
    kolektifSelfKeteranganLainnya.value = ''
    kolektifSelfFile.value = null
    await Promise.all([loadPengajuanSaya(), loadRiwayat()])
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 7000 })
  } finally {
    submittingKolektifSelf.value = false
  }
}

// dialog edit & ajukan ulang -- hanya untuk pengajuan berstatus "dikembalikan"
const editPengajuanDialog = ref(false)
const editPengajuanItem = ref(null)
const editPengajuanForm = ref({ tanggal: [], jenis: null, keterangan: '' })
const editPengajuanFile = ref(null)
const editPengajuanFileInput = ref(null)
const submittingEditPengajuan = ref(false)

// Sama seperti kolektifSelfKeteranganPilihan/Lainnya di atas -- lihat
// composables/keteranganSurat.js. bukaEditPengajuan menentukan pilihan awal
// dari teks keterangan yang sudah tersimpan (pisahkanKeterangan), termasuk
// untuk pengajuan lama yang keterangannya masih teks bebas dari sebelum
// dropdown ini ada -- otomatis jatuh ke "Lainnya" supaya teksnya tidak hilang.
const editPengajuanKeteranganPilihan = ref(null)
const editPengajuanKeteranganLainnya = ref('')
watch([editPengajuanKeteranganPilihan, editPengajuanKeteranganLainnya], () => {
  editPengajuanForm.value.keterangan = gabungkanKeterangan(editPengajuanKeteranganPilihan.value, editPengajuanKeteranganLainnya.value)
})

function bukaEditPengajuan(item) {
  editPengajuanItem.value = item
  editPengajuanForm.value = { tanggal: [...item.tanggal_list], jenis: item.jenis, keterangan: item.keterangan || '' }
  const { pilihan, lainnya } = pisahkanKeterangan(item.keterangan)
  editPengajuanKeteranganPilihan.value = pilihan
  editPengajuanKeteranganLainnya.value = lainnya
  editPengajuanFile.value = null
  editPengajuanDialog.value = true
}
function pickEditPengajuanFile() {
  editPengajuanFileInput.value?.click()
}
function onEditPengajuanFileChosen(e) {
  editPengajuanFile.value = e.target.files?.[0] || null
}
function closeEditPengajuan() {
  editPengajuanDialog.value = false
  editPengajuanItem.value = null
}

async function submitEditPengajuan() {
  if (!editPengajuanForm.value.tanggal.length || !editPengajuanForm.value.jenis) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih tanggal & jenis surat', life: 4000 })
    return
  }
  submittingEditPengajuan.value = true
  try {
    const fd = new FormData()
    for (const t of editPengajuanForm.value.tanggal) fd.append('tanggal', t)
    fd.append('jenis', editPengajuanForm.value.jenis)
    fd.append('keterangan', editPengajuanForm.value.keterangan || '')
    if (editPengajuanFile.value) fd.append('file', editPengajuanFile.value)
    const { data } = await http.put(`/pengajuan-surat-kolektif/${editPengajuanItem.value.id}`, fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 5000 })
    closeEditPengajuan()
    await Promise.all([loadPengajuanSaya(), loadRiwayat()])
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 7000 })
  } finally {
    submittingEditPengajuan.value = false
  }
}

async function downloadPengajuanFile(item) {
  try {
    const res = await http.get(`/pengajuan-surat-kolektif/${item.id}/file`, { responseType: 'blob' })
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

function statusPengajuanSeverity(status) {
  if (status === 'disetujui') return 'success'
  if (status === 'dikembalikan') return 'danger'
  return 'warn'
}
function statusPengajuanLabel(status) {
  if (status === 'disetujui') return 'Disetujui'
  if (status === 'dikembalikan') return 'Dikembalikan (perlu revisi)'
  return 'Menunggu Verifikasi'
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
  await Promise.all([loadRiwayat(), loadDokumen(), loadStatusHariIni(), loadJenisSuratOptions(), loadPengajuanSaya()])
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
// dinasDalam: tombol "Dinas Dalam" pada dialog kamera -- kalau dicentang,
// pegawai boleh absen dari mana saja (validasi radius kantor dilewati, lihat
// cekLokasiKantor & absensiCekRadius di backend), tapi hari itu akan tercatat
// sebagai "Dinas Dalam" pada rekap/riwayat, BUKAN "Hadir" (lihat backend
// models.Absensi.IsDinasDalam). Berbeda dari surat tugas/berita acara yang
// diinput admin -- ini laporan mandiri pegawai saat mengambil foto absen.
const dinasDalam = ref(false)

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

// -- Pengambilan lokasi GPS: beberapa sampel, bukan satu kali tembak --
// Versi sebelumnya hanya memanggil getCurrentPosition() SEKALI dan memakai
// apa pun hasilnya. Ini rawan meleset jauh (pernah tercatat selisih
// 500-800m padahal masih di kantor yang sama) karena chip GPS ponsel butuh
// beberapa detik untuk "settle" -- pembacaan pertama begitu GPS baru mulai
// mencari sinyal (apalagi kalau sebelumnya GPS mati/idle) sering jauh lebih
// tidak akurat daripada pembacaan berikutnya beberapa detik kemudian,
// walaupun browser tetap melaporkan angka accuracy yang kelihatan wajar.
//
// Sekarang dipakai watchPosition() untuk mengumpulkan beberapa sampel
// selama jendela waktu singkat, lalu diambil sampel dengan accuracy
// (radius kesalahan yang dilaporkan, dalam meter) TERKECIL -- itulah
// pembacaan yang paling presisi. Berhenti lebih awal begitu sudah dapat
// sampel yang cukup akurat, supaya pegawai tidak menunggu lebih lama dari
// perlu; tetap dibatasi jendela waktu maksimum supaya tidak menunggu tanpa
// henti kalau GPS memang tidak kunjung stabil (fallback: pakai sampel
// terbaik yang berhasil didapat sejauh itu, atau null kalau tidak ada sama
// sekali).
const GPS_TARGET_ACCURACY_M = 15
const GPS_MAX_WAIT_MS = 7000

function getLocationOnce() {
  return new Promise((resolve) => {
    if (!navigator.geolocation) {
      resolve(null)
      return
    }
    let best = null
    let watchId = null
    let settled = false
    let hardTimer = null

    const finish = () => {
      if (settled) return
      settled = true
      if (hardTimer) clearTimeout(hardTimer)
      if (watchId != null) navigator.geolocation.clearWatch(watchId)
      resolve(best)
    }

    const onSample = (pos) => {
      const sample = { lat: pos.coords.latitude, lng: pos.coords.longitude, accuracy: pos.coords.accuracy }
      if (!best || best.accuracy == null || (sample.accuracy != null && sample.accuracy < best.accuracy)) {
        best = sample
      }
      // sudah cukup akurat -- tak perlu menunggu sampel lagi.
      if (sample.accuracy != null && sample.accuracy <= GPS_TARGET_ACCURACY_M) {
        finish()
      }
    }

    const onError = (err) => {
      // izin lokasi ditolak: menunggu lebih lama tidak akan membantu.
      // Error lain (timeout sampel tunggal / posisi sesaat tak tersedia)
      // diabaikan -- watchPosition akan mencoba lagi sendiri, dan tetap ada
      // batas waktu total di bawah.
      if (err && err.code === 1) finish()
    }

    try {
      watchId = navigator.geolocation.watchPosition(onSample, onError, {
        enableHighAccuracy: true,
        maximumAge: 0,
        timeout: GPS_MAX_WAIT_MS,
      })
    } catch {
      resolve(null)
      return
    }

    hardTimer = setTimeout(finish, GPS_MAX_WAIT_MS)
  })
}

// cekLokasiKantor mencari titik koordinat pegawai lalu memeriksanya terhadap
// titik koordinat ACUAN untuk pegawai yang bersangkutan (titik_lat/titik_lng/
// titik_radius pada /absensi/pengaturan -- lihat TitikLat pada
// pengaturanAbsensiOut & resolveGeofenceTarget di backend absensi.go).
// SEBELUMNYA fungsi ini memakai kantor_lat/kantor_lng (titik kantor pusat
// TUNGGAL) langsung, tanpa memperhitungkan titik koordinat unit kerja/
// sekolah pegawai sendiri -- akibatnya pegawai di sekolah yang titik
// koordinatnya sendiri sudah benar diisi admin tetap ditolak di sini
// (SEBELUM request sempat dikirim ke backend sama sekali, jadi perbaikan di
// absensiCekRadius backend tidak pernah kelihatan), karena jaraknya
// dihitung ke kantor pusat yang jauh. Sekarang memakai titik yang sudah
// "resolved" oleh backend, sama seperti yang dipakai validasi akhir di
// server, supaya kedua validasi ini selalu konsisten. Kamera hanya dibuka
// kalau lolos -- kalau di luar radius atau lokasi tidak terdeteksi (padahal
// geofence aktif), kamera TIDAK dibuka dan peringatan ditampilkan (lihat
// locationBlocked di template).
async function cekLokasiKantor() {
  locationChecking.value = true
  geoStatus.value = 'mencari titik koordinat...'
  const pos = await getLocationOnce()
  locationChecking.value = false

  const kantorLat = pengaturan.value?.titik_lat
  const kantorLng = pengaturan.value?.titik_lng
  const radius = pengaturan.value?.titik_radius || 20
  const sumberTitik = pengaturan.value?.titik_sumber || 'kantor'
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
  // GPS ponsel (apalagi di dalam gedung) sering meleset walaupun pegawai
  // tidak bergerak -- accuracy yang dilaporkan browser sering terlalu percaya
  // diri di dalam gedung (sinyal memantul di dinding/lantai beton), jadi
  // toleransi minimal 30m tetap dipakai walau accuracy dilaporkan sangat
  // kecil (mis. 13m), supaya pegawai yang benar-benar di kantor tidak
  // berulang kali ditolak. Toleransi dibatasi maks. 50m (BUKAN 100m lagi --
  // lihat toleransiAkurasiMaksimal di backend absensi.go untuk alasannya:
  // toleransi sebesar itu digabung radius yang diperbesar admin bisa
  // membuat absen lolos dari ratusan meter jauhnya, termasuk dari rumah).
  // Pengecekan akhir & mengikat tetap dilakukan ulang di server
  // (absensiCekRadius di backend) dengan aturan yang sama (lihat
  // toleransiAkurasiMinimum/Maksimal di sana).
  const toleransi = pos.accuracy > 30 ? Math.min(pos.accuracy, 50) : 30
  const jarakEfektif = Math.max(jarak - toleransi, 0)
  if (jarakEfektif > radius) {
    locationBlocked.value = true
    const infoAkurasi = pos.accuracy > 0 ? ` (akurasi GPS perangkat Anda saat ini sekitar ${Math.round(pos.accuracy)} meter)` : ''
    locationBlockedMsg.value = `Anda berada di luar radius ${sumberTitik} (jarak sekitar ${Math.round(jarak)} meter, maksimal ${radius} meter dari titik acuan)${infoAkurasi}. Absen tidak dapat dilakukan dari lokasi ini.`
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
  if (mode === 'masuk' && masukTertutup.value) {
    toast.add({
      severity: 'warn',
      summary: 'Absen masuk ditutup',
      detail: `Batas waktu absen masuk sudah lewat (ditutup otomatis mulai jam ${pengaturan.value?.jam_tutup_pagi}), absen masuk untuk hari ini tidak lagi tersedia`,
      life: 5000,
    })
    return
  }
  if (mode === 'pulang' && sudahPulang.value) {
    toast.add({ severity: 'info', summary: 'Sudah absen', detail: 'Anda sudah absen pulang hari ini', life: 3000 })
    return
  }
  if (mode === 'pulang' && !sudahMasuk.value) {
    toast.add({ severity: 'warn', summary: 'Belum absen masuk', detail: 'Absen pulang baru tersedia setelah anda absen masuk hari ini', life: 4000 })
    return
  }
  if (mode === 'pulang' && pulangTertutup.value) {
    toast.add({
      severity: 'warn',
      summary: 'Absen pulang ditutup',
      detail: `Batas waktu absen pulang sudah lewat (ditutup otomatis mulai jam ${pengaturan.value?.jam_tutup_pulang}), absen pulang untuk hari ini tidak lagi tersedia`,
      life: 5000,
    })
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
  dinasDalam.value = false
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

// Kalau pegawai mencentang "Dinas Dalam" pada saat dialog sedang menampilkan
// peringatan lokasi di luar radius (locationBlocked), langsung lewati
// pemblokiran itu dan buka kameranya -- tidak perlu menekan tombol lain.
// Validasi radius tetap dilewati juga di backend selama dinas_dalam=true
// dikirim saat submit (lihat absenMasuk/absenPulang di absensi.go).
watch(dinasDalam, async (aktif) => {
  if (aktif && locationBlocked.value) {
    locationBlocked.value = false
    locationBlockedMsg.value = ''
    try {
      await blink.init()
    } catch {
      // lihat komentar di openCamera
    }
    await startCameraStream()
  }
})

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
  dinasDalam.value = false
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
    fd.append('dinas_dalam', dinasDalam.value ? 'true' : 'false')
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
          <b>{{ pengaturan?.jam_batas_pagi }}</b>, dan otomatis DITUTUP setelah jam <b>{{ pengaturan?.jam_tutup_pagi }}</b> kalau belum absen masuk sama sekali).
          Absen pulang dibuka mulai jam <b>{{ pengaturan?.jam_mulai_pulang }}</b> (setelah absen masuk berhasil dicatat), dan otomatis DITUTUP setelah jam <b>{{ pengaturan?.jam_tutup_pulang }}</b> kalau belum sempat absen pulang.
        </Message>
      </div>

      <div class="absen-actions">
        <div class="absen-card">
          <i class="pi pi-sign-in" style="font-size: 1.8rem; color: #16a34a"></i>
          <div class="absen-card-label">Absen Masuk</div>
          <div class="absen-card-status">
            <Tag v-if="sudahMasuk" severity="success" value="Sudah absen masuk" />
            <Tag v-else-if="masukTertutup" severity="danger" value="Absen masuk ditutup" />
            <span v-else-if="!canMasuk" class="text-muted">belum dibuka / tidak aktif</span>
          </div>
          <Button
            :label="sudahMasuk ? 'Sudah Absen Masuk' : masukTertutup ? 'Absen Masuk Ditutup' : 'Absen Masuk'"
            :icon="sudahMasuk ? 'pi pi-check' : masukTertutup ? 'pi pi-lock' : 'pi pi-camera'"
            :disabled="!canMasuk"
            @click="openCamera('masuk')"
          />
          <div v-if="todayRow?.jam_masuk" class="absen-card-detail">
            Jam masuk: {{ formatJam(todayRow.jam_masuk) }}
            <span v-if="todayRow.terlambat_menit > 0" class="text-danger"> (terlambat {{ todayRow.terlambat_menit }} menit)</span>
            <Tag v-if="todayRow.dinas_dalam_masuk" severity="info" value="Dinas Dalam" style="margin-left: 4px" />
          </div>
        </div>
        <div class="absen-card">
          <i class="pi pi-sign-out" style="font-size: 1.8rem; color: #dc2626"></i>
          <div class="absen-card-label">Absen Pulang</div>
          <div class="absen-card-status">
            <Tag v-if="sudahPulang" severity="success" value="Sudah absen pulang" />
            <span v-else-if="!sudahMasuk" class="text-muted">menunggu absen masuk</span>
            <Tag v-else-if="pulangTertutup" severity="danger" value="Absen pulang ditutup" />
            <span v-else-if="!canPulang" class="text-muted">belum dibuka / tidak aktif</span>
          </div>
          <Button
            :label="sudahPulang ? 'Sudah Absen Pulang' : pulangTertutup ? 'Absen Pulang Ditutup' : 'Absen Pulang'"
            :icon="sudahPulang ? 'pi pi-check' : pulangTertutup ? 'pi pi-lock' : 'pi pi-camera'"
            severity="danger"
            :disabled="!canPulang"
            @click="openCamera('pulang')"
          />
          <div v-if="todayRow?.jam_pulang" class="absen-card-detail">
            Jam pulang: {{ formatJam(todayRow.jam_pulang) }}
            <Tag v-if="todayRow.dinas_dalam_pulang" severity="info" value="Dinas Dalam" style="margin-left: 4px" />
          </div>
        </div>
      </div>

      <div class="section">
        <div class="section-header">
          <h3>Riwayat Absen</h3>
          <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        </div>
        <p v-if="riwayat?.total_hari_kerja" class="total-hari-kerja-info">
          Jumlah hari kerja bulan ini: <strong>{{ riwayat.total_hari_kerja }} hari</strong>
          <span v-if="isSekolahSaya"> (Senin&ndash;Sabtu, Minggu &amp; tanggal merah tidak dihitung)</span>
          <span v-else> (Senin&ndash;Jumat, Sabtu-Minggu &amp; tanggal merah tidak dihitung)</span>
        </p>
        <DataTable :value="riwayat.absensi" :loading="loadingRiwayat" size="small" stripedRows responsiveLayout="scroll">
          <Column header="No" style="width: 3rem">
            <template #body="{ index }">{{ index + 1 }}</template>
          </Column>
          <Column field="tanggal" header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
          </Column>
          <Column header="Status">
            <template #body="{ data }">
              <Tag v-if="data.dinas_dalam_masuk || data.dinas_dalam_pulang" severity="info" value="Dinas Dalam" />
              <Tag v-else-if="data.jam_masuk" severity="success" value="Hadir" />
              <span v-else>-</span>
            </template>
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
          <div v-for="(tgl, idx) in riwayat.tanggal_terlewat" :key="tgl" class="terlewat-item">
            <span class="terlewat-info">
              <span class="terlewat-no">{{ idx + 1 }}.</span>
              <span>{{ formatTanggal(tgl) }}</span>
            </span>
            <Tag severity="warn" value="Tidak melakukan absensi" />
          </div>
        </div>
      </div>

      <div v-if="isSekolahSaya" class="section">
        <h3>Ajukan Surat Kolektif</h3>
        <Message severity="info" :closable="false">
          Khusus pegawai bertugas di sekolah: ajukan surat (Surat Tugas/Berita Acara/Surat Izin/SKS/dst) untuk
          beberapa tanggal terlewat sekaligus. Pengajuan menunggu persetujuan administrator/admin verifikasi --
          absen Anda baru berubah jadi bersurat setelah disetujui.
        </Message>
        <div class="kolektif-self-form">
          <div class="field">
            <label>Tanggal Terlewat</label>
            <MultiSelect
              v-model="kolektifSelfForm.tanggal"
              :options="opsiTanggalKolektif"
              optionLabel="label"
              optionValue="value"
              placeholder="Pilih satu atau beberapa tanggal"
              display="chip"
              style="width: 100%"
            />
            <small v-if="!opsiTanggalKolektif.length" class="text-muted">Tidak ada tanggal terlewat pada bulan yang sedang dilihat.</small>
          </div>
          <div class="field">
            <label>Jenis Surat</label>
            <Select
              v-model="kolektifSelfForm.jenis"
              :options="jenisSuratOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="Pilih jenis surat"
              filter
              style="width: 100%"
            />
          </div>
          <div class="field">
            <label>Berkas (PDF/JPG/PNG)</label>
            <input ref="kolektifSelfFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onKolektifSelfFileChosen" />
            <Button
              :label="kolektifSelfFile ? kolektifSelfFile.name : 'Pilih Berkas'"
              icon="pi pi-file"
              severity="secondary"
              outlined
              @click="pickKolektifSelfFile"
            />
          </div>
          <div class="field">
            <label>Keterangan (opsional)</label>
            <Select
              v-model="kolektifSelfKeteranganPilihan"
              :options="KETERANGAN_SURAT_DROPDOWN"
              placeholder="Pilih keterangan"
              showClear
              style="width: 100%"
            />
            <Textarea
              v-if="kolektifSelfKeteranganPilihan === KETERANGAN_LAINNYA"
              v-model="kolektifSelfKeteranganLainnya"
              rows="2"
              placeholder="Isi keterangan sesuai surat"
              style="width: 100%; margin-top: 0.5rem"
            />
          </div>
          <Button label="Ajukan" icon="pi pi-send" :loading="submittingKolektifSelf" @click="submitKolektifSelf" />
        </div>

        <div v-if="pengajuanSayaList.length" class="pengajuan-saya-list">
          <h4>Pengajuan Saya</h4>
          <DataTable :value="pengajuanSayaList" size="small" stripedRows responsiveLayout="scroll">
            <Column header="Tanggal">
              <template #body="{ data }">{{ data.tanggal_list.map(formatTanggal).join(', ') }}</template>
            </Column>
            <Column field="label" header="Jenis Surat" />
            <Column header="Status">
              <template #body="{ data }">
                <Tag :severity="statusPengajuanSeverity(data.status)" :value="statusPengajuanLabel(data.status)" />
              </template>
            </Column>
            <Column header="Catatan Verifikasi">
              <template #body="{ data }">{{ data.catatan_verifikasi || '-' }}</template>
            </Column>
            <Column header="Aksi">
              <template #body="{ data }">
                <Button icon="pi pi-download" size="small" text rounded title="Unduh berkas" @click="downloadPengajuanFile(data)" />
                <Button
                  v-if="data.status === 'dikembalikan'"
                  icon="pi pi-pencil"
                  size="small"
                  text
                  rounded
                  title="Edit & ajukan ulang"
                  @click="bukaEditPengajuan(data)"
                />
              </template>
            </Column>
          </DataTable>
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
          <!-- data.kode dikirim backend (absensiDokumenSayaOut) -- kalau
               tetap kosong/"-", master Jenis Surat untuk baris ini belum
               diberi Kode (administrator perlu melengkapinya di menu Master
               Data -> Jenis Surat). -->
          <Column header="Kode">
            <template #body="{ data }">
              <Tag v-if="data.kode" :value="data.kode" :title="data.label" />
              <span v-else class="text-muted">-</span>
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

      <template v-else>
        <div class="dinas-dalam-toggle">
          <Checkbox v-model="dinasDalam" inputId="dinasDalamChk" binary />
          <label for="dinasDalamChk">
            Dinas Dalam <span class="text-muted">(boleh absen dari luar kantor -- rekap akan tertulis "Dinas Dalam", bukan "Hadir")</span>
          </label>
        </div>

        <Message v-if="locationBlocked" severity="warn" :closable="false">{{ locationBlockedMsg }}</Message>
      </template>

      <template v-if="!locationChecking && !locationBlocked">
        <Message v-if="cameraError" severity="error" :closable="false">{{ cameraError }}</Message>
        <Message v-else-if="blink.modelsError.value" severity="warn" :closable="false">
          {{ blink.modelsError.value }} -- deteksi kedipan otomatis tidak tersedia, gunakan tombol "Ambil Foto" secara manual.
        </Message>

        <div class="camera-box">
          <div v-if="!capturedUrl" class="video-wrap">
            <video ref="videoEl" autoplay playsinline muted class="camera-video"></video>
            <div class="camera-overlay">
              <span v-if="!blink.faceDetected.value && blink.lowLight.value">
                Wajah belum terdeteksi -- cahaya kurang terang, coba hadapkan wajah ke sumber cahaya atau nyalakan lampu
              </span>
              <span v-else-if="!blink.faceDetected.value">Arahkan wajah ke kamera...</span>
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

    <!-- ================= dialog edit & ajukan ulang pengajuan surat kolektif ================= -->
    <Dialog
      v-model:visible="editPengajuanDialog"
      modal
      header="Edit &amp; Ajukan Ulang"
      :style="{ width: '480px' }"
      :breakpoints="{ '640px': '94vw' }"
      @hide="closeEditPengajuan"
    >
      <Message v-if="editPengajuanItem?.catatan_verifikasi" severity="warn" :closable="false" style="margin-bottom: 1rem">
        Catatan dari verifikator: {{ editPengajuanItem.catatan_verifikasi }}
      </Message>
      <div class="field">
        <label>Tanggal Terlewat</label>
        <MultiSelect
          v-model="editPengajuanForm.tanggal"
          :options="opsiTanggalKolektif"
          optionLabel="label"
          optionValue="value"
          placeholder="Pilih satu atau beberapa tanggal"
          display="chip"
          style="width: 100%"
        />
      </div>
      <div class="field">
        <label>Jenis Surat</label>
        <Select
          v-model="editPengajuanForm.jenis"
          :options="jenisSuratOptions"
          optionLabel="label"
          optionValue="value"
          placeholder="Pilih jenis surat"
          filter
          style="width: 100%"
        />
      </div>
      <div class="field">
        <label>Berkas Baru (opsional -- kosongkan untuk memakai berkas lama)</label>
        <input ref="editPengajuanFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onEditPengajuanFileChosen" />
        <Button
          :label="editPengajuanFile ? editPengajuanFile.name : editPengajuanItem?.nama_file || 'Pilih Berkas'"
          icon="pi pi-file"
          severity="secondary"
          outlined
          @click="pickEditPengajuanFile"
        />
      </div>
      <div class="field">
        <label>Keterangan (opsional)</label>
        <Select
          v-model="editPengajuanKeteranganPilihan"
          :options="KETERANGAN_SURAT_DROPDOWN"
          placeholder="Pilih keterangan"
          showClear
          style="width: 100%"
        />
        <Textarea
          v-if="editPengajuanKeteranganPilihan === KETERANGAN_LAINNYA"
          v-model="editPengajuanKeteranganLainnya"
          rows="2"
          placeholder="Isi keterangan sesuai surat"
          style="width: 100%; margin-top: 0.5rem"
        />
      </div>
      <template #footer>
        <Button label="Batal" severity="secondary" text @click="closeEditPengajuan" />
        <Button label="Ajukan Ulang" icon="pi pi-send" :loading="submittingEditPengajuan" @click="submitEditPengajuan" />
      </template>
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
  border-color: #0d9488;
}
.koordinat-cell {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.koordinat-link {
  color: #0f766e;
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
.total-hari-kerja-info {
  margin: -0.25rem 0 0.75rem;
  font-size: 0.85rem;
  color: var(--p-text-muted-color);
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
.terlewat-info {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}
.terlewat-no {
  color: var(--p-text-muted-color, #64748b);
  min-width: 1.5rem;
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
.dinas-dalam-toggle {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
  margin-bottom: 0.75rem;
}
.dinas-dalam-toggle label {
  font-size: 0.85rem;
  line-height: 1.3;
  cursor: pointer;
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
.kolektif-self-form {
  max-width: 480px;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 1rem 1.25rem;
  margin-top: 0.75rem;
}
.pengajuan-saya-list {
  margin-top: 1.5rem;
}
.pengajuan-saya-list h4 {
  margin: 0 0 0.5rem;
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
