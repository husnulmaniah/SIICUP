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
    // isAdminVerifikasi: sama pola dengan isAdminAbsensi di atas & independen
    // darinya -- akun dengan ini boleh memverifikasi (menyetujui/
    // mengembalikan) Pengajuan Surat Kolektif pegawai sekolah, terlepas dari
    // role/isAdminAbsensi-nya.
    isAdminVerifikasi: (state) => !!state.user?.is_admin_verifikasi,
    canManageMaster: (state) => ['administrator', 'admin'].includes(state.user?.role),
    // isSekolah: true kalau akun ini terhubung ke data pegawai yang bertugas
    // di SEKOLAH (dihitung backend saat login, lihat is_sekolah pada
    // LoginHandler/handlers/auth.go) -- dipakai untuk menampilkan menu
    // "Template Surat" hanya ke akun sekolah (+ administrator/admin untuk
    // mengelola), tanpa perlu menebak dari data yang mungkin belum lengkap
    // di sisi klien.
    isSekolah: (state) => !!state.user?.is_sekolah,
    canViewTemplateSurat: (state) =>
      ['administrator', 'admin'].includes(state.user?.role) || !!state.user?.is_sekolah,
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
