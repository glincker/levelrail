package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func createInstallStack(t *testing.T, rt *Router, cookie *http.Cookie, name string) {
	t.Helper()
	body := jsonBody(t, map[string]any{"name": name, "scope": "install"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", body), nil); code != http.StatusCreated {
		t.Fatalf("create install stack status = %d", code)
	}
}

func TestStackTokenVerbScope(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	createInstallStack(t, rt, cookie, "nodes-eu")
	for _, c := range []struct {
		stack string
		verbs []string
		want  int
	}{
		{"nodes-eu", []string{"plan", "node"}, http.StatusCreated},
		{"nodes-eu", []string{"setting"}, http.StatusCreated},
		{"nodes-eu", []string{"placement"}, http.StatusBadRequest},
		{"shop-prod", []string{"placement"}, http.StatusCreated},
		{"shop-prod", []string{"node"}, http.StatusBadRequest},
		{"shop-prod", []string{"setting"}, http.StatusBadRequest},
		{"shop-prod", []string{"weaken"}, http.StatusBadRequest},
	} {
		body := jsonBody(t, map[string]any{"name": "ci", "abilities": []string{"read", "write"}, "stack": c.stack, "stack_verbs": c.verbs})
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body), nil); code != c.want {
			t.Errorf("mint %s %v = %d, want %d", c.stack, c.verbs, code, c.want)
		}
	}

	// A narrow verb implies apply on the direct stack routes.
	tok, _ := mintStackToken(t, rt, cookie, "shop-prod", []string{"placement"}, []string{AbilityRead, AbilityWrite})
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, iacTestDocs, nil)); code != http.StatusOK {
		t.Errorf("apply with placement verb = %d, want 200", code)
	}
	if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, iacTestDocs, nil)); code != http.StatusForbidden {
		t.Errorf("plan without plan verb = %d, want 403", code)
	}

	// OIDC trusts list the narrow verbs explicitly and meet the apply bar.
	e := newOIDCStackEnv(t)
	createInstallStack(t, e.rt, e.cookie, "nodes-eu")
	trust := func(verbs []string, ref string) map[string]any {
		return map[string]any{"repository": "acme/infra", "ref_pattern": ref, "environment": "prod", "verbs": verbs}
	}
	for _, c := range []struct {
		stack string
		body  map[string]any
		want  int
	}{
		{"nodes-eu", trust([]string{"plan", "node"}, "refs/heads/main"), http.StatusCreated},
		{"nodes-eu", trust([]string{"setting"}, "refs/heads/*"), http.StatusBadRequest},
		{"nodes-eu", trust([]string{"placement"}, "refs/heads/main"), http.StatusBadRequest},
		{"shop-prod", trust([]string{"node"}, "refs/heads/main"), http.StatusBadRequest},
		{"shop-prod", trust([]string{"placement"}, "refs/heads/release"), http.StatusCreated},
	} {
		if code, _ := e.addTrust(t, c.stack, c.body); code != c.want {
			t.Errorf("trust %s %v = %d, want %d", c.stack, c.body["verbs"], code, c.want)
		}
	}
}

