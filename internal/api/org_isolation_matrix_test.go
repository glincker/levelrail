package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// knownOrgLeaks maps a case name ("<Test>/<case>") to why it leaks today. A
// listed case skips while it leaks and fails once it stops, so the slice that
// fixes a leak must delete its entry here.
var knownOrgLeaks = map[string]string{}

const orgBMarker = "orgb-secret"

var orgBMarkers = []string{appB, dbB, projB, envB, teamB, orgBMarker}

// orgCase records one verdict: leak is empty when the case holds.
func orgCase(t *testing.T, name, leak string) {
	t.Helper()
	reason, known := knownOrgLeaks[name]
	switch {
	case known && leak != "":
		t.Run(name, func(t *testing.T) { t.Skip(reason) })
	case known:
		t.Errorf("%s no longer leaks, delete its knownOrgLeaks entry", name)
	case leak != "":
		t.Errorf("%s: %s", name, leak)
	}
}

// orgCases reports a batch of verdicts and flags stale entries of the group.
func orgCases(t *testing.T, group string, leaks map[string]string, checked []string) {
	t.Helper()
	sort.Strings(checked)
	seen := map[string]bool{}
	for _, name := range checked {
		seen[group+"/"+name] = true
		orgCase(t, group+"/"+name, leaks[name])
	}
	for name := range knownOrgLeaks {
		if strings.HasPrefix(name, group+"/") && !seen[name] {
			t.Errorf("knownOrgLeaks entry %q matches no checked case", name)
		}
	}
}

func orgLeakIn(body string) string {
	for _, m := range orgBMarkers {
		if strings.Contains(body, m) {
			return m
		}
	}
	return ""
}

type orgFixture struct {
	*teamFixture
	root     *http.Cookie
	bOwner   *http.Cookie
	dualUser store.User
	dual     *http.Cookie
}

// newOrgFixture adds a root session, an org B owner and a user in both the
// default org and org A, and seeds org B objects across the tenant lists.
func newOrgFixture(t *testing.T) *orgFixture {
	t.Helper()
	f := &orgFixture{teamFixture: newTeamFixture(t)}
	ctx := context.Background()
	// Doctor checks must not touch the real network or resolver, or a slow CI host times the run out.
	withOfflineDoctorNetwork(f.rt)
	f.rt.doctorLookupHost = func(context.Context, string) ([]string, error) {
		return nil, errors.New("lookup disabled in tests")
	}
	// Probes run with strict isolation on, the mode the leaks are closed for.
	if _, err := f.rt.runtimeSettings.Set(ctx, runtimesettings.KeyOrgsStrictIsolation, json.RawMessage("true"), "test"); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	f.root = loginTestSession(t, f.rt, f.db)
	bu, err := f.rt.createLocalUserInTeam(ctx, "owner@org-b.test", "", teamMatrixPassword, store.TeamRoleAbilities(store.TeamRoleOwner), teamB, store.TeamRoleOwner)
	must(err)
	f.bOwner = sessionCookieForTest(t, f.rt, bu.ID)
	f.dualUser, err = f.rt.createLocalUserInTeam(ctx, "dual@instance.test", "", teamMatrixPassword,
		[]string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy}, store.DefaultTeamID, store.TeamRoleDeveloper)
	must(err)
	must(f.db.AddTeamMember(ctx, teamA, f.dualUser.ID, store.TeamRoleDeveloper))
	f.dual = sessionCookieForTest(t, f.rt, f.dualUser.ID)

	for _, s := range []struct{ app, proj, env, domain string }{
		{appA, projA, envA, "orga-site.example.test"},
		{appB, projB, envB, orgBMarker + ".example.test"},
	} {
		must(f.db.SaveDesiredService(ctx, store.DesiredService{Name: s.app, Image: "nginx:1", Port: 80, Domains: []string{s.domain}}))
		must(f.db.UpdateServiceProject(ctx, s.app, s.proj))
		must(f.db.SetServiceEnvironment(ctx, s.app, s.env))
	}
	f.seedDeployAttempt(t, appB)
	must(f.db.ClaimMailDomain(ctx, store.MailDomain{Domain: "mail." + orgBMarker + ".example.test", Service: appB, TeamID: teamB, CreatedBy: "seed", CreatedAt: "2026-10-01T00:00:00Z"}))
	must(f.db.CreateSavedLogQuery(ctx, store.SavedLogQuery{ID: "slq_orgb", Name: "q-" + orgBMarker, App: appB, SpecJSON: "{}", OwnerType: store.PrincipalTypeUser,
		OwnerID: bu.ID, OwnerName: "b", Visibility: "shared", CreatedAt: time.Now(), UpdatedAt: time.Now()}))
	must(f.db.SaveBackupTarget(ctx, store.BackupTarget{ID: "bt_orgb", Name: "target-" + orgBMarker, Provider: "custom", Endpoint: "https://s3.example.test", Region: "r", Bucket: "bucket-" + orgBMarker, CreatedAt: "2026-10-01T00:00:00Z"}))
	must(f.db.CreateSilence(ctx, store.Silence{ID: "sil_orgb", IssueKey: "app:" + appB, CreatedBy: "seed", Reason: orgBMarker, CreatedAt: time.Now()}))
	auditID, err := store.NewAuditEntryID()
	must(err)
	must(f.db.SaveAuditEntry(ctx, store.AuditEntry{ID: auditID, ActorType: "session", ActorID: bu.ID, ActorName: "owner@org-b.test", Ability: AbilityWrite,
		Method: http.MethodPost, Path: "/api/v1/apps/" + appB + "/restart", StatusCode: 200, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "dashboard"}))
	return f
}

