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
