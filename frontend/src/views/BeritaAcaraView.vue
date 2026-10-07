<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { useBulkDelete } from '../composables/useBulkDelete'

import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import RadioButton from 'primevue/radiobutton'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Tag from 'primevue/tag'

// BeritaAcaraView -- menu "Berita Acara" (khusus administrator & admin):
// membuat Berita Acara resmi untuk pegawai yang tidak bisa melakukan absensi
// online lewat E-Office pada SATU tanggal kejadian, baik untuk satu pegawai
// saja ("individu") maupun beberapa pegawai sekaligus ("kolektif", BOLEH
// campur unit kerja -- penandatangan dipilih manual tiap kali). PDF dibuat
// otomatis di server (backend/handlers/berita_acara.go) begitu dikirim, dan
// tanggal kejadian otomatis tercatat DD (Dinas Dalam) di Rekap Absen pegawai
// terpilih -- SAMA seperti upload berkas manual di menu itu.
//
// Setiap "batch" (satu kali "Buat Berita Acara", individu ATAU kolektif)
// ditampilkan sebagai SATU kartu berisi seluruh pegawai di dalamnya --
// dikelompokkan oleh backend lewat nama_file yang sama (lihat listBeritaAcara
// di handlers/berita_acara.go). Hapus (satuan maupun terpilih/kolektif)
// CUKUP memanggil ulang endpoint hapus satuan yang sudah ada
// (DELETE /absensi/dokumen/{id}) untuk SETIAP baris pegawai dalam batch itu
// -- TIDAK ada endpoint hapus batch khusus di backend.

const toast = useToast()
const confirm = useConfirm()
const { selected: selectedItems, bulkDeleting, confirmBulkDelete } = useBulkDelete()

function isItemSelected(item) {
  return selectedItems.value.some((i) => i.nama_file === item.nama_file)
}
function toggleItemSelect(item) {
  const idx = selectedItems.value.findIndex((i) => i.nama_file === item.nama_file)
  if (idx === -1) selectedItems.value.push(item)
  else selectedItems.value.splice(idx, 1)
}
async function hapusSatuBatch(item) {
  await Promise.all(item.pegawai.map((pg) => http.delete(`/absensi/dokumen/${pg.id}`)))
}
function hapusTerpilih() {
  confirmBulkDelete({
    label: 'berita acara',
    deleteOne: hapusSatuBatch,
    onDone: loadItems,
  })
}
function konfirmasiHapusBatch(item) {
  confirm.require({
    message: `Hapus Berita Acara ${item.jenis === 'kolektif' ? `kolektif (${item.pegawai.length} pegawai)` : `untuk ${item.pegawai[0]?.nama || 'pegawai ini'}`}? Tanggal yang sudah tercover di Rekap Absen pegawai terkait akan ikut terhapus.`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await hapusSatuBatch(item)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Berita Acara dihapus', life: 4000 })
        loadItems()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 5000 })
      }
    },
  })
}

const items = ref([])
const loading = ref(true)
const search = ref('')
let searchTimer = null

async function loadItems() {
  loading.value = true
  selectedItems.value = []
  try {
    const params = {}
    if (search.value.trim()) params.q = search.value.trim()
    const { data } = await http.get('/berita-acara', { params })
    items.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(loadItems, 300)
})

function formatTanggal(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'long', day: 'numeric' })
}

// ============================================================
// alasan (4 pilihan tetap) -- diambil dari backend supaya SELALU sinkron
// dengan AlasanBeritaAcaraOptions di handlers/berita_acara.go; daftar
// bawaan di bawah cuma cadangan kalau endpointnya gagal dimuat.
// ============================================================
const alasanOptions = ref(['Jaringan tidak bagus', 'Server Error', 'Motor Rusak', 'Banjir'])
async function loadAlasanOptions() {
  try {
    const { data } = await http.get('/berita-acara/alasan-options')
    if (Array.isArray(data.data) && data.data.length) alasanOptions.value = data.data
  } catch {
    // tetap pakai daftar bawaan di atas
  }
}