func (f *orgFixture) seedDeployAttempt(t *testing.T, app string) {
	t.Helper()
	id, err := store.NewDeployAttemptID()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SaveDeployAttempt(context.Background(), store.DeployAttempt{ID: id, ServiceName: app, Image: "nginx:2", Status: "succeeded", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

var orgSkipPath = regexp.MustCompile(`stream|watch|events|tail|follow|sse|/ws`)

// orgInstanceGETs are the parameterless GET routes outside the tenant allowlist.
func orgInstanceGETs(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, r := range loadMatrixRoutes(t) {
		_, path, _ := strings.Cut(r.key(), " ")
		if r.method != http.MethodGet || strings.Contains(path, "{") || orgSkipPath.MatchString(path) {
			continue
		}
		if otherTeamPath(r.pattern) != "" || routeIsTenantOpen(r.key(), path) || strings.HasPrefix(path, "/api/v1/auth/") {
			continue
		}
		if _, ok := publicRoutes[r.key()]; ok {
			continue
		}
		if _, ok := libraryOnlyRoutes[r.key()]; ok {
			continue
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// orgProbe fetches paths on a worker pool and returns marker hits by path.
func orgProbe(rt *Router, paths []string, mutate func(*http.Request)) map[string]string {
	hits := map[string]string{}
	res := probeParallel(paths, func(p string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, p, nil)
		mutate(req)
		rec := httptest.NewRecorder()
		matrixHandler(rt).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			return ""
		}
		if m := orgLeakIn(rec.Body.String()); m != "" {
			return p + "\t" + fmt.Sprintf("200 body names %q", m)
		}
		return ""
	})
	for _, r := range res {
		p, why, _ := strings.Cut(r, "\t")
		hits[p] = why
	}
	return hits
}

// orgInstanceFamilies are route families only the instance can use: a tenant
// answers 403 (TestTeamAuthzMatrix_TenantInstanceRoutesClosed proves it).
// A new family must be added here after its tenant stance is reviewed.
var orgInstanceFamilies = strings.Fields(`
ai alert-defaults alert-history alert-maintenance-windows alert-silences alert-templates analysis app-types apply apps
apps-metrics apps-summary archives assets audit-log backup-targets backups benchmarks bitbucket-app build build-cache
build-limits builds cdn certificates changelog cleanup compliance compose control-plane costs dashboards database-catalog
database-engines database-versions databases deploy-policies deployments deploys diagnose dns dockerhub doctor domains
environments event-webhook-deliveries event-webhook-types event-webhooks export external-databases firewall
firewall-rules fixes flags forgejo-app frameworks function-invocations function-kv functions git git-connections
git-providers git-sources gitea-app github-app gitlab-app go-live gpus iam images import imports inbox incidents ingress
instance integrations invites loadbalancers local-mode log-archive logs mail maintenance mcp me mesh migration migrations
mobile model-cache model-catalog model-endpoints models monitors network network-shares node-drains node-moves
node-providers node-provisions nodes notification-channels onboarding openapi.json organizations package-registries
pipeline-runs pipelines plugin-keys plugin-marketplace plugin-panels plugins preflight previews private-networks
project-templates projects prometheus registry registry-credentials registry-vendors resilience roles scheduled-maintenance scim
search secret-rotation security service-templates settings setup silences ssh-node-provisions sso stack-templates stacks
static-sites status-page status-reasons storage storage-env-keys system tags teams templates updates usage users`)

func orgRouteClass(key, pattern string) string {
	_, path, _ := strings.Cut(key, " ")
	switch {
	case tenantRoutes[key]:
		return "tenant"
	case otherTeamPath(pattern) != "":
		return "team-owned"
	case strings.HasPrefix(path, "/api/v1/auth/") || strings.HasPrefix(path, "/api/v1/team-invites/") || strings.HasPrefix(path, "/api/v1/teams/{id}"):
		return "auth-or-team"
	}
	if _, ok := publicRoutes[key]; ok {
		return "public"
	}
	if _, ok := libraryOnlyRoutes[key]; ok {
		return "public"
	}
	if _, ok := tenantSessionOnly[key]; ok {
		return "session-only"
	}
	if path == "/metrics" || strings.HasPrefix(path, "/api/v2/") {
		return "instance"
	}
	if rest, ok := strings.CutPrefix(path, "/api/v1/"); ok {
		family, _, _ := strings.Cut(rest, "/")
		for _, f := range orgInstanceFamilies {
			if f == family {
				return "instance"
			}
		}
	}
	return ""
}

// TestOrgIsolation_RouteCoverage fails for a registered route no isolation
// class names, so a new route family gets a tenant review before it ships.
func TestOrgIsolation_RouteCoverage(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	var unclassified []string
	routes := loadMatrixRoutes(t)
	if len(routes) < 500 {
		t.Fatalf("only %d routes loaded, the route scan drifted", len(routes))
	}
	for _, r := range routes {
		class := orgRouteClass(r.key(), r.pattern)
		if class == "" {
			unclassified = append(unclassified, r.key())
			continue
		}
		_, path, _ := strings.Cut(r.key(), " ")
		if rest, ok := strings.CutPrefix(path, "/api/v1/"); ok && class == "instance" {
			family, _, _ := strings.Cut(rest, "/")
			used[family] = true
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Errorf("routes with no org isolation class (tenant route, team-owned, auth, or add the family to orgInstanceFamilies after review):\n%s", strings.Join(unclassified, "\n"))
	}
	for _, f := range orgInstanceFamilies {
		if !used[f] {
			t.Errorf("orgInstanceFamilies entry %q matches no route, remove it", f)
		}
	}
}

var orgIDRe = regexp.MustCompile(`(?:org|proj|env|appr)_[a-z0-9_]+|[ab]-(?:web|db)|zz-missing-[a-z]+|req_[A-Za-z0-9_-]+`)

// TestOrgIsolation_TenantPathRoutes404Uniform: an org B object answers an org
// A member exactly like an id that does not exist.
func TestOrgIsolation_TenantPathRoutes404Uniform(t *testing.T) {
	f := newOrgFixture(t)
	type pair struct{ found, missing string }
	var pairs []pair
	ids := map[string]string{appB: "zz-missing-app", dbB: "zz-missing-db", projB: "proj_zz_missing", envB: "env_zz_missing", teamB: "org_zz_missing", approvalB: "appr_zz_missing"}
	for _, r := range loadMatrixRoutes(t) {
		_, path, _ := strings.Cut(r.key(), " ")
		if r.method != http.MethodGet || otherTeamPath(r.pattern) == "" || orgSkipPath.MatchString(path) {
			continue
		}
		found := otherTeamPath(r.pattern)
		missing := found
		for id, gone := range ids {
			missing = strings.Replace(missing, id, gone, 1)
		}
		pairs = append(pairs, pair{found, missing})
	}
	if len(pairs) < 20 {
		t.Fatalf("only %d tenant path routes found", len(pairs))
	}
	owner := withCookie(f.cookies[store.TeamRoleOwner])
	leaks := map[string]string{}
	var checked []string
	for _, p := range pairs {
		name := "GET " + p.found
		checked = append(checked, name)
		c1, b1 := teamDo(t, f.rt, http.MethodGet, p.found, "", owner)
		c2, b2 := teamDo(t, f.rt, http.MethodGet, p.missing, "", owner)
		n1, n2 := orgIDRe.ReplaceAllString(b1, "ID"), orgIDRe.ReplaceAllString(b2, "ID")
		if c1 != c2 || n1 != n2 {
			leaks[name] = fmt.Sprintf("org B answers %d %q, a missing id answers %d %q", c1, n1, c2, n2)
		}
	}
	orgCases(t, "TenantPathRoutes404Uniform", leaks, checked)
}

// TestOrgIsolation_TenantListsAndCounts: lists, counts, search and suggest
// show an org A member nothing of org B.
func TestOrgIsolation_TenantListsAndCounts(t *testing.T) {
	f := newOrgFixture(t)
	lists := []string{
		"/api/v1/apps", "/api/v1/databases", "/api/v1/projects", "/api/v1/environments", "/api/v1/organizations", "/api/v1/teams",
		"/api/v1/domains", "/api/v1/deployments", "/api/v1/deployments/summary", "/api/v1/usage/summary", "/api/v1/traffic/summary",
		"/api/v1/mail/domains", "/api/v1/mail/mailboxes", "/api/v1/static-sites",
		"/api/v1/attention/feed", "/api/v1/deploy-approvals?status=all",
		"/api/v1/search?q=b&limit=100", "/api/v1/search?q=" + orgBMarker, "/api/v1/search?q=web", "/api/v1/search/suggest?q=b", "/api/v1/search/suggest?q=" + orgBMarker,
	}
	leaks := map[string]string{}
	var checked []string
	for _, role := range []string{store.TeamRoleOwner, store.TeamRoleViewer} {
		for _, path := range lists {
			name := role + " GET " + path
			checked = append(checked, name)
			code, body := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(f.cookies[role]))
			if code != http.StatusOK {
				leaks[name] = fmt.Sprintf("status %d %s", code, body)
			} else if m := orgLeakIn(body); m != "" {
				leaks[name] = fmt.Sprintf("body names %q", m)
			}
		}
	}
	// A count must not move when org B gains a deployment.
	counts := []string{"/api/v1/deployments/summary", "/api/v1/usage/summary", "/api/v1/traffic/summary"}
	owner := withCookie(f.cookies[store.TeamRoleOwner])
	before := map[string]string{}
	for _, p := range counts {
		_, before[p] = teamDo(t, f.rt, http.MethodGet, p, "", owner)
	}
	f.seedDeployAttempt(t, appB)
	for _, p := range counts {
		name := "count " + p
		checked = append(checked, name)
		if _, after := teamDo(t, f.rt, http.MethodGet, p, "", owner); after != before[p] {
			leaks[name] = "the summary changed when org B gained a deployment"
		}
	}
	for _, path := range []string{"/api/v1/domains", "/api/v1/apps"} {
		_, body := teamDo(t, f.rt, http.MethodGet, path, "", owner)
		if !strings.Contains(body, appA) && !strings.Contains(body, "orga-site.example.test") {
			t.Errorf("owner of org A does not see its own data in %s: %s", path, body)
		}
	}
	orgCases(t, "TenantListsAndCounts", leaks, checked)
}

// TestOrgIsolation_DefaultMemberInstanceRoutes: a member of the default org
// plus org A keeps instance routes, which must not show org B data.
func TestOrgIsolation_DefaultMemberInstanceRoutes(t *testing.T) {
	f := newOrgFixture(t)
	paths := orgInstanceGETs(t)
	if len(paths) < 100 {
		t.Fatalf("only %d instance GET routes found", len(paths))
	}
	for _, must := range []string{"/api/v1/audit-log", "/api/v1/logs/queries", "/api/v1/notification-channels", "/api/v1/backups", "/api/v1/nodes", "/api/v1/alert-silences", "/api/v1/event-webhooks"} {
		found := false
		for _, p := range paths {
			found = found || p == must
		}
		if !found {
			t.Errorf("expected instance route %s is not probed", must)
		}
	}
	hits := orgProbe(f.rt, paths, func(r *http.Request) { r.AddCookie(f.dual) })
	orgCases(t, "DefaultMemberInstanceRoutes", hits, paths)
}

// TestOrgIsolation_OwnerlessTokenScope: a token with no owner and no stored
// scope must not read every org.
func TestOrgIsolation_OwnerlessTokenScope(t *testing.T) {
	f := newOrgFixture(t)
	tok := mintAgent(t, f.rt, "ownerless", []string{AbilityRead, AbilityReadSensitive}, agentIdentity{})
	leaks := map[string]string{}
	var checked []string
	for _, p := range []string{"/api/v1/apps", "/api/v1/apps/" + appB, "/api/v1/databases", "/api/v1/databases/" + dbB, "/api/v1/projects", "/api/v1/projects/" + projB,
		"/api/v1/environments", "/api/v1/organizations", "/api/v1/organizations/" + teamB, "/api/v1/domains", "/api/v1/search?q=" + orgBMarker} {
		checked = append(checked, "GET "+p)
		code, body := teamDo(t, f.rt, http.MethodGet, p, "", bearer(tok))
		if code == http.StatusOK {
			if m := orgLeakIn(body); m != "" {
				leaks["GET "+p] = fmt.Sprintf("ownerless token reads org B, body names %q", m)
			}
		}
	}
	// A token minted by an org A member inherits that member's scope.
	code, minted := mintTeamToken(t, f.teamFixture, store.TeamRoleDeveloper, `{"name":"inherits","abilities":["read"]}`)
	if code != http.StatusCreated {
		t.Fatalf("mint = %d %s", code, minted)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appB, "", bearer(minted)); got != http.StatusNotFound {
		t.Errorf("a token minted by an org A member reads org B = %d, want 404", got)
	}
	orgCases(t, "OwnerlessTokenScope", leaks, checked)
}

// TestOrgIsolation_ScopedTokenOtherOrg: a token never reaches an org its
// owner is outside, nor instance routes.
func TestOrgIsolation_ScopedTokenOtherOrg(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	mk := func(id, owner, team string) string {
		plain := "scoped-secret-" + id
		if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityWrite, AbilityDeploy}, OwnerUserID: owner}); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: id, TeamID: team, Role: store.TeamRoleOwner}); err != nil {
			t.Fatal(err)
		}
		return plain
	}
	tokens := map[string]string{
		"owner-of-A scoped to B": mk("tok-a-owner-scoped-b", f.members[store.TeamRoleOwner].ID, teamB),
		"owner-of-A scoped to A": mk("tok-a-owner-scoped-a", f.members[store.TeamRoleOwner].ID, teamA),
	}
	leaks := map[string]string{}
	var checked []string
	for label, tok := range tokens {
		for _, p := range []string{"/api/v1/apps", "/api/v1/apps/" + appB, "/api/v1/databases/" + dbB, "/api/v1/projects/" + projB, "/api/v1/environments/" + envB,
			"/api/v1/organizations/" + teamB, "/api/v1/projects", "/api/v1/domains", "/api/v1/search?q=" + orgBMarker} {
			name := label + " GET " + p
			checked = append(checked, name)
			code, body := teamDo(t, f.rt, http.MethodGet, p, "", bearer(tok))
			if m := orgLeakIn(body); code == http.StatusOK && m != "" {
				leaks[name] = "token reads org B: " + m
			}
		}
		for _, p := range []string{"/api/v1/notification-channels", "/api/v1/nodes", "/api/v1/audit-log", "/api/v1/logs/queries", "/api/v1/backups"} {
			name := label + " instance GET " + p
			checked = append(checked, name)
			if code, _ := teamDo(t, f.rt, http.MethodGet, p, "", bearer(tok)); code != http.StatusForbidden {
				leaks[name] = fmt.Sprintf("org scoped token reaches an instance route = %d, want 403", code)
			}
		}
	}
	orgCases(t, "ScopedTokenOtherOrg", leaks, checked)
}

