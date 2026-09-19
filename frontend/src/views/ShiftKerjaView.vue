<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import http from '../api/http'

import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import ToggleSwitch from 'primevue/toggleswitch'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'

const toast = useToast()
const confirm = useConfirm()

const KATEGORI_PILIHAN = ['5 Hari Kerja', '6 Hari Kerja', 'Shift Pagi', 'Shift Siang', 'Shift Malam']

// Urutan hari untuk TAMPILAN (mulai Senin, lebih wajar dibaca administrator
// Indonesia) -- field `hari` yang dikirim/diterima backend tetap memakai
// urutan time.Weekday Go (0=Minggu, 1=Senin, ... 6=Sabtu), lihat
// models.ShiftKerjaHari di backend.
const URUTAN_HARI_TAMPILAN = [1, 2, 3, 4, 5, 6, 0]
const NAMA_HARI = { 0: 'Minggu', 1: 'Senin', 2: 'Selasa', 3: 'Rabu', 4: 'Kamis', 5: 'Jumat', 6: 'Sabtu' }

function hariKosong() {
  return URUTAN_HARI_TAMPILAN.map((hari) => ({
    hari,
    aktif: hari !== 0, // default: Minggu libur, hari lain kerja
    jam_mulai_pagi: '',
    jam_batas_pagi: '',
    jam_tutup_pagi: '',
    jam_istirahat_mulai: '',
    jam_istirahat_selesai: '',
    jam_mulai_pulang: '',
    jam_tutup_pulang: '',
  }))
}

const shiftList = ref([])
const loading = ref(true)
const unitKerjaList = ref([])

