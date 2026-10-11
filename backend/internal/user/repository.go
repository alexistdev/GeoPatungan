/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package user

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("user tidak ditemukan")
	ErrDuplicate = errors.New("email, telepon, atau kode referral sudah dipakai")
)

const columns = `id, name, phone, email, password_hash, role, kyc_status, referral_code, referred_by, created_at`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	return r.findOne(ctx, "id", id)
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.findOne(ctx, "email", email)
}

func (r *Repository) FindByReferralCode(ctx context.Context, code string) (*User, error) {
	return r.findOne(ctx, "referral_code", code)
}

// column hanya diisi konstanta dari dalam paket ini, bukan input pengguna.
func (r *Repository) findOne(ctx context.Context, column, value string) (*User, error) {
	var u User
	err := r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM users WHERE `+column+` = $1`, value,
	).Scan(&u.ID, &u.Name, &u.Phone, &u.Email, &u.PasswordHash, &u.Role,
		&u.KYCStatus, &u.ReferralCode, &u.ReferredBy, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Create menyimpan user baru dan, jika ada pengajak, mengisi tabel referrals
// (maksimal 4 level) dalam satu transaksi: keduanya berhasil atau keduanya batal.
func (r *Repository) Create(ctx context.Context, u *User) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx,
		`INSERT INTO users (name, phone, email, password_hash, referral_code, referred_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, role, kyc_status, created_at`,
		u.Name, u.Phone, u.Email, u.PasswordHash, u.ReferralCode, u.ReferredBy,
	).Scan(&u.ID, &u.Role, &u.KYCStatus, &u.CreatedAt)
	if err != nil {
		return mapErr(err)
	}

	if u.ReferredBy != nil {
		_, err = tx.Exec(ctx,
			`INSERT INTO referrals (descendant_id, ancestor_id, level)
			 SELECT $1::uuid, ancestor_id, level + 1 FROM referrals
			  WHERE descendant_id = $2::uuid AND level < 4
			 UNION ALL
			 SELECT $1::uuid, $2::uuid, 1`,
			u.ID, *u.ReferredBy,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func mapErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return ErrDuplicate
	}
	return err
}
