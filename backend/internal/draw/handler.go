/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

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

// Routes dipasang pada router /api/groups (rute rata, bersama modul group)
// dan wajib berada di balik middleware autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/{id}/draw", h.Get)
	r.Post("/{id}/draw/commit", h.Commit)
	r.Post("/{id}/draw/execute", h.Execute)
	r.Post("/{id}/draw/manual", h.Manual)
}

type manualRequest struct {
	Order []string `json:"order"` // member ID, dari giliran pertama sampai terakhir
}

func (h *Handler) Commit(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	d, err := h.svc.Commit(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "commit", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *Handler) Execute(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	d, err := h.svc.Execute(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "execute", err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) Manual(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	var req manualRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	for _, mid := range req.Order {
		if !httpx.IsUUID(mid) {
			httpx.Error(w, http.StatusBadRequest, "order harus berisi member ID (UUID)")
			return
		}
	}
	d, err := h.svc.Manual(r.Context(), httpx.UserID(r.Context()), id, req.Order)
	if err != nil {
		fail(w, "manual", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
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

func fail(w http.ResponseWriter, op string, err error) {
	var verr group.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, group.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "grup tidak ditemukan")
	case errors.Is(err, ErrNoDraw):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, group.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrAlreadyCommitted), errors.Is(err, ErrAlreadyExecuted),
		errors.Is(err, ErrGroupNotFull), errors.Is(err, ErrWrongMethod),
		errors.Is(err, ErrMembersChanged), errors.Is(err, ErrGroupStarted):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		log.Printf("draw %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
