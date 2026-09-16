package utils

import (
	"errors"
	"time"

	"cuti-app/config"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	RoleID    uint   `json:"role_id"`
	RoleName  string `json:"role_name"`
	IDPegawai *uint  `json:"id_pegawai"`
	// IsAdminAbsensi: lihat models.User.IsAdminAbsensi -- akun (role apa pun,
	// biasanya pegawai/atasan) yang tambahan boleh mengakses menu "Input
	// Rekapan Absensi" tanpa mengubah role utamanya.
	IsAdminAbsensi bool `json:"is_admin_absensi"`
	// IsAdminVerifikasi: lihat models.User.IsAdminVerifikasi -- akun (role
	// apa pun) yang tambahan boleh memverifikasi (menyetujui/mengembalikan)
	// Pengajuan Surat Kolektif dari pegawai sekolah, terlepas dari
	// IsAdminAbsensi. Kedua centang ini independen & bisa dicentang
	// bersamaan pada satu akun yang sama.
	IsAdminVerifikasi bool `json:"is_admin_verifikasi"`
	jwt.RegisteredClaims
}

func GenerateToken(userID uint, username string, roleID uint, roleName string, idPegawai *uint, isAdminAbsensi bool, isAdminVerifikasi bool) (string, error) {
	claims := Claims{
		UserID:            userID,
		Username:          username,
		RoleID:            roleID,
		RoleName:          roleName,
		IDPegawai:         idPegawai,
		IsAdminAbsensi:    isAdminAbsensi,
		IsAdminVerifikasi: isAdminVerifikasi,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.App.JWTSecret))
}

func ParseToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("metode signing token tidak valid")
		}
		return []byte(config.App.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token tidak valid")
	}
	return claims, nil
}
