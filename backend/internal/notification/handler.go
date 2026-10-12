/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package notification

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

// Routes dipasang di bawah /api/me/notifications dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/unread-count", h.UnreadCount)
	r.Post("/read-all", h.MarkAllRead)
	r.Post("/{id}/read", h.MarkRead)
}

// List melayani GET /api/me/notifications?limit=20&before=<id>&unread=true
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := intParam(w, q.Get("limit"), "limit")
	if !ok {
		return
	}
	before, ok := intParam(w, q.Get("before"), "before")
	if !ok {
		return
	}
	unread := false
	switch q.Get("unread") {
	case "", "false":
	case "true":
		unread = true
	default:
		httpx.Error(w, http.StatusBadRequest, "unread harus true atau false")
		return
	}
	page, err := h.svc.List(r.Context(), httpx.UserID(r.Context()), int(limit), before, unread)
	if err != nil {
		fail(w, "list", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.UnreadCount(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "unread-count", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"unread_count": n})
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err := h.svc.MarkRead(r.Context(), httpx.UserID(r.Context()), id); err != nil {
		fail(w, "mark-read", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"read": true})
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.MarkAllRead(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "read-all", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int64{"updated": n})
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
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	default:
		log.Printf("notification %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
