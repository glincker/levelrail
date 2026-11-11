package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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

func TestAddedCaps_ReportsOnlyNewCaps(t *testing.T) {
	held := map[string][]string{"web": {"CHOWN"}}
	if got := addedCaps(held, map[string][]string{"web": {"CHOWN"}}); len(got) != 0 {
		t.Fatalf("unchanged grants = %v; want none", got)
	}
	got := addedCaps(held, map[string][]string{"web": {"CHOWN", "FOWNER"}, "db": {"SETUID"}})
	if !slices.Equal(got, []string{"db:SETUID", "web:FOWNER"}) {
		t.Fatalf("added = %v", got)
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

// replaceCapTemplate swaps the fixture for a new version, as a catalog update would.
func replaceCapTemplate(t *testing.T, capLine, capAdd, image string) {
	t.Helper()
	fsys := fstest.MapFS{
		"m/captpl/template.yaml": {Data: []byte(capTplManifest + capLine)},
		"m/captpl/compose.yaml":  {Data: []byte("services:\n  web:\n    image: " + image + "\n    ports: [\"80\"]\n    cap_add: [" + capAdd + "]\n")},
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
	next := make([]catalog.Template, 0, len(saved))
	for _, c := range saved {
		if c.ID != "captpl" {
			next = append(next, c)
		}
	}
	catalog.Templates = append(next, tpl)
	t.Cleanup(func() { catalog.Templates = saved })
}

func enableElevatedTier(t *testing.T, rt *Router, db *store.DB) {
	t.Helper()
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyTemplatesCapNetAdmin, json.RawMessage(`true`), "test"); err != nil {
		t.Fatalf("enable setting: %v", err)
	}
}

// deployTokenDo calls the API with a token that holds read and deploy only.
func deployTokenPost(t *testing.T, rt *Router, db *store.DB, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	const plaintext = "deploy-cap-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_cap_deploy", Name: "capdeployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead, AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("seed token: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func wantRootRefusal(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "root ability") {
		t.Fatalf("status = %d body = %s, want 403 naming the root ability", rec.Code, rec.Body.String())
	}
}

func TestInstallTemplate_DeployTokenRefusedElevated(t *testing.T) {
	addCapTemplate(t, netAdminLine, "NET_ADMIN")
	rt, db := newTestRouterWithComposeSecrets(t)
	enableElevatedTier(t, rt, db)
	wantRootRefusal(t, deployTokenPost(t, rt, db, "/api/v1/service-templates/captpl/install", `{"name":"tokvpn","domains":{"web":"t.example.com"}}`))
	if _, err := db.GetDesiredService(context.Background(), "tokvpn-web"); err == nil {
		t.Fatal("a refused install must not create the service")
	}
}

func TestTemplateUpgrade_CapabilityGateAndDeployTokenRefused(t *testing.T) {
	t.Setenv("APP_TEMPLATE_UPGRADE_BACKUP_REQUIRED", "false")
	addCapTemplate(t, "", "")
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"capup","domains":{"web":"up.example.com"}}`, http.StatusOK)

	replaceCapTemplate(t, netAdminLine, "NET_ADMIN", "nginx:1.27.1")
	enableElevatedTier(t, rt, db)

	var plan struct {
		Added []string `json:"added_capabilities"`
	}
	if err := json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/capup/template-upgrade", "", http.StatusOK), &plan); err != nil || !slices.Equal(plan.Added, []string{"web:NET_ADMIN"}) {
		t.Fatalf("plan added_capabilities = %v, %v; want [web:NET_ADMIN]", plan.Added, err)
	}

	for _, body := range []string{`{"accept_risk":true}`, `{"accept_risk":true,"accept_capabilities":["web:NET_RAW"]}`} {
		var refusal templateUpgradeRefusal
		raw := composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/capup/template-upgrade", body, http.StatusConflict)
		if err := json.Unmarshal(raw, &refusal); err != nil || refusal.Code != codeUpgradeCapsAck || !slices.Equal(refusal.Capabilities, []string{"web:NET_ADMIN"}) {
			t.Fatalf("body %s: refusal = %s (%v), want %s listing web:NET_ADMIN", body, raw, err, codeUpgradeCapsAck)
		}
	}
	if got := capsOf(t, db, "capup-web"); len(got) != 0 {
		t.Fatalf("a refused upgrade changed the caps to %v", got)
	}

	wantRootRefusal(t, deployTokenPost(t, rt, db, "/api/v1/apps/capup/template-upgrade", `{"accept_risk":true,"accept_capabilities":["web:NET_ADMIN"]}`))
	if got := capsOf(t, db, "capup-web"); len(got) != 0 {
		t.Fatalf("a refused deploy-token upgrade changed the caps to %v", got)
	}
}

func TestComposeRollback_DeployTokenCannotAddElevated(t *testing.T) {
	addCapTemplate(t, netAdminLine, "NET_ADMIN")
	rt, db := newTestRouterWithComposeSecrets(t)
	enableElevatedTier(t, rt, db)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"rbvpn","domains":{"web":"r.example.com"}}`, http.StatusOK)
	// The plain redeploy revokes the grant, so rolling back to revision 1 would add it again.
	plain := "services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n"
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/rbvpn/compose", plain, http.StatusOK)
	wantRootRefusal(t, deployTokenPost(t, rt, db, "/api/v1/apps/rbvpn/compose/rollback", `{"revision":1}`))
	if got := capsOf(t, db, "rbvpn-web"); len(got) != 0 {
		t.Fatalf("caps after a refused rollback = %v", got)
	}
}

