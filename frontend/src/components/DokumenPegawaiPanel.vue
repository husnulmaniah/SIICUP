<script setup>
import { ref, computed, onMounted, watch } from 'vue';
import Swal from 'sweetalert2';
import api, { apiUrl, pesanError, toast, swalTema } from '../api';
import { JENIS_DOKUMEN } from '../dokumen-config';
import { formatWaktu } from '../cuti-config';

const props = defineProps({
  pegawaiId: { type: [Number, String], required: true },
  bolehKelola: { type: Boolean, default: true },
});

const dokumen = ref([]);
const sedangUnggah = ref('');

const perJenis = computed(() => {
  const map = {};
  for (const d of dokumen.value) map[d.jenis] = d;
  return map;
});

async function muat() {
  const { data } = await api.get(`/pegawai/${props.pegawaiId}/dokumen`);
  dokumen.value = data;
}
onMounted(muat);
watch(() => props.pegawaiId, muat);

const urlDok = (d) =>
  apiUrl(`/pegawai/${props.pegawaiId}/dokumen/${d.id}?token=${localStorage.getItem('sicuti_token')}`);

async function unggah(jenis, e) {
  const file = e.target.files?.[0];
  if (!file) return;
  sedangUnggah.value = jenis;
  const fd = new FormData();
  fd.append('jenis', jenis);
  fd.append('file', file);
  try {
    await api.post(`/pegawai/${props.pegawaiId}/dokumen`, fd);
    toast.fire({ icon: 'success', title: 'Dokumen tersimpan' });
    await muat();
  } catch (err) {
    Swal.fire({ icon: 'error', title: 'Gagal mengunggah', text: pesanError(err), ...swalTema });
  } finally {
    sedangUnggah.value = '';
    e.target.value = '';
  }
}

async function hapus(d) {
  const ok = await Swal.fire({
    icon: 'warning', title: `Hapus ${d.namaFile}?`, showCancelButton: true,
    confirmButtonText: 'Ya, hapus', cancelButtonText: 'Batal', ...swalTema,
  });
  if (!ok.isConfirmed) return;
  try {
    await api.delete(`/pegawai/${props.pegawaiId}/dokumen/${d.id}`);
    toast.fire({ icon: 'success', title: 'Dokumen dihapus' });
    await muat();
  } catch (err) {
    Swal.fire({ icon: 'error', title: 'Gagal menghapus', text: pesanError(err), ...swalTema });
  }
}
</script>

<template>
  <div class="card">
    <h3>Dokumen Kepegawaian</h3>
    <p class="hint" style="margin-bottom:12px">
      PDF/JPG/PNG maks. 10 MB per dokumen. <b>SK Terakhir</b> yang tersimpan di sini otomatis dilampirkan saat mengajukan cuti — tidak perlu mengunggah SK lagi.
    </p>
    <div v-for="j in JENIS_DOKUMEN" :key="j.kode" class="berkas-item" :class="{ terisi: perJenis[j.kode] }"
      style="display:flex; justify-content:space-between; align-items:center; gap:10px; flex-wrap:wrap">
      <div>
        <div class="judul">{{ j.label }} <span v-if="j.kode === 'SK_TERAKHIR'" title="Terhubung dengan pengajuan cuti">🔗</span></div>
        <div class="hint" v-if="perJenis[j.kode]">
          {{ perJenis[j.kode].namaFile }} · {{ (perJenis[j.kode].ukuran / 1024).toFixed(0) }} KB · diperbarui {{ formatWaktu(perJenis[j.kode].updatedAt) }}
        </div>
        <div class="hint" v-else>Belum diunggah</div>
      </div>
      <div style="display:flex; gap:8px; align-items:center; flex-wrap:wrap">
        <a v-if="perJenis[j.kode]" :href="urlDok(perJenis[j.kode])" target="_blank" class="btn btn-putih btn-kecil">Lihat</a>
        <label v-if="bolehKelola" class="btn btn-hijau btn-kecil" style="cursor:pointer">
          {{ sedangUnggah === j.kode ? 'Mengunggah…' : perJenis[j.kode] ? 'Ganti' : '⬆️ Unggah' }}
          <input type="file" accept=".pdf,.jpg,.jpeg,.png,.webp" hidden :disabled="!!sedangUnggah" @change="unggah(j.kode, $event)" />
        </label>
        <button v-if="bolehKelola && perJenis[j.kode]" class="btn btn-merah btn-kecil" @click="hapus(perJenis[j.kode])">Hapus</button>
      </div>
    </div>
  </div>
</template>
