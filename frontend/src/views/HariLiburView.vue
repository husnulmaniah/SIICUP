<script setup>
import { ref, computed, onMounted } from 'vue';
import Swal from 'sweetalert2';
import api, { pesanError, toast, swalTema } from '../api';
import { formatTanggal } from '../cuti-config';

const daftar = ref([]);
const loading = ref(true);
const tanggal = ref('');
const keterangan = ref('');

async function muat() {
  loading.value = true;
  const { data } = await api.get('/hari-libur');
  daftar.value = data;
  loading.value = false;
}
onMounted(muat);

const dataTabel = computed(() =>
  daftar.value.map((h) => [h.tanggal, formatTanggal(h.tanggal), h.keterangan, h.id])
);

const kolomDT = [
  { title: 'Tanggal' },
  { title: 'Hari' },
  { title: 'Keterangan' },
  {
    title: 'Aksi', orderable: false, searchable: false,
    render: (id) => `<button class="btn btn-merah btn-kecil" data-id="${id}">Hapus</button>`,
  },
];

const dtOptions = {
  pageLength: 25, order: [[0, 'asc']],
  language: {
    search: 'Cari:', lengthMenu: 'Tampilkan _MENU_ data', info: 'Menampilkan _START_–_END_ dari _TOTAL_ hari libur',
    infoEmpty: 'Belum ada hari libur terdaftar', zeroRecords: 'Tidak ditemukan', infoFiltered: '(disaring dari _MAX_ total)',
    paginate: { first: '«', last: '»', next: '›', previous: '‹' },
  },
};

async function tambah() {
  try {
    await api.post('/hari-libur', { tanggal: tanggal.value, keterangan: keterangan.value });
    toast.fire({ icon: 'success', title: 'Hari libur ditambahkan' });
    tanggal.value = '';
    keterangan.value = '';
    await muat();
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal menambah', text: pesanError(e), ...swalTema });
  }
}

async function klikTabel(e) {
  const btn = e.target.closest('button[data-id]');
  if (!btn) return;
  const h = daftar.value.find((x) => x.id === Number(btn.dataset.id));
  const ok = await Swal.fire({
    icon: 'warning', title: `Hapus libur ${h.tanggal}?`, text: h.keterangan,
    showCancelButton: true, confirmButtonText: 'Ya, hapus', cancelButtonText: 'Batal', ...swalTema,
  });
  if (!ok.isConfirmed) return;
  try {
    await api.delete(`/hari-libur/${h.id}`);
    toast.fire({ icon: 'success', title: 'Hari libur dihapus' });
    await muat();
  } catch (e2) {
    Swal.fire({ icon: 'error', title: 'Gagal menghapus', text: pesanError(e2), ...swalTema });
  }
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Hari Libur</h2>
        <p>Daftar hari libur nasional/cuti bersama. Tanggal di sini tidak dihitung sebagai hari cuti (pada pola 5/6 hari kerja).</p>
      </div>
    </div>

    <div class="card">
      <form @submit.prevent="tambah" style="display:flex; gap:12px; align-items:flex-end; flex-wrap:wrap">
        <div>
          <label class="form-label">Tanggal</label>
          <input v-model="tanggal" type="date" class="form-input" required />
        </div>
        <div style="flex:1; min-width:220px">
          <label class="form-label">Keterangan</label>
          <input v-model="keterangan" class="form-input" placeholder="Contoh: Hari Kemerdekaan RI" required />
        </div>
        <button class="btn btn-hijau">＋ Tambah Hari Libur</button>
      </form>
    </div>

    <div class="card" @click="klikTabel">
      <p v-if="loading" style="color:var(--cokelat-lembut)">Memuat hari libur…</p>
      <DataTable v-else :data="dataTabel" :columns="kolomDT" :options="dtOptions" class="dataTable" width="100%" />
    </div>
  </div>
</template>
