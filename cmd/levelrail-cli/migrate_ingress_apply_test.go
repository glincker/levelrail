package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type migrateCall struct{ method, path, body string }

func fakeMigrateTarget(t *testing.T, abilities string) (*httptest.Server, func() []migrateCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []migrateCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		mu.Lock()
		calls = append(calls, migrateCall{r.Method, r.URL.Path, string(raw)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/my-access":
			_, _ = w.Write([]byte(`{"kind":"token","name":"fake","abilities":` + abilities + `}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(`[{"id":"n1","name":"edge-1","status":"ready"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases":
			_, _ = w.Write([]byte(`[{"name":"pg"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			var body struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(raw, &body)
			_, _ = w.Write([]byte(`{"name":"` + body.Name + `"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []migrateCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]migrateCall(nil), calls...)
	}
}

func coolifyFixtureApp(name, server, network string) coolifyApplication {
	a := coolifyApplication{Name: name, BuildPack: "dockerfile", FQDN: "https://" + name + ".example.test", PortsExposes: "3000"}
	a.Destination.Server.Name = server
	a.Destination.Network = network
	return a
}

func TestMigrateCoolifyIngressPreviewsNetworkAndNodeMap(t *testing.T) {
	web := coolifyFixtureApp("web", "srv-a", "shared-net")
	web.BasicAuthEnabled = true
	web.PreviewsEnabled = true
	web.CustomLabels = "traefik.http.middlewares.old.redirectregex.regex=^/old\n" +
		"traefik.http.middlewares.old.redirectregex.replacement=https://web.example.test/new\n" +
		"traefik.http.middlewares.gate.basicauth.users=fakeuser:fakehash\n" +
		"traefik.http.middlewares.x.unknownthing.opt=1"
	pg := coolifyFixtureApp("pg", "srv-b", "shared-net")
	report := migrationReport{Applied: true, Apps: []mappedApp{
		mapCoolifyApplication(web, nil, false), mapCoolifyApplication(pg, nil, false),
	}}
	linkNetworkPeers(report.Apps)
	placeApps(report.Apps, map[string]string{"srv-a": "edge-1"})
	srv, calls := fakeMigrateTarget(t, `["root"]`)
	client := NewClient(srv.URL, "fake-token")
	ctx := context.Background()
	if err := resolvePlacements(ctx, client, report.Apps, map[string]string{"srv-a": "edge-1"}); err != nil {
		t.Fatal(err)
	}
	applyMigration(ctx, client, &report)

	want := map[string]bool{"PUT /api/v1/apps/web/domains/web.example.test/auth": false, "PUT /api/v1/apps/web/domains/web.example.test/forwarders": false}
	var password string
	for _, c := range calls() {
		key := c.method + " " + c.path
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if strings.HasSuffix(c.path, "/auth") {
			var b struct{ Password string }
			_ = json.Unmarshal([]byte(c.body), &b)
			password = b.Password
		}
		if strings.Contains(c.body, "fakehash") || strings.Contains(c.body, "fakeuser") {
			t.Fatalf("source credential sent: %s", c.body)
		}
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("missing call %s in %+v", k, calls())
		}
	}
	if len(password) < 32 {
		t.Fatalf("placeholder password too short: %d", len(password))
	}
	out, _ := json.Marshal(report)
	if strings.Contains(string(out), password) || strings.Contains(string(out), "fakehash") {
		t.Fatalf("report leaks a credential: %s", out)
	}
	w, p := report.Apps[0], report.Apps[1]
	if w.Node != "edge-1" || w.nodeID != "n1" || p.Node != "" || w.Service.Previews == nil {
		t.Fatalf("placement or previews wrong: %+v %+v", w, p)
	}
	if len(w.NetworkPeers) != 1 || w.NetworkPeers[0] != "pg" {
		t.Fatalf("peers %v", w.NetworkPeers)
	}
	var unmapped, unknownLabel bool
	for _, iss := range p.Issues {
		unmapped = unmapped || (iss.Field == "node" && strings.Contains(iss.Detail, "srv-b"))
	}
	for _, iss := range w.Issues {
		unknownLabel = unknownLabel || (iss.Field == "custom_labels" && strings.Contains(iss.Detail, "unknownthing"))
	}
	if !unmapped || !unknownLabel {
		t.Fatalf("missing findings: %+v %+v", w.Issues, p.Issues)
	}
}

func TestMigrateNodeMapUnknownNodeRefusedBeforeApply(t *testing.T) {
	report := migrationReport{Apps: []mappedApp{mapCoolifyApplication(coolifyFixtureApp("web", "srv-a", ""), nil, false)}}
	placeApps(report.Apps, map[string]string{"srv-a": "ghost"})
	srv, calls := fakeMigrateTarget(t, `["root"]`)
	err := resolvePlacements(context.Background(), NewClient(srv.URL, "fake-token"), report.Apps, map[string]string{"srv-a": "ghost"})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("unknown node accepted: %v", err)
	}
	for _, c := range calls() {
		if c.method != http.MethodGet {
			t.Fatalf("mutating call before refusal: %+v", c)
		}
	}
}

func TestMigrateNodeMapNeedsRootBeforeApply(t *testing.T) {
	report := migrationReport{Apps: []mappedApp{mapCoolifyApplication(coolifyFixtureApp("web", "srv-a", ""), nil, false)}}
	placeApps(report.Apps, map[string]string{"srv-a": "edge-1"})
	srv, calls := fakeMigrateTarget(t, `["apps:write"]`)
	if err := resolvePlacements(context.Background(), NewClient(srv.URL, "fake-token"), report.Apps, map[string]string{"srv-a": "edge-1"}); err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("non root token accepted: %v", err)
	}
	for _, c := range calls() {
		if c.method != http.MethodGet {
			t.Fatalf("mutating call before refusal: %+v", c)
		}
	}
}

func TestMigrateNetworkPeerNameCollisionNeverConnects(t *testing.T) {
	// The target has an unrelated database named pg; an imported app named pg shares the network.
	report := migrationReport{Applied: true, Apps: []mappedApp{
		mapCoolifyApplication(coolifyFixtureApp("web", "", "net"), nil, false), mapCoolifyApplication(coolifyFixtureApp("pg", "", "net"), nil, false),
	}}
	linkNetworkPeers(report.Apps)
	srv, calls := fakeMigrateTarget(t, `["root"]`)
	applyMigration(context.Background(), NewClient(srv.URL, "fake-token"), &report)
	for _, c := range calls() {
		if strings.Contains(c.path, "/connections") || c.path == "/api/v1/databases" {
			t.Fatalf("network peer triggered a database call: %+v", c)
		}
	}
}

func TestMigrateBasicAuthFileModeKeepsPublicWarning(t *testing.T) {
	a := coolifyFixtureApp("web", "", "")
	a.BasicAuthEnabled = true
	m := mapCoolifyApplication(a, nil, false)
	var warned bool
	for _, iss := range m.Issues {
		warned = warned || (iss.Field == "is_http_basic_auth_enabled" && iss.Severity == issueReview && strings.Contains(iss.Detail, "public"))
	}
	if !warned {
		t.Fatalf("public warning missing: %+v", m.Issues)
	}
}

func TestMigratePreviewsOfGatedAppAreGated(t *testing.T) {
	a := coolifyFixtureApp("web", "", "")
	a.BasicAuthEnabled, a.PreviewsEnabled = true, true
	report := migrationReport{Applied: true, Apps: []mappedApp{mapCoolifyApplication(a, nil, false)}}
	srv, calls := fakeMigrateTarget(t, `["root"]`)
	applyMigration(context.Background(), NewClient(srv.URL, "fake-token"), &report)
	var gated bool
	var pw string
	for _, c := range calls() {
		if strings.HasSuffix(c.path, "/preview-policy") || strings.Contains(c.path, "preview") {
			var b struct {
				Gate     *bool   `json:"gate_basic_auth"`
				Password *string `json:"gate_password"`
			}
			_ = json.Unmarshal([]byte(c.body), &b)
			gated = b.Gate != nil && *b.Gate && b.Password != nil && len(*b.Password) >= 32
			if b.Password != nil {
				pw = *b.Password
			}
		}
	}
	if !gated {
		t.Fatalf("previews of a gated app are not gated: %+v", calls())
	}
	out, _ := json.Marshal(report)
	if strings.Contains(string(out), pw) {
		t.Fatal("report leaks the preview gate password")
	}
}

func TestMigrateHostileServerNamePrintedQuoted(t *testing.T) {
	report := migrationReport{CoolifyURL: "https://coolify.example.test", Apps: []mappedApp{mapCoolifyApplication(coolifyFixtureApp("web", "evil\x1b[2J\r\nsrv", ""), nil, false)}}
	placeApps(report.Apps, map[string]string{"other": "edge-1"})
	report.Apps[0].Node = "edge\x1b]0;x"
	var buf bytes.Buffer
	printMigrationReportHuman(&buf, report, "cli", "./out")
	for _, iss := range report.Apps[0].Issues {
		if strings.ContainsAny(iss.Detail, "\x1b\r") {
			t.Fatalf("raw control characters in issue: %q", iss.Detail)
		}
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("raw escape printed: %q", buf.String())
	}
}
