/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes dipasang pada router /api/groups dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/{id}/rounds", h.List)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	rounds, err := h.svc.List(r.Context(), httpx.UserID(r.Context()), id)
	switch {
	case errors.Is(err, group.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "grup tidak ditemukan")
	case errors.Is(err, group.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case err != nil:
		log.Printf("rounds list: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	default:
		httpx.JSON(w, http.StatusOK, map[string]any{"rounds": rounds})
	}
}
