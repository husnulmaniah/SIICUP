package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

// absensi_admin.go: bagian khusus admin/administrator untuk menu Absen --
// ubah pengaturan (aktif/nonaktif & jam), dan rekap semua pegawai (tampilan
// & export Excel). Rekap sengaja HANYA bisa diakses admin/administrator
// (lihat RegisterAbsensiRoutes di absensi.go), tidak atasan.

type pengaturanAbsensiPayload struct {
	Aktif          bool   `json:"aktif"`
	JamMulaiPagi   string `json:"jam_mulai_pagi"`
	JamBatasPagi   string `json:"jam_batas_pagi"`
	JamMulaiPulang string `json:"jam_mulai_pulang"`
}

func updatePengaturanAbsensi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	var p pengaturanAbsensiPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		utils.Error(w, http.StatusBadRequest, "format data tidak valid")
		return
	}
	for label, v := range map[string]string{
		"jam mulai absen pagi":   p.JamMulaiPagi,
		"jam batas absen pagi":   p.JamBatasPagi,
		"jam mulai absen pulang": p.JamMulaiPulang,
	} {
		if _, ok := parseJamToMinutes(v); !ok {
			utils.Error(w, http.StatusBadRequest, label+" tidak valid, gunakan format HH:MM")
			return
		}
	}
	mulaiMin, _ := parseJamToMinutes(p.JamMulaiPagi)
	batasMin, _ := parseJamToMinutes(p.JamBatasPagi)
	if batasMin <= mulaiMin {
		utils.Error(w, http.StatusBadRequest, "jam batas absen pagi harus lebih besar dari jam mulai absen pagi")
		return
	}

	var item models.PengaturanAbsensi
	if err := db.First(&item, 1).Error; err != nil {
		item = models.PengaturanAbsensi{ID: 1}
	}
	item.Aktif = p.Aktif
	item.JamMulaiPagi = p.JamMulaiPagi
	item.JamBatasPagi = p.JamBatasPagi
	item.JamMulaiPulang = p.JamMulaiPulang
	if err := db.Save(&item).Error; err != nil {
		utils.Error(w, http.StatusBadRequest, "gagal menyimpan pengaturan: "+err.Error())
		return
	}
	utils.Success(w, "pengaturan absen berhasil disimpan", item)
}

type rekapAbsensiItem struct {
	Pegawai         models.Pegawai   `json:"pegawai"`
	Absensi         []models.Absensi `json:"absensi"`
	TanggalTerlewat []string         `json:"tanggal_terlewat"`
}

// rekapPeriode membaca query bulan/tahun/id_pegawai dan mengembalikan
// pegawaiList (sudah difilter bila id_pegawai diberikan) beserta rentang
// tanggal bulan tersebut dan "limit" (tanggal terakhir yang sudah lewat --
// hari ini bila bulan yang dipilih adalah bulan berjalan, atau akhir bulan
// bila bulan yang dipilih sudah lampau).
func rekapPeriode(r *http.Request, db *gorm.DB) (pegawaiList []models.Pegawai, start, end, limit time.Time) {
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
	start = time.Date(tahun, bulan, 1, 0, 0, 0, 0, loc)
	end = start.AddDate(0, 1, -1)
	limit = end
	today := absensiToday()
	if today.Before(limit) {
		limit = today
	}

	query := db.Model(&models.Pegawai{})
	if idStr := strings.TrimSpace(r.URL.Query().Get("id_pegawai")); idStr != "" {
		query = query.Where("id = ?", idStr)
	}
	query.Order("nama asc").Find(&pegawaiList)
	return
}

func buildRekapItems(db *gorm.DB, pegawaiList []models.Pegawai, start, end, limit time.Time) []rekapAbsensiItem {
	idList := make([]uint, 0, len(pegawaiList))
	for _, p := range pegawaiList {
		idList = append(idList, p.ID)
	}

	var absensiRows []models.Absensi
	var dokumenRows []models.AbsensiDokumen
	if len(idList) > 0 {
		db.Where("id_pegawai IN ? AND tanggal BETWEEN ? AND ?", idList, start, end).Find(&absensiRows)
		db.Where("id_pegawai IN ? AND tanggal BETWEEN ? AND ?", idList, start, limit).Find(&dokumenRows)
	}
	holidaySet := holidaySetInRange(db, start, limit)

	absensiByPegawai := map[uint][]models.Absensi{}
	hadirSet := map[uint]map[string]bool{}
	for _, a := range absensiRows {
		a.FotoMasuk = nil
		a.FotoPulang = nil
		absensiByPegawai[a.IDPegawai] = append(absensiByPegawai[a.IDPegawai], a)
		if a.JamMasuk != nil {
			if hadirSet[a.IDPegawai] == nil {
				hadirSet[a.IDPegawai] = map[string]bool{}
			}
			hadirSet[a.IDPegawai][a.Tanggal.Format("2006-01-02")] = true
		}
	}
	tercoverSet := map[uint]map[string]bool{}
	for _, d := range dokumenRows {
		if tercoverSet[d.IDPegawai] == nil {
			tercoverSet[d.IDPegawai] = map[string]bool{}
		}
		tercoverSet[d.IDPegawai][d.Tanggal.Format("2006-01-02")] = true
	}

	items := make([]rekapAbsensiItem, 0, len(pegawaiList))
	for _, p := range pegawaiList {
		// selalu slice kosong (bukan nil) -- lihat komentar serupa di
		// riwayatAbsenSaya (absensi.go): slice nil ter-encode JSON sebagai
		// null dan bikin frontend (item.absensi.filter(...)) crash saat
		// pegawai belum punya absen sama sekali pada bulan yang dipilih.
		rows := absensiByPegawai[p.ID]
		if rows == nil {
			rows = []models.Absensi{}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Tanggal.After(rows[j].Tanggal) })

		terlewat := []string{}
		if !start.After(limit) {
			sixDayWeek := sixDayWeekForTempatTgs(p.TempatTgs)
			for _, d := range workingDaysWithHolidaySet(start, limit, sixDayWeek, holidaySet) {
				key := d.Format("2006-01-02")
				if !hadirSet[p.ID][key] && !tercoverSet[p.ID][key] {
					terlewat = append(terlewat, key)
				}
			}
		}
		items = append(items, rekapAbsensiItem{Pegawai: p, Absensi: rows, TanggalTerlewat: terlewat})
	}
	return items
}

