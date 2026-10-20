package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type builderEnv struct {
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
}

func newBuilderEnv(t *testing.T) builderEnv {
	t.Helper()
	rt, db := newTestRouter(t)
	return builderEnv{rt: rt, db: db, cookie: loginTestSession(t, rt, db)}
}

func (e builderEnv) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, method, path, body))
	return rec
}

func (e builderEnv) seedApp(t *testing.T, name string) {
	t.Helper()
	seedApp(t, e.db, name)
}

func (e builderEnv) seedDB(t *testing.T, name string) {
	t.Helper()
	if err := e.db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
}

func (e builderEnv) seedToken(t *testing.T, id string, abilities []string) {
	t.Helper()
	seedMatrixToken(t, e.db, id, abilities)
}

func (e builderEnv) seedPolicy(t *testing.T, name, doc string) store.Policy {
	t.Helper()
	id, err := store.NewPolicyID()
	if err != nil {
		t.Fatal(err)
	}
	p := store.Policy{ID: id, Name: name, Document: doc, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := e.db.SavePolicy(context.Background(), p); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	return p
}

func (e builderEnv) attach(t *testing.T, p store.Policy, ptype, pid string) {
	t.Helper()
	id, err := store.NewPolicyAttachmentID()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.AttachPolicy(context.Background(), id, p.ID, ptype, pid); err != nil {
		t.Fatalf("attach: %v", err)
	}
}

func docOf(effect, actions, resources string) string {
	return `{"Statement":[{"Effect":"` + effect + `","Action":[` + actions + `],"Resource":[` + resources + `]}]}`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %T: %v (body %s)", out, err, rec.Body.String())
	}
	return out
}

func TestSimulatorAgreesWithEvaluator(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	e.seedApp(t, "prod-api")
	tests := []struct {
		name      string
		abilities []string
		policies  map[string]string
		action    string
		resource  string
		wantBy    string
		wantPol   string
	}{
		{"base read", []string{AbilityRead}, nil, AbilityRead, "app:web", decidedBaseAbility, ""},
		{"no grant", []string{AbilityRead}, nil, AbilityWrite, "app:web", decidedNoGrant, ""},
		{"allow grants", []string{AbilityRead}, map[string]string{"allow-web": docOf("Allow", `"write"`, `"app:web"`)}, AbilityWrite, "app:web", decidedExplicitAllow, "allow-web"},
		{"deny beats root", []string{AbilityRoot}, map[string]string{"deny-prod": docOf("Deny", `"write"`, `"app:prod*"`)}, AbilityWrite, "app:prod-api", decidedExplicitDeny, "deny-prod"},
		{"deny beats allow", nil, map[string]string{"a": docOf("Allow", `"*"`, `"*"`), "d": docOf("Deny", `"deploy"`, `"app:web"`)}, AbilityDeploy, "app:web", decidedExplicitDeny, "d"},
		{"deny elsewhere untouched", []string{AbilityRoot}, map[string]string{"deny-prod": docOf("Deny", `"write"`, `"app:prod*"`)}, AbilityWrite, "app:web", decidedBaseAbility, ""},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := fmt.Sprintf("sim-%d", i)
			e.seedToken(t, id, tt.abilities)
			var attached []store.Policy
			for n, doc := range tt.policies {
				p := e.seedPolicy(t, id+"-"+n, doc)
				p.Name = n
				e.attach(t, p, store.PrincipalTypeToken, id)
				attached = append(attached, p)
			}
			q := url.Values{"principal_type": {"token"}, "principal_id": {id}, "action": {tt.action}, "resource": {tt.resource}}
			rec := e.call(t, http.MethodGet, "/api/v1/iam/simulate?"+q.Encode(), "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			sim := decode[simulation](t, rec)
			want := authorizeResource(tt.abilities, attached, tt.action, tt.resource)
			if sim.Allowed != want {
				t.Errorf("simulator allowed = %v, evaluator = %v", sim.Allowed, want)
			}
			if sim.DecidedBy != tt.wantBy {
				t.Errorf("decided_by = %q, want %q", sim.DecidedBy, tt.wantBy)
			}
			if tt.wantPol != "" && (sim.Deciding == nil || sim.Deciding.PolicyName != id+"-"+tt.wantPol) {
				t.Errorf("deciding = %+v, want policy %q", sim.Deciding, tt.wantPol)
			}
		})
	}
}

