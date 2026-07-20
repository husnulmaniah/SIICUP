// Kebijakan password: minimal 8 karakter, mengandung huruf dan angka.
export function cekKebijakanPassword(pass) {
  if (!pass || pass.length < 8) return 'Password minimal 8 karakter.';
  if (!/[a-zA-Z]/.test(pass) || !/[0-9]/.test(pass)) return 'Password harus mengandung huruf dan angka.';
  return null;
}

export const MAKS_GAGAL_LOGIN = 5;
export const DURASI_KUNCI_MENIT = 15;
