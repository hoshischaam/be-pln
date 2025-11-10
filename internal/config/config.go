package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port       string
	DSN        string
	JWTSecret  string
	BcryptCost int
}

func Load() Config {
	// coba load .env, kalau gak ada ya di-skip
	_ = godotenv.Load()

	cfg := Config{
		Port:      getEnv("PORT", "8080"),
		DSN:       getEnv("DATABASE_URL", ""),
		JWTSecret: getEnv("JWT_SECRET", "changeme"),
	}

	// default cost bcrypt
	if cost := os.Getenv("BCRYPT_COST"); cost != "" {
		cfg.BcryptCost = 12 // nanti bisa parse string ke int
	} else {
		cfg.BcryptCost = 10
	}

	if cfg.DSN == "" {
		log.Fatal("DATABASE_URL tidak boleh kosong (cek file .env)")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
