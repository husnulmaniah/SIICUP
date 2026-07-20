<script setup>
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import Swal from 'sweetalert2';
import api, { pesanError, toast, swalTema } from '../api';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const router = useRouter();
const pegawai = ref([]);
const loading = ref(true);
const tampilSemua = ref(false);
const fileImport = ref(null);
const sedangImport = ref(false);

// modal tambah/edit
const showModal = ref(false);
const editId = ref(null);
const form = ref({});
const kolomForm = [
  ['nip', 'NIP BARU *'], ['nik', 'NIK'], ['nama', 'Nama Lengkap *'], ['tempatLahir', 'Tempat Lahir'],
  ['tanggalLahir', 'Tanggal Lahir'], ['jenisKelamin', 'Jenis Kelamin (L/P)'], ['statusCpnsPns', 'Status CPNS/PNS'],
  ['tanggalSkCpns', 'Tanggal SK CPNS'], ['tmtCpns', 'TMT CPNS'], ['tanggalSkPns', 'Tanggal SK PNS'],
  ['tmtPns', 'TMT PNS'], ['golAwal', 'Gol. Awal'], ['golAkhir', 'Gol. Akhir'], ['tmtGolongan', 'TMT Golongan'],
  ['jenisJabatan', 'Jenis Jabatan'], ['jabatan', 'Jabatan'], ['tmtJabatan', 'TMT Jabatan'],
  ['tingkatPendidikan', 'Tingkat Pendidikan'], ['pendidikan', 'Pendidikan'], ['kecamatan', 'Kecamatan'],
  ['unor', 'UNOR (Unit Organisasi)'], ['tempatTugas', 'Tempat Tugas'], ['status', 'Status'],
  ['tanggalKgbTerakhir', 'Tgl KGB Terakhir'], ['tanggalKpTerakhir', 'Tgl KP Terakhir'],
  ['tanggalPensiun', 'Tanggal Pensiun'], ['tanggalKgb', 'Tgl KGB Berikutnya'], ['tanggalKp', 'Tgl KP Berikutnya'],
  ['tahunPengangkatan', 'Tahun Pengangkatan'],
];

const dtOptions = {
  pageLength: 10,
  lengthMenu: [10, 25, 50, 100],
  order: [[1, 'asc']],
  language: {
    search: 'Cari:', lengthMenu: 'Tampilkan _MENU_ data', info: 'Menampilkan _START_–_END_ dari _TOTAL_ pegawai',
    infoEmpty: 'Tidak ada data', infoFiltered: '(disaring dari _MAX_ total)', zeroRecords: 'Data tidak ditemukan',
    paginate: { first: '«', last: '»', next: '›', previous: '‹' },
  },
};

const dataTabel = computed(() =>
  pegawai.value.map((p) => [
    p.nip, p.nama, p.jabatan || '-', p.tempatTugas || p.unor || '-', p.golAkhir || '-',
    p.statusCpnsPns || '-', p.aktif ? 'AKTIF' : 'NONAKTIF', p.id,
  ])
);

async function muat() {
  loading.value = true;
  const { data } = await api.get('/pegawai', { params: { semua: tampilSemua.value ? 1 : 0 } });
  pegawai.value = data;
  loading.value = false;
}
onMounted(muat);

async function unduh(url, namaDefault) {
  try {
    const res = await api.get(url, { responseType: 'blob' });
    const href = URL.createObjectURL(res.data);
    const a = Object.assign(document.createElement('a'), { href, download: namaDefault });
    a.click();
    URL.revokeObjectURL(href);
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal mengunduh', text: pesanError(e), ...swalTema });
  }
}
const unduhTemplate = () => unduh('/pegawai/template', 'TEMPLATE_IMPORT_PEGAWAI.xlsx');
const eksporExcel = () => unduh(`/pegawai/export?semua=${tampilSemua.value ? 1 : 0}`, 'DATA_PEGAWAI_AKTIF.xlsx');

async function imporExcel() {
  const file = fileImport.value?.files?.[0];
  if (!file) return Swal.fire({ icon: 'info', title: 'Pilih file dulu', text: 'Pilih file .xlsx sesuai template.', ...swalTema });
  sedangImport.value = true;
  const fd = new FormData();
  fd.append('file', file);
  try {
    const { data } = await api.post('/pegawai/import', fd);
    Swal.fire({ icon: 'success', title: 'Import berhasil', text: data.message, ...swalTema });
    fileImport.value.value = '';
    await muat();
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Import gagal', text: pesanError(e), ...swalTema });
  } finally {
    sedangImport.value = false;
  }
}