// TestOrgIsolation_BodyRefsOtherOrg: org B ids inside a request body are
// refused and change nothing.
func TestOrgIsolation_BodyRefsOtherOrg(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	dev := withCookie(f.cookies[store.TeamRoleDeveloper])
	admin := withCookie(f.cookies[store.TeamRoleAdmin])
	cases := []struct {
		name, method, path, body string
		who                      func(*http.Request)
	}{
		{"deploy org B app", http.MethodPost, "/api/v1/apps/" + appB + "/deploys", `{"image":"nginx:3","confirm":true}`, dev},
		{"move app into org B project", http.MethodPut, "/api/v1/apps/" + appA + "/project", `{"project_id":"` + projB + `"}`, dev},
		{"move app into org B environment", http.MethodPut, "/api/v1/apps/" + appA + "/environment", `{"environment_id":"` + envB + `"}`, dev},
		{"move database into org B environment", http.MethodPut, "/api/v1/databases/" + dbA + "/environment", `{"environment_id":"` + envB + `"}`, dev},
		{"move project into org B", http.MethodPut, "/api/v1/projects/" + projA + "/organization", `{"org_id":"` + teamB + `"}`, admin},
		{"clone org B app", http.MethodPost, "/api/v1/apps/" + appB + "/clone", `{"new_name":"b-copy"}`, dev},
		{"clone into org B environment", http.MethodPost, "/api/v1/apps/" + appA + "/clone", `{"new_name":"a-into-b","environment_id":"` + envB + `"}`, dev},
		{"clone into org B project", http.MethodPost, "/api/v1/apps/" + appA + "/clone", `{"new_name":"a-into-b2","project_id":"` + projB + `"}`, dev},
		{"clone org B environment", http.MethodPost, "/api/v1/environments/" + envB + "/clone", `{"new_environment_name":"stolen"}`, admin},
		{"promote into org B environment", http.MethodPost, "/api/v1/apps/" + appA + "/promote", `{"to":"` + envB + `"}`, dev},
		{"create app in org B project", http.MethodPost, "/api/v1/apps", `{"name":"sneak-b","image":"nginx:1","port":80,"project_id":"` + projB + `"}`, dev},
		{"approve org B approval", http.MethodPost, "/api/v1/deploy-approvals/" + approvalB + "/approve", "", admin},
		{"reject org B approval", http.MethodPost, "/api/v1/deploy-approvals/" + approvalB + "/reject", `{"reason":"no"}`, admin},
	}
	// A dropped body ref is fine while the copy stays in org A.
	ignoredRef := map[string]string{"clone into org B project": "a-into-b2"}
	leaks := map[string]string{}
	var checked []string
	for _, c := range cases {
		checked = append(checked, c.name)
		code, body := teamDo(t, f.rt, c.method, c.path, c.body, c.who)
		switch {
		case code < 400 && ignoredRef[c.name] != "":
			if o, _ := f.db.OwnerOfApp(ctx, ignoredRef[c.name]); o == nil || o.TeamID != teamA {
				leaks[c.name] = fmt.Sprintf("accepted with %d and the copy is not owned by org A: %+v", code, o)
			}
		case code < 400:
			leaks[c.name] = fmt.Sprintf("accepted with %d %s", code, body)
		case code >= 500:
			leaks[c.name] = fmt.Sprintf("server error %d", code)
		}
	}
	// Nothing may have moved or been created.
	if o, err := f.db.OwnerOfApp(ctx, appA); err != nil || o == nil || o.TeamID != teamA || o.ProjectID != projA {
		t.Errorf("org A app owner changed: %+v %v", o, err)
	}
	for _, name := range []string{"b-copy", "a-into-b", "sneak-b"} {
		if o, _ := f.db.OwnerOfApp(ctx, name); o != nil {
			t.Errorf("app %q was created, owner %+v", name, o)
		}
	}
	if a, err := f.db.GetDeployApproval(ctx, approvalB); err != nil || a.Status != store.DeployApprovalStatusPending {
		t.Errorf("org B approval changed: %+v %v", a, err)
	}
	orgCases(t, "BodyRefsOtherOrg", leaks, checked)
}

