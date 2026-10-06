package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// berita_acara.go: menu "Berita Acara" (administrator/admin) -- membuat
// Berita Acara resmi untuk pegawai yang tidak bisa melakukan absensi online
// lewat E-Office pada SATU tanggal tertentu, baik untuk satu pegawai saja
// ("individu") maupun beberapa pegawai sekaligus ("kolektif"), dengan
// ALASAN dipilih dari daftar TETAP (lihat AlasanBeritaAcaraOptions) --
// menggantikan alasan bawaan yang sebelumnya selalu sama di tiap berkas
// Berita Acara kertas yang diupload manual ("tidak dapat berhasil login
// Absen pagi dan sore di Aplikasi E-Office Morowali Utara").
//
// PDF-nya dibuat otomatis di server memakai penulis PDF bawaan aplikasi
// (utils/pdfwriter.go, tanpa dependensi eksternal -- sama seperti Surat
// Rekomendasi/Formulir Cuti di formulir.go & surat_rekomendasi.go), dengan
// kop surat yang SAMA dengan Lampiran 3 Surat Rekomendasi
// (drawLetterheadUnitKerjaKustom, lihat surat_rekomendasi.go) supaya
// otomatis memakai kop sekolah kustom (menu "Kop Surat Sekolah") kalau
// penandatangannya bertugas di sekolah yang sudah mengatur kopnya sendiri,
// atau jatuh ke kop Dinas bawaan kalau belum/bertugas di Dinas.
//
// Berkas yang sudah jadi DISIMPAN ke tabel yang SAMA dengan alur upload
// manual yang sudah ada (models.AbsensiDokumen, jenis "berita_acara" --
// lihat inputAbsensiDokumenKolektif di absensi_dokumen.go), supaya tanggal
// kejadian otomatis tercatat DD (Dinas Dalam) di Rekap Absen pegawai yang
// dipilih, SAMA PERSIS seperti kalau admin upload berkas BA manual di menu
// itu -- bedanya di sini berkasnya dibuat otomatis oleh sistem, bukan
// diupload. Karena menyimpan ke tabel yang sama, preview/unduh/hapus
// memakai ULANG endpoint yang SUDAH ADA (GET /api/absensi/dokumen/{id}/file,
// DELETE /api/absensi/dokumen/{id}) -- TIDAK ada endpoint baru untuk itu di
// sini, baik untuk satu baris maupun hapus kolektif (frontend cukup
// memanggil endpoint hapus itu berkali-kali, pola yang sama dengan fitur
// hapus terpilih/bulk delete di seluruh aplikasi).
//
// Satu kali "Buat Berita Acara" (individu ATAU kolektif) menghasilkan SATU
// berkas PDF yang sama, disalin ke satu baris AbsensiDokumen per pegawai
// terpilih (persis seperti pola inputAbsensiDokumenKolektif) -- nama berkas
// diberi akhiran unik (lihat generateBeritaAcaraNamaFile) supaya listBeritaAcara
// bisa mengelompokkan baris-baris itu kembali jadi satu "batch" tampilan di
// menu ini (satu kartu kolektif = N baris AbsensiDokumen dengan nama_file
// yang sama).

// AlasanBeritaAcaraOptions: SATU-SATUNYA 4 pilihan alasan yang boleh dipakai
// (tetap/fixed, BUKAN teks bebas) -- permintaan pengguna mengganti alasan
// bawaan ("tidak dapat berhasil login Absen pagi dan sore di Aplikasi
// E-Office Morowali Utara") dengan salah satu dari daftar ini. HARUS PERSIS
// SAMA dengan daftar di frontend/src/views/BeritaAcaraView.vue
// (ALASAN_OPTIONS) -- kalau salah satu diubah, ubah juga yang satunya,
// supaya validasi di sini tidak pernah menolak pilihan yang sudah
// ditampilkan di dropdown frontend.
var AlasanBeritaAcaraOptions = []string{
	"Jaringan tidak bagus",
	"Server Error",
	"Motor Rusak",
	"Banjir",
}

func isAlasanBeritaAcaraValid(alasan string) bool {
	for _, a := range AlasanBeritaAcaraOptions {
		if a == alasan {
			return true
		}
	}
	return false
}

// RegisterBeritaAcaraRoutes mendaftarkan seluruh endpoint di bawah
// /api/berita-acara*. Khusus administrator/admin (SAMA dengan hak akses
// inputAbsensiDokumenKolektif) -- bukan akun ber-flag IsAdminAbsensi saja,
// karena ini membuat dokumen resmi atas nama kantor/sekolah, bukan sekadar
// input data absen harian.
func RegisterBeritaAcaraRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	manage := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	mux.Handle("GET /api/berita-acara", manage(func(w http.ResponseWriter, r *http.Request) { listBeritaAcara(w, r, db) }))
	mux.Handle("GET /api/berita-acara/alasan-options", manage(func(w http.ResponseWriter, r *http.Request) {
		utils.Success(w, "ok", AlasanBeritaAcaraOptions)
	}))
	mux.Handle("POST /api/berita-acara", manage(func(w http.ResponseWriter, r *http.Request) { buatBeritaAcara(w, r, db) }))
}

