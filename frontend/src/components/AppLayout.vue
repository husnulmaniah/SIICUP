<script setup>
import { ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const route = useRoute();
const menuTerbuka = ref(false);

// Tutup menu otomatis saat berpindah halaman (mode HP)
watch(() => route.fullPath, () => { menuTerbuka.value = false; });
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">
        <div>
          <h1>SICUTI</h1>
          <small>Sistem Informasi Cuti Pegawai<br />Dinas Pendidikan &amp; Kebudayaan<br />Kab. Morowali Utara</small>
        </div>
        <button class="tombol-menu" @click="menuTerbuka = !menuTerbuka" aria-label="Buka menu">
          {{ menuTerbuka ? '✕' : '☰' }}
        </button>
      </div>
      <nav :class="{ terbuka: menuTerbuka }">
        <router-link to="/">🏠 Beranda</router-link>
        <router-link v-if="auth.isAdmin" to="/pegawai">👥 Data Pegawai Aktif</router-link>
        <router-link to="/cuti">📋 Pengajuan Cuti</router-link>
        <router-link to="/cuti/ajukan">✍️ Ajukan Cuti</router-link>
        <router-link to="/perubahan-data">🔄 Perubahan Data</router-link>
        <router-link v-if="auth.isAdmin" to="/laporan">📊 Laporan</router-link>
        <router-link v-if="auth.isAdmin" to="/hari-libur">📅 Hari Libur</router-link>
        <router-link v-if="auth.isAdminUtama" to="/akun">🔑 Kelola Akun</router-link>
        <router-link to="/profil">👤 Profil Saya</router-link>
      </nav>
      <div class="user-box" :class="{ terbuka: menuTerbuka }">
        <b>{{ auth.user?.nama }}</b>
        <span class="role">{{ auth.roleLabel }}</span>
        <button class="btn-logout" @click="auth.logout()">Keluar</button>
      </div>
    </aside>
    <main class="main"><slot /></main>
  </div>
</template>
