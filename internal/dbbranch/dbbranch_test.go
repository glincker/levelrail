package dbbranch

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/diskguard"
	"github.com/glincker/levelrail/internal/store"
)

func TestTransitions(t *testing.T) {
	ok := [][2]string{{StateRequested, StateCloning}, {StateCloning, StateMasking}, {StateCloning, StateCredentials}, {StateVerifying, StateReady}, {StateReady, StateResetting}, {StateResetting, StateReady}, {StateReady, StatePromoting}, {StateCloning, StateFailed}, {StateFailed, StateDeleting}}
	for _, p := range ok {
		if !CanTransition(p[0], p[1]) {
			t.Errorf("%s to %s should be allowed", p[0], p[1])
		}
	}
	bad := [][2]string{{StateRequested, StateReady}, {StateReady, StateFailed}, {StateDeleted, StateReady}, {StateMasking, StateReady}, {StatePromoted, StateDeleting}}
	for _, p := range bad {
		if CanTransition(p[0], p[1]) {
			t.Errorf("%s to %s should be refused", p[0], p[1])
		}
	}
	if Reachable(StateCloning) || !Reachable(StateReady) {
		t.Error("reachability wrong")
	}
	if err := Move(t.Context(), nil, "x", StateRequested, StateReady, "", ""); !errors.Is(err, ErrBadTransition) {
		t.Errorf("Move = %v", err)
	}
}

func TestCapabilityMatrix(t *testing.T) {
	cases := []struct {
		engine, mode string
		want         bool
	}{
		{"postgres", ModeFull, true}, {"postgres", ModePITR, false}, {"postgres", ModeAnonymized, true},
		{"mysql", ModePITR, false}, {"documentdb", ModeFull, false}, {"mariadb", ModeSchema, true},
		{"mongodb", ModePITR, false}, {"mongodb", ModeAnonymized, true},
		{"redis", ModeFull, true}, {"valkey", ModeSchema, false}, {"keydb", ModeAnonymized, false},
		{"clickhouse", ModeFull, false}, {"memcached", ModeFull, false}, {"unknown", ModeFull, false},
	}
	for _, c := range cases {
		got, reason := ModeAvailable(c.engine, c.mode, Hints{})
		if got != c.want || (!got && reason == "") {
			t.Errorf("%s %s = %v %q", c.engine, c.mode, got, reason)
		}
	}
	if ok, _ := ModeAvailable("mariadb", ModePITR, Hints{PITREnabled: true, MySQLPITRRunner: true}); !ok {
		t.Error("pitr should open when the database reports it")
	}
	if ok, why := ModeAvailable("mysql", ModePITR, Hints{PITREnabled: true}); ok || why == "" {
		t.Errorf("pitr without a runner = %v %q", ok, why)
	}
	if _, why := ModeAvailable("postgres", ModePITR, Hints{PITREnabled: true, MySQLPITRRunner: true}); !strings.Contains(why, "F08") {
		t.Errorf("postgres pitr reason = %q", why)
	}
	if CapabilityFor("clickhouse", Hints{}).Branches {
		t.Error("clickhouse must be refused")
	}
}

func guardWithFree(free int64) *diskguard.Guard {
	stat := func(string) (int64, int64, error) { return free, 1 << 50, nil }
	return diskguard.New("", "/docker", diskguard.Thresholds{LowFree: 1 << 20, CriticalFree: 1 << 19, Reserve: 1 << 20}, stat, nil)
}

