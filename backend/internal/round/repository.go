/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// ListByGroup mengembalikan seluruh putaran grup berurutan, beserta ringkasan iuran.
func (r *Repository) ListByGroup(ctx context.Context, groupID string) ([]Round, error) {
	rows, err := r.db.Query(ctx,
		`SELECT r.id, r.group_id, r.round_no, to_char(r.due_date, 'YYYY-MM-DD'),
		        r.recipient_member_id, u.name, r.status,
		        count(c.id) FILTER (WHERE c.status = 'paid'), count(c.id), COALESCE(sum(c.amount), 0)
		   FROM rounds r
		   JOIN group_members m ON m.id = r.recipient_member_id
		   JOIN users u ON u.id = m.user_id
		   LEFT JOIN contributions c ON c.round_id = r.id
		  WHERE r.group_id = $1
		  GROUP BY r.id, u.name
		  ORDER BY r.round_no`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rounds := []Round{}
	for rows.Next() {
		var x Round
		if err := rows.Scan(&x.ID, &x.GroupID, &x.RoundNo, &x.DueDate, &x.RecipientMemberID,
			&x.RecipientName, &x.Status, &x.PaidCount, &x.MemberCount, &x.PotAmount); err != nil {
			return nil, err
		}
		rounds = append(rounds, x)
	}
	return rounds, rows.Err()
}
