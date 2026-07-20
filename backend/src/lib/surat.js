import fs from 'fs';
import path from 'path';
import {
  Document, Packer, Paragraph, TextRun, Table, TableRow, TableCell,
  WidthType, AlignmentType, BorderStyle, VerticalAlign, ImageRun,
} from 'docx';
import { JENIS_CUTI } from './cuti-config';

const BULAN_ROMAWI = ['I','II','III','IV','V','VI','VII','VIII','IX','X','XI','XII'];
const fmt = (t) => new Date(t).toLocaleDateString('id-ID', { day: '2-digit', month: 'long', year: 'numeric' });

/** Hitung masa kerja "X Tahun Y Bulan" dari TMT CPNS (jika tanggal valid) */
export function hitungMasaKerja(tmt) {
  if (!tmt) return '-';
  const d = new Date(tmt);
  if (isNaN(d)) return '-';
  const now = new Date();
  let bulan = (now.getFullYear() - d.getFullYear()) * 12 + (now.getMonth() - d.getMonth());
  if (bulan < 0) return '-';
  return `${Math.floor(bulan / 12)} Tahun ${bulan % 12} Bulan`;
}

export function defaultOpsi(cuti) {
  const now = new Date();
  return {
    nomorSurat: `800.1.11.3/          /Disdikbud/${BULAN_ROMAWI[now.getMonth()]}/${now.getFullYear()}`,
    kota: 'Kolonodale',
    tanggalSurat: fmt(now),
    kepalaDinas: 'MOH. RIDWAN DM. S.Ag',
    nipKepalaDinas: '19740111 199803 1 004',
  };
}

// ---------- util ----------
const run = (text, o = {}) => new TextRun({ text, size: 22, font: 'Times New Roman', ...o });
const para = (text, o = {}, runOpts = {}) =>
  new Paragraph({ children: [run(text, runOpts)], spacing: { after: 60 }, ...o });

const NO_BORDER = { style: BorderStyle.NONE, size: 0, color: 'FFFFFF' };
const noBorders = { top: NO_BORDER, bottom: NO_BORDER, left: NO_BORDER, right: NO_BORDER };

function cell(children, o = {}) {
  return new TableCell({
    children: Array.isArray(children) ? children : [children],
    verticalAlign: VerticalAlign.CENTER,
    margins: { top: 60, bottom: 60, left: 100, right: 100 },
    ...o,
  });
}
const tcell = (text, o = {}, runOpts = {}) => cell(para(text, { spacing: { after: 0 } }, runOpts), o);

function kop() {
  const teks = [
    new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [run('PEMERINTAH KABUPATEN MOROWALI UTARA', { bold: true, size: 26 })],
    }),
    new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [run('DINAS PENDIDIKAN DAN KEBUDAYAAN DAERAH', { bold: true, size: 28 })],
    }),
    new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [run('Alamat : Jln. Bumi Nangka Kompleks Perkantoran Kode Pos (94971)', { italics: true, size: 20 })],
    }),
    new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [run('KOLONODALE', { bold: true, size: 24 })],
      spacing: { after: 0 },
    }),
  ];

  // Sisipkan logo kabupaten jika tersedia (assets/logo-morut.png)
  let logoCellChildren = [para('', { spacing: { after: 0 } })];
  try {
    const logoPath = path.join(process.cwd(), 'assets', 'logo-morut.png');
    const logo = fs.readFileSync(logoPath);
    logoCellChildren = [
      new Paragraph({
        alignment: AlignmentType.CENTER,
        spacing: { after: 0 },
        children: [new ImageRun({ data: logo, transformation: { width: 66, height: 96 } })],
      }),
    ];
  } catch {}

  const tabelKop = new Table({
    width: { size: 100, type: WidthType.PERCENTAGE },
    rows: [
      new TableRow({
        children: [
          cell(logoCellChildren, { borders: noBorders, width: { size: 14, type: WidthType.PERCENTAGE } }),
          cell(teks, { borders: noBorders, width: { size: 72, type: WidthType.PERCENTAGE } }),
          cell(para('', { spacing: { after: 0 } }), { borders: noBorders, width: { size: 14, type: WidthType.PERCENTAGE } }),
        ],
      }),
    ],
  });

  return [
    tabelKop,
    new Paragraph({
      children: [],
      border: { bottom: { style: BorderStyle.THICK_THIN_MEDIUM_GAP, size: 18, color: '000000' } },
      spacing: { after: 240 },
    }),
  ];
}

function ttdKepalaDinas(opsi, judul = 'Kepala Dinas') {
  return [
    para(judul, { indent: { left: 4700 }, spacing: { after: 900 } }),
    new Paragraph({
      indent: { left: 4700 },
      children: [run(opsi.kepalaDinas, { bold: true, underline: {} })],
    }),
    para(`NIP: ${opsi.nipKepalaDinas}`, { indent: { left: 4700 } }),
  ];
}

// ---------- Surat Rekomendasi (image 2) ----------
export async function buatRekomendasi(cuti, opsi) {
  const label = JENIS_CUTI[cuti.jenisCuti]?.label || cuti.jenisCuti;
  const periode = `${fmt(cuti.tanggalMulai)} - ${fmt(cuti.tanggalSelesai)}`;

  const infoRow = (k, v) =>
    new TableRow({
      children: [
        tcell(k, { borders: noBorders, width: { size: 18, type: WidthType.PERCENTAGE } }),
        tcell(':', { borders: noBorders, width: { size: 3, type: WidthType.PERCENTAGE } }),
        tcell(v, { borders: noBorders, width: { size: 79, type: WidthType.PERCENTAGE } }),
      ],
    });

  const doc = new Document({
    sections: [{
      properties: { page: { margin: { top: 900, bottom: 900, left: 1200, right: 1200 } } },
      children: [
        ...kop(),
        new Table({
          width: { size: 100, type: WidthType.PERCENTAGE },
          rows: [
            infoRow('Nomor', opsi.nomorSurat),
            infoRow('Lampiran', 'Satu Berkas'),
            infoRow('Perihal', `Rekomendasi Izin ${label}`),
          ],
        }),
        para('', {}),
        para('Yth. Bupati Morowali Utara', {}, { bold: true }),
        para('Cq Kepala Badan Kepegawaian dan', {}, { bold: true }),
        para('Pengembangan SDM', {}, { bold: true }),
        para('', {}),
        para('Di -', {}, { bold: true }),
        para('        Tempat', {}, { bold: true }),
        para('', {}),
        new Paragraph({
          alignment: AlignmentType.JUSTIFIED,
          indent: { left: 720, firstLine: 480 },
          spacing: { after: 120, line: 320 },
          children: [
            run(`Menindak lanjuti surat permohonan ${label} atas nama `),
            run(`${cuti.pegawai.nama}; `, { bold: true }),
            run(`Tanggal ${periode} dengan ini kami tidak keberatan dan menyetujui permohonan tersebut kami teruskan kepada Bapak untuk ditindaklanjuti (Permohonan Terlampir).`),
          ],
        }),
        new Paragraph({
          alignment: AlignmentType.JUSTIFIED,
          indent: { left: 720, firstLine: 480 },
          spacing: { after: 360, line: 320 },
          children: [run('Demikian Surat Permohonan Cuti ini kami teruskan kepada Bapak, atas pertimbangan Bapak kami ucapkan terima kasih.')],
        }),
        para(`${opsi.kota}, ${opsi.tanggalSurat}.`, { indent: { left: 4700 } }),
        ...ttdKepalaDinas(opsi),
      ],
    }],
  });
  return Packer.toBuffer(doc);
}
