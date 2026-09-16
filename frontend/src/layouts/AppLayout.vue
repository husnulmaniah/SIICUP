<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useProfilePhoto } from '../composables/useProfilePhoto'
import http from '../api/http'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import Popover from 'primevue/popover'
import Badge from 'primevue/badge'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const userMenu = ref(null)

// Foto profil pegawai yang login (kalau ada) untuk avatar topbar -- lihat
// composables/useProfilePhoto.js. State-nya dibagikan dengan ProfilSayaView
// supaya begitu pegawai ganti/hapus foto profilnya sendiri, avatar ini ikut
// berubah seketika tanpa reload halaman.
const { url: fotoProfilUrl, refresh: refreshFotoProfil, clear: clearFotoProfil } = useProfilePhoto()

// ============================================================
// notifikasi lonceng (khusus administrator & admin/Admin Kepegawaian) --
// menghitung & menampilkan Pengajuan Cuti + Perubahan Data Pegawai yang
// masih berstatus "pending" (baru masuk, belum diproses), supaya
// administrator/admin langsung tahu ada pengajuan baru (lihat
// backend/handlers/notifikasi.go untuk sumber datanya).
// ============================================================
const notifPanel = ref(null)
const notifData = ref({ total: 0, items: [] })
const notifLoading = ref(false)
let notifTimer = null

async function fetchNotifikasi() {
  if (!auth.canManageMaster) return
  notifLoading.value = true
  try {
    const { data } = await http.get('/notifikasi/pending')
    notifData.value = data.data
  } catch (e) {
    // diamkan -- gagal memuat notifikasi tidak boleh mengganggu layar utama
  } finally {
    notifLoading.value = false
  }
}

function toggleNotifPanel(event) {
  fetchNotifikasi()
  notifPanel.value?.toggle(event)
}

function bukaNotifikasi(item) {
  notifPanel.value?.hide()
  router.push(item.link)
}

function notifIcon(type) {
  if (type === 'pengajuan_cuti') return 'pi pi-calendar'
  if (type === 'pengajuan_pensiun') return 'pi pi-briefcase'
  return 'pi pi-user-edit'
}

function notifTitle(type) {
  if (type === 'pengajuan_cuti') return 'Pengajuan Cuti baru'
  if (type === 'pengajuan_pensiun') return 'Pengajuan Pensiun baru'
  return 'Perubahan Data Pegawai baru'
}

function waktuRelatif(iso) {
  if (!iso) return ''
  const diffMs = Date.now() - new Date(iso).getTime()
  const menit = Math.floor(diffMs / 60000)
  if (menit < 1) return 'Baru saja'
  if (menit < 60) return `${menit} menit lalu`
  const jam = Math.floor(menit / 60)
  if (jam < 24) return `${jam} jam lalu`
  return `${Math.floor(jam / 24)} hari lalu`
}

onMounted(() => {
  if (auth.canManageMaster) {
    fetchNotifikasi()
    // polling ringan setiap 30 detik supaya badge tetap update tanpa harus
    // reload halaman -- cukup jarang untuk tidak membebani server.
    notifTimer = setInterval(fetchNotifikasi, 30000)
  }
  if (auth.user?.id_pegawai) {
    refreshFotoProfil(auth.user.id_pegawai)
  }
})