function bukaTambah() { editId.value = null; form.value = { aktif: true }; showModal.value = true; }
function bukaEdit(id) {
  const p = pegawai.value.find((x) => x.id === id);
  editId.value = id;
  form.value = { ...p };
  showModal.value = true;
}
async function simpan() {
  try {
    if (editId.value) await api.put(`/pegawai/${editId.value}`, form.value);
    else await api.post('/pegawai', form.value);
    toast.fire({ icon: 'success', title: editId.value ? 'Data pegawai diperbarui' : 'Pegawai ditambahkan, akun NIP dibuat' });
    showModal.value = false;
    await muat();
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal menyimpan', text: pesanError(e), ...swalTema });
  }
}
async function toggleAktif(id) {
  const p = pegawai.value.find((x) => x.id === id);
  await api.put(`/pegawai/${id}`, { aktif: !p.aktif });
  toast.fire({ icon: 'success', title: p.aktif ? 'Pegawai dinonaktifkan' : 'Pegawai diaktifkan' });
  await muat();
}
async function hapus(id) {
  const p = pegawai.value.find((x) => x.id === id);
  const ok = await Swal.fire({
    icon: 'warning', title: `Hapus ${p.nama}?`,
    text: 'Data pegawai, akun login, dan riwayat cutinya akan ikut terhapus.',
    showCancelButton: true, confirmButtonText: 'Ya, hapus', cancelButtonText: 'Batal', ...swalTema,
  });
  if (!ok.isConfirmed) return;
  try {
    await api.delete(`/pegawai/${id}`);
    toast.fire({ icon: 'success', title: 'Pegawai dihapus' });
    await muat();
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal menghapus', text: pesanError(e), ...swalTema });
  }
}

// Delegasi klik tombol di dalam DataTable
function klikTabel(e) {
  const btn = e.target.closest('button[data-aksi]');
  if (!btn) return;
  const id = Number(btn.dataset.id);
  if (btn.dataset.aksi === 'detail') return router.push(`/pegawai/${id}`);
  if (btn.dataset.aksi === 'edit') bukaEdit(id);
  if (btn.dataset.aksi === 'toggle') toggleAktif(id);
  if (btn.dataset.aksi === 'hapus') hapus(id);
}

const kolomDT = [
  { title: 'NIP' },
  { title: 'Nama' },
  { title: 'Jabatan' },
  { title: 'Tempat Tugas' },
  { title: 'Gol. Akhir' },
  { title: 'Status CPNS/PNS' },
  { title: 'Status', render: (d) => `<span class="badge ${d === 'AKTIF' ? 'badge-DISETUJUI' : 'badge-DITOLAK'}">${d}</span>` },
  {
    title: 'Aksi', orderable: false, searchable: false,
    render: (id, _t, row) => `
      <button class="btn btn-terakota btn-kecil" data-aksi="detail" data-id="${id}">Detail</button>
      <button class="btn btn-putih btn-kecil" data-aksi="edit" data-id="${id}">Ubah</button>
      <button class="btn btn-kuning btn-kecil" data-aksi="toggle" data-id="${id}">${row[6] === 'AKTIF' ? 'Nonaktifkan' : 'Aktifkan'}</button>
      ${auth.isAdminUtama ? `<button class="btn btn-merah btn-kecil" data-aksi="hapus" data-id="${id}">Hapus</button>` : ''}
    `,
  },
];
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Data Pegawai Aktif</h2>
        <p>Kelola data pegawai: unduh template, import dari Excel, dan export ke Excel.</p>
      </div>
      <button class="btn btn-terakota" @click="bukaTambah">＋ Tambah Pegawai</button>
    </div>

    <div class="card" style="display:flex; flex-wrap:wrap; gap:12px; align-items:center">
      <button class="btn btn-putih" @click="unduhTemplate">📄 Unduh Template</button>
      <div style="display:flex; gap:8px; align-items:center; border:1.5px dashed var(--garis); padding:6px 10px; border-radius:9px; background:#fffdf9">
        <input type="file" ref="fileImport" accept=".xlsx,.xls" style="font-size:13px" />
        <button class="btn btn-hijau" :disabled="sedangImport" @click="imporExcel">{{ sedangImport ? 'Mengimpor…' : '⬆️ Import Excel' }}</button>
      </div>
      <button class="btn btn-hijau" @click="eksporExcel">⬇️ Export Excel</button>
      <label style="margin-left:auto; display:flex; gap:7px; align-items:center; font-size:13px; cursor:pointer">
        <input type="checkbox" v-model="tampilSemua" @change="muat" /> Tampilkan pegawai nonaktif
      </label>
    </div>

    <div class="card" @click="klikTabel">
      <p v-if="loading" style="color:var(--cokelat-lembut)">Memuat data pegawai…</p>
      <DataTable v-else :data="dataTabel" :columns="kolomDT" :options="dtOptions" class="dataTable" width="100%" />
    </div>

    <!-- Modal tambah/edit -->
    <div v-if="showModal" style="position:fixed; inset:0; background:rgba(59,35,20,.45); display:grid; place-items:center; z-index:50; padding:20px" @click.self="showModal = false">
      <div class="card" style="max-width:820px; width:100%; max-height:88vh; overflow:auto; margin:0">
        <h3>{{ editId ? 'Ubah Data Pegawai' : 'Tambah Pegawai Baru' }}</h3>
        <p class="hint" v-if="!editId">Akun login otomatis dibuat: username dan password awal = NIP.</p>
        <form @submit.prevent="simpan">
          <div class="form-grid">
            <div v-for="[field, label] in kolomForm" :key="field">
              <label class="form-label">{{ label }}</label>
              <input v-model="form[field]" class="form-input" :required="label.includes('*')" />
            </div>
          </div>
          <div style="display:flex; gap:10px; justify-content:flex-end; margin-top:18px">
            <button type="button" class="btn btn-putih" @click="showModal = false">Batal</button>
            <button type="submit" class="btn btn-hijau">Simpan</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
