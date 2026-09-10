import { prisma } from '@/lib/prisma';
import { requireAuth, ADMINS, json, errJson } from '@/lib/auth';

export async function DELETE(req, { params }) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const id = Number(params.id);
  const ada = await prisma.hariLibur.findUnique({ where: { id } });
  if (!ada) return errJson('Hari libur tidak ditemukan.', 404);
  await prisma.hariLibur.delete({ where: { id } });
  return json({ message: 'Hari libur dihapus.' });
}
