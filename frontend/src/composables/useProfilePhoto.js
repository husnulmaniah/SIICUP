import { ref } from 'vue'
import http from '../api/http'

// Foto profil pegawai yang sedang login -- state SATU SALINAN dibagikan ke
// seluruh aplikasi (avatar topbar di AppLayout.vue & halaman Profil Saya di
// ProfilSayaView.vue) memakai pola composable singleton Vue 3 (ref
// didefinisikan di LUAR fungsi yang di-export, jadi semua komponen yang
// import ini memakai state yang sama, bukan salinan masing-masing).
//
// Kenapa perlu state bersama: pegawai boleh mengganti/menghapus foto
// profilnya sendiri kapan saja tanpa persetujuan administrator/admin (lihat
// ProfilSayaView.vue) -- begitu itu terjadi, avatar di topbar (AppLayout.vue)
// harus ikut berubah SEKETIKA juga, tanpa perlu logout/login ulang atau
// reload halaman.
const url = ref('')
const loaded = ref(false)
let currentPegawaiId = null

function revoke() {
  if (url.value) {
    window.URL.revokeObjectURL(url.value)
    url.value = ''
  }
}

// refresh mengambil ulang foto profil pegawai idPegawai lewat endpoint
// GET /pegawai/{id}/foto (lihat backend/handlers/pegawai.go) sebagai blob,
// lalu membuat object URL baru untuk dipakai sebagai src <img>/Avatar.
// Kalau belum ada foto (404) atau gagal dimuat, url dibiarkan kosong --
// pemanggil tinggal fallback ke avatar inisial seperti sebelum fitur ini ada.
async function refresh(idPegawai) {
  currentPegawaiId = idPegawai || null
  revoke()
  loaded.value = false
  if (!idPegawai) {
    loaded.value = true
    return
  }
  try {
    const res = await http.get(`/pegawai/${idPegawai}/foto`, { responseType: 'blob' })
    // race guard sederhana -- kalau idPegawai berubah (mis. logout lalu login
    // akun lain) sementara request ini masih berjalan, jangan pakai hasilnya.
    if (currentPegawaiId === idPegawai) {
      url.value = window.URL.createObjectURL(res.data)
    }
  } catch (e) {
    // tidak ada foto / gagal muat -- biarkan url kosong (fallback inisial)
  } finally {
    loaded.value = true
  }
}

function clear() {
  currentPegawaiId = null
  revoke()
  loaded.value = false
}

export function useProfilePhoto() {
  return { url, loaded, refresh, clear }
}