func TestSimulatorListsEveryMatchedStatement(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	e.seedToken(t, "multi", []string{AbilityRead})
	for _, p := range []struct{ name, doc string }{
		{"one-allow", docOf("Allow", `"write"`, `"app:web"`)},
		{"two-allow", docOf("Allow", `"write"`, `"app:*"`)},
		{"three-deny", docOf("Deny", `"write"`, `"app:web"`)},
	} {
		e.attach(t, e.seedPolicy(t, p.name, p.doc), store.PrincipalTypeToken, "multi")
	}
	rec := e.call(t, http.MethodGet, "/api/v1/iam/simulate?principal_type=token&principal_id=multi&action=write&resource=app:web", "")
	sim := decode[simulation](t, rec)
	if len(sim.Matched) != 3 || sim.Allowed || sim.DecidedBy != decidedExplicitDeny {
		t.Fatalf("got %+v", sim)
	}
}

func TestSimulatorRejectsBadInput(t *testing.T) {
	e := newBuilderEnv(t)
	for _, q := range []string{
		"principal_type=group&principal_id=x&action=read&resource=app:web",
		"principal_type=token&principal_id=x&action=fly&resource=app:web",
		"principal_type=token&principal_id=x&action=read",
	} {
		if rec := e.call(t, http.MethodGet, "/api/v1/iam/simulate?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
	if rec := e.call(t, http.MethodGet, "/api/v1/iam/simulate?principal_type=token&principal_id=nope&action=read&resource=app:web", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown principal status = %d, want 404", rec.Code)
	}
}

func TestEffectivePermissionsGroupedByAbility(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	e.seedApp(t, "api")
	e.seedDB(t, "main")
	e.seedToken(t, "eff", []string{AbilityRead})
	e.attach(t, e.seedPolicy(t, "w-web", docOf("Allow", `"write"`, `"app:web"`)), store.PrincipalTypeToken, "eff")
	rec := e.call(t, http.MethodGet, "/api/v1/iam/principals/token/eff/effective", "")
	res := decode[effectiveResponse](t, rec)
	byAbility := map[string]effectiveAbility{}
	for _, a := range res.Abilities {
		byAbility[a.Ability] = a
	}
	if !byAbility[AbilityRead].All || byAbility[AbilityRead].Allowed != 3 {
		t.Errorf("read = %+v, want all 3", byAbility[AbilityRead])
	}
	w := byAbility[AbilityWrite]
	if w.Allowed != 1 || !slices.Equal(w.Apps, []string{"web"}) || w.GrantedByPolicy != 1 {
		t.Errorf("write = %+v, want only app web granted by policy", w)
	}
}

func TestAnalyzerFindsEachKind(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	e.seedToken(t, "rooty", []string{AbilityRoot})
	e.seedToken(t, "scoped", []string{AbilityRead})
	attachTo := func(p store.Policy, ptype, id string) { e.attach(t, p, ptype, id) }
	attachTo(e.seedPolicy(t, "allow-all", docOf("Allow", `"*"`, `"*"`)), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "wild-action", docOf("Allow", `"root"`, `"app:web"`)), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "sensitive", docOf("Allow", `"read:sensitive"`, `"*"`)), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "deny-gone", docOf("Deny", `"write"`, `"app:gone"`)), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "allow-gone", docOf("Allow", `"read"`, `"database:gone"`)), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "shadow", `{"Statement":[{"Effect":"Allow","Action":["write"],"Resource":["app:web"]},{"Effect":"Deny","Action":["*"],"Resource":["app:*"]}]}`), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "overlap", `{"Statement":[{"Effect":"Allow","Action":["read","write"],"Resource":["app:*"]},{"Effect":"Deny","Action":["write"],"Resource":["app:web"]}]}`), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "dup", `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]},{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}`), store.PrincipalTypeToken, "scoped")
	attachTo(e.seedPolicy(t, "orphan", docOf("Allow", `"read"`, `"app:web"`)), store.PrincipalTypeToken, "deleted-token")
	e.seedPolicy(t, "unused", docOf("Allow", `"read"`, `"app:web"`))
	e.seedPolicy(t, "cond", `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"],"Condition":{"IpAddress":"10.0.0.0/8"}}]}`)
	e.seedPolicy(t, "broken", `{`)

	res := decode[analysisResponse](t, e.call(t, http.MethodGet, "/api/v1/iam/analyze", ""))
	got := map[string]bool{}
	for _, f := range res.Findings {
		got[f.Kind] = true
		if f.Message == "" || f.Fix == "" && f.Kind != "" {
			t.Errorf("finding %+v needs a message and a fix", f)
		}
	}
	for _, kind := range []string{findingAllowAll, findingWildcardAction, findingSensitiveEverywhere, findingDenyNeverMatches, findingDanglingResource,
		findingUnusedPolicy, findingTokenRoot, findingOrphanAttachment, findingAllowShadowed, findingContradiction, findingDuplicate, findingUnsupportedKey, findingMalformed} {
		if !got[kind] {
			t.Errorf("analyzer did not report %q", kind)
		}
	}
	if res.Score >= 100 || res.Counts[sevCritical] == 0 {
		t.Errorf("score = %d counts = %v, want a penalized score with a critical", res.Score, res.Counts)
	}
	if len(res.Findings) > 1 && slices.Index(severityOrder, res.Findings[0].Severity) > slices.Index(severityOrder, res.Findings[len(res.Findings)-1].Severity) {
		t.Error("findings are not sorted most serious first")
	}
}

