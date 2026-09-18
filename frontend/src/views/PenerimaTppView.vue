<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Dialog from 'primevue/dialog'
import DatePicker from 'primevue/datepicker'
import Message from 'primevue/message'
import Paginator from 'primevue/paginator'
import ProgressSpinner from 'primevue/progressspinner'

// PenerimaTppView -- menu "Penerima TPP" (khusus administrator & admin,
// lihat auth.canManageMaster di router/index.js & AppLayout.vue).
//
// PENTING: menu ini TIDAK menampilkan seluruh pegawai secara otomatis --
// daftarnya mulai KOSONG, administrator/admin harus menambahkan pegawai
// satu-satu (pilih nama) atau sekaligus (berdasarkan Jabatan & tahun
// pengangkatan/TMT) lewat tombol di toolbar. Pegawai yang sudah tidak
// termasuk kategori penerima TPP (mis. pensiun) bisa dikeluarkan lagi lewat
// tombol hapus per baris. Backend: handlers/tpp.go, model
// models.PenerimaTpp.

const toast = useToast()

const rows = ref([])
const total = ref(0)
const kosongCount = ref(0)
const loading = ref(true)

const search = ref('')
const hanyaKosong = ref(false)
const page = ref(1)
const pageSize = ref(25)
const entriesOptions = [10, 25, 50, 100]

