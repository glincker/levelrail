package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/stackdrift"
	"github.com/GLINCKER/levelrail/internal/store"
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

func TestStackDrift_InstallStackNodeTaintEditIsDrift(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	ctx := t.Context()
	createInstallStack(t, rt, cookie, "base")
	if err := db.SaveNode(ctx, store.Node{ID: "n1", Name: "worker-1", Status: store.NodeStatusOnline, Schedulable: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetNodeTaints(ctx, "n1", []store.NodeTaint{{Key: "gpu", Value: "a", Effect: "NoSchedule"}}); err != nil {
		t.Fatal(err)
	}
	// Seeded as applied: an install stack apply waits on a second approver.
	s, _ := db.GetStackByName(ctx, "base")
	doc := "version: 1\nkind: Node\nmetadata:\n  name: worker-1\nspec:\n  taints:\n    - {key: gpu, value: \"a\", effect: NoSchedule}\n"
	docs, _ := json.Marshal([]map[string]string{{"name": "stack.yaml", "content": doc}})
	rev, err := db.AppendStackRevision(ctx, store.NewStackRevision{StackID: s.ID, Status: store.StackRevApplying, PlanHash: "h", PlanJSON: []byte(`{}`), RenderedDocs: docs, Actor: "admin"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStackRevisionStatus(ctx, s.ID, rev.Number, store.StackRevApplied, []byte(`{}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertStackResource(ctx, store.StackLedgerEntry{StackID: s.ID, Kind: "Node", Key: "Node/worker-1", LiveID: "n1", SinceRevision: 1}); err != nil {
		t.Fatal(err)
	}
	c := &stackdrift.Checker{Store: db, Live: rt.StackDriftLive, ProjectName: rt.StackProjectName, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	driftOf := func() store.StackDrift {
		c.CheckAll(ctx)
		d, _, _ := db.GetStackDrift(ctx, s.ID)
		return d
	}
	if d := driftOf(); d.Status != store.StackDriftNone || d.Revision != 1 {
		t.Fatalf("clean check = %+v", d)
	}
	if err := db.SetNodeTaints(ctx, "n1", []store.NodeTaint{{Key: "gpu", Value: "a", Effect: "NoSchedule"}, {Key: "lab-hand", Value: "hand-value", Effect: "NoSchedule"}}); err != nil {
		t.Fatal(err)
	}
	d := driftOf()
	raw, _ := json.Marshal(d.Summary)
	if d.Status != store.StackDriftDrifted || len(d.Summary) != 1 || d.Summary[0].Kind != "Node" || strings.Contains(string(raw), "hand-value") {
		t.Fatalf("hand taint check = %+v %s", d, raw)
	}

	// The grant is the drift token's own: a plain read token still cannot list nodes.
	exp := time.Now().Add(time.Hour)
	tok, _, err := MintAgentAPIToken(ctx, db, "reader", []string{AbilityRead}, &exp, agentIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := callWithToken(t, rt, tok, http.MethodGet, "/api/v1/nodes", ""); code != http.StatusForbidden {
		t.Fatalf("read token list nodes = %d, want 403", code)
	}
}

func TestStackDrift_ReadGrantIsNarrow(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.stackDrift.id = "tok-1"
	req := func(method, pattern, marker string) *http.Request {
		r := httptest.NewRequest(method, "/x", nil)
		r.Pattern = pattern
		if marker != "" {
			r = r.WithContext(context.WithValue(r.Context(), driftReadKey{}, marker))
		}
		return r
	}
	for _, c := range []struct {
		name string
		r    *http.Request
		tok  string
		want bool
	}{
		{"listed get", req(http.MethodGet, "GET /api/v1/nodes/{id}/taints", "tok-1"), "tok-1", true},
		{"listed put", req(http.MethodPut, "PUT /api/v1/nodes/{id}/taints", "tok-1"), "tok-1", false},
		{"stack verb get outside drift allowlist", req(http.MethodGet, "GET /api/v1/nodes/placement-simulation", "tok-1"), "tok-1", false},
		{"unlisted get", req(http.MethodGet, "GET /api/v1/auth/tokens", "tok-1"), "tok-1", false},
		{"no marker", req(http.MethodGet, "GET /api/v1/nodes", ""), "tok-1", false},
		{"other token", req(http.MethodGet, "GET /api/v1/nodes", "tok-1"), "tok-2", false},
		{"marker not current token", req(http.MethodGet, "GET /api/v1/nodes", "tok-2"), "tok-2", false},
	} {
		if got := rt.driftReadGranted(c.r, c.tok); got != c.want {
			t.Errorf("%s = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStackDrift_AttentionHidesStacksTheCallerCannotRead(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil); code != http.StatusOK {
		t.Fatalf("apply = %d", code)
	}
	stacks, err := db.ListStacks(t.Context())
	if err != nil || len(stacks) != 1 {
		t.Fatalf("stacks = %v %v", stacks, err)
	}
	if err := db.SetStackDrift(t.Context(), store.StackDrift{StackID: stacks[0].ID, Status: store.StackDriftDrifted, Revision: 1, CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	other := storeUserWithAbilitiesForTest(t, db, "other@example.com", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-stack", "Deny", AbilityRead, "stack:shop-prod", store.PrincipalTypeUser, reader.ID)

	denied := authedRequest(t, sessionCookieForTest(t, rt, reader.ID), http.MethodGet, "/api/v1/attention/feed", "")
	if got := rt.stackDriftAttentionItems(denied, []string{AbilityRead}); len(got) != 0 {
		t.Fatalf("caller denied on the stack sees %+v", got)
	}
	allowed := authedRequest(t, sessionCookieForTest(t, rt, other.ID), http.MethodGet, "/api/v1/attention/feed", "")
	if got := rt.stackDriftAttentionItems(allowed, []string{AbilityRead}); len(got) != 1 {
		t.Fatalf("caller with read sees %d items", len(got))
	}
}

func TestStackDrift_StartupRevokesStaleTokensAndTTLComesFromEnv(t *testing.T) {
	rt, db := newTestRouter(t)
	exp := time.Now().Add(24 * time.Hour)
	_, stale, err := MintAgentAPIToken(t.Context(), db, "stack-drift", []string{AbilityRead}, &exp, agentIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, keep, err := MintAgentAPIToken(t.Context(), db, "ci-deploy", []string{AbilityRead}, &exp, agentIdentity{}, "")
	if err != nil {
		t.Fatal(err)
	}
	rt.RevokeStaleStackDriftTokens(t.Context())
	toks, _ := db.ListAPITokens(t.Context())
	for _, tok := range toks {
		if tok.ID == stale.ID && tok.RevokedAt == nil {
			t.Error("stale stack-drift token still live")
		}
		if tok.ID == keep.ID && tok.RevokedAt != nil {
			t.Error("unrelated token revoked")
		}
	}

	t.Setenv(envStackDriftTTL, "30m")
	if _, err := rt.driftToken(t.Context()); err != nil {
		t.Fatal(err)
	}
	if left := time.Until(rt.stackDrift.expires); left > 30*time.Minute || left < 29*time.Minute {
		t.Fatalf("token life = %s, want about 30m", left)
	}
}

func TestStackDrift_EmitsDriftedEventOnceAndNamesOnly(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	sink := &captureEvents{}
	rt.SetEventWebhooks(sink)
	if code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil); code != http.StatusOK || out.Revision == nil {
		t.Fatalf("apply = %d", code)
	}
	c := &stackdrift.Checker{Store: rt.StackDriftStore(db), Live: rt.StackDriftLive, ProjectName: rt.StackProjectName, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.CheckAll(t.Context())
	if n := sink.count("stacks.drifted"); n != 0 {
		t.Fatalf("clean pass emitted %d drift events", n)
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
	c.CheckAll(t.Context())
	if n := sink.count("stacks.drifted"); n != 1 {
		t.Fatalf("drift events = %d, want 1", n)
	}
	if raw := sink.json(t); strings.Contains(raw, "9.9.9-live") {
		t.Fatalf("drift event holds a value: %s", raw)
	}
}

func TestStackDrift_SettingsComeFromRegistry(t *testing.T) {
	rt, _ := newTestRouter(t)
	if got := rt.StackDriftInterval(t.Context()); got != 15*time.Minute {
		t.Fatalf("default interval = %s", got)
	}
	if got := rt.StackDriftSummaryMax(t.Context()); got != 50 {
		t.Fatalf("default summary max = %d", got)
	}
}
