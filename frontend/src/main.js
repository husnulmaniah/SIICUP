import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import { definePreset } from '@primevue/themes'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import 'primeicons/primeicons.css'
// PrimeFlex: grid & utility CSS (mis. col-12 md:col-6, flex, gap, dsb) --
// setara dengan Bootstrap 5 grid/utilities tapi dibuat khusus untuk PrimeVue
// sehingga tidak bertabrakan dengan style komponennya. Dipakai untuk membuat
// form & layout lebih responsive di ukuran layar HP/tablet.
import 'primeflex/primeflex.css'

import './style.css'
import App from './App.vue'
import router from './router'

const app = createApp(App)

// Tema warna aplikasi: biru-teal (ganti dari default Aura yang emerald).
// Semua warna aksen di komponen PrimeVue (tombol, checkbox, switch, tab
// aktif, dsb) otomatis ikut lewat token --p-primary-* ini -- warna aksen
// yang di-hardcode langsung di style masing-masing halaman (AppLayout,
// LoginView, dst) diseragamkan terpisah supaya konsisten dengan palet yang
// sama (lihat komentar "tema biru-teal" di file-file tersebut).
const SuccessTeal = definePreset(Aura, {
  semantic: {
    primary: {
      50: '#f0fdfa',
      100: '#ccfbf1',
      200: '#99f6e4',
      300: '#5eead4',
      400: '#2dd4bf',
      500: '#14b8a6',
      600: '#0d9488',
      700: '#0f766e',
      800: '#115e59',
      900: '#134e4a',
      950: '#042f2e',
    },
  },
})

app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset: SuccessTeal,
    options: {
      darkModeSelector: '.app-dark-mode',
      cssLayer: false,
    },
  },
})
app.use(ToastService)
app.use(ConfirmationService)

app.mount('#app')
