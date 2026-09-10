/** @type {import('next').NextConfig} */
const nextConfig = {
  experimental: {
    // Pastikan logo kop ikut terbundel di serverless function pembuat surat
    outputFileTracingIncludes: {
      '/api/cuti/[id]/surat': ['./assets/**'],
    },
  },
<<<<<<< HEAD
  // Header CORS ditangani di src/middleware.js agar berlaku pada semua respons API.
=======
  async headers() {
    return [
      {
        source: '/api/:path*',
        headers: [
          { key: 'Access-Control-Allow-Origin', value: '*' },
          { key: 'Access-Control-Allow-Methods', value: 'GET,POST,PUT,PATCH,DELETE,OPTIONS' },
          { key: 'Access-Control-Allow-Headers', value: 'Content-Type, Authorization' },
        ],
      },
    ];
  },
>>>>>>> f1249a356178981b02639d968356a5a7494816f5
};
module.exports = nextConfig;
