/** @type {import('next').NextConfig} */
const nextConfig = {
  experimental: {
    // Pastikan logo kop ikut terbundel di serverless function pembuat surat
    outputFileTracingIncludes: {
      '/api/cuti/[id]/surat': ['./assets/**'],
    },
  },
  // Header CORS ditangani di src/middleware.js agar berlaku pada semua respons API.
};
module.exports = nextConfig;
