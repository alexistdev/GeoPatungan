/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package app merakit seluruh handler dan rute API. Dipisah dari cmd/api supaya tes (termasuk tes
// kontrak frontend-backend) memakai router yang sama persis dengan yang berjalan di produksi.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/auth"
	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/draw"
	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/kyc"
	"github.com/alexistdev/geopatungan/internal/middleware"
	"github.com/alexistdev/geopatungan/internal/notification"
	"github.com/alexistdev/geopatungan/internal/payment"
	"github.com/alexistdev/geopatungan/internal/referral"
	"github.com/alexistdev/geopatungan/internal/reputation"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/settings"
	"github.com/alexistdev/geopatungan/internal/topup"
	"github.com/alexistdev/geopatungan/internal/user"
	"github.com/alexistdev/geopatungan/internal/wallet"
	"github.com/alexistdev/geopatungan/internal/withdrawal"
)

// NewRouter membangun router API lengkap. db boleh nil untuk tujuan memeriksa daftar rute saja
// (tidak ada query yang berjalan saat router dibangun).
func NewRouter(db *pgxpool.Pool, tokens *auth.TokenManager, middlewares ...func(http.Handler) http.Handler) chi.Router {
	userRepo := user.NewRepository(db)
	authHandler := auth.NewHandler(auth.NewService(userRepo, tokens))
	userHandler := user.NewHandler(userRepo)
	groupRepo := group.NewRepository(db)
	reputationSvc := reputation.NewService(reputation.NewRepository(db))
	levels := reputationSvc // level dihitung dari data nyata (KYC, arisan tuntas, keterlambatan)
	groupHandler := group.NewHandler(group.NewService(groupRepo, levels))
	drawHandler := draw.NewHandler(draw.NewService(groupRepo, draw.NewRepository(db), levels))
	roundHandler := round.NewHandler(round.NewService(round.NewRepository(db), groupRepo))
	settingsRepo := settings.NewRepository(db)
	settingsHandler := settings.NewHandler(settings.NewService(settingsRepo))
	topupHandler := topup.NewHandler(topup.NewService(topup.NewRepository(db), settingsRepo))
	walletHandler := wallet.NewHandler(wallet.NewService(wallet.NewRepository(db)))
	withdrawalHandler := withdrawal.NewHandler(withdrawal.NewService(withdrawal.NewRepository(db), settingsRepo))
	referralHandler := referral.NewHandler(referral.NewService(referral.NewRepository(db)))
	notificationHandler := notification.NewHandler(notification.NewService(notification.NewRepository(db)))
	reputationHandler := reputation.NewHandler(reputationSvc)
	kycHandler := kyc.NewHandler(kyc.NewService(kyc.NewRepository(db)))
	paymentHandler := payment.NewHandler(payment.NewRepository(db))
	contributionHandler := contribution.NewHandler(contribution.NewService(contribution.NewRepository(db), groupRepo))

	r := chi.NewRouter()
	r.Use(middlewares...)

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
			r.Get("/me/reputation", reputationHandler.Me)
			r.Route("/me/kyc", kycHandler.Routes)
			r.Route("/me/referral", referralHandler.Routes)
			r.Route("/me/notifications", notificationHandler.Routes)
			r.Route("/contributions", paymentHandler.Routes)
			r.Route("/wallet", func(r chi.Router) {
				walletHandler.Routes(r)
				topupHandler.Routes(r)
				withdrawalHandler.Routes(r)
			})
			r.Route("/admin", func(r chi.Router) {
				r.Use(middleware.RequireRole(userRepo, "admin"))
				settingsHandler.AdminRoutes(r)
				walletHandler.AdminRoutes(r)
				topupHandler.AdminRoutes(r)
				withdrawalHandler.AdminRoutes(r)
				reputationHandler.AdminRoutes(r)
				kycHandler.AdminRoutes(r)
				referralHandler.AdminRoutes(r)
			})
			r.Route("/groups", func(r chi.Router) {
				groupHandler.Routes(r, middleware.RequireRole(userRepo, "admin"))
				drawHandler.Routes(r)
				roundHandler.Routes(r)
				contributionHandler.Routes(r)
			})
		})
	})
	return r
}
