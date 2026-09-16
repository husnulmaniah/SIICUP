<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { toApiDate } from '../utils/date'
import { useAuthStore } from '../stores/auth'

import Button from 'primevue/button'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import MultiSelect from 'primevue/multiselect'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'

const toast = useToast()
const confirm = useConfirm()
const auth = useAuthStore()

// jenis dokumen -> kode singkat, mengikuti models.AbsensiDokumenKode di
// backend (Surat Tugas & Berita Acara sama-sama dibaca DD/Dinas Dalam).
const JENIS_KODE = { sks: 'S', surat_tugas: 'DD', berita_acara: 'DD', surat_izin: 'I' }

// ============================================================
// pengaturan (aktif/nonaktif, jendela waktu, siapa yang boleh absen, &
// titik koordinat kantor)
// ============================================================

const pengaturan = reactive({
  aktif: true,
  jam_mulai_pagi: '',
  jam_batas_pagi: '',
  jam_tutup_pagi: '',
  jam_mulai_pulang: '',
  jam_tutup_pulang: '',
  jam_mulai_pagi_sekolah: '',
  jam_batas_pagi_sekolah: '',
  jam_tutup_pagi_sekolah: '',
  jam_mulai_pulang_sekolah: '',
  jam_tutup_pulang_sekolah: '',
  tempat_tugas_allowed: [],
  jabatan_allowed_ids: [],
  kecamatan_allowed_ids: [],
  kantor_lat: null,
  kantor_lng: null,
  radius_meter: 20,
})
const loadingPengaturan = ref(true)
const savingPengaturan = ref(false)
const locatingKantor = ref(false)

function ambilLokasiKantor() {
  if (!navigator.geolocation) {
    toast.add({ severity: 'warn', summary: 'Tidak didukung', detail: 'Perangkat/browser ini tidak mendukung deteksi lokasi', life: 4000 })
    return
  }
  locatingKantor.value = true
  navigator.geolocation.getCurrentPosition(
    (pos) => {
      pengaturan.kantor_lat = Number(pos.coords.latitude.toFixed(6))
      pengaturan.kantor_lng = Number(pos.coords.longitude.toFixed(6))
      locatingKantor.value = false
      toast.add({ severity: 'success', summary: 'Lokasi ditemukan', detail: `Akurasi ±${Math.round(pos.coords.accuracy)}m -- jangan lupa Simpan Pengaturan`, life: 4000 })
    },
    () => {
      locatingKantor.value = false
      toast.add({ severity: 'error', summary: 'Gagal mendeteksi lokasi', detail: 'Izinkan akses lokasi pada browser ini', life: 4000 })
    },
    { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 },
  )
}
function hapusLokasiKantor() {
  pengaturan.kantor_lat = null
  pengaturan.kantor_lng = null
}

// ------------------------------------------------------------
// Tempel koordinat/link Google Maps -- alternatif dari "Ambil Lokasi Saat
// Ini" (yang mensyaratkan administrator SEDANG BERADA di titik kantor
// dengan GPS aktif). Dengan ini administrator bisa menentukan Titik Kantor
// langsung dari Google Maps di komputer manapun: cari gedung kantornya di
// Google Maps, klik kanan titik yang tepat pada gedung lalu pilih koordinat
// yang muncul di menu (atau salin dari address bar), lalu tempel di sini.
// Koordinat dari Google Maps ini juga lebih presisi & stabil dibanding GPS
// ponsel (yang sering meleset 50-150m di dalam gedung), sehingga radius
// yang wajar (mis. 100-200m) sudah cukup menjangkau SELURUH gedung kantor
// walau titik yang dipilih bukan pas di tengah gedung.
// ------------------------------------------------------------
const tempelKoordinat = ref('')

// parseKoordinatMaps mengekstrak (lat, lng) dari teks yang ditempel dari
// Google Maps. Pola dicoba dari yang paling presisi ke paling umum, supaya
// kalau sebuah link Google Maps memuat lebih dari satu pola sekaligus, yang
// dipakai adalah titik PIN/tempat sebenarnya, bukan sekadar titik tengah
// tampilan peta saat itu:
//   1. "!3d<lat>!4d<lng>"  -- titik pin tempat (place) pada link ".../place/...@lat,lng,zoom/data=...!3d<lat>!4d<lng>..."
//   2. "?q=<lat>,<lng>"    -- link berbagi lokasi ("maps?q=lat,lng")
//   3. "@<lat>,<lng>"      -- titik tengah tampilan peta pada address bar ("maps/@lat,lng,zoomm")
//   4. "<lat>, <lng>"      -- koordinat polos hasil klik-kanan > salin koordinat pada peta
function parseKoordinatMaps(text) {
  const s = (text || '').trim()
  if (!s) return null

  const cobaPola = (regex) => {
    const m = s.match(regex)
    if (!m) return null
    const lat = Number(m[1])
    const lng = Number(m[2])
    if (Number.isNaN(lat) || Number.isNaN(lng)) return null
    if (lat < -90 || lat > 90 || lng < -180 || lng > 180) return null
    return { lat, lng }
  }

  return (
    cobaPola(/!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)/) ||
    cobaPola(/[?&]q=(-?\d+(?:\.\d+)?),\s*(-?\d+(?:\.\d+)?)/) ||
    cobaPola(/@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)/) ||
    cobaPola(/^(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)$/)
  )
}

function terapkanTempelKoordinat() {
  const hasil = parseKoordinatMaps(tempelKoordinat.value)
  if (!hasil) {
    toast.add({
      severity: 'error',
      summary: 'Format tidak dikenali',
      detail: 'Tempel koordinat (contoh: "-1.976688, 121.335284") atau link Google Maps yang mengandung koordinat, lalu klik Terapkan.',
      life: 6000,
    })
    return
  }
  pengaturan.kantor_lat = Number(hasil.lat.toFixed(6))
  pengaturan.kantor_lng = Number(hasil.lng.toFixed(6))
  tempelKoordinat.value = ''
  toast.add({
    severity: 'success',
    summary: 'Koordinat diterapkan',
    detail: `${pengaturan.kantor_lat}, ${pengaturan.kantor_lng} -- jangan lupa klik Simpan Pengaturan di bawah.`,
    life: 5000,
  })
}

const tempatTugasOptions = ref([])
const jabatanOptions = ref([])
const kecamatanOptions = ref([])

async function loadPengaturan() {
  loadingPengaturan.value = true
  try {
    const { data } = await http.get('/absensi/pengaturan')
    Object.assign(pengaturan, data.data)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan absen', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingPengaturan.value = false
  }
}

async function loadTempatTugasOptions() {
  try {
    const { data } = await http.get('/absensi/opsi-tempat-tugas')
    tempatTugasOptions.value = (data.data || []).map((t) => ({ label: t, value: t }))
  } catch {
    tempatTugasOptions.value = []
  }
}

async function loadJabatanOptions() {
  try {
    const { data } = await http.get('/ref/jabatan')
    jabatanOptions.value = (data.data || []).map((j) => ({ label: j.jabatan, value: j.id }))
  } catch {
    jabatanOptions.value = []
  }
}

async function loadKecamatanOptions() {
  try {
    const { data } = await http.get('/ref/kecamatan')
    kecamatanOptions.value = (data.data || []).map((k) => ({ label: k.nama, value: k.id }))
  } catch {
    kecamatanOptions.value = []
  }
}

