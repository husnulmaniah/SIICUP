import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
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

app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset: Aura,
    options: {
      darkModeSelector: '.app-dark-mode',
      cssLayer: false,
    },
  },
})
app.use(ToastService)
app.use(ConfirmationService)

app.mount('#app')
