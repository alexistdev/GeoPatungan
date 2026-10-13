/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package main

import (
	"context"
	"log"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/alexistdev/geopatungan/internal/app"
	"github.com/alexistdev/geopatungan/internal/auth"
	"github.com/alexistdev/geopatungan/internal/config"
	"github.com/alexistdev/geopatungan/internal/database"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("gagal konek database: %v", err)
	}
	defer db.Close()

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	r := app.NewRouter(db, tokens, chimw.Logger, chimw.Recoverer)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("GeoPatungan API jalan di :%s", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
