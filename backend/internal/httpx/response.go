/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import (
	"encoding/json"
	"net/http"
)

// ValidationError adalah kesalahan input dari pengguna (dipetakan ke HTTP 400).
// Dipakai bersama oleh semua modul.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Decode membaca body JSON. Mengembalikan false (dan sudah menulis respons 400)
// jika body tidak valid, terlalu besar, atau berisi field yang tidak dikenal.
func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // maks 1 MB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		Error(w, http.StatusBadRequest, "body JSON tidak valid")
		return false
	}
	return true
}
