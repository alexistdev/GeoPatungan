/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// DecodeOptional seperti Decode, tetapi body kosong dianggap sah (dst dibiarkan nol).
// Untuk aksi yang parameternya semua opsional, mis. persetujuan tanpa catatan.
func DecodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		Error(w, http.StatusBadRequest, "body JSON tidak valid")
		return false
	}
	return true
}

// Rupiah memformat nominal dengan titik ribuan untuk pesan ke pengguna: 1500000 -> "1.500.000".
func Rupiah(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 && s[0] != '-' || (i > 1 && (len(s)-i)%3 == 0 && s[0] == '-') {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}
