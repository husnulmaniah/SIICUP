import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';

// GET /api/hari-libur — semua user login (dipakai form cuti untuk menghitung lama cuti)
export async function GET(req) {
  const { response } = await requireAuth(req);
  if (response) return response;
  const data = await prisma.hariLibur.findMany({ orderBy: { tanggal: 'asc' } });
  return json(data);
}

// POST /api/hari-libur — admin tambah hari libur { tanggal: 'YYYY-MM-DD', keterangan }
export async function POST(req) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const { tanggal, keterangan } = await req.json();
  if (!/^\d{4}-\d{2}-\d{2}$/.test(tanggal || '')) return errJson('Tanggal harus berformat YYYY-MM-DD.');
  if (!keterangan?.trim()) return errJson('Keterangan wajib diisi.');
  const ada = await prisma.hariLibur.findUnique({ where: { tanggal } });
  if (ada) return errJson(`Tanggal ${tanggal} sudah terdaftar sebagai: ${ada.keterangan}.`);
  const item = await prisma.hariLibur.create({ data: { tanggal, keterangan: keterangan.trim() } });
  return json(item, 201);
}