// ============================================================
// pencarian pegawai (server-side, pola sama dengan Input Surat Kolektif di
// RekapAbsensiView.vue) -- SATU daftar opsi dipakai bersama baik untuk
// Select (individu) maupun MultiSelect (kolektif) field "Pegawai", dan SATU
// lagi TERPISAH untuk "Penandatangan" supaya kata kunci pencariannya tidak
// saling menimpa.
// ============================================================
const PEGAWAI_PAGE_SIZE = 100

function toPegawaiOption(p) {
  // isSekolah: ikut dibawa dari field is_sekolah hasil GET /api/pegawai (lihat
  // handlers/pegawai.go -- listPegawai) -- dipakai isDinasOnly di bawah untuk
  // mendeteksi otomatis apakah SELURUH pegawai terpilih bertempat tugas
  // Dinas/Kantor (penandatangan auto-resolve, field Penandatangan disembunyikan)
  // atau ada yang Sekolah (penandatangan tetap dipilih manual seperti biasa).
  return { label: `${p.nama} (${p.nip})`, value: p.id, isSekolah: !!p.is_sekolah }
}

function makePegawaiPicker() {
  const options = ref([])
  const loading = ref(false)
  const total = ref(0)
  const hasil = ref(0)
  const keyword = ref('')
  const terpilihCache = ref([])
  function gabungDenganTerpilih(list) {
    const adaDiHasil = new Set(list.map((o) => o.value))
    return [...terpilihCache.value.filter((o) => !adaDiHasil.has(o.value)), ...list]
  }
  async function load(kw = '') {
    loading.value = true
    try {
      const params = { pageSize: PEGAWAI_PAGE_SIZE }
      const q = (kw || '').trim()
      keyword.value = q
      if (q) params.q = q
      const { data } = await http.get('/pegawai', { params })
      const list = Array.isArray(data.data) ? data.data : []
      hasil.value = data.meta?.total ?? list.length
      if (!q) total.value = hasil.value
      options.value = gabungDenganTerpilih(list.map(toPegawaiOption))
    } catch {
      options.value = gabungDenganTerpilih([])
    } finally {
      loading.value = false
    }
  }
  let timer = null
  function onFilter(event) {
    const kw = event?.value || ''
    clearTimeout(timer)
    timer = setTimeout(() => load(kw), 300)
  }
  function ingatTerpilih(idList) {
    const terpilih = (Array.isArray(idList) ? idList : [idList]).filter((v) => v != null)
    const dikenal = new Map([...terpilihCache.value, ...options.value].map((o) => [o.value, o]))
    terpilihCache.value = [...new Set(terpilih)].map((id) => dikenal.get(id)).filter(Boolean)
  }
  return { options, loading, total, hasil, keyword, terpilihCache, load, onFilter, ingatTerpilih }
}

const pegawaiPicker = makePegawaiPicker()
const penandatanganPicker = makePegawaiPicker()

// ============================================================
// dialog "Buat Berita Acara" / "Edit Berita Acara" -- SATU dialog & form
// yang sama dipakai untuk dua mode (editingItem null = mode buat baru,
// terisi = mode edit batch yang sudah ada), sesuai permintaan pengguna
// "BA yg di buat menu administrator dan admin bisa di edit dan
// menambahkan nama" -- mengedit jenis (individu<->kolektif) di dialog ini
// SEKALIGUS cara "menambahkan nama" (tambah pegawai lewat MultiSelect
// kolektif yang sama) atau menghapus nama (hilangkan dari MultiSelect),
// dikirim ke PUT /api/berita-acara/{namaFile} (handlers/berita_acara.go --
// updateBeritaAcara) yang MENGHITUNG ULANG jenis otomatis dari jumlah
// pegawai akhir, bukan dari radio button jenis (lihat komentar di sana).
// ============================================================
const buatDialog = ref(false)
const submitting = ref(false)
const editingItem = ref(null)
const isEditing = computed(() => !!editingItem.value)

const form = ref({
  jenis: 'individu',
  id_pegawai_individu: null,
  id_pegawai_kolektif: [],
  id_penandatangan: null,
  alasan: null,
  tanggal_kejadian: null,
  tanggal_surat: null,
  nomor_surat: '',
})

