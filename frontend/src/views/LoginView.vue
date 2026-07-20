<script setup>
import { ref } from 'vue';
import api, { pesanError } from '../api';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const username = ref('');
const password = ref('');
const error = ref('');
const loading = ref(false);

async function masuk() {
  error.value = '';
  loading.value = true;
  try {
    const { data } = await api.post('/auth/login', { username: username.value, password: password.value });
    auth.setLogin(data);
    location.href = '/';
  } catch (e) {
    error.value = pesanError(e, 'Gagal masuk.');
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="top">
        <h1>SICUTI</h1>
        <div style="font-size:13px; opacity:.9">Sistem Informasi Cuti Guru &amp; Pegawai<br />Dinas Pendidikan &amp; Kebudayaan Kab. Morowali Utara</div>
      </div>
      <div class="body">
        <form @submit.prevent="masuk">
          <label class="form-label">Username</label>
          <input v-model="username" class="form-input" placeholder="NIP untuk pegawai" autocomplete="username" required />
          <label class="form-label">Password</label>
          <input v-model="password" type="password" class="form-input" placeholder="••••••••" autocomplete="current-password" required />
          <p v-if="error" style="color:var(--merah); font-size:13px; margin:10px 0 0">{{ error }}</p>
          <button class="btn btn-hijau" style="width:100%; justify-content:center; margin-top:18px" :disabled="loading">
            {{ loading ? 'Memproses…' : 'Masuk' }}
          </button>
          <p class="hint" style="margin-top:14px; text-align:center">
            Pegawai masuk dengan <b>NIP</b> sebagai username dan password awal.
          </p>
        </form>
      </div>
    </div>
  </div>
</template>