onUnmounted(() => {
  if (notifTimer) clearInterval(notifTimer)
})

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
  } else if (auth.isAdminAbsensi || auth.isAdminVerifikasi) {
    // Akun (pegawai/atasan) yang dicentang "Admin Absensi" dan/atau "Admin
    // Verifikasi" tetap punya semua menu normalnya (Dashboard, Pengajuan
    // Cuti, Absen, Profil Saya, dst.) dan HANYA mendapat tambahan 1 menu ini
    // -- bukan akun terpisah. Kalau hanya Admin Verifikasi (tanpa Admin
    // Absensi), menu tetap dibuka tapi isinya cuma tab verifikasi Pengajuan
    // Surat Kolektif sekolah (lihat v-if per tab di RekapAbsensiView.vue).
    sections.push({ header: null, items: [{ label: 'Input Rekapan Absensi', icon: 'pi pi-camera', to: '/rekap-absen' }] })
  }

  if (auth.canManageMaster) {
    sections.push({
      header: 'Data Kepegawaian',
      items: [
        { label: 'Data Pegawai', icon: 'pi pi-users', to: '/master/pegawai' },
        { label: 'Perubahan Data Pegawai', icon: 'pi pi-user-edit', to: '/perubahan-data' },
        { label: 'Pengajuan Pensiun', icon: 'pi pi-briefcase', to: '/pengajuan-pensiun' },
        { label: 'Jatah Cuti Tahunan', icon: 'pi pi-wallet', to: '/master/jatah-cuti' },
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
              { label: 'Kecamatan', icon: 'pi pi-map', to: '/master/kecamatan' },
              { label: 'Unit Kerja', icon: 'pi pi-sitemap', to: '/master/unit-kerja' },
              { label: 'Status Pegawai', icon: 'pi pi-tag', to: '/master/status' },
              { label: 'Pangkat', icon: 'pi pi-star', to: '/master/pangkat' },
              { label: 'Golongan', icon: 'pi pi-hashtag', to: '/master/golongan' },
              { label: 'Pangkat / Golongan', icon: 'pi pi-th-large', to: '/master/pangkat-gol' },
              { label: 'Jenis Cuti', icon: 'pi pi-book', to: '/master/jenis-cuti' },
              { label: 'Jenis Surat', icon: 'pi pi-file-edit', to: '/master/jenis-surat' },
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
    command: () => keluar(),
  },
]

function keluar() {
  auth.logout()
  clearFotoProfil()
  router.push({ name: 'login' })
}

function isActive(to) {
  return route.path === to
}

function navigate(to) {
  moreSheet.value = false
  router.push(to)
}

// ============================================================
// navigasi bawah untuk tampilan HP (mirip aplikasi Android)
// ============================================================
// Di layar kecil sidebar TIDAK dipakai sama sekali -- diganti bar ikon di
// bawah layar berisi maksimal 4 menu utama ditambah tombol "Lainnya" yang
// membuka lembar berisi seluruh menu (dikelompokkan sama seperti sidebar).

const moreSheet = ref(false)

// daftar rata semua menu sesuai role, urutan sama dengan sidebar
const flatNavItems = computed(() => navSections.value.flatMap((s) => s.items))

const MAX_BOTTOM_ITEMS = 4

const bottomNavItems = computed(() => {
  const items = flatNavItems.value
  // kalau menunya pas (<= 5), semuanya ditampilkan tanpa tombol "Lainnya"
  return items.length <= MAX_BOTTOM_ITEMS + 1 ? items : items.slice(0, MAX_BOTTOM_ITEMS)
})

const punyaMenuLainnya = computed(() => flatNavItems.value.length > MAX_BOTTOM_ITEMS + 1)

// menu aktif yang tidak muat di bar bawah ditandai lewat tombol "Lainnya"
const menuLainnyaAktif = computed(
  () => punyaMenuLainnya.value && !bottomNavItems.value.some((item) => isActive(item.to)),
)
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="sidebar-brand">
        <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="sidebar-logo" />
        <div class="sidebar-brand-text">
          <span class="sidebar-brand-title">SIMADU</span>
          <span class="sidebar-brand-subtitle">Sistem Informasi Manajemen Administrasi Dinas Utama</span>
        </div>
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

    <div class="main-area">
      <header class="topbar">
        <div class="topbar-brand">
          <img src="/logo-morowali-utara.png" alt="Logo Kabupaten Morowali Utara" class="topbar-logo" />
          <div class="topbar-brand-text">
            <span class="topbar-brand-title">SIMADU</span>
            <span class="topbar-brand-subtitle">Sistem Informasi Manajemen Administrasi Dinas Utama</span>
          </div>
        </div>
        <div class="topbar-title">{{ roleLabel }}</div>

        <div class="topbar-right">
          <!-- Lonceng notifikasi: khusus administrator & admin (Admin
               Kepegawaian) -- menampilkan jumlah Pengajuan Cuti + Perubahan
               Data Pegawai yang baru masuk & belum diproses. -->
          <button v-if="auth.canManageMaster" type="button" class="notif-bell" @click="toggleNotifPanel">
            <i class="pi pi-bell"></i>
            <Badge v-if="notifData.total > 0" :value="notifData.total > 99 ? '99+' : notifData.total" severity="danger" class="notif-badge" />
          </button>
          <Popover v-if="auth.canManageMaster" ref="notifPanel" class="notif-popover">
            <div class="notif-header">
              <span>Notifikasi</span>
              <span class="notif-header-count">{{ notifData.total }} menunggu diproses</span>
            </div>
            <div v-if="notifLoading && !notifData.items.length" class="notif-empty">Memuat...</div>
            <div v-else-if="!notifData.items.length" class="notif-empty">Tidak ada pengajuan baru.</div>
            <div v-else class="notif-list">
              <a v-for="item in notifData.items" :key="item.type + '-' + item.id" class="notif-item" @click="bukaNotifikasi(item)">
                <i :class="notifIcon(item.type)"></i>
                <div class="notif-item-body">
                  <div class="notif-item-title">
                    {{ notifTitle(item.type) }}
                  </div>
                  <div class="notif-item-desc">{{ item.nama }} — {{ item.keterangan }}</div>
                  <div class="notif-item-time">{{ waktuRelatif(item.created_at) }}</div>
                </div>
              </a>
            </div>
          </Popover>

          <div class="topbar-user" @click="userMenu.toggle($event)">
            <Avatar v-if="fotoProfilUrl" :image="fotoProfilUrl" shape="circle" />
            <Avatar v-else :label="(auth.user?.nama || '?').charAt(0)" shape="circle" style="background: #0d9488; color: #fff" />
            <span class="user-name">{{ auth.user?.nama }}</span>
            <i class="pi pi-angle-down"></i>
          </div>
          <Menu ref="userMenu" :model="userMenuItems" :popup="true" />
        </div>
      </header>

      <main class="content-area">
        <router-view />
      </main>
    </div>

    <!-- ================= navigasi bawah (khusus HP) ================= -->
    <nav class="bottom-nav">
      <a
        v-for="item in bottomNavItems"
        :key="item.to"
        class="bottom-nav-item"
        :class="{ active: isActive(item.to) }"
        @click="navigate(item.to)"
      >
        <i :class="item.icon"></i>
        <span>{{ item.label }}</span>
      </a>
      <a
        v-if="punyaMenuLainnya"
        class="bottom-nav-item"
        :class="{ active: menuLainnyaAktif }"
        @click="moreSheet = true"
      >
        <i class="pi pi-th-large"></i>
        <span>Lainnya</span>
      </a>
    </nav>

    <!-- lembar "Lainnya": seluruh menu, muncul dari bawah layar -->
    <div v-if="moreSheet" class="sheet-backdrop" @click="moreSheet = false"></div>
    <div class="more-sheet" :class="{ open: moreSheet }">
      <div class="more-sheet-handle" @click="moreSheet = false"></div>
      <div class="more-sheet-title">Menu</div>
      <div class="more-sheet-body">
        <template v-for="(section, si) in navSections" :key="si">
          <div v-if="section.header" class="more-section-header">{{ section.header }}</div>
          <a
            v-for="item in section.items"
            :key="item.to"
            class="more-item"
            :class="{ active: isActive(item.to) }"
            @click="navigate(item.to)"
          >
            <i :class="item.icon"></i>
            <span>{{ item.label }}</span>
          </a>
        </template>
        <a class="more-item keluar" @click="keluar()">
          <i class="pi pi-sign-out"></i>
          <span>Keluar</span>
        </a>
      </div>
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
  background: #0f2436;
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
  color: #fff;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.sidebar-logo {
  height: 2.1rem;
  width: auto;
  max-width: 2.6rem;
  flex-shrink: 0;
  object-fit: contain;
}

.sidebar-brand-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.sidebar-brand-title {
  font-weight: 700;
  font-size: 1.02rem;
  line-height: 1.2;
}

.sidebar-brand-subtitle {
  font-size: 0.62rem;
  font-weight: 400;
  color: #94a3b8;
  line-height: 1.25;
  margin-top: 0.15rem;
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
  background: #0d9488;
  color: #fff;
}

.main-area {
  flex: 1;
  margin-left: 260px;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar {
  min-height: 60px;
  background: #fff;
  border-bottom: 1px solid #e5e7eb;
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.45rem 1.25rem;
  position: sticky;
  top: 0;
  z-index: 20;
}

/* brand di topbar hanya muncul di HP (di desktop sudah ada di sidebar) */
.topbar-brand {
  display: none;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.topbar-logo {
  height: 1.8rem;
  width: auto;
  max-width: 2.2rem;
  flex-shrink: 0;
  object-fit: contain;
}

.topbar-brand-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar-brand-title {
  font-weight: 700;
  color: #111827;
  font-size: 0.98rem;
  line-height: 1.15;
}

.topbar-brand-subtitle {
  font-size: 0.56rem;
  font-weight: 400;
  color: #64748b;
  line-height: 1.2;
  margin-top: 0.1rem;
}

.topbar-title {
  font-weight: 600;
  color: #374151;
}

/* grup kanan topbar (lonceng notifikasi + profil/logout) -- margin-left:
   auto di sini (bukan di elemen masing-masing) supaya grup ini SELALU
   menempel di ujung kanan topbar, baik ada lonceng notifikasi (admin/
   administrator) maupun tidak (pegawai/atasan), dan konsisten di semua
   ukuran layar (HP, tablet, laptop). */
.topbar-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.topbar-user {
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

/* ============================================================
   Lonceng notifikasi (administrator & admin) di topbar
   ============================================================ */
.notif-bell {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.2rem;
  height: 2.2rem;
  border: none;
  background: transparent;
  border-radius: 999px;
  cursor: pointer;
  color: #4b5563;
  font-size: 1.05rem;
}

.notif-bell:hover {
  background: #f3f4f6;
}

.notif-bell :deep(.notif-badge) {
  position: absolute;
  top: 0.05rem;
  right: 0.05rem;
}

.notif-popover {
  width: min(360px, calc(100vw - 2rem));
}

.notif-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.75rem;
  padding: 0.15rem 0.1rem 0.6rem;
  border-bottom: 1px solid #f1f5f9;
  margin-bottom: 0.4rem;
  font-weight: 700;
  color: #111827;
}

.notif-header-count {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--p-text-muted-color);
  white-space: nowrap;
}

.notif-empty {
  padding: 1.25rem 0.5rem;
  text-align: center;
  font-size: 0.85rem;
  color: var(--p-text-muted-color);
}

.notif-list {
  max-height: 340px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.notif-item {
  display: flex;
  align-items: flex-start;
  gap: 0.6rem;
  padding: 0.55rem 0.5rem;
  border-radius: 8px;
  cursor: pointer;
}

.notif-item:hover {
  background: #f3f4f6;
}

.notif-item i {
  margin-top: 0.15rem;
  color: #0d9488;
  font-size: 0.95rem;
}

.notif-item-title {
  font-size: 0.84rem;
  font-weight: 600;
  color: #111827;
}

.notif-item-desc {
  font-size: 0.8rem;
  color: #4b5563;
  margin-top: 0.1rem;
}

.notif-item-time {
  font-size: 0.72rem;
  color: var(--p-text-muted-color);
  margin-top: 0.2rem;
}

.content-area {
  flex: 1;
}

/* ============================================================
   Navigasi bawah ala aplikasi HP (Android) -- disembunyikan di desktop,
   menggantikan sidebar sepenuhnya di layar kecil.
   ============================================================ */
.bottom-nav {
  display: none;
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 45;
  background: #fff;
  border-top: 1px solid #e5e7eb;
  box-shadow: 0 -2px 10px rgba(0, 0, 0, 0.06);
  /* aman dari home indicator iPhone / gesture bar Android */
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.bottom-nav-item {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.18rem;
  padding: 0.5rem 0.2rem 0.45rem;
  color: #6b7280;
  cursor: pointer;
  font-size: 0.68rem;
  line-height: 1.1;
  text-align: center;
  -webkit-tap-highlight-color: transparent;
}

.bottom-nav-item i {
  font-size: 1.15rem;
}

.bottom-nav-item span {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bottom-nav-item.active {
  color: #0d9488;
  font-weight: 600;
}

/* lembar "Lainnya" yang muncul dari bawah layar */
.sheet-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  z-index: 46;
}

.more-sheet {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 47;
  background: #fff;
  border-radius: 16px 16px 0 0;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.15);
  transform: translateY(100%);
  transition: transform 0.22s ease;
  max-height: 78vh;
  display: none;
  flex-direction: column;
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.more-sheet.open {
  transform: translateY(0);
}

.more-sheet-handle {
  width: 42px;
  height: 4px;
  border-radius: 999px;
  background: #d1d5db;
  margin: 0.6rem auto 0.2rem;
  cursor: pointer;
}

.more-sheet-title {
  font-weight: 700;
  padding: 0.35rem 1rem 0.5rem;
  color: #111827;
}

.more-sheet-body {
  overflow-y: auto;
  padding: 0 0.6rem 0.9rem;
}

.more-section-header {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: #9ca3af;
  padding: 0.85rem 0.6rem 0.3rem;
  font-weight: 700;
}

.more-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 0.7rem;
  border-radius: 10px;
  color: #374151;
  font-size: 0.92rem;
  cursor: pointer;
}

.more-item i {
  width: 1.3rem;
  color: #6b7280;
}

.more-item.active {
  background: #f0fdfa;
  color: #0f766e;
  font-weight: 600;
}

.more-item.active i {
  color: #0f766e;
}

.more-item.keluar {
  margin-top: 0.6rem;
  border-top: 1px solid #f1f5f9;
  color: #dc2626;
}

.more-item.keluar i {
  color: #dc2626;
}

@media (max-width: 900px) {
  /* Di HP panel/sidebar TIDAK ditampilkan sama sekali -- navigasi
     sepenuhnya lewat bar ikon di bawah layar. */
  .sidebar {
    display: none !important;
  }
  .main-area {
    margin-left: 0;
  }
  .topbar-brand {
    display: flex;
  }
  .topbar-title,
  .user-name {
    display: none;
  }
  .bottom-nav {
    display: flex;
  }
  .more-sheet {
    display: flex;
  }
  /* ruang supaya konten paling bawah tidak tertutup bar navigasi */
  .content-area {
    padding-bottom: 4.75rem;
  }
}
</style>