// buktiDukungFile: WAJIB diupload untuk jenis "individu" (BUKAN "kolektif",
// yang mencakup banyak pegawai sekaligus jadi tidak relevan satu bukti untuk
// semuanya) -- lihat validasi di buatBeritaAcara (handlers/berita_acara.go).
const buktiDukungFile = ref(null)
const buktiDukungFileInput = ref(null)
const BUKTI_DUKUNG_MAX_FILE_BYTES = 15 * 1024 * 1024
function pickBuktiDukungFile() {
  buktiDukungFileInput.value?.click()
}
function onBuktiDukungFileChosen(e) {
  const file = e.target.files?.[0] || null
  // Bukti dukung Berita Acara HANYA menerima gambar (JPG/JPEG/PNG) --
  // PDF/format lain DITOLAK di sini juga (bukan cuma di backend) supaya
  // penggunanya langsung tahu saat memilih berkas, tanpa perlu menunggu
  // submit dulu (lihat parseBuktiDukungUpload di
  // handlers/berita_acara.go untuk validasi sisi server-nya).
  if (file) {
    const ext = (file.name.split('.').pop() || '').toLowerCase()
    if (!['jpg', 'jpeg', 'png'].includes(ext)) {
      toast.add({
        severity: 'error',
        summary: 'Format tidak didukung',
        detail: 'Bukti dukung harus berupa gambar JPG/JPEG atau PNG -- berkas PDF atau format lain tidak diterima.',
        life: 7000,
      })
      e.target.value = ''
      buktiDukungFile.value = null
      return
    }
  }
  if (file && file.size > BUKTI_DUKUNG_MAX_FILE_BYTES) {
    toast.add({
      severity: 'error',
      summary: 'Berkas terlalu besar',
      detail: `Ukuran berkas ${(file.size / (1024 * 1024)).toFixed(1)}MB melebihi batas maksimal 15MB. Kompres berkas terlebih dahulu.`,
      life: 8000,
    })
    e.target.value = ''
    buktiDukungFile.value = null
    return
  }
  buktiDukungFile.value = file
}
// tandaTanggalSuratManual: begitu admin mengubah Tanggal Surat sendiri,
// Tanggal Kejadian berikutnya TIDAK lagi menimpanya secara otomatis.
let tanggalSuratManual = false
watch(() => form.value.tanggal_kejadian, (v) => {
  if (v && !tanggalSuratManual) form.value.tanggal_surat = v
})
watch(() => form.value.tanggal_surat, () => {
  tanggalSuratManual = true
})

watch(() => form.value.id_pegawai_individu, (v) => pegawaiPicker.ingatTerpilih(v))
watch(() => form.value.id_pegawai_kolektif, (v) => pegawaiPicker.ingatTerpilih(v))
watch(() => form.value.id_penandatangan, (v) => penandatanganPicker.ingatTerpilih(v))

function bukaBuatDialog() {
  editingItem.value = null
  form.value = {
    jenis: 'individu',
    id_pegawai_individu: null,
    id_pegawai_kolektif: [],
    id_penandatangan: null,
    alasan: null,
    tanggal_kejadian: null,
    tanggal_surat: null,
    nomor_surat: '',
  }
  buktiDukungFile.value = null
  tanggalSuratManual = false
  pegawaiPicker.terpilihCache.value = []
  penandatanganPicker.terpilihCache.value = []
  if (pegawaiPicker.options.value.length === 0) pegawaiPicker.load()
  if (penandatanganPicker.options.value.length === 0) penandatanganPicker.load()
  if (alasanOptions.value.length === 0) loadAlasanOptions()
  buatDialog.value = true
}

// nomorUrutanDariNomorLengkap: nomor yang tersimpan/ditampilkan
// (beritaAcaraBatchOut.Nomor) sudah format LENGKAP "800/{urutan}/Disdikbud/
// {bulan romawi}/{tahun}" (lihat nomorSuratBeritaAcaraLengkap di
// handlers/berita_acara.go) -- field "Nomor Urut Surat" pada dialog ini
// HANYA meminta bagian {urutan}-nya saja (dirangkai ulang otomatis oleh
// backend tiap kali disimpan), jadi saat edit perlu diurai balik supaya
// tidak ikut terbungkus dua kali ("800/800/.../Disdikbud/...").
function nomorUrutanDariNomorLengkap(nomorLengkap) {
  if (!nomorLengkap) return ''
  const m = /^800\/(.+)\/Disdikbud\/[IVXLCDM]+\/\d{4}$/i.exec(nomorLengkap.trim())
  return m ? m[1] : nomorLengkap
}

