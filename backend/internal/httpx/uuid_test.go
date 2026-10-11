/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import "testing"

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
