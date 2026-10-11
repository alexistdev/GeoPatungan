/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexistdev/geopatungan/internal/auth"
	"github.com/alexistdev/geopatungan/internal/httpx"
)

const secret = "0123456789abcdef0123456789abcdef-test-secret"

func TestAuthenticate(t *testing.T) {
	tokens := auth.NewTokenManager(secret, time.Hour)
	valid, err := tokens.Generate("user-42", "owner")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := auth.NewTokenManager(secret, -time.Minute).Generate("user-42", "owner")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := auth.NewTokenManager("kunci-lain-yang-juga-panjang-sekali-32", time.Hour).Generate("user-42", "owner")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantNext   bool
	}{
		{"tanpa header", "", http.StatusUnauthorized, false},
		{"skema bukan Bearer", "Basic " + valid, http.StatusUnauthorized, false},
		{"Bearer tanpa token", "Bearer ", http.StatusUnauthorized, false},
		{"token tanpa kata Bearer", valid, http.StatusUnauthorized, false},
		{"token sampah", "Bearer sampah", http.StatusUnauthorized, false},
		{"token kedaluwarsa", "Bearer " + expired, http.StatusUnauthorized, false},
		{"token kunci lain", "Bearer " + foreign, http.StatusUnauthorized, false},
		{"token valid", "Bearer " + valid, http.StatusOK, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var nextCalled bool
			var gotID, gotRole string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				gotID = httpx.UserID(r.Context())
				gotRole = httpx.Role(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			Authenticate(tokens)(next).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if nextCalled != tc.wantNext {
				t.Errorf("next dipanggil = %v, want %v", nextCalled, tc.wantNext)
			}
			if tc.wantNext && (gotID != "user-42" || gotRole != "owner") {
				t.Errorf("context berisi (%q, %q), want (user-42, owner)", gotID, gotRole)
			}
		})
	}
}
