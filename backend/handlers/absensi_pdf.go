package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cuti-app/assets"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// absensi_pdf.go menghasilkan berkas PDF rekap absen SATU pegawai untuk satu
// bulan -- dipakai tombol "Unduh PDF" pada dialog Detail Absen di halaman
// Rekap Absen (admin/administrator). Isinya sengaja dibuat sama dengan yang
// tampil di layar: identitas pegawai, tabel hari kerja (jam masuk, menit
// terlambat, jam pulang, foto masuk/pulang, titik koordinat, status), plus
// ringkasan jumlah hadir/terlambat/DD/izin/sakit/tidak absen.
//
// PDF ditulis memakai penulis PDF bawaan aplikasi (utils/pdfwriter.go, tanpa
// dependensi eksternal) dengan kop surat yang sama seperti formulir cuti
// (lihat drawLetterhead di formulir.go).

var hariIndo = map[time.Weekday]string{
	time.Sunday:    "Minggu",
	time.Monday:    "Senin",
	time.Tuesday:   "Selasa",
	time.Wednesday: "Rabu",
	time.Thursday:  "Kamis",
	time.Friday:    "Jumat",
	time.Saturday:  "Sabtu",
}

// barisRekapPDF adalah satu baris tabel (satu hari kerja) pada PDF.
type barisRekapPDF struct {
	Tanggal    time.Time
	JamMasuk   string
	Terlambat  string
	JamPulang  string
	Status     string
	Koordinat  string
	MapsURL    string // tautan Google Maps ke titik koordinat baris ini
	FotoMasuk  string // nama gambar yang sudah diregistrasi ke PDFDoc ("" = tidak ada)
	FotoPulang string
}

type ringkasanRekapPDF struct {
	Hadir      int
	Terlambat  int
	DinasDalam int
	Izin       int
	Sakit      int
	TidakAbsen int
}