// TestOrgIsolation_GlobalEnvironmentVars: a global environment is shared
// across orgs, so a value an org A member sets there must not reach an org B app.
func TestOrgIsolation_GlobalEnvironmentVars(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	const globalEnv = "env_dev"
	secret := "orga-global-" + "value"
	for _, app := range []string{appA, appB} {
		if err := f.db.SetServiceEnvironment(ctx, app, globalEnv); err != nil {
			t.Fatal(err)
		}
	}
	owner := withCookie(f.cookies[store.TeamRoleOwner])
	setCode, setBody := teamDo(t, f.rt, http.MethodPut, "/api/v1/environments/"+globalEnv+"/env", `{"SHARED_TOKEN":"`+secret+`"}`, owner)
	t.Logf("org A owner sets a global environment variable: %d %s", setCode, setBody)
	if setCode >= 300 {
		// Refused for a tenant, so the value cannot cross orgs.
		orgCase(t, "GlobalEnvironmentVars/org A value reaches org B", "")
		return
	}
	leak := ""
	for _, p := range []string{"/api/v1/apps/" + appB + "/env", "/api/v1/apps/" + appB + "/env/bound", "/api/v1/apps/" + appB + "/env/export", "/api/v1/environments/" + globalEnv + "/env"} {
		code, body := teamDo(t, f.rt, http.MethodGet, p, "", withCookie(f.bOwner))
		t.Logf("org B owner GET %s = %d", p, code)
		if code == http.StatusOK && strings.Contains(body, secret) {
			leak = "org B owner reads org A's global value through " + p
		}
	}
	orgCase(t, "GlobalEnvironmentVars/org A value reaches org B", leak)
}

