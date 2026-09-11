<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'
import { toApiDate } from '../utils/date'

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

const toast = useToast()
const confirm = useConfirm()

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
  jam_mulai_pulang: '',
  tempat_tugas_allowed: [],
  jabatan_allowed_ids: [],
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

const tempatTugasOptions = ref([])
const jabatanOptions = ref([])

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

const jamPattern = /^([01]\d|2[0-3]):[0-5]\d$/
function jamValid(v) {
  return jamPattern.test(v || '')
}
const pengaturanValid = computed(
  () => jamValid(pengaturan.jam_mulai_pagi) && jamValid(pengaturan.jam_batas_pagi) && jamValid(pengaturan.jam_mulai_pulang),
)

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

async function loadPegawaiOptions() {
  try {
    const { data } = await http.get('/pegawai', { params: { pageSize: 500 } })
    const list = data.data || []
    pegawaiOptions.value = list.map((p) => ({ label: `${p.nama} (${p.nip})`, value: p.id }))
  } catch {
    pegawaiOptions.value = []
  }
}

async function loadRekap() {
  loadingRekap.value = true
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    if (selectedPegawai.value) params.id_pegawai = selectedPegawai.value
    const { data } = await http.get('/absensi/rekap', { params })
    rekap.value = data.data?.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat rekap', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loadingRekap.value = false
  }
}

watch([periodDate, selectedPegawai], () => loadRekap())
watch(periodDate, () => loadDokumenAdmin())

onMounted(async () => {
  await Promise.all([loadPengaturan(), loadPegawaiOptions(), loadTempatTugasOptions(), loadJabatanOptions()])
  await Promise.all([loadRekap(), loadDokumenAdmin()])
})

