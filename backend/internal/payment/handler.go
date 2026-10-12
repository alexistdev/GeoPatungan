/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package payment

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/round"
)

// Store adalah akses data pembayaran (diimplementasikan *Repository).
type Store interface {
	Pay(ctx context.Context, userID, contributionID string) (*Result, error)
	Quote(ctx context.Context, userID, contributionID string) (*QuoteResult, error)
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

// Routes dipasang di bawah /api/contributions dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/{id}/quote", h.Quote)
	r.Post("/{id}/pay", h.Pay)
}

// Pay melayani POST /api/contributions/{id}/pay. Pembayar selalu pengguna yang login;
// tidak ada body.
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	res, err := h.store.Pay(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "pay", err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Quote(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	q, err := h.store.Quote(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "quote", err)
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

func fail(w http.ResponseWriter, op string, err error) {
	var short *InsufficientFundsError
	switch {
	case errors.As(err, &short):
		// 402 Payment Required: sertakan angka agar klien bisa menampilkan kekurangannya.
		httpx.JSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "saldo tidak cukup", "required": short.Required, "balance": short.Balance,
		})
	case errors.Is(err, contribution.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "iuran tidak ditemukan")
	case errors.Is(err, contribution.ErrAlreadyPaid):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, round.ErrRoundNotCollecting):
		httpx.Error(w, http.StatusConflict, "putaran belum dibuka atau sudah ditutup")
	default:
		log.Printf("payment %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