// bukaEditDialog: membuka dialog yang SAMA dengan "Buat Berita Acara" di
// atas, tapi terisi data batch terpilih -- admin bebas mengubah
// jenis/pegawai (termasuk menambah/menghapus nama)/alasan/tanggal/nomor/
// penandatangan sebelum menyimpan lewat submitBuat (yang akan memanggil PUT
// /api/berita-acara/{nama_file} karena editingItem terisi).
async function bukaEditDialog(item) {
  editingItem.value = item
  const idPegawai = item.pegawai.map((pg) => pg.id)
  form.value = {
    jenis: item.jenis,
    id_pegawai_individu: item.jenis === 'individu' ? idPegawai[0] ?? null : null,
    id_pegawai_kolektif: item.jenis === 'kolektif' ? idPegawai : [],
    id_penandatangan: null,
    alasan: item.alasan,
    tanggal_kejadian: item.tanggal ? new Date(item.tanggal) : null,
    // tanggal_surat TIDAK tersimpan terpisah dari tanggal kejadian pada data
    // batch (hanya ikut terbungkus di dalam Nomor yang sudah lengkap) --
    // default disamakan dengan tanggal kejadian seperti saat membuat baru,
    // admin bisa mengubahnya lagi kalau memang berbeda.
    tanggal_surat: item.tanggal ? new Date(item.tanggal) : null,
    nomor_surat: nomorUrutanDariNomorLengkap(item.nomor),
  }
  buktiDukungFile.value = null
  tanggalSuratManual = false
  pegawaiPicker.terpilihCache.value = []
  penandatanganPicker.terpilihCache.value = []
  if (alasanOptions.value.length === 0) loadAlasanOptions()
  buatDialog.value = true
  await Promise.all([pegawaiPicker.load(), penandatanganPicker.load()])
  // Opsi baru saja termuat -- segarkan ulang cache "terpilih" supaya
  // isDinasOnly (dan Select Penandatangan) langsung akurat memakai data
  // is_sekolah pegawai yang baru didapat, bukan cache kosong sebelumnya.
  pegawaiPicker.ingatTerpilih(idPegawai)
}

function toDateStr(d) {
  if (!d) return ''
  const dt = new Date(d)
  const pad = (n) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
}

const idPegawaiTerpilih = computed(() =>
  form.value.jenis === 'individu'
    ? form.value.id_pegawai_individu ? [form.value.id_pegawai_individu] : []
    : form.value.id_pegawai_kolektif
)

// isDinasOnly: true kalau SELURUH pegawai yang sudah terpilih sama-sama
// bertempat tugas Dinas/Kantor (tidak satupun Sekolah) -- pegawaiPicker.terpilihCache
// menyimpan objek opsi (termasuk isSekolah) dari setiap id yang pernah
// terpilih (lihat ingatTerpilih di makePegawaiPicker), jadi tetap akurat
// walau daftar options sudah berubah karena pencarian baru. Hanya penanda di
// SISI FRONTEND untuk menyembunyikan field Penandatangan -- backend (lihat
// buatBeritaAcara di handlers/berita_acara.go) mengecek ULANG hal yang sama
// sebagai sumber kebenaran sebenarnya, jadi tidak masalah kalau deteksi di
// sini meleset sedikit (field tetap bisa diisi manual kalau ternyata backend
// menilai beda).
const isDinasOnly = computed(() => {
  const ids = idPegawaiTerpilih.value
  if (ids.length === 0) return false
  const dikenal = new Map(pegawaiPicker.terpilihCache.value.map((o) => [o.value, o]))
  return ids.every((id) => dikenal.get(id) && !dikenal.get(id).isSekolah)
})

