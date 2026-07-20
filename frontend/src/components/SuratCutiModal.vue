<script setup>
import { ref, computed, onMounted } from 'vue';
import { jsPDF } from 'jspdf';
import html2canvas from 'html2canvas';
import Swal from 'sweetalert2';
import api, { pesanError, swalTema } from '../api';
import { labelJenis } from '../cuti-config';

const props = defineProps({ cutiId: { type: [Number, String], required: true } });
const emit = defineEmits(['tutup']);

const data = ref(null); // { cuti, opsi }
const kertasRef = ref(null);
const ukuranKertas = ref('a4'); // a4 | letter | legal — ukuran kertas PDF formulir
const tab = ref('formulir');
const mengunduh = ref(false);

const fmt = (t) => new Date(t).toLocaleDateString('id-ID', { day: '2-digit', month: 'long', year: 'numeric' });
function namaFileSurat(jenis, ekstensi = 'docx') {
  const p = data.value.cuti.pegawai;
  const namaAman = p.nama.normalize('NFKD').replace(/[^\w\s.-]/g, '').trim().replace(/\s+/g, '_');
  const prefix = { rekomendasi: 'REKOMENDASI', formulir: 'FORMULIR' }[jenis];
  return `${prefix}_CUTI_${p.nip}_${namaAman}.${ekstensi}`;
}

// Formulir diunduh sebagai PDF: render pratinjau apa adanya (WYSIWYG, termasuk logo kop)
async function unduhPdf() {
  if (!kertasRef.value) return;
  mengunduh.value = true;
  try {
    const canvas = await html2canvas(kertasRef.value, { scale: 2, useCORS: true, backgroundColor: '#ffffff' });
    // Satu halaman penuh sesuai ukuran kertas terpilih (A4 / Letter / Legal):
    // gambar diskalakan agar muat lebar DAN tinggi halaman, lalu diposisikan di tengah.
    const pdf = new jsPDF('p', 'mm', ukuranKertas.value);
    const pageW = pdf.internal.pageSize.getWidth();
    const pageH = pdf.internal.pageSize.getHeight();
    const margin = 8;
    const maxW = pageW - margin * 2;
    const maxH = pageH - margin * 2;
    const rasio = canvas.width / canvas.height;
    let w = maxW;
    let h = w / rasio;
    if (h > maxH) {
      h = maxH;
      w = h * rasio;
    }
    const x = (pageW - w) / 2;
    const img = canvas.toDataURL('image/png');
    pdf.addImage(img, 'PNG', x, margin, w, h);
    pdf.save(namaFileSurat('formulir', 'pdf'));
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal membuat PDF', text: String(e?.message || e), ...swalTema });
  } finally {
    mengunduh.value = false;
  }
}
const masaKerja = (tmt) => {
  if (!tmt) return '-';
  const d = new Date(tmt);
  if (isNaN(d)) return '-';
  const bulan = (new Date().getFullYear() - d.getFullYear()) * 12 + (new Date().getMonth() - d.getMonth());
  return bulan < 0 ? '-' : `${Math.floor(bulan / 12)} Tahun ${bulan % 12} Bulan`;
};

onMounted(async () => {
  try {
    const res = await api.get(`/cuti/${props.cutiId}/surat`);
    data.value = res.data;
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Tidak dapat memuat surat', text: pesanError(e), ...swalTema });
    emit('tutup');
  }
});

const NAMA_CUTI = {
  1: 'Cuti Tahunan', 2: 'Cuti Sakit', 3: 'Cuti Karena Alasan Penting',
  4: 'Cuti Besar', 5: 'Cuti Melahirkan', 6: 'Cuti di Luar Tanggungan Negara',
};
const NOMOR_JENIS = { TAHUNAN: 1, TAHUNAN_UMROH: 1, SAKIT: 2, ALASAN_PENTING: 3, MELAHIRKAN: 5 };
const nomorTerpilih = computed(() => NOMOR_JENIS[data.value?.cuti.jenisCuti] || 0);
const cek = (no) => no === nomorTerpilih.value;
const ket = (label) => (data.value?.catatanCuti || []).find((c) => c.label === label);

async function unduh(jenis) {
  mengunduh.value = true;
  try {
    const res = await api.post(
      `/cuti/${props.cutiId}/surat`,
      { jenis, opsi: data.value.opsi },
      { responseType: 'blob' }
    );
    const href = URL.createObjectURL(res.data);
    const a = Object.assign(document.createElement('a'), {
      href,
      download: namaFileSurat(jenis),
    });
    a.click();
    URL.revokeObjectURL(href);
  } catch (e) {
    Swal.fire({ icon: 'error', title: 'Gagal membuat Word', text: pesanError(e), ...swalTema });
  } finally {
    mengunduh.value = false;
  }
}
</script>