// ============================================================
// daftar (list) -- dikelompokkan per "batch" (nama_file yang sama)
// ============================================================

type beritaAcaraPegawaiOut struct {
	// ID: id baris AbsensiDokumen pegawai ini -- dipakai frontend untuk
	// memanggil ULANG endpoint hapus satuan yang sudah ada
	// (DELETE /api/absensi/dokumen/{id}) per baris dalam batch ini.
	ID        uint   `json:"id"`
	Nama      string `json:"nama"`
	NIP       string `json:"nip"`
	Jabatan   string `json:"jabatan"`
	UnitKerja string `json:"unit_kerja"`
}

type beritaAcaraBatchOut struct {
	NamaFile  string                  `json:"nama_file"`
	Nomor     string                  `json:"nomor"`
	Tanggal   time.Time               `json:"tanggal"`
	Alasan    string                  `json:"alasan"`
	Jenis     string                  `json:"jenis"` // "individu" | "kolektif", dihitung dari jumlah pegawai
	CreatedAt time.Time               `json:"created_at"`
	Pegawai   []beritaAcaraPegawaiOut `json:"pegawai"`
}

// listBeritaAcara mengembalikan SELURUH Berita Acara yang pernah dibuat
// lewat menu ini (jenis "berita_acara" pada AbsensiDokumen -- TIDAK dibatasi
// bulan berjalan seperti listAbsensiDokumenAdmin di Rekap Absen, karena menu
// ini memang arsip/riwayat Berita Acara tersendiri), dikelompokkan per
// "batch" (baris-baris dengan nama_file yang sama = dibuat dalam satu kali
// "Buat Berita Acara" yang sama, lihat generateBeritaAcaraNamaFile) supaya
// satu Berita Acara kolektif untuk 5 pegawai tampil sebagai SATU kartu
// berisi 5 nama, bukan 5 baris terpisah.
func listBeritaAcara(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	var rows []models.AbsensiDokumen
	err := db.Omit("file").
		Where("jenis = ?", models.AbsensiDokumenBeritaAcara).
		Preload("Pegawai.UnitKerja").Preload("Pegawai.Jabatan").
		Order("created_at desc").
		Find(&rows).Error
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal mengambil data")
		return
	}

	batches := map[string]*beritaAcaraBatchOut{}
	order := []string{}
	for _, row := range rows {
		b, ok := batches[row.NamaFile]
		if !ok {
			nomor := ""
			if row.Nomor != nil {
				nomor = *row.Nomor
			}
			b = &beritaAcaraBatchOut{
				NamaFile:  row.NamaFile,
				Nomor:     nomor,
				Tanggal:   row.Tanggal,
				Alasan:    row.Keterangan,
				CreatedAt: row.CreatedAt,
			}
			batches[row.NamaFile] = b
			order = append(order, row.NamaFile)
		}
		nama, nip, jabatan, unitKerja := "-", "-", "-", "-"
		if row.Pegawai != nil {
			nama = namaOrDash(row.Pegawai.Nama)
			nip = namaOrDash(row.Pegawai.NIP)
			if row.Pegawai.Jabatan != nil {
				jabatan = namaOrDash(row.Pegawai.Jabatan.Jabatan)
			}
			if row.Pegawai.UnitKerja != nil {
				unitKerja = namaOrDash(row.Pegawai.UnitKerja.Unit)
			}
		}
		b.Pegawai = append(b.Pegawai, beritaAcaraPegawaiOut{ID: row.ID, Nama: nama, NIP: nip, Jabatan: jabatan, UnitKerja: unitKerja})
	}

	out := make([]beritaAcaraBatchOut, 0, len(order))
	for _, name := range order {
		b := batches[name]
		b.Jenis = "individu"
		if len(b.Pegawai) > 1 {
			b.Jenis = "kolektif"
		}
		if q != "" {
			matched := strings.Contains(strings.ToLower(b.Nomor), q) || strings.Contains(strings.ToLower(b.Alasan), q)
			if !matched {
				for _, pg := range b.Pegawai {
					if strings.Contains(strings.ToLower(pg.Nama), q) || strings.Contains(strings.ToLower(pg.NIP), q) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, *b)
	}
	utils.Success(w, "ok", out)
}

// ============================================================
// buat (create)
// ============================================================

type buatBeritaAcaraPayload struct {
	Jenis           string `json:"jenis"` // "individu" | "kolektif"
	IDPegawai       []uint `json:"id_pegawai"`
	IDPenandatangan uint   `json:"id_penandatangan"`
	TanggalKejadian string `json:"tanggal_kejadian"`
	TanggalSurat    string `json:"tanggal_surat"`
	NomorSurat      string `json:"nomor_surat"`
	Alasan          string `json:"alasan"`
}

// uniqueOrderedUint membuang id 0/duplikat dari idList sambil
// mempertahankan urutan kemunculan pertama -- dipakai supaya urutan baris
// pada tabel lampiran kolektif mengikuti urutan pemilihan admin di frontend,
// bukan urutan bebas hasil query SQL (yang BISA beda-beda tiap kali
// dipanggil kalau tidak diberi ORDER BY eksplisit).
func uniqueOrderedUint(ids []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func buatBeritaAcara(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)

	var payload buatBeritaAcaraPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}

	jenis := strings.ToLower(strings.TrimSpace(payload.Jenis))
	if jenis != "individu" && jenis != "kolektif" {
		utils.Error(w, http.StatusBadRequest, "jenis berita acara tidak valid -- harus \"individu\" atau \"kolektif\"")
		return
	}

	idList := uniqueOrderedUint(payload.IDPegawai)
	if len(idList) == 0 {
		utils.Error(w, http.StatusBadRequest, "pilih minimal satu pegawai")
		return
	}
	if jenis == "individu" && len(idList) != 1 {
		utils.Error(w, http.StatusBadRequest, "Berita Acara individu hanya untuk satu pegawai -- pilih \"Kolektif\" untuk lebih dari satu pegawai")
		return
	}
	if payload.IDPenandatangan == 0 {
		utils.Error(w, http.StatusBadRequest, "penandatangan (yang mengetahui) wajib dipilih")
		return
	}

	alasan := strings.TrimSpace(payload.Alasan)
	if !isAlasanBeritaAcaraValid(alasan) {
		utils.Error(w, http.StatusBadRequest, "alasan tidak valid -- pilih salah satu dari daftar yang tersedia")
		return
	}

	tglKejadian, err := utils.ParseDateCell(strings.TrimSpace(payload.TanggalKejadian))
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "tanggal kejadian tidak valid")
		return
	}
	tglSurat := tglKejadian
	if ts := strings.TrimSpace(payload.TanggalSurat); ts != "" {
		tglSurat, err = utils.ParseDateCell(ts)
		if err != nil {
			utils.Error(w, http.StatusBadRequest, "tanggal surat tidak valid")
			return
		}
	}
	nomorSurat := strings.TrimSpace(payload.NomorSurat)

	// Muat pegawai terpilih LENGKAP (Jabatan/UnitKerja/PangkatGol/Atasan,
	// lewat pegawaiPreloads yang sudah ada di pegawai.go) -- dibutuhkan
	// untuk mengisi kolom identitas pada badan surat & tabel lampiran.
	var pegawaiRows []models.Pegawai
	queryPegawai := db
	for _, pl := range pegawaiPreloads {
		queryPegawai = queryPegawai.Preload(pl)
	}
	if err := queryPegawai.Where("id IN ?", idList).Find(&pegawaiRows).Error; err != nil || len(pegawaiRows) != len(idList) {
		utils.Error(w, http.StatusBadRequest, "data pegawai yang dipilih tidak ditemukan/tidak lengkap")
		return
	}
	// urutkan sesuai urutan id_pegawai yang dikirim frontend (bukan urutan
	// bebas hasil "WHERE id IN (...)").
	pegawaiByID := map[uint]models.Pegawai{}
	for _, pg := range pegawaiRows {
		pegawaiByID[pg.ID] = pg
	}
	pegawaiTerpilih := make([]models.Pegawai, 0, len(idList))
	for _, id := range idList {
		if pg, ok := pegawaiByID[id]; ok {
			pegawaiTerpilih = append(pegawaiTerpilih, pg)
		}
	}

	var penandatangan models.Pegawai
	queryTtd := db
	for _, pl := range pegawaiPreloads {
		queryTtd = queryTtd.Preload(pl)
	}
	if err := queryTtd.First(&penandatangan, payload.IDPenandatangan).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data penandatangan tidak ditemukan")
		return
	}

	// Jenis Surat "berita_acara" HARUS sudah ada di master (seed bawaan,
	// lihat database/migrate.go) -- Nama-nya dipakai sebagai Label yang
	// tersimpan di tiap baris AbsensiDokumen, SAMA seperti alur upload
	// manual (inputAbsensiDokumenKolektif).
	var jenisSurat models.JenisSurat
	if err := db.Where("slug = ?", models.AbsensiDokumenBeritaAcara).First(&jenisSurat).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "master Jenis Surat \"Berita Acara\" tidak ditemukan -- hubungi pengembang aplikasi")
		return
	}

	// Pegawai yang pada tanggal kejadian ini SUDAH tercatat absen masuk
	// sungguhan (hadir) TIDAK ditimpa -- pengaman yang sama dengan
	// inputAbsensiDokumenKolektif, supaya kehadiran asli tidak pernah
	// tertimpa surat buatan sistem.
	var absensiRows []models.Absensi
	db.Where("id_pegawai IN ? AND tanggal = ?", idList, tglKejadian).Find(&absensiRows)
	hadirSet := map[uint]bool{}
	for _, a := range absensiRows {
		if a.JamMasuk != nil {
			hadirSet[a.IDPegawai] = true
		}
	}
	var dilewatiHadir []string
	pegawaiDiinput := make([]models.Pegawai, 0, len(pegawaiTerpilih))
	for _, pg := range pegawaiTerpilih {
		if hadirSet[pg.ID] {
			dilewatiHadir = append(dilewatiHadir, pg.Nama)
			continue
		}
		pegawaiDiinput = append(pegawaiDiinput, pg)
	}
	if len(pegawaiDiinput) == 0 {
		utils.Error(w, http.StatusBadRequest,
			"Berita Acara tidak dibuat -- seluruh pegawai yang dipilih sudah tercatat absen masuk (hadir) pada tanggal ini: "+strings.Join(dilewatiHadir, ", "))
		return
	}

	pdfBytes, err := buildBeritaAcaraPDF(jenis, pegawaiDiinput, penandatangan, tglKejadian, tglSurat, nomorSurat, alasan)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat berkas PDF: "+err.Error())
		return
	}

	namaFile := generateBeritaAcaraNamaFile(tglKejadian)
	var nomorPtr *string
	if nomorSurat != "" {
		nomorPtr = &nomorSurat
	}
	var userIDPtr *uint
	if claims != nil {
		userID := claims.UserID
		userIDPtr = &userID
	}

	for _, pg := range pegawaiDiinput {
		var existing models.AbsensiDokumen
		found := db.Where("id_pegawai = ? AND tanggal = ?", pg.ID, tglKejadian).First(&existing).Error == nil
		existing.IDPegawai = pg.ID
		existing.Tanggal = tglKejadian
		existing.Jenis = models.AbsensiDokumenBeritaAcara
		existing.Label = jenisSurat.Nama
		existing.NamaFile = namaFile
		existing.File = pdfBytes
		existing.Keterangan = alasan
		existing.Nomor = nomorPtr
		existing.IDDiinputOleh = userIDPtr
		if found {
			db.Save(&existing)
		} else {
			existing.ID = 0
			db.Create(&existing)
		}
	}

	pesan := fmt.Sprintf("Berita Acara berhasil dibuat untuk %d pegawai", len(pegawaiDiinput))
	if len(dilewatiHadir) > 0 {
		pesan += fmt.Sprintf(" -- %d pegawai dilewati karena sudah tercatat absen masuk pada tanggal ini: %s", len(dilewatiHadir), strings.Join(dilewatiHadir, ", "))
	}
	utils.Created(w, pesan, nil)
}