async function loadShiftList() {
  loading.value = true
  try {
    const { data } = await http.get('/shift-kerja')
    shiftList.value = data.data || []
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Gagal memuat', detail: e.response?.data?.message || e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

async function loadUnitKerja() {
  try {
    const { data } = await http.get('/ref/unit-kerja')
    unitKerjaList.value = data.data || []
  } catch (e) {
    // non-kritikal -- dropdown unit kerja akan tampil kosong
  }
}

onMounted(() => {
  loadShiftList()
  loadUnitKerja()
})

// unit kerja yang boleh dipilih di dropdown: yang belum dipasangi shift lain,
// DITAMBAH unit kerja milik shift yang sedang diedit (supaya tetap muncul di
// dropdown walau sudah "terpakai" oleh dirinya sendiri). Satu shift sekarang
// boleh mencakup BANYAK unit kerja sekaligus (checkbox multi-pilih), tapi
// satu unit kerja tetap hanya boleh dipasangi satu shift.
const unitKerjaOptions = computed(() => {
  const idTerpakai = new Set(
    shiftList.value
      .filter((s) => s.id !== form.id)
      .flatMap((s) => (s.unit_kerja_list || []).map((u) => u.id))
  )
  return unitKerjaList.value.filter((u) => !idTerpakai.has(u.id))
})

// Dipanggil saat toggle "aktif" suatu hari dimatikan (dijadikan libur) --
// mengosongkan field jam supaya tidak ada sisa jam lama yang tersimpan ke
// database untuk hari yang sudah ditandai libur (murni kebersihan data;
// backend & absensi.go tetap fail-closed berdasarkan flag Aktif, bukan isi
// jam, jadi ini tidak mempengaruhi perilaku kamera absen).
function kosongkanJamHari(h) {
  h.jam_mulai_pagi = ''
  h.jam_batas_pagi = ''
  h.jam_tutup_pagi = ''
  h.jam_istirahat_mulai = ''
  h.jam_istirahat_selesai = ''
  h.jam_mulai_pulang = ''
  h.jam_tutup_pulang = ''
}

function jumlahHariKerja(shift) {
  return (shift.hari_list || []).filter((h) => h.aktif).length
}

// ============================================================
// tambah / ubah
// ============================================================
const dialogVisible = ref(false)
const dialogMode = ref('tambah') // 'tambah' | 'ubah'
const submitting = ref(false)
const formErrors = ref('')
const form = reactive({
  id: null,
  nama_shift: '',
  kategori_shift: null,
  id_unit_kerja_list: [],
  hari_list: hariKosong(),
})

function bukaTambah() {
  dialogMode.value = 'tambah'
  formErrors.value = ''
  Object.assign(form, { id: null, nama_shift: '', kategori_shift: null, id_unit_kerja_list: [], hari_list: hariKosong() })
  dialogVisible.value = true
}

function bukaUbah(item) {
  dialogMode.value = 'ubah'
  formErrors.value = ''
  const hariTersimpan = {}
  for (const h of item.hari_list || []) hariTersimpan[h.hari] = h
  Object.assign(form, {
    id: item.id,
    nama_shift: item.nama_shift,
    kategori_shift: item.kategori_shift,
    id_unit_kerja_list: (item.unit_kerja_list || []).map((u) => u.id),
    hari_list: URUTAN_HARI_TAMPILAN.map((hari) => {
      const h = hariTersimpan[hari]
      return h
        ? { ...h }
        : { hari, aktif: false, jam_mulai_pagi: '', jam_batas_pagi: '', jam_tutup_pagi: '', jam_istirahat_mulai: '', jam_istirahat_selesai: '', jam_mulai_pulang: '', jam_tutup_pulang: '' }
    }),
  })
  dialogVisible.value = true
}

function tutupDialog() {
  dialogVisible.value = false
}

// salin jam dari SATU baris (biasanya Senin) ke semua baris lain yang
// berstatus "Hari Kerja" -- mempercepat pengisian karena kebanyakan shift
// jamnya sama tiap hari kecuali satu-dua hari (mis. Jumat).
function salinKeSemuaHariKerja(sumberIndex) {
  const sumber = form.hari_list[sumberIndex]
  for (let i = 0; i < form.hari_list.length; i++) {
    if (i === sumberIndex) continue
    if (!form.hari_list[i].aktif) continue
    form.hari_list[i].jam_mulai_pagi = sumber.jam_mulai_pagi
    form.hari_list[i].jam_batas_pagi = sumber.jam_batas_pagi
    form.hari_list[i].jam_tutup_pagi = sumber.jam_tutup_pagi
    form.hari_list[i].jam_istirahat_mulai = sumber.jam_istirahat_mulai
    form.hari_list[i].jam_istirahat_selesai = sumber.jam_istirahat_selesai
    form.hari_list[i].jam_mulai_pulang = sumber.jam_mulai_pulang
    form.hari_list[i].jam_tutup_pulang = sumber.jam_tutup_pulang
  }
  toast.add({ severity: 'success', summary: 'Disalin', detail: `Jam hari ${NAMA_HARI[sumber.hari]} disalin ke semua hari kerja lainnya`, life: 3000 })
}

const jamPattern = /^([01]\d|2[0-3]):[0-5]\d$/
function jamValid(v) {
  return jamPattern.test(v || '')
}

function validasiForm() {
  if (!form.nama_shift?.trim()) return 'Nama shift wajib diisi'
  if (!form.kategori_shift) return 'Kategori shift wajib dipilih'
  if (!form.id_unit_kerja_list?.length) return 'Minimal satu unit kerja wajib dipilih'
  for (const h of form.hari_list) {
    if (!h.aktif) continue
    const label = NAMA_HARI[h.hari]
    if (!jamValid(h.jam_mulai_pagi) || !jamValid(h.jam_batas_pagi) || !jamValid(h.jam_tutup_pagi) || !jamValid(h.jam_mulai_pulang) || !jamValid(h.jam_tutup_pulang)) {
      return `Lengkapi jam masuk/pulang hari ${label} dengan format HH:MM`
    }
    const toMin = (s) => { const [hh, mm] = s.split(':').map(Number); return hh * 60 + mm }
    if (toMin(h.jam_batas_pagi) <= toMin(h.jam_mulai_pagi)) return `Hari ${label}: jam batas absen pagi harus lebih besar dari jam mulai absen pagi`
    if (toMin(h.jam_tutup_pagi) <= toMin(h.jam_batas_pagi)) return `Hari ${label}: jam tutup absen pagi harus lebih besar dari jam batas absen pagi`
    if (toMin(h.jam_tutup_pulang) <= toMin(h.jam_mulai_pulang)) return `Hari ${label}: jam tutup absen pulang harus lebih besar dari jam mulai absen pulang`
    const imFilled = !!h.jam_istirahat_mulai
    const isFilled = !!h.jam_istirahat_selesai
    if (imFilled !== isFilled) return `Hari ${label}: jam istirahat mulai & selesai harus diisi berdua atau dikosongkan berdua`
    if (imFilled) {
      if (!jamValid(h.jam_istirahat_mulai) || !jamValid(h.jam_istirahat_selesai)) return `Hari ${label}: jam istirahat tidak valid`
      if (toMin(h.jam_istirahat_selesai) <= toMin(h.jam_istirahat_mulai)) return `Hari ${label}: jam istirahat selesai harus lebih besar dari jam istirahat mulai`
    }
  }
  return ''
}

async function submitForm() {
  const err = validasiForm()
  if (err) {
    formErrors.value = err
    return
  }
  formErrors.value = ''
  submitting.value = true
  const payload = {
    nama_shift: form.nama_shift.trim(),
    kategori_shift: form.kategori_shift,
    id_unit_kerja_list: form.id_unit_kerja_list,
    hari_list: form.hari_list.map((h) => ({ ...h })),
  }
  try {
    if (dialogMode.value === 'tambah') {
      const { data } = await http.post('/shift-kerja', payload)
      toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 4000 })
    } else {
      const { data } = await http.put(`/shift-kerja/${form.id}`, payload)
      toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 4000 })
    }
    dialogVisible.value = false
    await loadShiftList()
  } catch (e) {
    formErrors.value = e.response?.data?.message || e.message
  } finally {
    submitting.value = false
  }
}

