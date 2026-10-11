/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestIsUUID(t *testing.T) {
	valid := []string{
		"123e4567-e89b-12d3-a456-426614174000",
		"123E4567-E89B-12D3-A456-426614174000",
		"00000000-0000-0000-0000-000000000000",
	}
	invalid := []string{
		"",
		"abc",
		"123e4567e89b12d3a456426614174000",
		"123e4567-e89b-12d3-a456-42661417400",   // kurang 1
		"123e4567-e89b-12d3-a456-4266141740000", // lebih 1
		"123e4567-e89b-12d3-a456-42661417400g",  // bukan hex
		"123e4567-e89b-12d3-a456-426614174000\n",
		" 123e4567-e89b-12d3-a456-426614174000",
		"1' OR '1'='1",
	}
	for _, s := range valid {
		if !IsUUID(s) {
			t.Errorf("IsUUID(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if IsUUID(s) {
			t.Errorf("IsUUID(%q) = true, want false", s)
		}
	}
}

func TestURLUUID(t *testing.T) {
	var got string
	var ok bool
	r := chi.NewRouter()
	r.Get("/x/{id}", func(w http.ResponseWriter, req *http.Request) {
		got, ok = URLUUID(w, req, "id")
	})

	t.Run("valid", func(t *testing.T) {
		got, ok = "", false
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x/123e4567-e89b-12d3-a456-426614174000", nil))
		if !ok || got != "123e4567-e89b-12d3-a456-426614174000" {
			t.Errorf("ok=%v got=%q", ok, got)
		}
		if rec.Body.Len() != 0 {
			t.Error("jalur valid tidak boleh menulis respons")
		}
	})

	t.Run("tidak valid -> 404 JSON", func(t *testing.T) {
		got, ok = "", true
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x/abc", nil))
		if ok {
			t.Error("ok seharusnya false")
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("body = %s, want JSON error", rec.Body.String())
		}
	})
}