// generateBeritaAcaraNamaFile membuat nama berkas yang unik per "batch"
// (satu kali "Buat Berita Acara") -- dipakai listBeritaAcara untuk
// mengelompokkan baris-baris AbsensiDokumen yang dibuat bersamaan kembali
// jadi satu baris tampilan (satu kartu kolektif = N baris dengan nama_file
// yang sama ini).
func generateBeritaAcaraNamaFile(tglKejadian time.Time) string {
	return fmt.Sprintf("berita_acara_%s_%d.pdf", tglKejadian.Format("20060102"), time.Now().UnixNano())
}

// ============================================================
// pembuatan PDF
// ============================================================

// angkaSatuan: dipakai angkaKeKata sebagai basis nama bilangan 0-11 --
// bilangan di atas itu (belasan/puluhan/dst) disusun dari basis ini.
var angkaSatuan = []string{"Nol", "Satu", "Dua", "Tiga", "Empat", "Lima", "Enam", "Tujuh", "Delapan", "Sembilan", "Sepuluh", "Sebelas"}

// angkaKeKata mengeja bilangan bulat NON-NEGATIF ke kata baku Bahasa
// Indonesia (contoh: 2025 -> "Dua Ribu Dua Puluh Lima") -- dipakai
// tahunTerbilang untuk mengeja tahun kejadian pada kalimat pernyataan
// Berita Acara, SAMA seperti contoh pada kedua dokumen rujukan pengguna
// ("...Tahun 2025 (Dua Ribu Dua Puluh Lima)"). Cukup untuk bilangan sampai
// miliaran -- jauh lebih dari cukup untuk kebutuhan tahun (4 digit), tapi
// ditulis generik (rekursif per skala 1000) supaya tidak gagal aneh kalau
// suatu saat dipakai ulang untuk nilai lain.
func angkaKeKata(n int) string {
	if n < 0 {
		return "Minus " + angkaKeKata(-n)
	}
	if n <= 11 {
		return angkaSatuan[n]
	}
	if n < 20 {
		return angkaSatuan[n-10] + " Belas"
	}
	if n < 100 {
		puluh := n / 10
		sisa := n % 10
		s := angkaSatuan[puluh] + " Puluh"
		if sisa > 0 {
			s += " " + angkaKeKata(sisa)
		}
		return s
	}
	if n < 200 {
		sisa := n - 100
		s := "Seratus"
		if sisa > 0 {
			s += " " + angkaKeKata(sisa)
		}
		return s
	}
	if n < 1000 {
		ratus := n / 100
		sisa := n % 100
		s := angkaSatuan[ratus] + " Ratus"
		if sisa > 0 {
			s += " " + angkaKeKata(sisa)
		}
		return s
	}
	if n < 2000 {
		sisa := n - 1000
		s := "Seribu"
		if sisa > 0 {
			s += " " + angkaKeKata(sisa)
		}
		return s
	}
	if n < 1000000 {
		ribu := n / 1000
		sisa := n % 1000
		s := angkaKeKata(ribu) + " Ribu"
		if sisa > 0 {
			s += " " + angkaKeKata(sisa)
		}
		return s
	}
	if n < 1000000000 {
		juta := n / 1000000
		sisa := n % 1000000
		s := angkaKeKata(juta) + " Juta"
		if sisa > 0 {
			s += " " + angkaKeKata(sisa)
		}
		return s
	}
	milyar := n / 1000000000
	sisa := n % 1000000000
	s := angkaKeKata(milyar) + " Milyar"
	if sisa > 0 {
		s += " " + angkaKeKata(sisa)
	}
	return s
}

