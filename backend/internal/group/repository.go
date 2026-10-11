/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const groupColumns = `g.id, g.owner_id, g.name, g.contribution_amount, g.period_type, g.slot_count,
	to_char(g.start_date, 'YYYY-MM-DD'), g.late_fee, g.draw_method, g.min_level_rank, g.status,
	g.invite_code, g.created_at,
	(SELECT count(*) FROM group_members m WHERE m.group_id = g.id AND m.status = 'active')`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func scanGroup(row pgx.Row, withMyStatus bool) (*Group, error) {
	var g Group
	dest := []any{&g.ID, &g.OwnerID, &g.Name, &g.ContributionAmount, &g.PeriodType, &g.SlotCount,
		&g.StartDate, &g.LateFee, &g.DrawMethod, &g.MinLevelRank, &g.Status,
		&g.InviteCode, &g.CreatedAt, &g.ActiveMembers}
	if withMyStatus {
		dest = append(dest, &g.MyStatus)
	}
	if err := row.Scan(dest...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &g, nil
}

// Create menyimpan grup dan langsung mendaftarkan pemiliknya sebagai anggota aktif,
// dalam satu transaksi.
func (r *Repository) Create(ctx context.Context, g *Group) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx,
		`INSERT INTO groups (owner_id, name, contribution_amount, period_type, slot_count,
		                     start_date, late_fee, draw_method, min_level_rank, invite_code)
		 VALUES ($1, $2, $3, $4, $5, $6::date, $7, $8, $9, $10)
		 RETURNING id, status, created_at`,
		g.OwnerID, g.Name, g.ContributionAmount, g.PeriodType, g.SlotCount,
		g.StartDate, g.LateFee, g.DrawMethod, g.MinLevelRank, g.InviteCode,
	).Scan(&g.ID, &g.Status, &g.CreatedAt)
	if err != nil {
		return mapErr(err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO group_members (group_id, user_id, status) VALUES ($1, $2, 'active')`,
		g.ID, g.OwnerID,
	); err != nil {
		return err
	}

	g.ActiveMembers = 1
	g.MyStatus = "active"
	return tx.Commit(ctx)
}

func (r *Repository) FindByID(ctx context.Context, id string) (*Group, error) {
	return scanGroup(r.db.QueryRow(ctx,
		`SELECT `+groupColumns+` FROM groups g WHERE g.id = $1`, id), false)
}

func (r *Repository) FindByInviteCode(ctx context.Context, code string) (*Group, error) {
	return scanGroup(r.db.QueryRow(ctx,
		`SELECT `+groupColumns+` FROM groups g WHERE g.invite_code = $1`, code), false)
}

// ListByUser mengembalikan grup yang diikuti (atau sedang diminta) oleh user.
func (r *Repository) ListByUser(ctx context.Context, userID string) ([]Group, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+groupColumns+`, m.status
		   FROM groups g JOIN group_members m ON m.group_id = g.id
		  WHERE m.user_id = $1 AND m.status <> 'removed'
		  ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []Group{}
	for rows.Next() {
		g, err := scanGroup(rows, true)
		if err != nil {
			return nil, err
		}
		groups = append(groups, *g)
	}
	return groups, rows.Err()
}

// AddMember membuat permintaan bergabung (status pending).
func (r *Repository) AddMember(ctx context.Context, groupID, userID string) (*Member, error) {
	m := Member{GroupID: groupID, UserID: userID}
	err := r.db.QueryRow(ctx,
		`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)
		 RETURNING id, turn_order, status, joined_at`, groupID, userID,
	).Scan(&m.ID, &m.Turn, &m.Status, &m.JoinedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyMember
		}
		return nil, err
	}
	return &m, nil
}

// FindMember mengembalikan keanggotaan user di grup; ErrNotFound jika bukan anggota.
func (r *Repository) FindMember(ctx context.Context, groupID, userID string) (*Member, error) {
	m := Member{GroupID: groupID, UserID: userID}
	err := r.db.QueryRow(ctx,
		`SELECT m.id, u.name, m.turn_order, m.status, m.joined_at
		   FROM group_members m JOIN users u ON u.id = m.user_id
		  WHERE m.group_id = $1 AND m.user_id = $2`, groupID, userID,
	).Scan(&m.ID, &m.UserName, &m.Turn, &m.Status, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repository) ListMembers(ctx context.Context, groupID string) ([]Member, error) {
	rows, err := r.db.Query(ctx,
		`SELECT m.id, m.group_id, m.user_id, u.name, m.turn_order, m.status, m.joined_at
		   FROM group_members m JOIN users u ON u.id = m.user_id
		  WHERE m.group_id = $1 AND m.status <> 'removed'
		  ORDER BY m.joined_at, m.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.UserName, &m.Turn, &m.Status, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// ApproveMember mengaktifkan anggota pending. Baris grup dikunci (FOR UPDATE)
// supaya dua persetujuan bersamaan tidak bisa melewati batas slot.
func (r *Repository) ApproveMember(ctx context.Context, groupID, memberID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slots int
	var status string
	err = tx.QueryRow(ctx,
		`SELECT slot_count, status FROM groups WHERE id = $1 FOR UPDATE`, groupID,
	).Scan(&slots, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "open" {
		return ErrGroupNotOpen
	}

	var active int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM group_members WHERE group_id = $1 AND status = 'active'`, groupID,
	).Scan(&active); err != nil {
		return err
	}
	if active >= slots {
		return ErrGroupFull
	}

	tag, err := tx.Exec(ctx,
		`UPDATE group_members SET status = 'active'
		  WHERE id = $1 AND group_id = $2 AND status = 'pending'`, memberID, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// RejectMember menolak permintaan bergabung yang masih pending.
func (r *Repository) RejectMember(ctx context.Context, groupID, memberID string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE group_members SET status = 'removed'
		  WHERE id = $1 AND group_id = $2 AND status = 'pending'`, memberID, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func mapErr(err error) error {
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}
