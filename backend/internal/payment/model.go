/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package payment menyatukan modul iuran, putaran, dan wallet: pengguna membayar iuran dari
// saldonya, dan pembayaran terakhir pada sebuah putaran otomatis mencairkan seluruh dana
// putaran itu ke penerima giliran, semuanya dalam satu transaksi.
package payment

import (
	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// InsufficientFundsError adalah alias dari wallet.InsufficientFundsError (dipakai bersama modul lain).
type InsufficientFundsError = wallet.InsufficientFundsError

// Payout adalah pencairan dana putaran ke penerima giliran.
type Payout struct {
	RecipientName string `json:"recipient_name"`
	Amount        int64  `json:"amount"`
}

// RoundOutcome menjelaskan apa yang terjadi pada putaran akibat pembayaran ini.
type RoundOutcome struct {
	// Completed: pembayaran ini adalah yang terakhir, putaran ditutup dan dana dicairkan.
	Completed bool    `json:"completed"`
	GroupDone bool    `json:"group_done"` // putaran terakhir selesai, grup menjadi done
	Payout    *Payout `json:"payout"`
}

// Result adalah hasil pembayaran iuran.
type Result struct {
	Receipt *contribution.Receipt `json:"receipt"`
	Balance int64                 `json:"balance"` // saldo pembayar setelah transaksi
	Round   RoundOutcome          `json:"round"`
}

// QuoteResult adalah rincian pembayaran beserta kecukupan saldo.
type QuoteResult struct {
	Quote      *contribution.Quote `json:"quote"`
	Balance    int64               `json:"balance"`
	Sufficient bool                `json:"sufficient"` // saldo >= total
}