func TestComposeRollback_HeldGrantsPassWithoutRootAndAreAudited(t *testing.T) {
	addCapTemplate(t, netAdminLine, "NET_ADMIN")
	rt, db := newTestRouterWithComposeSecrets(t)
	enableElevatedTier(t, rt, db)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"held","domains":{"web":"h.example.com"}}`, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/held/compose", "services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n", http.StatusOK)
	// The owner restores the grant, which is audited as an addition.
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/held/compose/rollback", `{"revision":1}`, http.StatusOK)
	if got := capsOf(t, db, "held-web"); !slices.Equal(got, []string{"NET_ADMIN"}) {
		t.Fatalf("caps after the owner rollback = %v", got)
	}
	// The app now holds the cap, so a deploy-only token may roll back to a revision that carries it.
	if rec := deployTokenPost(t, rt, db, "/api/v1/apps/held/compose/rollback", `{"revision":1}`); rec.Code != http.StatusOK {
		t.Fatalf("deploy token rollback with held caps = %d %s, want 200", rec.Code, rec.Body.String())
	}
	var rollbackEvents int
	for _, e := range listAuditLog(t, rt, cookie) {
		if e.Action == AuditActionCapabilityGranted && strings.Contains(string(e.Detail), "compose_rollback") {
			rollbackEvents++
		}
	}
	if rollbackEvents != 1 {
		t.Fatalf("compose_rollback capability_granted events = %d, want 1 (only the owner rollback added a cap)", rollbackEvents)
	}
}

func TestComposeRollback_ElevatedOffDropsInsteadOfStranding(t *testing.T) {
	addCapTemplate(t, netAdminLine, "NET_ADMIN")
	rt, db := newTestRouterWithComposeSecrets(t)
	enableElevatedTier(t, rt, db)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"flip","domains":{"web":"f.example.com"}}`, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/flip/compose", "services:\n  web:\n    image: nginx:1.27.0\n    ports: [\"80\"]\n", http.StatusOK)
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyTemplatesCapNetAdmin, json.RawMessage(`false`), "test"); err != nil {
		t.Fatalf("disable setting: %v", err)
	}
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/flip/compose/rollback", `{"revision":1}`, http.StatusOK)
	if got := capsOf(t, db, "flip-web"); len(got) != 0 {
		t.Fatalf("caps after a rollback with the tier off = %v, want none", got)
	}
}

func TestSetDesiredImage_DropsGrantsOnImageChangeOnly(t *testing.T) {
	addCapTemplate(t, "  - {class: cap_add, service: web, reason: chown data dir, caps: [CHOWN]}\n", "CHOWN")
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"img","domains":{"web":"i.example.com"}}`, http.StatusOK)
	svc, err := db.GetDesiredService(context.Background(), "img-web")
	if err != nil {
		t.Fatal(err)
	}
	same, err := rt.setDesiredImage(context.Background(), *svc, svc.Image)
	if err != nil || !slices.Equal(same.GrantedCaps, []string{"CHOWN"}) {
		t.Fatalf("same image: caps = %v, %v; want kept", same.GrantedCaps, err)
	}
	moved, err := rt.setDesiredImage(context.Background(), *svc, "evil.example/attacker:1")
	if err != nil || len(moved.GrantedCaps) != 0 || len(capsOf(t, db, "img-web")) != 0 {
		t.Fatalf("other image: caps = %v, %v; want dropped and persisted", moved.GrantedCaps, err)
	}
}

func TestPutApp_KeepsGrantsUnlessImageOrCommandChanges(t *testing.T) {
	existing := store.DesiredService{Image: "nginx:1.27.0", Command: []string{"a"}, GrantedCaps: []string{"CHOWN"}}
	keep := store.DesiredService{Image: "nginx:1.27.0", Command: []string{"a"}}
	preserveUnsentAppFields(&keep, existing, appResource{}, false)
	if !slices.Equal(keep.GrantedCaps, []string{"CHOWN"}) {
		t.Fatalf("unchanged PUT caps = %v, want kept", keep.GrantedCaps)
	}
	imageMoved := store.DesiredService{Image: "other:1", Command: []string{"a"}}
	preserveUnsentAppFields(&imageMoved, existing, appResource{}, true)
	cmdMoved := store.DesiredService{Image: "nginx:1.27.0", Command: []string{"sh"}}
	preserveUnsentAppFields(&cmdMoved, existing, appResource{}, false)
	if len(imageMoved.GrantedCaps) != 0 || len(cmdMoved.GrantedCaps) != 0 {
		t.Fatalf("image change caps = %v, command change caps = %v; want none", imageMoved.GrantedCaps, cmdMoved.GrantedCaps)
	}
}

func TestCloneDesiredService_DropsElevatedKeepsReview(t *testing.T) {
	src := store.DesiredService{Name: "a", Image: "x:1", GrantedCaps: []string{"CHOWN", "NET_ADMIN"}}
	if got := cloneDesiredService(src, "b", nil).GrantedCaps; !slices.Equal(got, []string{"CHOWN"}) {
		t.Fatalf("clone caps = %v, want [CHOWN]", got)
	}
}

func TestEgressPolicy_RefusedOnElevatedService(t *testing.T) {
	addCapTemplate(t, netAdminLine, "NET_ADMIN")
	rt, db := newTestRouterWithComposeSecrets(t)
	enableElevatedTier(t, rt, db)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/captpl/install", `{"name":"egr","domains":{"web":"e.example.com"}}`, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/egr-web/egress-policy", `{"mode":"allowlist","allow":[{"host":"example.com","port":443}]}`, http.StatusConflict)
}
