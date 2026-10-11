/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package auth

import (
	"strings"
	"testing"
)

func TestNewReferralCode(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := newReferralCode()
		if err != nil {
			t.Fatalf("newReferralCode error: %v", err)
		}
		if len(code) != 8 {
			t.Fatalf("panjang kode = %d, want 8 (%q)", len(code), code)
		}
		for _, c := range code {
			if !strings.ContainsRune(referralAlphabet, c) {
				t.Fatalf("karakter %q di kode %q tidak ada di alfabet", c, code)
			}
		}
		if seen[code] {
			t.Fatalf("kode %q muncul dua kali dalam 1000 percobaan", code)
		}
		seen[code] = true
	}
}

func TestReferralAlphabetHasNoAmbiguousChars(t *testing.T) {
	for _, c := range "0O1I" {
		if strings.ContainsRune(referralAlphabet, c) {
			t.Errorf("alfabet seharusnya tidak memuat %q (mudah tertukar)", c)
		}
	}
}
