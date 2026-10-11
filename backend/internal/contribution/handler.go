/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/round"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes dipasang pada router /api/groups dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/{id}/rounds/{roundID}/contributions", h.ListByRound)
}

func (h *Handler) ListByRound(w http.ResponseWriter, r *http.Request) {
	groupID, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	roundID, ok := httpx.URLUUID(w, r, "roundID")
	if !ok {
		return
	}
	list, err := h.svc.ListByRound(r.Context(), httpx.UserID(r.Context()), groupID, roundID)
	if err != nil {
		fail(w, "list by round", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"contributions": list})
}

// Mine melayani GET /api/me/contributions?status=unpaid|paid
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Mine(r.Context(), httpx.UserID(r.Context()), r.URL.Query().Get("status"))
	if err != nil {
		fail(w, "mine", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"contributions": list})
}

func fail(w http.ResponseWriter, op string, err error) {
	var verr group.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, group.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "grup tidak ditemukan")
	case errors.Is(err, round.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, group.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, err.Error())
	default:
		log.Printf("contribution %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
