/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package notification

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Execer dipenuhi *pgxpool.Pool maupun pgx.Tx.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func dataJSON(d map[string]any) (string, error) {
	if d == nil {
		return "{}", nil
	}
	b, err := json.Marshal(d)
	return string(b), err
}

// Send menyimpan satu pemberitahuan. Panggil di transaksi yang sama dengan kejadiannya.
func Send(ctx context.Context, db Execer, n Notice) error {
	if err := n.Validate(); err != nil {
		return err
	}
	data, err := dataJSON(n.Data)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`INSERT INTO notifications (user_id, type, title, body, data) VALUES ($1, $2, $3, $4, $5::jsonb)`,
		n.UserID, n.Type, n.Title, n.Body, data)
	return err
}

// SendToGroupMembers mengirim pemberitahuan yang sama ke semua anggota AKTIF sebuah grup.
// n.UserID diabaikan.
func SendToGroupMembers(ctx context.Context, db Execer, groupID string, n Notice) error {
	n.UserID = "-" // lolos validasi; diganti per anggota oleh SQL
	if err := n.Validate(); err != nil {
		return err
	}
	data, err := dataJSON(n.Data)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`INSERT INTO notifications (user_id, type, title, body, data)
		 SELECT user_id, $2, $3, $4, $5::jsonb FROM group_members WHERE group_id = $1 AND status = 'active'`,
		groupID, n.Type, n.Title, n.Body, data)
	return err
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// List mengembalikan pemberitahuan pengguna, terbaru dulu. before=0 berarti dari yang terbaru.
// Mengambil limit+1 baris supaya pemanggil tahu apakah masih ada halaman berikutnya.
func (r *Repository) List(ctx context.Context, userID string, limit int, before int64, unreadOnly bool) ([]Item, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, type, title, body, data, read_at IS NOT NULL, created_at
		   FROM notifications
		  WHERE user_id = $1 AND ($2::bigint = 0 OR id < $2) AND (NOT $3 OR read_at IS NULL)
		  ORDER BY id DESC LIMIT $4`, userID, before, unreadOnly, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		var raw []byte
		if err := rows.Scan(&it.ID, &it.Type, &it.Title, &it.Body, &raw, &it.Read, &it.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &it.Data); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UnreadCount menghitung pemberitahuan yang belum dibaca.
func (r *Repository) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// MarkRead menandai satu pemberitahuan milik userID sebagai dibaca. Idempoten (waktu baca pertama
// dipertahankan). Milik orang lain atau yang tidak ada -> ErrNotFound.
func (r *Repository) MarkRead(ctx context.Context, userID string, id int64) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead menandai semua pemberitahuan pengguna sebagai dibaca; mengembalikan jumlah yang berubah.
func (r *Repository) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
