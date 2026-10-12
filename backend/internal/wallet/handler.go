/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes dipasang di bawah /api/wallet dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.Get)
	r.Get("/ledger", h.Ledger)
}

// AdminRoutes dipasang di bawah /api/admin (RequireRole admin).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/wallet/reconcile", h.Reconcile)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	wlt, err := h.svc.Wallet(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "get", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"wallet": wlt})
}

// Ledger melayani GET /api/wallet/ledger?limit=20&before=<id>
func (h *Handler) Ledger(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := intParam(w, q.Get("limit"), "limit")
	if !ok {
		return
	}
	before, ok := intParam(w, q.Get("before"), "before")
	if !ok {
		return
	}
	page, err := h.svc.Ledger(r.Context(), httpx.UserID(r.Context()), int(limit), before)
	if err != nil {
		fail(w, "ledger", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) Reconcile(w http.ResponseWriter, r *http.Request) {
	rep, err := h.svc.Reconcile(r.Context())
	if err != nil {
		fail(w, "reconcile", err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

func intParam(w http.ResponseWriter, raw, name string) (int64, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, name+" harus berupa angka")
		return 0, false
	}
	return n, true
}

func fail(w http.ResponseWriter, op string, err error) {
	var verr httpx.ValidationError
	if errors.As(err, &verr) {
		httpx.Error(w, http.StatusBadRequest, verr.Error())
		return
	}
	log.Printf("wallet %s: %v", op, err)
	httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
}