func rekapAbsensi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	pegawaiList, start, end, limit := rekapPeriode(r, db)
	items := buildRekapItems(db, pegawaiList, start, end, limit)
	utils.Success(w, "ok", map[string]interface{}{
		"bulan": int(start.Month()),
		"tahun": start.Year(),
		"data":  items,
	})
}

// exportRekapAbsensi menghasilkan file .xlsx satu baris per (pegawai,
// tanggal hari kerja) dalam bulan yang dipilih -- baris "Hadir" berisi jam
// masuk/keluar, menit terlambat & koordinat; baris tanpa kehadiran ditandai
// "Tidak Hadir (ada surat pengganti)" atau "Tidak Hadir" saja.
func exportRekapAbsensi(w http.ResponseWriter, r *http.Request, db *gorm.DB) {
	pegawaiList, start, end, limit := rekapPeriode(r, db)
	items := buildRekapItems(db, pegawaiList, start, end, limit)

	type row struct {
		Nama      string
		NIP       string
		Tanggal   string
		JamMasuk  string
		Terlambat string
		JamPulang string
		Koordinat string
		Status    string
	}
	var rows []row
	for _, it := range items {
		byTanggal := map[string]models.Absensi{}
		for _, a := range it.Absensi {
			byTanggal[a.Tanggal.Format("2006-01-02")] = a
		}
		terlewatSet := map[string]bool{}
		for _, t := range it.TanggalTerlewat {
			terlewatSet[t] = true
		}
		sixDayWeek := sixDayWeekForTempatTgs(it.Pegawai.TempatTgs)
		holidaySet := holidaySetInRange(db, start, limit)
		for _, d := range workingDaysWithHolidaySet(start, limit, sixDayWeek, holidaySet) {
			key := d.Format("2006-01-02")
			a, ada := byTanggal[key]
			out := row{Nama: it.Pegawai.Nama, NIP: it.Pegawai.NIP, Tanggal: d.Format("02-01-2006")}
			switch {
			case ada && a.JamMasuk != nil:
				out.JamMasuk = a.JamMasuk.Format("15:04")
				if a.TerlambatMenit > 0 {
					out.Terlambat = strconv.Itoa(a.TerlambatMenit) + " menit"
				} else {
					out.Terlambat = "-"
				}
				if a.JamPulang != nil {
					out.JamPulang = a.JamPulang.Format("15:04")
				} else {
					out.JamPulang = "-"
				}
				if a.LatMasuk != nil && a.LngMasuk != nil {
					out.Koordinat = strconv.FormatFloat(*a.LatMasuk, 'f', 6, 64) + ", " + strconv.FormatFloat(*a.LngMasuk, 'f', 6, 64)
				} else {
					out.Koordinat = "-"
				}
				out.Status = "Hadir"
			case terlewatSet[key]:
				out.JamMasuk, out.Terlambat, out.JamPulang, out.Koordinat = "-", "-", "-", "-"
				out.Status = "Tidak Hadir"
			default:
				out.JamMasuk, out.Terlambat, out.JamPulang, out.Koordinat = "-", "-", "-", "-"
				out.Status = "Tidak Hadir (ada surat pengganti)"
			}
			rows = append(rows, out)
		}
	}

	columns := []utils.ExcelColumn{
		{Header: "Nama", Get: func(item interface{}) string { return item.(row).Nama }},
		{Header: "NIP", Get: func(item interface{}) string { return item.(row).NIP }},
		{Header: "Tanggal", Get: func(item interface{}) string { return item.(row).Tanggal }},
		{Header: "Jam Masuk", Get: func(item interface{}) string { return item.(row).JamMasuk }},
		{Header: "Terlambat", Get: func(item interface{}) string { return item.(row).Terlambat }},
		{Header: "Jam Pulang", Get: func(item interface{}) string { return item.(row).JamPulang }},
		{Header: "Koordinat", Get: func(item interface{}) string { return item.(row).Koordinat }},
		{Header: "Status", Get: func(item interface{}) string { return item.(row).Status }},
	}
	f, err := utils.ExportData(rows, columns)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "gagal membuat file excel: "+err.Error())
		return
	}
	filename := "rekap_absensi_" + start.Format("2006-01") + ".xlsx"
	writeXlsxResponse(w, f, filename)
}
