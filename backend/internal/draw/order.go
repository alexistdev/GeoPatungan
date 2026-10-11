/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"sort"
	"strings"
)

// Algorithm adalah nama algoritma kocokan berbobot (lihat docs/draw-algorithm.md).
const Algorithm = "weighted_es_v1"

// NewSeed membuat seed acak 32 byte.
func NewSeed(r io.Reader) ([]byte, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

// CommitOf menghitung komitmen (sha256 dari byte mentah seed, dalam hex).
func CommitOf(seed []byte) string {
	h := sha256.Sum256(seed)
	return hex.EncodeToString(h[:])
}

// VerifyCommit memeriksa bahwa seed (hex) yang dibuka cocok dengan komitmen
// yang diumumkan sebelum kocokan.
func VerifyCommit(seedHex, commit string) bool {
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(CommitOf(seed)), []byte(strings.ToLower(commit))) == 1
}

// score = ln(u) / weight, dengan u in (0,1) dari HMAC-SHA256(seed, memberID).
// Mengurutkan skor menurun sama dengan mengurutkan u^(1/weight) menurun
// (Efraimidis-Spirakis), yaitu sampling berbobot tanpa pengembalian.
func score(seed []byte, memberID string, weight float64) float64 {
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(memberID))
	x := binary.BigEndian.Uint64(mac.Sum(nil)[:8]) >> 11 // 53 bit
	u := (float64(x) + 0.5) / float64(uint64(1)<<53)     // tepat di dalam (0,1)
	return math.Log(u) / weight
}

// ComputeOrder menghitung urutan giliran secara deterministik dari seed:
// siapa pun yang memegang seed dan daftar (member, bobot) mendapat hasil yang sama.
// Input tidak diubah. Seri (praktis mustahil) dipecah dengan MemberID.
func ComputeOrder(seed []byte, entries []Entry) ([]Entry, error) {
	out := make([]Entry, len(entries))
	copy(out, entries)
	for i := range out {
		if !(out[i].Weight > 0) {
			return nil, errors.New("bobot harus lebih dari 0")
		}
		s := score(seed, out[i].MemberID, out[i].Weight)
		out[i].Score = &s
	}
	sort.Slice(out, func(a, b int) bool {
		if *out[a].Score != *out[b].Score {
			return *out[a].Score > *out[b].Score
		}
		return out[a].MemberID < out[b].MemberID
	})
	for i := range out {
		o := i + 1
		out[i].Order = &o
	}
	return out, nil
}
