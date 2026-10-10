package handlers

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// peta_jabatan_kenaikan_pangkat.go menangani pengajuan & persetujuan
// Kenaikan Pangkat (tahap PERTAMA dari dua tahap, lihat komentar alur di
// models.go bagian "PETA JABATAN -- SEKOLAH"): pegawai yang sudah lulus
// UKOM mengajukan naik dari Jabatan asal ke Jabatan tujuan, atasan (Kepala
// Sekolah) menyetujui tahap pertama (K bertambah otomatis + memilih
// SubJabatan tujuan kalau ada), administrator (mewakili Kepala Dinas)
// menyetujui tahap akhir (dokumen final siap cetak + memicu pembuatan
// PerubahanJabatanPegawai, tahap KEDUA -- lihat
// handlers/peta_jabatan_perubahan.go).

func RegisterPetaJabatanKenaikanPangkatRoutes(mux *http.ServeMux, db *gorm.DB) {
	authed := func(h http.HandlerFunc, roles ...string) http.Handler {
		return middleware.Chain(h, middleware.Auth, middleware.RequireActiveUser(db), middleware.RequireRole(roles...))
	}
	anyRole := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin", "atasan", "pegawai") }
	pegawaiAtasan := func(h http.HandlerFunc) http.Handler { return authed(h, "pegawai", "atasan") }
	atasanOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "atasan") }
	adminOnly := func(h http.HandlerFunc) http.Handler { return authed(h, "administrator", "admin") }

	mux.Handle("GET /api/peta-jabatan/kenaikan-pangkat", anyRole(func(w http.ResponseWriter, r *http.Request) { listKenaikanPangkat(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/kenaikan-pangkat/{id}", anyRole(func(w http.ResponseWriter, r *http.Request) { getKenaikanPangkatDetail(w, r, db) }))
	mux.Handle("POST /api/peta-jabatan/kenaikan-pangkat", pegawaiAtasan(func(w http.ResponseWriter, r *http.Request) { createKenaikanPangkat(w, r, db) }))
	mux.Handle("DELETE /api/peta-jabatan/kenaikan-pangkat/{id}", pegawaiAtasan(func(w http.ResponseWriter, r *http.Request) { batalkanKenaikanPangkat(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/kenaikan-pangkat/{id}/ukom", anyRole(func(w http.ResponseWriter, r *http.Request) { downloadUkomKenaikanPangkat(w, r, db) }))
	mux.Handle("GET /api/peta-jabatan/kenaikan-pangkat/{id}/cetak", anyRole(func(w http.ResponseWriter, r *http.Request) { cetakKenaikanPangkat(w, r, db) }))

	mux.Handle("PUT /api/peta-jabatan/kenaikan-pangkat/{id}/setujui-atasan", atasanOnly(func(w http.ResponseWriter, r *http.Request) { setujuiKenaikanPangkatAtasan(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/kenaikan-pangkat/{id}/kembalikan-atasan", atasanOnly(func(w http.ResponseWriter, r *http.Request) { kembalikanKenaikanPangkatAtasan(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/kenaikan-pangkat/{id}/setujui-admin", adminOnly(func(w http.ResponseWriter, r *http.Request) { setujuiKenaikanPangkatAdmin(w, r, db) }))
	mux.Handle("PUT /api/peta-jabatan/kenaikan-pangkat/{id}/kembalikan-admin", adminOnly(func(w http.ResponseWriter, r *http.Request) { kembalikanKenaikanPangkatAdmin(w, r, db) }))
}

func kenaikanPangkatPreload(db *gorm.DB) *gorm.DB {
	omitDokumen := func(tx *gorm.DB) *gorm.DB { return tx.Omit(dokumenFileFields...) }
	return db.Omit("ukom_file", "file").
		Preload("Pegawai", omitDokumen).Preload("Pegawai.Jabatan").Preload("Pegawai.UnitKerja").Preload("Pegawai.PangkatGol.Pangkat").Preload("Pegawai.PangkatGol.Gol").
		Preload("UnitKerja").
		Preload("JabatanAsal").Preload("SubJabatanAsal").
		Preload("JabatanTujuan").Preload("SubJabatanTujuan").
		Preload("AtasanApprove", omitDokumen).Preload("AdminApprove")
}

func canAccessKenaikanPangkat(claims *utils.Claims, item models.PengajuanKenaikanPangkat, db *gorm.DB) bool {
	switch claims.RoleName {
	case "administrator", "admin":
		return true
	case "atasan":
		return canManagePetaJabatanSekolah(claims, item.IDUnitKerja, db)
	case "pegawai":
		return claims.IDPegawai != nil && item.IDPegawai == *claims.IDPegawai
	}
	return false
}

func listKenaikanPangkat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	query := kenaikanPangkatPreload(db)
	switch claims.RoleName {
	case "pegawai":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.PengajuanKenaikanPangkat{})
			return
		}
		query = query.Where("id_pegawai = ?", *claims.IDPegawai)
	case "atasan":
		if claims.IDPegawai == nil {
			utils.Success(w, "ok", []models.PengajuanKenaikanPangkat{})
			return
		}
		var pegawai models.Pegawai
		if err := db.Select("id_unit_kerja").First(&pegawai, "id = ?", *claims.IDPegawai).Error; err != nil || pegawai.IDUnitKerja == nil {
			utils.Success(w, "ok", []models.PengajuanKenaikanPangkat{})
			return
		}
		query = query.Where("id_unit_kerja = ?", *pegawai.IDUnitKerja)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" && status != "semua" {
		query = query.Where("status = ?", status)
	}
	var items []models.PengajuanKenaikanPangkat
	query.Order("created_at desc").Find(&items)
	utils.Success(w, "ok", items)
}

func getKenaikanPangkatDetail(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := kenaikanPangkatPreload(db).First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if !canAccessKenaikanPangkat(claims, item, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	utils.Success(w, "ok", item)
}

// createKenaikanPangkat menangani POST /api/peta-jabatan/kenaikan-pangkat --
// pegawai/atasan mengajukan kenaikan pangkat UNTUK DIRI SENDIRI (sama pola
// dengan Pengajuan Pensiun/Perubahan Data: atasan juga punya Jabatan &
// boleh naik pangkat seperti pegawai biasa). Menerima multipart/form-data:
// "id_jabatan_tujuan" (wajib), "alasan" (opsional), "file" berisi bukti
// lulus UKOM (wajib). Jabatan/SubJabatan ASAL diambil otomatis dari data
// pegawai SAAT INI.
func createKenaikanPangkat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	if claims.IDPegawai == nil {
		utils.Error(w, http.StatusBadRequest, "akun anda belum terhubung dengan data pegawai")
		return
	}
	utils.LimitBody(w, r, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca data form (maksimal total 15MB)")
		return
	}

	var pegawai models.Pegawai
	if err := db.First(&pegawai, *claims.IDPegawai).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "data pegawai tidak ditemukan")
		return
	}
	if pegawai.IDJabatan == nil || pegawai.IDUnitKerja == nil {
		utils.Error(w, http.StatusBadRequest, "data jabatan/unit kerja anda belum lengkap, hubungi administrator")
		return
	}

	idJabatanTujuan, err := strconv.ParseUint(strings.TrimSpace(r.FormValue("id_jabatan_tujuan")), 10, 64)
	if err != nil || idJabatanTujuan == 0 {
		utils.Error(w, http.StatusBadRequest, "jabatan tujuan wajib dipilih")
		return
	}
	if uint(idJabatanTujuan) == *pegawai.IDJabatan {
		utils.Error(w, http.StatusBadRequest, "jabatan tujuan harus berbeda dari jabatan anda saat ini")
		return
	}
	var jabatanTujuan models.Jabatan
	if err := db.First(&jabatanTujuan, idJabatanTujuan).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "jabatan tujuan tidak ditemukan")
		return
	}

	var existingPending models.PengajuanKenaikanPangkat
	if err := db.Where("id_pegawai = ? AND status NOT IN ?", *claims.IDPegawai,
		[]string{models.KenaikanPangkatDisetujui, models.KenaikanPangkatDitolakAtasan, models.KenaikanPangkatDitolakAdmin}).
		First(&existingPending).Error; err == nil {
		utils.Error(w, http.StatusBadRequest, "anda masih memiliki pengajuan kenaikan pangkat yang sedang berjalan")
		return
	}

	alasan := strings.TrimSpace(r.FormValue("alasan"))

	fh := formFileHeader(r, "file")
	if fh == nil {
		utils.Error(w, http.StatusBadRequest, "berkas bukti lulus UKOM wajib diupload")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		utils.Error(w, http.StatusBadRequest, "berkas harus berformat PDF, JPG, atau PNG")
		return
	}
	f, err := fh.Open()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal membaca berkas")
		return
	}
	if len(data) == 0 {
		utils.Error(w, http.StatusBadRequest, "berkas yang dipilih kosong (0 byte) -- coba buka dulu berkasnya lalu pilih ulang")
		return
	}

	item := models.PengajuanKenaikanPangkat{
		IDPegawai:        *claims.IDPegawai,
		IDUnitKerja:      *pegawai.IDUnitKerja,
		IDJabatanAsal:    *pegawai.IDJabatan,
		IDSubJabatanAsal: pegawai.IDSubJabatan,
		IDJabatanTujuan:  uint(idJabatanTujuan),
		UkomNamaFile:     fh.Filename,
		UkomFile:         data,
		Alasan:           alasan,
		Status:           models.KenaikanPangkatMenungguAtasan,
	}
	if err := db.Create(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan pengajuan: "+err.Error())
		return
	}
	kenaikanPangkatPreload(db).First(&item, item.ID)
	utils.Created(w, "pengajuan kenaikan pangkat berhasil dikirim, menunggu persetujuan atasan", item)
}

// batalkanKenaikanPangkat: pegawai/atasan BISA membatalkan pengajuannya
// SENDIRI HANYA selama masih "menunggu_atasan" (belum ada keputusan apa
// pun) -- sama pola pembatasan dengan pembatalan pengajuan lain di sistem
// ini (mis. Pengajuan Cuti) supaya tidak ada pengajuan yang tiba-tiba
// hilang setelah mulai diproses.
func batalkanKenaikanPangkat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if claims.IDPegawai == nil || item.IDPegawai != *claims.IDPegawai {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke pengajuan ini")
		return
	}
	if item.Status != models.KenaikanPangkatMenungguAtasan {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses, tidak bisa dibatalkan sendiri lagi")
		return
	}
	if err := db.Delete(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membatalkan pengajuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan kenaikan pangkat berhasil dibatalkan", nil)
}

func downloadUkomKenaikanPangkat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.Select("id", "id_pegawai", "id_unit_kerja", "ukom_nama_file", "ukom_file").First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if !canAccessKenaikanPangkat(claims, item, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if len(item.UkomFile) == 0 {
		utils.Error(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	if r.URL.Query().Get("inline") == "1" {
		w.Header().Set("Content-Type", dokumenContentType(item.UkomNamaFile))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", item.UkomNamaFile))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.UkomNamaFile))
	}
	w.Write(item.UkomFile)
}

// ============================================================
// persetujuan tahap 1: ATASAN (Kepala Sekolah)
// ============================================================

// setujuiKenaikanPangkatAtasan menangani PUT
// /api/peta-jabatan/kenaikan-pangkat/{id}/setujui-atasan -- atasan
// (Kepala Sekolah tempat pegawai itu bertugas, dicek lewat
// canManagePetaJabatanSekolah) menyetujui tahap pertama. Menerima form
// opsional "id_sub_jabatan_tujuan" (kalau Jabatan tujuan sudah dipecah jadi
// beberapa SubJabatan di sekolah ini, atasan WAJIB memilih salah satu
// supaya K bertambah pada baris yang tepat) & "catatan" (opsional).
//
// K (Kebutuhan) pada Jabatan/SubJabatan TUJUAN otomatis bertambah 1 di
// sini (permintaan pengguna: "ketika atasan menyetujui maka otomatis ...
// akan memiliki formasi sesuai kebutuhan pegawai") -- Jabatan/SubJabatan
// ASAL TIDAK disentuh sama sekali, "B" pegawai juga BELUM berubah (masih
// dihitung di jabatan lama sampai PerubahanJabatanPegawai disetujui
// terpisah).
func setujuiKenaikanPangkatAtasan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if !canManagePetaJabatanSekolah(claims, item.IDUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda bukan atasan/Kepala Sekolah dari pegawai ini")
		return
	}
	if item.Status != models.KenaikanPangkatMenungguAtasan {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}

	r.ParseForm()
	var idSubTujuan *uint
	if raw := strings.TrimSpace(r.FormValue("id_sub_jabatan_tujuan")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			utils.Error(w, http.StatusBadRequest, "sub-jabatan tujuan tidak valid")
			return
		}
		var sub models.SubJabatan
		if err := db.First(&sub, v).Error; err != nil || sub.IDJabatan != item.IDJabatanTujuan || sub.IDUnitKerja != item.IDUnitKerja {
			utils.Error(w, http.StatusBadRequest, "sub-jabatan tujuan tidak valid untuk jabatan/sekolah ini")
			return
		}
		id64 := uint(v)
		idSubTujuan = &id64
	} else {
		var ada int64
		db.Model(&models.SubJabatan{}).Where("id_unit_kerja = ? AND id_jabatan = ?", item.IDUnitKerja, item.IDJabatanTujuan).Count(&ada)
		if ada > 0 {
			utils.Error(w, http.StatusBadRequest, "jabatan tujuan sudah dipecah menjadi beberapa sub-jabatan di sekolah ini, pilih salah satu sub-jabatan tujuan")
			return
		}
	}

	now := absensiNow()
	item.IDSubJabatanTujuan = idSubTujuan
	item.Status = models.KenaikanPangkatMenungguAdmin
	item.IDAtasanApprove = claims.IDPegawai
	item.TglAtasanApprove = &now
	item.CatatanAtasan = strings.TrimSpace(r.FormValue("catatan"))
	if _, err := upsertFormasiJabatan(db, item.IDUnitKerja, item.IDJabatanTujuan, idSubTujuan, func(lama int) int { return lama + 1 }); err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menambah kebutuhan (K) jabatan tujuan: "+err.Error())
		return
	}
	if item.NomorSurat == nil {
		nomor := fmt.Sprintf("%d/PTJ/%s/%d", item.ID, romanMonth(now.Month()), now.Year())
		item.NomorSurat = &nomor
	}
	if pdfBytes, err := buildKenaikanPangkatPDF(db, item); err == nil {
		item.File = pdfBytes
	}
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan persetujuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan disetujui, kebutuhan (K) jabatan tujuan bertambah, diteruskan ke administrator untuk persetujuan tahap akhir", nil)
}

func kembalikanKenaikanPangkatAtasan(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if !canManagePetaJabatanSekolah(claims, item.IDUnitKerja, db) {
		utils.Error(w, http.StatusForbidden, "anda bukan atasan/Kepala Sekolah dari pegawai ini")
		return
	}
	if item.Status != models.KenaikanPangkatMenungguAtasan {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya (status: "+item.Status+")")
		return
	}
	r.ParseForm()
	catatan := strings.TrimSpace(r.FormValue("catatan"))
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya pegawai tahu apa yang perlu diperbaiki")
		return
	}
	now := absensiNow()
	item.Status = models.KenaikanPangkatDitolakAtasan
	item.CatatanAtasan = catatan
	item.IDAtasanApprove = claims.IDPegawai
	item.TglAtasanApprove = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan dikembalikan/ditolak", nil)
}

