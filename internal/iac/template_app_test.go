package iac

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// tplFake answers the template endpoints and records installs; every other
// path goes to the shared in-memory control plane.
type tplFake struct {
	*fakeCP
	installs  []map[string]any
	installed map[string]string
}

const tplManifest = `{"id":"ghost","manifest":{"env":[{"key":"ADMIN_PASSWORD","required":true},{"key":"TZ","default":"UTC"}],"domains":[{"service":"web"}]}}`

func newTplFake() *tplFake { return &tplFake{fakeCP: newFake(), installed: map[string]string{}} }

func (f *tplFake) Do(ctx context.Context, method, path string, body, out any) error {
	switch {
	case method == http.MethodGet && path == "/api/v1/service-templates/ghost":
		return decodeInto(tplManifest, out)
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v1/service-templates/"):
		return errNotFound
	case method == http.MethodPost && path == "/api/v1/service-templates/ghost/install":
		b := body.(map[string]any)
		f.installs = append(f.installs, b)
		f.installed[b["name"].(string)] = "ghost"
		return nil
	case method == http.MethodGet && strings.HasSuffix(path, "/compose/revisions") && strings.HasPrefix(path, "/api/v1/apps/"):
		name := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/apps/"), "/compose/revisions")
		if id, ok := f.installed[name]; ok {
			return decodeInto(`{"revisions":[{"revision":1,"source":"template","template_id":"`+id+`"}]}`, out)
		}
	}
	return f.fakeCP.Do(ctx, method, path, body, out)
}

func decodeInto(raw string, out any) error {
	if out == nil {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

func tplPlan(t *testing.T, f Doer, src string, opts Options) Plan {
	t.Helper()
	res := mustBuild(t, src, opts)
	st, err := Load(context.Background(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	return PlanFor(st, res, opts)
}

const tplDoc = "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec:\n  template: ghost\n  env: {ADMIN_PASSWORD: \"${{ env.PW }}\"}\n  domains: {web: blog.example.com}\n"

func TestTemplateAppBuildValidation(t *testing.T) {
	cases := []struct{ name, doc, want string }{
		{"missing template", "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec: {}\n", "template"},
		{"unknown field", "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec: {template: ghost, image: x}\n", "image"},
		{"bad name", "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec: {template: ghost, name: Blog_1}\n", "name"},
		{"secret literal", "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec: {template: ghost, env: {ADMIN_PASSWORD: hunter2hunter2hunter2}}\n", "looks like a secret"},
		{"missing var", tplDoc, "PW"},
	}
	for _, c := range cases {
		docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(c.doc)}})
		if len(issues) == 0 {
			_, issues = Build(docs, Options{})
		}
		var all []string
		for _, is := range issues {
			all = append(all, is.Path+" "+is.Message)
		}
		if !strings.Contains(strings.Join(all, "\n"), c.want) {
			t.Errorf("%s: want %q in %v", c.name, c.want, all)
		}
	}
}

func TestTemplateAppPlanChecksTemplateInputs(t *testing.T) {
	opts := Options{Vars: map[string]string{"PW": "s3cret-value"}}
	cases := []struct{ name, doc, want string }{
		{"unknown template", strings.ReplaceAll(tplDoc, "ghost", "nope"), "not in the catalog"},
		{"unknown env key", strings.ReplaceAll(tplDoc, "ADMIN_PASSWORD", "BOGUS"), "no env setting BOGUS"},
		{"missing required", "version: 1\nkind: TemplateApp\nmetadata: {name: blog}\nspec: {template: ghost}\n", "needs a value for env: ADMIN_PASSWORD"},
		{"bad domain service", strings.ReplaceAll(tplDoc, "web:", "api:"), "does not offer a domain for: api"},
	}
	for _, c := range cases {
		p := tplPlan(t, newTplFake(), c.doc, opts)
		if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, c.want) {
			t.Errorf("%s: plan = %+v", c.name, p.Changes)
		}
	}
}

