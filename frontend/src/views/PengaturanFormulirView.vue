<script setup>
import { ref, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import http from '../api/http'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

const toast = useToast()
const loading = ref(true)
const saving = ref(false)
const namaKepalaDinas = ref('')
const nipKepalaDinas = ref('')

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

onMounted(load)
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
