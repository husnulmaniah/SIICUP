<script setup>
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Menu from 'primevue/menu'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const sidebarOpen = ref(false)
const userMenu = ref(null)

const roleLabel = computed(() => {
  const map = { administrator: 'Administrator', admin: 'Admin', pegawai: 'Pegawai', atasan: 'Atasan' }
  return map[auth.role] || auth.role
})

const navSections = computed(() => {
  const sections = [
    { header: null, items: [{ label: 'Dashboard', icon: 'pi pi-home', to: '/dashboard' }] },
    { header: null, items: [{ label: 'Pengajuan Cuti', icon: 'pi pi-calendar', to: '/pengajuan-cuti' }] },
  ]

  if (auth.isPegawai) {
    sections.push({
      header: null,
      items: [
        { label: 'Absen', icon: 'pi pi-camera', to: '/absen' },
        { label: 'Profil Saya', icon: 'pi pi-user-edit', to: '/profil-saya' },
      ],
    })
  }

  if (auth.canManageMaster) {
    sections.push({ header: null, items: [{ label: 'Rekap Absen', icon: 'pi pi-camera', to: '/rekap-absen' }] })
  }

  if (auth.canManageMaster) {
    sections.push({
      header: 'Data Kepegawaian',
      items: [
        { label: 'Data Pegawai', icon: 'pi pi-users', to: '/master/pegawai' },
        { label: 'Perubahan Data Pegawai', icon: 'pi pi-user-edit', to: '/perubahan-data' },
        { label: 'Jatah Cuti Tahunan', icon: 'pi pi-briefcase', to: '/master/jatah-cuti' },
      ],
    })
    sections.push({
      header: 'Master Data',
      // Tabel referensi inti (Jabatan, Unit Kerja, dst.) hanya boleh
      // dilihat/diubah oleh administrator -- role "admin" (Admin
      // Kepegawaian) hanya butuh mengelola "Tanggal Merah" (kalender hari
      // libur dipakai saat menghitung hari cuti), jadi menu-menu lain
      // disembunyikan untuknya agar tidak mengarah ke halaman yang memang
      // akan ditolak backend.
      items: [
        ...(auth.isAdministrator
          ? [
              { label: 'Jabatan', icon: 'pi pi-id-card', to: '/master/jabatan' },
              { label: 'Unit Kerja', icon: 'pi pi-sitemap', to: '/master/unit-kerja' },
              { label: 'Status Pegawai', icon: 'pi pi-tag', to: '/master/status' },
              { label: 'Pangkat', icon: 'pi pi-star', to: '/master/pangkat' },
              { label: 'Golongan', icon: 'pi pi-hashtag', to: '/master/golongan' },
              { label: 'Pangkat / Golongan', icon: 'pi pi-th-large', to: '/master/pangkat-gol' },
              { label: 'Jenis Cuti', icon: 'pi pi-book', to: '/master/jenis-cuti' },
              { label: 'Pola Hari Kerja', icon: 'pi pi-clock', to: '/master/pola-hari-kerja' },
            ]
          : []),
        { label: 'Tanggal Merah', icon: 'pi pi-calendar-times', to: '/master/tgl-merah' },
      ],
    })
  }

  if (auth.canManageMaster) {
    sections.push({
      header: 'Administrasi',
      items: [
        ...(auth.isAdministrator ? [{ label: 'Role', icon: 'pi pi-shield', to: '/master/role' }, { label: 'Akun Pengguna', icon: 'pi pi-user', to: '/master/user' }] : []),
        { label: 'Pengaturan Formulir', icon: 'pi pi-file-edit', to: '/pengaturan-formulir' },
      ],
    })
  }

  return sections
})

const userMenuItems = [
  {
    label: 'Keluar',
    icon: 'pi pi-sign-out',
    command: () => {
      auth.logout()
      router.push({ name: 'login' })
    },
  },
]

function isActive(to) {
  return route.path === to
}

function navigate(to) {
  sidebarOpen.value = false
  router.push(to)
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="sidebar-brand">
        <i class="pi pi-calendar-plus" style="font-size: 1.4rem"></i>
        <span>SI Cuti Pegawai</span>
      </div>
      <nav class="sidebar-nav">
        <template v-for="(section, si) in navSections" :key="si">
          <div v-if="section.header" class="nav-section-header">{{ section.header }}</div>
          <a
            v-for="item in section.items"
            :key="item.to"
            class="nav-item"
            :class="{ active: isActive(item.to) }"
            @click="navigate(item.to)"
          >
            <i :class="item.icon"></i>
            <span>{{ item.label }}</span>
          </a>
        </template>
      </nav>
    </aside>

    <div v-if="sidebarOpen" class="sidebar-backdrop" @click="sidebarOpen = false"></div>

    <div class="main-area">
      <header class="topbar">
        <button class="hamburger" @click="sidebarOpen = !sidebarOpen" aria-label="Menu">
          <i class="pi pi-bars"></i>
        </button>
        <div class="topbar-title">{{ roleLabel }}</div>
        <div class="topbar-user" @click="userMenu.toggle($event)">
          <Avatar :label="(auth.user?.nama || '?').charAt(0)" shape="circle" style="background: #6366f1; color: #fff" />
          <span class="user-name">{{ auth.user?.nama }}</span>
          <i class="pi pi-angle-down"></i>
        </div>
        <Menu ref="userMenu" :model="userMenuItems" :popup="true" />
      </header>

      <main class="content-area">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  min-height: 100vh;
}

.sidebar {
  width: 260px;
  background: #111827;
  color: #e5e7eb;
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  z-index: 40;
  display: flex;
  flex-direction: column;
  transform: translateX(0);
  transition: transform 0.2s ease;
  overflow-y: auto;
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 1.15rem 1.25rem;
  font-weight: 700;
  font-size: 1.02rem;
  color: #fff;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.sidebar-nav {
  padding: 0.75rem 0.6rem;
  flex: 1;
}

.nav-section-header {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: #6b7280;
  padding: 1rem 0.6rem 0.35rem;
  font-weight: 600;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  padding: 0.6rem 0.7rem;
  border-radius: 8px;
  cursor: pointer;
  color: #d1d5db;
  font-size: 0.89rem;
  margin-bottom: 0.1rem;
}

.nav-item:hover {
  background: rgba(255, 255, 255, 0.06);
  color: #fff;
}

.nav-item.active {
  background: #6366f1;
  color: #fff;
}

.sidebar-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  z-index: 35;
  display: none;
}

.main-area {
  flex: 1;
  margin-left: 260px;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar {
  height: 60px;
  background: #fff;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0 1.25rem;
  position: sticky;
  top: 0;
  z-index: 20;
}

.hamburger {
  display: none;
  background: none;
  border: none;
  font-size: 1.2rem;
  cursor: pointer;
  color: #374151;
}

.topbar-title {
  font-weight: 600;
  color: #374151;
}

.topbar-user {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
  padding: 0.3rem 0.5rem;
  border-radius: 8px;
}

.topbar-user:hover {
  background: #f3f4f6;
}

.user-name {
  font-size: 0.88rem;
  font-weight: 500;
  color: #374151;
}

.content-area {
  flex: 1;
}

@media (max-width: 900px) {
  .sidebar {
    transform: translateX(-100%);
  }
  .sidebar.open {
    transform: translateX(0);
  }
  .sidebar-backdrop {
    display: block;
  }
  .main-area {
    margin-left: 0;
  }
  .hamburger {
    display: inline-block;
  }
  .user-name {
    display: none;
  }
}
</style>
