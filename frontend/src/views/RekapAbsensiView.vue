<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { toApiDate } from '../utils/date'
import { KETERANGAN_SURAT_DROPDOWN, KETERANGAN_LAINNYA, gabungkanKeterangan } from '../composables/keteranganSurat'
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
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Badge from 'primevue/badge'

const toast = useToast()
const confirm = useConfirm()
const auth = useAuthStore()

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
  jam_mulai_pulang_jumat: '',
  jam_tutup_pulang_jumat: '',
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
const jenisSuratOptions = ref([])
// kodeLabelMap: kode (mis. "CT") -> nama jenis surat pertama yang memakainya
// -- dipakai untuk memberi label yang enak dibaca pada badge "Lainnya" di
// kolom DD/Izin/Sakit rekap, untuk kode custom di luar DD/I/S bawaan (lihat
// jumlah_lainnya dari backend, buildRekapItems di absensi_admin.go).
const kodeLabelMap = ref({})

async function loadPengaturan() {
  loadingPengaturan.value = true
  try {
    const { data } = await http.get('/absensi/pengaturan')
    Object.assign(pengaturan, data.data)
    // Normalisasi ke huruf kecil ("dinas"/"sekolah") supaya pengaturan lama
    // yang tersimpan dengan huruf besar (mis. "DINAS", dari sebelum filter
    // ini memakai kategori Tempat Kerja) tetap cocok dengan value pilihan
    // tetap di tempatTugasOptions dan tampil sebagai chip berlabel benar,
    // bukan chip kosong/rusak. Backend sendiri sudah mencocokkan tanpa
    // peduli huruf besar/kecil (lihat absensiEligible), ini murni supaya
    // tampilannya ikut rapi.
    if (Array.isArray(pengaturan.tempat_tugas_allowed)) {
      pengaturan.tempat_tugas_allowed = pengaturan.tempat_tugas_allowed.map((v) => (v || '').toLowerCase())
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan absen', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingPengaturan.value = false
  }
}

// opsi-tempat-tugas sekarang mengembalikan 2 pilihan tetap {value,label}
// (Dinas/Kantor, Sekolah) yang dicocokkan lewat kategori Tempat Kerja Unit
// Kerja pegawai (isSekolahPegawai di backend) -- bukan lagi daftar teks bebas
// Tempat Tugas pegawai apa adanya, supaya filter ini konsisten dengan jam
// kerja absen, 5/6 hari kerja, & syarat dokumen cuti yang sudah memakai
// kategori yang sama.
async function loadTempatTugasOptions() {
  try {
    const { data } = await http.get('/absensi/opsi-tempat-tugas')
    tempatTugasOptions.value = data.data || []
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

// Jenis Surat untuk dropdown form Surat Kolektif -- dimuat dari master data
// (menu Master Data -> Jenis Surat, administrator only untuk tambah/edit/
// hapus, tapi endpoint /ref/jenis-surat ini terbuka untuk siapa saja yang
// login) supaya jenis tambahan yang dibuat administrator otomatis muncul di
// sini juga, tidak lagi daftar tetap 4 jenis yang di-hardcode di frontend.
async function loadJenisSuratOptions() {
  try {
    const { data } = await http.get('/ref/jenis-surat')
    const items = data.data || []
    jenisSuratOptions.value = items.map((it) => ({ label: `${it.nama} (${it.kode})`, value: it.slug }))
    // beberapa jenis surat bisa berbagi kode custom yang sama -- ambil nama
    // yang pertama ketemu saja per kode supaya badge "Lainnya" tetap ringkas
    kodeLabelMap.value = items.reduce((acc, it) => (it.kode in acc ? acc : { ...acc, [it.kode]: it.nama }), {})
  } catch {
    jenisSuratOptions.value = []
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
    jamValid(pengaturan.jam_mulai_pulang_jumat) &&
    jamValid(pengaturan.jam_tutup_pulang_jumat) &&
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

// Pencarian bebas (nama/NIP/unit kerja) di tabel Rekap Absen -- dilakukan di
// BROWSER (bukan ke server) karena /absensi/rekap sudah mengembalikan seluruh
// pegawai bulan tsb sekaligus (lihat komentar rekapPageSize di atas), beda
// dengan pencarian pegawai pada dropdown filter (yang memang harus ke server
// karena daftar pegawai bisa jauh lebih banyak dari batas 500 baris/permintaan
// endpoint /pegawai).
const rekapSearch = ref('')
const rekapFiltered = computed(() => {
  const q = rekapSearch.value.trim().toLowerCase()
  if (!q) return rekap.value
  return rekap.value.filter((item) => {
    const nama = (item.pegawai?.nama || '').toLowerCase()
    const nip = (item.pegawai?.nip || '').toLowerCase()
    const unitKerja = (item.pegawai?.unit_kerja?.unit || '').toLowerCase()
    return nama.includes(q) || nip.includes(q) || unitKerja.includes(q)
  })
})
watch(rekapSearch, () => {
  rekapFirst.value = 0
})

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
  const verifikasiOnly = auth.isAdministrator || auth.isAdminVerifikasi ? [loadVerifikasiList(), loadJumlahMenungguVerifikasi()] : []
  await Promise.all([loadPegawaiOptions(), loadPengaturan(), loadJenisSuratOptions(), ...tugasAdminOnly, ...verifikasiOnly])
  if (auth.isAdministrator || auth.isAdminVerifikasi) {
    verifikasiCountInterval = setInterval(loadJumlahMenungguVerifikasi, 30000)
  }
  // loadRekap/loadDokumenAdmin dipakai tab "Rekap Absen" & "Surat Kolektif",
  // yang endpoint-nya (GET /absensi/rekap, /absensi/dokumen/rekap) memang
  // hanya boleh diakses administrator/admin/IsAdminAbsensi (manage() di
  // backend) -- akun yang HANYA IsAdminVerifikasi (tanpa salah satu itu)
  // tidak punya tab-tab tersebut sama sekali (lihat v-if pada Tab/TabPanel
  // di atas), jadi keduanya dilewati supaya tidak memicu error 403 yang
  // tidak perlu ditampilkan ke akun itu.
  if (auth.isAdministrator || auth.isAdmin || auth.isAdminAbsensi) {
    await Promise.all([loadRekap(), loadDokumenAdmin()])
  }
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

// ------------------------------------------------------------
// unduh ZIP berisi PDF rekap SEMUA pegawai (satu PDF per pegawai, nama
// berkas "No urut-NIP-Nama.pdf") -- khusus administrator, lihat
// exportRekapAbsensiZIP di handlers/absensi_pdf.go. Bisa makan waktu cukup
// lama kalau jumlah pegawainya banyak, jadi ada indikator loading terpisah.
// ------------------------------------------------------------

const unduhZipLoading = ref(false)

async function unduhZipRekap() {
  unduhZipLoading.value = true
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    if (selectedPegawai.value) params.id_pegawai = selectedPegawai.value
    const res = await http.get('/absensi/rekap/zip', { params, responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data], { type: 'application/zip' }))
    const link = document.createElement('a')
    link.href = url
    const bulanStr = String(params.bulan).padStart(2, '0')
    link.download = `rekap_absensi_${params.tahun}-${bulanStr}.zip`
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh ZIP', detail: await pesanErrorBlob(e), life: 5000 })
  } finally {
    unduhZipLoading.value = false
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

onBeforeUnmount(() => {
  revokeThumbnails()
  if (verifikasiCountInterval) clearInterval(verifikasiCountInterval)
})

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

// Keterangan sekarang dropdown (lihat composables/keteranganSurat.js) --
// kolektifForm.keterangan tetap dipakai (ini yang benar-benar dikirim ke
// server), nilainya diturunkan dari 2 ref terpisah: pilihan dropdown, dan
// teks bebas kalau pilihan = "Lainnya". Sama persis dengan pola yang dipakai
// AbsensiView.vue untuk "Ajukan Surat Kolektif" & dialog edit-nya.
const kolektifKeteranganPilihan = ref(null)
const kolektifKeteranganLainnya = ref('')
watch([kolektifKeteranganPilihan, kolektifKeteranganLainnya], () => {
  kolektifForm.keterangan = gabungkanKeterangan(kolektifKeteranganPilihan.value, kolektifKeteranganLainnya.value)
})

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
  if (!kolektifForm.keterangan.trim()) {
    toast.add({
      severity: 'warn',
      summary: 'Periksa kembali',
      detail: kolektifKeteranganPilihan.value === KETERANGAN_LAINNYA ? 'Isi keterangan sesuai surat' : 'Pilih keterangan',
      life: 4000,
    })
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
    kolektifKeteranganPilihan.value = null
    kolektifKeteranganLainnya.value = ''
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

// Pencarian bebas (nama/NIP/unit kerja) untuk tabel "Surat yang Sudah
// Diinput Bulan Ini" -- pola & alasannya sama persis dengan rekapSearch/
// rekapFiltered di atas (BROWSER, bukan ke server, karena satu bulan surat
// sudah diambil sekaligus). Bulan sendiri TIDAK butuh computed terpisah --
// dokumenAdminList sudah difilter bulan di SERVER lewat periodDate (lihat
// watch(periodDate, loadDokumenAdmin) di bawah), jadi di sini cukup
// menambah lapis pencarian teks di atasnya.
const dokumenSearch = ref('')
const dokumenFiltered = computed(() => {
  const q = dokumenSearch.value.trim().toLowerCase()
  if (!q) return dokumenAdminList.value
  return dokumenAdminList.value.filter((item) => {
    const nama = (item.pegawai?.nama || '').toLowerCase()
    const nip = (item.pegawai?.nip || '').toLowerCase()
    const unitKerja = (item.pegawai?.unit_kerja?.unit || '').toLowerCase()
    return nama.includes(q) || nip.includes(q) || unitKerja.includes(q)
  })
})
watch(dokumenSearch, () => {
  dokumenFirst.value = 0
})

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

// ============================================================
// Verifikasi Pengajuan Surat Kolektif Sekolah (pengajuan MANDIRI pegawai
// sekolah, lihat handlers/pengajuan_surat_kolektif.go) -- tab ini HANYA
// untuk administrator/akun IsAdminVerifikasi (lihat v-if pada Tab &
// TabPanel di template), berbeda dari tab "Surat Kolektif" di atas yang
// tetap dikelola administrator/admin/IsAdminAbsensi (input langsung, efek
// langsung, tanpa alur persetujuan).
// ============================================================

const verifikasiList = ref([])
const loadingVerifikasi = ref(false)
const verifikasiStatusFilter = ref('menunggu')
const verifikasiStatusOptions = [
  { label: 'Menunggu Verifikasi', value: 'menunggu' },
  { label: 'Disetujui', value: 'disetujui' },
  { label: 'Dikembalikan', value: 'dikembalikan' },
  { label: 'Semua', value: 'semua' },
]

async function loadVerifikasiList() {
  loadingVerifikasi.value = true
  try {
    const { data } = await http.get('/pengajuan-surat-kolektif', { params: { status: verifikasiStatusFilter.value } })
    verifikasiList.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingVerifikasi.value = false
  }
}
watch(verifikasiStatusFilter, () => loadVerifikasiList())

// ------------------------------------------------------------
// badge jumlah "menunggu" pada label tab "Verifikasi Surat Kolektif
// Sekolah" itu sendiri -- SENGAJA query terpisah dari verifikasiList/
// verifikasiStatusFilter di atas (yang cuma berisi hasil sesuai filter
// status yang sedang aktif di dropdown), supaya badge ini selalu
// menunjukkan jumlah "menunggu" yang sesungguhnya walaupun administrator
// sedang melihat filter lain (mis. "Disetujui"/"Semua"). Di-poll berkala
// (mengikuti pola lonceng notifikasi global di AppLayout.vue) supaya tetap
// akurat walau administrator berlama-lama di tab lain (Rekap Absen/
// Pengaturan/Surat Kolektif) tanpa perlu me-refresh halaman.
// ------------------------------------------------------------
const jumlahMenungguVerifikasi = ref(0)
let verifikasiCountInterval = null
async function loadJumlahMenungguVerifikasi() {
  try {
    const { data } = await http.get('/pengajuan-surat-kolektif/count-menunggu')
    jumlahMenungguVerifikasi.value = data.data?.menunggu || 0
  } catch {
    // non-kritikal -- badge cukup dilewati kalau gagal, tidak perlu toast
  }
}

// Pemilihan bulan & pencarian (nama/NIP/unit kerja) untuk tab Verifikasi --
// sama seperti tab Rekap Absen/Surat Kolektif, tapi dilakukan di BROWSER
// (bukan lewat parameter ke server) karena satu pengajuan bisa memuat
// banyak tanggal (tanggal_list) yang bisa saja melewati lebih dari satu
// bulan sekaligus -- lebih aman & sederhana dicocokkan di sini daripada
// lewat query SQL ke kolom teks JSON tanggal_list.
//
// Bulan HANYA dipakai untuk menyaring status "Disetujui"/"Dikembalikan"/
// "Semua" (riwayat) -- SENGAJA tidak ikut membatasi status "Menunggu
// Verifikasi", supaya pengajuan lama yang belum diproses tidak pernah
// "hilang" dari pandangan cuma karena bulan yang sedang dipilih berbeda.
// periodDate dipakai bersama (sinkron) dengan tab Rekap Absen/Surat
// Kolektif -- ganti bulan di satu tab otomatis ikut di tab lain juga,
// yang memang wajar karena sama-sama menyatakan "bulan yang sedang dilihat"
// untuk seluruh halaman ini.
const verifikasiSearch = ref('')
const verifikasiFiltered = computed(() => {
  const q = verifikasiSearch.value.trim().toLowerCase()
  const bulan = periodDate.value.getMonth() + 1
  const tahun = periodDate.value.getFullYear()
  const prefix = `${tahun}-${String(bulan).padStart(2, '0')}`
  return verifikasiList.value.filter((item) => {
    if (verifikasiStatusFilter.value !== 'menunggu') {
      const cocokBulan = (item.tanggal_list || []).some((t) => t.startsWith(prefix))
      if (!cocokBulan) return false
    }
    if (!q) return true
    const nama = (item.pegawai?.nama || '').toLowerCase()
    const nip = (item.pegawai?.nip || '').toLowerCase()
    const unitKerja = (item.pegawai?.unit_kerja?.unit || '').toLowerCase()
    return nama.includes(q) || nip.includes(q) || unitKerja.includes(q)
  })
})

// Karena request berkas di bawah pakai responseType: 'blob', axios TIDAK
// mem-parse body error (JSON) yang dikirim backend saat status bukan 2xx --
// e.response.data akan berupa objek Blob, bukan objek JSON, sehingga
// e.response?.data?.message selalu undefined dan pesan asli dari backend
// tersembunyi di balik pesan generik axios ("Request failed with status
// code 404"). Fungsi ini membaca isi Blob tsb sebagai teks lalu mem-parse-
// nya sebagai JSON agar pesan asli backend bisa ditampilkan ke user.
async function extractErrorMessage(e) {
  const data = e?.response?.data
  if (data instanceof Blob) {
    try {
      const text = await data.text()
      const parsed = JSON.parse(text)
      if (parsed?.message) return parsed.message
    } catch {
      // isi blob bukan JSON valid -- pakai fallback di bawah
    }
  } else if (data?.message) {
    return data.message
  }
  return e?.message || 'terjadi kesalahan tidak diketahui'
}

async function downloadVerifikasiFile(item) {
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
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: await extractErrorMessage(e), life: 4000 })
  }
}

// ---- lihat (pratinjau) berkas surat kolektif sekolah sebelum diunduh ----
const previewVerifikasiDialog = ref(false)
const previewVerifikasiUrl = ref('')
const previewVerifikasiType = ref('pdf')
const previewVerifikasiNamaFile = ref('')

async function previewVerifikasiFile(item) {
  try {
    const res = await http.get(`/pengajuan-surat-kolektif/${item.id}/file`, { params: { inline: 1 }, responseType: 'blob' })
    const ext = (item.nama_file || '').split('.').pop().toLowerCase()
    previewVerifikasiType.value = ['jpg', 'jpeg', 'png'].includes(ext) ? 'image' : ext === 'pdf' ? 'pdf' : 'other'
    previewVerifikasiUrl.value = window.URL.createObjectURL(res.data)
    previewVerifikasiNamaFile.value = item.nama_file || 'surat'
    previewVerifikasiDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat dokumen', detail: await extractErrorMessage(e), life: 4000 })
  }
}
function closePreviewVerifikasi() {
  if (previewVerifikasiUrl.value) window.URL.revokeObjectURL(previewVerifikasiUrl.value)
  previewVerifikasiUrl.value = ''
}

function confirmSetujuiVerifikasi(item) {
  confirm.require({
    message: `Setujui pengajuan surat kolektif "${item.label}" dari ${item.pegawai?.nama || 'pegawai ini'} untuk ${item.tanggal_list.length} tanggal? Absen pegawai akan otomatis berubah jadi bersurat untuk tanggal-tanggal tersebut.`,
    header: 'Konfirmasi Setujui',
    icon: 'pi pi-check-circle',
    acceptLabel: 'Ya, Setujui',
    rejectLabel: 'Batal',
    accept: async () => {
      try {
        const { data } = await http.put(`/pengajuan-surat-kolektif/${item.id}/setujui`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 6000 })
        await Promise.all([loadVerifikasiList(), loadJumlahMenungguVerifikasi()])
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 5000 })
      }
    },
  })
}

