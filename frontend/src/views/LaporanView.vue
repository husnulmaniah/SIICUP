<script setup>
import { ref, computed } from 'vue';
import Swal from 'sweetalert2';
import api, { pesanError, swalTema } from '../api';
import { JENIS_CUTI, labelJenis, formatTanggal } from '../cuti-config';

const dari = ref('');
const sampai = ref('');
const status = ref('SEMUA');
const jenis = ref('SEMUA');
const hasil = ref(null); // { data, ringkasan }
const memuat = ref(false);
const sudahCari = ref(false);

const STATUS_LIST = ['SEMUA', 'DIAJUKAN', 'DISETUJUI', 'DIKEMBALIKAN', 'DITOLAK'];

const params = computed(() => ({
  dari: dari.value || undefined,
  sampai: sampai.value || undefined,
  status: status.value,
  jenis: jenis.value,
}));

async function tampilkan() {
  memuat.value = true;
  try {
    const { data } = await api.get('/laporan', { params: params.value });
    hasil.value = data;
    sudahCari.value = true;
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal memuat laporan', text: pesanError(e), ...swalTema });
  } finally {
    memuat.value = false;
  }
}

async function eksporExcel() {
  try {
    const res = await api.get('/laporan/export', { params: params.value, responseType: 'blob' });
    const href = URL.createObjectURL(res.data);
    const a = Object.assign(document.createElement('a'), {
      href,
      download: `LAPORAN_REKAP_CUTI_${new Date().toISOString().slice(0, 10)}.xlsx`,
    });
    a.click();
    URL.revokeObjectURL(href);
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal export', text: pesanError(e), ...swalTema });
  }
}

const dataTabel = computed(() =>
  (hasil.value?.data || []).map((c, i) => [
    i + 1,
    `${c.pegawai.nama}|${c.pegawai.nip}`,
    c.pegawai.jabatan || '-',
    c.pegawai.tempatTugas || c.pegawai.unor || '-',
    labelJenis(c.jenisCuti),
    `${formatTanggal(c.tanggalMulai)} – ${formatTanggal(c.tanggalSelesai)}`,
    c.lamaCuti,
    c.status,
    c.diprosesOleh || '-',
  ])
);

const kolomDT = [
  { title: 'No' },
  { title: 'Pegawai', render: (d) => { const [n, nip] = d.split('|'); return `<b>${n}</b><br><small style="color:var(--cokelat-lembut)">${nip}</small>`; } },
  { title: 'Jabatan' },
  { title: 'Unit Kerja' },
  { title: 'Jenis Cuti' },
  { title: 'Periode' },
  { title: 'Lama (hari)' },
  { title: 'Status', render: (d) => `<span class="badge badge-${d}">${d}</span>` },
  { title: 'Diproses Oleh' },
];

const dtOptions = {
  pageLength: 25, order: [],
  language: {
    search: 'Cari:', lengthMenu: 'Tampilkan _MENU_ data', info: 'Menampilkan _START_–_END_ dari _TOTAL_ pengajuan',
    infoEmpty: 'Tidak ada data pada filter ini', zeroRecords: 'Tidak ditemukan', infoFiltered: '(disaring dari _MAX_ total)',
    paginate: { first: '«', last: '»', next: '›', previous: '‹' },
  },
};
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Laporan Rekap Cuti</h2>
        <p>Susun laporan cuti untuk atasan: saring berdasarkan periode, status, dan jenis cuti, lalu export ke Excel.</p>
      </div>
    </div>

    <div class="card">
      <form @submit.prevent="tampilkan" style="display:flex; gap:14px; align-items:flex-end; flex-wrap:wrap">
        <div>
          <label class="form-label">Dari Tanggal</label>
          <input v-model="dari" type="date" class="form-input" />
        </div>
        <div>
          <label class="form-label">Sampai Tanggal</label>
          <input v-model="sampai" type="date" class="form-input" />
        </div>
        <div>
          <label class="form-label">Status</label>
          <select v-model="status" class="form-input">
            <option v-for="st in STATUS_LIST" :key="st" :value="st">{{ st === 'SEMUA' ? 'Semua Status' : st }}</option>
          </select>
        </div>
        <div>
          <label class="form-label">Jenis Cuti</label>
          <select v-model="jenis" class="form-input">
            <option value="SEMUA">Semua Jenis</option>
            <option v-for="(v, k) in JENIS_CUTI" :key="k" :value="k">{{ v.label }}</option>
          </select>
        </div>
        <button class="btn btn-hijau" :disabled="memuat">{{ memuat ? 'Memuat…' : '🔎 Tampilkan' }}</button>
        <button type="button" class="btn btn-terakota" :disabled="!sudahCari" @click="eksporExcel">⬇️ Export Excel</button>
      </form>
    </div>

    <template v-if="hasil">
      <div class="stat-grid">
        <div class="stat"><div class="angka">{{ hasil.ringkasan.total }}</div><div class="label">Total Pengajuan</div></div>
        <div class="stat aksen"><div class="angka">{{ hasil.ringkasan.totalHari }}</div><div class="label">Total Hari Cuti</div></div>
        <div class="stat kuning"><div class="angka">{{ hasil.ringkasan.perStatus.DIAJUKAN || 0 }}</div><div class="label">Menunggu Proses</div></div>
        <div class="stat"><div class="angka">{{ hasil.ringkasan.perStatus.DISETUJUI || 0 }}</div><div class="label">Disetujui</div></div>
        <div class="stat merah"><div class="angka">{{ hasil.ringkasan.perStatus.DITOLAK || 0 }}</div><div class="label">Ditolak</div></div>
      </div>

      <div class="card">
        <h3 style="margin-bottom:10px">Rincian per Jenis Cuti</h3>
        <p style="margin:0 0 14px; color:var(--cokelat-lembut)">
          <template v-for="(jml, k, idx) in hasil.ringkasan.perJenis" :key="k">
            <b style="color:var(--cokelat)">{{ labelJenis(k) }}: {{ jml }}</b><span v-if="idx < Object.keys(hasil.ringkasan.perJenis).length - 1"> · </span>
          </template>
          <template v-if="!Object.keys(hasil.ringkasan.perJenis).length">Tidak ada data.</template>
        </p>
        <DataTable :data="dataTabel" :columns="kolomDT" :options="dtOptions" class="dataTable" width="100%" />
      </div>
    </template>
    <div v-else class="card" style="color:var(--cokelat-lembut)">
      Pilih rentang tanggal (opsional) lalu klik <b>Tampilkan</b> untuk menyusun laporan.
    </div>
  </div>
</template>