const jamPattern = /^([01]\d|2[0-3]):[0-5]\d$/
function jamValid(v) {
  return jamPattern.test(v || '')
}
const pengaturanValid = computed(
  () =>
    jamValid(pengaturan.jam_mulai_pagi) &&
    jamValid(pengaturan.jam_batas_pagi) &&
    jamValid(pengaturan.jam_tutup_pagi) &&
    jamValid(pengaturan.jam_mulai_pulang) &&
    jamValid(pengaturan.jam_tutup_pulang) &&
    jamValid(pengaturan.jam_mulai_pagi_sekolah) &&
    jamValid(pengaturan.jam_batas_pagi_sekolah) &&
    jamValid(pengaturan.jam_tutup_pagi_sekolah) &&
    jamValid(pengaturan.jam_mulai_pulang_sekolah) &&
    jamValid(pengaturan.jam_tutup_pulang_sekolah),
)

// -- Perkiraan jangkauan absen efektif --
// Radius yang diatur di sini BUKAN satu-satunya jarak yang dipakai untuk
// menerima/menolak absen -- backend & AbsensiView.vue selalu menambahkan
// toleransi otomatis (30-50m) di atas radius ini untuk menutupi noise GPS
// (lihat toleransiAkurasiMinimum/Maksimal di backend/handlers/absensi.go).
// Jadi radius 50m sebenarnya bisa menerima absen dari sekitar 80-100m, radius
// 350m bisa menerima dari 380-400m (sudah menjangkau luar area kantor,
// termasuk kemungkinan rumah pegawai terdekat). Ditampilkan di sini supaya
// administrator TIDAK memperbesar radius hanya untuk mengakali GPS yang
// kurang akurat -- toleransi itu sudah otomatis, radius cukup diisi sesuai
// luas gedung/halaman kantor sebenarnya (bisa diukur pakai fitur "Ukur
// jarak" di Google Maps).
const TOLERANSI_MIN = 30
const TOLERANSI_MAKS = 50
const jangkauanEfektif = computed(() => {
  const r = Number(pengaturan.radius_meter) || 0
  if (r <= 0) return null
  return { min: r + TOLERANSI_MIN, maks: r + TOLERANSI_MAKS }
})
const radiusTerlaluBesar = computed(() => Number(pengaturan.radius_meter) > 150)

async function savePengaturan() {
  if (!pengaturanValid.value) {
    toast.add({ severity: 'warn', summary: 'Periksa kembali', detail: 'Semua jam wajib berformat HH:MM (contoh: 07:30)', life: 4000 })
    return
  }
  savingPengaturan.value = true
  try {
    const { data } = await http.put('/absensi/pengaturan', { ...pengaturan })
    Object.assign(pengaturan, data.data)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    savingPengaturan.value = false
  }
}

// ============================================================
// rekap semua pegawai
// ============================================================

const periodDate = ref(new Date())
const pegawaiOptions = ref([])
const selectedPegawai = ref(null)
const rekap = ref([])
const loadingRekap = ref(false)

// Paginasi ala DataTables (dropdown "Tampilkan N entri" + info "Showing X to
// Y of Z entries" + nomor halaman bulat) -- dipakai konsisten di semua tabel
// pada aplikasi ini (lihat CrudManager.vue/PengajuanCutiView.vue). Di sini
// dilakukan di sisi BROWSER (bukan lazy ke server) karena /absensi/rekap
// sudah mengembalikan seluruh pegawai untuk bulan yang dipilih sekaligus --
// PrimeVue DataTable otomatis menghitung totalRecords dari panjang data saat
// paginator tidak "lazy".
const entriesOptions = [5, 10, 25, 50, 100]
const rekapPageSize = ref(10)
const rekapFirst = ref(0)

// ------------------------------------------------------------
// daftar pegawai untuk filter rekap & pilihan pegawai pada input surat
// kolektif.
//
// Instansi bisa punya ribuan pegawai sementara endpoint /pegawai dibatasi
// maksimal 500 baris per permintaan, jadi daftar TIDAK bisa dimuat sekaligus
// lalu disaring di browser -- pegawai yang urutan namanya di atas batas itu
// tidak akan pernah muncul saat dicari. Karena itu pencarian dilakukan di
// SERVER: setiap kali admin mengetik di kotak cari, daftar opsi diambil ulang
// pakai parameter q (cocok dengan nama ATAU NIP, lihat listPegawai di
// handlers/pegawai.go).
// ------------------------------------------------------------

const PEGAWAI_PAGE_SIZE = 100
const loadingPegawai = ref(false)
// pegawaiTotal = jumlah SELURUH pegawai (hanya diperbarui saat memuat tanpa
// kata kunci), pegawaiHasil = jumlah yang cocok dengan pencarian terakhir.
const pegawaiTotal = ref(0)
const pegawaiHasil = ref(0)
const pegawaiKeyword = ref('')
// opsi pegawai yang sedang dipilih disimpan terpisah supaya labelnya tetap
// ada saat daftar opsi diganti hasil pencarian baru (kalau hilang, chip
// pilihan berubah jadi kosong).
const pegawaiTerpilihCache = ref([])

function toPegawaiOption(p) {
  // tempat_tgs ikut dibawa karena menentukan pola hari kerja pegawai
  // (lihat hariNonaktif) saat memilih rentang tanggal surat kolektif
  return { label: `${p.nama} (${p.nip})`, value: p.id, tempat_tgs: p.tempat_tgs || '' }
}

function gabungDenganTerpilih(list) {
  const adaDiHasil = new Set(list.map((o) => o.value))
  return [...pegawaiTerpilihCache.value.filter((o) => !adaDiHasil.has(o.value)), ...list]
}

async function loadPegawaiOptions(keyword = '') {
  loadingPegawai.value = true
  try {
    const params = { pageSize: PEGAWAI_PAGE_SIZE }
    const q = (keyword || '').trim()
    pegawaiKeyword.value = q
    if (q) params.q = q
    const { data } = await http.get('/pegawai', { params })
    const list = Array.isArray(data.data) ? data.data : []
    pegawaiHasil.value = data.meta?.total ?? list.length
    if (!q) pegawaiTotal.value = pegawaiHasil.value
    pegawaiOptions.value = gabungDenganTerpilih(list.map(toPegawaiOption))
  } catch {
    pegawaiOptions.value = gabungDenganTerpilih([])
  } finally {
    loadingPegawai.value = false
  }
}

// pencarian ditunda sebentar supaya tidak menembak server tiap ketikan
let pegawaiFilterTimer = null
function onPegawaiFilter(event) {
  const keyword = event?.value || ''
  clearTimeout(pegawaiFilterTimer)
  pegawaiFilterTimer = setTimeout(() => loadPegawaiOptions(keyword), 300)
}

// ingatOpsiTerpilih dipanggil setiap pilihan pegawai berubah (lihat watch di
// bagian input surat kolektif, setelah kolektifForm dideklarasikan).
function ingatOpsiTerpilih(idList) {
  const terpilih = idList.filter((v) => v != null)
  const dikenal = new Map([...pegawaiTerpilihCache.value, ...pegawaiOptions.value].map((o) => [o.value, o]))
  pegawaiTerpilihCache.value = [...new Set(terpilih)].map((id) => dikenal.get(id)).filter(Boolean)
}

