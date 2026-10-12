/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package kyc

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

// Routes dipasang di bawah /api/me/kyc dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.Status)
	r.Post("/", h.Submit)
}

// AdminRoutes dipasang di bawah /api/admin (RequireRole admin).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/kyc", h.ListAdmin)
	r.Post("/kyc/{id}/approve", h.Approve)
	r.Post("/kyc/{id}/reject", h.Reject)
}

type noteRequest struct {
	Note string `json:"note"`
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "status", err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	var req noteRequest
	if !httpx.DecodeOptional(w, r, &req) {
		return
	}
	k, err := h.svc.Submit(r.Context(), httpx.UserID(r.Context()), req.Note)
	if err != nil {
		fail(w, "submit", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"request": k})
}

// ListAdmin melayani GET /api/admin/kyc?status=pending|verified|rejected|all&limit=50
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
	httpx.JSON(w, http.StatusOK, map[string]any{"requests": list})
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req noteRequest
	if !httpx.DecodeOptional(w, r, &req) {
		return
	}
	k, err := h.svc.Approve(r.Context(), httpx.UserID(r.Context()), id, req.Note)
	if err != nil {
		fail(w, "approve", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"request": k})
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req noteRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	k, err := h.svc.Reject(r.Context(), httpx.UserID(r.Context()), id, req.Note)
	if err != nil {
		fail(w, "reject", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"request": k})
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
	case errors.Is(err, ErrAlreadyPending), errors.Is(err, ErrAlreadyVerified), errors.Is(err, ErrInvalidState):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		log.Printf("kyc %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
