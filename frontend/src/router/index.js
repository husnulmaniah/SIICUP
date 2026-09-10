import { createRouter, createWebHistory } from 'vue-router';
import { useAuthStore } from '../stores/auth';

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue'), meta: { publik: true } },
  { path: '/', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
  { path: '/pegawai', name: 'pegawai', component: () => import('../views/PegawaiView.vue'), meta: { admin: true } },
  { path: '/pegawai/:id', name: 'pegawai-detail', component: () => import('../views/PegawaiDetailView.vue'), meta: { admin: true } },
  { path: '/cuti', name: 'cuti', component: () => import('../views/CutiListView.vue') },
  { path: '/cuti/ajukan', name: 'cuti-ajukan', component: () => import('../views/CutiFormView.vue') },
  { path: '/cuti/:id', name: 'cuti-detail', component: () => import('../views/CutiDetailView.vue') },
  { path: '/perubahan-data', name: 'perubahan', component: () => import('../views/PerubahanDataView.vue') },
  { path: '/laporan', name: 'laporan', component: () => import('../views/LaporanView.vue'), meta: { admin: true } },
  { path: '/hari-libur', name: 'hari-libur', component: () => import('../views/HariLiburView.vue'), meta: { admin: true } },
  { path: '/akun', name: 'akun', component: () => import('../views/UsersView.vue'), meta: { adminUtama: true } },
  { path: '/profil', name: 'profil', component: () => import('../views/ProfilView.vue') },
];

const router = createRouter({ history: createWebHistory(), routes });

router.beforeEach((to) => {
  const auth = useAuthStore();
  if (!to.meta.publik && !auth.isLoggedIn) return '/login';
  if (to.meta.publik && auth.isLoggedIn) return '/';
  // Wajib ganti password default sebelum mengakses halaman lain
  if (auth.isLoggedIn && auth.user?.mustChangePassword && to.path !== '/profil') return '/profil';
  if (to.meta.admin && !auth.isAdmin) return '/';
  if (to.meta.adminUtama && !auth.isAdminUtama) return '/';
});

export default router;
