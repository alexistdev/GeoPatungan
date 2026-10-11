/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/round"
)

const columns = `c.id, c.round_id, r.round_no, to_char(r.due_date, 'YYYY-MM-DD'),
	c.member_id, u.name, c.amount, c.platform_fee, c.late_fee, c.status, c.paid_on_time, c.paid_at,
	(c.status = 'unpaid' AND r.due_date < (now() AT TIME ZONE 'Asia/Jakarta')::date)`

const from = `FROM contributions c
	JOIN rounds r ON r.id = c.round_id
	JOIN group_members m ON m.id = c.member_id
	JOIN users u ON u.id = m.user_id`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func scan(row pgx.Row, c *Contribution, extra ...any) error {
	dest := []any{&c.ID, &c.RoundID, &c.RoundNo, &c.DueDate, &c.MemberID, &c.UserName,
		&c.Amount, &c.PlatformFee, &c.LateFee, &c.Status, &c.PaidOnTime, &c.PaidAt, &c.Overdue}
	return row.Scan(append(dest, extra...)...)
}

// ListByRound mengembalikan semua iuran sebuah putaran. ErrNotFound jika putaran
// tidak ada atau bukan milik grup tersebut.
func (r *Repository) ListByRound(ctx context.Context, groupID, roundID string) ([]Contribution, error) {
	var exists bool
	if err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM rounds WHERE id = $1 AND group_id = $2)`, roundID, groupID,
	).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, round.ErrNotFound
	}

	rows, err := r.db.Query(ctx,
		`SELECT `+columns+` `+from+` WHERE r.id = $1 ORDER BY u.name, c.id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Contribution{}
	for rows.Next() {
		var c Contribution
		if err := scan(rows, &c); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// ListByUser mengembalikan iuran milik seorang pengguna di semua grupnya.
// status kosong = semua; "unpaid" atau "paid" menyaring. Diurutkan menurut jatuh tempo.
func (r *Repository) ListByUser(ctx context.Context, userID, status string) ([]Mine, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+`, g.id, g.name `+from+`
		   JOIN groups g ON g.id = r.group_id
		  WHERE m.user_id = $1 AND m.status = 'active' AND ($2 = '' OR c.status = $2)
		  ORDER BY r.due_date, g.name, r.round_no
		  LIMIT 500`, userID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Mine{}
	for rows.Next() {
		var m Mine
		if err := scan(rows, &m.Contribution, &m.GroupID, &m.GroupName); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}
