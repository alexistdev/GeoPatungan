/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusCreated, map[string]int{"n": 1})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"n":1}` {
		t.Errorf("body = %s", got)
	}
}

func TestError(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusConflict, "sudah ada")

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"sudah ada"}` {
		t.Errorf("body = %s", got)
	}
}

func TestDecode(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("valid", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Ani"}`))
		var p payload
		if !Decode(rec, req, &p) {
			t.Fatal("Decode seharusnya berhasil")
		}
		if p.Name != "Ani" {
			t.Errorf("Name = %q, want Ani", p.Name)
		}
	})

	invalid := map[string]string{
		"JSON rusak":    `{"name":`,
		"field asing":   `{"name":"Ani","role":"admin"}`,
		"kosong":        ``,
		"terlalu besar": `{"name":"` + strings.Repeat("a", 2<<20) + `"}`,
	}
	for name, body := range invalid {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			var p payload
			if Decode(rec, req, &p) {
				t.Fatal("Decode seharusnya gagal")
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestUserContext(t *testing.T) {
	ctx := WithUser(context.Background(), "u-1", "admin")
	if got := UserID(ctx); got != "u-1" {
		t.Errorf("UserID = %q, want u-1", got)
	}
	if got := Role(ctx); got != "admin" {
		t.Errorf("Role = %q, want admin", got)
	}

	empty := context.Background()
	if UserID(empty) != "" || Role(empty) != "" {
		t.Error("context kosong seharusnya mengembalikan string kosong, bukan panic")
	}
}
