<script setup>
import { ref, onMounted } from 'vue';
import api from '../api';
import { useAuthStore } from '../stores/auth';
import StatusBadge from '../components/StatusBadge.vue';
import { labelJenis, formatTanggal } from '../cuti-config';

const auth = useAuthStore();
const stat = ref(null);

onMounted(async () => {
  const { data } = await api.get('/dashboard');
  stat.value = data;
});
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Selamat datang, {{ auth.user?.nama }}</h2>
        <p>Ringkasan pengajuan cuti {{ auth.isAdmin ? 'seluruh pegawai' : 'Anda' }}.</p>
      </div>
      <router-link to="/cuti/ajukan" class="btn btn-terakota">✍️ Ajukan Cuti Baru</router-link>
    </div>

    <div v-if="stat" class="stat-grid">
      <div v-if="auth.isAdmin" class="stat"><div class="angka">{{ stat.pegawaiAktif }}</div><div class="label">Pegawai Aktif</div></div>
      <div class="stat kuning"><div class="angka">{{ stat.diajukan }}</div><div class="label">Cuti Menunggu Proses</div></div>
      <div class="stat"><div class="angka">{{ stat.disetujui }}</div><div class="label">Cuti Disetujui</div></div>
      <div class="stat aksen"><div class="angka">{{ stat.dikembalikan }}</div><div class="label">Cuti Dikembalikan</div></div>
      <div class="stat merah"><div class="angka">{{ stat.ditolak }}</div><div class="label">Cuti Ditolak</div></div>
      <div v-if="auth.isAdmin" class="stat aksen"><div class="angka">{{ stat.perubahanMenunggu }}</div><div class="label">Usulan Perubahan Data</div></div>
    </div>

    <div class="card" v-if="stat">
      <h3>Pengajuan Terbaru</h3>
      <div class="tabel-scroll"><table class="dataTable" style="width:100%">
        <thead>
          <tr><th>Pegawai</th><th>Jenis Cuti</th><th>Tanggal</th><th>Status</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="c in stat.terbaru" :key="c.id">
            <td><b>{{ c.pegawai.nama }}</b><br /><small style="color:var(--cokelat-lembut)">{{ c.pegawai.nip }}</small></td>
            <td>{{ labelJenis(c.jenisCuti) }}</td>
            <td>{{ formatTanggal(c.tanggalMulai) }} – {{ formatTanggal(c.tanggalSelesai) }} ({{ c.lamaCuti }} hari)</td>
            <td><StatusBadge :status="c.status" /></td>
            <td><router-link :to="`/cuti/${c.id}`" class="btn btn-putih btn-kecil">Detail</router-link></td>
          </tr>
          <tr v-if="!stat.terbaru.length"><td colspan="5" style="text-align:center; color:var(--cokelat-lembut)">Belum ada pengajuan. Mulai dengan menu Ajukan Cuti.</td></tr>
        </tbody>
      </table></div>
    </div>
  </div>
</template>
