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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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

	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		c, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(c); err != nil {
			http.Error(w, `{"status":"db down"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("GeoPatungan API jalan di :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}