// exportRekapAbsensiPegawaiPDF menangani GET /api/absensi/rekap/pdf
// ?id_pegawai=..&bulan=..&tahun=..
func exportRekapAbsensiPegawaiPDF(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	idStr := strings.TrimSpace(r.URL.Query().Get("id_pegawai"))
	if idStr == "" {
		utils.Error(w, http.StatusBadRequest, "pegawai belum dipilih")
		return
	}

	var pegawai models.Pegawai
	if err := db.Omit(dokumenFileFields...).Preload("Jabatan").Preload("UnitKerja").
		First(&pegawai, "id = ?", idStr).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "data pegawai tidak ditemukan")
		return
	}

	now := absensiNow()
	bulan := now.Month()
	tahun := now.Year()
	if v, err := strconv.Atoi(r.URL.Query().Get("bulan")); err == nil && v >= 1 && v <= 12 {
		bulan = time.Month(v)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("tahun")); err == nil && v > 2000 {
		tahun = v
	}

	loc := now.Location()
	start := time.Date(tahun, bulan, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)
	limit := end
	if today := absensiToday(); today.Before(limit) {
		limit = today
	}

	// baris absen (BESERTA fotonya -- dipakai sebagai bukti di PDF) & surat
	// pendukung pada bulan yang diminta
	absensiRows := []models.Absensi{}
	db.Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", pegawai.ID, start, end).Find(&absensiRows)
	absenByTanggal := map[string]models.Absensi{}
	for _, a := range absensiRows {
		absenByTanggal[a.Tanggal.Format("2006-01-02")] = a
	}

	dokumenRows := []models.AbsensiDokumen{}
	db.Omit("file").Where("id_pegawai = ? AND tanggal BETWEEN ? AND ?", pegawai.ID, start, end).Find(&dokumenRows)
	dokumenByTanggal := map[string]models.AbsensiDokumen{}
	for _, d := range dokumenRows {
		dokumenByTanggal[d.Tanggal.Format("2006-01-02")] = d
	}

	doc := utils.NewPDFDoc()
	if err := doc.RegisterImage("logo", assets.LogoPNG); err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyiapkan logo: "+err.Error())
		return
	}

	// Hari kerja mengikuti pola pegawai (sekolah 6 hari, kantor dinas 5 hari)
	// dan melewati tanggal merah -- sama seperti perhitungan rekap di layar.
	holidaySet := holidaySetInRange(db, start, limit)
	hariKerja := workingDaysWithHolidaySet(start, limit, sixDayWeekForTempatTgs(pegawai.TempatTgs), holidaySet)

	var baris []barisRekapPDF
	var ringkasan ringkasanRekapPDF
	for _, d := range hariKerja {
		key := d.Format("2006-01-02")
		row := barisRekapPDF{Tanggal: d, JamMasuk: "-", Terlambat: "-", JamPulang: "-", Koordinat: "-", Status: "Tidak Absen"}

		if a, ada := absenByTanggal[key]; ada && (a.JamMasuk != nil || a.JamPulang != nil) {
			row.Status = "Hadir"
			ringkasan.Hadir++
			if a.JamMasuk != nil {
				row.JamMasuk = formatJamAbsensi(a.JamMasuk)
			}
			if a.JamPulang != nil {
				row.JamPulang = formatJamAbsensi(a.JamPulang)
			}
			if a.TerlambatMenit > 0 {
				row.Terlambat = strconv.Itoa(a.TerlambatMenit) + " mnt"
				ringkasan.Terlambat++
			}
			// titik koordinat yang ditampilkan mengikuti absen TERAKHIR pada
			// hari itu (pulang kalau sudah ada, kalau belum ya masuk) dan
			// dibuat bisa diklik langsung ke Google Maps.
			if lat, lng, ok := koordinatTerakhir(a); ok {
				row.Koordinat = fmt.Sprintf("%.5f, %.5f", lat, lng)
				row.MapsURL = mapsURL(lat, lng)
			}
			// foto dikecilkan dulu (64px) lalu diubah ke PNG karena penulis PDF
			// hanya menerima PNG -- lihat fotoThumbPNG di absensi.go
			if len(a.FotoMasuk) > 0 {
				if pngData, err := fotoThumbPNG(a.FotoMasuk, 64); err == nil {
					name := fmt.Sprintf("fm%d", a.ID)
					if doc.RegisterImage(name, pngData) == nil {
						row.FotoMasuk = name
					}
				}
			}
			if len(a.FotoPulang) > 0 {
				if pngData, err := fotoThumbPNG(a.FotoPulang, 64); err == nil {
					name := fmt.Sprintf("fp%d", a.ID)
					if doc.RegisterImage(name, pngData) == nil {
						row.FotoPulang = name
					}
				}
			}
		} else if dok, ada := dokumenByTanggal[key]; ada {
			kode := models.AbsensiDokumenKode[dok.Jenis]
			row.Status = fmt.Sprintf("%s (%s)", models.AbsensiDokumenKodeLabel[kode], kode)
			switch kode {
			case "DD":
				ringkasan.DinasDalam++
			case "I":
				ringkasan.Izin++
			case "S":
				ringkasan.Sakit++
			}
		} else {
			ringkasan.TidakAbsen++
		}

		baris = append(baris, row)
	}

	pdfBytes, err := buildRekapAbsensiPDF(doc, pegawai, bulan, tahun, baris, ringkasan)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat PDF: "+err.Error())
		return
	}

	namaFile := fmt.Sprintf("rekap_absen_%s_%s.pdf", slugNamaFile(pegawai.Nama), start.Format("2006-01"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", namaFile))
	w.Write(pdfBytes)
}

// slugNamaFile membersihkan nama pegawai supaya aman dipakai sebagai nama
// berkas (spasi/tanda baca jadi garis bawah).
func slugNamaFile(nama string) string {
	var b strings.Builder
	for _, r := range nama {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// buildRekapAbsensiPDF menggambar seluruh halaman PDF-nya. Tabel dipecah
// otomatis ke halaman berikutnya bila barisnya tidak muat dalam satu halaman.
func buildRekapAbsensiPDF(
	doc *utils.PDFDoc,
	pegawai models.Pegawai,
	bulan time.Month,
	tahun int,
	baris []barisRekapPDF,
	ringkasan ringkasanRekapPDF,
) ([]byte, error) {
	const (
		marginX    = 42.0
		rowH       = 22.0
		headerH    = 18.0
		bottomY    = 781.0 // batas bawah tabel (sisakan ruang untuk footer)
		fontBaris  = 7.5
		fotoSisi   = 18.0
		yTabelHal1 = 276.0
		yTabelLain = 78.0
	)
	pageW := utils.PageWidthA4
	rightX := pageW - marginX

	// lebar kolom (total 511 = lebar area cetak A4 dengan margin 42)
	kolom := []struct {
		judul string
		lebar float64
	}{
		{"No", 24},
		{"Tanggal", 58},
		{"Hari", 40},
		{"Masuk", 36},
		{"Terlambat", 42},
		{"Pulang", 36},
		{"Status", 94},
		// judul kolom foto sengaja disingkat -- kolomnya hanya selebar
		// thumbnail, "Foto Masuk"/"Foto Pulang" akan saling bertindih
		{"Foto M.", 36},
		{"Foto P.", 36},
		{"Titik Koordinat", 109},
	}

	// berapa baris yang muat di halaman pertama & halaman berikutnya
	muatHal1 := int(math.Floor((bottomY - (yTabelHal1 + headerH)) / rowH))
	muatLain := int(math.Floor((bottomY - (yTabelLain + headerH)) / rowH))
	if muatHal1 < 1 {
		muatHal1 = 1
	}
	if muatLain < 1 {
		muatLain = 1
	}

	totalHalaman := 1
	if sisa := len(baris) - muatHal1; sisa > 0 {
		totalHalaman += (sisa + muatLain - 1) / muatLain
	}

	periode := fmt.Sprintf("%s %d", bulanIndo[int(bulan)], tahun)
	dicetak := absensiNow().Format("02-01-2006 15:04")

	// gambar satu halaman tabel
	gambarHeaderTabel := func(p *utils.PDFPage, yTop float64) float64 {
		p.SetFont(true, fontBaris)
		x := marginX
		p.Line(marginX, yTop, rightX, yTop)
		for _, k := range kolom {
			p.TextCentered(x+k.lebar/2, yTop+12, k.judul)
			x += k.lebar
		}
		p.Line(marginX, yTop+headerH, rightX, yTop+headerH)
		return yTop + headerH
	}

	gambarBaris := func(p *utils.PDFPage, yTop float64, no int, b barisRekapPDF) {
		p.SetFont(false, fontBaris)
		teksY := yTop + 14
		x := marginX

		tulisTengah := func(lebar float64, s string) {
			p.TextCentered(x+lebar/2, teksY, s)
			x += lebar
		}

		tulisTengah(kolom[0].lebar, strconv.Itoa(no))
		tulisTengah(kolom[1].lebar, b.Tanggal.Format("02-01-2006"))
		tulisTengah(kolom[2].lebar, hariIndo[b.Tanggal.Weekday()])
		tulisTengah(kolom[3].lebar, b.JamMasuk)
		tulisTengah(kolom[4].lebar, b.Terlambat)
		tulisTengah(kolom[5].lebar, b.JamPulang)
		tulisTengah(kolom[6].lebar, b.Status)

		// dua kolom foto: gambar kalau ada, kalau tidak tulis "-"
		for _, nama := range []string{b.FotoMasuk, b.FotoPulang} {
			lebar := kolom[7].lebar
			if nama != "" {
				p.Image(nama, x+(lebar-fotoSisi)/2, yTop+(rowH-fotoSisi)/2, fotoSisi, fotoSisi)
			} else {
				p.TextCentered(x+lebar/2, teksY, "-")
			}
			x += lebar
		}

		// kolom koordinat: teksnya digarisbawahi dan seluruh selnya dijadikan
		// area klik yang membuka Google Maps pada titik tersebut
		lebarKoord := kolom[9].lebar
		p.TextCentered(x+lebarKoord/2, teksY, b.Koordinat)
		if b.MapsURL != "" {
			lebarTeks := utils.TextWidth(b.Koordinat, fontBaris)
			xTeks := x + (lebarKoord-lebarTeks)/2
			p.Line(xTeks, teksY+1.5, xTeks+lebarTeks, teksY+1.5)
			p.Link(x, yTop, lebarKoord, rowH, b.MapsURL)
		}

		p.Line(marginX, yTop+rowH, rightX, yTop+rowH)
	}

	gambarFooter := func(p *utils.PDFPage, halaman int) {
		p.SetFont(false, 7.5)
		p.Text(marginX, 800, "Dicetak dari aplikasi SIICUP pada "+dicetak+" WITA")
		p.TextRight(rightX, 800, fmt.Sprintf("Halaman %d dari %d", halaman, totalHalaman))
	}

	idx := 0
	for halaman := 1; halaman <= totalHalaman; halaman++ {
		p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
		doc.AddPage(p)

		yTabel := yTabelLain
		if halaman == 1 {
			drawLetterhead(p, marginX, rightX)

			p.SetFont(true, 13)
			p.TextCentered(pageW/2, 126, "REKAP ABSENSI PEGAWAI")
			p.SetFont(false, 10)
			p.TextCentered(pageW/2, 142, "Periode "+periode)

			// identitas pegawai
			p.SetFont(false, 9.5)
			labelX := marginX
			valueX := marginX + 110
			y := 172.0
			pola := "5 Hari Kerja (Senin - Jumat)"
			if sixDayWeekForTempatTgs(pegawai.TempatTgs) {
				pola = "6 Hari Kerja (Senin - Sabtu)"
			}
			jabatan := "-"
			if pegawai.Jabatan != nil && pegawai.Jabatan.Jabatan != "" {
				jabatan = pegawai.Jabatan.Jabatan
			}
			tempat := pegawai.TempatTgs
			if strings.TrimSpace(tempat) == "" {
				tempat = "-"
			}
			for _, f := range [][2]string{
				{"Nama", pegawai.Nama},
				{"NIP", pegawai.NIP},
				{"Jabatan", jabatan},
				{"Tempat Tugas", tempat},
				{"Pola Hari Kerja", pola},
			} {
				p.Text(labelX, y, f[0])
				p.Text(valueX-8, y, ":")
				p.Text(valueX, y, f[1])
				y += 14
			}

			// ringkasan
			p.SetFont(true, 9.5)
			p.Text(marginX, 250, "Ringkasan:")
			p.SetFont(false, 9.5)
			p.Text(marginX+60, 250, fmt.Sprintf(
				"Hadir %d hari  |  Terlambat %d kali  |  Dinas Dalam %d  |  Izin %d  |  Sakit %d  |  Tidak Absen %d",
				ringkasan.Hadir, ringkasan.Terlambat, ringkasan.DinasDalam, ringkasan.Izin, ringkasan.Sakit, ringkasan.TidakAbsen,
			))
			p.SetFont(false, 7.5)
			p.Text(marginX, 265, "Titik koordinat pada tabel bisa diklik untuk membuka lokasi absen di Google Maps.")

			yTabel = yTabelHal1
		} else {
			p.SetFont(true, 9.5)
			p.Text(marginX, 60, fmt.Sprintf("Lanjutan rekap absensi %s -- %s", pegawai.Nama, periode))
		}

		y := gambarHeaderTabel(p, yTabel)
		muat := muatLain
		if halaman == 1 {
			muat = muatHal1
		}
		for i := 0; i < muat && idx < len(baris); i++ {
			gambarBaris(p, y, idx+1, baris[idx])
			y += rowH
			idx++
		}

		// garis tepi kiri/kanan tabel supaya terlihat sebagai satu kotak
		p.Line(marginX, yTabel, marginX, y)
		p.Line(rightX, yTabel, rightX, y)

		if len(baris) == 0 && halaman == 1 {
			p.SetFont(false, 9)
			p.Text(marginX+6, y+16, "Belum ada hari kerja pada periode ini.")
		}

		gambarFooter(p, halaman)
	}

	return doc.Output()
}
