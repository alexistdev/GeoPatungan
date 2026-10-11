/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package settings

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/audit"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Get membaca pengaturan; kunci yang belum pernah diatur memakai nilai bawaan.
func (r *Repository) Get(ctx context.Context) (Settings, error) {
	return Load(ctx, r.db)
}

// Querier dipenuhi *pgxpool.Pool maupun pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Load membaca pengaturan lewat pool atau transaksi, supaya modul uang bisa
// membacanya dalam transaksinya sendiri.
func Load(ctx context.Context, q Querier) (Settings, error) {
	rows, err := q.Query(ctx, `SELECT key, value FROM platform_settings`)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()

	s := Defaults()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Settings{}, err
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Settings{}, fmt.Errorf("pengaturan %s berisi nilai yang tidak valid: %w", key, err)
		}
		switch key {
		case "platform_fee_bps":
			s.PlatformFeeBps = int(n)
		case "topup_min_amount":
			s.TopupMinAmount = n
		case "topup_max_amount":
			s.TopupMaxAmount = n
		case "withdraw_min_amount":
			s.WithdrawMinAmount = n
		case "topup_expiry_hours":
			s.TopupExpiryHours = int(n)
		}
	}
	return s, rows.Err()
}

// Update menyimpan seluruh pengaturan dan mencatat audit dalam satu transaksi.
func (r *Repository) Update(ctx context.Context, adminID string, s Settings) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := save(ctx, tx, adminID, s); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// save menulis pengaturan + audit di transaksi yang diberikan (dipisah dari Update
// supaya tes bisa menjalankannya di transaksi yang di-rollback, tanpa menyentuh
// pengaturan sungguhan).
func save(ctx context.Context, tx pgx.Tx, adminID string, s Settings) error {
	values := map[string]int64{
		"platform_fee_bps":    int64(s.PlatformFeeBps),
		"topup_min_amount":    s.TopupMinAmount,
		"topup_max_amount":    s.TopupMaxAmount,
		"withdraw_min_amount": s.WithdrawMinAmount,
		"topup_expiry_hours":  int64(s.TopupExpiryHours),
	}
	for key, v := range values {
		if _, err := tx.Exec(ctx,
			`INSERT INTO platform_settings (key, value, updated_by, updated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (key) DO UPDATE
			   SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			key, strconv.FormatInt(v, 10), adminID); err != nil {
			return err
		}
	}
	meta := make(map[string]any, len(values))
	for k, v := range values {
		meta[k] = v
	}
	return audit.Record(ctx, tx, adminID, "settings.update", "settings", "", meta)
}

const bankColumns = `id, bank_name, account_number, account_holder, notes, is_active, created_at, updated_at`

func scanBank(row pgx.Row) (*BankAccount, error) {
	var b BankAccount
	err := row.Scan(&b.ID, &b.BankName, &b.AccountNumber, &b.AccountHolder, &b.Notes, &b.IsActive, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) CreateBank(ctx context.Context, adminID string, b *BankAccount) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	got, err := scanBank(tx.QueryRow(ctx,
		`INSERT INTO bank_accounts (bank_name, account_number, account_holder, notes, is_active, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+bankColumns,
		b.BankName, b.AccountNumber, b.AccountHolder, b.Notes, b.IsActive, adminID))
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	*b = *got
	if err := audit.Record(ctx, tx, adminID, "bank_account.create", "bank_account", b.ID,
		map[string]any{"bank_name": b.BankName, "is_active": b.IsActive}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpdateBank(ctx context.Context, adminID, id string, b *BankAccount) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	got, err := scanBank(tx.QueryRow(ctx,
		`UPDATE bank_accounts
		    SET bank_name = $2, account_number = $3, account_holder = $4, notes = $5,
		        is_active = $6, updated_at = now()
		  WHERE id = $1
		 RETURNING `+bankColumns,
		id, b.BankName, b.AccountNumber, b.AccountHolder, b.Notes, b.IsActive))
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	*b = *got
	if err := audit.Record(ctx, tx, adminID, "bank_account.update", "bank_account", id,
		map[string]any{"bank_name": b.BankName, "is_active": b.IsActive}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListBanks mengembalikan rekening; onlyActive=true hanya yang aktif.
func (r *Repository) ListBanks(ctx context.Context, onlyActive bool) ([]BankAccount, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bankColumns+` FROM bank_accounts
		  WHERE ($1 = false OR is_active) ORDER BY bank_name, account_number`, onlyActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []BankAccount{}
	for rows.Next() {
		b, err := scanBank(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *b)
	}
	return list, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
