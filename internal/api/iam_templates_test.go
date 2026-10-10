package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func renderedTemplatePolicy(t *testing.T, id string, params map[string]string) store.Policy {
	t.Helper()
	tpl, ok := findPolicyTemplate(id)
	if !ok {
		t.Fatalf("template %q missing", id)
	}
	raw, err := json.Marshal(tpl.build(params))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDocument(string(raw)); err != nil {
		t.Fatalf("template %q renders an invalid document: %v", id, err)
	}
	return store.Policy{Document: string(raw)}
}

func TestPolicyTemplates_AllValidate(t *testing.T) {
	for _, tpl := range policyTemplates {
		t.Run(tpl.ID, func(t *testing.T) {
			renderedTemplatePolicy(t, tpl.ID, map[string]string{templateParamEnvironment: "env_dev"})
			renderedTemplatePolicy(t, tpl.ID, tpl.placeholderParams())
		})
	}
	if len(policyTemplates) != 10 {
		t.Fatalf("templates = %d, want 10", len(policyTemplates))
	}
}

func TestPolicyTemplates_Behavior(t *testing.T) {
	prod := &store.EnvironmentRef{ID: "env_production", Kind: "production"}
	dev := &store.EnvironmentRef{ID: "env_dev", Kind: "dev"}
	uat := &store.EnvironmentRef{ID: "env_uat", Kind: "uat"}
	custom := &store.EnvironmentRef{ID: "env_x", Kind: "custom"}
	tests := []struct {
		template string
		base     []string
		ability  string
		ref      *store.EnvironmentRef
		want     bool
	}{
		{"deployer-nonprod", nil, AbilityDeploy, dev, true},
		{"deployer-nonprod", nil, AbilityDeploy, uat, true},
		{"deployer-nonprod", nil, AbilityDeploy, prod, false},
		{"deployer-nonprod", []string{AbilityDeploy}, AbilityDeploy, prod, false},
		{"deployer-nonprod", nil, AbilityDeploy, custom, false},
		{"deployer-nonprod", nil, AbilityDeploy, nil, false},
		{"deployer-nonprod", nil, AbilityWrite, dev, true},
		{"deployer-nonprod", nil, AbilityRoot, dev, false},
		{"ai-operator-nonprod", nil, AbilityDeploy, dev, true},
		{"ai-operator-nonprod", []string{AbilityDeploy}, AbilityDeploy, prod, false},
		{"ai-operator-nonprod", []string{AbilityRoot}, AbilityRoot, dev, false},
		{"read-only", []string{AbilityRead, AbilityWrite}, AbilityWrite, dev, false},
		{"read-only", nil, AbilityRead, dev, true},
		{"production-approver", nil, AbilityDeploy, prod, true},
		{"production-approver", nil, AbilityDeploy, dev, false},
		{"production-approver", nil, AbilityWrite, prod, false},
		{"guest-one-environment", nil, AbilityRead, dev, true},
		{"guest-one-environment", nil, AbilityRead, prod, false},
		{"guest-one-environment", []string{AbilityWrite}, AbilityWrite, dev, false},
	}
	for _, tt := range tests {
		name := tt.template + " " + tt.ability
		if tt.ref != nil {
			name += " " + tt.ref.Kind
		}
		t.Run(name, func(t *testing.T) {
			p := renderedTemplatePolicy(t, tt.template, map[string]string{templateParamEnvironment: "env_dev"})
			got := authorizeResource(tt.base, []store.Policy{p}, tt.ability, "app:web", environmentResources(tt.ref)...)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPolicyTemplates_HTTP(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)
	target := storeUserWithAbilitiesForTest(t, db, "target@example.com", []string{AbilityRead})

	do := func(c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, c, method, path, body))
		return rec
	}

	rec := do(readerCookie, http.MethodGet, "/api/v1/iam/policy-templates", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list policyTemplateListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, tpl := range list.Templates {
		ids = append(ids, tpl.ID)
	}
	if strings.Join(ids, ",") != "read-only,guest-one-environment,deployer-nonprod,ai-operator-nonprod,database-read-only,database-operator,database-owner,production-approver,app-operator,project-deployer" {
		t.Fatalf("ids = %v", ids)
	}

	if rec := do(readerCookie, http.MethodPost, "/api/v1/iam/policy-templates/read-only/apply", `{}`); rec.Code != http.StatusForbidden {
		t.Fatalf("non-root apply = %d, want 403", rec.Code)
	}
	if rec := do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/nope/apply", `{}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown template = %d, want 404", rec.Code)
	}
	bad := []string{
		`{}`, // missing environment param for guest template handled below
		`not json`,
	}
	if rec := do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/guest-one-environment/apply", bad[0]); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing param = %d, want 400", rec.Code)
	}
	if rec := do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/read-only/apply", bad[1]); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json = %d, want 400", rec.Code)
	}
	for _, body := range []string{
		`{"params":{"environment":"env_missing"}}`,
		`{"params":{"environment":"a b"}}`,
		`{"params":{"environment":"env_dev","extra":"x"}}`,
	} {
		if rec := do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/guest-one-environment/apply", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", body, rec.Code)
		}
	}
	if rec := do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/read-only/apply", `{"attach":{"principal_type":"group","principal_id":"x"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad attach = %d, want 400", rec.Code)
	}

	body := `{"params":{"environment":"env_dev"},"attach":{"principal_type":"user","principal_id":"` + target.ID + `"}}`
	rec = do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/guest-one-environment/apply", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply = %d, body = %s", rec.Code, rec.Body.String())
	}
	var applied applyTemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if !applied.Attached || applied.Policy.Name != "guest-one-environment-env_dev" {
		t.Fatalf("applied = %+v", applied)
	}
	attached, err := db.ListPoliciesForPrincipal(t.Context(), store.PrincipalTypeUser, target.ID)
	if err != nil || len(attached) != 1 || attached[0].ID != applied.Policy.ID {
		t.Fatalf("attached = %+v, err = %v", attached, err)
	}
	if rec := do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/guest-one-environment/apply", body); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
	rec = do(cookie, http.MethodPost, "/api/v1/iam/policy-templates/read-only/apply", `{"name":"my-ro"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply without attach = %d", rec.Code)
	}
}
