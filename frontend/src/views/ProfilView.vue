<script setup>
import { ref, onMounted } from 'vue';
import Swal from 'sweetalert2';
import api, { pesanError, toast, swalTema } from '../api';
import { useAuthStore } from '../stores/auth';
import DokumenPegawaiPanel from '../components/DokumenPegawaiPanel.vue';

const auth = useAuthStore();
const profil = ref(null);
const passwordLama = ref('');
const passwordBaru = ref('');

onMounted(async () => {
  const { data } = await api.get('/auth/me');
  profil.value = data;
});

async function gantiPassword() {
  try {
    const { data } = await api.put('/auth/me', { passwordLama: passwordLama.value, passwordBaru: passwordBaru.value });
    toast.fire({ icon: 'success', title: data.message });
    auth.selesaiGantiPassword();
    passwordLama.value = '';
    passwordBaru.value = '';
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal mengubah password', text: pesanError(e), ...swalTema });
  }
}

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
</script>

<template>
  <div v-if="profil">
    <div v-if="auth.user?.mustChangePassword" class="card" style="border-left:5px solid var(--merah); background:#fdf0ee">
      <b style="color:var(--merah)">⚠ Demi keamanan, Anda wajib mengganti password terlebih dahulu.</b>
      <p class="hint" style="margin:4px 0 0">Password Anda masih bawaan (NIP) atau baru direset admin. Menu lain terbuka setelah password diganti.</p>
    </div>
    <div class="page-head">
      <div>
        <h2>Profil Saya</h2>
        <p>{{ profil.nama }} — {{ auth.roleLabel }}</p>
      </div>
      <router-link v-if="profil.pegawai" to="/perubahan-data" class="btn btn-terakota">🔄 Ajukan Perubahan Data</router-link>
    </div>

    <div style="display:grid; grid-template-columns: 1.5fr 1fr; gap:18px; align-items:start">
      <div class="card" v-if="profil.pegawai">
        <h3>Data Kepegawaian</h3>
        <table class="tabel-detail">
          <tr><th>Nama</th><td>{{ profil.pegawai.nama }}</td></tr>
          <tr v-for="[f, l] in baris" :key="f"><th>{{ l }}</th><td>{{ profil.pegawai[f] || '-' }}</td></tr>
        </table>
        <p class="hint" style="margin-top:10px">Data tidak sesuai? Ajukan perubahan melalui menu Perubahan Data — perubahan berlaku setelah disetujui admin.</p>
      </div>
      <div class="card" v-else>
        <h3>Akun Admin</h3>
        <table class="tabel-detail">
          <tr><th>Username</th><td>{{ profil.username }}</td></tr>
          <tr><th>Nama</th><td>{{ profil.nama }}</td></tr>
          <tr><th>Role</th><td>{{ auth.roleLabel }}</td></tr>
        </table>
      </div>

      <div>
      <DokumenPegawaiPanel v-if="profil.pegawai" :pegawai-id="profil.pegawai.id" :boleh-kelola="true" />
      <div class="card">
        <h3>Ganti Password</h3>
        <form @submit.prevent="gantiPassword">
          <label class="form-label">Password Lama</label>
          <input v-model="passwordLama" type="password" class="form-input" required />
          <label class="form-label">Password Baru</label>
          <input v-model="passwordBaru" type="password" class="form-input" minlength="8" required />
          <p class="hint">Minimal 8 karakter, mengandung huruf dan angka, dan tidak boleh sama dengan NIP/username.</p>
          <button class="btn btn-hijau" style="margin-top:14px">Simpan Password</button>
        </form>
      </div>
      </div>
    </div>
  </div>
  <p v-else style="color:var(--cokelat-lembut)">Memuat profil…</p>
</template>
