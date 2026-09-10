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
    <div class="login-card">
      <div class="login-brand">
        <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="login-logo" />
        <div>
          <div class="login-instansi">Dinas Pendidikan dan Kebudayaan Daerah Kabupaten Morowali Utara</div>
          <h1>SI Cuti Pegawai</h1>
        </div>
      </div>
      <p class="login-subtitle">Masuk untuk mengelola pengajuan cuti</p>

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
  background: linear-gradient(135deg, #4f46e5 0%, #6366f1 50%, #818cf8 100%);
  padding: 1rem;
}

.login-card {
  background: #fff;
  border-radius: 16px;
  padding: 2.25rem 2rem;
  width: 100%;
  max-width: 380px;
  box-shadow: 0 20px 45px rgba(0, 0, 0, 0.18);
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: #4338ca;
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