func TestCheckQuota(t *testing.T) {
	l := Limits{MaxPerRoot: 2, MaxTotal: 10, MaxTotalBytes: 1000, MaxDepth: 2, SlowPathMaxBytes: 500, Headroom: 2}
	root := func(name string) store.DatabaseBranch {
		return store.DatabaseBranch{DatabaseName: name, ParentName: "app", RootName: "app", State: StateReady, SizeBytes: 100}
	}
	g := guardWithFree(1 << 40)
	req := Request{ParentIsRoot: true, RootName: "app", ParentBytes: 100, SlowPath: true}
	if err := CheckQuota(l, req, nil, g); err != nil {
		t.Fatalf("empty = %v", err)
	}
	if err := CheckQuota(l, req, []store.DatabaseBranch{root("a"), root("b")}, g); !errors.Is(err, ErrQuota) {
		t.Fatalf("per root = %v", err)
	}
	big := req
	big.ParentBytes = 600
	if err := CheckQuota(l, big, nil, g); !errors.Is(err, ErrQuota) {
		t.Fatalf("slow path max = %v", err)
	}
	deep := Request{Parent: store.DatabaseBranch{DatabaseName: "b", ParentName: "a"}, RootName: "app", ParentBytes: 1, SlowPath: true}
	all := []store.DatabaseBranch{root("a"), {DatabaseName: "b", ParentName: "a", RootName: "app", State: StateReady}}
	l.MaxPerRoot = 10
	if err := CheckQuota(l, deep, all, g); !errors.Is(err, ErrQuota) {
		t.Fatalf("depth = %v", err)
	}
	var low *diskguard.LowDiskError
	if err := CheckQuota(l, Request{ParentIsRoot: true, RootName: "app", ParentBytes: 400, SlowPath: true}, nil, guardWithFree(700<<0+1<<20)); !errors.As(err, &low) {
		t.Fatalf("disk = %v", err)
	}
	if err := CheckQuota(l, Request{ParentIsRoot: true, RootName: "app", ParentBytes: 400}, nil, guardWithFree(2<<20)); err != nil {
		t.Fatalf("fast path = %v", err)
	}
}

func TestExpiry(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	if ExpiryFor(now, 0) != "" || ExpiryFor(now, time.Hour) != "2026-01-10T01:00:00Z" {
		t.Fatal("ExpiryFor")
	}
	rows := []store.DatabaseBranch{
		{DatabaseName: "due", State: StateReady, ExpiresAt: "2026-01-09T00:00:00Z"},
		{DatabaseName: "later", State: StateReady, ExpiresAt: "2026-01-11T00:00:00Z"},
		{DatabaseName: "never", State: StateReady},
		{DatabaseName: "locked", State: StateReady, ExpiresAt: "2026-01-09T00:00:00Z", Protected: true},
		{DatabaseName: "oldfail", State: StateFailed, UpdatedAt: "2026-01-08T00:00:00Z"},
		{DatabaseName: "newfail", State: StateFailed, UpdatedAt: "2026-01-09T23:00:00Z"},
		{DatabaseName: "busy", State: StateCloning, ExpiresAt: "2026-01-09T00:00:00Z"},
		{DatabaseName: "stuck", State: StateDeleting, UpdatedAt: "2026-01-09T23:00:00Z"},
		{DatabaseName: "fresh", State: StateExpiring, UpdatedAt: "2026-01-09T23:55:00Z"},
		{DatabaseName: "oldreq", State: StateRequested, UpdatedAt: "2026-01-09T00:00:00Z"},
		{DatabaseName: "newreq", State: StateRequested, UpdatedAt: "2026-01-09T23:30:00Z"},
		{DatabaseName: "parent", State: StateReady, ExpiresAt: "2026-01-09T00:00:00Z"},
		{DatabaseName: "kid", ParentName: "parent", State: StateCloning},
	}
	got := SelectExpired(rows, now, store.DuePolicy{FailedRetention: 24 * time.Hour, Retry: 10 * time.Minute, RequestedTimeout: time.Hour})
	if len(got) != 4 || got[0].DatabaseName != "due" || got[1].DatabaseName != "oldfail" || got[2].DatabaseName != "stuck" || got[3].DatabaseName != "oldreq" {
		t.Fatalf("got %+v", got)
	}
}

func TestQuotaIgnoresPromoted(t *testing.T) {
	l := Limits{MaxPerRoot: 1, Headroom: 1}
	all := []store.DatabaseBranch{{DatabaseName: "p", RootName: "app", State: StatePromoted}}
	if err := CheckQuota(l, Request{ParentIsRoot: true, RootName: "app"}, all, guardWithFree(1<<40)); err != nil {
		t.Fatalf("promoted counted: %v", err)
	}
}
