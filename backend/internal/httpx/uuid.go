/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID memeriksa format UUID, supaya ID sembarang dari URL ditolak
// sebelum sampai ke database.
func IsUUID(s string) bool {
	return uuidRe.MatchString(s)
}

// URLUUID membaca parameter rute chi yang harus berupa UUID. Jika tidak valid,
// langsung menjawab 404 dan mengembalikan false.
func URLUUID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	v := chi.URLParam(r, name)
	if !IsUUID(v) {
		Error(w, http.StatusNotFound, "tidak ditemukan")
		return "", false
	}
	return v, true
}
