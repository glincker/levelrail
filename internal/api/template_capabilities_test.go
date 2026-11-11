package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const capTplManifest = `id: captpl
name: Cap Tpl
description: Capability fixture.
category: Testing
documentation_url: https://example.com
recommended_memory_bytes: 1024
domains:
  - service: web
    port: 80
healthcheck:
  service: web
  port: 80
  path: /
capabilities:
`

func addCapTemplate(t *testing.T, capLine, capAdd string) {
	t.Helper()
	fsys := fstest.MapFS{
		"m/captpl/template.yaml": {Data: []byte(capTplManifest + capLine)},
		"m/captpl/compose.yaml":  {Data: []byte("services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n    cap_add: [" + capAdd + "]\n")},
	}
	m, err := catalog.LoadManifest(fsys, "m/captpl")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	tpl, err := m.Template()
	if err != nil {
		t.Fatalf("Template: %v", err)
	}
	saved := catalog.Templates
	catalog.Templates = append(append([]catalog.Template(nil), saved...), tpl)
	t.Cleanup(func() { catalog.Templates = saved })
}

func TestInstallTemplate_CapabilityGrantAudited(t *testing.T) {
	addCapTemplate(t, "  - {class: cap_add, service: web, reason: chown data dir, caps: [CHOWN]}\n", "CHOWN")
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"capapp","domains":{"web":"cap.example.com"}}`, http.StatusOK)

	var granted int
	for _, e := range listAuditLog(t, rt, cookie) {
		if e.Action != AuditActionCapabilityGranted {
			continue
		}
		granted++
		var d map[string]any
		if err := json.Unmarshal(e.Detail, &d); err != nil || d["class"] != "cap_add" || d["service"] != "web" || d["app"] != "capapp" || d["digest"] == "" {
			t.Errorf("detail = %s, err = %v", e.Detail, err)
		}
	}
	if granted != 1 {
		t.Fatalf("capability_granted events = %d, want 1", granted)
	}
}

const netAdminLine = "  - {class: net_admin, service: web, reason: vpn tunnel, caps: [NET_ADMIN]}\n"

func capsOf(t *testing.T, db *store.DB, name string) []string {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), name)
	if err != nil {
		t.Fatalf("GetDesiredService(%s): %v", name, err)
	}
	return svc.GrantedCaps
}

func TestInstallTemplate_GrantReachesStoredService(t *testing.T) {
	addCapTemplate(t, "  - {class: cap_add, service: web, reason: chown data dir, caps: [chown]}\n", "chown")
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"applied","domains":{"web":"a.example.com"}}`, http.StatusOK)
	if got := capsOf(t, db, "applied-web"); !slices.Equal(got, []string{"CHOWN"}) {
		t.Fatalf("stored caps = %v, want [CHOWN]", got)
	}
	// A plain redeploy carries no grant, so the record is revoked with it.
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/applied/compose", "services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n", http.StatusOK)
	if got := capsOf(t, db, "applied-web"); len(got) != 0 {
		t.Fatalf("caps after the manifest dropped them = %v, want none", got)
	}
}

func TestInstallTemplate_ElevatedTierOffBlocksThenOnApplies(t *testing.T) {
	addCapTemplate(t, netAdminLine, "NET_ADMIN")
	rt, db := newTestRouterWithComposeSecrets(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	cookie := loginTestSession(t, rt, db)
	body := `{"name":"vpn","domains":{"web":"v.example.com"}}`
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", body, http.StatusForbidden)
	if _, err := db.GetDesiredService(context.Background(), "vpn-web"); err == nil {
		t.Fatal("a blocked install must not create the service")
	}
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyTemplatesCapNetAdmin, json.RawMessage(`true`), "test"); err != nil {
		t.Fatalf("enable setting: %v", err)
	}
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", body, http.StatusOK)
	if got := capsOf(t, db, "vpn-web"); !slices.Equal(got, []string{"NET_ADMIN"}) {
		t.Fatalf("stored caps = %v, want [NET_ADMIN]", got)
	}
}

func TestInstallTemplate_UnknownCapRejected(t *testing.T) {
	fsys := fstest.MapFS{
		"m/captpl/template.yaml": {Data: []byte(capTplManifest + "  - {class: cap_add, service: web, reason: mount things, caps: [SYS_ADMIN]}\n")},
		"m/captpl/compose.yaml":  {Data: []byte("services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n    cap_add: [SYS_ADMIN]\n")},
	}
	if _, err := catalog.LoadManifest(fsys, "m/captpl"); err == nil {
		t.Fatal("SYS_ADMIN is outside every grantable tier and must not load")
	}
}

func TestWidenedCapGrants_ReportsOnlyNewCaps(t *testing.T) {
	addCapTemplate(t, "  - {class: cap_add, service: web, reason: chown data dir, caps: [CHOWN]}\n", "CHOWN")
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"wide","domains":{"web":"w.example.com"}}`, http.StatusOK)
	same, err := rt.widenedCapGrants(context.Background(), "wide", map[string][]string{"web": {"CHOWN"}})
	if err != nil || len(same) != 0 {
		t.Fatalf("unchanged grants = %v, %v; want none", same, err)
	}
	wider, err := rt.widenedCapGrants(context.Background(), "wide", map[string][]string{"web": {"CHOWN", "FOWNER"}, "db": {"SETUID"}})
	if err != nil || !slices.Equal(wider, []string{"db:SETUID", "web:FOWNER"}) {
		t.Fatalf("widened = %v, %v", wider, err)
	}
}

func TestCapAddTemplate_RollbackRederivesGrantsAndUserComposeStillFails(t *testing.T) {
	addCapTemplate(t, "  - {class: cap_add, service: web, reason: chown data dir, caps: [CHOWN]}\n", "CHOWN")
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"rbapp","domains":{"web":"rb.example.com"}}`, http.StatusOK)
	plain := "services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n"
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/rbapp/compose", plain, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/rbapp/compose", plain+"    cap_add: [CHOWN]\n", http.StatusBadRequest)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/rbapp/compose/rollback", `{"revision":1}`, http.StatusOK)
}
