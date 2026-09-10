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
	jwt.RegisteredClaims
}

func GenerateToken(userID uint, username string, roleID uint, roleName string, idPegawai *uint) (string, error) {
	claims := Claims{
		UserID:    userID,
		Username:  username,
		RoleID:    roleID,
		RoleName:  roleName,
		IDPegawai: idPegawai,
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
