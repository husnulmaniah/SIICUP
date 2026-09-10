<script setup>
import { ref, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import api from '../api';
import DokumenPegawaiPanel from '../components/DokumenPegawaiPanel.vue';

const route = useRoute();
const router = useRouter();
const pegawai = ref(null);

const baris = [
  ['nip', 'NIP BARU'], ['nik', 'NIK'], ['tempatLahir', 'Tempat Lahir'], ['tanggalLahir', 'Tanggal Lahir'],
  ['jenisKelamin', 'Jenis Kelamin'], ['statusCpnsPns', 'Status CPNS/PNS'], ['tanggalSkCpns', 'Tanggal SK CPNS'],
  ['tmtCpns', 'TMT CPNS'], ['tanggalSkPns', 'Tanggal SK PNS'], ['tmtPns', 'TMT PNS'],
  ['golAwal', 'Gol. Awal'], ['golAkhir', 'Gol. Akhir'], ['tmtGolongan', 'TMT Golongan'],
  ['jenisJabatan', 'Jenis Jabatan'], ['jabatan', 'Jabatan'], ['tmtJabatan', 'TMT Jabatan'],
  ['tingkatPendidikan', 'Tingkat Pendidikan'], ['pendidikan', 'Pendidikan'], ['kecamatan', 'Kecamatan'],
  ['unor', 'UNOR'], ['tempatTugas', 'Tempat Tugas'], ['status', 'Status'],
  ['tanggalKgbTerakhir', 'Tgl KGB Terakhir'], ['tanggalKpTerakhir', 'Tgl KP Terakhir'],
  ['tanggalPensiun', 'Tanggal Pensiun'], ['tanggalKgb', 'Tgl KGB Berikutnya'], ['tanggalKp', 'Tgl KP Berikutnya'],
  ['tahunPengangkatan', 'Tahun Pengangkatan'],
];

onMounted(async () => {
  const { data } = await api.get(`/pegawai/${route.params.id}`);
  pegawai.value = data;
});
</script>

<template>
  <div v-if="pegawai">
    <div class="page-head">
      <div>
        <h2>{{ pegawai.nama }}</h2>
        <p>
          {{ pegawai.jabatan || '-' }} — {{ pegawai.tempatTugas || pegawai.unor || '-' }}
          <span class="badge" :class="pegawai.aktif ? 'badge-DISETUJUI' : 'badge-DITOLAK'" style="margin-left:8px">
            {{ pegawai.aktif ? 'AKTIF' : 'NONAKTIF' }}
          </span>
        </p>
      </div>
      <button class="btn btn-putih" @click="router.back()">← Kembali</button>
    </div>

    <div style="display:grid; grid-template-columns: 1.4fr 1fr; gap:18px; align-items:start">
      <div class="card">
        <h3>Data Kepegawaian</h3>
        <table class="tabel-detail">
          <tr v-for="[f, l] in baris" :key="f"><th>{{ l }}</th><td>{{ pegawai[f] || '-' }}</td></tr>
        </table>
      </div>
      <DokumenPegawaiPanel :pegawai-id="pegawai.id" :boleh-kelola="true" />
    </div>
  </div>
  <p v-else style="color:var(--cokelat-lembut)">Memuat data pegawai…</p>
</template>

<style scoped>
@media (max-width: 1000px) {
  div[style*='grid-template-columns'] { grid-template-columns: 1fr !important; }
}
</style>
