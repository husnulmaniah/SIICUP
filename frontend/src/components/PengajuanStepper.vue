<template>
  <div class="pengajuan-stepper" :class="{ 'pengajuan-stepper--compact': compact }">
    <template v-for="(step, idx) in steps" :key="idx">
      <div class="step-connector" v-if="idx > 0" :class="connectorClass(idx)"></div>
      <div class="step-item" :class="`step-item--${step.state}`">
        <div class="step-circle">
          <i v-if="step.state === 'done'" class="pi pi-check"></i>
          <i v-else-if="step.state === 'rejected'" class="pi pi-times"></i>
          <i v-else-if="step.state === 'current'" class="pi pi-spin pi-spinner" style="font-size: 0.7rem"></i>
          <span v-else>{{ idx + 1 }}</span>
        </div>
        <div class="step-body">
          <div class="step-label">{{ step.label }}</div>
          <div v-if="step.actor" class="step-meta">{{ step.actor }}</div>
          <div v-if="step.timestamp" class="step-meta step-meta--time">{{ step.timestamp }}</div>
          <div v-if="step.note" class="step-note">"{{ step.note }}"</div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
// Komponen progress bar/stepper generik untuk SEMUA jenis pengajuan
// (Pengajuan Cuti, Kenaikan Pangkat, Perubahan Jabatan, Pensiun, Perubahan
// Data Pegawai, Berita Acara, dst) -- permintaan pengguna: setiap pengajuan
// harus punya progress bar yang jelas, bukan cuma satu Tag status + pesan
// terpisah seperti sebelumnya.
//
// Pemanggil (masing-masing *View.vue) bertanggung jawab menyusun array
// `steps` sesuai alur pipeline-nya sendiri (jumlah tahap beda-beda per
// jenis pengajuan -- lihat computed `kpStepperSteps`/`cutiStepperSteps`/dst
// di masing-masing view) -- komponen ini TIDAK tahu apa pun soal status
// string pengajuan tertentu, murni render visual dari data yang sudah
// diterjemahkan.
//
// step = { label, state: 'done'|'current'|'rejected'|'pending', actor?,
//           timestamp?, note? }
// - done: tahap ini sudah lewat/disetujui (lingkaran hijau, centang)
// - current: tahap ini SEDANG berjalan/ditunggu (lingkaran biru/kuning, ikon muter)
// - rejected: tahap ini ditolak/dikembalikan (lingkaran merah, silang) --
//   tahap SETELAHNYA otomatis ikut digambar abu-abu (pending) oleh pemanggil
// - pending: tahap ini belum sampai gilirannya (lingkaran abu-abu, nomor urut)
const props = defineProps({
  steps: { type: Array, required: true },
  compact: { type: Boolean, default: false },
})

function connectorClass(idx) {
  // garis penghubung sebelum step[idx] -- hijau kalau step SEBELUMNYA sudah
  // "done", selain itu abu-abu (termasuk kalau sebelumnya "rejected", karena
  // alur berhenti/kembali di situ, jadi step setelahnya belum benar-benar
  // "dilewati").
  return { 'step-connector--done': props.steps[idx - 1]?.state === 'done' }
}
</script>

<style scoped>
.pengajuan-stepper {
  display: flex;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 0;
  margin: 0.25rem 0 1rem 0;
}
.step-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  width: 8.5rem;
  flex-shrink: 0;
}
.pengajuan-stepper--compact .step-item {
  width: 7rem;
}
.step-circle {
  width: 2rem;
  height: 2rem;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.85rem;
  font-weight: 700;
  flex-shrink: 0;
  border: 2px solid var(--p-content-border-color, #e2e8f0);
  background: var(--p-content-background, #f8fafc);
  color: var(--p-text-muted-color, #64748b);
}
.step-item--done .step-circle {
  background: var(--p-green-500, #22c55e);
  border-color: var(--p-green-500, #22c55e);
  color: #fff;
}
.step-item--current .step-circle {
  background: var(--p-orange-100, #ffedd5);
  border-color: var(--p-orange-500, #f97316);
  color: var(--p-orange-600, #ea580c);
}
.step-item--rejected .step-circle {
  background: var(--p-red-500, #ef4444);
  border-color: var(--p-red-500, #ef4444);
  color: #fff;
}
.step-body {
  margin-top: 0.4rem;
}
.step-label {
  font-size: 0.78rem;
  font-weight: 600;
  line-height: 1.2;
}
.step-item--pending .step-label {
  color: var(--p-text-muted-color, #94a3b8);
  font-weight: 500;
}
.step-item--rejected .step-label {
  color: var(--p-red-600, #dc2626);
}
.step-meta {
  font-size: 0.7rem;
  color: var(--p-text-muted-color, #94a3b8);
  margin-top: 0.1rem;
  line-height: 1.25;
}
.step-note {
  font-size: 0.7rem;
  color: var(--p-text-muted-color, #94a3b8);
  font-style: italic;
  margin-top: 0.15rem;
  line-height: 1.25;
  word-break: break-word;
}
.step-connector {
  flex: 1;
  min-width: 1.25rem;
  height: 2px;
  background: var(--p-content-border-color, #e2e8f0);
  margin-top: 1rem;
  align-self: flex-start;
}
.step-connector--done {
  background: var(--p-green-500, #22c55e);
}

@media (max-width: 560px) {
  .pengajuan-stepper {
    flex-direction: column;
    align-items: flex-start;
  }
  .step-item {
    flex-direction: row;
    width: 100%;
    text-align: left;
    gap: 0.6rem;
  }
  .step-body {
    margin-top: 0;
  }
  .step-connector {
    width: 2px;
    height: 1.25rem;
    margin: 0 0 0 0.9rem;
    align-self: stretch;
  }
}
</style>
