-- CreateTable
CREATE TABLE "User" (
    "id" SERIAL NOT NULL,
    "username" TEXT NOT NULL,
    "password" TEXT NOT NULL,
    "role" TEXT NOT NULL,
    "nama" TEXT NOT NULL,
    "pegawaiId" INTEGER,
    "mustChangePassword" BOOLEAN NOT NULL DEFAULT false,
    "gagalLogin" INTEGER NOT NULL DEFAULT 0,
    "terkunciSampai" TIMESTAMP(3),
    "loginTerakhir" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "User_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Pegawai" (
    "id" SERIAL NOT NULL,
    "nip" TEXT NOT NULL,
    "nik" TEXT,
    "nama" TEXT NOT NULL,
    "tempatLahir" TEXT,
    "tanggalLahir" TEXT,
    "jenisKelamin" TEXT,
    "statusCpnsPns" TEXT,
    "tanggalSkCpns" TEXT,
    "tmtCpns" TEXT,
    "tanggalSkPns" TEXT,
    "tmtPns" TEXT,
    "golAwal" TEXT,
    "golAkhir" TEXT,
    "tmtGolongan" TEXT,
    "jenisJabatan" TEXT,
    "jabatan" TEXT,
    "tmtJabatan" TEXT,
    "tingkatPendidikan" TEXT,
    "pendidikan" TEXT,
    "kecamatan" TEXT,
    "unor" TEXT,
    "tempatTugas" TEXT,
    "status" TEXT,
    "tanggalKgbTerakhir" TEXT,
    "tanggalKpTerakhir" TEXT,
    "tanggalPensiun" TEXT,
    "tanggalKgb" TEXT,
    "tanggalKp" TEXT,
    "tahunPengangkatan" TEXT,
    "aktif" BOOLEAN NOT NULL DEFAULT true,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Pegawai_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "PengajuanCuti" (
    "id" SERIAL NOT NULL,
    "pegawaiId" INTEGER NOT NULL,
    "jenisCuti" TEXT NOT NULL,
    "tanggalMulai" TIMESTAMP(3) NOT NULL,
    "tanggalSelesai" TIMESTAMP(3) NOT NULL,
    "lamaCuti" INTEGER NOT NULL,
    "alasan" TEXT NOT NULL,
    "alamatSelamaCuti" TEXT,
    "noHp" TEXT,
    "polaKerja" TEXT NOT NULL DEFAULT '5_HARI',
    "status" TEXT NOT NULL DEFAULT 'DIAJUKAN',
    "catatan" TEXT,
    "diajukanOlehId" INTEGER,
    "diprosesOleh" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "PengajuanCuti_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "BerkasCuti" (
    "id" SERIAL NOT NULL,
    "cutiId" INTEGER NOT NULL,
    "jenisBerkas" TEXT NOT NULL,
    "namaBerkas" TEXT NOT NULL,
    "namaFile" TEXT NOT NULL,
    "path" TEXT NOT NULL,
    "mimeType" TEXT,
    "ukuran" INTEGER,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "BerkasCuti_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "RiwayatCuti" (
    "id" SERIAL NOT NULL,
    "cutiId" INTEGER NOT NULL,
    "aksi" TEXT NOT NULL,
    "oleh" TEXT NOT NULL,
    "catatan" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "RiwayatCuti_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "PerubahanData" (
    "id" SERIAL NOT NULL,
    "pegawaiId" INTEGER NOT NULL,
    "dataBaru" TEXT NOT NULL,
    "alasan" TEXT,
    "status" TEXT NOT NULL DEFAULT 'DIAJUKAN',
    "catatan" TEXT,
    "diprosesOleh" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "PerubahanData_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "DokumenPegawai" (
    "id" SERIAL NOT NULL,
    "pegawaiId" INTEGER NOT NULL,
    "jenis" TEXT NOT NULL,
    "namaFile" TEXT NOT NULL,
    "path" TEXT NOT NULL,
    "mimeType" TEXT,
    "ukuran" INTEGER,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "DokumenPegawai_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "HariLibur" (
    "id" SERIAL NOT NULL,
    "tanggal" TEXT NOT NULL,
    "keterangan" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "HariLibur_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "User_username_key" ON "User"("username");

-- CreateIndex
CREATE UNIQUE INDEX "User_pegawaiId_key" ON "User"("pegawaiId");

-- CreateIndex
CREATE UNIQUE INDEX "Pegawai_nip_key" ON "Pegawai"("nip");

-- CreateIndex
CREATE UNIQUE INDEX "DokumenPegawai_pegawaiId_jenis_key" ON "DokumenPegawai"("pegawaiId", "jenis");

-- CreateIndex
CREATE UNIQUE INDEX "HariLibur_tanggal_key" ON "HariLibur"("tanggal");

-- AddForeignKey
ALTER TABLE "User" ADD CONSTRAINT "User_pegawaiId_fkey" FOREIGN KEY ("pegawaiId") REFERENCES "Pegawai"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "PengajuanCuti" ADD CONSTRAINT "PengajuanCuti_pegawaiId_fkey" FOREIGN KEY ("pegawaiId") REFERENCES "Pegawai"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "BerkasCuti" ADD CONSTRAINT "BerkasCuti_cutiId_fkey" FOREIGN KEY ("cutiId") REFERENCES "PengajuanCuti"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "RiwayatCuti" ADD CONSTRAINT "RiwayatCuti_cutiId_fkey" FOREIGN KEY ("cutiId") REFERENCES "PengajuanCuti"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "PerubahanData" ADD CONSTRAINT "PerubahanData_pegawaiId_fkey" FOREIGN KEY ("pegawaiId") REFERENCES "Pegawai"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "DokumenPegawai" ADD CONSTRAINT "DokumenPegawai_pegawaiId_fkey" FOREIGN KEY ("pegawaiId") REFERENCES "Pegawai"("id") ON DELETE CASCADE ON UPDATE CASCADE;
