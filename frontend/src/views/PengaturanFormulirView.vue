<script setup>
import { ref, computed, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

const toast = useToast()
const loading = ref(true)
const saving = ref(false)
const namaKepalaDinas = ref('')
const nipKepalaDinas = ref('')

// ---- pilih otomatis dari Data Pegawai (jabatan Kepala Dinas / Sekretaris
// Dinas Pendidikan dan Kebudayaan Daerah) ----
const calonPenandatangan = ref([])
const calonLoading = ref(true)
const selectedPenandatangan = ref(null)

async function loadCalonPenandatangan() {
  calonLoading.value = true
  try {
    const { data } = await http.get('/pengaturan-surat/calon-penandatangan')
    // PrimeVue Select memakai optionLabel juga sebagai field pencarian saat
    // filter=true -- kalau optionLabel berupa function (bukan nama field
    // string), pencarian bawaannya gagal resolve teksnya (hasil pencarian
    // selalu kosong walau datanya ada). Jadi label gabungan "Nama (Jabatan)"
    // dihitung sekali di sini jadi field string biasa ("label"), supaya
    // optionLabel="label" bisa dipakai baik untuk tampilan maupun pencarian.
    calonPenandatangan.value = (data.data || []).map((p) => ({
      ...p,
      label: `${p.nama} (${p.jabatan?.jabatan || '-'})`,
    }))
  } catch (e) {
    calonPenandatangan.value = []
  } finally {
    calonLoading.value = false
  }
}

function onPilihPenandatangan(pegawaiId) {
  const p = calonPenandatangan.value.find((x) => x.id === pegawaiId)
  if (p) {
    namaKepalaDinas.value = p.nama || ''
    nipKepalaDinas.value = p.nip || ''
  }
}

async function load() {
  loading.value = true
  try {
    const { data } = await http.get('/pengaturan-surat')
    namaKepalaDinas.value = data.data?.nama_kepala_dinas || ''
    nipKepalaDinas.value = data.data?.nip_kepala_dinas || ''
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await http.put('/pengaturan-surat', {
      nama_kepala_dinas: namaKepalaDinas.value,
      nip_kepala_dinas: nipKepalaDinas.value,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengaturan formulir berhasil disimpan', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal menyimpan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  load()
  loadCalonPenandatangan()
})
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">Pengaturan Formulir</div>
    <p class="page-subtitle">
      Data Kepala Dinas ini dicetak sebagai penandatangan pada "Surat Rekomendasi Izin Cuti" dan "Formulir Permintaan
      dan Pemberian Cuti" yang bisa diunduh setelah sebuah pengajuan cuti disetujui.
    </p>

    <div class="card" style="max-width: 32rem">
      <div v-if="loading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <div v-else style="display: flex; flex-direction: column; gap: 1rem">
        <Message severity="info" :closable="false">
          Perubahan di sini berlaku untuk semua formulir yang akan dicetak setelah ini disimpan (formulir yang sudah
          pernah diunduh sebelumnya tidak berubah).
        </Message>

        <div>
          <label class="field-label">Pilih Otomatis dari Data Pegawai</label>
          <Select
            v-model="selectedPenandatangan"
            :options="calonPenandatangan"
            optionLabel="label"
            optionValue="id"
            :loading="calonLoading"
            filter
            showClear
            style="width: 100%"
            placeholder="Pilih pejabat (Kepala Dinas / Sekretaris)..."
            @update:modelValue="onPilihPenandatangan"
          />
          <small v-if="!calonLoading && !calonPenandatangan.length" style="color: var(--p-text-muted-color); display: block; margin-top: 0.35rem">
            Belum ada pegawai dengan jabatan "Kepala Dinas" atau "Sekretaris Dinas Pendidikan dan Kebudayaan Daerah". Atur
            jabatan pegawai yang bersangkutan lewat menu Data Pegawai, atau isi manual di bawah.
          </small>
          <small v-else style="color: var(--p-text-muted-color); display: block; margin-top: 0.35rem">
            Memilih di sini otomatis mengisi Nama & NIP di bawah. Field di bawah tetap bisa diedit manual bila perlu.
          </small>
        </div>

        <div>
          <label class="field-label">Nama Kepala Dinas</label>
          <InputText v-model="namaKepalaDinas" style="width: 100%" placeholder="mis. MOH. RIDWAN DM. S.Ag" />
        </div>
        <div>
          <label class="field-label">NIP Kepala Dinas</label>
          <InputText v-model="nipKepalaDinas" style="width: 100%" placeholder="mis. 19740111 199803 1 004" />
        </div>
        <div>
          <Button label="Simpan" icon="pi pi-save" :loading="saving" @click="save" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
}
</style>
