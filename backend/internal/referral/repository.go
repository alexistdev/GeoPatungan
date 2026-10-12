/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package referral

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/audit"
)

// Querier dipenuhi *pgxpool.Pool maupun pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Execer dipenuhi *pgxpool.Pool maupun pgx.Tx.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Plan menentukan upline mana (maksimal 4 level) yang berhak menerima komisi atas pembayaran
// payerUserID, beserta bagiannya. Hanya upline yang sudah terverifikasi (KYC) dan pada level yang
// bagiannya > 0. Dibaca di dalam transaksi pembayaran supaya konsisten dengan aturan yang berlaku.
func Plan(ctx context.Context, q Querier, payerUserID string) ([]Beneficiary, error) {
	rows, err := q.Query(ctx,
		`SELECT r.level, r.ancestor_id::text, cr.share_bps
		   FROM referrals r
		   JOIN commission_rules cr ON cr.level = r.level AND cr.share_bps > 0
		   JOIN users u ON u.id = r.ancestor_id AND u.kyc_status = 'verified'
		  WHERE r.descendant_id = $1
		  ORDER BY r.level`, payerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plan []Beneficiary
	for rows.Next() {
		var b Beneficiary
		if err := rows.Scan(&b.Level, &b.UserID, &b.ShareBps); err != nil {
			return nil, err
		}
		plan = append(plan, b)
	}
	return plan, rows.Err()
}

// Record menyimpan komisi yang sudah dibayarkan. Dipanggil di transaksi yang sama dengan pencatatan
// uangnya (wallet.Post). Satu iuran hanya bisa punya satu komisi per level (UNIQUE).
func Record(ctx context.Context, db Execer, contributionID, sourceUserID string, cs []Commission) error {
	for _, c := range cs {
		if _, err := db.Exec(ctx,
			`INSERT INTO referral_commissions (contribution_id, beneficiary_id, source_user_id, level, share_bps, amount)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			contributionID, c.UserID, sourceUserID, c.Level, c.ShareBps, c.Amount); err != nil {
			return err
		}
	}
	return nil
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Rules mengembalikan aturan komisi, terurut menurut level.
func (r *Repository) Rules(ctx context.Context) ([]Rule, error) {
	return loadRules(ctx, r.db)
}

func loadRules(ctx context.Context, q Querier) ([]Rule, error) {
	rows, err := q.Query(ctx, `SELECT level, share_bps FROM commission_rules ORDER BY level`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		var x Rule
		if err := rows.Scan(&x.Level, &x.ShareBps); err != nil {
			return nil, err
		}
		rules = append(rules, x)
	}
	return rules, rows.Err()
}

// UpdateRules mengganti aturan komisi dan mencatat audit dalam satu transaksi. Total bagian dicek
// ulang oleh trigger database saat commit.
func (r *Repository) UpdateRules(ctx context.Context, adminID string, rules []Rule) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := saveRules(ctx, tx, adminID, rules); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// saveRules dipisah dari UpdateRules supaya tes bisa menjalankannya di transaksi yang di-rollback
// tanpa menyentuh aturan sungguhan.
func saveRules(ctx context.Context, tx pgx.Tx, adminID string, rules []Rule) error {
	for _, x := range rules {
		if _, err := tx.Exec(ctx,
			`UPDATE commission_rules SET share_bps = $2, updated_at = now() WHERE level = $1`, x.Level, x.ShareBps); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, adminID, "referral.rules.update", "commission_rules", "", map[string]any{"rules": rules})
}

// Summary mengumpulkan ringkasan referral seorang pengguna.
func (r *Repository) Summary(ctx context.Context, userID string) (*Summary, error) {
	s := &Summary{Earned: EarnedStats{ByLevel: map[string]int64{"1": 0, "2": 0, "3": 0, "4": 0}}}
	var kyc string
	err := r.db.QueryRow(ctx, `SELECT referral_code, kyc_status FROM users WHERE id = $1`, userID).Scan(&s.ReferralCode, &kyc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Eligible = kyc == "verified"

	if s.Rules, err = loadRules(ctx, r.db); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `SELECT level, count(*) FROM referrals WHERE ancestor_id = $1 GROUP BY level`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var level, n int
		if err := rows.Scan(&level, &n); err != nil {
			rows.Close()
			return nil, err
		}
		switch level {
		case 1:
			s.Downline.Level1 = n
		case 2:
			s.Downline.Level2 = n
		case 3:
			s.Downline.Level3 = n
		case 4:
			s.Downline.Level4 = n
		}
		s.Downline.Total += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.db.Query(ctx,
		`SELECT level, COALESCE(SUM(amount), 0)::bigint FROM referral_commissions WHERE beneficiary_id = $1 GROUP BY level`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var level int
		var sum int64
		if err := rows.Scan(&level, &sum); err != nil {
			return nil, err
		}
		s.Earned.ByLevel[strconv.Itoa(level)] = sum
		s.Earned.Total += sum
	}
	return s, rows.Err()
}

// Commissions mengembalikan riwayat komisi yang diterima, terbaru dulu. before=0 berarti dari yang
// terbaru. Mengambil limit+1 baris agar pemanggil tahu apakah masih ada halaman berikutnya. Nama
// sumber dikembalikan utuh; penyamaran dilakukan service.
func (r *Repository) Commissions(ctx context.Context, userID string, limit int, before int64) ([]rawEntry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT c.id, c.level, c.share_bps, c.amount, u.name, c.created_at
		   FROM referral_commissions c JOIN users u ON u.id = c.source_user_id
		  WHERE c.beneficiary_id = $1 AND ($2 = 0 OR c.id < $2)
		  ORDER BY c.id DESC LIMIT $3`, userID, before, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []rawEntry{}
	for rows.Next() {
		var e rawEntry
		if err := rows.Scan(&e.ID, &e.Level, &e.ShareBps, &e.Amount, &e.SourceName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