// canReuseBuktiDukung: khusus mode EDIT -- kalau jenis akhir masih
// "individu" dengan pegawai yang SAMA PERSIS dengan sebelumnya (bukan hasil
// ganti pegawai) dan batch lama memang sudah punya bukti dukung, upload
// baru boleh dikosongkan (backend otomatis memakai ulang berkas lama --
// lihat updateBeritaAcara di handlers/berita_acara.go). Begitu jenis
// berubah jadi kolektif, atau pegawainya diganti, atau ini mode buat baru,
// upload tetap wajib seperti biasa.
const canReuseBuktiDukung = computed(() => {
  if (!editingItem.value || form.value.jenis !== 'individu' || editingItem.value.jenis !== 'individu') return false
  const satuSatunya = idPegawaiTerpilih.value[0]
  return satuSatunya != null && editingItem.value.pegawai[0]?.id === satuSatunya && !!editingItem.value.pegawai[0]?.ada_bukti_dukung
})

async function submitBuat() {
  if (idPegawaiTerpilih.value.length === 0) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih minimal satu pegawai', life: 4000 })
    return
  }
  if (!isDinasOnly.value && !form.value.id_penandatangan) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Penandatangan (yang mengetahui) wajib dipilih', life: 4000 })
    return
  }
  if (!form.value.alasan) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih alasan', life: 4000 })
    return
  }
  if (!form.value.tanggal_kejadian) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Tanggal kejadian wajib diisi', life: 4000 })
    return
  }
  if (form.value.jenis === 'individu' && !buktiDukungFile.value && !canReuseBuktiDukung.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Bukti dukung (foto/scan pendukung alasan terpilih) wajib dilampirkan untuk Berita Acara individu', life: 5000 })
    return
  }
  submitting.value = true
  try {
    const fd = new FormData()
    fd.append('jenis', form.value.jenis)
    for (const id of idPegawaiTerpilih.value) fd.append('id_pegawai', id)
    if (form.value.id_penandatangan) fd.append('id_penandatangan', form.value.id_penandatangan)
    fd.append('tanggal_kejadian', toDateStr(form.value.tanggal_kejadian))
    fd.append('tanggal_surat', toDateStr(form.value.tanggal_surat))
    fd.append('nomor_surat', form.value.nomor_surat.trim())
    fd.append('alasan', form.value.alasan)
    if (form.value.jenis === 'individu' && buktiDukungFile.value) fd.append('file', buktiDukungFile.value)
    const { data } = isEditing.value
      ? await http.put(`/berita-acara/${encodeURIComponent(editingItem.value.nama_file)}`, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      : await http.post('/berita-acara', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 7000 })
    buatDialog.value = false
    await loadItems()
  } catch (e) {
    const pesan = e.response?.data?.message || (e.message === 'Network Error'
      ? 'Koneksi terputus saat mengirim data (biasanya karena sinyal/koneksi internet tidak stabil saat mengupload berkas). Periksa koneksi internet Anda lalu coba ajukan ulang.'
      : e.message)
    toast.add({ severity: 'error', summary: isEditing.value ? 'Gagal menyimpan perubahan' : 'Gagal membuat', detail: pesan, life: 8000 })
  } finally {
    submitting.value = false
  }
}

// ============================================================
// preview / unduh -- berkas identik untuk semua pegawai dalam satu batch,
// jadi diambil dari baris pegawai PERTAMA saja (item.pegawai[0].id).
// ============================================================
const previewDialog = ref(false)
const previewTitle = ref('')
const previewPdfUrl = ref('')
let previewObjectUrl = ''