function jumlahHadir(item) {
  return item.absensi.filter((a) => a.jam_masuk).length
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
function formatKoordinat(row, jenis) {
  const lat = jenis === 'masuk' ? row.lat_masuk : row.lat_pulang
  const lng = jenis === 'masuk' ? row.lng_masuk : row.lng_pulang
  if (lat == null || lng == null) return '-'
  return `${Number(lat).toFixed(5)}, ${Number(lng).toFixed(5)}`
}

// ============================================================
// lihat foto absen masuk/pulang (admin/administrator)
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
    await Promise.all([loadRekap(), loadDokumenAdmin()])
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

async function loadDokumenAdmin() {
  loadingDokumenAdmin.value = true
  try {
    const params = { bulan: periodDate.value.getMonth() + 1, tahun: periodDate.value.getFullYear() }
    const { data } = await http.get('/absensi/dokumen/rekap', { params })
    dokumenAdminList.value = data.data || []
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
    <p class="page-subtitle">Rekap kehadiran seluruh pegawai, pengaturan jendela waktu absen, dan aktif/nonaktifkan menu Absen.</p>

    <div class="card" style="max-width: 40rem; margin-bottom: 1.5rem">
      <div v-if="loadingPengaturan" style="display: flex; justify-content: center; padding: 1.5rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <div v-else style="display: flex; flex-direction: column; gap: 1rem">
        <div style="display: flex; align-items: center; gap: 0.75rem">
          <ToggleSwitch v-model="pengaturan.aktif" />
          <span>Menu Absen {{ pengaturan.aktif ? 'aktif' : 'nonaktif' }} untuk pegawai</span>
        </div>
        <Message v-if="!pengaturan.aktif" severity="warn" :closable="false">
          Pegawai tidak akan bisa absen selama menu ini nonaktif.
        </Message>
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
            <label class="field-label">Jam Mulai Absen Pulang</label>
            <InputText v-model="pengaturan.jam_mulai_pulang" placeholder="15:00" style="width: 100%" />
          </div>
        </div>
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
        <Message severity="info" :closable="false">
          Pegawai yang tempat tugas &amp; jabatannya tidak cocok dengan filter di atas akan melihat pesan "menu ini
          bukan untuk Anda" saat membuka menu Absen. Kosongkan kedua filter untuk membuka menu Absen bagi semua pegawai.
        </Message>

        <div>
          <label class="field-label">Titik Koordinat Kantor &amp; Radius Absen</label>
          <div class="kantor-grid">
            <InputNumber v-model="pengaturan.kantor_lat" placeholder="Lintang (lat)" :minFractionDigits="6" :maxFractionDigits="6" style="width: 100%" />
            <InputNumber v-model="pengaturan.kantor_lng" placeholder="Bujur (lng)" :minFractionDigits="6" :maxFractionDigits="6" style="width: 100%" />
            <InputNumber v-model="pengaturan.radius_meter" placeholder="Radius (meter)" suffix=" m" :min="1" style="width: 100%" />
          </div>
          <div class="kantor-actions">
            <Button label="Ambil Lokasi Saat Ini" icon="pi pi-map-marker" size="small" outlined :loading="locatingKantor" @click="ambilLokasiKantor" />
            <Button v-if="pengaturan.kantor_lat != null" label="Hapus Titik Kantor" icon="pi pi-times" size="small" text severity="danger" @click="hapusLokasiKantor" />
          </div>
          <small class="text-muted">
            Kalau diisi, kamera absen hanya akan terbuka jika pegawai berada dalam radius ini dari titik kantor.
            Kosongkan (Hapus Titik Kantor) untuk menonaktifkan pembatasan lokasi. Gunakan "Ambil Lokasi Saat Ini" saat
            Anda berada di titik kantor yang ingin dijadikan patokan.
          </small>
        </div>

        <div>
          <Button label="Simpan Pengaturan" icon="pi pi-save" :loading="savingPengaturan" @click="savePengaturan" />
        </div>
      </div>
    </div>

    <div class="card">
      <div class="rekap-toolbar">
        <DatePicker v-model="periodDate" view="month" dateFormat="MM yy" showIcon style="width: 180px" />
        <Select v-model="selectedPegawai" :options="pegawaiOptions" optionLabel="label" optionValue="value" filter showClear placeholder="Semua pegawai" style="min-width: 220px" />
        <Button label="Export Excel" icon="pi pi-file-excel" severity="success" outlined @click="exportExcel" />
      </div>

      <DataTable :value="rekap" :loading="loadingRekap" size="small" stripedRows responsiveLayout="scroll">
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
        <Column header="Aksi">
          <template #body="{ data }">
            <Button icon="pi pi-eye" size="small" text rounded title="Lihat Detail" @click="openDetail(data)" />
          </template>
        </Column>
        <template #empty>Tidak ada data pegawai.</template>
      </DataTable>
    </div>

    <Dialog v-model:visible="detailDialog" modal :header="`Detail Absen -- ${detailItem?.pegawai?.nama || ''}`" :style="{ width: '640px' }">
      <template v-if="detailItem">
        <h4>Riwayat Absen</h4>
        <DataTable :value="detailItem.absensi" size="small" stripedRows responsiveLayout="scroll">
          <Column header="Tanggal">
            <template #body="{ data }">{{ formatTanggal(dateKey(data.tanggal)) }}</template>
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
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Koordinat">
            <template #body="{ data }">{{ formatKoordinat(data, data.jam_pulang ? 'pulang' : 'masuk') }}</template>
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
    <Dialog v-model:visible="fotoDialog" modal :header="fotoDialogTitle" :style="{ width: '420px' }" @hide="closeFotoDialog">
      <img v-if="fotoDialogUrl" :src="fotoDialogUrl" style="width: 100%; border-radius: 8px" alt="Foto absen" />
    </Dialog>

    <!-- ================= input surat kolektif (BA/Surat Tugas/Izin/SKS) ================= -->
    <div class="card" style="margin-top: 1.5rem">
      <h3 style="margin-top: 0">Input Surat Kolektif (BA / Surat Tugas / Surat Izin / SKS)</h3>
      <p class="text-muted">
        Input surat pendukung untuk beberapa pegawai &amp; rentang tanggal sekaligus -- tanggal yang tercover akan
        terbaca DD (Dinas Dalam) untuk Surat Tugas/Berita Acara, I (Izin) untuk Surat Izin, atau S (Sakit) untuk SKS
        pada riwayat/rekap pegawai bersangkutan.
      </p>
      <div class="kolektif-form">
        <div>
          <label class="field-label">Pegawai</label>
          <MultiSelect
            v-model="kolektifForm.id_pegawai"
            :options="pegawaiOptions"
            optionLabel="label"
            optionValue="value"
            filter
            display="chip"
            placeholder="Pilih satu atau beberapa pegawai"
            style="width: 100%"
          />
        </div>
        <div class="kolektif-grid">
          <div>
            <label class="field-label">Tanggal Mulai</label>
            <DatePicker v-model="kolektifForm.tanggal_mulai" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Tanggal Selesai</label>
            <DatePicker v-model="kolektifForm.tanggal_selesai" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
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
        </div>
        <div>
          <label class="field-label">Keterangan (opsional)</label>
          <Textarea v-model="kolektifForm.keterangan" rows="2" style="width: 100%" />
        </div>
        <div>
          <label class="field-label">Berkas (PDF/JPG/PNG)</label>
          <input ref="kolektifFileInput" type="file" accept=".pdf,.jpg,.jpeg,.png" style="display: none" @change="onKolektifFileChosen" />
          <Button :label="kolektifFile ? kolektifFile.name : 'Pilih Berkas'" icon="pi pi-file" severity="secondary" outlined @click="pickKolektifFile" />
        </div>
        <div>
          <Button label="Input Surat" icon="pi pi-upload" :loading="submittingKolektif" @click="submitKolektif" />
        </div>
      </div>

      <h4 style="margin-top: 1.75rem">Surat yang Sudah Diinput Bulan Ini</h4>
      <DataTable :value="dokumenAdminList" :loading="loadingDokumenAdmin" size="small" stripedRows responsiveLayout="scroll">
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
.kolektif-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 40rem;
}
.kolektif-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 1rem;
}
</style>
