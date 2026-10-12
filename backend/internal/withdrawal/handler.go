/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package withdrawal

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes dipasang di bawah /api/wallet (rute rata, bersama modul wallet) dan wajib
// berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/withdrawals", h.Create)
	r.Get("/withdrawals", h.List)
	r.Get("/withdrawals/{id}", h.Get)
	r.Post("/withdrawals/{id}/cancel", h.Cancel)
}

// AdminRoutes dipasang di bawah /api/admin (RequireRole admin).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/withdrawals", h.ListAdmin)
	r.Post("/withdrawals/{id}/complete", h.Complete)
	r.Post("/withdrawals/{id}/reject", h.Reject)
}

type createRequest struct {
	Amount        int64  `json:"amount"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountHolder string `json:"account_holder"`
	Note          string `json:"note"`
}

type completeRequest struct {
	Reference string `json:"reference"`
	Note      string `json:"note"`
}

type rejectRequest struct {
	Note string `json:"note"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	wd, err := h.svc.Create(r.Context(), httpx.UserID(r.Context()), Input(req))
	if err != nil {
		fail(w, "create", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"withdrawal": wd})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "list", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"withdrawals": list})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	wd, err := h.svc.Get(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "get", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"withdrawal": wd})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	wd, err := h.svc.Cancel(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "cancel", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"withdrawal": wd})
}

// ListAdmin melayani GET /api/admin/withdrawals?status=pending|completed|rejected|cancelled|all&limit=50
func (h *Handler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "limit harus berupa angka")
			return
		}
		limit = n
	}
	list, err := h.svc.ListAdmin(r.Context(), r.URL.Query().Get("status"), limit)
	if err != nil {
		fail(w, "list admin", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"withdrawals": list})
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req completeRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	wd, err := h.svc.Complete(r.Context(), httpx.UserID(r.Context()), id, req.Reference, req.Note)
	if err != nil {
		fail(w, "complete", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"withdrawal": wd})
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req rejectRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	wd, err := h.svc.Reject(r.Context(), httpx.UserID(r.Context()), id, req.Note)
	if err != nil {
		fail(w, "reject", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"withdrawal": wd})
}

func fail(w http.ResponseWriter, op string, err error) {
	var verr httpx.ValidationError
	var short *wallet.InsufficientFundsError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.As(err, &short):
		httpx.JSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "saldo tidak cukup", "required": short.Required, "balance": short.Balance,
		})
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSelfReview):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidState), errors.Is(err, ErrTooManyOpen):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		log.Printf("withdrawal %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
