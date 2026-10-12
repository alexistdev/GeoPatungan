/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package topup

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

// Routes dipasang di bawah /api/wallet (rute rata, bersama modul wallet) dan wajib
// berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/topups", h.Create)
	r.Get("/topups", h.List)
	r.Get("/topups/{id}", h.Get)
	r.Post("/topups/{id}/confirm", h.Confirm)
	r.Post("/topups/{id}/cancel", h.Cancel)
}

// AdminRoutes dipasang di bawah /api/admin (RequireRole admin).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/topups", h.ListAdmin)
	r.Post("/topups/{id}/approve", h.Approve)
	r.Post("/topups/{id}/reject", h.Reject)
}

type createRequest struct {
	Amount int64 `json:"amount"`
}

type confirmRequest struct {
	SenderName string `json:"sender_name"`
	SenderBank string `json:"sender_bank"`
	Note       string `json:"note"`
}

type approveRequest struct {
	ReceivedAmount *int64 `json:"received_amount"` // kosong = sebesar nominal transfer
	Note           string `json:"note"`
}

type rejectRequest struct {
	Note string `json:"note"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	d, err := h.svc.Create(r.Context(), httpx.UserID(r.Context()), req.Amount)
	if err != nil {
		fail(w, "create", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "list", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"topups": list})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "get", err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req confirmRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	t, err := h.svc.Confirm(r.Context(), httpx.UserID(r.Context()), id, req.SenderName, req.SenderBank, req.Note)
	if err != nil {
		fail(w, "confirm", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"topup": t})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.Cancel(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "cancel", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"topup": t})
}

// ListAdmin melayani GET /api/admin/topups?status=open|all|pending|...&limit=50
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
	httpx.JSON(w, http.StatusOK, map[string]any{"topups": list})
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req approveRequest
	if !httpx.DecodeOptional(w, r, &req) {
		return
	}
	t, err := h.svc.Approve(r.Context(), httpx.UserID(r.Context()), id, req.ReceivedAmount, req.Note)
	if err != nil {
		fail(w, "approve", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"topup": t})
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
	t, err := h.svc.Reject(r.Context(), httpx.UserID(r.Context()), id, req.Note)
	if err != nil {
		fail(w, "reject", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"topup": t})
}

func fail(w http.ResponseWriter, op string, err error) {
	var verr httpx.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSelfReview):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidState), errors.Is(err, ErrTooManyOpen), errors.Is(err, ErrBusy):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoBankAccount):
		httpx.Error(w, http.StatusServiceUnavailable, err.Error())
	default:
		log.Printf("topup %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
