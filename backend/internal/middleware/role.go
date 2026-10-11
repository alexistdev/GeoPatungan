/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package middleware

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/user"
)

// RoleLookup dipenuhi *user.Repository.
type RoleLookup interface {
	FindByID(ctx context.Context, id string) (*user.User, error)
}

// RequireRole hanya meloloskan pengguna yang perannya SAAT INI (dibaca dari database,
// bukan dari token) ada di daftar roles. Dengan begitu pencabutan hak admin langsung
// berlaku, tidak menunggu token kedaluwarsa. Harus dipasang setelah Authenticate.
func RequireRole(users RoleLookup, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := httpx.UserID(r.Context())
			if id == "" {
				httpx.Error(w, http.StatusUnauthorized, "token diperlukan")
				return
			}
			u, err := users.FindByID(r.Context(), id)
			switch {
			case errors.Is(err, user.ErrNotFound):
				httpx.Error(w, http.StatusUnauthorized, "akun tidak ditemukan")
				return
			case err != nil:
				log.Printf("require role: %v", err)
				httpx.Error(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
				return
			}
			if !slices.Contains(roles, u.Role) {
				httpx.Error(w, http.StatusForbidden, "akses ditolak")
				return
			}
			next.ServeHTTP(w, r.WithContext(httpx.WithUser(r.Context(), id, u.Role)))
		})
	}
}
