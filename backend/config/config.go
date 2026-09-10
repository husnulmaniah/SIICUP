package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	// DatabaseURL, when set, is used as-is (e.g. Railway/Render/Neon style
	// "postgres://user:pass@host:port/db?sslmode=require"). Falls back to the
	// individual DB_* vars below when empty, for local development.
	DatabaseURL string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	DBSSLMode   string
	JWTSecret   string
	AppPort     string
	// FrontendOrigins: comma-separated list of allowed CORS origins in
	// production (e.g. "https://siicup.vercel.app"). Empty means allow all
	// origins ("*"), which is fine for local development.
	FrontendOrigins string
}

var App Config

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("info: .env file not found, using system environment variables")
	}

	App = Config{
		DatabaseURL:     getEnv("DATABASE_URL", ""),
		DBHost:          getEnv("DB_HOST", "localhost"),
		DBPort:          getEnv("DB_PORT", "5432"),
		DBUser:          getEnv("DB_USER", "postgres"),
		DBPassword:      getEnv("DB_PASSWORD", "postgres"),
		DBName:          getEnv("DB_NAME", "cuti_app"),
		DBSSLMode:       getEnv("DB_SSLMODE", "disable"),
		JWTSecret:       getEnv("JWT_SECRET", "ganti-secret-ini-di-production"),
		// Railway/Render inject PORT automatically; APP_PORT is the local-dev name.
		AppPort:         getEnv("PORT", getEnv("APP_PORT", "8080")),
		FrontendOrigins: getEnv("FRONTEND_ORIGINS", ""),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
