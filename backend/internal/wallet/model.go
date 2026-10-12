/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package wallet adalah pembukuan uang: dompet, buku besar berpasangan (double-entry),
// dan rekonsiliasi. Semua nominal dalam rupiah utuh (BIGINT).
//
// Satu-satunya cara mengubah saldo adalah Post, yang mencatat peristiwa uang sebagai
// beberapa baris bertanda (+/-) berjumlah 0. Karena itu jumlah seluruh saldo di sistem
// selalu 0: uang yang masuk/keluar dicatat terhadap dompet "external".
package wallet

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInsufficientFunds = errors.New("saldo tidak cukup")
	ErrInvalidLegs       = errors.New("catatan uang tidak valid")
	ErrAlreadyPosted     = errors.New("peristiwa ini sudah pernah dicatat")
	ErrWalletNotFound    = errors.New("dompet tidak ditemukan")
)

// InsufficientFundsError menyertakan berapa yang dibutuhkan dan berapa saldo saat ini, supaya
// klien bisa menampilkan kekurangannya. errors.Is(err, ErrInsufficientFunds) bernilai true.
type InsufficientFundsError struct {
	Required int64 `json:"required"`
	Balance  int64 `json:"balance"`
}

func (e *InsufficientFundsError) Error() string {
	return fmt.Sprintf("saldo tidak cukup: dibutuhkan %d, saldo %d", e.Required, e.Balance)
}

func (e *InsufficientFundsError) Unwrap() error { return ErrInsufficientFunds }

// Jenis dompet sistem (selain dompet pengguna).
const (
	TypeUser     = "user"
	TypeEscrow   = "escrow"
	TypePlatform = "platform"
	TypeHolding  = "holding"
	TypeExternal = "external"
)

type Wallet struct {
	ID        string    `json:"id"`
	Balance   int64     `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
}

// Entry adalah satu baris riwayat mutasi dompet. Amount positif = uang masuk.
type Entry struct {
	ID           int64     `json:"id"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	RefType      string    `json:"ref_type"`
	RefID        string    `json:"ref_id"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
}

// Leg adalah satu sisi catatan: perubahan saldo satu dompet. Amount bertanda.
type Leg struct {
	WalletID string
	Amount   int64
}

// Ref mengidentifikasi peristiwa bisnis penyebab catatan, mis. ("topup", id permintaan).
// Pasangan (Type, ID, WalletID) unik di database, sehingga peristiwa yang sama tidak
// bisa tercatat dua kali pada dompet yang sama.
type Ref struct {
	Type        string
	ID          string
	Description string
}