func TestAnalyzerCleanPlatformScoresFull(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	e.seedToken(t, "ok", []string{AbilityRead})
	e.attach(t, e.seedPolicy(t, "tidy", docOf("Allow", `"write"`, `"app:web"`)), store.PrincipalTypeToken, "ok")
	res := decode[analysisResponse](t, e.call(t, http.MethodGet, "/api/v1/iam/analyze", ""))
	if res.Score != 100 || len(res.Findings) != 0 {
		t.Errorf("got score %d findings %+v, want 100 and none", res.Score, res.Findings)
	}
}

func TestValidateAgreesWithParseDocument(t *testing.T) {
	docs := []string{
		`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}`,
		`{"Statement":[]}`,
		`{`,
		`{"Statement":[{"Effect":"Maybe","Action":["read"],"Resource":["*"]}]}`,
		`{"Statement":[{"Effect":"Allow","Action":[],"Resource":["*"]}]}`,
		`{"Statement":[{"Effect":"Allow","Action":["fly"],"Resource":["*"]}]}`,
		`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":[]}]}`,
		`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":[" "]}]}`,
		`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["environment-kind:staging"]}]}`,
		`{"Statement":[{"Effect":"Allow","Action":["*"],"Resource":["environment:a*b"]}]}`,
	}
	for _, d := range docs {
		_, perr := ParseDocument(d)
		issues := validateDocumentFields(d, nil)
		if (perr == nil) == hasErrorIssue(issues) {
			t.Errorf("doc %s: ParseDocument err = %v, validate issues = %+v", d, perr, issues)
		}
	}
}

func TestValidateFieldPaths(t *testing.T) {
	issues := validateDocumentFields(`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]},{"Effect":"Deny","Action":["read","fly"],"Resource":["app:a","  "],"Condition":{}}]}`, nil)
	paths := []string{}
	for _, i := range issues {
		paths = append(paths, i.Path)
	}
	for _, want := range []string{"Statement[1].Action[1]", "Statement[1].Resource[1]", "Statement[1].Condition"} {
		if !slices.Contains(paths, want) {
			t.Errorf("paths = %v, missing %q", paths, want)
		}
	}
}

func TestMatchPatternCounts(t *testing.T) {
	e := newBuilderEnv(t)
	for _, n := range []string{"web", "web-worker", "api"} {
		e.seedApp(t, n)
	}
	e.seedDB(t, "main")
	rec := e.call(t, http.MethodPost, "/api/v1/iam/resources/match", `{"resources":["*","app:web*","database:main","app:nope","environment:"]}`)
	got := decode[[]resourceMatch](t, rec)
	if got[0].Apps != 3 || got[0].Databases != 1 {
		t.Errorf("* = %+v", got[0])
	}
	if got[1].Apps != 2 || got[2].Databases != 1 || got[3].Apps != 0 || got[4].Error == "" {
		t.Errorf("got %+v", got)
	}
}

