<script setup>
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import Swal from 'sweetalert2';
import api, { pesanError, swalTema } from '../api';
import { useAuthStore } from '../stores/auth';
import { JENIS_CUTI } from '../cuti-config';
import { hitungLamaCuti, POLA_KERJA } from '../hari-kerja';

const auth = useAuthStore();
const router = useRouter();

const jenisCuti = ref('TAHUNAN');
const pegawaiId = ref('');
const daftarPegawai = ref([]);
const tanggalMulai = ref('');
const tanggalSelesai = ref('');
const alasan = ref('');
const alamat = ref('');
const noHp = ref('');
const files = ref({}); // kode -> FileList
const mengirim = ref(false);
const skTersedia = ref(null); // dokumen SK_TERAKHIR pegawai terpilih (jika ada)
const pegawaiTerpilih = ref(null); // data pegawai utk aturan rekomendasi kepsek
const sisaTahunan = ref(null); // { totalSisa, rincian } utk jenis cuti tahunan
const polaKerja = ref('5_HARI');
const hariLibur = ref([]); // daftar 'YYYY-MM-DD'

const config = computed(() => JENIS_CUTI[jenisCuti.value]);
const totalHariKalender = computed(() => {
  if (!tanggalMulai.value || !tanggalSelesai.value) return 0;
  const d = (new Date(tanggalSelesai.value) - new Date(tanggalMulai.value)) / 86400000 + 1;
  return d > 0 ? Math.round(d) : 0;
});
const lamaCuti = computed(() => {
  if (!tanggalMulai.value || !tanggalSelesai.value) return 0;
  return hitungLamaCuti(tanggalMulai.value, tanggalSelesai.value, polaKerja.value, hariLibur.value);
});
const hariTakDihitung = computed(() => Math.max(0, totalHariKalender.value - lamaCuti.value));

async function cekSkTerakhir(idPegawai) {
  skTersedia.value = null;
  pegawaiTerpilih.value = null;
  sisaTahunan.value = null;
  if (!idPegawai) return;
  try {
    const [dok, peg, sisa] = await Promise.all([
      api.get(`/pegawai/${idPegawai}/dokumen`),
      api.get(`/pegawai/${idPegawai}`),
      api.get('/cuti/sisa', { params: { pegawaiId: idPegawai } }),
    ]);
    skTersedia.value = dok.data.find((d) => d.jenis === 'SK_TERAKHIR') || null;
    pegawaiTerpilih.value = peg.data;
    sisaTahunan.value = sisa.data;
  } catch {}
}

// Pegawai yang bertugas di Dinas tidak perlu rekomendasi kepala sekolah
const diDinas = computed(() =>
  ((pegawaiTerpilih.value?.tempatTugas || pegawaiTerpilih.value?.unor || '').toUpperCase()).includes('DINAS')
);
const jenisTahunan = computed(() => ['TAHUNAN', 'TAHUNAN_UMROH'].includes(jenisCuti.value));

onMounted(async () => {
  try {
    const { data } = await api.get('/hari-libur');
    hariLibur.value = data.map((h) => h.tanggal);
  } catch {}
  if (auth.isAdmin) {
    const { data } = await api.get('/pegawai');
    daftarPegawai.value = data;
  } else if (auth.user?.pegawaiId) {
    await cekSkTerakhir(auth.user.pegawaiId);
  }
});

watch(pegawaiId, (id) => cekSkTerakhir(id));

function gantiJenis() { files.value = {}; }
function pilihFile(kode, e) { files.value = { ...files.value, [kode]: e.target.files }; }

