/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("rahasia-banget-123")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "rahasia-banget-123" {
		t.Fatal("hash tidak boleh sama dengan password asli")
	}
	if !CheckPassword(hash, "rahasia-banget-123") {
		t.Error("password yang benar seharusnya cocok")
	}
	if CheckPassword(hash, "salah-password") {
		t.Error("password yang salah seharusnya tidak cocok")
	}
	if CheckPassword(hash, "") {
		t.Error("password kosong seharusnya tidak cocok")
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	h1, err := HashPassword("password-sama")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashPassword("password-sama")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("dua hash dari password yang sama seharusnya berbeda (salt acak)")
	}
}

func TestCheckPasswordInvalidHash(t *testing.T) {
	if CheckPassword("bukan-hash-bcrypt", "apa-saja") {
		t.Error("hash tidak valid seharusnya ditolak, bukan panic atau lolos")
	}
}