func TestTemplateRenderExpandsParameters(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	e.seedDB(t, "main")
	ctx := context.Background()
	if err := e.db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "shop", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := e.db.SaveProject(ctx, store.Project{ID: "proj_empty", Name: "empty", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"env_a", "env_b"} {
		if err := e.db.SaveEnvironment(ctx, store.Environment{ID: env, ProjectID: "proj_1", Name: env}); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name     string
		template string
		body     string
		code     int
		resource []string
	}{
		{"app", "app-operator", `{"params":{"app":"web"}}`, 200, []string{"app:web"}},
		{"app missing", "app-operator", `{"params":{"app":"ghost"}}`, 400, nil},
		{"app required", "app-operator", `{}`, 400, nil},
		{"database", "database-owner", `{"params":{"database":"main"}}`, 200, []string{"database:main"}},
		{"project", "project-deployer", `{"params":{"project":"proj_1"}}`, 200, []string{"environment:env_a", "environment:env_b"}},
		{"project without environments", "project-deployer", `{"params":{"project":"proj_empty"}}`, 400, nil},
		{"unknown param", "app-operator", `{"params":{"app":"web","x":"y"}}`, 400, nil},
		{"unknown template", "nope", `{}`, 404, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.call(t, http.MethodPost, "/api/v1/iam/policy-templates/"+tt.template+"/render", tt.body)
			if rec.Code != tt.code {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.code, rec.Body.String())
			}
			if tt.code != 200 {
				return
			}
			res := decode[renderTemplateResponse](t, rec)
			if !slices.Equal(res.Document.Statement[0].Resource, tt.resource) {
				t.Errorf("resources = %v, want %v", res.Document.Statement[0].Resource, tt.resource)
			}
			raw, _ := json.Marshal(res.Document)
			if _, err := ParseDocument(string(raw)); err != nil {
				t.Errorf("rendered document invalid: %v", err)
			}
		})
	}
}

func TestRootGuardRefusesLastRootLockout(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedApp(t, "web")
	admin := rootUserID(t, e)
	blocking := []string{
		docOf("Deny", `"root"`, `"*"`),
		docOf("Deny", `"*"`, `"*"`),
		docOf("Deny", `"root"`, `"app:*"`),
	}
	for i, doc := range blocking {
		p := e.seedPolicy(t, fmt.Sprintf("block-%d", i), doc)
		body := `{"principal_type":"user","principal_id":"` + admin + `"}`
		if rec := e.call(t, http.MethodPost, "/api/v1/iam/policies/"+p.ID+"/attachments", body); rec.Code != http.StatusConflict {
			t.Errorf("doc %s: attach status = %d, want 409, body = %s", doc, rec.Code, rec.Body.String())
		}
	}
	allowed := e.seedPolicy(t, "fine", docOf("Deny", `"write"`, `"app:web"`))
	if rec := e.call(t, http.MethodPost, "/api/v1/iam/policies/"+allowed.ID+"/attachments", `{"principal_type":"user","principal_id":"`+admin+`"}`); rec.Code != http.StatusNoContent {
		t.Errorf("narrow deny attach status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/iam/policies/"+allowed.ID, `{"name":"fine","document":`+docOf("Deny", `"root"`, `"*"`)+`}`); rec.Code != http.StatusConflict {
		t.Errorf("update to lockout status = %d, want 409", rec.Code)
	}
}

func TestRootGuardAllowsWhenAnotherRootRemains(t *testing.T) {
	e := newBuilderEnv(t)
	e.seedToken(t, "spare-root", []string{AbilityRoot})
	admin := rootUserID(t, e)
	p := e.seedPolicy(t, "deny-root-for-admin", docOf("Deny", `"root"`, `"*"`))
	if rec := e.call(t, http.MethodPost, "/api/v1/iam/policies/"+p.ID+"/attachments", `{"principal_type":"user","principal_id":"`+admin+`"}`); rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 while a second root remains, body = %s", rec.Code, rec.Body.String())
	}
}

func rootUserID(t *testing.T, e builderEnv) string {
	t.Helper()
	users, err := e.db.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if slices.Contains(u.Abilities, AbilityRoot) {
			return u.ID
		}
	}
	t.Fatal("no root user")
	return ""
}

