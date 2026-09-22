<script setup>
import Message from 'primevue/message'
import Kp4FormPanel from '../components/Kp4FormPanel.vue'
</script>

<template>
  <div class="page-wrap">
    <div class="page-title">KP4</div>
    <p class="page-subtitle">
      Surat Keterangan Untuk Mendapatkan Pembayaran Tunjangan Keluarga. Isi/lengkapi data di bawah ini kapan saja --
      tersimpan langsung tanpa perlu menunggu persetujuan. Formulir yang dicetak akan ditandatangani otomatis oleh
      Kepala Dinas/PLT.
    </p>
    <Message severity="error" :closable="false" style="margin-bottom: 1.25rem">
      <strong>Catatan:</strong> Isi KP4 berdasarkan tunjangan sesuai dengan gaji yang diterima saat ini. Jangan
      dikurangi dan jangan dilebihkan.
    </Message>
    <Kp4FormPanel />
  </div>
</template>