async function loadRekap() {
  loadingRekap.value = true
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    if (selectedPegawai.value) params.id_pegawai = selectedPegawai.value
    const { data } = await http.get('/absensi/rekap', { params })
    rekap.value = data.data?.data || []
    rekapFirst.value = 0
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat rekap', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingRekap.value = false
  }
}

watch([periodDate, selectedPegawai], () => loadRekap())
watch(periodDate, () => loadDokumenAdmin())

onMounted(async () => {
  // kartu Pengaturan (filter siapa yang boleh absen, titik koordinat
  // kantor) HANYA untuk role administrator -- role admin/Admin Absensi
  // tidak boleh melihat maupun mengubahnya (lihat v-if pada kartu
  // Pengaturan di template, dan pembatasan yang sama di backend: PUT
  // /absensi/pengaturan & GET /absensi/opsi-tempat-tugas sekarang
  // administratorOnly), jadi loadTempatTugasOptions/loadJabatanOptions
  // tetap hanya untuk administrator.
  //
  // Tapi loadPengaturan() (GET jendela waktu absen, termasuk
  // jam_tutup_pulang) dimuat untuk SEMUA role yang bisa membuka halaman
  // ini -- endpoint-nya memang terbuka untuk siapa saja yang login (lihat
  // anyRole di GET /absensi/pengaturan), dan nilainya juga dipakai tabel
  // Riwayat Absen untuk menentukan kapan jendela absen pulang sudah
  // otomatis DITUTUP (lihat absenPulangSudahTutup), bukan cuma dipakai
  // kartu Pengaturan.
  const tugasAdminOnly = auth.isAdministrator ? [loadTempatTugasOptions(), loadJabatanOptions(), loadKecamatanOptions()] : []
  await Promise.all([loadPegawaiOptions(), loadPengaturan(), ...tugasAdminOnly])
  await Promise.all([loadRekap(), loadDokumenAdmin()])
})

// Hadir TIDAK termasuk hari yang ditandai Dinas Dalam (baik absen masuk
// maupun pulangnya) -- hari itu sudah dihitung terpisah lewat jumlah_dd
// (lihat buildRekapItems di backend, yang menggabungkan Dinas Dalam mandiri
// pegawai dengan dokumen Surat Tugas/Berita Acara yang diinput admin).
// Juga TIDAK termasuk hari yang sudah berubah status jadi "Tidak Absen
// Pulang" (lihat tidakAbsenPulang) -- hari itu dianggap belum lengkap
// absennya, jadi tidak lagi dihitung sebagai Hadir penuh.
function jumlahHadir(item) {
  return item.absensi.filter(
    (a) => a.jam_masuk && !a.dinas_dalam_masuk && !a.dinas_dalam_pulang && !tidakAbsenPulang(a),
  ).length
}
function jumlahTerlambat(item) {
  return item.absensi.filter((a) => a.terlambat_menit > 0).length
}