func TestPreviewShowsGainsAndLosses(t *testing.T) {
	e := newBuilderEnv(t)
	for _, n := range []string{"a", "b", "c"} {
		e.seedApp(t, n)
	}
	e.seedToken(t, "reader", []string{AbilityRead})
	grant := e.seedPolicy(t, "grant-write", docOf("Allow", `"write"`, `"app:*"`))

	rec := e.call(t, http.MethodPost, "/api/v1/iam/preview", `{"policy_id":"`+grant.ID+`","attach":[{"principal_type":"token","principal_id":"reader"}]}`)
	res := decode[previewResponse](t, rec)
	if len(res.Changes) != 1 || len(res.Changes[0].Gains) != 1 || res.Changes[0].Gains[0].Ability != AbilityWrite || res.Changes[0].Gains[0].Count != 3 {
		t.Fatalf("attach preview = %+v", res)
	}
	if len(res.Changes[0].Losses) != 0 || res.UsageNote == "" {
		t.Errorf("unexpected losses or missing usage note: %+v", res)
	}

	e.attach(t, grant, store.PrincipalTypeToken, "reader")
	narrow := `{"policy_id":"` + grant.ID + `","document":` + docOf("Allow", `"write"`, `"app:a"`) + `}`
	res = decode[previewResponse](t, e.call(t, http.MethodPost, "/api/v1/iam/preview", narrow))
	if len(res.Changes) != 1 || len(res.Changes[0].Losses) != 1 || res.Changes[0].Losses[0].Count != 2 {
		t.Fatalf("update preview = %+v", res)
	}

	bad := decode[previewResponse](t, e.call(t, http.MethodPost, "/api/v1/iam/preview", `{"document":{"Statement":[]}}`))
	if bad.Valid || len(bad.Issues) == 0 {
		t.Errorf("invalid document preview = %+v", bad)
	}
}

func TestPolicyVersionsRecordedAndDiffed(t *testing.T) {
	e := newBuilderEnv(t)
	rec := e.call(t, http.MethodPost, "/api/v1/iam/policies", `{"name":"versioned","document":`+docOf("Allow", `"read"`, `"app:web"`)+`}`)
	created := decode[policyResource](t, rec)
	e.call(t, http.MethodPut, "/api/v1/iam/policies/"+created.ID, `{"name":"versioned","document":`+docOf("Allow", `"read","write"`, `"app:web"`)+`}`)

	versions := decode[[]policyVersionResource](t, e.call(t, http.MethodGet, "/api/v1/iam/policies/"+created.ID+"/versions", ""))
	if len(versions) != 2 || versions[0].Version != 2 {
		t.Fatalf("versions = %+v", versions)
	}
	changes := versions[0].Changes
	if len(changes) != 2 || changes[0].Change != changeRemoved || changes[1].Change != changeAdded {
		t.Errorf("changes = %+v, want one removed and one added", changes)
	}
}

func TestDiffDocumentsIgnoresOrder(t *testing.T) {
	a := docOf("Allow", `"read","write"`, `"app:a","app:b"`)
	b := docOf("Allow", `"write","read"`, `"app:b","app:a"`)
	if got := diffDocuments(a, b); len(got) != 0 {
		t.Errorf("reordered document diff = %+v, want none", got)
	}
}

func TestPatternCoverage(t *testing.T) {
	tests := []struct {
		a, b            string
		covers, overlap bool
	}{
		{"*", "app:web", true, true},
		{"app:*", "app:web", true, true},
		{"app:web", "app:*", false, true},
		{"app:prod*", "app:*", false, true},
		{"app:prod*", "app:production", true, true},
		{"app:web", "app:api", false, false},
		{"database:*", "app:web", false, false},
	}
	for _, tt := range tests {
		if got := patternCovers(tt.a, tt.b); got != tt.covers {
			t.Errorf("patternCovers(%q,%q) = %v, want %v", tt.a, tt.b, got, tt.covers)
		}
		if got := patternsOverlap(tt.a, tt.b); got != tt.overlap {
			t.Errorf("patternsOverlap(%q,%q) = %v, want %v", tt.a, tt.b, got, tt.overlap)
		}
	}
}

func TestCatalogDeclaresConditionSupport(t *testing.T) {
	e := newBuilderEnv(t)
	cat := decode[iamCatalogResponse](t, e.call(t, http.MethodGet, "/api/v1/iam/catalog", ""))
	if len(cat.Abilities) != len(validAbilities) {
		t.Errorf("abilities = %d, want %d", len(cat.Abilities), len(validAbilities))
	}
	if !slices.Equal(cat.Conditions.Supported, []string{"environment_kind"}) {
		t.Errorf("supported conditions = %v", cat.Conditions.Supported)
	}
}
