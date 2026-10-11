/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Perintah admin untuk mengelola peran pengguna. Peran admin sengaja tidak bisa
// diberikan lewat API; hanya operator server yang bisa melakukannya.
//
//	go run ./cmd/admin promote user@example.com
//	go run ./cmd/admin demote  user@example.com
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/alexistdev/geopatungan/internal/config"
	"github.com/alexistdev/geopatungan/internal/database"
	"github.com/alexistdev/geopatungan/internal/user"
)

func main() {
	if len(os.Args) != 3 || (os.Args[1] != "promote" && os.Args[1] != "demote") {
		fmt.Fprintln(os.Stderr, "pemakaian: go run ./cmd/admin <promote|demote> <email>")
		os.Exit(2)
	}
	action, email := os.Args[1], os.Args[2]
	role := "admin"
	if action == "demote" {
		role = "member"
	}

	cfg := config.Load()
	ctx := context.Background()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gagal konek database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := user.NewRepository(db).SetRole(ctx, email, role); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "pengguna dengan email %q tidak ditemukan\n", email)
		} else {
			fmt.Fprintf(os.Stderr, "gagal mengubah peran: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf("%s sekarang berperan %s (berlaku segera, tanpa perlu login ulang)\n", email, role)
}