async function muatData() {
  loading.value = true
  try {
    const params = { page: page.value, pageSize: pageSize.value }
    if (search.value.trim()) params.q = search.value.trim()
    if (hanyaKosong.value) params.hanya_kosong = 1
    const { data: res } = await http.get('/tpp/penerima', { params })
    rows.value = res.data || []
    total.value = res.meta?.total || 0
    kosongCount.value = res.meta?.kosong_count || 0
    // buang seleksi pegawai yang tidak lagi tampil (halaman berubah dll.)
    const idPegawaiDitampilkan = new Set(rows.value.map((r) => r.id_pegawai))
    for (const id of Array.from(selectedIds.value)) {
      if (!idPegawaiDitampilkan.has(id)) selectedIds.value.delete(id)
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

watch([search, hanyaKosong], () => {
  page.value = 1
  muatData()
})
watch(pageSize, () => {
  page.value = 1
  muatData()
})

function onPage(e) {
  page.value = e.page + 1
  pageSize.value = e.rows
  muatData()
}

// ------------------------------------------------------------
// seleksi baris -- boleh mencentang SIAPA SAJA (bukan cuma yang SK-nya
// kosong): administrator juga boleh mengirim permintaan SK ke pegawai yang
// SK Terakhir-nya SUDAH ADA, mis. minta diperiksa ulang/diganti kalau
// ternyata kurang sesuai (lihat kirimPermintaanSk di handlers/tpp.go &
// notifikasi dashboard pegawai yang membedakan tampilannya). Kunci seleksi
// adalah ID PEGAWAI (row.id_pegawai), BUKAN row.id (yang sekarang adalah ID
// baris keanggotaan Penerima TPP).
// ------------------------------------------------------------
const selectedIds = ref(new Set())

function skKosong(row) {
  return !row.sk_terakhir_nama
}

function toggleSelect(row) {
  const s = new Set(selectedIds.value)
  if (s.has(row.id_pegawai)) s.delete(row.id_pegawai)
  else s.add(row.id_pegawai)
  selectedIds.value = s
}

// centang semua / kosongkan semua baris pada HALAMAN yang sedang tampil
// (dipakai checkbox pada header kolom "Pilih").
const semuaTerpilihHalIni = computed(() => rows.value.length > 0 && rows.value.every((r) => selectedIds.value.has(r.id_pegawai)))
function toggleSemuaHalIni() {
  const s = new Set(selectedIds.value)
  if (semuaTerpilihHalIni.value) {
    for (const r of rows.value) s.delete(r.id_pegawai)
  } else {
    for (const r of rows.value) s.add(r.id_pegawai)
  }
  selectedIds.value = s
}

function pilihSemuaKosongHalIni() {
  const s = new Set(selectedIds.value)
  for (const r of rows.value) {
    if (skKosong(r)) s.add(r.id_pegawai)
  }
  selectedIds.value = s
}

const jumlahTerpilih = computed(() => selectedIds.value.size)

// ------------------------------------------------------------
// dialog "Kirim Permintaan SK"
// ------------------------------------------------------------
const kirimDialog = ref(false)
const batasTanggal = ref(null)
const kirimLoading = ref(false)

async function bukaKirimDialog() {
  try {
    const { data: res } = await http.get('/pengaturan-tpp')
    const v = res.data?.batas_tanggal_upload
    batasTanggal.value = v ? new Date(v) : new Date(Date.now() + 30 * 24 * 60 * 60 * 1000)
  } catch {
    batasTanggal.value = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000)
  }
  kirimDialog.value = true
}

function toDateStr(d) {
  if (!d) return ''
  const dt = new Date(d)
  const pad = (n) => String(n).padStart(2, '0')
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
}

async function kirimPermintaan() {
  if (!batasTanggal.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih batas tanggal upload SK', life: 4000 })
    return
  }
  if (jumlahTerpilih.value === 0) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih minimal satu pegawai', life: 4000 })
    return
  }
  kirimLoading.value = true
  try {
    const { data: res } = await http.post('/tpp/permintaan-sk', {
      id_pegawai: Array.from(selectedIds.value),
      batas_tanggal: toDateStr(batasTanggal.value),
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: res.message, life: 4000 })
    kirimDialog.value = false
    selectedIds.value = new Set()
    await muatData()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengirim', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    kirimLoading.value = false
  }
}

// ------------------------------------------------------------
// dialog "Tambah Pegawai" (pilih nama satu-satu / beberapa sekaligus)
// ------------------------------------------------------------
const tambahDialog = ref(false)
const tambahLoading = ref(false)
const calonOptions = ref([])
const calonLoading = ref(false)
const calonSelected = ref([])
const calonTerpilihCache = ref([])
let calonFilterTimer = null

function toCalonOption(p) {
  return { label: `${p.nama} (${p.nip})`, value: p.id }
}
function gabungCalonDenganTerpilih(list) {
  const adaDiHasil = new Set(list.map((o) => o.value))
  return [...calonTerpilihCache.value.filter((o) => !adaDiHasil.has(o.value)), ...list]
}
async function loadCalonOptions(keyword = '') {
  calonLoading.value = true
  try {
    const params = { pageSize: 100 }
    if (keyword.trim()) params.q = keyword.trim()
    const { data } = await http.get('/tpp/calon-pegawai', { params })
    const list = Array.isArray(data.data) ? data.data : []
    calonOptions.value = gabungCalonDenganTerpilih(list.map(toCalonOption))
  } catch {
    calonOptions.value = gabungCalonDenganTerpilih([])
  } finally {
    calonLoading.value = false
  }
}
function onCalonFilter(e) {
  clearTimeout(calonFilterTimer)
  calonFilterTimer = setTimeout(() => loadCalonOptions(e?.value || ''), 300)
}
watch(calonSelected, (ids) => {
  const dikenal = new Map([...calonTerpilihCache.value, ...calonOptions.value].map((o) => [o.value, o]))
  calonTerpilihCache.value = ids.map((id) => dikenal.get(id)).filter(Boolean)
})

function bukaTambahDialog() {
  calonSelected.value = []
  calonTerpilihCache.value = []
  loadCalonOptions('')
  tambahDialog.value = true
}

async function submitTambah() {
  if (calonSelected.value.length === 0) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih minimal satu pegawai', life: 4000 })
    return
  }
  tambahLoading.value = true
  try {
    const { data: res } = await http.post('/tpp/penerima', { id_pegawai: calonSelected.value })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: res.message, life: 4000 })
    tambahDialog.value = false
    page.value = 1
    await muatData()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menambahkan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    tambahLoading.value = false
  }
}

// ------------------------------------------------------------
// dialog "Tambah dari Jabatan & Tahun Pengangkatan" (bulk)
// ------------------------------------------------------------
const tambahKriteriaDialog = ref(false)
const tambahKriteriaLoading = ref(false)
const refJabatan = ref([])
const kriteriaJabatan = ref(null)
const kriteriaTahun = ref(null)

async function loadRefJabatan() {
  try {
    const { data } = await http.get('/ref/jabatan')
    refJabatan.value = data.data || []
  } catch {
    refJabatan.value = []
  }
}

