// Daftar field pegawai yang boleh diisi/diubah (format SIASN/BKN)
export const FIELD_PEGAWAI = [
  'nik','nama','tempatLahir','tanggalLahir','jenisKelamin','statusCpnsPns',
  'tanggalSkCpns','tmtCpns','tanggalSkPns','tmtPns','golAwal','golAkhir','tmtGolongan',
  'jenisJabatan','jabatan','tmtJabatan','tingkatPendidikan','pendidikan','kecamatan',
  'unor','tempatTugas','status','tanggalKgbTerakhir','tanggalKpTerakhir','tanggalPensiun',
  'tanggalKgb','tanggalKp','tahunPengangkatan',
];

export function sanitize(body) {
  const out = {};
  for (const f of FIELD_PEGAWAI) if (body[f] !== undefined) out[f] = body[f] === '' ? null : body[f];
  if (body.aktif !== undefined) out.aktif = !!body.aktif;
  return out;
}
