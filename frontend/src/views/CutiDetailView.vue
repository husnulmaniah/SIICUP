<script setup>
import { ref, computed, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Swal from 'sweetalert2';
import api, { apiUrl, pesanError, swalTema } from '../api';
import { useAuthStore } from '../stores/auth';
import StatusBadge from '../components/StatusBadge.vue';
import SuratCutiModal from '../components/SuratCutiModal.vue';
import { JENIS_CUTI, labelJenis, formatTanggal, formatWaktu } from '../cuti-config';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const cuti = ref(null);
const memproses = ref(false);
const showSurat = ref(false);
const fileTtd = ref(null);
const mengunggahTtd = ref(false);

// mode perbaikan (status DIKEMBALIKAN)
const modePerbaikan = ref(false);
const perbaikan = ref({});
const filesBaru = ref({});
const hapusBerkas = ref([]);

const config = computed(() => (cuti.value ? JENIS_CUTI[cuti.value.jenisCuti] : null));
const bolehPerbaiki = computed(
  () => cuti.value?.status === 'DIKEMBALIKAN' && (auth.isAdmin || cuti.value.pegawaiId === auth.user?.pegawaiId)
);
const berkasKelengkapan = computed(() => (cuti.value?.berkas || []).filter((b) => b.jenisBerkas !== 'BERKAS_TTD'));
const berkasTtd = computed(() => (cuti.value?.berkas || []).filter((b) => b.jenisBerkas === 'BERKAS_TTD'));

async function unggahTtd() {
  const files = fileTtd.value?.files;
  if (!files?.length) {
    return Swal.fire({ icon: 'info', title: 'Pilih file dulu', text: 'Pilih hasil scan berkas yang telah ditandatangani Kepala Dinas (PDF/JPG/PNG).', ...swalTema });
  }
  mengunggahTtd.value = true;
  const fd = new FormData();
  for (const f of files) fd.append('file', f);
  try {
    const { data } = await api.post(`/cuti/${cuti.value.id}/berkas-ttd`, fd);
    fileTtd.value.value = '';
    await muat();
    Swal.fire({ icon: 'success', title: 'Berkas tersimpan', text: data.message, ...swalTema });
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal mengunggah', text: pesanError(e), ...swalTema });
  } finally {
    mengunggahTtd.value = false;
  }
}

async function muat() {
  const { data } = await api.get(`/cuti/${route.params.id}`);
  cuti.value = data;
  perbaikan.value = {
    tanggalMulai: data.tanggalMulai.slice(0, 10),
    tanggalSelesai: data.tanggalSelesai.slice(0, 10),
    alasan: data.alasan,
    alamatSelamaCuti: data.alamatSelamaCuti || '',
  };
}
onMounted(muat);

const urlBerkas = (b) => apiUrl(`/cuti/${cuti.value.id}/berkas/${b.id}?token=${localStorage.getItem('sicuti_token')}`);

async function aksi(jenis) {
  const judul = { SETUJUI: 'Setujui pengajuan ini?', KEMBALIKAN: 'Kembalikan untuk diperbaiki?', TOLAK: 'Tolak pengajuan ini?' }[jenis];
  const { value: catatan, isConfirmed } = await Swal.fire({
    title: judul,
    input: 'textarea',
    inputLabel: jenis === 'SETUJUI' ? 'Catatan (opsional)' : 'Catatan untuk pegawai (wajib)',
    inputPlaceholder: jenis === 'KEMBALIKAN' ? 'Contoh: SK terakhir belum terbaca, unggah ulang.' : '',
    showCancelButton: true, confirmButtonText: 'Ya, proses', cancelButtonText: 'Batal', ...swalTema,
    inputValidator: (v) => (jenis !== 'SETUJUI' && !v?.trim() ? 'Catatan wajib diisi.' : undefined),
  });
  if (!isConfirmed) return;
  memproses.value = true;
  try {
    await api.post(`/cuti/${cuti.value.id}/aksi`, { aksi: jenis, catatan });
    await muat();
    Swal.fire({ icon: 'success', title: 'Pengajuan diproses', ...swalTema });
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal memproses', text: pesanError(e), ...swalTema });
  } finally {
    memproses.value = false;
  }
}

function pilihFileBaru(kode, e) { filesBaru.value = { ...filesBaru.value, [kode]: e.target.files }; }
function toggleHapus(id) {
  hapusBerkas.value = hapusBerkas.value.includes(id)
    ? hapusBerkas.value.filter((x) => x !== id)
    : [...hapusBerkas.value, id];
}

async function kirimPerbaikan() {
  memproses.value = true;
  const fd = new FormData();
  for (const [k, v] of Object.entries(perbaikan.value)) fd.append(k, v);
  fd.append('hapusBerkas', hapusBerkas.value.join(','));
  for (const [kode, list] of Object.entries(filesBaru.value)) {
    for (const f of list) fd.append(`berkas_${kode}`, f);
  }
  try {
    await api.put(`/cuti/${cuti.value.id}`, fd);
    modePerbaikan.value = false;
    filesBaru.value = {};
    hapusBerkas.value = [];
    await muat();
    Swal.fire({ icon: 'success', title: 'Pengajuan diperbaiki', text: 'Status kembali menjadi DIAJUKAN.', ...swalTema });
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal memperbaiki', text: pesanError(e), ...swalTema });
  } finally {
    memproses.value = false;
  }
}
</script>

<template>
  <div v-if="cuti">
    <div class="page-head">
      <div>
        <h2>Detail Pengajuan Cuti #{{ cuti.id }}</h2>
        <p>{{ labelJenis(cuti.jenisCuti) }} — <StatusBadge :status="cuti.status" /></p>
      </div>
      <button class="btn btn-putih" @click="router.back()">← Kembali</button>
    </div>

    <div style="display:grid; grid-template-columns: 1.6fr 1fr; gap:18px; align-items:start">
      <div>
        <div class="card">
          <h3>Data Pengajuan</h3>
          <table class="tabel-detail">
            <tr><th>Nama Pegawai</th><td>{{ cuti.pegawai.nama }}</td></tr>
            <tr><th>NIP</th><td>{{ cuti.pegawai.nip }}</td></tr>
            <tr><th>Jabatan / Tempat Tugas</th><td>{{ cuti.pegawai.jabatan || '-' }} — {{ cuti.pegawai.tempatTugas || '-' }}</td></tr>
            <tr><th>Jenis Cuti</th><td>{{ labelJenis(cuti.jenisCuti) }}</td></tr>
            <tr><th>Periode</th><td>{{ formatTanggal(cuti.tanggalMulai) }} s.d. {{ formatTanggal(cuti.tanggalSelesai) }} ({{ cuti.lamaCuti }} hari kerja)</td></tr>
            <tr><th>Pola Hari Kerja</th><td>{{ { '5_HARI': '5 Hari Kerja', '6_HARI': '6 Hari Kerja', 'KALENDER': 'Hari Kalender' }[cuti.polaKerja] || cuti.polaKerja }}</td></tr>
            <tr><th>Alasan</th><td>{{ cuti.alasan }}</td></tr>
            <tr><th>Alamat Selama Cuti</th><td>{{ cuti.alamatSelamaCuti || '-' }}</td></tr>
            <tr><th>No. HP</th><td>{{ cuti.noHp || '-' }}</td></tr>
            <tr v-if="cuti.catatan"><th>Catatan Admin</th><td style="color:var(--terakota)"><b>{{ cuti.catatan }}</b></td></tr>
            <tr v-if="cuti.diprosesOleh"><th>Diproses Oleh</th><td>{{ cuti.diprosesOleh }}</td></tr>
          </table>
        </div>

        <div class="card">
          <h3>Kelengkapan Berkas ({{ berkasKelengkapan.length }})</h3>
          <div v-for="b in berkasKelengkapan" :key="b.id" class="berkas-item terisi" style="display:flex; justify-content:space-between; align-items:center; gap:10px; flex-wrap:wrap">
            <div>
              <div class="judul">{{ b.namaBerkas }}</div>
              <div class="hint">{{ b.namaFile }} · {{ (b.ukuran / 1024).toFixed(0) }} KB</div>
            </div>
            <div style="display:flex; gap:8px">
              <a :href="urlBerkas(b)" target="_blank" class="btn btn-putih btn-kecil">Lihat / Unduh</a>
              <button v-if="modePerbaikan" class="btn btn-kecil" :class="hapusBerkas.includes(b.id) ? 'btn-merah' : 'btn-kuning'" @click="toggleHapus(b.id)">
                {{ hapusBerkas.includes(b.id) ? 'Batal hapus' : 'Tandai hapus' }}
              </button>
            </div>
          </div>
          <p v-if="!berkasKelengkapan.length" class="hint">Belum ada berkas terunggah.</p>
        </div>

        <!-- Berkas ditandatangani Kepala Dinas -->
        <div v-if="cuti.status === 'DISETUJUI'" class="card" style="border-left:5px solid var(--hijau)">
          <h3>Berkas Ditandatangani Kepala Dinas</h3>
          <div v-for="b in berkasTtd" :key="b.id" class="berkas-item terisi" style="display:flex; justify-content:space-between; align-items:center; gap:10px; flex-wrap:wrap">
            <div>
              <div class="judul">🖋 {{ b.namaFile }}</div>
              <div class="hint">{{ (b.ukuran / 1024).toFixed(0) }} KB · diunggah {{ formatWaktu(b.createdAt) }}</div>
            </div>
            <a :href="urlBerkas(b)" target="_blank" class="btn btn-putih btn-kecil">Lihat / Unduh</a>
          </div>
          <p v-if="!berkasTtd.length" class="hint">Belum ada berkas bertanda tangan yang diunggah.</p>
          <div v-if="auth.isAdmin" style="display:flex; gap:10px; align-items:center; flex-wrap:wrap; margin-top:10px; border:1.5px dashed var(--garis); padding:8px 12px; border-radius:9px; background:#fffdf9">
            <input type="file" ref="fileTtd" accept=".pdf,.jpg,.jpeg,.png,.webp" multiple style="font-size:13px" />
            <button class="btn btn-hijau btn-kecil" :disabled="mengunggahTtd" @click="unggahTtd">
              {{ mengunggahTtd ? 'Mengunggah…' : '⬆️ Unggah Berkas Ber-TTD' }}
            </button>
          </div>
        </div>

        <!-- Form perbaikan -->
        <div v-if="modePerbaikan" class="card" style="border-left:5px solid var(--terakota)">
          <h3>Perbaiki Pengajuan</h3>
          <div class="form-grid">
            <div><label class="form-label">Tanggal Mulai</label><input v-model="perbaikan.tanggalMulai" type="date" class="form-input" /></div>
            <div><label class="form-label">Tanggal Selesai</label><input v-model="perbaikan.tanggalSelesai" type="date" class="form-input" /></div>
          </div>
          <label class="form-label">Alasan</label>
          <textarea v-model="perbaikan.alasan" class="form-input" rows="2"></textarea>
          <label class="form-label">Alamat Selama Cuti</label>
          <input v-model="perbaikan.alamatSelamaCuti" class="form-input" />
          <h3 style="margin-top:16px; font-size:16px">Unggah Berkas Baru (opsional)</h3>
          <div v-for="b in config.berkas" :key="b.kode" class="berkas-item" :class="{ terisi: filesBaru[b.kode]?.length }">
            <div class="judul">{{ b.label }}</div>
            <input type="file" accept=".pdf,.jpg,.jpeg,.png,.webp" :multiple="!!b.multiple" @change="pilihFileBaru(b.kode, $event)" />
          </div>
          <div style="display:flex; gap:10px; justify-content:flex-end; margin-top:12px">
            <button class="btn btn-putih" @click="modePerbaikan = false">Batal</button>
            <button class="btn btn-hijau" :disabled="memproses" @click="kirimPerbaikan">Kirim Perbaikan</button>
          </div>
        </div>
      </div>

      <div>
        <div class="card" v-if="cuti.status === 'DISETUJUI'" style="border-left:5px solid var(--terakota)">
          <h3>Surat Cuti</h3>
          <p class="hint" style="margin-bottom:12px">
            {{ auth.isAdmin
              ? 'Buat Formulir Permintaan & Pemberian Cuti (PDF) dan Surat Rekomendasi (Word) — pratinjau dulu sebelum diunduh.'
              : 'Surat cuti dibuat oleh admin. Hasil yang telah ditandatangani Kepala Dinas dapat Anda lihat pada panel Berkas Ditandatangani.' }}
          </p>
          <button v-if="auth.isAdmin" class="btn btn-terakota" style="width:100%; justify-content:center" @click="showSurat = true">👁 Preview &amp; Unduh Surat</button>
          <button v-else class="btn btn-terakota" style="width:100%; justify-content:center; opacity:.55; cursor:not-allowed" disabled title="Hanya Admin Utama dan Admin Pembantu yang dapat membuat surat">🔒 Preview &amp; Unduh Surat (khusus admin)</button>
        </div>

        <div class="card" v-if="auth.isAdmin && cuti.status === 'DIAJUKAN'">
          <h3>Proses Pengajuan</h3>
          <p class="hint" style="margin-bottom:12px">Periksa data dan berkas, lalu ambil keputusan.</p>
          <div style="display:flex; flex-direction:column; gap:9px">
            <button class="btn btn-hijau" :disabled="memproses" @click="aksi('SETUJUI')">✔ Setujui</button>
            <button class="btn btn-kuning" :disabled="memproses" @click="aksi('KEMBALIKAN')">↩ Kembalikan untuk Perbaikan</button>
            <button class="btn btn-merah" :disabled="memproses" @click="aksi('TOLAK')">✖ Tolak</button>
          </div>
        </div>

        <div class="card" v-if="bolehPerbaiki && !modePerbaikan">
          <h3>Pengajuan Dikembalikan</h3>
          <p class="hint" style="margin-bottom:10px">Perbaiki sesuai catatan admin, lalu ajukan ulang.</p>
          <button class="btn btn-terakota" style="width:100%; justify-content:center" @click="modePerbaikan = true">✏️ Perbaiki Pengajuan</button>
        </div>

        <div class="card">
          <h3>Riwayat Proses</h3>
          <ul class="timeline">
            <li v-for="r in cuti.riwayat" :key="r.id">
              <b>{{ r.aksi }}</b> oleh {{ r.oleh }}
              <div v-if="r.catatan" class="hint">"{{ r.catatan }}"</div>
              <div class="waktu">{{ formatWaktu(r.createdAt) }}</div>
            </li>
          </ul>
        </div>
      </div>
    </div>

    <SuratCutiModal v-if="showSurat && auth.isAdmin" :cuti-id="cuti.id" @tutup="showSurat = false" />
  </div>
  <p v-else style="color:var(--cokelat-lembut)">Memuat detail pengajuan…</p>
</template>
