/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package user

import (
	"errors"
	"log"
	"net/http"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// Me mengembalikan profil pengguna yang sedang login.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u, err := h.repo.FindByID(r.Context(), httpx.UserID(r.Context()))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusUnauthorized, "akun tidak ditemukan")
	case err != nil:
		log.Printf("me: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	default:
		httpx.JSON(w, http.StatusOK, u)
	}
}