// ============================================================
// persetujuan tahap 2: ADMINISTRATOR (mewakili Kepala Dinas)
// ============================================================

// setujuiKenaikanPangkatAdmin menangani PUT
// /api/peta-jabatan/kenaikan-pangkat/{id}/setujui-admin -- tahap akhir.
// Dokumen PDF digenerate ulang DENGAN QR tanda tangan Kepala Dinas (QR
// Kepala Sekolah sudah tampil sejak tahap atasan), DAN baris
// PerubahanJabatanPegawai otomatis dibuat (status "menunggu_upload") --
// inilah yang memicu peringatan pada akun pegawai untuk mengupload SK
// jabatan/pangkat barunya (lihat handlers/peta_jabatan_perubahan.go).
func setujuiKenaikanPangkatAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Status != models.KenaikanPangkatMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya, atau belum disetujui atasan (status: "+item.Status+")")
		return
	}

	now := absensiNow()
	item.Status = models.KenaikanPangkatDisetujui
	item.IDAdminApprove = &claims.UserID
	item.TglAdminApprove = &now
	r.ParseForm()
	item.CatatanAdmin = strings.TrimSpace(r.FormValue("catatan"))
	if pdfBytes, err := buildKenaikanPangkatPDF(db, item); err == nil {
		item.File = pdfBytes
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		perubahan := models.PerubahanJabatanPegawai{
			IDPengajuanKenaikanPangkat: item.ID,
			IDPegawai:                  item.IDPegawai,
			IDJabatanBaru:              item.IDJabatanTujuan,
			IDSubJabatanBaru:           item.IDSubJabatanTujuan,
			Status:                     models.PerubahanJabatanMenungguUpload,
		}
		return tx.Create(&perubahan).Error
	})
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan persetujuan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan kenaikan pangkat disetujui, dokumen final siap dicetak. Peringatan perubahan jabatan (wajib upload SK) sudah muncul pada akun pegawai", nil)
}

