<script setup>
// Kp4FormPanel: form isi/ubah data KP4 satu pegawai -- dipakai DUA tempat:
// Kp4View.vue (pegawai isi data SENDIRI, idPegawai=null -> panggil
// /kp4/saya) dan Kp4AdminView.vue (administrator/admin mengedit data KP4
// pegawai MANAPUN, idPegawai diisi -> panggil /kp4/pegawai/{id}). Tidak ada
// proses approval -- begitu "Simpan Data KP4" diklik, data langsung
// tersimpan.
import { ref, reactive, computed, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import { toApiDate } from '../utils/date'
import { jenisJabatanLabels, jenisKelaminOptions, statusAnakOptions } from '../config/tables'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import Checkbox from 'primevue/checkbox'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import Divider from 'primevue/divider'

const props = defineProps({
  idPegawai: { type: [Number, String], default: null },
})
const emit = defineEmits(['saved'])

const toast = useToast()
const loading = ref(true)
const saving = ref(false)
const printing = ref(false)
const ringkasan = ref(null)

const form = reactive({
  tempat_lahir: '',
  jenis_kelamin: null,
  agama: '',
  alamat_jalan: '',
  desa: '',
  kecamatan: '',
  kabupaten: '',
  provinsi: '',
  digaji_menurut: '',
  besarnya_penghasilan: 0,
  sk_terakhir: '',
})

const punyaPasangan = ref(false)
const pasangan = reactive({
  nama: '',
  tempat_lahir: '',
  tgl_lahir: null,
  nik: '',
  pekerjaan: '',
  tgl_perkawinan: null,
  pasangan_ke: 1,
  penghasilan: 0,
})

const anakList = ref([])

function anakBaru() {
  return {
    nama: '',
    tempat_lahir: '',
    tgl_lahir: null,
    status_anak: 'kandung',
    dari_pasangan_ke: 1,
    jenis_kelamin: 'L',
    dapat_tunjangan: true,
    sudah_kawin: false,
    sudah_bekerja: false,
    masih_sekolah: true,
    no_putusan_pengadilan: '',
  }
}

function tambahAnak() {
  anakList.value.push(anakBaru())
}

function hapusAnak(idx) {
  anakList.value.splice(idx, 1)
}

const basePath = computed(() => (props.idPegawai ? `/kp4/pegawai/${props.idPegawai}` : '/kp4/saya'))

function isiFormDariRingkasan(data) {
  ringkasan.value = data
  const kp4 = data.kp4_data || {}
  form.tempat_lahir = kp4.tempat_lahir || ''
  form.jenis_kelamin = kp4.jenis_kelamin || null
  form.agama = kp4.agama || ''
  form.alamat_jalan = kp4.alamat_jalan || ''
  form.desa = kp4.desa || ''
  form.kecamatan = kp4.kecamatan || ''
  form.kabupaten = kp4.kabupaten || ''
  form.provinsi = kp4.provinsi || ''
  form.digaji_menurut = kp4.digaji_menurut || ''
  form.besarnya_penghasilan = kp4.besarnya_penghasilan || 0
  form.sk_terakhir = kp4.sk_terakhir || ''

  punyaPasangan.value = !!data.pasangan
  const p = data.pasangan || {}
  pasangan.nama = p.nama || ''
  pasangan.tempat_lahir = p.tempat_lahir || ''
  pasangan.tgl_lahir = p.tgl_lahir ? new Date(p.tgl_lahir) : null
  pasangan.nik = p.nik || ''
  pasangan.pekerjaan = p.pekerjaan || ''
  pasangan.tgl_perkawinan = p.tgl_perkawinan ? new Date(p.tgl_perkawinan) : null
  pasangan.pasangan_ke = p.pasangan_ke || 1
  pasangan.penghasilan = p.penghasilan || 0

  anakList.value = (data.anak || []).map((a) => ({
    nama: a.nama || '',
    tempat_lahir: a.tempat_lahir || '',
    tgl_lahir: a.tgl_lahir ? new Date(a.tgl_lahir) : null,
    status_anak: a.status_anak || 'kandung',
    dari_pasangan_ke: a.dari_pasangan_ke || 1,
    jenis_kelamin: a.jenis_kelamin || 'L',
    dapat_tunjangan: !!a.dapat_tunjangan,
    sudah_kawin: !!a.sudah_kawin,
    sudah_bekerja: !!a.sudah_bekerja,
    masih_sekolah: a.masih_sekolah !== false,
    no_putusan_pengadilan: a.no_putusan_pengadilan || '',
  }))
}

async function load() {
  loading.value = true
  try {
    const { data } = await http.get(basePath.value)
    isiFormDariRingkasan(data.data)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat data KP4', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

async function simpan() {
  saving.value = true
  try {
    const payload = {
      ...form,
      pasangan: punyaPasangan.value
        ? {
            ...pasangan,
            tgl_lahir: toApiDate(pasangan.tgl_lahir),
            tgl_perkawinan: toApiDate(pasangan.tgl_perkawinan),
          }
        : null,
      anak: anakList.value.map((a) => ({ ...a, tgl_lahir: toApiDate(a.tgl_lahir) })),
    }
    const { data } = await http.put(basePath.value, payload)
    isiFormDariRingkasan(data.data)
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Data KP4 berhasil disimpan', life: 3000 })
    emit('saved', data.data)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    saving.value = false
  }
}

async function cetak() {
  printing.value = true
  try {
    const res = await http.get(`${basePath.value}/cetak`, { responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data], { type: 'application/pdf' }))
    window.open(url, '_blank')
    setTimeout(() => window.URL.revokeObjectURL(url), 60000)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal membuat PDF', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    printing.value = false
  }
}

function formatDate(v) {
  if (!v) return '-'
  return new Date(v).toLocaleDateString('id-ID', { day: '2-digit', month: 'long', year: 'numeric' })
}

defineExpose({ reload: load })

onMounted(load)
</script>

<template>
  <div>
    <div v-if="loading" style="display: flex; justify-content: center; padding: 2rem">
      <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
    </div>

    <div v-else-if="ringkasan" style="display: flex; flex-direction: column; gap: 1.25rem">
      <Message
        v-if="ringkasan.kelengkapan.data_pegawai_kosong && ringkasan.kelengkapan.data_pegawai_kosong.length"
        severity="warn"
        :closable="false"
      >
        <div>
          <strong>Data dasar pegawai belum lengkap:</strong> {{ ringkasan.kelengkapan.data_pegawai_kosong.join(', ') }}.
          <span v-if="!idPegawai">Silakan lengkapi lewat menu Profil Saya &rarr; Ajukan Perubahan Data.</span>
          <span v-else>Silakan lengkapi lewat menu Data Pegawai.</span>
        </div>
      </Message>

      <div class="card">
        <div class="section-title">Data Pegawai (dari Data Pegawai)</div>
        <div class="detail-grid">
          <div><span class="detail-label">Nama Lengkap</span><div>{{ ringkasan.pegawai.nama || '-' }}</div></div>
          <div><span class="detail-label">NIP</span><div>{{ ringkasan.pegawai.nip || '-' }}</div></div>
          <div>
            <span class="detail-label">Pangkat / Golongan</span>
            <div>{{ ringkasan.pegawai.pangkat_gol?.pangkat?.pangkat || '-' }} / {{ ringkasan.pegawai.pangkat_gol?.gol?.gol || '-' }}</div>
          </div>
          <div><span class="detail-label">TMT</span><div>{{ formatDate(ringkasan.pegawai.tmt) }}</div></div>
          <div><span class="detail-label">Tanggal Lahir</span><div>{{ formatDate(ringkasan.pegawai.tgl_lahir) }}</div></div>
          <div><span class="detail-label">Status Kepegawaian</span><div>{{ ringkasan.pegawai.status?.status || '-' }}</div></div>
          <div>
            <span class="detail-label">Jenis Jabatan</span>
            <div>{{ jenisJabatanLabels[ringkasan.pegawai.jabatan?.jenis_jabatan] || '-' }}</div>
          </div>
        </div>
        <div class="stat-row">
          <Tag severity="info">Jumlah Keluarga Tertanggung: {{ ringkasan.jumlah_keluarga_tertanggung }} Orang</Tag>
          <Tag severity="info">Masa Kerja Golongan: {{ ringkasan.masa_kerja_golongan }}</Tag>
          <Tag severity="info">Masa Kerja Keseluruhan: {{ ringkasan.masa_kerja_keseluruhan }}</Tag>
        </div>
      </div>

      <div class="card">
        <div class="section-title">Data Tambahan KP4</div>
        <div class="form-grid">
          <div>
            <label class="field-label">Tempat Lahir</label>
            <InputText v-model="form.tempat_lahir" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Jenis Kelamin</label>
            <Select v-model="form.jenis_kelamin" :options="jenisKelaminOptions" optionLabel="label" optionValue="id" showClear style="width: 100%" placeholder="Pilih..." />
          </div>
          <div>
            <label class="field-label">Agama</label>
            <InputText v-model="form.agama" style="width: 100%" placeholder="mis. Islam" />
          </div>
          <div>
            <label class="field-label">Digaji Menurut (PP/SK)</label>
            <InputText v-model="form.digaji_menurut" style="width: 100%" placeholder="mis. SK Nomor .../Tanggal ..." />
          </div>
          <div>
            <label class="field-label">Besarnya Penghasilan</label>
            <InputNumber v-model="form.besarnya_penghasilan" mode="currency" currency="IDR" locale="id-ID" :minFractionDigits="0" style="width: 100%" fluid />
          </div>
          <div>
            <label class="field-label">SK Terakhir yang Dimiliki</label>
            <InputText v-model="form.sk_terakhir" style="width: 100%" placeholder="mis. SK PPPK / SK PNS" />
          </div>
        </div>

        <Divider />
        <div class="section-subtitle">Alamat Lengkap</div>
        <div class="form-grid">
          <div style="grid-column: 1 / -1">
            <label class="field-label">Jalan / RT / RW</label>
            <InputText v-model="form.alamat_jalan" style="width: 100%" placeholder="mis. Jl. Trans Sulawesi No. 12 RT 02 RW 01" />
          </div>
          <div>
            <label class="field-label">Desa / Kelurahan</label>
            <InputText v-model="form.desa" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Kecamatan</label>
            <InputText v-model="form.kecamatan" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Kabupaten / Kotamadya</label>
            <InputText v-model="form.kabupaten" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Provinsi</label>
            <InputText v-model="form.provinsi" style="width: 100%" />
          </div>
        </div>
      </div>

      <div class="card">
        <div class="section-title-row">
          <div class="section-title">Data Keluarga -- Isteri/Suami</div>
          <div class="toggle-inline">
            <ToggleSwitch v-model="punyaPasangan" />
            <span>Punya Isteri/Suami yang Ditanggung</span>
          </div>
        </div>
        <div v-if="punyaPasangan" class="form-grid">
          <div>
            <label class="field-label">Nama Isteri/Suami</label>
            <InputText v-model="pasangan.nama" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Tempat Lahir</label>
            <InputText v-model="pasangan.tempat_lahir" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Tanggal Lahir</label>
            <DatePicker v-model="pasangan.tgl_lahir" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
          </div>
          <div>
            <label class="field-label">N.I.K</label>
            <InputText v-model="pasangan.nik" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Pekerjaan</label>
            <InputText v-model="pasangan.pekerjaan" style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Tanggal Perkawinan</label>
            <DatePicker v-model="pasangan.tgl_perkawinan" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
          </div>
          <div>
            <label class="field-label">Isteri/Suami Ke-</label>
            <InputNumber v-model="pasangan.pasangan_ke" :min="1" :max="20" showButtons style="width: 100%" fluid />
          </div>
          <div>
            <label class="field-label">Penghasilan Perbulan</label>
            <InputNumber v-model="pasangan.penghasilan" mode="currency" currency="IDR" locale="id-ID" :minFractionDigits="0" style="width: 100%" fluid />
          </div>
        </div>
        <Message v-else severity="secondary" :closable="false">Belum/tidak memiliki isteri/suami yang menjadi tanggungan.</Message>
      </div>

      <div class="card">
        <div class="section-title-row">
          <div class="section-title">Data Keluarga -- Anak-anak yang Menjadi Tanggungan</div>
          <Button label="Tambah Anak" icon="pi pi-plus" size="small" @click="tambahAnak" />
        </div>

        <Message v-if="!anakList.length" severity="secondary" :closable="false">Belum ada data anak yang ditambahkan.</Message>

        <div v-for="(anak, idx) in anakList" :key="idx" class="anak-card">
          <div class="anak-card-header">
            <strong>Anak ke-{{ idx + 1 }}</strong>
            <Button icon="pi pi-trash" severity="danger" text size="small" @click="hapusAnak(idx)" />
          </div>
          <div class="form-grid">
            <div>
              <label class="field-label">Nama Anak</label>
              <InputText v-model="anak.nama" style="width: 100%" />
            </div>
            <div>
              <label class="field-label">Tempat Lahir</label>
              <InputText v-model="anak.tempat_lahir" style="width: 100%" />
            </div>
            <div>
              <label class="field-label">Tanggal Lahir</label>
              <DatePicker v-model="anak.tgl_lahir" dateFormat="dd-mm-yy" showIcon style="width: 100%" />
            </div>
            <div>
              <label class="field-label">Status Anak</label>
              <Select v-model="anak.status_anak" :options="statusAnakOptions" optionLabel="label" optionValue="id" style="width: 100%" />
            </div>
            <div>
              <label class="field-label">Dari Isteri/Suami Ke-</label>
              <InputNumber v-model="anak.dari_pasangan_ke" :min="1" :max="20" showButtons style="width: 100%" fluid />
            </div>
            <div>
              <label class="field-label">Jenis Kelamin</label>
              <Select v-model="anak.jenis_kelamin" :options="jenisKelaminOptions" optionLabel="label" optionValue="id" style="width: 100%" />
            </div>
            <div v-if="anak.status_anak === 'angkat'" style="grid-column: 1 / -1">
              <label class="field-label">No. Putusan Pengadilan (Khusus Anak Angkat)</label>
              <InputText v-model="anak.no_putusan_pengadilan" style="width: 100%" />
            </div>
          </div>
          <div class="anak-checkbox-row">
            <label class="checkbox-inline"><Checkbox v-model="anak.dapat_tunjangan" binary /> Dapat Tunjangan</label>
            <label class="checkbox-inline"><Checkbox v-model="anak.sudah_kawin" binary /> Sudah Kawin</label>
            <label class="checkbox-inline"><Checkbox v-model="anak.sudah_bekerja" binary /> Sudah Bekerja</label>
            <label class="checkbox-inline"><Checkbox v-model="anak.masih_sekolah" binary /> Masih Sekolah/Kuliah</label>
          </div>
        </div>
      </div>

      <div style="display: flex; gap: 0.75rem; flex-wrap: wrap">
        <Button label="Simpan Data KP4" icon="pi pi-save" :loading="saving" @click="simpan" />
        <Button label="Cetak PDF KP4" icon="pi pi-file-pdf" severity="secondary" :loading="printing" @click="cetak" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.card {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 1.25rem;
}

.section-title {
  font-weight: 700;
  font-size: 0.95rem;
  margin-bottom: 0.9rem;
}

.section-subtitle {
  font-weight: 600;
  font-size: 0.85rem;
  margin-bottom: 0.75rem;
  color: var(--p-text-muted-color);
}

.section-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-bottom: 0.9rem;
}

.toggle-inline {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  font-size: 0.9rem;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.9rem 1.25rem;
}

@media (max-width: 480px) {
  .detail-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }
}

.detail-label {
  display: block;
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--p-text-muted-color);
  text-transform: uppercase;
  letter-spacing: 0.02em;
  margin-bottom: 0.15rem;
}

.stat-row {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
  margin-top: 1rem;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem 1.25rem;
}

.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
}

.anak-card {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 1rem;
  background: #f9fafb;
}

.anak-card:last-child {
  margin-bottom: 0;
}

.anak-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.anak-checkbox-row {
  display: flex;
  gap: 1.25rem;
  flex-wrap: wrap;
  margin-top: 0.9rem;
}

.checkbox-inline {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
}
</style>
