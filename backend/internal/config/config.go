/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	DatabaseURL string
}

func Load() Config {
	_ = godotenv.Load() // abaikan jika .env tidak ada (mis. di produksi)

	cfg := Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL wajib diisi")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
