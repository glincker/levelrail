package authengine_test

import (
	"context"
	"testing"
)

func TestSessionsListAndRevokeOwnedSession(t *testing.T) {
	e := newSessEnv(t)
	ctx := context.Background()
	e.user(t, "user_a", "a@example.test")
	e.user(t, "user_b", "b@example.test")
	mine, _ := e.login(t, "a@example.test", sessPassword)
	other, _ := e.login(t, "a@example.test", sessPassword)
	theirs, _ := e.login(t, "b@example.test", sessPassword)

	list, err := e.sess.ListUserSessions(ctx, "user_a", mine)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	currents := 0
	for _, s := range list {
		if s.Current {
			currents++
		}
		if s.UserAgent != "test" || s.IP == "" {
			t.Fatalf("raw context missing: %+v", s)
		}
	}
	if currents != 1 {
		t.Fatalf("want exactly one current session, got %d", currents)
	}
	otherID, _ := e.sess.SessionIDFor(ctx, other)
	theirID, _ := e.sess.SessionIDFor(ctx, theirs)

	tests := []struct {
		name, owner, id string
		want            bool
	}{
		{"another user's session is not found", "user_a", theirID, false},
		{"malformed id is not found", "user_a", "not-a-ulid", false},
		{"own session revokes", "user_a", otherID, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.sess.RevokeUserSession(ctx, tt.owner, tt.id)
			if err != nil || got != tt.want {
				t.Fatalf("revoke = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	if _, ok := e.sess.Lookup(ctx, theirs); !ok {
		t.Fatal("another user's session must survive")
	}
	if _, ok := e.sess.Lookup(ctx, other); ok {
		t.Fatal("revoked session still validates")
	}
}
