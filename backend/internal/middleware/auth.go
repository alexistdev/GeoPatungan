/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package middleware

import (
	"net/http"
	"strings"

	"github.com/alexistdev/geopatungan/internal/auth"
	"github.com/alexistdev/geopatungan/internal/httpx"
)

// Authenticate menolak request tanpa header "Authorization: Bearer <token>" yang valid,
// dan menyimpan ID serta role pengguna ke context untuk handler berikutnya.
func Authenticate(tokens *auth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			raw, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || raw == "" {
				httpx.Error(w, http.StatusUnauthorized, "token diperlukan")
				return
			}
			claims, err := tokens.Parse(raw)
			if err != nil || claims.Subject == "" {
				httpx.Error(w, http.StatusUnauthorized, "token tidak valid atau kedaluwarsa")
				return
			}
			next.ServeHTTP(w, r.WithContext(httpx.WithUser(r.Context(), claims.Subject, claims.Role)))
		})
	}
}
