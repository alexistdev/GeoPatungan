/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	JWTTTL      time.Duration
}

func Load() Config {
	_ = godotenv.Load() // abaikan jika .env tidak ada (mis. di produksi)

	hours, err := strconv.Atoi(getenv("JWT_TTL_HOURS", "24"))
	if err != nil || hours <= 0 {
		log.Fatal("JWT_TTL_HOURS harus angka positif")
	}

	cfg := Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		JWTTTL:      time.Duration(hours) * time.Hour,
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL wajib diisi")
	}
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET wajib diisi, minimal 32 karakter")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
