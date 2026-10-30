package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/mail/mox"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const tenantCompose = "services:\n  web:\n    image: nginx:1\n"

// A tenant can never install, deploy or label an app as the mail server,
// at any team role, through any path that records a template ID.
func TestMailTemplate_TenantCannotInstallOrDeploy(t *testing.T) {
	experimental.Set(experimental.Mail)
	// TestMain turns every feature on; restoring only Mail would gate later stack tests.
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	f := newTeamFixture(t)
	for _, role := range allTeamRoles {
		for _, c := range []struct{ path, body string }{
			{"/api/v1/service-templates/mail-mox/install", `{"env":{"MAIL_DOMAIN":"evil.test"}}`},
			{"/api/v1/service-templates/mail-mox/deploy", "{}"},
			{"/api/v1/apps/" + appA + "/compose?template=mail-mox", tenantCompose},
			{"/api/v1/apps/tenant-mail/compose?template=mail-mox", tenantCompose}, // new names already 404 for tenants
		} {
			rec := sdReq(t, f.rt, f.cookies[role], http.MethodPost, c.path, c.body)
			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Errorf("%s POST %s = %d, want 403 or 404: %s", role, c.path, rec.Code, rec.Body.String())
			}
		}
	}
	assertNoMailTemplateApps(t, f.db)
}

// A tenant's ?template= label on a generic compose deploy is ignored.
func TestComposeDeploy_TenantTemplateLabelIgnored(t *testing.T) {
	f := newTeamFixture(t)
	var other string
	for _, tpl := range catalog.Templates {
		if !instanceOnlyTemplates[tpl.ID] && templateAvailable(tpl.ID) {
			other = tpl.ID
			break
		}
	}
	if other == "" {
		t.Skip("no ungated catalog template")
	}
	rec := sdReq(t, f.rt, f.cookies[store.TeamRoleOwner], http.MethodPost, "/api/v1/apps/"+appA+"/compose?template="+other, tenantCompose)
	t.Logf("tenant compose deploy = %d", rec.Code)
	svcs, err := f.db.ListDesiredServices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range svcs {
		if s.TemplateID != "" {
			t.Errorf("tenant deploy recorded template %q on %s", s.TemplateID, s.Name)
		}
	}
}

func assertNoMailTemplateApps(t *testing.T, db *store.DB) {
	t.Helper()
	svcs, err := db.ListDesiredServices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range svcs {
		if s.TemplateID == mailTemplateID {
			t.Errorf("a tenant request created mail app %s", s.Name)
		}
	}
}

// A mail-mox app owned by a tenant team is never the mail server: the
// lookup skips it and no admin request ever reaches its container.
func TestMailLookup_IgnoresTenantOwnedMailApp(t *testing.T) {
	var hits atomic.Int32
	tenantMox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer tenantMox.Close()
	_, port, _ := net.SplitHostPort(tenantMox.Listener.Addr().String())
	t.Setenv(mox.AdminPortEnv, port)

	f := newTeamFixture(t)
	ctx := context.Background()
	rt := f.rt
	rt.composeSecrets = fixedSecrets{}
	tenant := store.DesiredService{Name: "t-mail", Image: "r.xmox.nl/mox:v0.0.17", Port: 8080, TemplateID: mailTemplateID}
	if err := f.db.SaveDesiredService(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if err := f.db.UpdateServiceProject(ctx, tenant.Name, projA); err != nil {
		t.Fatal(err)
	}
	saved, err := f.db.GetDesiredService(ctx, tenant.Name)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	if rt.instance != nil {
		info, err := rt.instanceInfo(ctx)
		if err != nil {
			t.Fatal(err)
		}
		labels[appspec.InstanceLabelKey] = info.InstanceID
	}
	rt.containers = prefixLister{all: []docker.ContainerState{{
		Name: application.ContainerName(saved.Name, application.NameImage(*saved), saved.RestartNonce), Running: true, Labels: labels,
		Networks: []docker.ContainerNetwork{{Name: "app", IPAddress: "127.0.0.1"}},
	}}}

	if _, err := rt.mailEngineContainer(ctx, *saved); err == nil {
		t.Fatal("a tenant owned mail app must not resolve as the engine")
	}
	if e, err := rt.defaultMailEngine(ctx, saved.Name); err == nil {
		_, _ = e.Domains(ctx)
		t.Fatal("defaultMailEngine built an engine for a tenant owned app")
	}
	if _, found, err := rt.pickMailApp(httptest.NewRequest(http.MethodGet, "/api/v1/mail/domains", nil), ""); err != nil || found {
		t.Fatalf("pickMailApp chose a tenant app: found=%v err=%v", found, err)
	}
	if st := rt.mailProbe(ctx, *saved); st.AdminReachable {
		t.Fatalf("status probed the tenant app: %+v", st)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the tenant mox received %d admin requests", n)
	}

	// Control: the same app owned by the instance resolves.
	if err := f.db.UpdateServiceProject(ctx, tenant.Name, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.mailEngineContainer(ctx, *saved); err != nil {
		t.Fatalf("instance owned mail app: %v", err)
	}
}