// tahunTerbilang mengeja tahun (contoh: 2025 -> "Dua Ribu Dua Puluh Lima"),
// dipakai persis seperti pada kalimat "...Tahun 2025 (Dua Ribu Dua Puluh
// Lima)" di kedua dokumen rujukan pengguna.
func tahunTerbilang(year int) string { return angkaKeKata(year) }

// drawParagraphWithBoldPhrase menggambar SATU paragraf rata kiri-kanan
// (gaya JustifiedText yang sudah dipakai di surat lain) yang terdiri dari 3
// bagian: prefix (reguler) + boldPhrase (TEBAL, SATU frasa saja) + suffix
// (reguler) -- dipakai untuk kalimat pernyataan Berita Acara, supaya alasan
// terpilih ("Jaringan tidak bagus"/dst) tercetak tebal persis seperti pada
// kedua dokumen rujukan pengguna (alasan tercetak **tebal** di tengah
// kalimat), sisanya tetap reguler.
//
// Trik yang dipakai: utils.helveticaWidth (dasar WrapText/TextWidth) SAMA
// SEKALI tidak membedakan lebar huruf tebal vs reguler (lihat catatan di
// utils/pdfwriter.go) -- jadi urutan kata hasil SATU KALI pemenggalan baris
// (WrapText) atas gabungan ketiga bagian tetap valid dipakai apa pun
// kombinasi tebal/reguler per katanya. Fungsi ini menandai setiap kata asal
// bagian mana (reguler/tebal), lalu menggambar ulang kata-per-kata sesuai
// baris hasil WrapText itu, mengganti font tepat sebelum setiap kata.
func drawParagraphWithBoldPhrase(p *utils.PDFPage, x, yTop, maxWidth, lineHeight, size float64, prefix, boldPhrase, suffix string) float64 {
	type kataTag struct {
		kata string
		bold bool
	}
	var toks []kataTag
	for _, k := range strings.Fields(prefix) {
		toks = append(toks, kataTag{k, false})
	}
	for _, k := range strings.Fields(boldPhrase) {
		toks = append(toks, kataTag{k, true})
	}
	for _, k := range strings.Fields(suffix) {
		toks = append(toks, kataTag{k, false})
	}
	full := make([]string, len(toks))
	for i, t := range toks {
		full[i] = t.kata
	}
	fullText := strings.Join(full, " ")

	lines := utils.WrapText(fullText, maxWidth, size)
	y := yTop
	idx := 0
	spaceW := utils.TextWidth(" ", size)
	for li, line := range lines {
		words := strings.Fields(line)
		n := len(words)
		// lebar efektif tiap kata -- kata yang akan dirender TEBAL dihitung
		// dengan kompensasi boldWidthSafety (lihat catatan di formulir.go:
		// utils.TextWidth hanya punya metrik Helvetica REGULER, padahal
		// glyph tebal yang SUNGGUHAN dirender pembaca PDF sedikit lebih
		// lebar) -- dipakai KONSISTEN baik untuk menghitung total lebar kata
		// (dasar celah rata kanan-kiri) maupun saat menggambar tiap kata,
		// supaya kata tebal (alasan) tidak "nempel" tanpa jarak dengan kata
		// sesudahnya (pernah terlihat pada "Server Error" + "di" yang jadi
		// "Server Errordi" di hasil cetak sebelum kompensasi ini).
		wordWidths := make([]float64, n)
		wordsWidth := 0.0
		for i, w := range words {
			bold := idx+i < len(toks) && toks[idx+i].bold
			ww := utils.TextWidth(w, size)
			if bold {
				ww /= boldWidthSafety
			}
			wordWidths[i] = ww
			wordsWidth += ww
		}
		isLast := li == len(lines)-1 || n < 2
		gap := spaceW
		if !isLast && n > 1 {
			gap = (maxWidth - wordsWidth) / float64(n-1)
		}
		cx := x
		for i, w := range words {
			bold := false
			if idx < len(toks) {
				bold = toks[idx].bold
			}
			p.SetFont(bold, size)
			p.Text(cx, y, w)
			cx += wordWidths[i] + gap
			idx++
		}
		y += lineHeight
	}
	p.SetFont(false, size)
	return y
}