function bukaTambahKriteriaDialog() {
  kriteriaJabatan.value = null
  kriteriaTahun.value = null
  if (refJabatan.value.length === 0) loadRefJabatan()
  tambahKriteriaDialog.value = true
}

async function submitTambahKriteria() {
  if (!kriteriaJabatan.value) {
    toast.add({ severity: 'warn', summary: 'Belum lengkap', detail: 'Pilih jabatan terlebih dahulu', life: 4000 })
    return
  }
  tambahKriteriaLoading.value = true
  try {
    const { data: res } = await http.post('/tpp/penerima/by-kriteria', {
      id_jabatan: kriteriaJabatan.value,
      tahun_pengangkatan: kriteriaTahun.value || 0,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: res.message, life: 5000 })
    tambahKriteriaDialog.value = false
    page.value = 1
    await muatData()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menambahkan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    tambahKriteriaLoading.value = false
  }
}

// ------------------------------------------------------------
// dialog hapus (keluarkan pegawai dari Penerima TPP, mis. pensiun/mutasi)
// ------------------------------------------------------------
const hapusDialog = ref(false)
const hapusLoading = ref(false)
const hapusTarget = ref(null)
const hapusAlasanPilihan = ref(null)
const hapusAlasanLainnya = ref('')
const ALASAN_HAPUS_OPTIONS = ['Pensiun', 'Mutasi / Pindah Tugas', 'Berhenti', 'Lainnya']

function bukaHapusDialog(row) {
  hapusTarget.value = row
  hapusAlasanPilihan.value = null
  hapusAlasanLainnya.value = ''
  hapusDialog.value = true
}

async function submitHapus() {
  if (!hapusTarget.value) return
  hapusLoading.value = true
  try {
    const alasan = hapusAlasanPilihan.value === 'Lainnya' ? hapusAlasanLainnya.value.trim() : hapusAlasanPilihan.value || ''
    const { data: res } = await http.delete(`/tpp/penerima/${hapusTarget.value.id}`, { data: { alasan } })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: res.message, life: 4000 })
    hapusDialog.value = false
    await muatData()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengeluarkan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    hapusLoading.value = false
  }
}

