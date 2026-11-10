package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppScrapeTargetAuditRows(t *testing.T) {
	f := newTeamFixture(t)
	dev := f.cookies[store.TeamRoleDeveloper]
	as := func(r *http.Request) {
		r.AddCookie(dev)
		r.Header.Set("Content-Type", "application/json")
	}
	path := "/api/v1/apps/" + appA + "/scrape-target"
	if code, raw := teamDo(t, f.rt, http.MethodPut, path, `{"port":9100,"path":"/metrics"}`, as); code != http.StatusOK {
		t.Fatalf("PUT = %d %s", code, raw)
	}
	if code, raw := teamDo(t, f.rt, http.MethodDelete, path, "", as); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d %s", code, raw)
	}
	for action, want := range map[string][]string{
		AuditActionAppScrapeTargetSet:    {`"app":"` + appA + `"`, `"port":"9100"`, `"path":"/metrics"`},
		AuditActionAppScrapeTargetDelete: {`"app":"` + appA + `"`},
	} {
		entries, err := f.db.ListAuditEntries(context.Background(), 10, nil, store.AuditEntryFilter{Action: action})
		if err != nil || len(entries) != 1 {
			t.Fatalf("%s audit entries = %d, err = %v", action, len(entries), err)
		}
		for _, w := range want {
			if !strings.Contains(entries[0].Detail, w) {
				t.Fatalf("%s detail %s lacks %s", action, entries[0].Detail, w)
			}
		}
		if entries[0].ActorID == "" {
			t.Fatalf("%s audit row has no actor", action)
		}
	}
}