func TestStackSubRequestVerbs(t *testing.T) {
	t.Setenv("APP_NODES_PROTECTED_TAINT_KEYS", "backup")
	rt, db, cookie := stackFixture(t)
	ctx := t.Context()
	createInstallStack(t, rt, cookie, "nodes-eu")
	createInstallStack(t, rt, cookie, "nodes-us")
	eu, _ := db.GetStackByName(ctx, "nodes-eu")
	us, _ := db.GetStackByName(ctx, "nodes-us")
	shop, _ := db.GetStackByName(ctx, "shop-prod")
	for _, n := range []string{"n1", "n2", "n3", "n4"} {
		if err := db.SaveNode(ctx, store.Node{ID: n, Name: "box-" + n, Status: store.NodeStatusOnline, Schedulable: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetNodeTaints(ctx, "n1", []store.NodeTaint{{Key: "backup", Value: "yes", Effect: "NoSchedule"}, {Key: "gpu", Value: "a", Effect: "NoSchedule"}}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []store.StackLedgerEntry{
		{StackID: eu.ID, Kind: "Node", Key: "Node/box-n1", LiveID: "n1"},
		{StackID: us.ID, Kind: "Node", Key: "Node/box-n2", LiveID: "n2"},
		{StackID: eu.ID, Kind: "Node", Key: "Node/box-n4", LiveID: "renamed-away"},
		{StackID: us.ID, Kind: "Setting", Key: "Setting/" + runtimesettings.KeyTokensDefaultTTLDays},
		{StackID: shop.ID, Kind: "App", Key: "App/web"},
	} {
		if err := db.UpsertStackResource(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	nodeTok, nodeID := mintStackToken(t, rt, cookie, "nodes-eu", []string{"plan", "node"}, []string{AbilityRead, AbilityWrite})
	setTok, setID := mintStackToken(t, rt, cookie, "nodes-eu", []string{"setting"}, []string{AbilityRead, AbilityWrite})
	planTok, planID := mintStackToken(t, rt, cookie, "nodes-eu", []string{"plan"}, []string{AbilityRead})
	plcTok, plcID := mintStackToken(t, rt, cookie, "shop-prod", []string{"placement"}, []string{AbilityRead, AbilityWrite})

	scoped := func(id string, stack store.StackRecord, apply bool) context.Context {
		c := context.WithValue(ctx, stackTokenCallKey{}, &stackTokenCall{tokenID: id, stackID: stack.ID, created: map[string]bool{}})
		if apply {
			c = iac.WithStackScope(c, iac.StackScope{StackID: stack.ID, StackName: stack.Name, Revision: 1})
		}
		return c
	}
	id := func(v string) map[string]string { return map[string]string{"id": v} }
	key := func(v string) map[string]string { return map[string]string{"key": v} }
	name := func(v string) map[string]string { return map[string]string{"name": v} }
	keepBackup := `{"taints":[{"key":"backup","value":"yes","effect":"NoSchedule"}]}`
	ttl := runtimesettings.KeyTokensDefaultTTLDays

	cases := []struct {
		name    string
		tok, id string
		ctx     context.Context
		pattern string
		body    string
		values  map[string]string
		allow   bool
	}{
		{"list nodes", nodeTok, nodeID, scoped(nodeID, eu, false), "GET /api/v1/nodes", "", nil, true},
		{"read own node taints", nodeTok, nodeID, scoped(nodeID, eu, false), "GET /api/v1/nodes/{id}/taints", "", id("n1"), true},
		{"read unclaimed node", nodeTok, nodeID, scoped(nodeID, eu, false), "GET /api/v1/nodes/{id}/labels", "", id("n3"), true},
		{"read other stack node", nodeTok, nodeID, scoped(nodeID, eu, false), "GET /api/v1/nodes/{id}/labels", "", id("n2"), false},
		{"plan token reads nodes", planTok, planID, scoped(planID, eu, false), "GET /api/v1/nodes", "", nil, true},
		{"plan token cannot write labels", planTok, planID, scoped(planID, eu, true), "PUT /api/v1/nodes/{id}/labels", `{"labels":{}}`, id("n1"), false},
		{"labels outside apply", nodeTok, nodeID, scoped(nodeID, eu, false), "PUT /api/v1/nodes/{id}/labels", `{"labels":{}}`, id("n1"), false},
		{"labels on own node", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/labels", `{"labels":{"zone":"a"}}`, id("n1"), true},
		{"labels on unclaimed node", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/labels", `{"labels":{}}`, id("n3"), false},
		{"labels on other stack node", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/labels", `{"labels":{}}`, id("n2"), false},
		{"labels on renamed node", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/labels", `{"labels":{}}`, id("n4"), false},
		{"taints keep protected", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/taints", keepBackup, id("n1"), true},
		{"taints tighten protected", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/taints", `{"taints":[{"key":"backup","value":"yes","effect":"NoExecute"}]}`, id("n1"), true},
		{"taints drop protected", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/taints", `{"taints":[{"key":"gpu","value":"a","effect":"NoSchedule"}]}`, id("n1"), false},
		{"taints loosen protected", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/taints", `{"taints":[{"key":"backup","value":"yes","effect":"PreferNoSchedule"}]}`, id("n1"), false},
		{"taints revalue protected", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/taints", `{"taints":[{"key":"backup","value":"no","effect":"NoSchedule"}]}`, id("n1"), false},
		{"taints folded duplicate", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/taints", `{"taints":[],"TAINTS":[]}`, id("n1"), false},
		{"node verb no settings", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":30}`, key(ttl), false},
		{"delete node", nodeTok, nodeID, scoped(nodeID, eu, true), "DELETE /api/v1/nodes/{id}", "", id("n1"), false},
		{"node workloads", nodeTok, nodeID, scoped(nodeID, eu, true), "PUT /api/v1/nodes/{id}/workloads", `{}`, id("n1"), false},
		{"read settings", setTok, setID, scoped(setID, eu, false), "GET /api/v1/settings/runtime", "", nil, true},
		{"read one setting", setTok, setID, scoped(setID, eu, false), "GET /api/v1/settings/runtime/{key}", "", key(runtimesettings.KeyNodesProtectedTaintKeys), true},
		{"write free setting", setTok, setID, scoped(setID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":["backup","db"]}`, key(runtimesettings.KeyNodesProtectedTaintKeys), true},
		{"write other stack setting", setTok, setID, scoped(setID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":30}`, key(ttl), false},
		{"confirm weakening body", setTok, setID, scoped(setID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":[],"confirm_weakening":true}`, key(runtimesettings.KeyNodesProtectedTaintKeys), false},
		{"confirm weakening folded", setTok, setID, scoped(setID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":[],"Confirm_Weakening":1}`, key(runtimesettings.KeyNodesProtectedTaintKeys), false},
		{"confirm weakening query", setTok, setID, scoped(setID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":[]}`, key(runtimesettings.KeyNodesProtectedTaintKeys), false},
		{"unknown setting", setTok, setID, scoped(setID, eu, true), "PUT /api/v1/settings/runtime/{key}", `{"value":1}`, key("nope.nothing"), false},
		{"reset setting", setTok, setID, scoped(setID, eu, true), "DELETE /api/v1/settings/runtime/{key}", "", key(runtimesettings.KeyNodesProtectedTaintKeys), false},
		{"restart apply", setTok, setID, scoped(setID, eu, true), "POST /api/v1/settings/runtime/apply", `{}`, nil, false},
		{"placement read own app", plcTok, plcID, scoped(plcID, shop, false), "GET /api/v1/apps/{name}/placement", "", name("web"), true},
		{"placement own app", plcTok, plcID, scoped(plcID, shop, true), "PUT /api/v1/apps/{name}/placement", `{}`, name("web"), true},
		{"placement unowned app", plcTok, plcID, scoped(plcID, shop, true), "PUT /api/v1/apps/{name}/placement", `{}`, name("loose"), false},
		{"placement unowned db", plcTok, plcID, scoped(plcID, shop, true), "PUT /api/v1/databases/{name}/placement", `{}`, name("orphan"), false},
		{"placement simulation", plcTok, plcID, scoped(plcID, shop, false), "GET /api/v1/nodes/placement-simulation", "", nil, true},
		{"placement verb no nodes", plcTok, plcID, scoped(plcID, shop, true), "PUT /api/v1/nodes/{id}/labels", `{"labels":{}}`, id("n3"), false},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := subRequest(c.ctx, c.tok, c.pattern, c.body, c.values)
		if c.name == "confirm weakening query" {
			q := req.URL.Query()
			q.Set("confirm_weakening", "true")
			req.URL.RawQuery = q.Encode()
		}
		got, ok := rt.gateStackToken(rec, req, c.id)
		if ok != c.allow {
			t.Errorf("%s: allowed = %v, want %v (status %d)", c.name, ok, c.allow, rec.Code)
		}
		if ok && !stackVerbGranted(got) {
			t.Errorf("%s: allowed without the grant marker", c.name)
		}
	}

	// The grant covers only the routed pattern it was issued for.
	got, ok := rt.gateStackToken(httptest.NewRecorder(), subRequest(scoped(nodeID, eu, false), nodeTok, "GET /api/v1/nodes", "", nil), nodeID)
	if !ok {
		t.Fatal("list nodes denied")
	}
	got.Pattern = "DELETE /api/v1/nodes/{id}"
	if stackVerbGranted(got) {
		t.Error("grant leaked to another pattern")
	}

	if taintsKeepProtected([]string{"backup"}, nil, []placement.Taint{}) != true {
		t.Error("no current protected taint should pass")
	}
}
