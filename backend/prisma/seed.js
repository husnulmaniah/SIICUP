const { PrismaClient } = require('@prisma/client');
const bcrypt = require('bcryptjs');

const prisma = new PrismaClient();

async function main() {
  const hash = (s) => bcrypt.hashSync(s, 10);

  await prisma.user.upsert({
    where: { username: 'admin' },
    update: {},
    create: { username: 'admin', password: hash('admin123'), role: 'ADMIN_UTAMA', nama: 'Administrator Utama' },
  });

  await prisma.user.upsert({
    where: { username: 'operator' },
    update: {},
    create: { username: 'operator', password: hash('operator123'), role: 'ADMIN_PEMBANTU', nama: 'Admin Pembantu' },
  });

  // Contoh pegawai + akun (username & password = NIP)
  const contoh = [
    { nip: '197401111998031004', nik: '7206091101740000', nama: 'MOH. RIDWAN DM. S.Ag', jabatan: 'KEPALA DINAS', jenisJabatan: 'STRUKTURAL', golAkhir: 'PEMBINA Tkt. I, IV/B', jenisKelamin: 'L', statusCpnsPns: 'PNS', unor: 'DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH', tempatTugas: 'DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH', kecamatan: 'PETASIA', status: 'AKTIF', tahunPengangkatan: '1998', tmtCpns: '1998-03-01' },
    { nip: '196902281997021002', nik: '7206032804690001', nama: 'BERNOULLI TANARI, S.Pd, M.Pd', jabatan: 'An. Kepala Dinas', jenisJabatan: 'STRUKTURAL', golAkhir: 'PEMBINA Tkt. I, IV/B', jenisKelamin: 'L', statusCpnsPns: 'PNS', unor: 'DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH', tempatTugas: 'DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH', kecamatan: 'PETASIA', status: 'AKTIF', tahunPengangkatan: '1997', tmtCpns: '1997-02-01' },
  ];
  for (const p of contoh) {
    const pegawai = await prisma.pegawai.upsert({ where: { nip: p.nip }, update: {}, create: p });
    await prisma.user.upsert({
      where: { username: p.nip },
      update: {},
      create: { username: p.nip, password: hash(p.nip), role: 'PEGAWAI', nama: p.nama, pegawaiId: pegawai.id },
    });
  }

  console.log('Seed selesai.');
  console.log('Admin Utama    -> username: admin     password: admin123');
  console.log('Admin Pembantu -> username: operator  password: operator123');
  console.log('Pegawai contoh -> username & password = NIP (mis. 197401111998031004)');
}

main().finally(() => prisma.$disconnect());
