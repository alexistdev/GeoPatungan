/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"context"
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

// Routes dipasang di bawah /api/groups dan wajib berada di balik middleware autentikasi.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/", h.Create)
	r.Get("/", h.ListMine)
	r.Post("/join", h.Join)
	// Rute dibuat rata (tanpa sub-router bersarang) supaya modul lain, mis. draw,
	// bisa mendaftarkan rute di bawah /{id}/... pada router yang sama.
	r.Get("/{id}", h.Get)
	r.Get("/{id}/members", h.Members)
	r.Post("/{id}/members/{memberID}/approve", h.Approve)
	r.Post("/{id}/members/{memberID}/reject", h.Reject)
}

type createRequest struct {
	Name               string `json:"name"`
	ContributionAmount int64  `json:"contribution_amount"`
	PeriodType         string `json:"period_type"`
	SlotCount          int    `json:"slot_count"`
	StartDate          string `json:"start_date"`
	LateFee            int64  `json:"late_fee"`
	DrawMethod         string `json:"draw_method"`
	MinLevelRank       int    `json:"min_level_rank"`
}

type joinRequest struct {
	InviteCode string `json:"invite_code"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	g, err := h.svc.Create(r.Context(), httpx.UserID(r.Context()), CreateInput(req))
	if err != nil {
		fail(w, "create", err, "")
		return
	}
	httpx.JSON(w, http.StatusCreated, g)
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	groups, err := h.svc.ListMine(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		fail(w, "list", err, "")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	var req joinRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.InviteCode == "" {
		httpx.Error(w, http.StatusBadRequest, "invite_code wajib diisi")
		return
	}
	g, err := h.svc.Join(r.Context(), httpx.UserID(r.Context()), req.InviteCode)
	if err != nil {
		fail(w, "join", err, "kode undangan tidak ditemukan")
		return
	}
	httpx.JSON(w, http.StatusCreated, g)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	g, err := h.svc.Get(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "get", err, "grup tidak ditemukan")
		return
	}
	httpx.JSON(w, http.StatusOK, g)
}

func (h *Handler) Members(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	members, err := h.svc.Members(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		fail(w, "members", err, "grup tidak ditemukan")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": members})
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, "approve", "active", h.svc.Approve)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, "reject", "removed", h.svc.Reject)
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, op, resultStatus string,
	act func(ctx context.Context, userID, groupID, memberID string) error) {
	groupID, ok := httpx.URLUUID(w, r, "id")
	if !ok {
		return
	}
	memberID, ok := httpx.URLUUID(w, r, "memberID")
	if !ok {
		return
	}
	if err := act(r.Context(), httpx.UserID(r.Context()), groupID, memberID); err != nil {
		fail(w, op, err, "permintaan bergabung tidak ditemukan")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": resultStatus})
}

// fail menerjemahkan error domain menjadi respons HTTP. notFoundMsg mengganti
// pesan default untuk ErrNotFound agar sesuai konteks endpoint.
func fail(w http.ResponseWriter, op string, err error, notFoundMsg string) {
	var verr ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, ErrNotFound):
		if notFoundMsg == "" {
			notFoundMsg = ErrNotFound.Error()
		}
		httpx.Error(w, http.StatusNotFound, notFoundMsg)
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrLevelTooLow),
		errors.Is(err, ErrCannotCreateGroup), errors.Is(err, ErrGroupTooExpensive):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrAlreadyMember), errors.Is(err, ErrGroupFull), errors.Is(err, ErrGroupNotOpen):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		log.Printf("group %s: %v", op, err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}
