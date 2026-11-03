package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDNSWatch_RoutesFilterByAppVisibility(t *testing.T) {
	f := newTeamFixture(t)
	w := dnsstatus.NewWatcher(dnsstatus.WatchConfig{
		Store: dnsWatchStore{db: f.db},
		Check: func(context.Context, string, string) (dnsstatus.WatchState, error) {
			return dnsstatus.WatchWaitingDNS, nil
		},
		Settings: f.rt.dnsWatchSettings,
	})
	ctx := context.Background()
	w.Register(ctx, appA, "a.example.test")
	w.Register(ctx, appB, "b.example.test")
	f.rt.dnsWatcher = w

	member := f.cookies[store.TeamRoleViewer]
	code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/watch", "", withCookie(member))
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, body)
	}
	var rows []dnsstatus.WatchEntry
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Domain != "a.example.test" {
		t.Fatalf("member sees %+v, want only team A's domain", rows)
	}
	if code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/b.example.test/watch", "", withCookie(member)); code != http.StatusNotFound {
		t.Fatalf("other team's snapshot = %d, want 404", code)
	}
	if code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/domains/a.example.test/watch", "", withCookie(member)); code != http.StatusOK {
		t.Fatalf("own snapshot = %d, want 200", code)
	}
}
