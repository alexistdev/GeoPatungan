/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"github.com/alexistdev/geopatungan/internal/user"
)

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrInvalidReferral    = errors.New("kode referral tidak ditemukan")
)

type Service struct {
	users  *user.Repository
	tokens *TokenManager
}

func NewService(users *user.Repository, tokens *TokenManager) *Service {
	return &Service{users: users, tokens: tokens}
}

type RegisterInput struct {
	Name         string
	Phone        string
	Email        string
	Password     string
	ReferralCode string // opsional
}

type AuthResult struct {
	Token string     `json:"token"`
	User  *user.User `json:"user"`
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	u := &user.User{
		Name:  strings.TrimSpace(in.Name),
		Phone: strings.TrimSpace(in.Phone),
		Email: in.Email,
	}

	if code := strings.ToUpper(strings.TrimSpace(in.ReferralCode)); code != "" {
		referrer, err := s.users.FindByReferralCode(ctx, code)
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidReferral
		}
		if err != nil {
			return nil, err
		}
		u.ReferredBy = &referrer.ID
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = hash

	// Kode referral acak bisa (jarang) bentrok; coba ulang beberapa kali.
	for attempt := 0; attempt < 5; attempt++ {
		u.ReferralCode, err = newReferralCode()
		if err != nil {
			return nil, err
		}
		err = s.users.Create(ctx, u)
		if !errors.Is(err, user.ErrDuplicate) {
			break
		}
		if s.emailOrPhoneTaken(ctx, in.Email) {
			return nil, user.ErrDuplicate
		}
	}
	if err != nil {
		return nil, err
	}

	token, err := s.tokens.Generate(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	return &AuthResult{Token: token, User: u}, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	u, err := s.users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, user.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !CheckPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	token, err := s.tokens.Generate(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	return &AuthResult{Token: token, User: u}, nil
}

func (s *Service) emailOrPhoneTaken(ctx context.Context, email string) bool {
	_, err := s.users.FindByEmail(ctx, email)
	return err == nil
}

// Tanpa huruf/angka yang mudah tertukar (0/O, 1/I).
const referralAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newReferralCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(referralAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = referralAlphabet[n.Int64()]
	}
	return string(b), nil
}
