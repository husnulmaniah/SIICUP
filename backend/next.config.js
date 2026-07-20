/** @type {import('next').NextConfig} */
const nextConfig = {
  experimental: {
    // Pastikan logo kop ikut terbundel di serverless function pembuat surat
    outputFileTracingIncludes: {
      '/api/cuti/[id]/surat': ['./assets/**'],
    },
  },
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
};
module.exports = nextConfig;
