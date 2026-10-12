/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package referral

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

// Routes dipasang di bawah /api/me/referral dan wajib berada di balik autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.Summary)
	r.Get("/commissions", h.Commissions)
}

// AdminRoutes dipasang di bawah /api/admin (RequireRole admin).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/referral/rules", h.GetRules)
	r.Put("/referral/rules", h.PutRules)
}

type rulesRequest struct {
	Rules []Rule `json:"rules"`
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Summary(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "summary", err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

// Commissions melayani GET /api/me/referral/commissions?limit=20&before=<id>
func (h *Handler) Commissions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := intParam(w, q.Get("limit"), "limit")
	if !ok {
		return
	}
	before, ok := intParam(w, q.Get("before"), "before")
	if !ok {
		return
	}
	page, err := h.svc.Commissions(r.Context(), httpx.UserID(r.Context()), int(limit), before)
	if err != nil {
		fail(w, "commissions", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) GetRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.Rules(r.Context())
	if err != nil {
		fail(w, "get rules", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) PutRules(w http.ResponseWriter, r *http.Request) {
	var req rulesRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	rules, err := h.svc.UpdateRules(r.Context(), httpx.UserID(r.Context()), req.Rules)
	if err != nil {
		fail(w, "put rules", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rules": rules})
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
		log.Printf("referral %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