// TestOrgIsolation_CreateOrgPathsAgree: POST /organizations and POST /teams
// must apply one rule.
func TestOrgIsolation_CreateOrgPathsAgree(t *testing.T) {
	f := newOrgFixture(t)
	leaks := map[string]string{}
	names := []string{"non-root member of the default org with write", "creator becomes owner"}
	codeOrg, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/organizations", `{"name":"made-by-dual"}`, withCookie(f.dual))
	codeTeam, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/teams", `{"name":"made-by-dual-team"}`, withCookie(f.dual))
	if (codeOrg < 300) != (codeTeam < 300) {
		leaks[names[0]] = fmt.Sprintf("POST /organizations = %d, POST /teams = %d", codeOrg, codeTeam)
	}
	owned := map[string]bool{}
	for path, name := range map[string]string{"/api/v1/organizations": "root-org", "/api/v1/teams": "root-team"} {
		code, body := teamDo(t, f.rt, http.MethodPost, path, `{"name":"`+name+`"}`, withCookie(f.root))
		if code != http.StatusCreated {
			t.Fatalf("root POST %s = %d %s", path, code, body)
		}
		var res struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(body), &res); err != nil || res.ID == "" {
			t.Fatalf("create response %s: %v", body, err)
		}
		members, err := f.db.ListTeamMembers(context.Background(), res.ID)
		if err != nil {
			t.Fatal(err)
		}
		owned[path] = len(members) > 0
	}
	if owned["/api/v1/organizations"] != owned["/api/v1/teams"] {
		leaks[names[1]] = fmt.Sprintf("POST /organizations gives the creator a member row = %v, POST /teams = %v", owned["/api/v1/organizations"], owned["/api/v1/teams"])
	}
	orgCases(t, "CreateOrgPathsAgree", leaks, names)
}

