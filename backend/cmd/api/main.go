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
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/alexistdev/geopatungan/internal/auth"
	"github.com/alexistdev/geopatungan/internal/config"
	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/database"
	"github.com/alexistdev/geopatungan/internal/draw"
	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/middleware"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/settings"
	"github.com/alexistdev/geopatungan/internal/topup"
	"github.com/alexistdev/geopatungan/internal/user"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("gagal konek database: %v", err)
	}
	defer db.Close()

	userRepo := user.NewRepository(db)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	authHandler := auth.NewHandler(auth.NewService(userRepo, tokens))
	userHandler := user.NewHandler(userRepo)
	groupRepo := group.NewRepository(db)
	levels := group.BaseLevel{} // diganti implementasi reputasi nanti
	groupHandler := group.NewHandler(group.NewService(groupRepo, levels))
	drawHandler := draw.NewHandler(draw.NewService(groupRepo, draw.NewRepository(db), levels))
	roundHandler := round.NewHandler(round.NewService(round.NewRepository(db), groupRepo))
	settingsRepo := settings.NewRepository(db)
	settingsHandler := settings.NewHandler(settings.NewService(settingsRepo))
	topupHandler := topup.NewHandler(topup.NewService(topup.NewRepository(db), settingsRepo))
	walletHandler := wallet.NewHandler(wallet.NewService(wallet.NewRepository(db)))
	contributionHandler := contribution.NewHandler(contribution.NewService(contribution.NewRepository(db), groupRepo))

	r := chi.NewRouter()
	r.Use(chimw.Logger, chimw.Recoverer)

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

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(tokens))
			r.Get("/me", userHandler.Me)
			r.Get("/me/contributions", contributionHandler.Mine)
			r.Get("/payment-info", settingsHandler.PaymentInfo)
			r.Route("/wallet", func(r chi.Router) {
				walletHandler.Routes(r)
				topupHandler.Routes(r)
			})
			r.Route("/admin", func(r chi.Router) {
				r.Use(middleware.RequireRole(userRepo, "admin"))
				settingsHandler.AdminRoutes(r)
				walletHandler.AdminRoutes(r)
				topupHandler.AdminRoutes(r)
			})
			r.Route("/groups", func(r chi.Router) {
				groupHandler.Routes(r)
				drawHandler.Routes(r)
				roundHandler.Routes(r)
				contributionHandler.Routes(r)
			})
		})
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("GeoPatungan API jalan di :%s", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
