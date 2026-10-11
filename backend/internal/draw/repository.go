/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/round"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// lockGroup mengunci baris grup (FOR UPDATE) selama transaksi dan mengembalikan
// jumlah slot serta statusnya.
func lockGroup(ctx context.Context, tx pgx.Tx, groupID string) (slots int, status string, err error) {
	err = tx.QueryRow(ctx,
		`SELECT slot_count, status FROM groups WHERE id = $1 FOR UPDATE`, groupID,
	).Scan(&slots, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", group.ErrNotFound
	}
	return slots, status, err
}

// lockOpenGroup seperti lockGroup, tetapi menolak grup yang tidak lagi open.
func lockOpenGroup(ctx context.Context, tx pgx.Tx, groupID string) (int, error) {
	slots, status, err := lockGroup(ctx, tx, groupID)
	if err != nil {
		return 0, err
	}
	if status != "open" {
		return 0, ErrGroupStarted
	}
	return slots, nil
}

// requireExactlyActiveMembers memastikan grup penuh dan daftar entri persis sama
// dengan anggota aktif (tidak lebih, tidak kurang).
func requireExactlyActiveMembers(ctx context.Context, tx pgx.Tx, groupID string, slots int, entries []Entry) error {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.MemberID
	}
	var active, matched int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE status = 'active'),
		        count(*) FILTER (WHERE status = 'active' AND id = ANY($2::uuid[]))
		   FROM group_members WHERE group_id = $1`, groupID, ids,
	).Scan(&active, &matched)
	if err != nil {
		return err
	}
	if active != slots {
		return ErrGroupNotFull
	}
	if len(entries) != slots || matched != slots {
		return ErrMembersChanged
	}
	return nil
}

// Commit menyimpan kocokan (seed rahasia + hash-nya) beserta bobot tiap anggota.
// Seed dan bobot terkunci sejak saat ini.
func (r *Repository) Commit(ctx context.Context, d *Draw) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	slots, err := lockOpenGroup(ctx, tx, d.GroupID)
	if err != nil {
		return err
	}
	if err := requireExactlyActiveMembers(ctx, tx, d.GroupID, slots, d.Entries); err != nil {
		return err
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO group_draws (group_id, algorithm, seed, seed_commit)
		 VALUES ($1, $2, $3, $4) RETURNING id, committed_at`,
		d.GroupID, d.Algorithm, d.seed, d.SeedCommit,
	).Scan(&d.ID, &d.CommittedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyCommitted
		}
		return err
	}
	for _, e := range d.Entries {
		if _, err := tx.Exec(ctx,
			`INSERT INTO group_draw_entries (draw_id, member_id, weight) VALUES ($1, $2, $3)`,
			d.ID, e.MemberID, e.Weight); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Execute menyimpan hasil urutan, mengisi turn_order tiap anggota, membuka seed,
// dan mengubah status grup menjadi running, semuanya atomik.
func (r *Repository) Execute(ctx context.Context, groupID string, ordered []Entry) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, status, err := lockGroup(ctx, tx, groupID)
	if err != nil {
		return err
	}

	var drawID string
	var executed bool
	err = tx.QueryRow(ctx,
		`SELECT id, executed_at IS NOT NULL FROM group_draws WHERE group_id = $1 FOR UPDATE`, groupID,
	).Scan(&drawID, &executed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoDraw
	}
	if err != nil {
		return err
	}
	if executed {
		return ErrAlreadyExecuted // dicek sebelum status grup agar pesannya tepat
	}
	if status != "open" {
		return ErrGroupStarted
	}

	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM group_draw_entries WHERE draw_id = $1`, drawID).Scan(&n); err != nil {
		return err
	}
	if n != len(ordered) {
		return ErrMembersChanged
	}

	for _, e := range ordered {
		tag, err := tx.Exec(ctx,
			`UPDATE group_draw_entries SET score = $3, resulting_order = $4
			  WHERE draw_id = $1 AND member_id = $2`, drawID, e.MemberID, e.Score, e.Order)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrMembersChanged
		}
		if _, err := tx.Exec(ctx,
			`UPDATE group_members SET turn_order = $3 WHERE id = $2 AND group_id = $1`,
			groupID, e.MemberID, e.Order); err != nil {
			return err
		}
	}
	if err := round.CreateSchedule(ctx, tx, groupID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE group_draws SET executed_at = now() WHERE id = $1`, drawID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE groups SET status = 'running' WHERE id = $1`, groupID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Manual menyimpan urutan yang ditentukan pemilik grup dan langsung memulai grup.
func (r *Repository) Manual(ctx context.Context, d *Draw) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	slots, err := lockOpenGroup(ctx, tx, d.GroupID)
	if err != nil {
		return err
	}
	if err := requireExactlyActiveMembers(ctx, tx, d.GroupID, slots, d.Entries); err != nil {
		return err
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO group_draws (group_id, algorithm, executed_at)
		 VALUES ($1, 'manual', now()) RETURNING id, committed_at, executed_at`, d.GroupID,
	).Scan(&d.ID, &d.CommittedAt, &d.ExecutedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyCommitted
		}
		return err
	}
	for _, e := range d.Entries {
		if _, err := tx.Exec(ctx,
			`INSERT INTO group_draw_entries (draw_id, member_id, weight, resulting_order)
			 VALUES ($1, $2, 1, $3)`, d.ID, e.MemberID, e.Order); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE group_members SET turn_order = $3 WHERE id = $2 AND group_id = $1`,
			d.GroupID, e.MemberID, e.Order); err != nil {
			return err
		}
	}
	if err := round.CreateSchedule(ctx, tx, d.GroupID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE groups SET status = 'running' WHERE id = $1`, d.GroupID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Get mengambil kocokan grup. Seed hanya dimasukkan ke SeedReveal jika sudah dijalankan;
// field internal seed selalu terisi agar Service bisa menghitung urutan.
func (r *Repository) Get(ctx context.Context, groupID string) (*Draw, error) {
	d := Draw{GroupID: groupID, Entries: []Entry{}}
	var seed, commit *string
	err := r.db.QueryRow(ctx,
		`SELECT id, algorithm, seed, seed_commit, committed_at, executed_at
		   FROM group_draws WHERE group_id = $1`, groupID,
	).Scan(&d.ID, &d.Algorithm, &seed, &commit, &d.CommittedAt, &d.ExecutedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoDraw
	}
	if err != nil {
		return nil, err
	}
	if seed != nil {
		d.seed = *seed
		if d.ExecutedAt != nil {
			d.SeedReveal = *seed
		}
	}
	if commit != nil {
		d.SeedCommit = *commit
	}

	rows, err := r.db.Query(ctx,
		`SELECT e.member_id, u.name, e.weight, e.score, e.resulting_order
		   FROM group_draw_entries e
		   JOIN group_members m ON m.id = e.member_id
		   JOIN users u ON u.id = m.user_id
		  WHERE e.draw_id = $1
		  ORDER BY e.resulting_order NULLS LAST, u.name, e.member_id`, d.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.MemberID, &e.UserName, &e.Weight, &e.Score, &e.Order); err != nil {
			return nil, err
		}
		d.Entries = append(d.Entries, e)
	}
	return &d, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
