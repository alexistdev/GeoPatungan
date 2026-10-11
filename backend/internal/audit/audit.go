/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package audit mencatat tindakan penting (terutama tindakan admin) ke audit_logs.
package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgconn"
)

// Execer dipenuhi oleh *pgxpool.Pool maupun pgx.Tx, sehingga catatan audit bisa
// ditulis di transaksi yang sama dengan perubahan yang dicatatnya.
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Record menulis satu catatan. actorID dan entityID boleh kosong. meta tidak boleh
// berisi rahasia (password, token) atau data pribadi yang tidak perlu.
func Record(ctx context.Context, db Execer, actorID, action, entity, entityID string, meta map[string]any) error {
	raw := []byte("{}")
	if meta != nil {
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		raw = b
	}
	_, err := db.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, action, entity, entity_id, meta)
		 VALUES (NULLIF($1, '')::uuid, $2, $3, NULLIF($4, '')::uuid, $5::jsonb)`,
		actorID, action, entity, entityID, string(raw))
	return err
}
