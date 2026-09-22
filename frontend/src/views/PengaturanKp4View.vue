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
const namaInstansi = ref('')
const alamatInstansi = ref('')
const instansiInduk = ref('')
const bendaharawanGaji = ref('')

async function load() {
  loading.value = true
  try {
    const { data } = await http.get('/pengaturan-kp4')
    namaInstansi.value = data.data?.nama_instansi || ''
    alamatInstansi.value = data.data?.alamat_instansi || ''
    instansiInduk.value = data.data?.instansi_induk || ''
    bendaharawanGaji.value = data.data?.bendaharawan_gaji || ''
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat pengaturan', detail: e.response?.data?.message || e.message, life: 4000 })
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await http.put('/pengaturan-kp4', {
      nama_instansi: namaInstansi.value,
      alamat_instansi: alamatInstansi.value,
      instansi_induk: instansiInduk.value,
      bendaharawan_gaji: bendaharawanGaji.value,
    })
    toast.add({ severity: 'success', summary: 'Berhasil', detail: 'Pengaturan KP4 berhasil disimpan', life: 3000 })
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
    <div class="page-title">Pengaturan KP4</div>
    <p class="page-subtitle">
      Data identitas instansi ini dicetak sebagai kop pada formulir KP4 (Surat Keterangan Untuk Mendapatkan
      Pembayaran Tunjangan Keluarga) yang diunduh dari menu KP4.
    </p>

    <div class="card" style="max-width: 36rem">
      <div v-if="loading" style="display: flex; justify-content: center; padding: 2rem">
        <ProgressSpinner style="width: 2.5rem; height: 2.5rem" />
      </div>
      <div v-else style="display: flex; flex-direction: column; gap: 1rem">
        <Message severity="info" :closable="false">
          Perubahan di sini berlaku untuk semua formulir KP4 yang akan dicetak setelah ini disimpan (formulir yang
          sudah pernah diunduh sebelumnya tidak berubah).
        </Message>

        <div>
          <label class="field-label">Nama Instansi</label>
          <InputText v-model="namaInstansi" style="width: 100%" placeholder="mis. Dinas Pendidikan dan Kebudayaan Daerah" />
        </div>
        <div>
          <label class="field-label">Alamat Lengkap Instansi</label>
          <InputText v-model="alamatInstansi" style="width: 100%" placeholder="mis. Jln. Bumi Nangka Kompleks Perkantoran Kode Pos (94971) Kolonodale" />
        </div>
        <div>
          <label class="field-label">Instansi Induk</label>
          <InputText v-model="instansiInduk" style="width: 100%" placeholder="mis. Pemerintah Kabupaten Morowali Utara" />
        </div>
        <div>
          <label class="field-label">Bendaharawan Gaji</label>
          <InputText v-model="bendaharawanGaji" style="width: 100%" placeholder="Nama bendaharawan gaji" />
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