async function lihatBatch(item) {
  const firstId = item.pegawai[0]?.id
  if (!firstId) return
  previewTitle.value = `Berita Acara -- ${formatTanggal(item.tanggal)}`
  try {
    const res = await http.get(`/absensi/dokumen/${firstId}/file`, { params: { inline: 1 }, responseType: 'blob' })
    previewObjectUrl = URL.createObjectURL(res.data)
    previewPdfUrl.value = previewObjectUrl
    previewDialog.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal membuka berkas', detail: e.response?.data?.message || e.message, life: 5000 })
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
async function unduhBatch(item) {
  const firstId = item.pegawai[0]?.id
  if (!firstId) return
  try {
    const res = await http.get(`/absensi/dokumen/${firstId}/file`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `berita_acara_${toDateStr(item.tanggal)}.pdf`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

// unduhBuktiDukung: HANYA relevan untuk batch "individu" (ada_bukti_dukung
// pada baris pegawai pertama, lihat beritaAcaraPegawaiOut di
// handlers/berita_acara.go) -- Berita Acara "kolektif" tidak mewajibkan
// bukti dukung sama sekali.
async function unduhBuktiDukung(item) {
  const firstId = item.pegawai[0]?.id
  if (!firstId) return
  try {
    const res = await http.get(`/absensi/dokumen/${firstId}/bukti-dukung`, { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `bukti_dukung_berita_acara_${toDateStr(item.tanggal)}`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 5000 })
  }
}

onMounted(() => {
  loadItems()
  loadAlasanOptions()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-header-row">
      <div style="flex: 1; min-width: 0">
        <div class="page-title">Berita Acara</div>
        <p class="page-subtitle">
          Buat Berita Acara resmi untuk pegawai yang tidak bisa melakukan absensi online lewat E-Office pada satu
          tanggal kejadian -- satu pegawai (individu) atau beberapa pegawai sekaligus (kolektif, boleh campur unit
          kerja). Tanggal kejadian otomatis tercatat DD (Dinas Dalam) pada Rekap Absen pegawai terpilih.
        </p>
      </div>
      <div class="page-header-actions">
        <Button
          v-if="selectedItems.length"
          :label="`Hapus Terpilih (${selectedItems.length})`"
          icon="pi pi-trash"
          severity="danger"
          outlined
          :loading="bulkDeleting"
          @click="hapusTerpilih"
        />
        <Button label="Buat Berita Acara" icon="pi pi-file-edit" @click="bukaBuatDialog" />
      </div>
    </div>

    <div class="toolbar-row">
      <IconField class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
        <InputText v-model="search" placeholder="Cari nomor, alasan, nama, atau NIP..." style="width: 100%" />
        <InputIcon class="pi pi-search" />
      </IconField>
    </div>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <Message v-else-if="!items.length" severity="info" :closable="false">
      Belum ada Berita Acara yang dibuat. Klik "Buat Berita Acara" untuk mulai.
    </Message>

    <div v-else class="ba-grid">
      <div v-for="item in items" :key="item.nama_file" class="ba-card">
        <div class="ba-card-select">
          <Checkbox :modelValue="isItemSelected(item)" binary @update:modelValue="toggleItemSelect(item)" />
        </div>
        <div class="ba-card-icon"><i class="pi pi-file-pdf"></i></div>
        <div class="ba-card-body">
          <div class="ba-card-top">
            <Tag :value="item.jenis === 'kolektif' ? 'Kolektif' : 'Individu'" :severity="item.jenis === 'kolektif' ? 'warn' : 'info'" />
            <Tag :value="item.alasan" severity="secondary" />
          </div>
          <div class="ba-card-nomor">NO : {{ item.nomor || '-' }}</div>
          <div class="ba-card-meta">Tanggal kejadian: {{ formatTanggal(item.tanggal) }}</div>
          <div class="ba-card-pegawai">
            <div v-for="pg in item.pegawai.slice(0, 4)" :key="pg.id" class="ba-card-pegawai-row">
              <span class="ba-card-pegawai-nama">{{ pg.nama }}</span>
              <span class="ba-card-pegawai-meta">{{ pg.nip }} &middot; {{ pg.unit_kerja }}</span>
            </div>
            <div v-if="item.pegawai.length > 4" class="ba-card-pegawai-lainnya">
              +{{ item.pegawai.length - 4 }} pegawai lainnya
            </div>
          </div>
        </div>
        <div class="ba-card-actions">
          <Button label="Lihat" icon="pi pi-eye" size="small" @click="lihatBatch(item)" />
          <Button icon="pi pi-download" size="small" severity="secondary" outlined @click="unduhBatch(item)" title="Unduh" />
          <Button
            v-if="item.pegawai[0]?.ada_bukti_dukung"
            icon="pi pi-paperclip"
            size="small"
            severity="secondary"
            outlined
            @click="unduhBuktiDukung(item)"
            title="Unduh Bukti Dukung"
          />
          <Button icon="pi pi-pencil" size="small" severity="secondary" outlined @click="bukaEditDialog(item)" title="Edit" />
          <Button icon="pi pi-trash" size="small" severity="danger" outlined @click="konfirmasiHapusBatch(item)" title="Hapus" />
        </div>
      </div>
    </div>

    <!-- dialog buat / edit (satu dialog, dua mode -- lihat editingItem) -->
    <Dialog v-model:visible="buatDialog" modal :header="isEditing ? 'Edit Berita Acara' : 'Buat Berita Acara'" style="width: 42rem; max-width: 96vw">
      <div class="form-field">
        <label>Jenis</label>
        <div style="display: flex; gap: 1.25rem; margin-top: 0.4rem">
          <label style="display: flex; align-items: center; gap: 0.5rem; font-weight: 400; cursor: pointer">
            <RadioButton v-model="form.jenis" value="individu" />
            Individu (satu pegawai)
          </label>
          <label style="display: flex; align-items: center; gap: 0.5rem; font-weight: 400; cursor: pointer">
            <RadioButton v-model="form.jenis" value="kolektif" />
            Kolektif (beberapa pegawai)
          </label>
        </div>
      </div>

      <div class="form-field" v-if="form.jenis === 'individu'">
        <label>Pegawai</label>
        <Select
          v-model="form.id_pegawai_individu"
          :options="pegawaiPicker.options.value"
          optionLabel="label"
          optionValue="value"
          filter
          :loading="pegawaiPicker.loading.value"
          filterPlaceholder="Ketik nama atau NIP pegawai"
          emptyFilterMessage="Pegawai tidak ditemukan -- coba nama atau NIP yang lain"
          placeholder="Pilih pegawai"
          style="width: 100%"
          @filter="pegawaiPicker.onFilter"
        />
      </div>
      <div class="form-field" v-else>
        <label>Pegawai</label>
        <MultiSelect
          v-model="form.id_pegawai_kolektif"
          :options="pegawaiPicker.options.value"
          optionLabel="label"
          optionValue="value"
          filter
          display="chip"
          :loading="pegawaiPicker.loading.value"
          :maxSelectedLabels="20"
          filterPlaceholder="Ketik nama atau NIP pegawai"
          emptyFilterMessage="Pegawai tidak ditemukan -- coba nama atau NIP yang lain"
          placeholder="Pilih satu atau beberapa pegawai"
          style="width: 100%"
          @filter="pegawaiPicker.onFilter"
        />
        <small class="text-muted">Boleh mencampur pegawai dari unit kerja yang berbeda dalam satu Berita Acara.</small>
      </div>

      <div class="form-field" v-if="isDinasOnly">
        <label>Penandatangan (Yang Mengetahui)</label>
        <Message severity="info" :closable="false">
          Otomatis -- Kepala Dinas/Plt Kepala Dinas sesuai menu Pengaturan Surat, karena seluruh pegawai yang dipilih
          bertempat tugas Dinas/Kantor. QR tanda tangan akan langsung disertakan pada PDF, tidak perlu dipilih manual.
        </Message>
      </div>
      <div class="form-field" v-else>
        <label>Penandatangan (Yang Mengetahui)</label>
        <Select
          v-model="form.id_penandatangan"
          :options="penandatanganPicker.options.value"
          optionLabel="label"
          optionValue="value"
          filter
          :loading="penandatanganPicker.loading.value"
          filterPlaceholder="Ketik nama atau NIP penandatangan"
          emptyFilterMessage="Pegawai tidak ditemukan -- coba nama atau NIP yang lain"
          placeholder="Pilih penandatangan"
          style="width: 100%"
          @filter="penandatanganPicker.onFilter"
        />
        <small class="text-muted">
          Kop surat &amp; tanda tangan pada Berita Acara mengikuti unit kerja &amp; jabatan penandatangan ini, bukan
          pegawai yang dipilih di atas -- pilih atasan yang memang berwenang menandatangani (misalnya Kepala
          Sekolah/Kepala Dinas).
        </small>
      </div>

      <div class="form-grid-2">
        <div class="form-field">
          <label>Alasan</label>
          <Select
            v-model="form.alasan"
            :options="alasanOptions"
            placeholder="Pilih alasan"
            style="width: 100%"
          />
        </div>
        <div class="form-field">
          <label>Nomor Urut Surat (opsional)</label>
          <InputText v-model="form.nomor_surat" placeholder="Boleh dikosongkan, cth: 483.1" style="width: 100%" />
          <small class="text-muted">
            Cukup isi nomor urutnya saja -- sistem otomatis merangkai jadi
            "800/{{ form.nomor_surat || '...' }}/Disdikbud/{bulan romawi berjalan}/{tahun berjalan}".
          </small>
        </div>
      </div>

      <div class="form-field" v-if="form.jenis === 'individu'">
        <label>Bukti Dukung (JPG/JPEG/PNG) -- {{ canReuseBuktiDukung ? 'opsional' : 'wajib' }}</label>
        <input ref="buktiDukungFileInput" type="file" accept=".jpg,.jpeg,.png" style="display: none" @change="onBuktiDukungFileChosen" />
        <Button
          :label="buktiDukungFile ? buktiDukungFile.name : 'Pilih Berkas'"
          icon="pi pi-file"
          severity="secondary"
          outlined
          @click="pickBuktiDukungFile"
        />
        <small class="text-muted" v-if="canReuseBuktiDukung">
          Biarkan kosong untuk tetap memakai bukti dukung yang sudah diupload sebelumnya, atau pilih berkas baru untuk menggantinya.
        </small>
        <small class="text-muted" v-else>Foto/scan pendukung alasan terpilih (mis. screenshot error jaringan, foto motor rusak, dsb).</small>
      </div>

      <div class="form-grid-2">
        <div class="form-field">
          <label>Tanggal Kejadian</label>
          <DatePicker v-model="form.tanggal_kejadian" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
        </div>
        <div class="form-field">
          <label>Tanggal Surat</label>
          <DatePicker v-model="form.tanggal_surat" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
          <small class="text-muted">Mengikuti Tanggal Kejadian secara otomatis kalau belum diubah manual.</small>
        </div>
      </div>

      <Message severity="info" :closable="false">
        Pegawai yang pada tanggal kejadian ini SUDAH tercatat absen masuk (hadir) akan otomatis dilewati -- kehadiran
        asli tidak pernah ditimpa Berita Acara.
      </Message>

      <template #footer>
        <Button label="Batal" severity="secondary" text @click="buatDialog = false" />
        <Button :label="isEditing ? 'Simpan Perubahan' : 'Buat Berita Acara'" :icon="isEditing ? 'pi pi-save' : 'pi pi-file-edit'" :loading="submitting" @click="submitBuat" />
      </template>
    </Dialog>

    <!-- dialog preview -->
    <Dialog v-model:visible="previewDialog" modal :header="previewTitle" style="width: 90vw; max-width: 900px" @hide="tutupPreview">
      <iframe :src="previewPdfUrl" class="preview-frame"></iframe>
    </Dialog>
  </div>
</template>

<style scoped>
.page-header-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.page-header-actions {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.toolbar-row {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.text-muted {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.85rem;
}

.ba-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1rem;
}

.ba-card {
  position: relative;
  background: var(--p-content-background, #fff);
  border-radius: 12px;
  padding: 1.1rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.ba-card-select {
  position: absolute;
  top: 0.75rem;
  right: 0.75rem;
}

.ba-card-icon {
  font-size: 2rem;
  color: #b91c1c;
}

.ba-card-top {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.ba-card-nomor {
  font-size: 0.85rem;
  font-family: monospace;
  color: #0d9488;
  margin-top: 0.2rem;
}

.ba-card-meta {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.8rem;
}

.ba-card-pegawai {
  margin-top: 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border-top: 1px dashed var(--p-content-border-color, #e2e8f0);
  padding-top: 0.4rem;
}

.ba-card-pegawai-row {
  display: flex;
  flex-direction: column;
}

.ba-card-pegawai-nama {
  font-weight: 600;
  font-size: 0.86rem;
}

.ba-card-pegawai-meta {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.76rem;
}

.ba-card-pegawai-lainnya {
  font-size: 0.78rem;
  color: var(--p-text-muted-color, #64748b);
  font-style: italic;
}

.ba-card-actions {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: auto;
  padding-top: 0.4rem;
}

.form-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.form-field {
  margin-bottom: 1rem;
}

.form-field label {
  display: block;
  font-size: 0.82rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
  color: var(--p-text-muted-color, #64748b);
}

.preview-frame {
  width: 100%;
  height: 75vh;
  border: none;
}

@media (max-width: 640px) {
  .form-grid-2 {
    grid-template-columns: 1fr;
  }
}
</style>
