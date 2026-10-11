/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import "context"

// MemberFinder adalah bagian dari Store yang dibutuhkan untuk memeriksa keanggotaan.
type MemberFinder interface {
	FindMember(ctx context.Context, groupID, userID string) (*Member, error)
}

// RequireActiveMember memastikan userID adalah anggota aktif grup (termasuk pemilik).
// Orang luar atau anggota yang ditolak mendapat ErrNotFound (keberadaan grup tidak
// bocor); anggota yang masih pending mendapat ErrForbidden.
func RequireActiveMember(ctx context.Context, f MemberFinder, groupID, userID string) error {
	m, err := f.FindMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	switch m.Status {
	case "active":
		return nil
	case "removed":
		return ErrNotFound
	default:
		return ErrForbidden
	}
}