// ------------------------------------------------------------
// unduh SK Terakhir yang sudah ada (dipakai administrator untuk memeriksa
// dokumen yang sudah diupload)
// ------------------------------------------------------------
async function unduhSk(row) {
  try {
    const res = await http.get(`/pegawai/${row.id_pegawai}/dokumen/sk-terakhir`, { responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const link = document.createElement('a')
    link.href = url
    link.download = row.sk_terakhir_nama || 'sk-terakhir'
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal mengunduh', detail: e.response?.data?.message || e.message, life: 4000 })
  }
}

function formatTanggal(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  return d.toLocaleDateString('id-ID', { year: 'numeric', month: 'short', day: 'numeric' })
}

onMounted(() => {
  muatData()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Penerima TPP</div>
    <p class="page-subtitle">
      Daftar pegawai yang termasuk kategori penerima TPP beserta status SK Terakhir. Menu ini tidak menampilkan
      seluruh pegawai secara otomatis -- tambahkan pegawai lewat tombol "Tambah Pegawai" (pilih nama) atau "Tambah
      dari Jabatan &amp; Tahun" (sekaligus banyak pegawai), dan keluarkan pegawai yang sudah tidak termasuk kategori
      ini (mis. pensiun) lewat tombol hapus pada kolom Aksi.
    </p>

    <div class="card">
      <div class="rekap-toolbar">
        <IconField class="table-search" style="min-width: 220px; max-width: 320px; flex: 1">
          <InputText v-model="search" placeholder="Cari nama atau NIP..." style="width: 100%" />
          <InputIcon class="pi pi-search" />
        </IconField>
        <div style="display: flex; align-items: center; gap: 0.5rem">
          <Checkbox v-model="hanyaKosong" binary inputId="hanyaKosong" />
          <label for="hanyaKosong" style="cursor: pointer">Hanya SK Kosong</label>
        </div>
        <Button label="Tambah Pegawai" icon="pi pi-user-plus" @click="bukaTambahDialog" />
        <Button label="Tambah dari Jabatan & Tahun" icon="pi pi-sitemap" severity="secondary" outlined @click="bukaTambahKriteriaDialog" />
        <Button
          v-if="kosongCount > 0"
          label="Pilih semua SK kosong (hal. ini)"
          icon="pi pi-check-square"
          severity="secondary"
          outlined
          size="small"
          @click="pilihSemuaKosongHalIni"
        />
        <Button
          v-if="total > 0"
          :label="`Kirim Permintaan SK${jumlahTerpilih ? ' (' + jumlahTerpilih + ')' : ''}`"
          icon="pi pi-send"
          severity="warn"
          :disabled="jumlahTerpilih === 0"
          @click="bukaKirimDialog"
        />
      </div>

      <Message v-if="kosongCount > 0" severity="warn" :closable="false" style="margin-bottom: 1rem">
        Ada {{ kosongCount }} pegawai Penerima TPP yang SK Terakhir-nya belum diupload. Centang pegawai yang ingin
        diminta pada kolom "Pilih" (atau centang kolom header untuk memilih semua di halaman ini) lalu klik "Kirim
        Permintaan SK". Pegawai yang SK-nya sudah ada juga boleh dicentang & diminta lagi, misalnya untuk minta
        diperiksa ulang atau diganti kalau kurang sesuai.
      </Message>

      <div v-if="loading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>

      <div v-else class="responsive-table-wrap">
        <DataTable :value="rows" size="small" style="min-width: 950px">
          <template #empty>
            Belum ada pegawai yang ditambahkan ke Penerima TPP. Klik "Tambah Pegawai" atau "Tambah dari Jabatan &amp;
            Tahun" di atas untuk menambahkan.
          </template>
          <Column style="width: 4rem">
            <template #header>
              <Checkbox :modelValue="semuaTerpilihHalIni" binary @update:modelValue="toggleSemuaHalIni" />
            </template>
            <template #body="{ data: row }">
              <Checkbox :modelValue="selectedIds.has(row.id_pegawai)" binary @update:modelValue="toggleSelect(row)" />
            </template>
          </Column>
          <Column header="No" style="width: 3.5rem">
            <template #body="{ index }">{{ (page - 1) * pageSize + index + 1 }}</template>
          </Column>
          <Column field="nip" header="NIP" />
          <Column field="nama" header="Nama" />
          <Column field="jabatan" header="Jabatan">
            <template #body="{ data: row }">{{ row.jabatan || '-' }}</template>
          </Column>
          <Column field="unit_kerja" header="Unit Kerja">
            <template #body="{ data: row }">{{ row.unit_kerja || '-' }}</template>
          </Column>
          <Column header="SK Terakhir">
            <template #body="{ data: row }">
              <Button
                v-if="row.sk_terakhir_nama"
                :label="row.sk_terakhir_nama"
                icon="pi pi-file"
                link
                size="small"
                style="padding: 0; max-width: 220px; overflow: hidden; text-overflow: ellipsis"
                @click="unduhSk(row)"
              />
              <Tag v-else value="Belum Ada" severity="danger" />
            </template>
          </Column>
          <Column header="Status Permintaan">
            <template #body="{ data: row }">
              <Tag
                v-if="row.permintaan_sk"
                severity="warn"
                :value="`Menunggu ${skKosong(row) ? 'upload' : 'verifikasi ulang'} (batas ${formatTanggal(row.permintaan_sk.batas_tanggal)})`"
              />
              <span v-else>-</span>
            </template>
          </Column>
          <Column header="Aksi" style="width: 5rem">
            <template #body="{ data: row }">
              <Button icon="pi pi-trash" severity="danger" text rounded aria-label="Keluarkan dari Penerima TPP" @click="bukaHapusDialog(row)" />
            </template>
          </Column>
        </DataTable>

        <div style="display: flex; align-items: center; justify-content: space-between; margin-top: 0.75rem; flex-wrap: wrap; gap: 0.5rem">
          <div class="entries-picker">
            <span class="entries-picker-label">Tampilkan</span>
            <Select v-model="pageSize" :options="entriesOptions" style="width: 5.5rem" />
          </div>
          <Paginator :rows="pageSize" :totalRecords="total" :first="(page - 1) * pageSize" @page="onPage" />
        </div>
      </div>
    </div>

    <!-- Tambah Pegawai (pilih nama) -->
    <Dialog v-model:visible="tambahDialog" header="Tambah Pegawai ke Penerima TPP" modal style="width: 32rem">
      <label class="field-label">Pilih Pegawai</label>
      <MultiSelect
        v-model="calonSelected"
        :options="calonOptions"
        optionLabel="label"
        optionValue="value"
        filter
        :loading="calonLoading"
        display="chip"
        filterPlaceholder="Ketik nama / NIP"
        placeholder="Pilih satu atau beberapa pegawai..."
        style="width: 100%"
        @filter="onCalonFilter"
      />
      <small style="color: var(--p-text-muted-color); display: block; margin-top: 0.5rem">
        Pegawai yang sudah menjadi anggota Penerima TPP tidak akan muncul di pilihan ini.
      </small>
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="tambahDialog = false" />
        <Button label="Tambahkan" icon="pi pi-user-plus" :loading="tambahLoading" @click="submitTambah" />
      </template>
    </Dialog>

    <!-- Tambah dari Jabatan & Tahun Pengangkatan -->
    <Dialog v-model:visible="tambahKriteriaDialog" header="Tambah dari Jabatan & Tahun Pengangkatan" modal style="width: 28rem">
      <p style="margin-top: 0">
        Semua pegawai dengan jabatan ini (dan tahun pengangkatan/TMT ini, kalau diisi) akan ditambahkan sekaligus ke
        Penerima TPP.
      </p>
      <label class="field-label">Jabatan</label>
      <Select
        v-model="kriteriaJabatan"
        :options="refJabatan"
        optionLabel="jabatan"
        optionValue="id"
        filter
        showClear
        placeholder="Pilih jabatan..."
        style="width: 100%"
      />
      <label class="field-label">Tahun Pengangkatan (TMT) -- opsional</label>
      <InputNumber
        v-model="kriteriaTahun"
        :useGrouping="false"
        :min="1950"
        :max="2100"
        placeholder="Kosongkan untuk semua tahun"
        style="width: 100%"
      />
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="tambahKriteriaDialog = false" />
        <Button label="Tambahkan" icon="pi pi-sitemap" :loading="tambahKriteriaLoading" @click="submitTambahKriteria" />
      </template>
    </Dialog>

    <!-- Kirim Permintaan SK -->
    <Dialog v-model:visible="kirimDialog" header="Kirim Permintaan SK" modal style="width: 28rem">
      <p>
        Permintaan SK Terakhir akan dikirim ke <strong>{{ jumlahTerpilih }}</strong> pegawai terpilih dan muncul sebagai
        notifikasi pada dashboard akun masing-masing. Pegawai yang belum punya SK akan diminta mengupload, sedangkan
        yang sudah punya SK akan diminta memeriksa ulang (bisa upload ulang kalau SK yang ada kurang sesuai).
      </p>
      <label class="field-label">Batas Tanggal Upload</label>
      <DatePicker v-model="batasTanggal" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="kirimDialog = false" />
        <Button label="Kirim Permintaan" icon="pi pi-send" :loading="kirimLoading" @click="kirimPermintaan" />
      </template>
    </Dialog>

    <!-- Keluarkan dari Penerima TPP -->
    <Dialog v-model:visible="hapusDialog" header="Keluarkan dari Penerima TPP" modal style="width: 26rem">
      <p style="margin-top: 0">
        Keluarkan <strong>{{ hapusTarget?.nama }}</strong> dari daftar Penerima TPP? Pegawai ini bisa ditambahkan
        kembali kapan saja lewat "Tambah Pegawai".
      </p>
      <label class="field-label">Alasan (opsional)</label>
      <Select
        v-model="hapusAlasanPilihan"
        :options="ALASAN_HAPUS_OPTIONS"
        showClear
        placeholder="Pilih alasan..."
        style="width: 100%"
      />
      <Textarea
        v-if="hapusAlasanPilihan === 'Lainnya'"
        v-model="hapusAlasanLainnya"
        rows="2"
        placeholder="Tulis alasan lainnya..."
        style="width: 100%; margin-top: 0.5rem"
      />
      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="hapusDialog = false" />
        <Button label="Keluarkan" icon="pi pi-trash" severity="danger" :loading="hapusLoading" @click="submitHapus" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin: 0.75rem 0 0.35rem;
}
.rekap-toolbar {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  align-items: center;
  margin-bottom: 1rem;
}
</style>