// TestOrgIsolation_SSOJITDoesNotGrantDefaultOrg: SSO and SCIM provisioning
// share one store path; a new user must not land in the default org, which is
// instance access.
func TestOrgIsolation_SSOJITDoesNotGrantDefaultOrg(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	if err := f.db.CreateRole(ctx, store.Role{ID: "role_jit", Name: "jit", Abilities: []string{AbilityRead}, Visibility: store.RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}
	// Existing installs keep the default org join until the setting is turned off.
	if err := f.db.CreateProvisionedUser(f.rt.provisionCtx(ctx), store.User{ID: "user_kept", Email: "kept@idp.test", DisplayName: "kept", CreatedAt: time.Now()}, "role_jit"); err != nil {
		t.Fatal(err)
	}
	if kept, err := f.db.UserTeamRoles(ctx, "user_kept"); err != nil || kept[store.DefaultTeamID] == "" {
		t.Fatalf("default setting must keep the default org join, roles=%v err=%v", kept, err)
	}
	if _, err := f.rt.runtimeSettings.Set(ctx, runtimesettings.KeyOrgsProvisionedDefaultMembership, json.RawMessage("false"), "test"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.CreateProvisionedUser(f.rt.provisionCtx(ctx), store.User{ID: "user_jit", Email: "jit@idp.test", DisplayName: "jit", CreatedAt: time.Now()}, "role_jit"); err != nil {
		t.Fatal(err)
	}
	roles, err := f.db.UserTeamRoles(ctx, "user_jit")
	if err != nil {
		t.Fatal(err)
	}
	leak := ""
	if roles[store.DefaultTeamID] != "" {
		leak = "a provisioned user joins the default org, which opens every instance route"
	}
	name := "provisioned user org membership"
	orgCases(t, "SSOJITDoesNotGrantDefaultOrg", map[string]string{name: leak}, []string{name})
}

// TestOrgIsolation_MCPToolsUseCallerScope: MCP over HTTP runs tools with the
// calling token's scope.
func TestOrgIsolation_MCPToolsUseCallerScope(t *testing.T) {
	mcpEnabled(t)
	f := newOrgFixture(t)
	ctx := context.Background()
	mk := func(id, owner, team string) string {
		plain := "mcp-secret-" + id
		if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: []string{AbilityRead}, OwnerUserID: owner}); err != nil {
			t.Fatal(err)
		}
		if team != "" {
			if err := f.db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: id, TeamID: team, Role: store.TeamRoleViewer}); err != nil {
				t.Fatal(err)
			}
		}
		return plain
	}
	tokens := map[string]string{
		"org A scoped token": mk("mcp-a", f.members[store.TeamRoleOwner].ID, teamA),
		"org A owner token":  mk("mcp-a-owner", f.members[store.TeamRoleOwner].ID, ""),
		"ownerless token":    mk("mcp-none", "", ""),
	}
	leaks := map[string]string{}
	var checked []string
	for label, tok := range tokens {
		for _, call := range []string{"list_apps", "list_databases", "list_projects"} {
			name := label + " " + call
			checked = append(checked, name)
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":%q,"arguments":{}}}`, call)
			rec := mcpPost(f.rt, f.rt.Handler(), tok, body, nil)
			if m := orgLeakIn(rec.Body.String()); rec.Code == http.StatusOK && m != "" {
				leaks[name] = "tool result names org B: " + m
			}
		}
	}
	orgCases(t, "MCPToolsUseCallerScope", leaks, checked)
}