<template>
  <div class="surat-overlay" @click.self="emit('tutup')">
    <div class="surat-modal card" v-if="data">
      <div class="surat-toolbar">
        <div class="tab-group">
          <button class="btn btn-kecil" :class="tab === 'formulir' ? 'btn-hijau' : 'btn-putih'" @click="tab = 'formulir'">Formulir Cuti</button>
          <button class="btn btn-kecil" :class="tab === 'rekomendasi' ? 'btn-hijau' : 'btn-putih'" @click="tab = 'rekomendasi'">Surat Rekomendasi</button>
        </div>
        <div style="display:flex; gap:8px; align-items:center">
          <select v-if="tab === 'formulir'" v-model="ukuranKertas" class="form-input" style="width:auto; padding:5px 10px; font-size:12.5px" title="Ukuran kertas PDF">
            <option value="a4">A4 (210×297)</option>
            <option value="letter">Letter (216×279)</option>
            <option value="legal">Legal (216×356)</option>
          </select>
          <button class="btn btn-terakota btn-kecil" :disabled="mengunduh" @click="tab === 'formulir' ? unduhPdf() : unduh(tab)">
            ⬇️ {{ mengunduh ? 'Menyiapkan…' : tab === 'formulir' ? 'Unduh PDF (Formulir)' : 'Unduh Word (Rekomendasi)' }}
          </button>
          <button class="btn btn-putih btn-kecil" @click="emit('tutup')">Tutup</button>
        </div>
      </div>

      <!-- Opsi surat -->
      <div class="form-grid" style="margin-bottom:14px">
        <div v-if="tab === 'rekomendasi'">
          <label class="form-label">Nomor Surat</label>
          <input v-model="data.opsi.nomorSurat" class="form-input" />
        </div>
        <div>
          <label class="form-label">Tanggal Surat</label>
          <input v-model="data.opsi.tanggalSurat" class="form-input" />
        </div>
        <div>
          <label class="form-label">Nama Kepala Dinas</label>
          <input v-model="data.opsi.kepalaDinas" class="form-input" />
        </div>
        <div>
          <label class="form-label">NIP Kepala Dinas</label>
          <input v-model="data.opsi.nipKepalaDinas" class="form-input" />
        </div>
      </div>

      <!-- ============ PREVIEW ============ -->
      <div class="kertas-scroll"><div class="kertas" ref="kertasRef">
        <div class="kop">
          <img src="/logo-morut.png" alt="Lambang Kab. Morowali Utara" class="kop-logo" />
          <div class="kop-teks">
            <div class="kop-1">PEMERINTAH KABUPATEN MOROWALI UTARA</div>
            <div class="kop-2">DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH</div>
            <div class="kop-3"><i>Alamat : Jln. Bumi Nangka Kompleks Perkantoran Kode Pos (94971)</i></div>
            <div class="kop-4">KOLONODALE</div>
          </div>
        </div>

        <!-- FORMULIR -->
        <template v-if="tab === 'formulir'">
          <div style="margin: 0 0 12px 52%; text-align:left; line-height:1.5">
            {{ data.opsi.kota }}, {{ data.opsi.tanggalSurat }}.<br />
            Kepada<br />
            <b>Yth. Bupati Morowali Utara</b><br />
            Cq. Kepala Badan Kepegawaian dan<br />
            Pengembangan SDM<br />
            Di -<br />&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{{ data.opsi.kota }}
          </div>
          <div class="judul-surat">FORMULIR PERMINTAAN DAN PEMBERIAN CUTI</div>
          <!-- I. Data Pegawai (tanpa kolom kosong) -->
          <table class="tabel-surat">
            <tr>
              <td style="width:14%">Nama</td><td style="width:40%"><b>{{ data.cuti.pegawai.nama }}</b></td>
              <td style="width:14%">NIP</td><td>{{ data.cuti.pegawai.nip }}</td>
            </tr>
            <tr>
              <td>Jabatan</td><td>{{ data.cuti.pegawai.jabatan || '-' }}</td>
              <td>Masa Kerja</td><td>{{ masaKerja(data.cuti.pegawai.tmtCpns) }}</td>
            </tr>
            <tr><td>Unit Kerja</td><td colspan="3">{{ data.cuti.pegawai.tempatTugas || data.cuti.pegawai.unor || '-' }}</td></tr>
          </table>

          <!-- II. Jenis Cuti -->
          <table class="tabel-surat" style="margin-top:-1px">
            <tr class="seksi"><td colspan="6">Jenis Cuti yang di ambil</td></tr>
            <tr v-for="[a, b] in [[1,4],[2,5],[3,6]]" :key="a">
              <td style="width:4%">{{ a }}</td>
              <td style="width:38%">{{ NAMA_CUTI[a] }}</td>
              <td style="width:6%; text-align:center"><b v-if="cek(a)">✓</b></td>
              <td style="width:4%">{{ b }}</td>
              <td style="width:42%">{{ NAMA_CUTI[b] }}</td>
              <td style="width:6%; text-align:center"><b v-if="cek(b)">✓</b></td>
            </tr>
          </table>

          <!-- III. Alasan -->
          <table class="tabel-surat" style="margin-top:-1px">
            <tr class="seksi"><td>Alasan Cuti</td></tr>
            <tr><td>{{ data.cuti.alasan }}</td></tr>
          </table>

          <!-- IV. Lama Cuti (tanpa garis dalam) -->
          <table class="tabel-surat" style="margin-top:-1px">
            <tr class="seksi"><td>Lama Cuti</td></tr>
            <tr><td>Selama&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<b>{{ data.cuti.lamaCuti }}</b>&nbsp;Hari&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Mulai&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Tanggal {{ fmt(data.cuti.tanggalMulai) }} - {{ fmt(data.cuti.tanggalSelesai) }}</td></tr>
          </table>

          <!-- V. Catatan Cuti -->
          <table class="tabel-surat" style="margin-top:-1px">
            <tr class="seksi"><td colspan="2">Catatan Cuti</td></tr>
            <tr>
              <td style="width:52%; vertical-align:top; padding:0">
                <table class="tabel-dalam">
                  <tr><td style="width:8%">{{ nomorTerpilih || '' }}</td><td colspan="2">{{ nomorTerpilih ? NAMA_CUTI[nomorTerpilih] : '-' }}</td></tr>
                  <tr><td style="width:28%; text-align:center"><b>Tahun</b></td><td style="width:24%; text-align:center"><b>Sisa</b></td><td style="text-align:center"><b>Keterangan</b></td></tr>
                  <tr v-for="label in ['N-1','N-2','N']" :key="label">
                    <td>{{ label }}</td>
                    <td>{{ ket(label) && ket(label).sisa !== null ? ket(label).sisa + ' hari' : '' }}</td>
                    <td>{{ ket(label)?.keterangan || '' }}</td>
                  </tr>
                </table>
              </td>
              <td style="width:48%; vertical-align:top; padding:0">
                <table class="tabel-dalam">
                  <tr v-for="no in [1,2,3,4,5,6]" :key="no">
                    <td style="width:10%">{{ no }}</td>
                    <td>{{ NAMA_CUTI[no] }}</td>
                    <td style="width:12%; text-align:center"><b v-if="cek(no)">✓</b></td>
                  </tr>
                </table>
              </td>
            </tr>
          </table>

          <!-- VI. Alamat -->
          <table class="tabel-surat" style="margin-top:-1px">
            <tr class="seksi"><td colspan="2">Alamat Selama Menjalankan Cuti</td></tr>
            <tr>
              <td style="width:55%; vertical-align:top">{{ data.cuti.alamatSelamaCuti || '-' }}<br /><br />Telp: {{ data.cuti.noHp || '-' }}</td>
              <td style="text-align:center">Hormat Saya<br /><br /><br /><br /><b><u>{{ data.cuti.pegawai.nama }}</u></b><br />NIP: {{ data.cuti.pegawai.nip }}</td>
            </tr>
          </table>

          <!-- VII. Pertimbangan -->
          <table class="tabel-surat" style="margin-top:-1px">
            <tr class="seksi"><td colspan="4">Pertimbangan Atasan Langsung</td></tr>
            <tr>
              <td style="width:25%" class="dicentang"><b>Disetujui ✓</b></td>
              <td style="width:25%">Perubahan</td>
              <td style="width:25%">Ditangguhkan</td>
              <td style="width:25%">Tidak Disetujui</td>
            </tr>
            <tr>
              <td colspan="2"></td>
              <td colspan="2" style="text-align:center">Kepala Dinas<br /><br /><br /><br /><b><u>{{ data.opsi.kepalaDinas }}</u></b><br />NIP: {{ data.opsi.nipKepalaDinas }}</td>
            </tr>
          </table>
        </template>

        <!-- REKOMENDASI -->
        <template v-else>
          <table style="line-height:1.6">
            <tr><td>Nomor</td><td style="padding:0 8px">:</td><td>{{ data.opsi.nomorSurat }}</td></tr>
            <tr><td>Lampiran</td><td style="padding:0 8px">:</td><td>Satu Berkas</td></tr>
            <tr><td>Perihal</td><td style="padding:0 8px">:</td><td>Rekomendasi Izin {{ labelJenis(data.cuti.jenisCuti) }}</td></tr>
          </table>
          <div style="margin:18px 0; line-height:1.5">
            <b>Yth. Bupati Morowali Utara<br />Cq Kepala Badan Kepegawaian dan<br />Pengembangan SDM</b><br /><br />
            <b>Di -</b><br /><b>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Tempat</b>
          </div>
          <p style="text-align:justify; text-indent:40px; margin:0 0 10px 40px; line-height:1.8">
            Menindak lanjuti surat permohonan {{ labelJenis(data.cuti.jenisCuti) }} atas nama
            <b>{{ data.cuti.pegawai.nama }}</b>; Tanggal {{ fmt(data.cuti.tanggalMulai) }} - {{ fmt(data.cuti.tanggalSelesai) }}
            dengan ini kami tidak keberatan dan menyetujui permohonan tersebut kami teruskan kepada Bapak untuk
            ditindaklanjuti (Permohonan Terlampir).
          </p>
          <p style="text-align:justify; text-indent:40px; margin:0 0 40px 40px; line-height:1.8">
            Demikian Surat Permohonan Cuti ini kami teruskan kepada Bapak, atas pertimbangan Bapak kami ucapkan terima kasih.
          </p>
          <div style="margin-left:52%; text-align:left; line-height:1.6">
            {{ data.opsi.kota }}, {{ data.opsi.tanggalSurat }}.<br />
            Kepala Dinas<br /><br /><br /><br />
            <b><u>{{ data.opsi.kepalaDinas }}</u></b><br />
            NIP: {{ data.opsi.nipKepalaDinas }}
          </div>
        </template>
      </div></div>
    </div>
    <div v-else class="card" style="margin:0">Memuat preview surat…</div>
  </div>
