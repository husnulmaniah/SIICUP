<script setup>
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
</script>

<template>
  <Toast />
  <ConfirmDialog />
  <!-- Transisi fade+naik tipis setiap pindah halaman -- permintaan pengguna
       "tampilan lebih modern": tanpa ini perpindahan route terasa "patah"
       (konten lama hilang, konten baru muncul seketika tanpa transisi).
       durasinya SENGAJA pendek (180ms) supaya tidak terasa lambat/mengganggu
       navigasi, dan mode="out-in" supaya hanya SATU halaman yang terlihat
       pada satu waktu (tidak tumpang tindih). -->
  <router-view v-slot="{ Component }">
    <transition name="page-fade" mode="out-in">
      <component :is="Component" />
    </transition>
  </router-view>
</template>

<style>
.page-fade-enter-active,
.page-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}
.page-fade-enter-from {
  opacity: 0;
  transform: translateY(4px);
}
.page-fade-leave-to {
  opacity: 0;
}
</style>
