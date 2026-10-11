/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package user

import "time"

type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	KYCStatus    string    `json:"kyc_status"`
	ReferralCode string    `json:"referral_code"`
	ReferredBy   *string   `json:"referred_by"`
	CreatedAt    time.Time `json:"created_at"`
}
