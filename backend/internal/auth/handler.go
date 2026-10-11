/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package auth

import (
	"errors"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/user"
)

var phoneRe = regexp.MustCompile(`^\+?[0-9]{9,15}$`)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	ReferralCode string `json:"referral_code"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if msg := validateRegister(req); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}

	res, err := h.svc.Register(r.Context(), RegisterInput{
		Name:         req.Name,
		Phone:        req.Phone,
		Email:        req.Email,
		Password:     req.Password,
		ReferralCode: req.ReferralCode,
	})
	switch {
	case errors.Is(err, ErrInvalidReferral):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, user.ErrDuplicate):
		httpx.Error(w, http.StatusConflict, "email atau nomor telepon sudah terdaftar")
	case err != nil:
		log.Printf("register: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	default:
		httpx.JSON(w, http.StatusCreated, res)
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "email dan password wajib diisi")
		return
	}

	res, err := h.svc.Login(r.Context(), req.Email, req.Password)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Error(w, http.StatusUnauthorized, err.Error())
	case err != nil:
		log.Printf("login: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}

// validateRegister mengembalikan pesan error, atau "" jika input valid.
func validateRegister(req registerRequest) string {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		return "nama wajib diisi (maksimal 100 karakter)"
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(req.Email)); err != nil {
		return "format email tidak valid"
	}
	if !phoneRe.MatchString(strings.TrimSpace(req.Phone)) {
		return "nomor telepon tidak valid (9-15 digit, boleh diawali +)"
	}
	if len(req.Password) < 8 || len(req.Password) > 72 {
		return "password harus 8 sampai 72 karakter"
	}
	return ""
}
