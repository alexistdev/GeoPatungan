/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"context"
	"errors"
	"testing"
)

func TestRequireActiveMember(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := newTestService(store, 1)
	g, err := svc.Create(ctx, "owner", validInput())
	if err != nil {
		t.Fatal(err)
	}
	// pending
	if _, err := svc.Join(ctx, "pending", g.InviteCode); err != nil {
		t.Fatal(err)
	}
	// ditolak
	if _, err := svc.Join(ctx, "rejected", g.InviteCode); err != nil {
		t.Fatal(err)
	}
	rej, _ := store.FindMember(ctx, g.ID, "rejected")
	if err := svc.Reject(ctx, "owner", g.ID, rej.ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		user string
		want error
	}{
		{"owner", nil},
		{"pending", ErrForbidden},
		{"rejected", ErrNotFound},
		{"stranger", ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.user, func(t *testing.T) {
			err := RequireActiveMember(ctx, store, g.ID, tc.user)
			if !errors.Is(err, tc.want) && !(tc.want == nil && err == nil) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