func TestTemplateAppApplyIsIdempotentAndHidesEnv(t *testing.T) {
	f := newTplFake()
	opts := Options{Vars: map[string]string{"PW": "s3cret-value"}}
	p := tplPlan(t, f, tplDoc, opts)
	if p.Summary.Create != 1 {
		t.Fatalf("plan = %+v", p.Changes)
	}
	for _, c := range p.Changes {
		for _, fc := range c.Fields {
			if strings.Contains(fc.New, "s3cret-value") {
				t.Fatalf("plan leaks env value: %+v", fc)
			}
		}
	}
	res := mustBuild(t, tplDoc, opts)
	out, err := Apply(context.Background(), f, res, opts, "")
	if err != nil || !out.OK() || out.Applied != 1 {
		t.Fatalf("apply = %+v, %v", out, err)
	}
	if len(f.installs) != 1 || f.installs[0]["name"] != "blog" || f.installs[0]["env"].(map[string]string)["ADMIN_PASSWORD"] != "s3cret-value" {
		t.Fatalf("install body = %+v", f.installs)
	}
	again := tplPlan(t, f, tplDoc, opts)
	if again.Pending() || again.Summary.Noop != 1 {
		t.Fatalf("second plan is not a no-op: %+v", again.Changes)
	}
	res = mustBuild(t, tplDoc, opts)
	if _, err := Apply(context.Background(), f, res, opts, ""); err != nil || len(f.installs) != 1 {
		t.Fatalf("re-apply installed again: %d, %v", len(f.installs), err)
	}
}

func TestTemplateAppRefusesForeignApp(t *testing.T) {
	f := newTplFake()
	f.installed["blog"] = "other"
	p := tplPlan(t, f, tplDoc, Options{Vars: map[string]string{"PW": "x"}})
	if p.Summary.Error != 1 || p.Changes[0].Code != CodeOwnershipConflict {
		t.Fatalf("plan = %+v", p.Changes)
	}
}

func TestTemplateAppLatestRevisionAndChangedInputs(t *testing.T) {
	opts := Options{Vars: map[string]string{"PW": "x"}}
	f := newTplFake()
	f.installed["blog"] = "ghost"
	res := mustBuild(t, tplDoc, opts)
	applied := map[string]string{"blog": templateInputsDigest(res[0].tmpl)}
	same := opts
	same.ComposeEnvApplied = applied
	if p := tplPlan(t, f, tplDoc, same); p.Summary.Noop != 1 {
		t.Fatalf("same inputs: %+v", p.Changes)
	}
	changed := opts
	changed.ComposeEnvApplied = map[string]string{"blog": "other"}
	if p := tplPlan(t, f, tplDoc, changed); p.Changes[0].Code != CodeTemplateAppInputsChanged {
		t.Fatalf("changed inputs: %+v", p.Changes)
	}
	f.installed["blog"] = "other"
	if p := tplPlan(t, f, tplDoc, opts); p.Changes[0].Code != CodeOwnershipConflict {
		t.Fatalf("latest is another template: %+v", p.Changes)
	}
}

func TestTemplateAppProjectScopeAndStackVars(t *testing.T) {
	docs, _ := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(tplDoc)}})
	if got := FilterProject(docs, "demo"); len(got) != 1 {
		t.Fatalf("filter dropped the document: %d", len(got))
	}
	_, issues := Build(docs, Options{Project: "demo", Vars: map[string]string{"PW": "x"}})
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "no project") {
		t.Fatalf("project scope issues = %+v", issues)
	}
	_, issues = Build(docs, Options{NoTemplateEnvVars: true, Vars: map[string]string{"PW": "x"}})
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "cannot use") {
		t.Fatalf("stack vars issues = %+v", issues)
	}
}

func TestTemplateAppHostClashIsCaught(t *testing.T) {
	src := tplDoc + "---\nversion: 1\nkind: Domain\nmetadata: {name: blog.example.com}\nspec: {app: other}\n"
	docs, _ := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(src)}})
	_, issues := Build(docs, Options{Vars: map[string]string{"PW": "x"}})
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "claimed by both") {
		t.Fatalf("issues = %+v", issues)
	}
}
