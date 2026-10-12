/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package reputation

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

// AdminRoutes dipasang di bawah /api/admin (RequireRole admin).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/reputation/levels", h.GetLevels)
	r.Put("/reputation/levels", h.PutLevels)
}

type levelsRequest struct {
	Levels []Level `json:"levels"`
}

// Me melayani GET /api/me/reputation.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Profile(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "me", err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) GetLevels(w http.ResponseWriter, r *http.Request) {
	levels, err := h.svc.Levels(r.Context())
	if err != nil {
		fail(w, "get levels", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"levels": levels})
}

func (h *Handler) PutLevels(w http.ResponseWriter, r *http.Request) {
	var req levelsRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	levels, err := h.svc.UpdateLevels(r.Context(), httpx.UserID(r.Context()), req.Levels)
	if err != nil {
		fail(w, "put levels", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"levels": levels})
}

func fail(w http.ResponseWriter, op string, err error) {
	var verr httpx.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	default:
		log.Printf("reputation %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