async function kirim() {
  if (jenisTahunan.value && sisaTahunan.value) {
    if (sisaTahunan.value.totalSisa <= 0) {
      return Swal.fire({ icon: 'error', title: 'Jatah cuti tahunan habis', text: 'Anda telah mengambil 12 hari cuti tahunan (termasuk pengajuan yang sedang diproses).', ...swalTema });
    }
    if (lamaCuti.value > sisaTahunan.value.totalSisa) {
      return Swal.fire({ icon: 'warning', title: 'Melebihi sisa cuti', text: `Sisa cuti tahunan ${sisaTahunan.value.totalSisa} hari, pengajuan ${lamaCuti.value} hari.`, ...swalTema });
    }
  }
  // validasi berkas wajib di sisi klien
  // (SK Terakhir dilewati jika sudah tersedia di dokumen pegawai — server otomatis melampirkannya)
  for (const b of config.value.berkas) {
    if (b.wajib && !(files.value[b.kode]?.length)) {
      if (b.kode === 'SK_TERAKHIR' && skTersedia.value) continue;
      if (b.kode === 'REKOMENDASI_KEPSEK' && diDinas.value) continue;
      return Swal.fire({ icon: 'warning', title: 'Berkas belum lengkap', text: `Unggah dulu: ${b.label}`, ...swalTema });
    }
  }
  mengirim.value = true;
  const fd = new FormData();
  fd.append('jenisCuti', jenisCuti.value);
  if (auth.isAdmin) fd.append('pegawaiId', pegawaiId.value);
  fd.append('tanggalMulai', tanggalMulai.value);
  fd.append('tanggalSelesai', tanggalSelesai.value);
  fd.append('polaKerja', polaKerja.value);
  fd.append('alasan', alasan.value);
  fd.append('alamatSelamaCuti', alamat.value);
  fd.append('noHp', noHp.value);
  for (const [kode, list] of Object.entries(files.value)) {
    for (const f of list) fd.append(`berkas_${kode}`, f);
  }
  try {
    const { data } = await api.post('/cuti', fd);
    await Swal.fire({ icon: 'success', title: 'Pengajuan terkirim', text: 'Pengajuan cuti menunggu diproses admin.', ...swalTema });
    router.push(`/cuti/${data.id}`);
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Pengajuan gagal', text: pesanError(e), ...swalTema });
  } finally {
    mengirim.value = false;
  }
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Ajukan Cuti</h2>
        <p>Lengkapi data dan unggah kelengkapan berkas sesuai jenis cuti (PDF/JPG/PNG, maks. 5 MB per file).</p>
      </div>
    </div>

    <form @submit.prevent="kirim">
      <div class="card">
        <div class="form-grid">
          <div v-if="auth.isAdmin">
            <label class="form-label">Pegawai yang Diajukan <span class="wajib">*</span></label>
            <select v-model="pegawaiId" class="form-input" required>
              <option value="" disabled>— pilih pegawai aktif —</option>
              <option v-for="p in daftarPegawai" :key="p.id" :value="p.id">{{ p.nama }} — {{ p.nip }}</option>
            </select>
          </div>
          <div>
            <label class="form-label">Jenis Cuti <span class="wajib">*</span></label>
            <select v-model="jenisCuti" class="form-input" @change="gantiJenis">
              <option v-for="(v, k) in JENIS_CUTI" :key="k" :value="k">{{ v.label }}</option>
            </select>
          </div>
          <div>
            <label class="form-label">Tanggal Mulai <span class="wajib">*</span></label>
            <input v-model="tanggalMulai" type="date" class="form-input" required />
          </div>
          <div>
            <label class="form-label">Tanggal Selesai <span class="wajib">*</span></label>
            <input v-model="tanggalSelesai" type="date" class="form-input" :min="tanggalMulai" required />
          </div>
          <div>
            <label class="form-label">Pola Hari Kerja <span class="wajib">*</span></label>
            <select v-model="polaKerja" class="form-input">
              <option v-for="pk in POLA_KERJA" :key="pk.kode" :value="pk.kode">{{ pk.label }}</option>
            </select>
            <p class="hint" v-if="totalHariKalender">
              Lama cuti terhitung: <b style="color:var(--hijau)">{{ lamaCuti }} hari</b>
              <template v-if="hariTakDihitung"> — {{ hariTakDihitung }} hari (akhir pekan/hari libur) tidak dihitung dari {{ totalHariKalender }} hari kalender.</template>
            </p>
          </div>
        </div>
        <label class="form-label">Alasan Cuti <span class="wajib">*</span></label>
        <textarea v-model="alasan" class="form-input" rows="3" required placeholder="Tuliskan alasan pengajuan cuti"></textarea>
        <label class="form-label">Alamat Selama Cuti</label>
        <input v-model="alamat" class="form-input" placeholder="Alamat yang dapat dihubungi selama cuti" />
        <label class="form-label">No. HP <span class="wajib">*</span></label>
        <input v-model="noHp" type="tel" class="form-input" placeholder="Nomor HP yang dapat dihubungi" required />
        <div v-if="jenisTahunan && sisaTahunan && sisaTahunan.firstYear !== null" class="hint" style="margin-top:10px; padding:9px 12px; border-radius:8px; background:var(--hijau-muda); color:var(--hijau); font-weight:600">
          Sisa cuti tahunan: {{ sisaTahunan.totalSisa }} hari
          (<template v-for="(r, i) in sisaTahunan.rincian" :key="r.tahun">{{ r.tahun }}: {{ r.sisa }}<template v-if="i < sisaTahunan.rincian.length - 1"> · </template></template>)
          — pemakaian memotong sisa tahun terlama lebih dulu.
        </div>
        <div v-else-if="jenisTahunan && sisaTahunan" class="hint" style="margin-top:10px">Belum ada riwayat pengajuan; jatah cuti tahunan penuh 12 hari.</div>
      </div>

      <div class="card">
        <h3>Kelengkapan Berkas — {{ config.label }}</h3>
        <p class="hint" style="margin-bottom:14px">Berkas bertanda <span class="wajib">*</span> wajib diunggah.</p>
        <div v-for="b in config.berkas" :key="b.kode" class="berkas-item" :class="{ terisi: files[b.kode]?.length || (b.kode === 'SK_TERAKHIR' && skTersedia) }">
          <div class="judul">{{ b.label }} <span v-if="b.wajib" class="wajib">*</span></div>
          <div v-if="b.kode === 'REKOMENDASI_KEPSEK' && diDinas" class="hint" style="color:var(--hijau); font-weight:600; margin-top:4px">
            ✓ Tidak wajib — pegawai bertempat tugas di Dinas (rekomendasi kepala sekolah hanya untuk pegawai sekolah).
          </div>
          <div v-if="b.kode === 'SK_TERAKHIR' && skTersedia" class="hint" style="color:var(--hijau); font-weight:600; margin-top:4px">
            ✓ SK Terakhir sudah tersedia di data pegawai ({{ skTersedia.namaFile }}) — otomatis dilampirkan, tidak perlu unggah lagi.
            Unggah file di bawah hanya jika ingin memakai SK yang berbeda.
          </div>
          <input type="file" accept=".pdf,.jpg,.jpeg,.png,.webp" :multiple="!!b.multiple" @change="pilihFile(b.kode, $event)" />
          <div class="hint" v-if="files[b.kode]?.length">✓ {{ files[b.kode].length }} file dipilih</div>
        </div>
      </div>

      <div style="display:flex; gap:10px; justify-content:flex-end">
        <router-link to="/cuti" class="btn btn-putih">Batal</router-link>
        <button type="submit" class="btn btn-hijau" :disabled="mengirim">{{ mengirim ? 'Mengirim…' : 'Kirim Pengajuan' }}</button>
      </div>
    </form>
  </div>
</template>