async function exportExcel() {
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    if (selectedPegawai.value) params.id_pegawai = selectedPegawai.value
    const res = await http.get('/absensi/rekap/export', { params, responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = url
    const bulanStr = String(params.bulan).padStart(2, '0')
    link.download = `rekap_absensi_${params.tahun}-${bulanStr}.xlsx`
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

// ============================================================
// detail per pegawai (dialog)
// ============================================================

const detailDialog = ref(false)
const detailItem = ref(null)

function openDetail(item) {
  detailItem.value = item
  detailDialog.value = true
  loadThumbnails(item.absensi)
}

// ------------------------------------------------------------
// unduh rekap absen SATU pegawai sebagai PDF (lihat
// exportRekapAbsensiPegawaiPDF di handlers/absensi_pdf.go)
// ------------------------------------------------------------

const unduhPdfId = ref(null)

// pesan error dari backend ikut terbaca walaupun respons diminta sebagai blob
async function pesanErrorBlob(e) {
  const data = e.response?.data
  if (data instanceof Blob) {
    try {
      return JSON.parse(await data.text()).message
    } catch {
      /* bukan JSON -- pakai pesan bawaan di bawah */
    }
  }
  return e.response?.data?.message || e.message
}

async function unduhPdfPegawai(item) {
  const pegawai = item?.pegawai
  if (!pegawai?.id) return
  unduhPdfId.value = pegawai.id
  try {
    const params = {
      id_pegawai: pegawai.id,
      bulan: periodDate.value.getMonth() + 1,
      tahun: periodDate.value.getFullYear(),
    }
    const res = await http.get('/absensi/rekap/pdf', { params, responseType: 'blob' })
    const url = URL.createObjectURL(new Blob([res.data], { type: 'application/pdf' }))
    const link = document.createElement('a')
    link.href = url
    const namaBersih = (pegawai.nama || 'pegawai').replace(/[^A-Za-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
    const nipBersih = (pegawai.nip || '-').toString().trim() || '-'
    link.download = `${nipBersih}-${namaBersih}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh PDF', detail: await pesanErrorBlob(e), life: 5000 })
  } finally {
    unduhPdfId.value = null
  }
}
function dateKey(iso) {
  return (iso || '').slice(0, 10)
}
function formatTanggal(key) {
  const [y, m, d] = key.split('-')
  return `${d}-${m}-${y}`
}
function formatJam(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' })
}

// ============================================================
// tandai baris "Tidak Absen Pulang (TAP)": sudah absen masuk tapi sampai
// jendela absen pulang DITUTUP tidak pernah absen pulang. Memakai jam
// tutup yang sesungguhnya (pengaturan.jam_tutup_pulang, mis. "20:00")
// dan zona waktu kantor (WITA / Asia/Makassar) -- sama persis dengan
// aturan yang dipakai backend untuk menolak absen pulang setelah lewat
// JamTutupPulang (lihat handlers/absensi.go) -- BUKAN sekadar "sudah
// ganti hari kalender", supaya untuk hari ini pun begitu jendela absen
// pulang tutup (mis. jam 20:00), pegawai yang belum absen pulang langsung
// berubah statusnya, tidak perlu menunggu sampai besok.
//
// Dipakai zona waktu kantor (bukan zona waktu perangkat admin yang
// sedang melihat rekap) supaya hasilnya konsisten di mana pun admin
// mengakses halaman ini.
const waktuKantorFmt = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Makassar',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})
function waktuKantorSekarang() {
  const parts = Object.fromEntries(waktuKantorFmt.formatToParts(new Date()).map((p) => [p.type, p.value]))
  return { tanggal: `${parts.year}-${parts.month}-${parts.day}`, jam: `${parts.hour}:${parts.minute}` }
}
function absenPulangSudahTutup(data) {
  const tglAbsen = dateKey(data.tanggal)
  const skrg = waktuKantorSekarang()
  if (tglAbsen < skrg.tanggal) return true // sudah ganti hari -- jendela pasti sudah tutup
  if (tglAbsen > skrg.tanggal) return false // tanggal masa depan, seharusnya tidak terjadi
  // hari yang sama: baru dianggap tutup begitu jam sekarang melewati jam
  // tutup pulang pada pengaturan. Kalau pengaturan belum termuat (kosong),
  // jangan buru-buru menandai TAP.
  if (!pengaturan.jam_tutup_pulang) return false
  return skrg.jam >= pengaturan.jam_tutup_pulang
}
function tidakAbsenPulang(data) {
  if (data.dinas_dalam_masuk || data.dinas_dalam_pulang) return false
  if (!data.jam_masuk || data.jam_pulang) return false
  return absenPulangSudahTutup(data)
}
function rowClassRiwayat(data) {
  return { 'row-tap': tidakAbsenPulang(data) }
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
// langsung terbuka dengan penanda pada titik itu, baik di browser maupun di
// aplikasi Google Maps pada HP.
function mapsUrl(lat, lng) {
  return `https://www.google.com/maps/search/?api=1&query=${Number(lat).toFixed(6)},${Number(lng).toFixed(6)}`
}

// koordinat absen TERAKHIR pada satu baris absensi: pakai titik absen pulang
// bila sudah ada, kalau belum pakai titik absen masuk
function koordinatTerakhir(row) {
  if (row?.lat_pulang != null && row?.lng_pulang != null) {
    return { lat: row.lat_pulang, lng: row.lng_pulang, jenis: 'pulang' }
  }
  if (row?.lat_masuk != null && row?.lng_masuk != null) {
    return { lat: row.lat_masuk, lng: row.lng_masuk, jenis: 'masuk' }
  }
  return null
}

// lokasi absen TERAKHIR seorang pegawai pada periode yang sedang dilihat --
// dipakai kolom "Lokasi Terakhir" pada tabel rekap. item.absensi sudah urut
// dari tanggal terbaru (lihat buildRekapItems di backend), jadi baris pertama
// yang punya koordinat adalah absen terakhirnya.
function lokasiTerakhirPegawai(item) {
  for (const a of item?.absensi || []) {
    const k = koordinatTerakhir(a)
    if (k) return { ...k, tanggal: dateKey(a.tanggal) }
  }
  return null
}

// ============================================================
// foto absen masuk/pulang (admin/administrator) -- thumbnail langsung di
// tabel detail, klik untuk memperbesar
// ============================================================

// Foto hanya bisa diambil dengan token (bukan URL publik), jadi ditarik
// sebagai blob lalu disimpan object URL-nya dengan kunci "<id>-masuk/pulang".
// Backend mengecilkan foto lewat ?w=96 supaya satu bulan penuh foto tetap
// ringan dimuat (lihat resizeJPEG di handlers/absensi.go).
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

onBeforeUnmount(() => revokeThumbnails())

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
// input surat kolektif (BA/Surat Tugas/Surat Izin/SKS) untuk beberapa
// pegawai & rentang tanggal sekaligus -- lihat inputAbsensiDokumenKolektif
// di handlers/absensi_dokumen.go
// ============================================================

const kolektifForm = reactive({
  id_pegawai: [],
  tanggal_mulai: new Date(),
  tanggal_selesai: new Date(),
  jenis: null,
  keterangan: '',
})
const kolektifFile = ref(null)
const kolektifFileInput = ref(null)
const submittingKolektif = ref(false)

// pegawai yang sedang dipilih (di filter rekap maupun di form kolektif)
// diingat labelnya supaya tetap tampil walau daftar opsi berganti karena
// pencarian baru -- lihat komentar pada loadPegawaiOptions.
watch(
  () => [selectedPegawai.value, ...kolektifForm.id_pegawai],
  (ids) => ingatOpsiTerpilih(ids),
)

// ------------------------------------------------------------
// pola hari kerja pegawai yang dipilih -> hari apa saja yang tidak bisa
// dipilih pada rentang tanggal surat.
//
// Aturannya sama dengan sixDayWeekForTempatTgs di backend: pegawai yang
// bertugas di sekolah masuk Senin-Sabtu (hanya Minggu libur), pegawai kantor
// dinas masuk Senin-Jumat (Sabtu & Minggu libur). Kalau pegawai yang dipilih
// bercampur, hanya Minggu yang dimatikan di kalender -- tanggal Sabtu tetap
// bisa dipilih untuk pegawai sekolah, dan backend yang melewati tanggal
// Sabtu itu khusus untuk pegawai kantor dinas (lihat
// inputAbsensiDokumenKolektif).
// ------------------------------------------------------------

function polaSekolah(tempatTgs) {
  return (tempatTgs || '').toLowerCase().includes('sekolah')
}

const pegawaiKolektifTerpilih = computed(() =>
  kolektifForm.id_pegawai
    .map(
      (id) =>
        pegawaiOptions.value.find((o) => o.value === id) || pegawaiTerpilihCache.value.find((o) => o.value === id),
    )
    .filter(Boolean),
)

const adaPegawaiSekolah = computed(() => pegawaiKolektifTerpilih.value.some((o) => polaSekolah(o.tempat_tgs)))
const semuaPegawaiDinas = computed(
  () => pegawaiKolektifTerpilih.value.length > 0 && !adaPegawaiSekolah.value,
)
// 0 = Minggu, 6 = Sabtu
const hariNonaktif = computed(() => (semuaPegawaiDinas.value ? [0, 6] : [0]))

function pickKolektifFile() {
  kolektifFileInput.value?.click()
}
function onKolektifFileChosen(e) {
  kolektifFile.value = e.target.files[0] || null
}

async function submitKolektif() {
  if (!kolektifForm.id_pegawai.length || !kolektifForm.jenis || !kolektifFile.value) {
    toast.add({ severity: 'warn', summary: 'Periksa kembali', detail: 'Pilih minimal satu pegawai, jenis surat, dan berkasnya', life: 4000 })
    return
  }
  submittingKolektif.value = true
  try {
    const fd = new FormData()
    kolektifForm.id_pegawai.forEach((id) => fd.append('id_pegawai', id))
    fd.append('tanggal_mulai', toApiDate(kolektifForm.tanggal_mulai))
    fd.append('tanggal_selesai', toApiDate(kolektifForm.tanggal_selesai || kolektifForm.tanggal_mulai))
    fd.append('jenis', kolektifForm.jenis)
    fd.append('keterangan', kolektifForm.keterangan || '')
    fd.append('file', kolektifFile.value)
    const { data } = await http.post('/absensi/dokumen/kolektif', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 5000 })
    kolektifForm.id_pegawai = []
    kolektifForm.jenis = null
    kolektifForm.keterangan = ''
    kolektifFile.value = null
    // kembalikan daftar pegawai ke keadaan awal (tanpa kata kunci pencarian)
    await Promise.all([loadRekap(), loadDokumenAdmin(), loadPegawaiOptions()])
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    submittingKolektif.value = false
  }
}

// daftar surat yang sudah diinput (bulan yang sama dengan rekap) -- supaya
// admin bisa lihat & hapus kalau salah input.
const dokumenAdminList = ref([])
const loadingDokumenAdmin = ref(false)
// Paginasi ala DataTables untuk tabel "Surat yang Sudah Diinput Bulan Ini" --
// lihat komentar pada rekapPageSize/rekapFirst di atas untuk penjelasan pola
// yang sama (client-side, karena /absensi/dokumen/rekap juga mengembalikan
// seluruh surat bulan itu sekaligus).
const dokumenPageSize = ref(10)
const dokumenFirst = ref(0)

async function loadDokumenAdmin() {
  loadingDokumenAdmin.value = true
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    const { data } = await http.get('/absensi/dokumen/rekap', { params })
    dokumenAdminList.value = data.data || []
    dokumenFirst.value = 0
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat daftar surat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingDokumenAdmin.value = false
  }
}

function confirmHapusDokumenAdmin(item) {
  confirm.require({
    message: `Hapus surat "${item.label}" untuk ${item.pegawai?.nama || 'pegawai ini'} tanggal ${formatTanggal(dateKey(item.tanggal))}?`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    accept: async () => {
      try {
        await http.delete(`/absensi/dokumen/${item.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Surat dihapus', life: 3000 })
        await Promise.all([loadRekap(), loadDokumenAdmin()])
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 4000 })
      }
    },
  })
}

function kodeDokumen(jenis) {
  return JENIS_KODE[jenis] || ''
}
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Rekap Absen</div>
    <p class="page-subtitle">
      {{
        auth.isAdministrator
          ? 'Rekap kehadiran seluruh pegawai, pengaturan jendela waktu absen, dan aktif/nonaktifkan menu Absen.'
          : 'Rekap kehadiran seluruh pegawai.'
      }}
    </p>

    <!-- Tab "Rekap Absen" (semua bisa lihat), "Pengaturan Absen" (HANYA
         administrator -- role admin tidak boleh melihat/mengubah sama
         sekali) & "Surat Kolektif" (admin & administrator), menggantikan
         tampilan lama yang menumpuk ketiganya sekaligus di satu halaman. -->
    <Tabs value="rekap">
      <TabList>
        <Tab value="rekap"><i class="pi pi-list" style="margin-right: 0.4rem"></i> Rekap Absen</Tab>
        <Tab v-if="auth.isAdministrator" value="pengaturan"><i class="pi pi-cog" style="margin-right: 0.4rem"></i> Pengaturan Absen</Tab>
        <Tab value="surat"><i class="pi pi-file" style="margin-right: 0.4rem"></i> Surat Kolektif</Tab>
      </TabList>
      <TabPanels>
    <TabPanel v-if="auth.isAdministrator" value="pengaturan">
    <div class="card">
      <div v-if="loadingPengaturan" style="display: flex; justify-content: center; padding: 1.5rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <div v-else class="pengaturan-body">
        <div class="pengaturan-head">
          <ToggleSwitch v-model="pengaturan.aktif" />
          <span>Menu Absen {{ pengaturan.aktif ? 'aktif' : 'nonaktif' }} untuk pegawai</span>
        </div>
        <Message v-if="!pengaturan.aktif" severity="warn" :closable="false">
          Pegawai tidak akan bisa absen selama menu ini nonaktif.
        </Message>

        <!-- dua kolom di layar lebar (desktop), menumpuk otomatis di HP -->
        <div class="pengaturan-cols">
          <div class="pengaturan-col">
            <div class="pengaturan-group">
              <div class="group-title">Jendela Waktu Absen - Dinas/Kantor</div>
              <div class="jam-grid">
                <div>
                  <label class="field-label">Jam Mulai Absen Pagi</label>
                  <InputText v-model="pengaturan.jam_mulai_pagi" placeholder="06:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Batas Absen Pagi (setelah ini terlambat)</label>
                  <InputText v-model="pengaturan.jam_batas_pagi" placeholder="07:30" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Tutup Absen Masuk (setelah ini otomatis ditutup)</label>
                  <InputText v-model="pengaturan.jam_tutup_pagi" placeholder="09:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Mulai Absen Pulang</label>
                  <InputText v-model="pengaturan.jam_mulai_pulang" placeholder="15:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Tutup Absen Pulang (setelah ini otomatis ditutup)</label>
                  <InputText v-model="pengaturan.jam_tutup_pulang" placeholder="20:00" style="width: 100%" />
                </div>
              </div>
              <Message severity="info" :closable="false" style="margin-top: 0.5rem">
                Berlaku untuk pegawai yang tempat tugasnya BUKAN sekolah (dinas/kantor). Absen masuk otomatis ditutup (tidak bisa lagi absen masuk maupun pulang) begitu lewat jam tutup, walaupun pegawai belum absen masuk sama sekali hari itu. Absen pulang juga otomatis ditutup begitu lewat jam tutup absen pulang, walaupun pegawai sudah absen masuk dan belum sempat absen pulang.
              </Message>
            </div>

            <div class="pengaturan-group">
              <div class="group-title">Jendela Waktu Absen - Sekolah</div>
              <div class="jam-grid">
                <div>
                  <label class="field-label">Jam Mulai Absen Pagi</label>
                  <InputText v-model="pengaturan.jam_mulai_pagi_sekolah" placeholder="06:30" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Batas Absen Pagi (setelah ini terlambat)</label>
                  <InputText v-model="pengaturan.jam_batas_pagi_sekolah" placeholder="07:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Tutup Absen Masuk (setelah ini otomatis ditutup)</label>
                  <InputText v-model="pengaturan.jam_tutup_pagi_sekolah" placeholder="08:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Mulai Absen Pulang</label>
                  <InputText v-model="pengaturan.jam_mulai_pulang_sekolah" placeholder="12:30" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Tutup Absen Pulang (setelah ini otomatis ditutup)</label>
                  <InputText v-model="pengaturan.jam_tutup_pulang_sekolah" placeholder="15:00" style="width: 100%" />
                </div>
              </div>
              <Message severity="info" :closable="false" style="margin-top: 0.5rem">
                Berlaku untuk pegawai yang tempat tugasnya mengandung kata "sekolah" (guru/staf sekolah) -- jam
                kerjanya otomatis dipakai menggantikan set Dinas/Kantor di atas untuk pegawai tersebut, mengikuti
                aturan yang sama dengan penentuan 5/6 hari kerja & syarat dokumen cuti di menu lain.
              </Message>
            </div>

            <div class="pengaturan-group">
              <div class="group-title">Titik Koordinat Kantor &amp; Radius Absen (Default)</div>
              <Message severity="info" :closable="false" style="margin-bottom: 0.75rem">
                Titik ini dipakai sebagai DEFAULT/CADANGAN untuk pegawai yang unit kerja/sekolahnya belum diberi titik
                koordinat sendiri. Untuk instansi dengan banyak sekolah di beberapa kecamatan, atur titik koordinat
                khusus per sekolah lewat menu Master Data -> Unit Kerja (titik di sana jadi prioritas utama).
              </Message>

              <label class="field-label">Tempel Koordinat / Link Google Maps</label>
              <div class="kantor-tempel">
                <InputText
                  v-model="tempelKoordinat"
                  placeholder='Contoh: -1.976688, 121.335284 (atau tempel link Google Maps)'
                  style="flex: 1 1 220px"
                  @keyup.enter="terapkanTempelKoordinat"
                />
                <Button label="Terapkan" icon="pi pi-map" size="small" @click="terapkanTempelKoordinat" />
              </div>
              <small class="text-muted" style="display: block; margin-bottom: 0.75rem">
                Cari gedung kantor di Google Maps, klik-kanan pada titik yang tepat di gedung tersebut lalu pilih
                koordinat yang muncul paling atas (atau salin link/koordinatnya), lalu tempel di atas ini.
              </small>

              <div class="kantor-grid">
                <InputNumber v-model="pengaturan.kantor_lat" placeholder="Lintang (lat)" :minFractionDigits="6" :maxFractionDigits="6" style="width: 100%" />
                <InputNumber v-model="pengaturan.kantor_lng" placeholder="Bujur (lng)" :minFractionDigits="6" :maxFractionDigits="6" style="width: 100%" />
                <InputNumber v-model="pengaturan.radius_meter" placeholder="Radius (meter)" suffix=" m" :min="1" style="width: 100%" />
              </div>
              <div class="kantor-actions">
                <Button label="Ambil Lokasi Saat Ini" icon="pi pi-map-marker" size="small" outlined :loading="locatingKantor" @click="ambilLokasiKantor" />
                <Button v-if="pengaturan.kantor_lat != null" label="Hapus Titik Kantor" icon="pi pi-times" size="small" text severity="danger" @click="hapusLokasiKantor" />
              </div>
              <small class="text-muted" style="display: block; margin-bottom: 0.5rem">
                Kalau diisi, kamera absen hanya akan terbuka jika pegawai berada dalam radius ini dari titik kantor.
                Kosongkan (Hapus Titik Kantor) untuk menonaktifkan pembatasan lokasi. Isi radius sesuai luas gedung/
                halaman kantor SEBENARNYA (mis. 50-100m -- bisa diukur pakai fitur "Ukur jarak" di Google Maps),
                bukan dibesar-besarkan untuk mengakali GPS yang kurang akurat -- sistem SUDAH otomatis menambah
                toleransi {{ TOLERANSI_MIN }}-{{ TOLERANSI_MAKS }}m di atas radius ini untuk noise GPS, jadi radius
                tidak perlu diperbesar lagi untuk alasan itu. "Ambil Lokasi Saat Ini" hanya akurat kalau Anda sedang
                berada tepat di titik kantor yang dijadikan patokan.
              </small>
              <small v-if="jangkauanEfektif" class="text-muted" style="display: block">
                Dengan radius {{ pengaturan.radius_meter }}m, absen akan diterima dari jarak sekitar
                <strong>{{ jangkauanEfektif.min }}-{{ jangkauanEfektif.maks }} meter</strong> dari titik kantor
                (radius + toleransi akurasi GPS otomatis).
              </small>
              <Message v-if="radiusTerlaluBesar" severity="warn" :closable="false" style="margin-top: 0.5rem">
                Radius {{ pengaturan.radius_meter }}m tergolong besar -- jangkauan efektifnya bisa sampai
                {{ jangkauanEfektif?.maks }}m dari titik kantor, kemungkinan sudah menjangkau luar area kantor
                (termasuk rumah pegawai yang berdekatan). Kalau tujuannya supaya pegawai di kantor tidak tertolak
                karena GPS kurang akurat, itu sudah ditangani otomatis oleh toleransi di atas -- coba kecilkan radius
                ini agar sesuai luas kantor sebenarnya.
              </Message>
            </div>
          </div>

          <div class="pengaturan-col">
            <div class="pengaturan-group">
              <div class="group-title">Siapa yang Boleh Absen</div>
              <div>
                <label class="field-label">Tempat Tugas yang Boleh Absen</label>
                <MultiSelect
                  v-model="pengaturan.tempat_tugas_allowed"
                  :options="tempatTugasOptions"
                  optionLabel="label"
                  optionValue="value"
                  filter
                  display="chip"
                  placeholder="Semua tempat tugas (belum dibatasi)"
                  style="width: 100%"
                />
                <small class="text-muted">Kosongkan untuk mengizinkan semua tempat tugas.</small>
              </div>
              <div>
                <label class="field-label">Jabatan yang Boleh Absen</label>
                <MultiSelect
                  v-model="pengaturan.jabatan_allowed_ids"
                  :options="jabatanOptions"
                  optionLabel="label"
                  optionValue="value"
                  filter
                  display="chip"
                  placeholder="Semua jabatan (belum dibatasi)"
                  style="width: 100%"
                />
                <small class="text-muted">Kosongkan untuk mengizinkan semua jabatan.</small>
              </div>
              <div>
                <label class="field-label">Kecamatan yang Boleh Absen</label>
                <MultiSelect
                  v-model="pengaturan.kecamatan_allowed_ids"
                  :options="kecamatanOptions"
                  optionLabel="label"
                  optionValue="value"
                  filter
                  display="chip"
                  placeholder="Semua kecamatan (belum dibatasi)"
                  style="width: 100%"
                />
                <small class="text-muted">
                  Dicocokkan lewat kecamatan unit kerja/sekolah pegawai (Master Data -> Unit Kerja). Kosongkan untuk
                  mengizinkan semua kecamatan. Pegawai yang unit kerjanya belum diberi kecamatan otomatis tidak lolos
                  begitu filter ini diisi.
                </small>
              </div>
              <Message severity="info" :closable="false">
                Pegawai yang tempat tugas, jabatan, atau kecamatannya tidak cocok dengan filter di atas akan melihat
                pesan "menu ini bukan untuk Anda" saat membuka menu Absen. Kosongkan ketiga filter untuk membuka menu
                Absen bagi semua pegawai.
              </Message>
            </div>
          </div>
        </div>

        <div>
          <Button label="Simpan Pengaturan" icon="pi pi-save" :loading="savingPengaturan" @click="savePengaturan" />
        </div>
      </div>
    </div>
    </TabPanel>

    <TabPanel value="rekap">
    <div class="card">
      <div class="rekap-toolbar">
        <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        <Select
          v-model="selectedPegawai"
          :options="pegawaiOptions"
          optionLabel="label"
          optionValue="value"
          filter
          showClear
          :loading="loadingPegawai"
          filterPlaceholder="Ketik nama / NIP"
          emptyFilterMessage="Pegawai tidak ditemukan"
          placeholder="Semua pegawai"
          style="min-width: 260px"
          @filter="onPegawaiFilter"
        />
        <Button label="Export Excel" icon="pi pi-file-excel" severity="success" outlined @click="exportExcel" />
      </div>

      <div class="entries-picker">
        <span class="entries-picker-label">Tampilkan</span>
        <Select v-model="rekapPageSize" :options="entriesOptions" @change="rekapFirst = 0" />
      </div>

      <DataTable
        :value="rekap"
        :loading="loadingRekap"
        paginator
        :rows="rekapPageSize"
        v-model:first="rekapFirst"
        paginatorTemplate="CurrentPageReport FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} entries"
        size="small"
        stripedRows
        responsiveLayout="scroll"
      >
        <Column header="Nama">
          <template #body="{ data }">{{ data.pegawai?.nama }}</template>
        </Column>
        <Column header="NIP">
          <template #body="{ data }">{{ data.pegawai?.nip }}</template>
        </Column>
        <Column header="Jumlah Hadir">
          <template #body="{ data }">{{ jumlahHadir(data) }}</template>
        </Column>
        <Column header="Jumlah Terlambat">
          <template #body="{ data }">
            <Tag v-if="jumlahTerlambat(data) > 0" severity="danger" :value="jumlahTerlambat(data)" />
            <span v-else>0</span>
          </template>
        </Column>
        <Column header="Tidak Absen">
          <template #body="{ data }">
            <Tag v-if="data.tanggal_terlewat?.length" severity="warn" :value="data.tanggal_terlewat.length" />
            <span v-else>0</span>
          </template>
        </Column>
        <Column header="DD / Izin / Sakit">
          <template #body="{ data }">
            <span v-if="!data.jumlah_dd && !data.jumlah_izin && !data.jumlah_sakit">-</span>
            <span v-else class="kode-badges">
              <Tag v-if="data.jumlah_dd" severity="info" :value="`DD ${data.jumlah_dd}`" />
              <Tag v-if="data.jumlah_izin" severity="secondary" :value="`Izin ${data.jumlah_izin}`" />
              <Tag v-if="data.jumlah_sakit" severity="secondary" :value="`Sakit ${data.jumlah_sakit}`" />
            </span>
          </template>
        </Column>
        <Column header="Lokasi Terakhir">
          <template #body="{ data }">
            <a
              v-if="lokasiTerakhirPegawai(data)"
              class="koordinat-link"
              :href="mapsUrl(lokasiTerakhirPegawai(data).lat, lokasiTerakhirPegawai(data).lng)"
              target="_blank"
              rel="noopener"
              :title="`Buka lokasi absen ${lokasiTerakhirPegawai(data).jenis} tanggal ${formatTanggal(lokasiTerakhirPegawai(data).tanggal)} di Google Maps`"
            >
              <i class="pi pi-map-marker"></i>
              {{ formatTanggal(lokasiTerakhirPegawai(data).tanggal) }}
            </a>
            <span v-else>-</span>
          </template>
        </Column>
        <Column header="Aksi">
          <template #body="{ data }">
            <Button icon="pi pi-eye" size="small" text rounded title="Lihat Detail" @click="openDetail(data)" />
            <Button
              v-if="auth.canManageMaster"
              icon="pi pi-file-pdf"
              size="small"
              text
              rounded
              severity="danger"
              title="Unduh PDF rekap pegawai ini"
              :loading="unduhPdfId === data.pegawai?.id"
              @click="unduhPdfPegawai(data)"
            />
          </template>
        </Column>
        <template #empty>Tidak ada data pegawai.</template>
      </DataTable>
    </div>
    </TabPanel>

    <TabPanel value="surat">
    <div class="card">
      <h3 style="margin-top: 0">Input Surat Kolektif (BA / Surat Tugas / Surat Izin / SKS)</h3>
      <p class="text-muted">
        Input surat pendukung untuk beberapa pegawai &amp; rentang tanggal sekaligus -- tanggal yang tercover akan
        terbaca DD (Dinas Dalam) untuk Surat Tugas/Berita Acara, I (Izin) untuk Surat Izin, atau S (Sakit) untuk SKS
        pada riwayat/rekap pegawai bersangkutan.
      </p>
      <div class="kolektif-form">
        <div class="kolektif-span">
          <label class="field-label">Pegawai</label>
          <MultiSelect
            v-model="kolektifForm.id_pegawai"
            :options="pegawaiOptions"
            optionLabel="label"
            optionValue="value"
            filter
            display="chip"
            :loading="loadingPegawai"
            :maxSelectedLabels="20"
            filterPlaceholder="Ketik nama atau NIP pegawai"
            emptyFilterMessage="Pegawai tidak ditemukan -- coba nama atau NIP yang lain"
            placeholder="Pilih satu atau beberapa pegawai"
            style="width: 100%"
            @filter="onPegawaiFilter"
          />
          <small v-if="pegawaiKeyword" class="text-muted">
            {{ pegawaiHasil }} pegawai cocok dengan pencarian "{{ pegawaiKeyword }}"<span v-if="pegawaiHasil > PEGAWAI_PAGE_SIZE">
              -- ditampilkan {{ PEGAWAI_PAGE_SIZE }} teratas, persempit pencarian bila pegawai yang dicari belum
              terlihat</span
            >.
          </small>
          <small v-else class="text-muted">
            Menampilkan {{ Math.min(PEGAWAI_PAGE_SIZE, pegawaiTotal) }} dari {{ pegawaiTotal }} pegawai -- ketik nama
            atau NIP pada kotak cari untuk menemukan pegawai lainnya.
          </small>
        </div>

        <div>
          <label class="field-label">Tanggal Mulai</label>
          <DatePicker
            v-model="kolektifForm.tanggal_mulai"
            dateFormat="dd-mm-yy"
            showIcon
            :disabledDays="hariNonaktif"
            style="width: 100%"
          />
        </div>
        <div>
          <label class="field-label">Tanggal Selesai</label>
          <DatePicker
            v-model="kolektifForm.tanggal_selesai"
            dateFormat="dd-mm-yy"
            showIcon
            :disabledDays="hariNonaktif"
            style="width: 100%"
          />
        </div>
        <div>
          <label class="field-label">Jenis Surat</label>
          <Select
            v-model="kolektifForm.jenis"
            :options="[
              { label: 'Surat Tugas (DD)', value: 'surat_tugas' },
              { label: 'Berita Acara (DD)', value: 'berita_acara' },
              { label: 'Surat Izin (I)', value: 'surat_izin' },
              { label: 'SKS -- Surat Keterangan Sakit (S)', value: 'sks' },
            ]"
            optionLabel="label"
            optionValue="value"
            placeholder="Pilih jenis surat"
            style="width: 100%"
          />
        </div>
        <div>
          <label class="field-label">Berkas (PDF/JPG/PNG)</label>
          <input ref="kolektifFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onKolektifFileChosen" />
          <Button
            class="berkas-btn"
            :label="kolektifFile ? kolektifFile.name : 'Pilih Berkas'"
            icon="pi pi-file"
            severity="secondary"
            outlined
            @click="pickKolektifFile"
          />
        </div>

        <div class="kolektif-span">
          <Message severity="info" :closable="false">
            Tanggal di luar hari kerja otomatis dilewati saat disimpan, mengikuti tempat tugas masing-masing pegawai:
            pegawai <b>kantor dinas</b> tidak diinput pada <b>Sabtu &amp; Minggu</b>, pegawai <b>sekolah</b> tidak
            diinput pada <b>Minggu</b>, dan <b>tanggal merah</b> dilewati untuk keduanya
            <span v-if="semuaPegawaiDinas">-- semua pegawai yang dipilih bertugas di kantor dinas, jadi Sabtu &amp; Minggu dinonaktifkan di kalender.</span>
            <span v-else-if="adaPegawaiSekolah">-- ada pegawai sekolah di antara yang dipilih, jadi tanggal Sabtu tetap bisa dipilih (hanya berlaku untuk pegawai sekolah).</span>
          </Message>
        </div>

        <div class="kolektif-span">
          <label class="field-label">Keterangan (opsional)</label>
          <Textarea v-model="kolektifForm.keterangan" rows="2" style="width: 100%" />
        </div>

        <div class="kolektif-span">
          <Button label="Input Surat" icon="pi pi-upload" :loading="submittingKolektif" @click="submitKolektif" />
        </div>
      </div>

      <h4 style="margin-top: 1.75rem">Surat yang Sudah Diinput Bulan Ini</h4>
      <div class="entries-picker">
        <span class="entries-picker-label">Tampilkan</span>
        <Select v-model="dokumenPageSize" :options="entriesOptions" @change="dokumenFirst = 0" />
      </div>
      <DataTable
        :value="dokumenAdminList"
        :loading="loadingDokumenAdmin"
        paginator
        :rows="dokumenPageSize"
        v-model:first="dokumenFirst"
        paginatorTemplate="CurrentPageReport FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} entries"
        size="small"
        stripedRows
        responsiveLayout="scroll"
      >
        <Column header="Tanggal">
          <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
        </Column>
        <Column header="Nama Pegawai">
          <template #body="{ data }">{{ data.pegawai?.nama }}</template>
        </Column>
        <Column field="label" header="Jenis Surat" />
        <Column header="Kode">
          <template #body="{ data }"><Tag :value="kodeDokumen(data.jenis)" /></template>
        </Column>
        <Column field="keterangan" header="Keterangan" />
        <Column header="Aksi">
          <template #body="{ data }">
            <Button icon="pi pi-trash" size="small" text rounded severity="danger" title="Hapus" @click="confirmHapusDokumenAdmin(data)" />
          </template>
        </Column>
        <template #empty>Belum ada surat yang diinput pada bulan ini.</template>
      </DataTable>
    </div>
    </TabPanel>
      </TabPanels>
    </Tabs>

    <Dialog
      v-model:visible="detailDialog"
      modal
      :header="`Detail Absen -- ${detailItem?.pegawai?.nama || ''}`"
      :style="{ width: '900px' }"
      :breakpoints="{ '1100px': '92vw', '640px': '94vw' }"
    >
      <template v-if="detailItem">
        <div class="detail-toolbar">
          <span class="text-muted">
            Periode {{ periodDate.toLocaleDateString('id-ID', { month: 'long', year: 'numeric' }) }} --
            NIP {{ detailItem.pegawai?.nip }}
          </span>
          <Button
            v-if="auth.canManageMaster"
            label="Unduh PDF"
            icon="pi pi-file-pdf"
            size="small"
            severity="danger"
            outlined
            :loading="unduhPdfId === detailItem.pegawai?.id"
            @click="unduhPdfPegawai(detailItem)"
          />
        </div>

        <h4>Riwayat Absen</h4>
        <DataTable
          :value="detailItem.absensi"
          size="small"
          stripedRows
          responsiveLayout="scroll"
          :rowClass="rowClassRiwayat"
        >
          <Column header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
          </Column>
          <Column header="Status">
            <template #body="{ data }">
              <Tag v-if="data.dinas_dalam_masuk || data.dinas_dalam_pulang" severity="info" value="Dinas Dalam" />
              <Tag v-else-if="tidakAbsenPulang(data)" severity="danger" value="Tidak Absen Pulang" />
              <Tag v-else-if="data.jam_masuk" severity="success" value="Hadir" />
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Jam Masuk">
            <template #body="{ data }">
              <a v-if="data.jam_masuk" href="#" @click.prevent="lihatFoto(data, 'masuk')">{{ formatJam(data.jam_masuk) }}</a>
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Terlambat">
            <template #body="{ data }">{{ data.terlambat_menit > 0 ? data.terlambat_menit + ' menit' : '-' }}</template>
          </Column>
          <Column header="Jam Pulang">
            <template #body="{ data }">
              <a v-if="data.jam_pulang" href="#" @click.prevent="lihatFoto(data, 'pulang')">{{ formatJam(data.jam_pulang) }}</a>
              <strong v-else-if="tidakAbsenPulang(data)" class="tap-note" title="Tidak Absen Pulang">TAP</strong>
              <span v-else>-</span>
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
                  v-if="data.lat_masuk != null && data.lng_masuk != null"
                  class="koordinat-link"
                  :href="mapsUrl(data.lat_masuk, data.lng_masuk)"
                  target="_blank"
                  rel="noopener"
                  title="Buka lokasi absen masuk di Google Maps"
                >
                  <i class="pi pi-map-marker"></i> Masuk: {{ formatKoordinat(data, 'masuk') }}
                </a>
                <a
                  v-if="data.lat_pulang != null && data.lng_pulang != null"
                  class="koordinat-link"
                  :href="mapsUrl(data.lat_pulang, data.lng_pulang)"
                  target="_blank"
                  rel="noopener"
                  title="Buka lokasi absen pulang di Google Maps"
                >
                  <i class="pi pi-map-marker"></i> Pulang: {{ formatKoordinat(data, 'pulang') }}
                </a>
                <span v-if="!koordinatTerakhir(data)">-</span>
              </div>
            </template>
          </Column>
          <template #empty>Belum ada absen pada bulan ini.</template>
        </DataTable>

        <template v-if="detailItem.tanggal_tercover?.length">
          <h4 style="margin-top: 1.25rem">Dinas Dalam / Izin / Sakit (Bersurat)</h4>
          <ul>
            <li v-for="t in detailItem.tanggal_tercover" :key="t.tanggal">
              {{ formatTanggal(t.tanggal) }} -- <Tag :value="t.kode" /> {{ t.label }}
            </li>
          </ul>
        </template>

        <template v-if="detailItem.tanggal_terlewat?.length">
          <h4 style="margin-top: 1.25rem">Tidak Melakukan Absensi</h4>
          <ul>
            <li v-for="tgl in detailItem.tanggal_terlewat" :key="tgl">{{ formatTanggal(tgl) }}</li>
          </ul>
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
.text-muted {
  color: #6b7280;
  font-size: 0.85rem;
}
.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
}
.jam-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 1rem;
}
.rekap-toolbar {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}
.kantor-tempel {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.35rem;
}
.kantor-tempel :deep(.p-inputtext) {
  min-width: 220px;
}
.kantor-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0.75rem;
  margin-bottom: 0.5rem;
}
.kantor-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.4rem;
}
.kode-badges {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}
/* Form input surat kolektif: satu kolom di HP, lalu melebar mengikuti layar
   desktop (2 kolom mulai 900px, 4 kolom di layar lebar) supaya sejajar
   dengan kartu pengaturan dan tidak menyisakan ruang kosong di kanan. */
.kolektif-form {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
  max-width: 100%;
  align-items: start;
}
.kolektif-span {
  grid-column: 1 / -1;
}
@media (min-width: 900px) {
  .kolektif-form {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (min-width: 1400px) {
  .kolektif-form {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
.berkas-btn.p-button {
  width: 100%;
  justify-content: flex-start;
  overflow: hidden;
}

/* ---------- kartu pengaturan: dua kolom di desktop ---------- */
.pengaturan-body {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}
.pengaturan-head {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.pengaturan-cols {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1.5rem;
}
@media (min-width: 1100px) {
  .pengaturan-cols {
    grid-template-columns: 1fr 1fr;
  }
}
.pengaturan-col {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  min-width: 0;
}
.pengaturan-group {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
  border: 1px solid var(--p-surface-200, #e2e8f0);
  border-radius: 10px;
  padding: 1rem;
}
.group-title {
  font-weight: 700;
  font-size: 0.92rem;
  color: #334155;
}

/* ---------- baris atas dialog detail (periode + tombol unduh PDF) ---------- */
.detail-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--p-surface-200, #e2e8f0);
}

/* ---------- thumbnail foto absen ---------- */
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

/* ---------- baris "Tidak Absen Pulang (TAP)" ---------- */
:deep(.row-tap) > td {
  background-color: #fce4ec !important;
}
.tap-note {
  color: #c2185b;
  font-weight: 700;
  white-space: nowrap;
}

/* ---------- tautan titik koordinat ke Google Maps ---------- */
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

/* ---------- tampilan HP ---------- */
@media (max-width: 640px) {
  .rekap-toolbar > * {
    width: 100%;
  }
  .rekap-toolbar :deep(.p-datepicker),
  .rekap-toolbar :deep(.p-select) {
    width: 100% !important;
  }
  .kolektif-form {
    max-width: 100%;
  }
  .kantor-actions :deep(.p-button) {
    width: 100%;
    justify-content: center;
  }
}
</style>
