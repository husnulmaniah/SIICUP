import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { getConfig } from '../config/tables'

import LoginView from '../views/LoginView.vue'
import AppLayout from '../layouts/AppLayout.vue'
import DashboardView from '../views/DashboardView.vue'
import PengajuanCutiView from '../views/PengajuanCutiView.vue'
import MasterDataView from '../views/MasterDataView.vue'
import PengaturanFormulirView from '../views/PengaturanFormulirView.vue'
import ProfilSayaView from '../views/ProfilSayaView.vue'
import PerubahanDataView from '../views/PerubahanDataView.vue'
import AbsensiView from '../views/AbsensiView.vue'
import RekapAbsensiView from '../views/RekapAbsensiView.vue'
import NotFoundView from '../views/NotFoundView.vue'

const routes = [
  { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
  {
    path: '/',
    component: AppLayout,
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', name: 'dashboard', component: DashboardView },
      { path: 'pengajuan-cuti', name: 'pengajuan-cuti', component: PengajuanCutiView },
      { path: 'master/:tableKey', name: 'master', component: MasterDataView },
      { path: 'pengaturan-formulir', name: 'pengaturan-formulir', component: PengaturanFormulirView, meta: { roles: ['administrator', 'admin'] } },
      { path: 'profil-saya', name: 'profil-saya', component: ProfilSayaView, meta: { roles: ['pegawai'] } },
      { path: 'absen', name: 'absen', component: AbsensiView, meta: { roles: ['pegawai'] } },
      { path: 'rekap-absen', name: 'rekap-absen', component: RekapAbsensiView, meta: { roles: ['administrator', 'admin'] } },
      { path: 'perubahan-data', name: 'perubahan-data', component: PerubahanDataView, meta: { roles: ['administrator', 'admin'] } },
    ],
  },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { public: true } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()

  if (!to.meta.public && !auth.isLoggedIn) {
    return { name: 'login' }
  }
  if (to.name === 'login' && auth.isLoggedIn) {
    return { name: 'dashboard' }
  }
  if (to.name === 'master') {
    const cfg = getConfig(to.params.tableKey)
    if (!cfg || !cfg.roles.includes(auth.role)) {
      return { name: 'dashboard' }
    }
  }
  if (to.meta.roles && !to.meta.roles.includes(auth.role)) {
    return { name: 'dashboard' }
  }
  return true
})

export default router