// beritaAcaraKolom/drawBeritaAcaraTable: tabel lampiran daftar pegawai pada
// Berita Acara Kolektif (halaman 2) -- gaya & cara gambar SAMA PERSIS
// dengan drawKp4Table di kp4_pdf.go (header tebal dibungkus multi-baris
// kalau perlu, baris data dipotong satu baris karena isinya singkat/diskrit
// seperti nama/NIP, bukan paragraf).
type beritaAcaraKolom struct {
	Judul string
	Lebar float64
}

func drawBeritaAcaraTable(p *utils.PDFPage, x, yTop float64, kolom []beritaAcaraKolom, rows [][]string) float64 {
	const headerFont = 9.5
	const bodyFont = 9.5
	const pad = 4.0

	p.SetFont(true, headerFont)
	headerLineH := headerFont + 3
	maxHeaderLines := 1
	headerLines := make([][]string, len(kolom))
	for i, k := range kolom {
		lines := utils.WrapText(k.Judul, (k.Lebar-2*pad)*boldWidthSafety, headerFont)
		if len(lines) == 0 {
			lines = []string{""}
		}
		headerLines[i] = lines
		if len(lines) > maxHeaderLines {
			maxHeaderLines = len(lines)
		}
	}
	headerH := float64(maxHeaderLines)*headerLineH + 2*pad

	cx := x
	for i, k := range kolom {
		p.Rect(cx, yTop, k.Lebar, headerH)
		ty := yTop + pad + headerFont
		for _, ln := range headerLines[i] {
			p.TextCentered(cx+k.Lebar/2, ty, ln)
			ty += headerLineH
		}
		cx += k.Lebar
	}
	y := yTop + headerH

	p.SetFont(false, bodyFont)
	rowH := bodyFont + 2*pad + 2
	for _, row := range rows {
		cx = x
		for i, k := range kolom {
			p.Rect(cx, y, k.Lebar, rowH)
			val := "-"
			if i < len(row) && row[i] != "" {
				val = row[i]
			}
			// truncateToWidth (BUKAN WrapText+ambil baris pertama) supaya
			// nilai yang kepanjangan diberi "..." eksplisit -- pengguna
			// langsung tahu ada bagian yang terpotong, bukan diam-diam
			// hilang (sempat terlihat pada "Penata Muda / III/a" yang
			// terpotong jadi "Penata Muda /" tanpa tanda apa pun saat kolom
			// Pangkat/Gol. masih terlalu sempit).
			display := truncateToWidth(val, k.Lebar-2*pad, bodyFont)
			if i == 0 {
				p.TextCentered(cx+k.Lebar/2, y+pad+bodyFont-1, display)
			} else {
				p.Text(cx+pad, y+pad+bodyFont-1, display)
			}
			cx += k.Lebar
		}
		y += rowH
	}
	return y
}

