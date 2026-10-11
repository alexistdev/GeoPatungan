/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strings"
	"testing"
)

func seedN(i int) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(i))
	h := sha256.Sum256(b[:])
	return h[:]
}

func entriesWithWeights(ws ...float64) []Entry {
	out := make([]Entry, len(ws))
	for i, w := range ws {
		out[i] = Entry{MemberID: "member-" + string(rune('a'+i)), Weight: w}
	}
	return out
}

func TestComputeOrderIsDeterministic(t *testing.T) {
	in := entriesWithWeights(1, 2, 3, 1, 2)
	seed := seedN(42)

	a, err := ComputeOrder(seed, in)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ComputeOrder(seed, in)
	for i := range a {
		if a[i].MemberID != b[i].MemberID || *a[i].Score != *b[i].Score {
			t.Fatalf("hasil berbeda pada posisi %d untuk seed yang sama", i)
		}
	}

	// urutan input tidak boleh mempengaruhi hasil
	rev := make([]Entry, len(in))
	for i := range in {
		rev[len(in)-1-i] = in[i]
	}
	c, _ := ComputeOrder(seed, rev)
	for i := range a {
		if a[i].MemberID != c[i].MemberID {
			t.Fatalf("urutan input mempengaruhi hasil (posisi %d)", i)
		}
	}
}

