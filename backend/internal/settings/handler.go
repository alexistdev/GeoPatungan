/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package settings

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// AdminRoutes dipasang di bawah /api/admin, di balik autentikasi dan RequireRole("admin").
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/settings", h.GetSettings)
	r.Put("/settings", h.UpdateSettings)
	r.Get("/bank-accounts", h.ListBanks)
	r.Post("/bank-accounts", h.CreateBank)
	r.Put("/bank-accounts/{id}", h.UpdateBank)
}

type bankRequest struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountHolder string `json:"account_holder"`
	Notes         string `json:"notes"`
	IsActive      *bool  `json:"is_active"` // kosong = aktif
}

func (b bankRequest) toBank() BankAccount {
	active := true
	if b.IsActive != nil {
		active = *b.IsActive
	}
	return BankAccount{BankName: b.BankName, AccountNumber: b.AccountNumber,
		AccountHolder: b.AccountHolder, Notes: b.Notes, IsActive: active}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context())
	if err != nil {
		fail(w, "get settings", err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if !httpx.Decode(w, r, &in) {
		return
	}
	s, err := h.svc.Update(r.Context(), httpx.UserID(r.Context()), in)
	if err != nil {
		fail(w, "update settings", err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) ListBanks(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListBanks(r.Context())
	if err != nil {
		fail(w, "list banks", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bank_accounts": list})
}

func (h *Handler) CreateBank(w http.ResponseWriter, r *http.Request) {
	var req bankRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	b, err := h.svc.CreateBank(r.Context(), httpx.UserID(r.Context()), req.toBank())
	if err != nil {
		fail(w, "create bank", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, b)
}

func (h *Handler) UpdateBank(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req bankRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	b, err := h.svc.UpdateBank(r.Context(), httpx.UserID(r.Context()), id, req.toBank())
	if err != nil {
		fail(w, "update bank", err)
		return
	}
	httpx.JSON(w, http.StatusOK, b)
}

// PaymentInfo melayani GET /api/payment-info untuk pengguna biasa.
func (h *Handler) PaymentInfo(w http.ResponseWriter, r *http.Request) {
	info, err := h.svc.PaymentInfo(r.Context())
	if err != nil {
		fail(w, "payment info", err)
		return
	}
	httpx.JSON(w, http.StatusOK, info)
}

func fail(w http.ResponseWriter, op string, err error) {
	var verr httpx.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "data tidak ditemukan")
	case errors.Is(err, ErrDuplicate):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		log.Printf("settings %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
