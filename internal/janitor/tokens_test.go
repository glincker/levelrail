package janitor

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeToken struct {
	revoked, expires *time.Time
}

// tokenStore applies the same selection rules as the real store.
type tokenStore struct {
	*fakeStore
	toks []fakeToken
}

func (s *tokenStore) PurgeInactiveAPITokens(_ context.Context, kind store.TokenPurgeKind, cutoff time.Time, limit int, dry bool) (int, error) {
	n := 0
	keep := s.toks[:0:0]
	for _, t := range s.toks {
		match := false
		switch kind {
		case store.TokenPurgeRevoked:
			match = t.revoked != nil && t.revoked.Before(cutoff)
		case store.TokenPurgeExpired:
			match = t.revoked == nil && t.expires != nil && t.expires.Before(cutoff)
		}
		if match && n < limit {
			n++
			if dry {
				keep = append(keep, t)
			}
			continue
		}
		keep = append(keep, t)
	}
	s.toks = keep
	return n, nil
}

func TestTokenStage(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	day := 24 * time.Hour
	cases := []struct {
		name        string
		windows     TokenWindows
		toks        []fakeToken
		dry         bool
		wantItems   int
		wantCount   int
		wantRemain  int
		wantAudited bool
	}{
		{name: "active tokens are never deleted", windows: TokenWindows{Revoked: 30, Expired: 30},
			toks: []fakeToken{{}, {expires: ago(-10 * day)}}, wantRemain: 2},
		{name: "keep forever disables the scheduled run", windows: TokenWindows{},
			toks: []fakeToken{{revoked: ago(400 * day)}}, wantRemain: 1},
		{name: "boundary day stays, older goes", windows: TokenWindows{Revoked: 30},
			toks:      []fakeToken{{revoked: ago(30*day - time.Minute)}, {revoked: ago(30*day + time.Minute)}},
			wantItems: 1, wantCount: 1, wantRemain: 1, wantAudited: true},
		{name: "expired window is independent", windows: TokenWindows{Expired: 60},
			toks:      []fakeToken{{expires: ago(61 * day)}, {revoked: ago(61 * day), expires: ago(61 * day)}},
			wantItems: 1, wantCount: 1, wantRemain: 1, wantAudited: true},
		{name: "dry run counts and keeps everything", windows: TokenWindows{Revoked: 30}, dry: true,
			toks:      []fakeToken{{revoked: ago(90 * day)}},
			wantItems: 1, wantCount: 1, wantRemain: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := &tokenStore{fakeStore: &fakeStore{quarantine: map[string]store.JanitorQuarantine{}}, toks: tc.toks}
			var audits []string
			e := New(Deps{
				Store: ts, Docker: &fakeDocker{},
				DiskFree:     func(context.Context) (int64, int64, error) { return 90, 100, nil },
				TokenWindows: func(context.Context) TokenWindows { return tc.windows },
				Audit:        func(_ context.Context, action, _ string) { audits = append(audits, action) },
				Now:          func() time.Time { return now },
			})
			rep, err := e.Run(context.Background(), Options{Trigger: TriggerScheduled, DryRun: tc.dry, Stages: []Stage{StageTokens}})
			if err != nil {
				t.Fatal(err)
			}
			res := stage(t, rep, StageTokens)
			if res.Status != StatusRan || len(res.Items) != tc.wantItems {
				t.Fatalf("status %s items %+v, want %d items", res.Status, res.Items, tc.wantItems)
			}
			if tc.wantItems > 0 && res.Items[0].Count != tc.wantCount {
				t.Errorf("count = %d, want %d", res.Items[0].Count, tc.wantCount)
			}
			if len(ts.toks) != tc.wantRemain {
				t.Errorf("remaining = %d, want %d", len(ts.toks), tc.wantRemain)
			}
			if got := len(audits) == 1 && audits[0] == AuditTokensCleanup; got != tc.wantAudited {
				t.Errorf("audited = %v, want %v (%v)", got, tc.wantAudited, audits)
			}
		})
	}
}

func TestPurgeTokensExplicitZeroRemovesAllRevoked(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	ts := &tokenStore{fakeStore: &fakeStore{}, toks: []fakeToken{{revoked: &past}, {revoked: &past}, {}}}
	n, err := PurgeTokens(context.Background(), ts, store.TokenPurgeRevoked, 0, now, 1, 5, false)
	if err != nil || n != 2 || len(ts.toks) != 1 {
		t.Fatalf("deleted %d, left %d, err %v; want 2 deleted, 1 left", n, len(ts.toks), err)
	}
}
