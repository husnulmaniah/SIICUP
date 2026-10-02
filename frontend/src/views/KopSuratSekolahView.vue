<script setup>
import { ref, computed, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'

import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

// KopSuratSekolahView -- menu "Kop Surat Sekolah", KHUSUS akun atasan
// (Kepala Sekolah/Kepala Puskesmas) yang bertugas di unit kerja Sekolah/
// Puskesmas (lihat GET/PUT /api/kop-surat-sekolah di
// backend/handlers/kop_surat.go). Perubahan di sini LANGSUNG berlaku
// begitu disimpan (TANPA menunggu persetujuan admin) & dipakai pada
// Lampiran 3 (Surat Rekomendasi perpanjangan kontrak bawahan sekolah ini --
// lihat buildSuratRekomendasiSekolah di backend/handlers/surat_rekomendasi.go).

const toast = useToast()
const loading = ref(true)
const saving = ref(false)

const unitNama = ref('')
const namaSekolahKop = ref('')
const alamatKopKiri = ref('')
const alamatKopKanan = ref('')
const perataanKop = ref('tengah')
const tampilkanLogoTutwuri = ref(false)
const logoTutwuriTersedia = ref(false)

const perataanOptions = [
  { label: 'Rata Kiri', value: 'kiri' },
  { label: 'Rata Tengah', value: 'tengah' },
  { label: 'Rata Kanan', value: 'kanan' },
]

async function load() {
  loading.value = true
  try {
    const { data } = await http.get('/kop-surat-sekolah')
    const d = data.data || {}
    unitNama.value = d.unit || ''
    namaSekolahKop.value = d.nama_sekolah_kop || ''
    alamatKopKiri.value = d.alamat_kop_kiri || ''
    alamatKopKanan.value = d.alamat_kop_kanan || ''
    perataanKop.value = d.perataan_kop || 'tengah'
    tampilkanLogoTutwuri.value = !!d.tampilkan_logo_tutwuri
    logoTutwuriTersedia.value = !!d.logo_tutwuri_tersedia
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const { data } = await http.put('/kop-surat-sekolah', {
      nama_sekolah_kop: namaSekolahKop.value,
      alamat_kop_kiri: alamatKopKiri.value,
      alamat_kop_kanan: alamatKopKanan.value,
      perataan_kop: perataanKop.value,
      tampilkan_logo_tutwuri: tampilkanLogoTutwuri.value,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message || 'Kop surat berhasil disimpan', life: 4000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    saving.value = false
  }
}

// ---- preview sederhana (meniru tata letak kop surat PDF Lampiran 3) ----
const previewTitleLines = computed(() =>
  (namaSekolahKop.value.trim() ? namaSekolahKop.value : `PEMERINTAH KABUPATEN MOROWALI UTARA\n${unitNama.value || '-'}\nKabupaten Morowali Utara`)
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
)
const previewAlign = computed(() => ({ kiri: 'left', kanan: 'right', tengah: 'center' })[perataanKop.value] || 'center')
const previewKiriLines = computed(() => alamatKopKiri.value.split('\n').map((s) => s.trim()).filter(Boolean))
const previewKananLines = computed(() => alamatKopKanan.value.split('\n').map((s) => s.trim()).filter(Boolean))
const previewDuaKolom = computed(() => previewKiriLines.value.length > 0 && previewKananLines.value.length > 0)

onMounted(load)
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Kop Surat Sekolah</div>
    <p class="page-subtitle">
      Atur kop surat "{{ unitNama || 'sekolah/puskesmas anda' }}" yang dipakai pada surat rekomendasi perpanjangan
      kontrak bawahan anda (Lampiran 3). Perubahan di sini <strong>langsung berlaku</strong> begitu disimpan, tanpa
      perlu menunggu persetujuan admin.
    </p>

    <div v-if="loading" style="display: flex; justify-content: center; padding: 3rem">
      <ProgressSpinner style="width: 42px; height: 42px" />
    </div>

    <div v-else class="kop-layout">
      <div class="card kop-form">
        <Message severity="info" :closable="false">
          Kosongkan "Nama Sekolah di Kop" untuk memakai kop bawaan sistem (nama pemerintah + nama unit kerja apa
          adanya). Isi field ini untuk menampilkan kop sesuai kop surat fisik sekolah anda (boleh beberapa baris,
          mis. nama sekolah lalu baris akreditasi).
        </Message>

        <div class="field">
          <label class="field-label">Nama Sekolah di Kop (boleh beberapa baris)</label>
          <Textarea v-model="namaSekolahKop" rows="3" style="width: 100%" placeholder='mis. SD NEGERI LANUMOR&#10;TERAKREDITASI "B"' />
        </div>

        <div class="field">
          <label class="field-label">Alamat / Info Kop Kiri</label>
          <Textarea
            v-model="alamatKopKiri"
            rows="2"
            style="width: 100%"
            placeholder="mis. NPSN/NSS: 40202766/101180706016&#10;Alamat : Desa Lanumor, Kec Mori Atas, KodePos 94965"
          />
        </div>

        <div class="field">
          <label class="field-label">Alamat / Info Kop Kanan (opsional)</label>
          <Textarea v-model="alamatKopKanan" rows="2" style="width: 100%" placeholder="Kosongkan kalau tidak ada" />
          <small class="field-hint">
            Kalau KEDUA kolom (kiri & kanan) diisi, keduanya ditampilkan berdampingan sebagai dua kolom (kiri rata
            kiri, kanan rata kanan) -- perataan di bawah tidak berlaku untuk mode dua kolom ini.
          </small>
        </div>

        <div class="field">
          <label class="field-label">Perataan Teks Kop Surat</label>
          <SelectButton v-model="perataanKop" :options="perataanOptions" optionLabel="label" optionValue="value" />
        </div>

        <div class="field">
          <div style="display: flex; align-items: center; gap: 0.5rem">
            <Checkbox v-model="tampilkanLogoTutwuri" binary inputId="logoTutwuri" />
            <label for="logoTutwuri">Tampilkan logo Tut Wuri Handayani di sisi kanan kop surat</label>
          </div>
          <small v-if="!logoTutwuriTersedia" class="field-hint">
            Berkas logo Tut Wuri Handayani belum ditambahkan ke sistem -- centang ini tetap bisa disimpan, logonya
            akan otomatis tampil begitu berkasnya ditambahkan oleh pengelola sistem.
          </small>
        </div>

        <div>
          <Button label="Simpan" icon="pi pi-save" :loading="saving" @click="save" />
        </div>
      </div>

      <div class="card kop-preview">
        <div class="kop-preview-label">Pratinjau Kop Surat</div>
        <div class="kop-preview-box">
          <div class="kop-preview-logo kiri">
            <i class="pi pi-shield"></i>
          </div>
          <div v-if="tampilkanLogoTutwuri" class="kop-preview-logo kanan">
            <i class="pi pi-shield"></i>
          </div>
          <div class="kop-preview-text" :style="{ textAlign: previewAlign }">
            <div v-for="(line, i) in previewTitleLines" :key="'t' + i" class="kop-preview-title">{{ line }}</div>
            <div v-if="!previewDuaKolom && previewKiriLines.length" class="kop-preview-addr">
              <div v-for="(line, i) in previewKiriLines" :key="'k' + i">{{ line }}</div>
            </div>
          </div>
          <div v-if="previewDuaKolom" class="kop-preview-dua-kolom">
            <div class="kop-preview-addr kiri-col">
              <div v-for="(line, i) in previewKiriLines" :key="'dk' + i">{{ line }}</div>
            </div>
            <div class="kop-preview-addr kanan-col">
              <div v-for="(line, i) in previewKananLines" :key="'dkn' + i">{{ line }}</div>
            </div>
          </div>
          <div class="kop-preview-rule"></div>
        </div>
        <small class="field-hint">
          Pratinjau ini hanya ilustrasi kasar tata letak -- logo di atas berupa ikon pengganti, pada PDF sebenarnya
          berupa logo Kabupaten (kiri, selalu tampil) dan logo Tut Wuri Handayani (kanan, kalau dicentang).
        </small>
      </div>
    </div>
  </div>
</template>

<style scoped>
.kop-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr);
  gap: 1.25rem;
  align-items: start;
}

@media (max-width: 900px) {
  .kop-layout {
    grid-template-columns: 1fr;
  }
}

.kop-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.field-label {
  font-size: 0.85rem;
  font-weight: 600;
}

.field-hint {
  color: var(--p-text-muted-color, #64748b);
  font-size: 0.78rem;
}

.kop-preview {
  position: sticky;
  top: 1rem;
}

.kop-preview-label {
  font-weight: 700;
  margin-bottom: 0.75rem;
}

.kop-preview-box {
  position: relative;
  min-height: 150px;
  padding: 0.5rem 0.25rem 0.75rem;
}

.kop-preview-logo {
  position: absolute;
  top: 0;
  width: 40px;
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.4rem;
  color: #64748b;
  border: 1px dashed var(--p-content-border-color, #cbd5e1);
  border-radius: 6px;
}

.kop-preview-logo.kiri {
  left: 0;
}

.kop-preview-logo.kanan {
  right: 0;
}

.kop-preview-text {
  padding: 0 48px;
}

.kop-preview-title {
  font-weight: 700;
  font-size: 0.82rem;
  line-height: 1.35;
}

.kop-preview-addr {
  font-size: 0.72rem;
  color: var(--p-text-muted-color, #475569);
  margin-top: 0.3rem;
  line-height: 1.4;
}

.kop-preview-dua-kolom {
  display: flex;
  justify-content: space-between;
  padding: 0 48px;
  margin-top: 0.3rem;
  gap: 0.5rem;
}

.kop-preview-dua-kolom .kiri-col {
  text-align: left;
}

.kop-preview-dua-kolom .kanan-col {
  text-align: right;
}

.kop-preview-rule {
  margin-top: 0.6rem;
  border-top: 2px solid var(--p-text-color, #1e293b);
}
</style>
