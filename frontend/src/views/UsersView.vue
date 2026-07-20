<script setup>
import { ref, computed, onMounted } from 'vue';
import Swal from 'sweetalert2';
import api, { pesanError, toast, swalTema } from '../api';
import { useAuthStore } from '../stores/auth';
import { formatWaktu } from '../cuti-config';

const auth = useAuthStore();
const users = ref([]);
const loading = ref(true);
const showForm = ref(false);
const form = ref({ role: 'ADMIN_PEMBANTU' });

const roleLabel = { ADMIN_UTAMA: 'Admin Utama', ADMIN_PEMBANTU: 'Admin Pembantu', PEGAWAI: 'Pegawai' };

async function muat() {
  loading.value = true;
  const { data } = await api.get('/users');
  users.value = data;
  loading.value = false;
}
onMounted(muat);

const dataTabel = computed(() =>
  users.value.map((u) => [u.username, u.nama, roleLabel[u.role], formatWaktu(u.createdAt), u.id])
);

const kolomDT = [
  { title: 'Username' },
  { title: 'Nama' },
  { title: 'Role', render: (d) => `<span class="badge ${d === 'Pegawai' ? 'badge-DIAJUKAN' : 'badge-DISETUJUI'}">${d}</span>` },
  { title: 'Dibuat' },
  {
    title: 'Aksi', orderable: false, searchable: false,
    render: (id, _t, row) => {
      const rolenya = row[2];
      let tombolRole = '';
      if (rolenya === 'Pegawai') tombolRole = `<button class="btn btn-terakota btn-kecil" data-aksi="role" data-role="ADMIN_PEMBANTU" data-id="${id}">Jadikan Admin Pembantu</button>`;
      else if (rolenya === 'Admin Pembantu') tombolRole = `<button class="btn btn-terakota btn-kecil" data-aksi="role" data-role="PEGAWAI" data-id="${id}">Jadikan Pegawai</button>`;
      return `${tombolRole}
      <button class="btn btn-kuning btn-kecil" data-aksi="reset" data-id="${id}">Reset Password</button>
      <button class="btn btn-merah btn-kecil" data-aksi="hapus" data-id="${id}">Hapus</button>`;
    },
  },
];

const dtOptions = {
  pageLength: 10, order: [],
  language: {
    search: 'Cari:', lengthMenu: 'Tampilkan _MENU_ akun', info: 'Menampilkan _START_–_END_ dari _TOTAL_ akun',
    infoEmpty: 'Belum ada akun', zeroRecords: 'Akun tidak ditemukan', infoFiltered: '(disaring dari _MAX_ total)',
    paginate: { first: '«', last: '»', next: '›', previous: '‹' },
  },
};

async function klikTabel(e) {
  const btn = e.target.closest('button[data-aksi]');
  if (!btn) return;
  const id = Number(btn.dataset.id);
  const u = users.value.find((x) => x.id === id);
  if (btn.dataset.aksi === 'role') {
    const roleBaru = btn.dataset.role;
    const ok = await Swal.fire({
      icon: 'question',
      title: `Ubah role ${u.username}?`,
      text: roleBaru === 'ADMIN_PEMBANTU'
        ? `${u.nama} akan menjadi Admin Pembantu: dapat mengelola pegawai, memproses cuti, dan membuat laporan.`
        : `${u.nama} akan kembali menjadi Pegawai biasa.`,
      showCancelButton: true, confirmButtonText: 'Ya, ubah role', cancelButtonText: 'Batal', ...swalTema,
    });
    if (!ok.isConfirmed) return;
    try {
      const { data } = await api.patch(`/users/${id}`, { role: roleBaru });
      toast.fire({ icon: 'success', title: data.message });
      await muat();
    } catch (err) {
      Swal.fire({ icon: 'error', title: 'Gagal mengubah role', text: pesanError(err), ...swalTema });
    }
    return;
  }
  if (btn.dataset.aksi === 'reset') {
    const { value, isConfirmed } = await Swal.fire({
      title: `Reset password ${u.username}?`,
      input: 'text',
      inputLabel: u.role === 'PEGAWAI'
        ? 'Password baru (kosongkan = kembali ke NIP; pengguna wajib menggantinya saat login)'
        : 'Password baru (min. 8 karakter, huruf & angka)',
      showCancelButton: true, confirmButtonText: 'Reset', cancelButtonText: 'Batal', ...swalTema,
    });
    if (!isConfirmed) return;
    try {
      const { data } = await api.put(`/users/${id}`, { passwordBaru: value || undefined });
      toast.fire({ icon: 'success', title: data.message });
    } catch (err) {
      Swal.fire({ icon: 'error', title: 'Gagal reset', text: pesanError(err), ...swalTema });
    }
  }
  if (btn.dataset.aksi === 'hapus') {
    const ok = await Swal.fire({
      icon: 'warning', title: `Hapus akun ${u.username}?`, showCancelButton: true,
      confirmButtonText: 'Ya, hapus', cancelButtonText: 'Batal', ...swalTema,
    });
    if (!ok.isConfirmed) return;
    try {
      await api.delete(`/users/${id}`);
      toast.fire({ icon: 'success', title: 'Akun dihapus' });
      await muat();
    } catch (err) {
      Swal.fire({ icon: 'error', title: 'Gagal menghapus', text: pesanError(err), ...swalTema });
    }
  }
}

async function simpan() {
  try {
    await api.post('/users', form.value);
    toast.fire({ icon: 'success', title: 'Akun admin dibuat' });
    showForm.value = false;
    form.value = { role: 'ADMIN_PEMBANTU' };
    await muat();
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal membuat akun', text: pesanError(e), ...swalTema });
  }
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Kelola Akun</h2>
        <p>Buat akun admin pembantu, reset password, dan kelola akun pegawai.</p>
      </div>
      <button class="btn btn-terakota" @click="showForm = true">＋ Buat Akun Admin</button>
    </div>

    <div class="card" @click="klikTabel">
      <p v-if="loading" style="color:var(--cokelat-lembut)">Memuat akun…</p>
      <DataTable v-else :data="dataTabel" :columns="kolomDT" :options="dtOptions" class="dataTable" width="100%" />
    </div>

    <div v-if="showForm" style="position:fixed; inset:0; background:rgba(59,35,20,.45); display:grid; place-items:center; z-index:50; padding:20px" @click.self="showForm = false">
      <div class="card" style="max-width:440px; width:100%; margin:0">
        <h3>Buat Akun Admin</h3>
        <form @submit.prevent="simpan">
          <label class="form-label">Nama Lengkap</label>
          <input v-model="form.nama" class="form-input" required />
          <label class="form-label">Username</label>
          <input v-model="form.username" class="form-input" required />
          <label class="form-label">Password (min. 8 karakter, huruf &amp; angka)</label>
          <input v-model="form.password" type="password" class="form-input" minlength="8" required />
          <label class="form-label">Role</label>
          <select v-model="form.role" class="form-input">
            <option value="ADMIN_PEMBANTU">Admin Pembantu</option>
            <option value="ADMIN_UTAMA">Admin Utama</option>
          </select>
          <div style="display:flex; gap:10px; justify-content:flex-end; margin-top:16px">
            <button type="button" class="btn btn-putih" @click="showForm = false">Batal</button>
            <button type="submit" class="btn btn-hijau">Buat Akun</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
