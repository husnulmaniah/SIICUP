<script setup>
import { ref, computed, onMounted } from 'vue';
import Swal from 'sweetalert2';
import api, { pesanError, toast, swalTema } from '../api';
import { useAuthStore } from '../stores/auth';
import { formatWaktu } from '../cuti-config';

const auth = useAuthStore();
const daftar = ref([]);
const loading = ref(true);
const showForm = ref(false);
const alasan = ref('');
const dataBaru = ref({});
const kolomForm = [
  ['nama', 'Nama Lengkap'], ['nik', 'NIK'], ['tempatLahir', 'Tempat Lahir'], ['tanggalLahir', 'Tanggal Lahir'],
  ['jenisKelamin', 'Jenis Kelamin (L/P)'], ['statusCpnsPns', 'Status CPNS/PNS'],
  ['tanggalSkCpns', 'Tanggal SK CPNS'], ['tmtCpns', 'TMT CPNS'], ['tanggalSkPns', 'Tanggal SK PNS'],
  ['tmtPns', 'TMT PNS'], ['golAwal', 'Gol. Awal'], ['golAkhir', 'Gol. Akhir'], ['tmtGolongan', 'TMT Golongan'],
  ['jenisJabatan', 'Jenis Jabatan'], ['jabatan', 'Jabatan'], ['tmtJabatan', 'TMT Jabatan'],
  ['tingkatPendidikan', 'Tingkat Pendidikan'], ['pendidikan', 'Pendidikan'], ['kecamatan', 'Kecamatan'],
  ['unor', 'UNOR (Unit Organisasi)'], ['tempatTugas', 'Tempat Tugas'],
  ['tanggalKgbTerakhir', 'Tgl KGB Terakhir'], ['tanggalKpTerakhir', 'Tgl KP Terakhir'],
  ['tanggalPensiun', 'Tanggal Pensiun'], ['tanggalKgb', 'Tgl KGB Berikutnya'], ['tanggalKp', 'Tgl KP Berikutnya'],
  ['tahunPengangkatan', 'Tahun Pengangkatan'],
];
const labelField = Object.fromEntries(kolomForm);

async function muat() {
  loading.value = true;
  const { data } = await api.get('/perubahan');
  daftar.value = data;
  loading.value = false;
}
onMounted(muat);

const parseData = (s) => { try { return JSON.parse(s); } catch { return {}; } };

const dataTabel = computed(() =>
  daftar.value.map((u) => [
    `${u.pegawai.nama}|${u.pegawai.nip}`,
    Object.entries(parseData(u.dataBaru)).map(([k, v]) => `<b>${labelField[k] || k}:</b> ${v}`).join('<br>'),
    u.alasan || '-',
    formatWaktu(u.createdAt),
    u.status,
    u.id,
  ])
);

const kolomDT = [
  { title: 'Pegawai', render: (d) => { const [n, nip] = d.split('|'); return `<b>${n}</b><br><small style="color:var(--cokelat-lembut)">${nip}</small>`; } },
  { title: 'Usulan Perubahan' },
  { title: 'Alasan' },
  { title: 'Diajukan' },
  { title: 'Status', render: (d) => `<span class="badge badge-${d}">${d}</span>` },
  {
    title: 'Aksi', orderable: false, searchable: false,
    render: (id, _t, row) =>
      auth.isAdmin && row[4] === 'DIAJUKAN'
        ? `<button class="btn btn-hijau btn-kecil" data-aksi="SETUJUI" data-id="${id}">Setujui</button>
           <button class="btn btn-merah btn-kecil" data-aksi="TOLAK" data-id="${id}">Tolak</button>`
        : '-',
  },
];

const dtOptions = {
  pageLength: 10, order: [],
  language: {
    search: 'Cari:', lengthMenu: 'Tampilkan _MENU_ data', info: 'Menampilkan _START_–_END_ dari _TOTAL_ usulan',
    infoEmpty: 'Belum ada usulan perubahan data', zeroRecords: 'Usulan tidak ditemukan', infoFiltered: '(disaring dari _MAX_ total)',
    paginate: { first: '«', last: '»', next: '›', previous: '‹' },
  },
};

async function klikTabel(e) {
  const btn = e.target.closest('button[data-aksi]');
  if (!btn) return;
  const { aksi, id } = btn.dataset;
  const { value: catatan, isConfirmed } = await Swal.fire({
    title: aksi === 'SETUJUI' ? 'Setujui & terapkan perubahan?' : 'Tolak usulan ini?',
    text: aksi === 'SETUJUI' ? 'Data pegawai akan langsung diperbarui.' : '',
    input: 'textarea', inputLabel: aksi === 'SETUJUI' ? 'Catatan (opsional)' : 'Alasan penolakan (wajib)',
    showCancelButton: true, confirmButtonText: 'Ya, proses', cancelButtonText: 'Batal', ...swalTema,
    inputValidator: (v) => (aksi === 'TOLAK' && !v?.trim() ? 'Catatan wajib diisi.' : undefined),
  });
  if (!isConfirmed) return;
  try {
    await api.post(`/perubahan/${id}/aksi`, { aksi, catatan });
    toast.fire({ icon: 'success', title: 'Usulan diproses' });
    await muat();
  } catch (err) {
    Swal.fire({ icon: 'error', title: 'Gagal memproses', text: pesanError(err), ...swalTema });
  }
}

async function kirim() {
  try {
    await api.post('/perubahan', { dataBaru: dataBaru.value, alasan: alasan.value });
    toast.fire({ icon: 'success', title: 'Usulan perubahan terkirim' });
    showForm.value = false;
    dataBaru.value = {};
    alasan.value = '';
    await muat();
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal mengirim', text: pesanError(e), ...swalTema });
  }
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Perubahan Data Pegawai</h2>
        <p>{{ auth.isAdmin ? 'Usulan perubahan data dari pegawai — setujui untuk menerapkan langsung.' : 'Ajukan pembaruan data kepegawaian Anda; perubahan berlaku setelah disetujui admin.' }}</p>
      </div>
      <button v-if="!auth.isAdmin" class="btn btn-terakota" @click="showForm = true">＋ Ajukan Perubahan</button>
    </div>

    <div class="card" @click="klikTabel">
      <p v-if="loading" style="color:var(--cokelat-lembut)">Memuat usulan…</p>
      <DataTable v-else :data="dataTabel" :columns="kolomDT" :options="dtOptions" class="dataTable" width="100%" />
    </div>

    <div v-if="showForm" style="position:fixed; inset:0; background:rgba(59,35,20,.45); display:grid; place-items:center; z-index:50; padding:20px" @click.self="showForm = false">
      <div class="card" style="max-width:760px; width:100%; max-height:88vh; overflow:auto; margin:0">
        <h3>Ajukan Perubahan Data</h3>
        <p class="hint">Isi hanya field yang ingin diubah; sisanya biarkan kosong.</p>
        <form @submit.prevent="kirim">
          <div class="form-grid">
            <div v-for="[field, label] in kolomForm" :key="field">
              <label class="form-label">{{ label }}</label>
              <input v-model="dataBaru[field]" class="form-input" :placeholder="'Nilai baru ' + label.toLowerCase()" />
            </div>
          </div>
          <label class="form-label">Alasan Perubahan</label>
          <textarea v-model="alasan" class="form-input" rows="2" placeholder="Contoh: kenaikan pangkat per TMT terbaru"></textarea>
          <div style="display:flex; gap:10px; justify-content:flex-end; margin-top:16px">
            <button type="button" class="btn btn-putih" @click="showForm = false">Batal</button>
            <button type="submit" class="btn btn-hijau">Kirim Usulan</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