// TestOrgIsolation_CreateOrgOwnerRowStrictOff: with strict isolation off the
// permission is unchanged (write is enough) but the creator still becomes owner.
func TestOrgIsolation_CreateOrgOwnerRowStrictOff(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	if _, err := f.rt.runtimeSettings.Set(ctx, runtimesettings.KeyOrgsStrictIsolation, json.RawMessage("false"), "test"); err != nil {
		t.Fatal(err)
	}
	code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/organizations", `{"name":"made-by-dual"}`, withCookie(f.dual))
	if code != http.StatusCreated {
		t.Fatalf("POST /organizations with strict off = %d %s", code, body)
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil || res.ID == "" {
		t.Fatalf("create response %s: %v", body, err)
	}
	roles, err := f.db.UserTeamRoles(ctx, f.dualUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if roles[res.ID] != store.TeamRoleOwner {
		t.Errorf("creator role in the new org = %q, want owner", roles[res.ID])
	}
}

// ctxCapturingOAuth stands in for the library engine: it provisions the user
// with the context it is given, the way the real engine does.
type ctxCapturingOAuth struct {
	AuthLibOAuth
	db *store.DB
	id string
}

func (c *ctxCapturingOAuth) ResolveOAuthUser(ctx context.Context, _, _, _ string) (string, bool, error) {
	err := c.db.CreateProvisionedUser(ctx, store.User{ID: c.id, Email: c.id + "@idp.test", DisplayName: c.id, CreatedAt: time.Now()}, "role_oauth_jit")
	return c.id, false, err
}

// TestOrgIsolation_OAuthJITHonorsDefaultMembership drives the real settle path.
func TestOrgIsolation_OAuthJITHonorsDefaultMembership(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	if err := f.db.CreateRole(ctx, store.Role{ID: "role_oauth_jit", Name: "oauth-jit", Abilities: []string{AbilityRead}, Visibility: store.RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.runtimeSettings.Set(ctx, runtimesettings.KeyOrgsProvisionedDefaultMembership, json.RawMessage("false"), "test"); err != nil {
		t.Fatal(err)
	}
	f.rt.oauthSettings = nil
	f.rt.authLibOAuth = &ctxCapturingOAuth{db: f.db, id: "user_oidc_jit"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/x/callback", nil)
	rec := httptest.NewRecorder()
	_, ok := f.rt.settleOAuthUser(rec, req, "x", authengine.OAuthOutcome{SessionCookie: &http.Cookie{Value: "s", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}, Provider: "x", ProviderUserID: "p"})
	if !ok {
		t.Fatalf("settle refused: %s", rec.Header().Get("Location"))
	}
	roles, err := f.db.UserTeamRoles(ctx, "user_oidc_jit")
	if err != nil {
		t.Fatal(err)
	}
	if roles[store.DefaultTeamID] != "" {
		t.Errorf("an OAuth provisioned user joined the default org: %v", roles)
	}
}

func orgDoctorMarkers(body string) bool { return strings.Contains(body, orgBMarker) }

// TestOrgIsolation_ReviewFixes covers the follow-up review findings.
func TestOrgIsolation_ReviewFixes(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	dual := withCookie(f.dual)
	root := withCookie(f.root)
	leaks := map[string]string{}
	var checked []string
	check := func(name string, bad bool, why string) {
		checked = append(checked, name)
		if bad {
			leaks[name] = why
		}
	}

	// H1
	code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/iam/resources/match", `{"resources":["app:*","database:*"]}`, dual)
	check("H1 iam match", code != http.StatusOK || orgLeakIn(body) != "", fmt.Sprintf("%d %s", code, body))

	// H2 and M3: a control proves root sees the org B domain in the check.
	_, rootRun := teamDo(t, f.rt, http.MethodPost, "/api/v1/system/doctor/checks/dns_domains/run", "", root)
	if !orgDoctorMarkers(rootRun) {
		t.Fatalf("control: root run-check does not name the org B domain: %s", rootRun)
	}
	code, body = teamDo(t, f.rt, http.MethodPost, "/api/v1/system/doctor/checks/dns_domains/run", "", dual)
	check("H2 run-check", code != http.StatusOK || orgLeakIn(body) != "", fmt.Sprintf("%d %s", code, body))
	code, body = teamDo(t, f.rt, http.MethodGet, "/api/v1/system/doctor/stream", "", dual)
	check("H2 stream", code != http.StatusOK || orgLeakIn(body) != "", fmt.Sprintf("%d", code))
	code, body = teamDo(t, f.rt, http.MethodGet, "/api/v1/system/doctor", "", dual)
	check("H2 doctor", code != http.StatusOK || orgLeakIn(body) != "", fmt.Sprintf("%d", code))

	// M3: an org B app called "disk" must not hide the host disk check.
	if err := f.db.SaveDesiredService(ctx, store.DesiredService{Name: "disk", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.UpdateServiceProject(ctx, "disk", projB); err != nil {
		t.Fatal(err)
	}
	_, body = teamDo(t, f.rt, http.MethodGet, "/api/v1/system/doctor", "", dual)
	check("M3 disk check stays", !strings.Contains(body, `"code":"disk_space"`), "the disk check was hidden by an org B app named disk")
	fl := &doctorFilter{names: []string{"disk", "orgb-app"}}
	check("M3 full name hides", !fl.hides(doctorCheckResource{Name: "Certificates", Message: "orgb-app expires soon"}), "a check naming the app is shown")
	check("M3 substring kept", fl.hides(doctorCheckResource{Name: "Backups", Message: "diskette and orgb-apps-extra"}), "a substring hid a check")

	// M1: a limit to org B is trusted for an ownerless token.
	mkTok := func(id string, scope *store.APITokenScope) string {
		plain := "rev-secret-" + id
		if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: []string{AbilityRead}}); err != nil {
			t.Fatal(err)
		}
		if scope != nil {
			scope.TokenID = id
			if err := f.db.SetAPITokenScope(ctx, *scope); err != nil {
				t.Fatal(err)
			}
		}
		return plain
	}
	limited := mkTok("tok-limited-b", &store.APITokenScope{TeamID: teamB, Role: store.TeamRoleOwner})
	code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appB, "", bearer(limited))
	check("M1 limited ownerless token", code != http.StatusOK, fmt.Sprintf("org B limited token = %d, want 200", code))

	// M2: marked system tokens are not narrowed, unmarked ones are.
	marked := mkTok("tok-system", &store.APITokenScope{TeamID: systemTokenTeamID, Role: store.TeamRoleOwner})
	code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appB, "", bearer(marked))
	check("M2 system token", code != http.StatusOK, fmt.Sprintf("system token = %d, want 200", code))
	plain := mkTok("tok-plain", nil)
	code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appB, "", bearer(plain))
	check("M2 plain ownerless stays narrowed", code != http.StatusNotFound, fmt.Sprintf("plain ownerless token = %d, want 404", code))
	drift, err := f.rt.driftToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appB, "", bearer(drift))
	check("M2 drift token", code != http.StatusOK, fmt.Sprintf("drift token = %d, want 200", code))

	// L1
	for _, p := range []string{"/api/v1/backup-targets/bt_orgb", "/api/v1/backup-targets/bt_orgb/encryption"} {
		code, _ = teamDo(t, f.rt, http.MethodGet, p, "", dual)
		check("L1 "+p, code != http.StatusNotFound, fmt.Sprintf("%d", code))
	}
	code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/backup-targets/bt_orgb", "", root)
	check("L1 root keeps access", code != http.StatusOK, fmt.Sprintf("root = %d", code))

	// L2
	c1, b1 := teamDo(t, f.rt, http.MethodGet, "/api/v1/imports/platform/databases/"+dbB, "", dual)
	c2, b2 := teamDo(t, f.rt, http.MethodGet, "/api/v1/imports/platform/databases/zz-missing-db", "", dual)
	check("L2 import database parity", c1 != c2 || c1 != http.StatusNotFound || orgLeakIn(b1) != "" || len(b2) == 0, fmt.Sprintf("%d vs %d %s", c1, c2, b1))

	// L3
	code, _ = teamDo(t, f.rt, http.MethodDelete, "/api/v1/silences/sil_orgb", "", dual)
	left, _ := f.db.ListSilences(ctx)
	check("L3 silence delete", code != http.StatusNotFound || len(left) == 0, fmt.Sprintf("%d, silences left %d", code, len(left)))

	// L4
	code, _ = teamDo(t, f.rt, http.MethodPost, "/api/v1/organizations", `{"name":"   "}`, root)
	check("L4 whitespace name", code != http.StatusBadRequest, fmt.Sprintf("%d", code))

	orgCases(t, "ReviewFixes", leaks, checked)
}