// dialog "Kembalikan" -- wajib isi catatan supaya pegawai tahu apa yang
// perlu diperbaiki (lihat kembalikanPengajuanSuratKolektif di backend).
const kembalikanDialog = ref(false)
const kembalikanItem = ref(null)
const kembalikanCatatan = ref('')
const submittingKembalikan = ref(false)

function bukaKembalikanDialog(item) {
  kembalikanItem.value = item
  kembalikanCatatan.value = ''
  kembalikanDialog.value = true
}
function closeKembalikanDialog() {
  kembalikanDialog.value = false
  kembalikanItem.value = null
}
async function submitKembalikan() {
  if (!kembalikanCatatan.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Catatan wajib diisi', life: 4000 })
    return
  }
  submittingKembalikan.value = true
  try {
    const fd = new FormData()
    fd.append('catatan', kembalikanCatatan.value.trim())
    const { data } = await http.put(`/pengajuan-surat-kolektif/${kembalikanItem.value.id}/kembalikan`, fd)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 5000 })
    closeKembalikanDialog()
    await Promise.all([loadVerifikasiList(), loadJumlahMenungguVerifikasi()])
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    submittingKembalikan.value = false
  }
}

// defaultTab: tab pertama yang benar-benar terlihat untuk akun yang login --
// akun yang HANYA IsAdminVerifikasi (tanpa administrator/admin/
// IsAdminAbsensi) tidak punya tab "Rekap Absen" sama sekali, jadi tab
// default untuk mereka langsung ke "Verifikasi Surat Kolektif Sekolah".
const defaultTab = computed(() => {
  if (auth.isAdministrator || auth.isAdmin || auth.isAdminAbsensi) return 'rekap'
  if (auth.isAdminVerifikasi) return 'verifikasi-sekolah'
  return 'rekap'
})
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
         sekali), "Surat Kolektif" (admin/administrator/IsAdminAbsensi --
         input langsung, efek langsung, dipakai juga untuk pegawai DINAS)
         & "Verifikasi Surat Kolektif Sekolah" (HANYA administrator/
         IsAdminVerifikasi -- menyetujui/mengembalikan pengajuan MANDIRI
         pegawai sekolah, lihat handlers/pengajuan_surat_kolektif.go),
         menggantikan tampilan lama yang menumpuk semuanya sekaligus di
         satu halaman. -->
    <Tabs :value="defaultTab">
      <TabList>
        <Tab v-if="auth.isAdministrator || auth.isAdmin || auth.isAdminAbsensi" value="rekap"><i class="pi pi-list" style="margin-right: 0.4rem"></i> Rekap Absen</Tab>
        <Tab v-if="auth.isAdministrator" value="pengaturan"><i class="pi pi-cog" style="margin-right: 0.4rem"></i> Pengaturan Absen</Tab>
        <Tab v-if="auth.isAdministrator || auth.isAdmin || auth.isAdminAbsensi" value="surat"><i class="pi pi-file" style="margin-right: 0.4rem"></i> Surat Kolektif</Tab>
        <Tab v-if="auth.isAdministrator || auth.isAdminVerifikasi" value="verifikasi-sekolah">
          <i class="pi pi-check-square" style="margin-right: 0.4rem"></i> Verifikasi Surat Kolektif Sekolah
          <Badge
            v-if="jumlahMenungguVerifikasi > 0"
            :value="jumlahMenungguVerifikasi"
            severity="danger"
            style="margin-left: 0.4rem"
            :title="`${jumlahMenungguVerifikasi} pengajuan menunggu verifikasi`"
          />
        </Tab>
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
                  <label class="field-label">Jam Mulai Absen Pulang (Senin-Kamis)</label>
                  <InputText v-model="pengaturan.jam_mulai_pulang" placeholder="15:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Tutup Absen Pulang (Senin-Kamis, setelah ini otomatis ditutup)</label>
                  <InputText v-model="pengaturan.jam_tutup_pulang" placeholder="20:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Mulai Absen Pulang (Jumat)</label>
                  <InputText v-model="pengaturan.jam_mulai_pulang_jumat" placeholder="15:00" style="width: 100%" />
                </div>
                <div>
                  <label class="field-label">Jam Tutup Absen Pulang (Jumat, setelah ini otomatis ditutup)</label>
                  <InputText v-model="pengaturan.jam_tutup_pulang_jumat" placeholder="20:00" style="width: 100%" />
                </div>
              </div>
              <Message severity="info" :closable="false" style="margin-top: 0.5rem">
                Berlaku untuk pegawai berkategori Dinas/Kantor (lihat "Tempat Kerja" di menu Unit Kerja -- kalau unit kerja pegawai belum dikategorikan, dipakai tebakan otomatis dari kata "sekolah" pada Tempat Tugas). Jam masuk pagi sama untuk semua hari kerja, tapi jam pulang punya 2 pengaturan terpisah: Senin-Kamis dan Jumat (karena pegawai Dinas/Kantor biasanya pulang lebih awal di hari Jumat). Absen masuk otomatis ditutup (tidak bisa lagi absen masuk maupun pulang) begitu lewat jam tutup, walaupun pegawai belum absen masuk sama sekali hari itu. Absen pulang juga otomatis ditutup begitu lewat jam tutup absen pulang yang berlaku hari itu, walaupun pegawai sudah absen masuk dan belum sempat absen pulang.
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
                Berlaku untuk pegawai berkategori Sekolah (lihat "Tempat Kerja" di menu Unit Kerja -- kalau unit
                kerja pegawai belum dikategorikan, dipakai tebakan otomatis dari kata "sekolah" pada Tempat Tugas) --
                jam kerjanya otomatis dipakai menggantikan set Dinas/Kantor di atas untuk pegawai tersebut, mengikuti
                aturan yang sama dengan penentuan 5/6 hari kerja & syarat dokumen cuti di menu lain.
              </Message>
            </div>

            <!--
              Card "Titik Koordinat Kantor & Radius Absen (Default)" SENGAJA
              disembunyikan dari sini (atas permintaan: titik koordinat kini
              diatur per Unit Kerja lewat menu Master Data -> Unit Kerja, yang
              jadi prioritas utama). Field pengaturan.kantor_lat/kantor_lng/
              radius_meter & fallback-nya di backend (lihat absensiCekRadius,
              handlers/absensi.go) TETAP ada & tetap aktif sebagai jaring
              pengaman -- kalau ada Unit Kerja kategori Dinas/Kantor yang
              belum diisi titik sendiri, absen di situ tidak otomatis terbuka
              tanpa validasi jarak sama sekali. Fungsi ambilLokasiKantor/
              hapusLokasiKantor/terapkanTempelKoordinat & state
              tempelKoordinat di <script> sengaja dibiarkan (tidak dihapus)
              kalau suatu saat card ini perlu ditampilkan kembali.
            -->
          </div>

          <div class="pengaturan-col">
            <div class="pengaturan-group">
              <div class="group-title">Siapa yang Boleh Absen</div>
              <div>
                <label class="field-label">Kategori Tempat Kerja yang Boleh Absen</label>
                <MultiSelect
                  v-model="pengaturan.tempat_tugas_allowed"
                  :options="tempatTugasOptions"
                  optionLabel="label"
                  optionValue="value"
                  filter
                  display="chip"
                  placeholder="Semua kategori (belum dibatasi)"
                  style="width: 100%"
                />
                <small class="text-muted">
                  Kosongkan untuk mengizinkan semua pegawai (Dinas/Kantor maupun Sekolah). Dicocokkan lewat kategori
                  "Tempat Kerja" Unit Kerja pegawai (menu Unit Kerja) -- pegawai yang unit kerjanya belum dikategorikan
                  memakai tebakan otomatis dari Tempat Tugas seperti biasa.
                </small>
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

    <TabPanel v-if="auth.isAdministrator || auth.isAdmin || auth.isAdminAbsensi" value="rekap">
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
        <IconField class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
          <InputText v-model="rekapSearch" placeholder="Cari nama, NIP, atau unit kerja..." style="width: 100%" />
          <InputIcon class="pi pi-search" />
        </IconField>
        <Button label="Export Excel" icon="pi pi-file-excel" severity="success" outlined @click="exportExcel" />
        <Button
          v-if="auth.isAdministrator"
          label="Download ZIP"
          icon="pi pi-file-pdf"
          severity="help"
          outlined
          :loading="unduhZipLoading"
          @click="unduhZipRekap"
        />
      </div>

      <div class="entries-picker">
        <span class="entries-picker-label">Tampilkan</span>
        <Select v-model="rekapPageSize" :options="entriesOptions" @change="rekapFirst = 0" />
      </div>

      <DataTable
        :value="rekapFiltered"
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
        <Column header="No" style="width: 3rem">
          <template #body="{ index }">{{ rekapFirst + index + 1 }}</template>
        </Column>
        <Column header="Nama">
          <template #body="{ data }">{{ data.pegawai?.nama }}</template>
        </Column>
        <Column header="NIP">
          <template #body="{ data }">{{ data.pegawai?.nip }}</template>
        </Column>
        <Column header="Unit Kerja">
          <template #body="{ data }">{{ data.pegawai?.unit_kerja?.unit || '-' }}</template>
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
        <Column header="DD / Izin / Sakit / Lainnya">
          <template #body="{ data }">
            <span v-if="!data.jumlah_dd && !data.jumlah_izin && !data.jumlah_sakit && !Object.keys(data.jumlah_lainnya || {}).length">-</span>
            <span v-else class="kode-badges">
              <Tag v-if="data.jumlah_dd" severity="info" :value="`DD ${data.jumlah_dd}`" />
              <Tag v-if="data.jumlah_izin" severity="secondary" :value="`Izin ${data.jumlah_izin}`" />
              <Tag v-if="data.jumlah_sakit" severity="secondary" :value="`Sakit ${data.jumlah_sakit}`" />
              <Tag
                v-for="(jumlah, kode) in data.jumlah_lainnya || {}"
                :key="kode"
                severity="warn"
                :value="`${kodeLabelMap[kode] || kode} ${jumlah}`"
              />
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
        <template #empty>{{ rekapSearch.trim() ? 'Tidak ada pegawai yang cocok dengan pencarian.' : 'Tidak ada data pegawai.' }}</template>
      </DataTable>
    </div>
    </TabPanel>

    <TabPanel v-if="auth.isAdministrator || auth.isAdmin || auth.isAdminAbsensi" value="surat">
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
            :options="jenisSuratOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="Pilih jenis surat"
            filter
            style="width: 100%"
          />
          <small v-if="jenisSuratOptions.length === 0" style="color: var(--p-text-muted-color)">
            Belum ada Jenis Surat -- tambahkan dulu di menu Master Data -&gt; Jenis Surat.
          </small>
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
          <label class="field-label">Keterangan</label>
          <Select
            v-model="kolektifKeteranganPilihan"
            :options="KETERANGAN_SURAT_DROPDOWN"
            placeholder="Pilih keterangan"
            showClear
            style="width: 100%"
          />
          <Textarea
            v-if="kolektifKeteranganPilihan === KETERANGAN_LAINNYA"
            v-model="kolektifKeteranganLainnya"
            rows="2"
            placeholder="Isi keterangan sesuai surat"
            style="width: 100%; margin-top: 0.5rem"
          />
        </div>

        <div class="kolektif-span">
          <Button label="Input Surat" icon="pi pi-upload" :loading="submittingKolektif" @click="submitKolektif" />
        </div>
      </div>

      <h4 style="margin-top: 1.75rem">Surat yang Sudah Diinput Bulan Ini</h4>
      <div class="rekap-toolbar">
        <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        <IconField class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
          <InputText v-model="dokumenSearch" placeholder="Cari nama, NIP, atau unit kerja..." style="width: 100%" />
          <InputIcon class="pi pi-search" />
        </IconField>
      </div>
      <div class="entries-picker">
        <span class="entries-picker-label">Tampilkan</span>
        <Select v-model="dokumenPageSize" :options="entriesOptions" @change="dokumenFirst = 0" />
      </div>
      <DataTable
        :value="dokumenFiltered"
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
        <!-- data.kode dihitung LIVE oleh backend (lihat absensiDokumenAdminOut,
             handlers/absensi_dokumen.go) -- kalau tetap kosong/"-", berarti
             master Jenis Surat untuk baris ini memang belum/tidak punya Kode
             (cek & lengkapi di menu Master Data -> Jenis Surat). -->
        <Column header="Kode">
          <template #body="{ data }">
            <Tag v-if="data.kode" :value="data.kode" />
            <span v-else class="text-muted">-</span>
          </template>
        </Column>
        <Column field="keterangan" header="Keterangan" />
        <!-- Diinput Oleh: HANYA tampil untuk role administrator -- data.diinput_oleh
             sendiri memang hanya dikirim backend untuk administrator (lihat
             listAbsensiDokumenAdmin, handlers/absensi_dokumen.go), v-if di sini
             cuma lapis tampilan tambahan, bukan satu-satunya pengaman. Baris
             lama (sebelum kolom id_diinput_oleh ada) tampil "-". -->
        <Column v-if="auth.isAdministrator" header="Diinput Oleh">
          <template #body="{ data }">{{ data.diinput_oleh?.nama || '-' }}</template>
        </Column>
        <Column header="Aksi">
          <template #body="{ data }">
            <Button icon="pi pi-trash" size="small" text rounded severity="danger" title="Hapus" @click="confirmHapusDokumenAdmin(data)" />
          </template>
        </Column>
        <template #empty>{{ dokumenSearch.trim() ? 'Tidak ada surat yang cocok dengan pencarian.' : 'Belum ada surat yang diinput pada bulan ini.' }}</template>
      </DataTable>
    </div>
    </TabPanel>

    <TabPanel v-if="auth.isAdministrator || auth.isAdminVerifikasi" value="verifikasi-sekolah">
    <div class="card">
      <h3 style="margin-top: 0">Verifikasi Pengajuan Surat Kolektif Sekolah</h3>
      <p class="text-muted">
        Pengajuan surat kolektif yang diajukan MANDIRI oleh pegawai bertugas di sekolah untuk tanggal absen yang
        terlewat. Setujui untuk membuat absen pegawai otomatis berubah jadi bersurat, atau kembalikan untuk direvisi
        (wajib isi catatan).
      </p>
      <div class="rekap-toolbar">
        <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        <IconField class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
          <InputText v-model="verifikasiSearch" placeholder="Cari nama, NIP, atau unit kerja..." style="width: 100%" />
          <InputIcon class="pi pi-search" />
        </IconField>
      </div>
      <div class="entries-picker">
        <span class="entries-picker-label">Status</span>
        <Select v-model="verifikasiStatusFilter" :options="verifikasiStatusOptions" optionLabel="label" optionValue="value" />
      </div>
      <Message v-if="verifikasiStatusFilter === 'menunggu'" severity="info" :closable="false" style="margin-bottom: 1rem">
        Filter bulan tidak berlaku untuk status "Menunggu Verifikasi" -- semua pengajuan yang belum diproses selalu
        ditampilkan apa pun bulannya, supaya tidak ada yang tidak sengaja terlewat.
      </Message>
      <DataTable :value="verifikasiFiltered" :loading="loadingVerifikasi" size="small" stripedRows responsiveLayout="scroll">
        <Column header="Pegawai">
          <template #body="{ data }">{{ data.pegawai?.nama }}</template>
        </Column>
        <Column header="Tanggal">
          <template #body="{ data }">{{ data.tanggal_list.map(formatTanggal).join(', ') }}</template>
        </Column>
        <Column field="label" header="Jenis Surat" />
        <Column field="keterangan" header="Keterangan" />
        <Column header="Status">
          <template #body="{ data }">
            <Tag
              :severity="data.status === 'disetujui' ? 'success' : data.status === 'dikembalikan' ? 'danger' : 'warn'"
              :value="data.status === 'disetujui' ? 'Disetujui' : data.status === 'dikembalikan' ? 'Dikembalikan' : 'Menunggu'"
            />
          </template>
        </Column>
        <Column v-if="verifikasiStatusFilter !== 'menunggu'" header="Catatan Verifikasi">
          <template #body="{ data }">{{ data.catatan_verifikasi || '-' }}</template>
        </Column>
        <!-- Diverifikasi Oleh: HANYA tampil untuk role administrator, sama
             seperti "Diinput Oleh" pada tab Surat Kolektif di atas --
             data.verifikator sendiri memang hanya dikirim backend untuk
             administrator (lihat listPengajuanSuratKolektifAdmin,
             handlers/pengajuan_surat_kolektif.go). Cuma relevan begitu
             pengajuan sudah diproses (bukan status "menunggu"). -->
        <Column v-if="auth.isAdministrator && verifikasiStatusFilter !== 'menunggu'" header="Diverifikasi Oleh">
          <template #body="{ data }">{{ data.verifikator?.nama || '-' }}</template>
        </Column>
        <Column header="Aksi">
          <template #body="{ data }">
            <Button icon="pi pi-eye" size="small" text rounded title="Lihat berkas" @click="previewVerifikasiFile(data)" />
            <Button icon="pi pi-download" size="small" text rounded title="Unduh berkas" @click="downloadVerifikasiFile(data)" />
            <template v-if="data.status === 'menunggu'">
              <Button icon="pi pi-check" size="small" text rounded severity="success" title="Setujui" @click="confirmSetujuiVerifikasi(data)" />
              <Button icon="pi pi-undo" size="small" text rounded severity="danger" title="Kembalikan untuk direvisi" @click="bukaKembalikanDialog(data)" />
            </template>
          </template>
        </Column>
        <template #empty>{{ verifikasiSearch.trim() ? 'Tidak ada pengajuan yang cocok dengan pencarian.' : 'Tidak ada pengajuan pada status ini.' }}</template>
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

    <!-- ================= dialog kembalikan pengajuan surat kolektif sekolah ================= -->
    <Dialog
      v-model:visible="kembalikanDialog"
      modal
      header="Kembalikan Pengajuan untuk Direvisi"
      :style="{ width: '440px' }"
      :breakpoints="{ '640px': '94vw' }"
      @hide="closeKembalikanDialog"
    >
      <p class="text-muted">
        Pengajuan dari <b>{{ kembalikanItem?.pegawai?.nama }}</b> akan dikembalikan ke pegawai untuk direvisi &amp;
        diajukan ulang. Jelaskan apa yang perlu diperbaiki.
      </p>
      <div class="field-label">Catatan (wajib)</div>
      <Textarea v-model="kembalikanCatatan" rows="3" style="width: 100%" autofocus />
      <template #footer>
        <Button label="Batal" severity="secondary" text @click="closeKembalikanDialog" />
        <Button label="Kembalikan" icon="pi pi-undo" severity="danger" :loading="submittingKembalikan" @click="submitKembalikan" />
      </template>
    </Dialog>

    <!-- ================= dialog lihat (pratinjau) berkas surat kolektif sekolah ================= -->
    <Dialog
      v-model:visible="previewVerifikasiDialog"
      modal
      :header="previewVerifikasiNamaFile || 'Pratinjau Berkas'"
      :style="{ width: '95vw', maxWidth: '62rem' }"
      :breakpoints="{ '640px': '96vw' }"
      @hide="closePreviewVerifikasi"
    >
      <div v-if="previewVerifikasiType === 'pdf'" style="width: 100%; height: 75vh">
        <iframe :src="previewVerifikasiUrl" style="width: 100%; height: 100%; border: none" title="Pratinjau berkas"></iframe>
      </div>
      <div v-else-if="previewVerifikasiType === 'image'" style="text-align: center">
        <img :src="previewVerifikasiUrl" style="max-width: 100%; max-height: 75vh" alt="Pratinjau berkas" />
      </div>
      <Message v-else severity="warn" :closable="false">
        Jenis berkas ini tidak bisa ditampilkan langsung -- silakan unduh berkasnya untuk membukanya.
      </Message>
      <template #footer>
        <Button label="Tutup" severity="secondary" outlined @click="previewVerifikasiDialog = false" />
      </template>
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
