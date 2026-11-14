package iac

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const execDoc = "version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  compose:\n    path: compose/shop.yaml\n"

const (
	composeA = "services:\n  web:\n    image: nginx:1.27\n  worker:\n    image: busybox:1.36\n"
	composeB = "services:\n  web:\n    image: nginx:1.28\n"
)

// composeRoutes go first so they win over any generic app route.
func (f *fakeCP) composeRoutes() {
	saved := f.routes
	f.routes = nil
	f.composeRevs = map[string][]map[string]any{}
	f.route("POST", `/api/v1/apps/([^/?]+)/compose(?:\?profiles=(.*))?`, func(m []string, _ map[string]any) (any, error) {
		if f.lastCompose == nil {
			return nil, &StatusError{Status: 400, Message: "no compose body"}
		}
		prof, _ := url.QueryUnescape(m[2])
		var profiles []string
		if prof != "" {
			profiles = strings.Split(prof, ",")
		}
		revs := f.composeRevs[m[1]]
		f.composeRevs[m[1]] = append(revs, map[string]any{"revision": len(revs) + 1, "compose": string(f.lastCompose.Data), "profiles": profiles})
		return map[string]any{"app_id": m[1]}, nil
	})
	f.route("GET", `/api/v1/apps/([^/]+)/compose/revisions`, func(m []string, _ map[string]any) (any, error) {
		revs := f.composeRevs[m[1]]
		out := []any{}
		for i := len(revs) - 1; i >= 0; i-- {
			out = append(out, map[string]any{"revision": revs[i]["revision"]})
		}
		return map[string]any{"revisions": out}, nil
	})
	f.route("GET", `/api/v1/apps/([^/]+)/compose/revisions/(\d+)`, func(m []string, _ map[string]any) (any, error) {
		n, _ := strconv.Atoi(m[2])
		revs := f.composeRevs[m[1]]
		if n < 1 || n > len(revs) {
			return nil, errNotFound
		}
		return revs[n-1], nil
	})
	f.routes = append(f.routes, saved...)
}

func enableComposeExec(t *testing.T) {
	t.Helper()
	SetComposeSource(func() ComposeLimits {
		return ComposeLimits{Enabled: true, MaxFileBytes: 64 << 10, MaxFiles: 4, MaxTotalBytes: 256 << 10}
	})
	t.Cleanup(func() { SetComposeSource(func() ComposeLimits { return ComposeLimits{} }) })
}

func composePlan(t *testing.T, f *fakeCP, body string, opts Options) ([]*Resource, Plan, Options) {
	t.Helper()
	opts.Attachments = map[string][]byte{"compose/shop.yaml": []byte(body)}
	res, issues := buildCompose(t, execDoc, opts.Attachments)
	if len(issues) > 0 {
		t.Fatalf("build: %v", issues)
	}
	st, err := Load(context.Background(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res, PlanFor(st, res, opts), opts
}

func TestComposeStrictRefusalsAtPlan(t *testing.T) {
	enableComposeExec(t)
	const canary = "Zq8vN2kLp4Xr7TmW9cHs"
	t.Setenv("LR_CANARY_HOST_VAR", "canary-host-value")
	cases := map[string]struct{ body, code string }{
		"privileged":  {"services:\n  web:\n    image: nginx\n    privileged: true\n", CodeComposeRejected},
		"build":       {"services:\n  web:\n    build: .\n", CodeComposeUnsupportedKey},
		"secret":      {"services:\n  web:\n    image: nginx\n    environment:\n      DB_PASSWORD: " + canary + "\n", CodeComposeSecretLiteral},
		"undeclared":  {"services:\n  web:\n    image: nginx:${LR_CANARY_HOST_VAR}\n", CodeComposeVarMissing},
		"bind":        {"services:\n  web:\n    image: nginx\n    volumes: [\"/etc:/host\"]\n", CodeComposeRejected},
		"unknown_top": {"services:\n  web: {image: nginx}\ninclude: [other.yaml]\n", CodeComposeUnsupportedKey},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, issues := buildCompose(t, execDoc, map[string][]byte{"compose/shop.yaml": []byte(c.body)})
			raw, _ := json.Marshal(issues)
			if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "canary-host-value") {
				t.Fatalf("a value leaked into the issues: %s", raw)
			}
			for _, i := range issues {
				if i.Code == c.code && i.File == "compose/shop.yaml" {
					return
				}
			}
			t.Fatalf("want %s, got %s", c.code, raw)
		})
	}
}

func TestComposeApplyLifecycle(t *testing.T) {
	enableComposeExec(t)
	f := newFake()
	res, plan, opts := composePlan(t, f, composeA, Options{})
	if len(plan.Changes) != 1 || plan.Changes[0].Action != ActionCreate || len(plan.Changes[0].Services) != 2 {
		t.Fatalf("create plan: %+v", plan.Changes)
	}
	if _, err := Apply(context.Background(), f, res, opts, plan.Hash); err != nil {
		t.Fatal(err)
	}
	if f.lastCompose == nil || string(f.lastCompose.Data) != composeA {
		t.Fatal("the compose endpoint must get the raw attachment bytes")
	}
	_, plan, _ = composePlan(t, f, composeA, Options{})
	if plan.Changes[0].Action != ActionNoop {
		t.Fatalf("re-plan must be a noop: %+v", plan.Changes[0])
	}
	_, planB, _ := composePlan(t, f, composeB, Options{})
	c := planB.Changes[0]
	if c.Action != ActionUpdate || len(c.Fields) != 1 || c.Fields[0].Old == c.Fields[0].New {
		t.Fatalf("changed file must update with a new digest: %+v", c)
	}
	if last := c.Services[len(c.Services)-1]; last.Service != "worker" || last.Action != ActionDelete {
		t.Fatalf("dropped service must show as removed: %+v", c.Services)
	}
	if planB.Hash == plan.Hash {
		t.Fatal("a compose change must change the plan hash")
	}
	f.lastCompose = nil
	_, _, optsB := composePlan(t, f, composeB, Options{})
	stale, _, _ := composePlan(t, f, composeB+"# edit\n", Options{})
	if _, err := Apply(context.Background(), f, stale, optsB, planB.Hash); err == nil || f.lastCompose != nil {
		t.Fatalf("a digest that differs from the approved plan must refuse: %v", err)
	}
	f.route("GET", `/api/v1/apps/([^/]+)/protection`, func(m []string, _ map[string]any) (any, error) {
		return map[string]any{"protected": m[1] == "shop"}, nil
	})
	_, planP, _ := composePlan(t, f, composeB, Options{})
	if planP.Changes[0].Code != CodeProtectedResource {
		t.Fatalf("protected app must refuse an update: %+v", planP.Changes[0])
	}
}

func TestComposeNoDeployAndGate(t *testing.T) {
	enableComposeExec(t)
	_, plan, _ := composePlan(t, newFake(), composeA, Options{NoDeploy: true})
	if plan.Changes[0].Code != CodeComposeNoDeployUnsupported {
		t.Fatalf("no_deploy must refuse: %+v", plan.Changes[0])
	}
	SetComposeSource(func() ComposeLimits { return ComposeLimits{} })
	_, issues := buildCompose(t, execDoc, map[string][]byte{"compose/shop.yaml": []byte(composeA)})
	if len(issues) == 0 || issues[0].Code != CodeUnavailable {
		t.Fatalf("gate off must refuse: %v", issues)
	}
}
