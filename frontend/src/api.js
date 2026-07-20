import axios from 'axios';
import Swal from 'sweetalert2';
import { useAuthStore } from './stores/auth';

// Di Vercel: biarkan '/api' dan gunakan rewrite di vercel.json (disarankan),
// atau isi VITE_API_URL dengan URL backend penuh (mis. https://sicuti-api.vercel.app/api)
const api = axios.create({ baseURL: import.meta.env.VITE_API_URL || '/api' });

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('sicuti_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      const auth = useAuthStore();
      auth.logout(false);
      if (location.pathname !== '/login') location.href = '/login';
    }
    return Promise.reject(err);
  }
);

export const pesanError = (err, fallback = 'Terjadi kesalahan.') =>
  err?.response?.data?.error || fallback;

export const toast = Swal.mixin({
  toast: true, position: 'top-end', showConfirmButton: false, timer: 2600, timerProgressBar: true,
});

export const swalTema = {
  confirmButtonColor: '#2f5233',
  cancelButtonColor: '#c05621',
  background: '#fdfbf7',
  color: '#3b2314',
};

// Bangun URL absolut endpoint API (untuk link 'Lihat/Unduh' yang dibuka di tab baru)
export const apiUrl = (p) => `${import.meta.env.VITE_API_URL || '/api'}${p}`;

export default api;
