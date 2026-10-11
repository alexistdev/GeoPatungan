/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func validRegister() registerRequest {
	return registerRequest{
		Name:     "Budi Santoso",
		Phone:    "081234567890",
		Email:    "budi@example.com",
		Password: "password123",
	}
}

func TestValidateRegister(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*registerRequest)
		wantErr bool
	}{
		{"valid", func(r *registerRequest) {}, false},
		{"telepon dengan +62", func(r *registerRequest) { r.Phone = "+6281234567890" }, false},
		{"password tepat 8", func(r *registerRequest) { r.Password = "12345678" }, false},
		{"password tepat 72", func(r *registerRequest) { r.Password = strings.Repeat("a", 72) }, false},
		{"nama kosong", func(r *registerRequest) { r.Name = "" }, true},
		{"nama hanya spasi", func(r *registerRequest) { r.Name = "   " }, true},
		{"nama 101 karakter", func(r *registerRequest) { r.Name = strings.Repeat("a", 101) }, true},
		{"email tanpa @", func(r *registerRequest) { r.Email = "budi.example.com" }, true},
		{"email kosong", func(r *registerRequest) { r.Email = "" }, true},
		{"telepon berisi huruf", func(r *registerRequest) { r.Phone = "0812abc7890" }, true},
		{"telepon terlalu pendek", func(r *registerRequest) { r.Phone = "0812345" }, true},
		{"telepon terlalu panjang", func(r *registerRequest) { r.Phone = "08123456789012345" }, true},
		{"password 7 karakter", func(r *registerRequest) { r.Password = "1234567" }, true},
		{"password 73 karakter", func(r *registerRequest) { r.Password = strings.Repeat("a", 73) }, true},
		{"password kosong", func(r *registerRequest) { r.Password = "" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRegister()
			tc.mutate(&req)
			msg := validateRegister(req)
			if tc.wantErr && msg == "" {
				t.Error("seharusnya ditolak, tetapi lolos validasi")
			}
			if !tc.wantErr && msg != "" {
				t.Errorf("seharusnya lolos, tetapi ditolak: %s", msg)
			}
		})
	}
}

// Semua kasus di bawah ditolak sebelum menyentuh service/database,
// jadi Handler boleh dibuat dengan service nil.
func TestHandlerRejectsBadInputBeforeService(t *testing.T) {
	h := NewHandler(nil)

	tests := []struct {
		name    string
		handler http.HandlerFunc
		body    string
	}{
		{"register: bukan JSON", h.Register, `bukan json`},
		{"register: body kosong", h.Register, ``},
		{"register: field asing", h.Register, `{"name":"A","phone":"081234567890","email":"a@example.com","password":"password123","role":"admin"}`},
		{"register: email salah", h.Register, `{"name":"A","phone":"081234567890","email":"salah","password":"password123"}`},
		{"register: password pendek", h.Register, `{"name":"A","phone":"081234567890","email":"a@example.com","password":"pendek"}`},
		{"login: bukan JSON", h.Login, `{`},
		{"login: email kosong", h.Login, `{"email":"","password":"password123"}`},
		{"login: password kosong", h.Login, `{"email":"a@example.com","password":""}`},
		{"login: field asing", h.Login, `{"email":"a@example.com","password":"x","admin":true}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			tc.handler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body bukan JSON: %v", err)
			}
			if body["error"] == "" {
				t.Error(`body seharusnya berisi {"error": "..."}`)
			}
		})
	}
}
