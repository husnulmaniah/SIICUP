import axios from 'axios'
import router from '../router'

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api',
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('cuti_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response && error.response.status === 401) {
      // Bersihkan state store auth (bukan cuma localStorage) supaya
      // auth.isLoggedIn langsung false dan router guard tidak
      // membatalkan redirect ke halaman login (lihat router/index.js).
      try {
        const { useAuthStore } = await import('../stores/auth')
        useAuthStore().logout()
      } catch {
        localStorage.removeItem('cuti_token')
        localStorage.removeItem('cuti_user')
      }
      if (router.currentRoute.value.name !== 'login') {
        router.push({ name: 'login' })
      }
    }
    return Promise.reject(error)
  },
)

export default http
