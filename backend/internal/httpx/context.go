/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package httpx

import "context"

type ctxKey string

const (
	userIDKey ctxKey = "user_id"
	roleKey   ctxKey = "role"
)

// WithUser menyimpan identitas pengguna yang sudah terautentikasi ke context.
func WithUser(ctx context.Context, userID, role string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	return context.WithValue(ctx, roleKey, role)
}

func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

func Role(ctx context.Context) string {
	v, _ := ctx.Value(roleKey).(string)
	return v
}