function namaUnitKerjaList(item) {
  return (item.unit_kerja_list || []).map((u) => u.unit).join(', ') || '-'
}

function konfirmasiHapus(item) {
  confirm.require({
    message: `Hapus shift kerja "${item.nama_shift}"? Pegawai di unit kerja "${namaUnitKerjaList(item)}" akan otomatis kembali memakai jam kerja lama (jam unit kerja atau default sekolah/dinas).`,
    header: 'Konfirmasi Hapus',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Ya, Hapus',
    rejectLabel: 'Batal',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        const { data } = await http.delete(`/shift-kerja/${item.id}`)
        toast.add({ severity: 'success', summary: 'Berhasil', detail: data.message, life: 5000 })
        await loadShiftList()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Gagal menghapus', detail: e.response?.data?.message || e.message, life: 5000 })
      }
    },
  })
}
</script>

<template>
  <div class="page-wrap">
    <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 0.75rem">
      <div>
        <h2 style="margin: 0 0 0.35rem 0">Shift Kerja</h2>
        <p class="text-muted" style="max-width: 60rem">
          Atur jam masuk, istirahat & pulang per hari untuk satu atau beberapa Unit Kerja sekaligus. Begitu shift
          dipasang ke suatu unit kerja, SEMUA pegawai yang tempat kerjanya unit tersebut otomatis mengikuti jam &
          jendela kamera absen shift ini -- tidak perlu diatur satu per satu per pegawai. Satu unit kerja tetap hanya
          boleh dipasangi satu shift. Di luar jam yang ditetapkan (atau di hari yang ditandai libur untuk shift ini),
          kamera absen otomatis tertutup.
        </p>
      </div>
      <Button label="Tambah Shift Kerja" icon="pi pi-plus" @click="bukaTambah" />
    </div>

    <div v-if="loading" style="text-align: center; padding: 3rem 0"><ProgressSpinner style="width: 40px; height: 40px" /></div>
    <DataTable v-else :value="shiftList" size="small" stripedRows responsiveLayout="scroll">
      <Column field="nama_shift" header="Nama Shift" />
      <Column field="kategori_shift" header="Kategori" />
      <Column header="Unit Kerja">
        <template #body="{ data }">{{ namaUnitKerjaList(data) }}</template>
      </Column>
      <Column header="Hari Kerja">
        <template #body="{ data }">
          <Tag :value="`${jumlahHariKerja(data)} / 7 hari`" :severity="jumlahHariKerja(data) >= 6 ? 'info' : 'success'" />
        </template>
      </Column>
      <Column header="Aksi" style="width: 8rem">
        <template #body="{ data }">
          <Button icon="pi pi-pencil" size="small" text rounded title="Ubah" @click="bukaUbah(data)" />
          <Button icon="pi pi-trash" size="small" text rounded severity="danger" title="Hapus" @click="konfirmasiHapus(data)" />
        </template>
      </Column>
      <template #empty>Belum ada shift kerja. Klik "Tambah Shift Kerja" untuk membuat yang pertama.</template>
    </DataTable>

    <Dialog
      v-model:visible="dialogVisible"
      modal
      :header="dialogMode === 'tambah' ? 'Tambah Shift Kerja' : 'Ubah Shift Kerja'"
      :style="{ width: '95vw', maxWidth: '75rem' }"
      :breakpoints="{ '640px': '98vw' }"
      @hide="tutupDialog"
    >
      <Message v-if="formErrors" severity="error" :closable="false" style="margin-bottom: 1rem">{{ formErrors }}</Message>

      <div class="jam-grid" style="margin-bottom: 1rem">
        <div>
          <div class="field-label">Nama Shift</div>
          <InputText v-model="form.nama_shift" placeholder="Contoh: Shift Pagi, Reguler 5 Hari Kerja" style="width: 100%" />
        </div>
        <div>
          <div class="field-label">Kategori Shift</div>
          <Select v-model="form.kategori_shift" :options="KATEGORI_PILIHAN" placeholder="Pilih kategori" style="width: 100%" />
        </div>
        <div>
          <div class="field-label">Unit Kerja</div>
          <MultiSelect
            v-model="form.id_unit_kerja_list"
            :options="unitKerjaOptions"
            optionLabel="unit"
            optionValue="id"
            filter
            display="chip"
            placeholder="Pilih unit kerja (bisa lebih dari satu)"
            style="width: 100%"
          />
          <small class="text-muted">Bisa pilih lebih dari satu unit kerja. Hanya unit kerja yang belum dipasangi shift lain yang muncul di sini.</small>
        </div>
      </div>

      <div class="field-label">Ketentuan Jam Per Hari</div>
      <p class="text-muted" style="margin-top: 0">
        Nonaktifkan toggle untuk menjadikan hari itu libur (kamera absen tertutup penuh hari itu untuk shift ini).
        Jam istirahat opsional, murni informasi jadwal (tidak menutup kamera).
      </p>
      <div style="overflow-x: auto">
        <table class="shift-hari-table">
          <thead>
            <tr>
              <th style="min-width: 6rem">Hari</th>
              <th style="min-width: 5.5rem">Mulai Pagi</th>
              <th style="min-width: 5.5rem">Batas Pagi</th>
              <th style="min-width: 5.5rem">Tutup Pagi</th>
              <th style="min-width: 5.5rem">Istirahat Mulai</th>
              <th style="min-width: 5.5rem">Istirahat Selesai</th>
              <th style="min-width: 5.5rem">Mulai Pulang</th>
              <th style="min-width: 5.5rem">Tutup Pulang</th>
              <th style="min-width: 3rem"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(h, idx) in form.hari_list" :key="h.hari">
              <td>
                <div style="display: flex; align-items: center; gap: 0.5rem">
                  <ToggleSwitch v-model="h.aktif" @update:model-value="(v) => !v && kosongkanJamHari(h)" />
                  <span :style="{ opacity: h.aktif ? 1 : 0.5 }">{{ NAMA_HARI[h.hari] }}</span>
                </div>
              </td>
              <template v-if="h.aktif">
                <td><InputText v-model="h.jam_mulai_pagi" placeholder="06:00" style="width: 5.5rem" /></td>
                <td><InputText v-model="h.jam_batas_pagi" placeholder="07:30" style="width: 5.5rem" /></td>
                <td><InputText v-model="h.jam_tutup_pagi" placeholder="09:00" style="width: 5.5rem" /></td>
                <td><InputText v-model="h.jam_istirahat_mulai" placeholder="opsional" style="width: 5.5rem" /></td>
                <td><InputText v-model="h.jam_istirahat_selesai" placeholder="opsional" style="width: 5.5rem" /></td>
                <td><InputText v-model="h.jam_mulai_pulang" placeholder="15:00" style="width: 5.5rem" /></td>
                <td><InputText v-model="h.jam_tutup_pulang" placeholder="20:00" style="width: 5.5rem" /></td>
                <td>
                  <Button icon="pi pi-copy" size="small" text rounded title="Salin jam hari ini ke semua hari kerja lain" @click="salinKeSemuaHariKerja(idx)" />
                </td>
              </template>
              <template v-else>
                <td colspan="8" style="color: #9ca3af; font-style: italic">Libur -- kamera absen tertutup penuh hari ini</td>
              </template>
            </tr>
          </tbody>
        </table>
      </div>

      <template #footer>
        <Button label="Batal" severity="secondary" outlined @click="tutupDialog" />
        <Button label="Simpan" icon="pi pi-check" :loading="submitting" @click="submitForm" />
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
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 1rem;
}
.shift-hari-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}
.shift-hari-table th {
  text-align: left;
  padding: 0.5rem;
  border-bottom: 2px solid #e5e7eb;
  color: #6b7280;
  font-weight: 600;
}
.shift-hari-table td {
  padding: 0.4rem 0.5rem;
  border-bottom: 1px solid #f1f5f9;
  vertical-align: middle;
}
</style>
