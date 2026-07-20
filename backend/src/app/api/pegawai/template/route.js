import { requireAuth, ADMINS } from '@/lib/auth';
import { buatTemplate } from '@/lib/excel';

export async function GET(req) {
  const { response } = await requireAuth(req, ADMINS);
  if (response) return response;
  const buffer = buatTemplate();
  return new Response(buffer, {
    headers: {
      'Content-Type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'Content-Disposition': 'attachment; filename="TEMPLATE_IMPORT_PEGAWAI.xlsx"',
    },
  });
}
