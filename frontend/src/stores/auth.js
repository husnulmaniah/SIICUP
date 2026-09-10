import { defineStore } from 'pinia';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('sicuti_token') || null,
    user: JSON.parse(localStorage.getItem('sicuti_user') || 'null'),
  }),
  getters: {
    isLoggedIn: (s) => !!s.token,
    isAdmin: (s) => ['ADMIN_UTAMA', 'ADMIN_PEMBANTU'].includes(s.user?.role),
    isAdminUtama: (s) => s.user?.role === 'ADMIN_UTAMA',
    roleLabel: (s) => ({ ADMIN_UTAMA: 'Admin Utama', ADMIN_PEMBANTU: 'Admin Pembantu', PEGAWAI: 'Pegawai' }[s.user?.role] || ''),
  },
  actions: {
    setLogin({ token, user }) {
      this.token = token; this.user = user;
      localStorage.setItem('sicuti_token', token);
      localStorage.setItem('sicuti_user', JSON.stringify(user));
    },
    selesaiGantiPassword() {
      if (this.user) {
        this.user = { ...this.user, mustChangePassword: false };
        localStorage.setItem('sicuti_user', JSON.stringify(this.user));
      }
    },
    logout(redirect = true) {
      this.token = null; this.user = null;
      localStorage.removeItem('sicuti_token');
      localStorage.removeItem('sicuti_user');
      if (redirect) location.href = '/login';
    },
  },
});
