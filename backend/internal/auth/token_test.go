/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef-test-secret"

func TestTokenRoundTrip(t *testing.T) {
	m := NewTokenManager(testSecret, time.Hour)

	tok, err := m.Generate("user-123", "owner")
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", claims.Subject)
	}
	if claims.Role != "owner" {
		t.Errorf("Role = %q, want owner", claims.Role)
	}
	if claims.ExpiresAt == nil || !claims.ExpiresAt.After(time.Now()) {
		t.Error("ExpiresAt seharusnya di masa depan")
	}
}

func TestTokenRejected(t *testing.T) {
	m := NewTokenManager(testSecret, time.Hour)
	valid, err := m.Generate("user-123", "member")
	if err != nil {
		t.Fatal(err)
	}

	expired, err := NewTokenManager(testSecret, -time.Minute).Generate("user-123", "member")
	if err != nil {
		t.Fatal(err)
	}

	otherSecret, err := NewTokenManager("secret-lain-yang-juga-panjang-32-karakter", time.Hour).Generate("user-123", "member")
	if err != nil {
		t.Fatal(err)
	}

	// payload diubah, tanda tangan tetap lama
	parts := strings.Split(valid, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]

	// penyerang memakai algoritma "none" (tanpa tanda tangan)
	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		Role:             "admin",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-123", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	// algoritma lain (HS512) dengan kunci yang benar tetap harus ditolak
	hs512, err := jwt.NewWithClaims(jwt.SigningMethodHS512, Claims{
		Role:             "member",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-123", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{"kosong", ""},
		{"sampah", "bukan-token"},
		{"kedaluwarsa", expired},
		{"kunci berbeda", otherSecret},
		{"payload dimanipulasi", tampered},
		{"algoritma none", none},
		{"algoritma HS512", hs512},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := m.Parse(tc.token)
			if !errors.Is(err, ErrInvalidToken) {
				t.Errorf("err = %v, want ErrInvalidToken", err)
			}
			if claims != nil {
				t.Error("claims seharusnya nil untuk token tidak valid")
			}
		})
	}
}