func kembalikanKenaikanPangkatAdmin(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if item.Status != models.KenaikanPangkatMenungguAdmin {
		utils.Error(w, http.StatusBadRequest, "pengajuan ini sudah diproses sebelumnya, atau belum disetujui atasan (status: "+item.Status+")")
		return
	}
	r.ParseForm()
	catatan := strings.TrimSpace(r.FormValue("catatan"))
	if catatan == "" {
		utils.Error(w, http.StatusBadRequest, "catatan wajib diisi supaya diketahui apa yang perlu diperbaiki")
		return
	}
	now := absensiNow()
	// Dikembalikan/ditolak di tahap admin TIDAK mengurangi lagi K yang sudah
	// bertambah di tahap atasan -- SENGAJA, karena K adalah formasi yang
	// dibutuhkan sekolah (independen dari status pengajuan pegawai
	// tertentu); kalau formasi itu memang tidak lagi relevan, atasan/
	// administrator tetap bisa menguranginya manual lewat dialog "Atur
	// Kebutuhan" di tabel Peta Jabatan.
	item.Status = models.KenaikanPangkatDitolakAdmin
	item.CatatanAdmin = catatan
	item.IDAdminApprove = &claims.UserID
	item.TglAdminApprove = &now
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal menyimpan: "+err.Error())
		return
	}
	utils.Success(w, "pengajuan dikembalikan/ditolak", nil)
}

func cetakKenaikanPangkat(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	claims, _ := middleware.GetClaims(r)
	id := r.PathValue("id")
	var item models.PengajuanKenaikanPangkat
	if err := db.First(&item, "id = ?", id).Error; err != nil {
		utils.Error(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if !canAccessKenaikanPangkat(claims, item, db) {
		utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses ke data ini")
		return
	}
	if item.Status == models.KenaikanPangkatMenungguAtasan || len(item.File) == 0 {
		utils.Error(w, http.StatusBadRequest, "dokumen belum bisa dicetak, menunggu persetujuan atasan terlebih dahulu")
		return
	}
	writePDFResponse(w, r, item.File, fmt.Sprintf("peta_jabatan_kenaikan_pangkat_%d.pdf", item.ID))
}