</template>

<style scoped>
.surat-overlay {
  position: fixed; inset: 0; background: rgba(59, 35, 20, .5);
  display: grid; place-items: start center; z-index: 60; padding: 24px; overflow: auto;
}
.surat-modal { max-width: 860px; width: 100%; margin: 0; }
.surat-toolbar { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 10px; margin-bottom: 12px; }
.tab-group { display: flex; gap: 6px; }
.kertas {
  background: #fff; border: 1px solid var(--garis); padding: 34px 44px;
  font-family: 'Times New Roman', Times, serif; font-size: 13.5px; color: #000;
  box-shadow: inset 0 0 0 1px #f0e9dd, 0 2px 8px rgba(59,35,20,.08);
}
.kop {
  display: grid; grid-template-columns: 72px 1fr 72px; align-items: center;
  text-align: center; border-bottom: 3.5px double #000; padding-bottom: 6px; margin-bottom: 16px;
}
.kop-logo { width: 58px; height: auto; justify-self: start; }
.kop-teks { grid-column: 2; }
.kop-1 { font-weight: bold; font-size: 15px; }
.kop-2 { font-weight: bold; font-size: 17px; }
.kop-3 { font-size: 12px; }
.kop-4 { font-weight: bold; font-size: 14px; }
.judul-surat { text-align: center; font-weight: bold; text-decoration: underline; font-size: 15px; margin: 14px 0; }
.tabel-surat { width: 100%; border-collapse: collapse; }
.tabel-surat td { border: 1px solid #000; padding: 5px 8px; }
.tabel-surat .seksi td { font-weight: bold; background: #f6f2ea; }
.tabel-surat .dicentang { background: #eaf0e6; }
.tabel-dalam { width: 100%; border-collapse: collapse; }
.tabel-dalam td { border: 1px solid #000; padding: 4px 8px; }
.tabel-dalam tr:first-child td { border-top: none; }
.tabel-dalam td:first-child { border-left: none; }
.tabel-dalam tr:last-child td { border-bottom: none; }
.tabel-dalam td:last-child { border-right: none; }
</style>
