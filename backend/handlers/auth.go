package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"cuti-app/middleware"
	"cuti-app/models"
	"cuti-app/utils"

	"gorm.io/gorm"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func LoginHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
			utils.Error(w, http.StatusBadRequest, "username dan password wajib diisi")
			return
		}

		// "Pegawai.UnitKerja" ditambahkan (di samping "Pegawai" itu sendiri)
		// supaya isSekolahPegawai bisa dihitung akurat langsung dari sini --
		// dipakai frontend (auth store) untuk menentukan visibilitas menu
		// "Template Surat" (hanya akun sekolah) tanpa harus memanggil /api/me
		// atau menebak-nebak dari data yang mungkin belum lengkap.
		var user models.User
		if err := db.Preload("Role").Preload("Pegawai.UnitKerja").
			Where("username = ?", req.Username).First(&user).Error; err != nil {
			utils.Error(w, http.StatusUnauthorized, "username atau password salah")
			return
		}

		if !utils.CheckPasswordHash(req.Password, user.Pass) {
			utils.Error(w, http.StatusUnauthorized, "username atau password salah")
			return
		}

		// Akun yang sudah dinonaktifkan (mis. karena pengajuan pensiun
		// pegawai pemiliknya disetujui, lihat approvePengajuanPensiun di
		// handlers/pengajuan_pensiun.go) tidak boleh login lagi sama sekali,
		// walau username/password-nya benar.
		if !user.Aktif {
			utils.Error(w, http.StatusForbidden, "akun ini sudah tidak aktif. Hubungi administrator jika ini keliru.")
			return
		}

		roleName := ""
		if user.Role != nil {
			roleName = user.Role.Role
		}

		isSekolah := false
		if user.Pegawai != nil {
			isSekolah = isSekolahPegawai(*user.Pegawai)
		}

		token, err := utils.GenerateToken(user.ID, user.Username, user.IDRole, roleName, user.IDPegawai, user.IsAdminAbsensi, user.IsAdminVerifikasi)
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal membuat token")
			return
		}

		utils.Success(w, "login berhasil", map[string]interface{}{
			"token": token,
			"user": map[string]interface{}{
				"id":                  user.ID,
				"username":            user.Username,
				"nama":                user.Nama,
				"role":                roleName,
				"id_role":             user.IDRole,
				"id_pegawai":          user.IDPegawai,
				"pegawai":             user.Pegawai,
				"is_admin_absensi":    user.IsAdminAbsensi,
				"is_admin_verifikasi": user.IsAdminVerifikasi,
				"is_sekolah":          isSekolah,
			},
		})
	}
}

type changePasswordRequest struct {
	PasswordLama string `json:"password_lama"`
	PasswordBaru string `json:"password_baru"`
}

// ChangePasswordHandler mengizinkan SETIAP pegawai (pegawai/atasan/admin/
// administrator) mengganti password akunnya sendiri -- permintaan pengguna:
// "buat agar masing-masing pegawai bisa mengganti passwordnya masing-masing".
// Selalu memproses akun milik token JWT yang sedang login (claims.UserID),
// TIDAK PERNAH menerima id user lain lewat body/URL, jadi endpoint ini aman
// didaftarkan tanpa RequireRole -- berbeda dari PUT /api/user/{id} (lihat
// handlers/user.go) yang khusus administrator dan BISA mengganti password
// user lain (dipakai untuk reset password, tetap tidak diubah/dihapus oleh
// fitur ini).
func ChangePasswordHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.GetClaims(r)
		if !ok {
			utils.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			utils.Error(w, http.StatusBadRequest, "data tidak valid")
			return
		}
		req.PasswordLama = strings.TrimSpace(req.PasswordLama)
		req.PasswordBaru = strings.TrimSpace(req.PasswordBaru)

		if req.PasswordLama == "" || req.PasswordBaru == "" {
			utils.Error(w, http.StatusBadRequest, "password lama dan password baru wajib diisi")
			return
		}
		if len(req.PasswordBaru) < 6 {
			utils.Error(w, http.StatusBadRequest, "password baru minimal 6 karakter")
			return
		}

		var user models.User
		if err := db.First(&user, claims.UserID).Error; err != nil {
			utils.Error(w, http.StatusNotFound, "user tidak ditemukan")
			return
		}

		if !utils.CheckPasswordHash(req.PasswordLama, user.Pass) {
			utils.Error(w, http.StatusBadRequest, "password lama salah")
			return
		}

		hash, err := utils.HashPassword(req.PasswordBaru)
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal memproses password baru")
			return
		}

		if err := db.Model(&user).Updates(map[string]interface{}{"pass": hash}).Error; err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal menyimpan password baru: "+err.Error())
			return
		}

		utils.Success(w, "password berhasil diganti", nil)
	}
}

func MeHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.GetClaims(r)
		if !ok {
			utils.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var user models.User
		if err := db.Preload("Role").Preload("Pegawai.Jabatan").Preload("Pegawai.UnitKerja").
			First(&user, claims.UserID).Error; err != nil {
			utils.Error(w, http.StatusNotFound, "user tidak ditemukan")
			return
		}
		utils.Success(w, "ok", user)
	}
}
