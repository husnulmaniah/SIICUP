<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')

async function submit() {
  if (!username.value || !password.value) {
    errorMsg.value = 'Username dan password wajib diisi'
    return
  }
  errorMsg.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    router.push({ name: 'dashboard' })
  } catch (e) {
    errorMsg.value = e.response?.data?.message || 'Login gagal, periksa kembali username/password'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <!-- dua lingkaran blur dekoratif -- permintaan pengguna "tampilan lebih
         modern": murni hiasan CSS (tanpa gambar), aman diabaikan pembaca
         layar (aria-hidden) dan tidak mengubah apa pun secara fungsional. -->
    <div class="login-decor login-decor-1" aria-hidden="true"></div>
    <div class="login-decor login-decor-2" aria-hidden="true"></div>
    <div class="login-card">
      <div class="login-brand">
        <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="login-logo" />
        <div>
          <div class="login-instansi">Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara</div>
          <h1>SIMADU</h1>
          <div class="login-kepanjangan">Sistem Informasi Manajemen Administrasi Dinas Utama</div>
        </div>
      </div>
      <p class="login-subtitle">Masuk untuk mengelola administrasi dinas utama</p>

      <Message v-if="errorMsg" severity="error" :closable="false" style="margin-bottom: 1rem">{{ errorMsg }}</Message>

      <form @submit.prevent="submit" style="display: flex; flex-direction: column; gap: 1rem">
        <div>
          <label class="field-label">Username</label>
          <InputText v-model="username" style="width: 100%" placeholder="masukkan username" autofocus />
        </div>
        <div>
          <label class="field-label">Password</label>
          <Password v-model="password" style="width: 100%" inputStyle="width: 100%" :feedback="false" toggleMask placeholder="masukkan password" />
        </div>
        <Button type="submit" label="Masuk" :loading="loading" style="width: 100%; margin-top: 0.5rem" />
      </form>

      <div class="login-footer">
        &copy; {{ new Date().getFullYear() }} Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0d3b52 0%, #0d9488 55%, #5eead4 100%);
  padding: 1rem;
  position: relative;
  overflow: hidden;
}

/* lingkaran blur besar di pojok -- hanya dekorasi, memberi kesan kedalaman
   pada gradien latar yang sebelumnya polos. */
.login-decor {
  position: absolute;
  border-radius: 50%;
  filter: blur(4px);
  pointer-events: none;
}

.login-decor-1 {
  width: 420px;
  height: 420px;
  top: -140px;
  left: -140px;
  background: radial-gradient(circle, rgba(255, 255, 255, 0.16), transparent 70%);
}

.login-decor-2 {
  width: 520px;
  height: 520px;
  bottom: -220px;
  right: -180px;
  background: radial-gradient(circle, rgba(255, 255, 255, 0.12), transparent 70%);
}

.login-card {
  position: relative;
  z-index: 1;
  background: #fff;
  border-radius: 20px;
  padding: 2.25rem 2rem;
  width: 100%;
  max-width: 380px;
  box-shadow: 0 25px 60px -10px rgba(0, 0, 0, 0.25);
  animation: login-card-in 0.35s ease both;
}

@keyframes login-card-in {
  from {
    opacity: 0;
    transform: translateY(10px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .login-card {
    animation: none;
  }
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: #0f766e;
  margin-bottom: 0.25rem;
}

.login-logo {
  height: 48px;
  width: auto;
  flex-shrink: 0;
}

.login-instansi {
  font-size: 0.72rem;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.login-brand h1 {
  font-size: 1.15rem;
  margin: 0.1rem 0 0 0;
}

.login-kepanjangan {
  font-size: 0.72rem;
  color: #64748b;
  margin-top: 0.1rem;
  line-height: 1.3;
}

.login-subtitle {
  color: #64748b;
  font-size: 0.88rem;
  margin: 0 0 1.25rem 0;
}

.field-label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 0.35rem;
  color: #374151;
}

.login-footer {
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid #e5e7eb;
  font-size: 0.72rem;
  color: #94a3b8;
  text-align: center;
  line-height: 1.5;
}
</style>
