<script setup>
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import api from '../api';
import { useAuthStore } from '../stores/auth';
import { labelJenis, formatTanggal } from '../cuti-config';

const auth = useAuthStore();
const router = useRouter();
const daftar = ref([]);
const loading = ref(true);

onMounted(async () => {
  const { data } = await api.get('/cuti');
  daftar.value = data;
  loading.value = false;
});

const dataTabel = computed(() =>
  daftar.value.map((c) => [
    `${c.pegawai.nama}|${c.pegawai.nip}`,
    labelJenis(c.jenisCuti),
    `${formatTanggal(c.tanggalMulai)} – ${formatTanggal(c.tanggalSelesai)}`,
    `${c.lamaCuti} hari`,
    c.berkas.length,
    c.status,
    c.id,
  ])
);

const kolomDT = [
  {
    title: 'Pegawai',
    render: (d) => {
      const [nama, nip] = d.split('|');
      return `<b>${nama}</b><br><small style="color:var(--cokelat-lembut)">${nip}</small>`;
    },
  },
  { title: 'Jenis Cuti' },
  { title: 'Periode' },
  { title: 'Lama' },
  { title: 'Berkas', render: (d) => `${d} file` },
  { title: 'Status', render: (d) => `<span class="badge badge-${d}">${d}</span>` },
  {
    title: 'Aksi', orderable: false, searchable: false,
    render: (id) => `<button class="btn btn-putih btn-kecil" data-id="${id}">Detail</button>`,
  },
];

const dtOptions = {
  pageLength: 10,
  order: [],
  language: {
    search: 'Cari:', lengthMenu: 'Tampilkan _MENU_ data', info: 'Menampilkan _START_–_END_ dari _TOTAL_ pengajuan',
    infoEmpty: 'Belum ada pengajuan', zeroRecords: 'Pengajuan tidak ditemukan', infoFiltered: '(disaring dari _MAX_ total)',
    paginate: { first: '«', last: '»', next: '›', previous: '‹' },
  },
};

function klikTabel(e) {
  const btn = e.target.closest('button[data-id]');
  if (btn) router.push(`/cuti/${btn.dataset.id}`);
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Pengajuan Cuti</h2>
        <p>{{ auth.isAdmin ? 'Seluruh pengajuan cuti pegawai — setujui, kembalikan, atau tolak dari halaman detail.' : 'Riwayat pengajuan cuti Anda.' }}</p>
      </div>
      <router-link to="/cuti/ajukan" class="btn btn-terakota">✍️ Ajukan Cuti</router-link>
    </div>
    <div class="card" @click="klikTabel">
      <p v-if="loading" style="color:var(--cokelat-lembut)">Memuat pengajuan…</p>
      <DataTable v-else :data="dataTabel" :columns="kolomDT" :options="dtOptions" class="dataTable" width="100%" />
    </div>
  </div>
</template>
