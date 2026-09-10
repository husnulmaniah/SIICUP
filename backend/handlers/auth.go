package handlers

import (
	"encoding/json"
	"net/http"

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

		var user models.User
		if err := db.Preload("Role").Preload("Pegawai").
			Where("username = ?", req.Username).First(&user).Error; err != nil {
			utils.Error(w, http.StatusUnauthorized, "username atau password salah")
			return
		}

		if !utils.CheckPasswordHash(req.Password, user.Pass) {
			utils.Error(w, http.StatusUnauthorized, "username atau password salah")
			return
		}

		roleName := ""
		if user.Role != nil {
			roleName = user.Role.Role
		}

		token, err := utils.GenerateToken(user.ID, user.Username, user.IDRole, roleName, user.IDPegawai)
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "gagal membuat token")
			return
		}

		utils.Success(w, "login berhasil", map[string]interface{}{
			"token": token,
			"user": map[string]interface{}{
				"id":         user.ID,
				"username":   user.Username,
				"nama":       user.Nama,
				"role":       roleName,
				"id_role":    user.IDRole,
				"id_pegawai": user.IDPegawai,
				"pegawai":    user.Pegawai,
			},
		})
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
