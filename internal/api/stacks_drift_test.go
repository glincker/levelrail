package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/stackdrift"
)

func TestStackDrift_PayloadAndAttentionAfterLiveChange(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	if code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil); code != http.StatusOK || out.Revision == nil {
		t.Fatalf("apply = %d %+v", code, out)
	}
	c := &stackdrift.Checker{Store: db, Live: rt.StackDriftLive, ProjectName: rt.StackProjectName, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	stackDrift := func() stackDriftResource {
		var s stackResource
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod", ""), &s); code != http.StatusOK {
			t.Fatalf("get stack = %d", code)
		}
		return s.Drift
	}

	if d := stackDrift(); d.Status != "none" || d.CheckedAt != "" || d.Summary == nil {
		t.Fatalf("before first check = %+v", d)
	}
	c.CheckAll(t.Context())
	if d := stackDrift(); d.Status != "none" || d.CheckedAt == "" || d.Revision != 1 {
		t.Fatalf("clean check = %+v", d)
	}

	svc, err := db.GetDesiredService(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	svc.Image = "ghcr.io/acme/web:9.9.9-live"
	if err := db.SaveDesiredService(t.Context(), *svc); err != nil {
		t.Fatal(err)
	}
	c.CheckAll(t.Context())
	d := stackDrift()
	raw, _ := json.Marshal(d)
	if d.Status != "drifted" || len(d.Summary) == 0 || strings.Contains(string(raw), "9.9.9-live") {
		t.Fatalf("drifted check = %s", raw)
	}

	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", "")
	items := rt.stackDriftAttentionItems(req, []string{AbilityRead})
	if len(items) != 1 || items[0].Kind != attention.KindStackDrift || items[0].Subject != "shop-prod" {
		t.Fatalf("attention items = %+v", items)
	}
	if got := rt.stackDriftAttentionItems(req, nil); len(got) != 0 {
		t.Fatalf("caller without read sees %d items", len(got))
	}
}

func TestStackDrift_ReusesOneSystemTokenAcrossPasses(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil); code != http.StatusOK {
		t.Fatalf("apply = %d", code)
	}
	c := &stackdrift.Checker{Store: db, Live: rt.StackDriftLive, ProjectName: rt.StackProjectName, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for range 4 {
		c.CheckAll(t.Context())
	}
	toks, _ := db.ListAPITokens(t.Context())
	rows := 0
	for _, tok := range toks {
		if tok.Name == "stack-drift" {
			rows++
		}
	}
	if rows != 1 {
		t.Fatalf("stack-drift token rows = %d, want 1", rows)
	}
}
