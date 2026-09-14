import { defineStore } from 'pinia'
import http from '../api/http'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('cuti_token') || null,
    user: JSON.parse(localStorage.getItem('cuti_user') || 'null'),
  }),

  getters: {
    isLoggedIn: (state) => !!state.token,
    role: (state) => state.user?.role || null,
    isAdministrator: (state) => state.user?.role === 'administrator',
    isAdmin: (state) => state.user?.role === 'admin',
    isPegawai: (state) => state.user?.role === 'pegawai',
    isAtasan: (state) => state.user?.role === 'atasan',
    // isAdminAbsensi: centang tambahan pada akun (pegawai/atasan/dll), BUKAN
    // role tersendiri -- akun tetap punya menu sesuai role aslinya (Dashboard,
    // Pengajuan Cuti, Absen, dst.) dan HANYA mendapat tambahan 1 menu "Input
    // Rekapan Absensi" (lihat layouts/AppLayout.vue & router/index.js).
    isAdminAbsensi: (state) => !!state.user?.is_admin_absensi,
    canManageMaster: (state) => ['administrator', 'admin'].includes(state.user?.role),
  },

  actions: {
    async login(username, password) {
      const { data } = await http.post('/login', { username, password })
      this.token = data.data.token
      this.user = data.data.user
      localStorage.setItem('cuti_token', this.token)
      localStorage.setItem('cuti_user', JSON.stringify(this.user))
    },

    logout() {
      this.token = null
      this.user = null
      localStorage.removeItem('cuti_token')
      localStorage.removeItem('cuti_user')
    },
  },
})
