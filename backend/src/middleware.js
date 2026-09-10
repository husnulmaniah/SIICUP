import { NextResponse } from 'next/server';

<<<<<<< HEAD
// Daftar origin yang diizinkan. Kosongkan ALLOWED_ORIGINS untuk mengizinkan semua (*).
// Contoh isi env: ALLOWED_ORIGINS="https://app.siicupdisdikbud.com,https://sicuti-app.vercel.app"
const daftarIzin = (process.env.ALLOWED_ORIGINS || '')
  .split(',')
  .map((o) => o.trim())
  .filter(Boolean);

function headerCors(origin) {
  const izinkan = daftarIzin.length === 0 ? '*' : daftarIzin.includes(origin) ? origin : daftarIzin[0];
  return {
    'Access-Control-Allow-Origin': izinkan,
    'Access-Control-Allow-Methods': 'GET,POST,PUT,PATCH,DELETE,OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type, Authorization',
    'Access-Control-Expose-Headers': 'Content-Disposition',
    'Access-Control-Max-Age': '86400',
    Vary: 'Origin',
  };
}

export function middleware(req) {
  const origin = req.headers.get('origin') || '';
  const cors = headerCors(origin);

  // Preflight
  if (req.method === 'OPTIONS') {
    return new NextResponse(null, { status: 204, headers: cors });
  }

  // Tempelkan header CORS ke seluruh respons /api/*
  const res = NextResponse.next();
  for (const [k, v] of Object.entries(cors)) res.headers.set(k, v);
  return res;
=======
export function middleware(req) {
  if (req.method === 'OPTIONS') {
    return new NextResponse(null, {
      status: 204,
      headers: {
        'Access-Control-Allow-Origin': '*',
        'Access-Control-Allow-Methods': 'GET,POST,PUT,PATCH,DELETE,OPTIONS',
        'Access-Control-Allow-Headers': 'Content-Type, Authorization',
        'Access-Control-Max-Age': '86400',
      },
    });
  }
  return NextResponse.next();
>>>>>>> f1249a356178981b02639d968356a5a7494816f5
}

export const config = { matcher: '/api/:path*' };