func TestComputeOrderIsPermutationWithSequentialOrders(t *testing.T) {
	in := entriesWithWeights(1, 1, 2, 3, 1, 2, 3, 1)
	out, err := ComputeOrder(seedN(7), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	seen := map[string]bool{}
	for i, e := range out {
		if seen[e.MemberID] {
			t.Fatalf("%s muncul dua kali", e.MemberID)
		}
		seen[e.MemberID] = true
		if e.Order == nil || *e.Order != i+1 {
			t.Errorf("posisi %d punya Order %v, want %d", i, e.Order, i+1)
		}
		if e.Score == nil {
			t.Errorf("posisi %d tidak punya Score", i)
		}
	}
	for i := 1; i < len(out); i++ {
		if *out[i-1].Score < *out[i].Score {
			t.Errorf("skor tidak menurun di posisi %d", i)
		}
	}
}

func TestComputeOrderDoesNotMutateInput(t *testing.T) {
	in := entriesWithWeights(1, 2, 3)
	_, _ = ComputeOrder(seedN(1), in)
	for _, e := range in {
		if e.Score != nil || e.Order != nil {
			t.Error("input seharusnya tidak diubah")
		}
	}
}

func TestComputeOrderDependsOnSeed(t *testing.T) {
	in := entriesWithWeights(1, 1, 1, 1, 1, 1)
	first := map[string]bool{}
	for i := 0; i < 200; i++ {
		out, _ := ComputeOrder(seedN(i), in)
		first[out[0].MemberID] = true
	}
	if len(first) < 5 {
		t.Errorf("hanya %d dari 6 anggota pernah di posisi pertama; seed seharusnya mempengaruhi hasil", len(first))
	}
}

func TestComputeOrderRejectsBadWeights(t *testing.T) {
	for _, w := range []float64{0, -1, math.NaN()} {
		if _, err := ComputeOrder(seedN(1), entriesWithWeights(1, w)); err == nil {
			t.Errorf("bobot %v seharusnya ditolak", w)
		}
	}
}

// Peluang menempati posisi pertama harus sebanding dengan bobot:
// P(i pertama) = w_i / sum(w). Seed deterministik, jadi tes ini stabil.
func TestFirstPlaceProbabilityMatchesWeights(t *testing.T) {
	const runs = 30000
	cases := []struct {
		name    string
		weights []float64
	}{
		{"setara 4 anggota", []float64{1, 1, 1, 1}},
		{"3 banding 1", []float64{3, 1}},
		{"level 1-2-3", []float64{1, 2, 3}},
		{"campuran 6 anggota", []float64{1, 1, 2, 2, 3, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := entriesWithWeights(tc.weights...)
			var total float64
			for _, w := range tc.weights {
				total += w
			}
			wins := map[string]int{}
			for i := 0; i < runs; i++ {
				out, err := ComputeOrder(seedN(i), in)
				if err != nil {
					t.Fatal(err)
				}
				wins[out[0].MemberID]++
			}
			for i, w := range tc.weights {
				want := w / total
				got := float64(wins[in[i].MemberID]) / runs
				if math.Abs(got-want) > 0.015 { // >5 simpangan baku
					t.Errorf("bobot %v: peluang pertama %.3f, want %.3f", w, got, want)
				}
			}
		})
	}
}

// Anggota berbobot tinggi cenderung mendapat giliran lebih awal (rata-rata posisi lebih kecil),
// tetapi anggota berbobot rendah tetap bisa di posisi pertama.
func TestHigherWeightGetsEarlierTurnsOnAverage(t *testing.T) {
	in := entriesWithWeights(1, 3) // a=1, b=3
	var sumPosA, sumPosB float64
	aFirst := 0
	const runs = 5000
	for i := 0; i < runs; i++ {
		out, _ := ComputeOrder(seedN(i), in)
		for _, e := range out {
			if e.MemberID == in[0].MemberID {
				sumPosA += float64(*e.Order)
			} else {
				sumPosB += float64(*e.Order)
			}
		}
		if out[0].MemberID == in[0].MemberID {
			aFirst++
		}
	}
	if !(sumPosB/runs < sumPosA/runs) {
		t.Errorf("rata-rata posisi bobot 3 (%.2f) seharusnya lebih kecil dari bobot 1 (%.2f)", sumPosB/runs, sumPosA/runs)
	}
	if aFirst == 0 {
		t.Error("anggota berbobot rendah seharusnya tetap bisa jadi pertama")
	}
}

func TestCommitAndVerify(t *testing.T) {
	seed := seedN(99)
	commit := CommitOf(seed)

	if len(commit) != 64 {
		t.Fatalf("panjang commit = %d, want 64 hex", len(commit))
	}
	if commit == hex.EncodeToString(seed) {
		t.Error("commit tidak boleh sama dengan seed")
	}
	if !VerifyCommit(hex.EncodeToString(seed), commit) {
		t.Error("seed asli seharusnya cocok dengan komitmennya")
	}
	if !VerifyCommit(hex.EncodeToString(seed), strings.ToUpper(commit)) {
		t.Error("perbandingan seharusnya tidak peka huruf besar/kecil")
	}
	if VerifyCommit(hex.EncodeToString(seedN(100)), commit) {
		t.Error("seed lain seharusnya tidak cocok")
	}
	if VerifyCommit("bukan-hex", commit) {
		t.Error("seed bukan hex seharusnya ditolak")
	}
	if VerifyCommit("", commit) {
		t.Error("seed kosong seharusnya ditolak")
	}
}

func TestWeightForRank(t *testing.T) {
	want := map[int]float64{-1: 1, 0: 1, 1: 1, 2: 2, 3: 3, 4: 3, 99: 3}
	for rank, w := range want {
		if got := WeightForRank(rank); got != w {
			t.Errorf("WeightForRank(%d) = %v, want %v", rank, got, w)
		}
	}
	if WeightForRank(3)/WeightForRank(1) > 3 {
		t.Error("rasio bobot maksimal harus 3:1")
	}
}

func TestNewSeed(t *testing.T) {
	a, err := NewSeed(strings.NewReader(strings.Repeat("x", 64)))
	if err != nil || len(a) != 32 {
		t.Fatalf("len=%d err=%v", len(a), err)
	}
	if _, err := NewSeed(strings.NewReader("pendek")); err == nil {
		t.Error("sumber acak yang kurang dari 32 byte seharusnya menghasilkan error")
	}
}
