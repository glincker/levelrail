package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/GLINCKER/levelrail/internal/catalog"
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

// net_admin has no compose path until an Elevated runtime exists, so such a manifest cannot load at all.
func TestNetAdminTemplateCannotLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"m/captpl/template.yaml": {Data: []byte(capTplManifest + "  - {class: net_admin, service: web, reason: vpn tunnel, caps: [NET_ADMIN]}\n")},
		"m/captpl/compose.yaml":  {Data: []byte("services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n    cap_add: [NET_ADMIN]\n")},
	}
	if _, err := catalog.LoadManifest(fsys, "m/captpl"); err == nil {
		t.Fatal("a net_admin manifest must not load until an Elevated runtime path exists")
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