// pegawaiPangkatGolText/pegawaiJabatanText/pegawaiUnitKerjaText: ekstrak
// field identitas yang butuh preload (Jabatan/UnitKerja/PangkatGol.*) dari
// models.Pegawai secara aman (nil-safe) -- dipakai berulang baik untuk
// identitas penandatangan maupun identitas pegawai/baris lampiran.
func pegawaiPangkatGolText(pg models.Pegawai) string {
	if pg.PangkatGol == nil {
		return "-"
	}
	pk, gol := "-", "-"
	if pg.PangkatGol.Pangkat != nil {
		pk = pg.PangkatGol.Pangkat.Pangkat
	}
	if pg.PangkatGol.Gol != nil {
		gol = pg.PangkatGol.Gol.Gol
	}
	return pk + " / " + gol
}

func pegawaiJabatanText(pg models.Pegawai) string {
	if pg.Jabatan == nil {
		return "-"
	}
	return namaOrDash(pg.Jabatan.Jabatan)
}

func pegawaiUnitKerjaText(pg models.Pegawai) string {
	if pg.UnitKerja == nil {
		return "-"
	}
	return namaOrDash(pg.UnitKerja.Unit)
}

// buildBeritaAcaraPDF menggambar PDF Berita Acara -- SATU halaman untuk
// jenis "individu" (identitas yang ditampilkan di badan surat adalah
// PEGAWAI yang bersangkutan, mengikuti BA_PERORANG.docx), DUA halaman untuk
// "kolektif" (halaman 1 identitas yang ditampilkan adalah PENANDATANGAN
// sendiri + kalimat pernyataan generik, halaman 2 lampiran tabel seluruh
// pegawai, mengikuti BERITA_ACARA_KOLEKTIF.docx).
//
// Kop surat (drawLetterheadUnitKerjaKustom, SAMA dipakai Lampiran 3 Surat
// Rekomendasi) SELALU memakai Unit Kerja milik PENANDATANGAN -- bukan Unit
// Kerja pegawai -- supaya Berita Acara Kolektif yang mencampur pegawai dari
// beberapa unit kerja (sesuai keputusan pengguna) tetap punya SATU kop yang
// konsisten, yaitu kop kantor/sekolah tempat penandatangan bertugas &
// menandatangani surat ini.
func buildBeritaAcaraPDF(jenis string, pegawaiList []models.Pegawai, penandatangan models.Pegawai, tglKejadian, tglSurat time.Time, nomorSurat, alasan string) ([]byte, error) {
	doc := utils.NewPDFDoc()

	marginX := 48.0
	pageW := utils.PageWidthA4
	rightX := pageW - marginX
	centerX := marginX + (rightX-marginX)/2
	const lineH = 15.0
	const labelW = 110.0

	unitKerjaTtdNama := "-"
	var unitKerjaTtdForKop *models.UnitKerja
	if penandatangan.UnitKerja != nil {
		unitKerjaTtdNama = penandatangan.UnitKerja.Unit
		unitKerjaTtdForKop = penandatangan.UnitKerja
	}

	// kalimat pernyataan (sama untuk individu & kolektif, hanya subjeknya yang
	// berbeda -- lihat statementPrefix per jenis di bawah), alasan terpilih
	// dicetak TEBAL di tengah kalimat (lihat drawParagraphWithBoldPhrase).
	hari := hariIndo[tglKejadian.Weekday()]
	statementSuffix := fmt.Sprintf(
		" di Aplikasi E-Office Morowali Utara.",
	)
	statementTengah := fmt.Sprintf(
		"Pada hari %s tanggal %d Bulan %s Tahun %d (%s) tidak dapat melakukan absensi online melalui E-Office dikarenakan ",
		hari, tglKejadian.Day(), bulanIndo[int(tglKejadian.Month())], tglKejadian.Year(), tahunTerbilang(tglKejadian.Year()),
	)

	sanksi := "Apabila hal ini di atas tidak benar, maka saya siap menerima sanksi sesuai peraturan yang berlaku."
	penutup := "Demikian Berita Acara ini dibuat dengan sebenarnya untuk dapat dipergunakan sebagaimana mestinya."

	drawJudulDanNomor := func(p *utils.PDFPage, yStart float64) float64 {
		y := yStart
		p.SetFont(true, 13)
		const judul = "BERITA ACARA"
		p.TextCentered(centerX, y, judul)
		titleW := utils.TextWidth(judul, 13)
		p.Line(centerX-titleW/2, y+3, centerX+titleW/2, y+3)
		y += lineH * 1.6
		p.SetFont(false, 12)
		nomorText := nomorSurat
		if strings.TrimSpace(nomorText) == "" {
			nomorText = "-"
		}
		p.TextCentered(centerX, y, "NO : "+nomorText)
		y += lineH * 1.8
		return y
	}

	drawTtdBlock := func(p *utils.PDFPage, yStart float64) float64 {
		y := yStart
		p.SetFont(false, 12)
		p.Text(marginX, y, "Kolonodale, "+formatDateID(tglSurat)+".")
		y += lineH * 1.4
		p.Text(marginX, y, "Mengetahui,")
		y += lineH
		jabatanLines, jabatanSize := wrapJabatan(namaOrDash(pegawaiJabatanText(penandatangan)), rightX-marginX, 2, []float64{12, 11, 10.5, 10, 9.5, 9})
		for _, jl := range jabatanLines {
			p.SetFont(false, jabatanSize)
			p.Text(marginX, y, jl)
			y += jabatanSize + 2
		}
		y += lineH * 2.4
		namaDisp := strings.ToUpper(namaOrDash(penandatangan.Nama))
		p.SetFont(true, 12)
		p.Text(marginX, y, namaDisp)
		nameW := utils.TextWidth(namaDisp, 12)
		p.Line(marginX, y+3, marginX+nameW, y+3)
		y += 16
		p.SetFont(false, 12)
		p.Text(marginX, y, "NIP. "+namaOrDash(penandatangan.NIP))
		y += lineH
		return y
	}

	if jenis == "individu" {
		pegawai := pegawaiList[0]
		p := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
		doc.AddPage(p)

		kopBawahY := drawLetterheadUnitKerjaKustom(doc, p, marginX, rightX, unitKerjaTtdNama, unitKerjaTtdForKop)
		y := kopBawahY + 16
		y = drawJudulDanNomor(p, y)

		p.SetFont(false, 12)
		ttdJabatan := "Kepala Sekolah"
		if j := pegawaiJabatanText(penandatangan); j != "-" {
			ttdJabatan = j
		}
		intro := fmt.Sprintf("Yang bertanda tangan di bawah ini %s %s, dengan ini menyatakan bahwa Pegawai atas nama :", ttdJabatan, namaOrDash(unitKerjaTtdNama))
		y = p.MultilineText(marginX, y, rightX-marginX, lineH, intro) + lineH*0.5

		drawField := func(label, val string) {
			p.SetFont(false, 12)
			p.Text(marginX, y, label)
			p.Text(marginX+labelW, y, ": "+namaOrDash(val))
			y += lineH
		}
		drawField("Nama", strings.ToUpper(namaOrDash(pegawai.Nama)))
		drawField("NIP", pegawai.NIP)
		drawField("Pangkat/Gol.", pegawaiPangkatGolText(pegawai))
		drawField("Jabatan", pegawaiJabatanText(pegawai))
		drawField("Unit Kerja", pegawaiUnitKerjaText(pegawai))
		y += lineH * 0.6

		y = drawParagraphWithBoldPhrase(p, marginX, y, rightX-marginX, lineH, 12, statementTengah, alasan, statementSuffix) + lineH*0.4
		y = p.JustifiedText(marginX, y, rightX-marginX, lineH, sanksi) + lineH*0.4
		y = p.JustifiedText(marginX, y, rightX-marginX, lineH, penutup) + lineH*1.8

		drawTtdBlock(p, y)

		return doc.Output()
	}

	// ---------------- kolektif: halaman 1 ----------------
	p1 := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p1)
	kopBawahY := drawLetterheadUnitKerjaKustom(doc, p1, marginX, rightX, unitKerjaTtdNama, unitKerjaTtdForKop)
	y := kopBawahY + 16
	y = drawJudulDanNomor(p1, y)

	p1.Text(marginX, y, "Yang bertanda tangan di bawah ini :")
	y += lineH
	drawField1 := func(label, val string) {
		p1.SetFont(false, 12)
		p1.Text(marginX, y, label)
		p1.Text(marginX+labelW, y, ": "+namaOrDash(val))
		y += lineH
	}
	drawField1("Nama", strings.ToUpper(namaOrDash(penandatangan.Nama)))
	drawField1("NIP", penandatangan.NIP)
	drawField1("Pangkat/Gol.", pegawaiPangkatGolText(penandatangan))
	drawField1("Jabatan", pegawaiJabatanText(penandatangan))
	drawField1("Unit Kerja", unitKerjaTtdNama)
	y += lineH * 0.6

	p1.SetFont(false, 12)
	p1.Text(marginX, y, "Dengan ini menyatakan bahwa :")
	y += lineH * 1.2

	statementKolektifTengah := fmt.Sprintf(
		"Pegawai-pegawai yang namanya tercantum dalam lampiran surat ini, pada hari %s tanggal %d Bulan %s Tahun %d (%s) tidak dapat melakukan absensi online melalui E-Office dikarenakan ",
		hari, tglKejadian.Day(), bulanIndo[int(tglKejadian.Month())], tglKejadian.Year(), tahunTerbilang(tglKejadian.Year()),
	)
	y = drawParagraphWithBoldPhrase(p1, marginX, y, rightX-marginX, lineH, 12, statementKolektifTengah, alasan, statementSuffix) + lineH*0.4
	y = p1.JustifiedText(marginX, y, rightX-marginX, lineH, sanksi) + lineH*0.4
	y = p1.JustifiedText(marginX, y, rightX-marginX, lineH, penutup) + lineH*1.8

	drawTtdBlock(p1, y)

	// ---------------- kolektif: halaman 2 (lampiran) ----------------
	p2 := utils.NewPDFPage(utils.PageWidthA4, utils.PageHeightA4)
	doc.AddPage(p2)
	y2 := 48.0
	p2.SetFont(true, 12)
	judulLampiran := fmt.Sprintf("Lampiran Daftar Nama Pegawai Tidak Dapat Berhasil Login Absen Pagi dan Sore di Aplikasi E-Office Dikarenakan %s", alasan)
	y2 = p2.MultilineText(marginX, y2, rightX-marginX, lineH, judulLampiran) + lineH*0.6

	p2.SetFont(false, 12)
	nomorText := nomorSurat
	if strings.TrimSpace(nomorText) == "" {
		nomorText = "-"
	}
	p2.Text(marginX, y2, "Nomor  : "+nomorText)
	y2 += lineH
	p2.Text(marginX, y2, "Tanggal : "+formatDateID(tglSurat))
	y2 += lineH * 1.4

	tableW := rightX - marginX
	const tableFont = 9.5
	const tablePad = 4.0
	noW := 28.0
	// lebar kolom NIP dihitung PASTI cukup untuk NIP 18 digit penuh (BUKAN
	// persentase tetap seperti kolom lain) -- NIP adalah data identitas yang
	// TIDAK BOLEH terpotong "..." sama sekali (beda dengan Nama/Jabatan yang
	// masih bisa dipotong kalau kepanjangan). +6 ekstra jaga-jaga spasi antar
	// karakter dari perkiraan lebar yang dipakai TextWidth.
	nipW := utils.TextWidth(strings.Repeat("0", 18), tableFont) + 2*tablePad + 6
	sisaW := tableW - noW - nipW
	kolom := []beritaAcaraKolom{
		{Judul: "No", Lebar: noW},
		{Judul: "Nama", Lebar: sisaW * 0.36},
		{Judul: "NIP", Lebar: nipW},
		{Judul: "Pangkat/Gol.", Lebar: sisaW * 0.30},
		{Judul: "Jabatan", Lebar: 0}, // diisi di bawah supaya total == tableW
	}
	usedW := 0.0
	for _, k := range kolom[:len(kolom)-1] {
		usedW += k.Lebar
	}
	kolom[len(kolom)-1].Lebar = tableW - usedW

	rows := make([][]string, 0, len(pegawaiList))
	for i, pg := range pegawaiList {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			strings.ToUpper(namaOrDash(pg.Nama)),
			namaOrDash(pg.NIP),
			pegawaiPangkatGolText(pg),
			pegawaiJabatanText(pg),
		})
	}
	y2 = drawBeritaAcaraTable(p2, marginX, y2, kolom, rows) + lineH*1.8

	drawTtdBlock(p2, y2)

	return doc.Output()
}
